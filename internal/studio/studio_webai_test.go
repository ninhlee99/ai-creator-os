package studio

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func TestAnalyzeImagesWithWebAIFallback(t *testing.T) {
	st := &Studio{}
	calls := make([]string, 0)

	st.SetWebAIResolver(func(platform string) (*WebAISession, error) {
		switch platform {
		case webai.PlatformGemini:
			return &WebAISession{Platform: platform, CookieHeader: "dummy_cookie=1"}, nil
		default:
			return nil, fmt.Errorf("no session")
		}
	})

	st.SetWebAIImageAnalyzer(func(ctx context.Context, session *WebAISession, prompt string, images []string) (string, error) {
		calls = append(calls, session.Platform)
		return `{"decision":"pass","summary":"Gemini Web AI đã phân tích thành công 2 ảnh","character_identity":"match","motion":"visible","sharpness":"sharp","composition":"professional","continuity":"consistent"}`, nil
	})

	sampleImg := filepath.Join("..", "web", "static", "agents", "qc.jpg")
	out, err := st.AnalyzeImagesWithWebAI(context.Background(), "Phân tích ảnh", []string{sampleImg})
	if err != nil {
		t.Fatalf("AnalyzeImagesWithWebAI failed: %v", err)
	}

	if !strings.Contains(out, "Gemini Web AI") {
		t.Errorf("output does not contain expected Gemini Web AI response, got: %s", out)
	}

	if len(calls) != 1 || calls[0] != webai.PlatformGemini {
		t.Errorf("expected 1 call to Gemini, got: %v", calls)
	}
}

func TestAnalyzeVisualQCFallsBackToWebAI(t *testing.T) {
	st := &Studio{}
	// s.mg is nil -> should fall back to Web AI automatically
	st.SetWebAIResolver(func(platform string) (*WebAISession, error) {
		if platform == webai.PlatformClaude {
			return &WebAISession{Platform: platform, CookieHeader: "claude_cookie=1"}, nil
		}
		return nil, fmt.Errorf("unavailable")
	})

	st.SetWebAIImageAnalyzer(func(ctx context.Context, session *WebAISession, prompt string, images []string) (string, error) {
		return "```json\n" +
			"{\n" +
			`  "decision": "pass",` + "\n" +
			`  "summary": "Claude Web AI xác nhận chất lượng hình ảnh đạt chuẩn QC",` + "\n" +
			`  "character_identity": "match",` + "\n" +
			`  "motion": "visible",` + "\n" +
			`  "sharpness": "sharp",` + "\n" +
			`  "composition": "professional",` + "\n" +
			`  "continuity": "consistent"` + "\n" +
			"}\n```", nil
	})

	sampleImg := filepath.Join("..", "web", "static", "agents", "qc.jpg")
	refs := []ImageRef{{Path: sampleImg}}

	report, err := st.analyzeVisualQC(context.Background(), "Kiểm tra QC", refs)
	if err != nil {
		t.Fatalf("analyzeVisualQC with Web AI fallback failed: %v", err)
	}

	if report.Decision != "pass" {
		t.Errorf("report.Decision = %s, want pass", report.Decision)
	}

	if !strings.Contains(report.Summary, "Claude Web AI") {
		t.Errorf("report.Summary = %s, want mention of Claude Web AI", report.Summary)
	}
}
