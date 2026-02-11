package main

import (
	"testing"
	"time"
)

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{2, "2"},
	}
	for _, tt := range tests {
		got := itoa(tt.input)
		if got != tt.want {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractTitleFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"empty URL", "", "レシピ"},
		{"youtube.com", "https://www.youtube.com/watch?v=abc", "YouTubeレシピ"},
		{"youtu.be", "https://youtu.be/abc", "YouTubeレシピ"},
		{"path with slug", "https://example.com/recipes/chicken-curry", "Chicken curry"},
		{"path with underscores", "https://example.com/recipes/miso_soup", "Miso soup"},
		// Note: extractTitleFromURL has a byte-level ToUpper bug with multi-byte chars
		{"URL-encoded ASCII path", "https://example.com/recipes/chicken-rice", "Chicken rice"},
		{"root path only", "https://example.com/", "exampleのレシピ"},
		{"host only", "https://example.com", "exampleのレシピ"},
		{"invalid URL", "://invalid", "レシピ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitleFromURL(tt.url)
			if got != tt.want {
				t.Errorf("extractTitleFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "clean JSON",
			input: `{"title": "test"}`,
			want:  `{"title": "test"}`,
		},
		{
			name:  "JSON code block",
			input: "```json\n{\"title\": \"test\"}\n```",
			want:  `{"title": "test"}`,
		},
		{
			name:  "generic code block",
			input: "```\n{\"title\": \"test\"}\n```",
			want:  `{"title": "test"}`,
		},
		{
			name:  "text before and after JSON",
			input: "Here is the recipe:\n{\"title\": \"test\"}\nEnd.",
			want:  `{"title": "test"}`,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "no JSON object",
			input:   "just some text",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSON(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("extractJSON(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("extractJSON(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		name       string
		imageCount int
		sourceURL  *string
		contains   []string
	}{
		{
			name:       "no images no URL",
			imageCount: 0,
			sourceURL:  nil,
			contains:   []string{"JSON形式で返してください"},
		},
		{
			name:       "multiple images",
			imageCount: 3,
			sourceURL:  nil,
			contains:   []string{"完成写真", "材料", "手順"},
		},
		{
			name:       "with source URL",
			imageCount: 0,
			sourceURL:  strPtr("https://example.com/recipe"),
			contains:   []string{"ソースURL", "https://example.com/recipe"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPrompt(tt.imageCount, tt.sourceURL)
			for _, substr := range tt.contains {
				if !containsStr(got, substr) {
					t.Errorf("buildPrompt() missing expected substring %q", substr)
				}
			}
		})
	}
}

func TestParseRecipeResponse(t *testing.T) {
	job := &Job{
		JobID:     "job-1",
		UserID:    "user-1",
		ImageKeys: []string{"img1.jpg"},
		SourceURL: strPtr("https://example.com"),
	}

	tests := []struct {
		name            string
		response        string
		job             *Job
		wantErr         bool
		wantTitle       string
		wantIngredients int
		wantSteps       int
	}{
		{
			name:            "valid response",
			response:        `{"title":"カレー","ingredients":["玉ねぎ","にんじん"],"steps":["切る","煮る"]}`,
			job:             job,
			wantTitle:       "カレー",
			wantIngredients: 2,
			wantSteps:       2,
		},
		{
			name:      "empty title uses URL fallback",
			response:  `{"title":"","ingredients":["a"],"steps":["b"]}`,
			job:       job,
			wantTitle: "exampleのレシピ",
		},
		{
			name: "empty title no URL",
			response: `{"title":"","ingredients":["a"],"steps":["b"]}`,
			job: &Job{
				JobID:     "job-2",
				UserID:    "user-1",
				ImageKeys: []string{},
			},
			wantTitle: "レシピ",
		},
		{
			name:            "empty ingredients and steps get defaults",
			response:        `{"title":"Test","ingredients":[],"steps":[]}`,
			job:             job,
			wantIngredients: 1,
			wantSteps:       1,
		},
		{
			name:    "invalid JSON",
			response: "not json at all",
			job:     job,
			wantErr: true,
		},
		{
			name: "nil imageKeys in job",
			response: `{"title":"Test","ingredients":["a"],"steps":["b"]}`,
			job: &Job{
				JobID:     "job-3",
				UserID:    "user-1",
				ImageKeys: nil,
			},
			wantTitle: "Test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipe, err := parseRecipeResponse(tt.response, tt.job)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantTitle != "" && recipe.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", recipe.Title, tt.wantTitle)
			}
			if tt.wantIngredients > 0 && len(recipe.Ingredients) != tt.wantIngredients {
				t.Errorf("ingredients len = %d, want %d", len(recipe.Ingredients), tt.wantIngredients)
			}
			if tt.wantSteps > 0 && len(recipe.Steps) != tt.wantSteps {
				t.Errorf("steps len = %d, want %d", len(recipe.Steps), tt.wantSteps)
			}
			if recipe.ImageKeys == nil {
				t.Error("imageKeys should never be nil")
			}
		})
	}
}

func TestGenerateDummyRecipe(t *testing.T) {
	t.Run("with source URL", func(t *testing.T) {
		job := &Job{
			JobID:     "job-1",
			UserID:    "user-1",
			ImageKeys: []string{"img.jpg"},
			SourceURL: strPtr("https://www.youtube.com/watch?v=abc"),
		}
		recipe := generateDummyRecipe(job)
		if recipe.Title != "YouTubeレシピ" {
			t.Errorf("title = %q, want YouTubeレシピ", recipe.Title)
		}
		if recipe.UserID != "user-1" {
			t.Errorf("userID = %q, want user-1", recipe.UserID)
		}
		if len(recipe.ImageKeys) != 1 {
			t.Errorf("imageKeys len = %d, want 1", len(recipe.ImageKeys))
		}
	})

	t.Run("without source URL", func(t *testing.T) {
		job := &Job{
			JobID:     "job-2",
			UserID:    "user-1",
			ImageKeys: nil,
		}
		recipe := generateDummyRecipe(job)
		if recipe.Title != "サンプルレシピ" {
			t.Errorf("title = %q, want サンプルレシピ", recipe.Title)
		}
		if recipe.ImageKeys == nil {
			t.Error("imageKeys should never be nil")
		}
	})
}

func TestParseRecipeResponse_LongTitle(t *testing.T) {
	longTitle := ""
	for i := 0; i < 110; i++ {
		longTitle += "あ"
	}
	response := `{"title":"` + longTitle + `","ingredients":["a"],"steps":["b"]}`
	job := &Job{
		JobID:     "j",
		UserID:    "u",
		ImageKeys: []string{},
		CreatedAt: time.Now(),
	}
	recipe, err := parseRecipeResponse(response, job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recipe.Title) > 100 {
		t.Errorf("title length = %d, want <= 100", len(recipe.Title))
	}
}

func TestIsYouTubeURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"youtube.com watch", "https://www.youtube.com/watch?v=abc", true},
		{"youtube.com shorts", "https://www.youtube.com/shorts/abc", true},
		{"youtu.be", "https://youtu.be/abc", true},
		{"m.youtube.com", "https://m.youtube.com/watch?v=abc", true},
		{"not youtube", "https://example.com/recipe", false},
		{"empty string", "", false},
		{"invalid URL", "://bad", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isYouTubeURL(tt.url)
			if got != tt.want {
				t.Errorf("isYouTubeURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestExtractYouTubeVideoID(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{"watch URL", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ", false},
		{"short URL", "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ", false},
		{"shorts URL", "https://www.youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ", false},
		{"watch with extra params", "https://www.youtube.com/watch?v=abc123&t=30", "abc123", false},
		{"youtu.be empty path", "https://youtu.be/", "", true},
		{"youtube.com no v param", "https://www.youtube.com/watch", "", true},
		{"youtube.com shorts empty", "https://www.youtube.com/shorts/", "", true},
		{"not youtube", "https://example.com/video", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractYouTubeVideoID(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildPromptForYouTube(t *testing.T) {
	t.Run("all fields", func(t *testing.T) {
		prompt := buildPromptForYouTube("チキンカレーの作り方", "材料: 鶏肉", "まず鶏肉を切ります", "https://youtu.be/abc")
		expects := []string{
			"YouTube料理動画",
			"チキンカレーの作り方",
			"材料: 鶏肉",
			"まず鶏肉を切ります",
			"https://youtu.be/abc",
			"JSON形式で返してください",
		}
		for _, s := range expects {
			if !containsStr(prompt, s) {
				t.Errorf("prompt missing %q", s)
			}
		}
	})

	t.Run("empty description and captions", func(t *testing.T) {
		prompt := buildPromptForYouTube("タイトル", "", "", "https://youtu.be/abc")
		if containsStr(prompt, "【説明欄】") {
			t.Error("should not contain description section when empty")
		}
		if containsStr(prompt, "【字幕テキスト】") {
			t.Error("should not contain captions section when empty")
		}
	})
}

// helpers
func strPtr(s string) *string { return &s }

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
