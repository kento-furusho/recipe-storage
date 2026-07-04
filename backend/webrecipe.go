package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxWebPageBodySize = 3 * 1024 * 1024 // 3MB

// fetchWebPageText はURLからHTMLを取得し、本文らしきテキストを抽出する
func fetchWebPageText(ctx context.Context, pageURL string) (title, text string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept-Language", "ja,en;q=0.9")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("page returned status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxWebPageBodySize)
	title, text = extractTextFromHTML(limited)
	if strings.TrimSpace(text) == "" {
		return "", "", fmt.Errorf("no readable text found on page")
	}
	return title, text, nil
}

// extractTextFromHTML はHTMLトークンを走査し、タイトルと可視テキストを抽出する。
// script/style/nav/header/footer 等、レシピ本文になりにくい要素は除外する。
func extractTextFromHTML(r io.Reader) (title, text string) {
	skipTags := map[string]bool{
		"script": true, "style": true, "noscript": true,
		"nav": true, "header": true, "footer": true, "aside": true,
		"iframe": true, "svg": true, "form": true, "button": true,
	}

	tokenizer := html.NewTokenizer(r)
	var sb strings.Builder
	var titleSb strings.Builder
	var inTitle, inSkip bool
	skipDepth := 0

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}

		token := tokenizer.Token()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tag := strings.ToLower(token.Data)
			if tag == "title" {
				inTitle = true
			}
			if skipTags[tag] {
				inSkip = true
				if tt == html.StartTagToken {
					skipDepth++
				}
			}
			if tag == "br" || tag == "p" || tag == "li" || tag == "tr" {
				sb.WriteString("\n")
			}
		case html.EndTagToken:
			tag := strings.ToLower(token.Data)
			if tag == "title" {
				inTitle = false
			}
			if skipTags[tag] {
				skipDepth--
				if skipDepth <= 0 {
					inSkip = false
					skipDepth = 0
				}
			}
			if tag == "p" || tag == "div" || tag == "li" || tag == "tr" || tag == "h1" || tag == "h2" || tag == "h3" {
				sb.WriteString("\n")
			}
		case html.TextToken:
			if inSkip {
				continue
			}
			text := strings.TrimSpace(token.Data)
			if text == "" {
				continue
			}
			if inTitle {
				titleSb.WriteString(text)
				continue
			}
			sb.WriteString(text)
			sb.WriteString(" ")
		}
	}

	return strings.TrimSpace(titleSb.String()), collapseWhitespace(sb.String())
}

// collapseWhitespace は連続する空白・改行をまとめる
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, "\n")
}

// buildPromptForWebPage はレシピサイトのページ本文からLLM用プロンプトを構築する
func buildPromptForWebPage(pageTitle, bodyText, sourceURL string) string {
	var prompt strings.Builder

	prompt.WriteString("以下はレシピサイトのページから抽出したテキストです。この内容からレシピ情報を抽出してください。\n\n")

	prompt.WriteString("【ページタイトル】\n")
	prompt.WriteString(pageTitle + "\n\n")

	prompt.WriteString("【ページ本文】\n")
	prompt.WriteString(bodyText + "\n\n")

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
	prompt.WriteString("- 上記のページ本文に実際に書かれている情報だけを抽出してください。本文にない内容を推測・創作しないでください\n")
	prompt.WriteString("- ページ本文がレシピの内容でない、または材料・手順が読み取れない場合は、該当する配列を空 [] にしてください\n")
	prompt.WriteString("- 材料は本文に記載がある場合のみ、分量を含めて記載してください（例: \"鶏もも肉 200g\"）\n")
	prompt.WriteString("- 手順は本文の記載に沿って具体的に記載してください。本文にない手順を作り出さないでください\n")
	prompt.WriteString("- 必ず有効なJSON形式で返してください\n")
	prompt.WriteString("- 返答はJSONのみ。前後の説明文、Markdown、コードブロックは出力しないでください\n")
	prompt.WriteString("- JSONの先頭は「{」で始め、末尾は「}」で終えてください\n")

	return prompt.String()
}

// generateRecipeFromWebPage は一般のレシピサイトURLからレシピを生成する
func generateRecipeFromWebPage(ctx context.Context, job *Job) (*Recipe, error) {
	sourceURL := ""
	if job.SourceURL != nil {
		sourceURL = *job.SourceURL
	}

	pageTitle, bodyText, err := fetchWebPageText(ctx, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch web page: %w", err)
	}
	log.Printf("Fetched web page for job %s: title=%q, %d chars of text", job.JobID, pageTitle, len(bodyText))

	// テキストを最大12000文字に切り詰め
	if len(bodyText) > 12000 {
		bodyText = bodyText[:12000]
	}

	prompt := buildPromptForWebPage(pageTitle, bodyText, sourceURL)

	log.Printf("Calling Bedrock API for web page job %s", job.JobID)
	var responseText string
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		responseText, err = callBedrockAPI(ctx, prompt, nil)
		if err == nil {
			log.Printf("Bedrock API call succeeded for web page job %s", job.JobID)
			break
		}
		log.Printf("Bedrock API call failed (attempt %d/%d) for web page job %s: %v", i+1, maxRetries, job.JobID, err)
		if i < maxRetries-1 {
			backoff := time.Duration(1<<uint(i)) * time.Second
			time.Sleep(backoff)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to call Bedrock API after %d retries: %w", maxRetries, err)
	}

	recipe, err := parseRecipeResponse(responseText, job)
	if err != nil {
		log.Printf("Failed to parse web page response for job %s, retrying: %v", job.JobID, err)
		responseText, err = callBedrockAPI(ctx, prompt, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to retry Bedrock API: %w", err)
		}
		recipe, err = parseRecipeResponse(responseText, job)
		if err != nil {
			return nil, fmt.Errorf("failed to parse response after retry: %w", err)
		}
	}

	log.Printf("Successfully generated recipe from web page for job %s: %s", job.JobID, recipe.Title)
	return recipe, nil
}
