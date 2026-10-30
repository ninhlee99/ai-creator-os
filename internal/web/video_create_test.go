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

func TestVideoCreatePageNoStudio(t *testing.T) {
	s := newTestServer(t) // Studio == nil
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Video nhảy") {
		t.Errorf("trang / thiếu tiêu đề: %s", body)
	}
}

func TestVideoCreatePageRenders(t *testing.T) {
	s := newViralServer(t)
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Video tham chiếu bắt buộc",
		"Tên video nhảy",
		"Video từ TikTok Radar",
		"Thay nhân vật",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang / thiếu nội dung %q", want)
		}
	}
}

func TestVideoCreateSubmit(t *testing.T) {
	s := newViralServer(t)

	// Test form create redirecting when editor is not ready
	rec := postForm(t, s, "/", url.Values{
		"topic":       {"3 Sai Lầm Tài Chính"},
		"genre":       {"lifehack"},
		"visual_mood": {"cinematic"},
		"duration":    {"30"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST / = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "err=") {
		t.Errorf("Location = %q, want err=", loc)
	}
}

func TestViralPageOnePromptFeatures(t *testing.T) {
	s := newViralServer(t)
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Tạo video bằng 1 câu lệnh",
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
			t.Errorf("GET / thiếu tính năng %q", want)
		}
	}
	for _, forbidden := range []string{"4k", "4K", "8k", "8K"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("GET / chứa tùy chọn độ phân giải bị loại bỏ %q", forbidden)
		}
	}
}

func TestStudioJobOutputResolutionHidesLegacy4K(t *testing.T) {
	cases := map[string]string{
		"":        "1080P",
		"1080p":   "1080P",
		"2k":      "2K",
		"4k":      "2K",
		"2160p":   "2K",
		"8k":      "2K",
		"unknown": "1080P",
	}
	for input, want := range cases {
		if got := studioJobOutputResolution(input); got != want {
			t.Errorf("studioJobOutputResolution(%q) = %q, want %q", input, got, want)
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

func TestParsePromptDirectives(t *testing.T) {
	prompt := "Tạo video review son môi Merzy cực xinh https://shopee.vn/son-123 độ phân giải 2K giọng nam truyền cảm"
	d := parsePromptDirectives(prompt)
	if d.Resolution != "2K" {
		t.Errorf("Resolution = %q, want 2K", d.Resolution)
	}
	if d.Voice != "male" {
		t.Errorf("Voice = %q, want male", d.Voice)
	}
	if d.Genre != "affiliate" {
		t.Errorf("Genre = %q, want affiliate", d.Genre)
	}
	if strings.Contains(d.CleanTopic, "http") {
		t.Errorf("CleanTopic should not contain URLs: %q", d.CleanTopic)
	}
}

func TestDetectAutoWorkflow(t *testing.T) {
	// Fashion try-on
	outType, wf := detectAutoWorkflow("Phối đồ outfit mùa đông ấm áp thời trang", "Thời trang", "", "", 1, false, "")
	if outType != "workflow" || wf != "preset_fashion_tryon" {
		t.Errorf("got %s %s, want workflow preset_fashion_tryon", outType, wf)
	}
	// Viral dance
	outType, wf = detectAutoWorkflow("Nhảy vũ đạo bắt trend hot tiktok triệu view", "Vũ đạo", "", "", 0, true, "")
	if outType != "viral" || wf != "preset_viral_dance" {
		t.Errorf("got %s %s, want viral preset_viral_dance", outType, wf)
	}
	// Affiliate
	outType, wf = detectAutoWorkflow("Review đập hộp tai nghe bluetooth", "Tai nghe", "https://shopee.vn/sp", "", 1, false, "")
	if outType != "affiliate" || wf != "preset_affiliate_daily" {
		t.Errorf("got %s %s, want affiliate preset_affiliate_daily", outType, wf)
	}
}
