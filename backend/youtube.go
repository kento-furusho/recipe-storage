package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// isYouTubeURL はURLがYouTubeかどうか判定する
func isYouTubeURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	return strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be")
}

// extractYouTubeVideoID はYouTube URLから動画IDを抽出する
func extractYouTubeVideoID(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	host := strings.ToLower(parsed.Host)

	// youtu.be/xxx
	if strings.Contains(host, "youtu.be") {
		id := strings.TrimPrefix(parsed.Path, "/")
		if id == "" {
			return "", fmt.Errorf("no video ID in youtu.be URL")
		}
		return id, nil
	}

	// youtube.com/watch?v=xxx or youtube.com/shorts/xxx
	if strings.Contains(host, "youtube.com") {
		if strings.HasPrefix(parsed.Path, "/shorts/") {
			id := strings.TrimPrefix(parsed.Path, "/shorts/")
			if id == "" {
				return "", fmt.Errorf("no video ID in shorts URL")
			}
			return id, nil
		}
		v := parsed.Query().Get("v")
		if v != "" {
			return v, nil
		}
		return "", fmt.Errorf("no video ID in youtube.com URL")
	}

	return "", fmt.Errorf("not a YouTube URL")
}

// fetchYouTubeVideoInfo はYouTube Data API v3でタイトル・説明文を取得する
func fetchYouTubeVideoInfo(ctx context.Context, videoID string) (title, description string, err error) {
	if youtubeAPIKey == "" {
		return "", "", fmt.Errorf("YOUTUBE_API_KEY is not set")
	}

	apiURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/videos?id=%s&part=snippet&key=%s", url.QueryEscape(videoID), url.QueryEscape(youtubeAPIKey))

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch YouTube API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("YouTube API returned status %d", resp.StatusCode)
	}

	var result struct {
		Items []struct {
			Snippet struct {
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"snippet"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode YouTube API response: %w", err)
	}

	if len(result.Items) == 0 {
		return "", "", fmt.Errorf("video not found: %s", videoID)
	}

	return result.Items[0].Snippet.Title, result.Items[0].Snippet.Description, nil
}

// buildPromptForYouTube はYouTube動画情報からLLM用プロンプトを構築する
func buildPromptForYouTube(title, description, sourceURL string) string {
	var prompt strings.Builder

	prompt.WriteString("以下のYouTube料理動画の情報からレシピを抽出してください。\n\n")

	prompt.WriteString("【動画タイトル】\n")
	prompt.WriteString(title + "\n\n")

	if description != "" {
		prompt.WriteString("【説明欄】\n")
		prompt.WriteString(description + "\n\n")
	}

	prompt.WriteString("【ソースURL】\n")
	prompt.WriteString(sourceURL + "\n\n")

	prompt.WriteString("【出力形式】\n")
	prompt.WriteString("以下のJSON形式で返してください：\n")
	prompt.WriteString("{\n")
	prompt.WriteString(`  "title": "料理名（日本語）",` + "\n")
	prompt.WriteString(`  "ingredients": [` + "\n")
	prompt.WriteString(`    "材料1（分量含む）",` + "\n")
	prompt.WriteString(`    "材料2（分量含む）",` + "\n")
	prompt.WriteString(`    ...` + "\n")
	prompt.WriteString(`  ],` + "\n")
	prompt.WriteString(`  "steps": [` + "\n")
	prompt.WriteString(`    "手順1（具体的な調理方法）",` + "\n")
	prompt.WriteString(`    "手順2（具体的な調理方法）",` + "\n")
	prompt.WriteString(`    ...` + "\n")
	prompt.WriteString(`  ]` + "\n")
	prompt.WriteString("}\n\n")

	prompt.WriteString("【注意事項】\n")
	prompt.WriteString("- 動画タイトルや説明欄から料理名を特定してください\n")
	prompt.WriteString("- 上記のタイトルと説明欄に実際に書かれている情報だけを抽出してください。書かれていない内容を推測・創作しないでください\n")
	prompt.WriteString("- 材料は説明欄に記載がある場合のみ、分量を含めて記載してください（例: \"鶏もも肉 200g\"）。書かれていなければ \"ingredients\" を空配列 [] にしてください\n")
	prompt.WriteString("- 手順も説明欄に記載がある場合のみ記載してください。書かれていなければ \"steps\" を空配列 [] にしてください\n")
	prompt.WriteString("- 必ず有効なJSON形式で返してください\n")
	prompt.WriteString("- 返答はJSONのみ。前後の説明文、Markdown、コードブロックは出力しないでください\n")
	prompt.WriteString("- JSONの先頭は「{」で始め、末尾は「}」で終えてください\n")

	return prompt.String()
}

// generateRecipeFromYouTube はYouTube動画からレシピを生成する
func generateRecipeFromYouTube(ctx context.Context, job *Job) (*Recipe, error) {
	sourceURL := ""
	if job.SourceURL != nil {
		sourceURL = *job.SourceURL
	}

	videoID, err := extractYouTubeVideoID(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract video ID: %w", err)
	}
	log.Printf("Extracted YouTube video ID: %s", videoID)

	// タイトル・説明文を取得
	title, description, err := fetchYouTubeVideoInfo(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch video info: %w", err)
	}
	log.Printf("YouTube video title: %s", title)

	// 説明欄が長すぎる場合は切り詰める
	if len(description) > 8000 {
		description = description[:8000]
	}

	// プロンプト構築
	prompt := buildPromptForYouTube(title, description, sourceURL)

	// Bedrock APIを呼び出し（リトライ付き）
	log.Printf("Calling Bedrock API for YouTube job %s", job.JobID)
	var responseText string
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		responseText, err = callBedrockAPI(ctx, prompt, nil)
		if err == nil {
			log.Printf("Bedrock API call succeeded for YouTube job %s", job.JobID)
			break
		}
		log.Printf("Bedrock API call failed (attempt %d/%d) for YouTube job %s: %v", i+1, maxRetries, job.JobID, err)
		if i < maxRetries-1 {
			backoff := time.Duration(1<<uint(i)) * time.Second
			time.Sleep(backoff)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to call Bedrock API after %d retries: %w", maxRetries, err)
	}

	// レスポンスをパース
	recipe, err := parseRecipeResponse(responseText, job)
	if err != nil {
		log.Printf("Failed to parse YouTube response for job %s, retrying: %v", job.JobID, err)
		responseText, err = callBedrockAPI(ctx, prompt, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to retry Bedrock API: %w", err)
		}
		recipe, err = parseRecipeResponse(responseText, job)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response after retry: %w", err)
		}
	}

	log.Printf("Successfully generated YouTube recipe for job %s: %s", job.JobID, recipe.Title)
	return recipe, nil
}
