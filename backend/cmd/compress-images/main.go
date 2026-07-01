package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/image/draw"
)

type result struct {
	key         string
	originalSize int64
	newSize      int64
	skipped      bool
	skipReason   string
	err          error
}

func main() {
	bucket := flag.String("bucket", "", "S3 bucket name (required)")
	prefix := flag.String("prefix", "uploads/", "S3 key prefix to scan")
	dryRun := flag.Bool("dry-run", false, "Simulate without uploading")
	skipSize := flag.Int64("skip-size", 300*1024, "Skip images smaller than this (bytes)")
	maxDimension := flag.Int("max-dimension", 1200, "Max width/height in pixels")
	quality := flag.Int("quality", 80, "JPEG output quality (1-100)")
	concurrency := flag.Int("concurrency", 5, "Number of parallel workers")
	flag.Parse()

	if *bucket == "" {
		fmt.Fprintln(os.Stderr, "Error: -bucket is required")
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received %s, shutting down gracefully...", sig)
		cancel()
	}()

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("ap-northeast-1"))
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	client := s3.NewFromConfig(cfg)

	keys, err := listObjects(ctx, client, *bucket, *prefix)
	if err != nil {
		log.Fatalf("Failed to list objects: %v", err)
	}
	log.Printf("Found %d objects in s3://%s/%s", len(keys), *bucket, *prefix)

	if len(keys) == 0 {
		log.Println("No objects to process.")
		return
	}

	jobs := make(chan string, len(keys))
	for _, k := range keys {
		jobs <- k
	}
	close(jobs)

	results := make(chan result, len(keys))
	var wg sync.WaitGroup

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range jobs {
				if ctx.Err() != nil {
					return
				}
				r := processImage(ctx, client, *bucket, key, *skipSize, *maxDimension, *quality, *dryRun)
				results <- r
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var processed, skipped, errored int64
	var savedBytes int64

	for r := range results {
		if r.err != nil {
			atomic.AddInt64(&errored, 1)
			log.Printf("ERROR %s: %v", r.key, r.err)
			continue
		}
		if r.skipped {
			atomic.AddInt64(&skipped, 1)
			log.Printf("SKIP  %s (%s)", r.key, r.skipReason)
			continue
		}
		atomic.AddInt64(&processed, 1)
		saved := r.originalSize - r.newSize
		atomic.AddInt64(&savedBytes, saved)
		action := "COMPRESSED"
		if *dryRun {
			action = "DRY-RUN"
		}
		log.Printf("%s %s: %s → %s (-%s, %.0f%% reduction)",
			action, r.key,
			humanBytes(r.originalSize), humanBytes(r.newSize),
			humanBytes(saved),
			float64(saved)/float64(r.originalSize)*100,
		)
	}

	log.Println("========== Summary ==========")
	log.Printf("Processed: %d", processed)
	log.Printf("Skipped:   %d", skipped)
	log.Printf("Errors:    %d", errored)
	log.Printf("Saved:     %s", humanBytes(savedBytes))
	if *dryRun {
		log.Println("(dry-run mode — no files were modified)")
	}
}

func listObjects(ctx context.Context, client *s3.Client, bucket, prefix string) ([]string, error) {
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			lower := strings.ToLower(key)
			if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".png") {
				keys = append(keys, key)
			}
		}
	}
	return keys, nil
}

func processImage(ctx context.Context, client *s3.Client, bucket, key string, skipSize int64, maxDim, quality int, dryRun bool) result {
	// Download
	getOut, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return result{key: key, err: fmt.Errorf("download: %w", err)}
	}
	defer getOut.Body.Close()

	originalSize := aws.ToInt64(getOut.ContentLength)

	if originalSize <= skipSize {
		io.Copy(io.Discard, getOut.Body)
		return result{key: key, skipped: true, skipReason: fmt.Sprintf("size %s <= %s", humanBytes(originalSize), humanBytes(skipSize))}
	}

	data, err := io.ReadAll(getOut.Body)
	if err != nil {
		return result{key: key, err: fmt.Errorf("read body: %w", err)}
	}

	// Decode
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return result{key: key, skipped: true, skipReason: fmt.Sprintf("decode error: %v", err)}
	}

	// Resize if needed
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	newW, newH := fitDimensions(w, h, maxDim)

	if newW != w || newH != h {
		dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	// Encode as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return result{key: key, err: fmt.Errorf("encode: %w", err)}
	}

	newSize := int64(buf.Len())

	// If compressed is larger (rare for PNG→JPEG), skip
	if newSize >= originalSize {
		return result{key: key, skipped: true, skipReason: fmt.Sprintf("no size reduction (%s → %s, format=%s)", humanBytes(originalSize), humanBytes(newSize), format)}
	}

	if dryRun {
		return result{key: key, originalSize: originalSize, newSize: newSize}
	}

	// Upload
	contentType := "image/jpeg"
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return result{key: key, err: fmt.Errorf("upload: %w", err)}
	}

	return result{key: key, originalSize: originalSize, newSize: newSize}
}

func fitDimensions(w, h, maxDim int) (int, int) {
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	if w >= h {
		return maxDim, h * maxDim / w
	}
	return w * maxDim / h, maxDim
}

func humanBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)
	switch {
	case b >= MB:
		return fmt.Sprintf("%.1fMB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
