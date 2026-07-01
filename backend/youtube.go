package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
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

// fetchYouTubeCaptions はYouTube動画の字幕テキストを取得する
func fetchYouTubeCaptions(ctx context.Context, videoID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	watchURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	req, err := http.NewRequestWithContext(ctx, "GET", watchURL, nil)
	if err != nil {
		return "", nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept-Language", "ja,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil
	}
	html := string(body)

	// ytInitialPlayerResponse JSONを正規表現で抽出
	re := regexp.MustCompile(`ytInitialPlayerResponse\s*=\s*(\{.+?\});`)
	matches := re.FindStringSubmatch(html)
	if len(matches) < 2 {
		log.Printf("Could not find ytInitialPlayerResponse for video %s", videoID)
		return "", nil
	}

	var playerResp struct {
		Captions struct {
			PlayerCaptionsTracklistRenderer struct {
				CaptionTracks []struct {
					BaseURL      string `json:"baseUrl"`
					LanguageCode string `json:"languageCode"`
					Kind         string `json:"kind"`
				} `json:"captionTracks"`
			} `json:"playerCaptionsTracklistRenderer"`
		} `json:"captions"`
	}

	if err := json.Unmarshal([]byte(matches[1]), &playerResp); err != nil {
		log.Printf("Failed to parse player response for video %s: %v", videoID, err)
		return "", nil
	}

	tracks := playerResp.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks
	if len(tracks) == 0 {
		log.Printf("No caption tracks found for video %s", videoID)
		return "", nil
	}

	// 優先順位: 日本語手動字幕 → 日本語自動生成 → 英語 → その他
	var selected string
	var jaManual, jaAuto, en, fallback string
	for _, track := range tracks {
		if track.LanguageCode == "ja" && track.Kind != "asr" {
			jaManual = track.BaseURL
		} else if track.LanguageCode == "ja" && track.Kind == "asr" {
			jaAuto = track.BaseURL
		} else if track.LanguageCode == "en" && en == "" {
			en = track.BaseURL
		} else if fallback == "" {
			fallback = track.BaseURL
		}
	}
	switch {
	case jaManual != "":
		selected = jaManual
	case jaAuto != "":
		selected = jaAuto
	case en != "":
		selected = en
	case fallback != "":
		selected = fallback
	default:
		return "", nil
	}

	// 字幕XMLをフェッチ
	captionReq, err := http.NewRequestWithContext(ctx, "GET", selected, nil)
	if err != nil {
		return "", nil
	}

	captionResp, err := http.DefaultClient.Do(captionReq)
	if err != nil {
		return "", nil
	}
	defer captionResp.Body.Close()

	captionBody, err := io.ReadAll(captionResp.Body)
	if err != nil {
		return "", nil
	}

	// XMLをパース
	var transcript struct {
		XMLName xml.Name `xml:"transcript"`
		Texts   []struct {
			Text string `xml:",chardata"`
		} `xml:"text"`
	}

	if err := xml.Unmarshal(captionBody, &transcript); err != nil {
		log.Printf("Failed to parse caption XML for video %s: %v", videoID, err)
		return "", nil
	}

	var parts []string
	for _, t := range transcript.Texts {
		text := strings.TrimSpace(t.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, " "), nil
}

// buildPromptForYouTube はYouTube動画情報からLLM用プロンプトを構築する
func buildPromptForYouTube(title, description, captions, sourceURL string) string {
	var prompt strings.Builder

	prompt.WriteString("以下のYouTube料理動画の情報からレシピを抽出してください。\n\n")

	prompt.WriteString("【動画タイトル】\n")
	prompt.WriteString(title + "\n\n")

	if description != "" {
		prompt.WriteString("【説明欄】\n")
		prompt.WriteString(description + "\n\n")
	}

	if captions != "" {
		prompt.WriteString("【字幕テキスト】\n")
		prompt.WriteString(captions + "\n\n")
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
	prompt.WriteString("- 材料は分量を含めて記載してください（例: \"鶏もも肉 200g\"）\n")
	prompt.WriteString("- 手順は具体的で実行可能な内容にしてください\n")
	prompt.WriteString("- 字幕から調理手順を丁寧に抽出してください\n")
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

	// 字幕を取得（失敗してもエラーにしない）
	captions, err := fetchYouTubeCaptions(ctx, videoID)
	if err != nil {
		log.Printf("Failed to fetch captions for video %s (continuing without): %v", videoID, err)
		captions = ""
	}
	if captions != "" {
		log.Printf("Got %d chars of captions for video %s", len(captions), videoID)
	} else {
		log.Printf("No captions available for video %s", videoID)
	}

	// テキストを最大10000文字に切り詰め
	allText := title + "\n" + description + "\n" + captions
	if len(allText) > 10000 {
		maxDesc := 3000
		maxCaptions := 6000
		if len(description) > maxDesc {
			description = description[:maxDesc]
		}
		if len(captions) > maxCaptions {
			captions = captions[:maxCaptions]
		}
	}

	// プロンプト構築
	prompt := buildPromptForYouTube(title, description, captions, sourceURL)

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
