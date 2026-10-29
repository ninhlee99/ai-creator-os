package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func newViralServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st
	return s
}

func TestViralLegacyRedirect(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/viral")
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /viral = %d, want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/workflow/create" {
		t.Fatalf("GET /viral redirect = %q, want /workflow/create", loc)
	}
}

func TestVideoCreatePageNoStudio(t *testing.T) {
	s := newTestServer(t) // Studio == nil
	rec := get(t, s, "/workflow/create")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /workflow/create = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Video nhảy") {
		t.Errorf("trang /workflow/create thiếu tiêu đề: %s", body)
	}
}

func TestVideoCreatePageRenders(t *testing.T) {
	s := newViralServer(t)
	rec := get(t, s, "/workflow/create")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /workflow/create = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Video tham chiếu bắt buộc",
		"Tên video nhảy",
		"Video từ TikTok Radar",
		"Thay nhân vật",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /workflow/create thiếu nội dung %q", want)
		}
	}
}

func TestVideoCreateSubmit(t *testing.T) {
	s := newViralServer(t)

	// Test form create redirecting when editor is not ready
	rec := postForm(t, s, "/workflow/create", url.Values{
		"topic":       {"3 Sai Lầm Tài Chính"},
		"genre":       {"lifehack"},
		"visual_mood": {"cinematic"},
		"duration":    {"30"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /workflow/create = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "err=") {
		t.Errorf("Location = %q, want err=", loc)
	}
}

func TestViralPageOnePromptFeatures(t *testing.T) {
	s := newViralServer(t)
	rec := get(t, s, "/workflow/create")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /workflow/create = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Tạo Video AI Bằng 1 Câu Lệnh",
		"Tự Động (Auto)",
		"Video Affiliate",
		"Video TikTok Viral",
		`name="prompt"`,
		`name="output_type"`,
		`name="character_photos"`,
		`name="video_file"`,
		"1080p Full HD",
		"2K QHD",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /workflow/create thiếu tính năng %q", want)
		}
	}
	for _, forbidden := range []string{"4k", "4K", "8k", "8K"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("GET /workflow/create chứa tùy chọn độ phân giải bị loại bỏ %q", forbidden)
		}
	}
}

func TestDetectAutoVideoType(t *testing.T) {
	// Affiliate with product URL
	if got := detectAutoVideoType("Video hay", "Chu de", "https://shopee.vn/sp-1", "", 0, false); got != "affiliate" {
		t.Errorf("product_url got %q, want affiliate", got)
	}
	// Viral with tiktok link
	if got := detectAutoVideoType("Video hay", "Chu de", "", "https://tiktok.com/@u/video/1", 0, false); got != "viral" {
		t.Errorf("tiktokLink got %q, want viral", got)
	}
	// Affiliate by prompt keywords
	if got := detectAutoVideoType("Review son môi dưỡng ẩm giỏ hàng giảm giá săn deal", "Son môi", "", "", 1, false); got != "affiliate" {
		t.Errorf("affiliate keywords got %q, want affiliate", got)
	}
	// Viral by prompt keywords
	if got := detectAutoVideoType("Bắt trend vũ đạo biến hình nhảy triệu view hot tiktok", "Vũ đạo", "", "", 0, true); got != "viral" {
		t.Errorf("viral keywords got %q, want viral", got)
	}
}

func TestExtractURLsFromPrompt(t *testing.T) {
	prompt := `Làm video review sản phẩm https://shopee.vn/product/123/456 kết hợp phong cách của video https://www.tiktok.com/@creator/video/9876543210 và dùng ảnh nhân vật https://example.com/face.jpg`
	res := extractURLsFromPrompt(prompt)
	if res.ProductURL != "https://shopee.vn/product/123/456" {
		t.Errorf("ProductURL = %q, want shopee url", res.ProductURL)
	}
	if res.TikTokLink != "https://www.tiktok.com/@creator/video/9876543210" {
		t.Errorf("TikTokLink = %q, want tiktok url", res.TikTokLink)
	}
	if len(res.ImageURLs) != 1 || res.ImageURLs[0] != "https://example.com/face.jpg" {
		t.Errorf("ImageURLs = %v, want [https://example.com/face.jpg]", res.ImageURLs)
	}
}

func TestValidateAndEnforceViralChannelFallback(t *testing.T) {
	accounts := []*network.Account{
		{ID: 10, Username: "Meow"},
	}
	// When requested account is empty, it should automatically fallback to the first account
	user, id, errStr := ValidateAndEnforceViralChannel("Topic", "dance", "", accounts)
	if errStr != "" {
		t.Fatalf("unexpected error: %s", errStr)
	}
	if user != "Meow" || id != 10 {
		t.Errorf("user=%q id=%d, want Meow 10", user, id)
	}
}
