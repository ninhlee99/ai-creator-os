package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestDetectCommandIntent(t *testing.T) {
	tests := []struct {
		input     string
		wantMode  string
		wantNiche string
	}{
		{
			input:     "Kể chuyện đêm khuya về ngôi làng bí ẩn vùng Tây Bắc",
			wantMode:  "story",
			wantNiche: "mystery",
		},
		{
			input:     "Truyện audio tâm sự đêm khuya",
			wantMode:  "story",
			wantNiche: "mystery",
		},
		{
			input:     "Review serum dưỡng ẩm B5 phục hồi da căng bóng",
			wantMode:  "affiliate",
			wantNiche: "mỹ phẩm",
		},
		{
			input:     "Làm video review nồi chiên không dầu 8L giá rẻ",
			wantMode:  "affiliate",
			wantNiche: "gia dụng",
		},
		{
			input:     "Review tai nghe bluetooth chống ồn Sony",
			wantMode:  "affiliate",
			wantNiche: "công nghệ",
		},
		{
			input:     "5 sự thật kỳ lạ về vũ trụ",
			wantMode:  "viral",
			wantNiche: "mystery",
		},
		{
			input:     "Vũ đạo biến hình bắt trend hot",
			wantMode:  "viral",
			wantNiche: "dance",
		},
		{
			input:     "5 mẹo làm việc hiệu quả với AI",
			wantMode:  "viral",
			wantNiche: "lifehack",
		},
	}

	for _, tc := range tests {
		mode, topic, niche := detectCommandIntent(tc.input)
		if mode != tc.wantMode {
			t.Errorf("detectCommandIntent(%q) mode = %s, want %s", tc.input, mode, tc.wantMode)
		}
		if tc.wantNiche != "" && niche != tc.wantNiche {
			t.Errorf("detectCommandIntent(%q) niche = %s, want %s", tc.input, niche, tc.wantNiche)
		}
		if topic == "" {
			t.Errorf("detectCommandIntent(%q) topic should not be empty", tc.input)
		}
	}
}

func TestOneCommandCreateEndpoint(t *testing.T) {
	s := newViralServer(t)

	// 1. Trống command -> báo lỗi
	rec := postForm(t, s, "/create-one-command", url.Values{
		"command": {""},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("empty command should redirect, got %d", rec.Code)
	}

	// 2. Tạo video Kể chuyện qua form submit
	rec = postForm(t, s, "/create-one-command", url.Values{
		"command": {"Kể chuyện ma đêm khuya vùng núi Tây Bắc"},
		"back":    {"/"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("story command post should redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/?ok=") {
		t.Errorf("expected redirect back to /?ok=..., got %s", loc)
	}

	// 3. Tạo video Affiliate qua API JSON
	jsonReq := httptest.NewRequest(http.MethodPost, "/api/create-one-command", strings.NewReader(url.Values{
		"command": {"Review serum chống lão hóa phục hồi da"},
	}.Encode()))
	jsonReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	jsonReq.Header.Set("Accept", "application/json")
	jsonRec := httptest.NewRecorder()
	serveLocalTestRoute(s, jsonRec, jsonReq)

	if jsonRec.Code != http.StatusOK {
		t.Fatalf("API request expected 200, got %d, body: %s", jsonRec.Code, jsonRec.Body.String())
	}
	var res OneCommandResult
	if err := json.Unmarshal(jsonRec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if !res.OK || res.JobID == "" || res.Type != "affiliate" {
		t.Errorf("unexpected JSON result: %+v", res)
	}
}

func TestDashboardProductMediaPicker(t *testing.T) {
	s := newViralServer(t)
	page := get(t, s, "/")
	if page.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", page.Code)
	}
	for _, want := range []string{
		`name="product_media"`,
		`multiple`,
		`image/jpeg`,
		"Kéo thả ảnh listing sản phẩm vào đây",
	} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("quick creator media picker missing %q", want)
		}
	}
	if strings.Contains(page.Body.String(), "video/mp4") {
		t.Error("product listing picker must accept product images, not source videos")
	}

	settings := get(t, s, "/settings/system")
	if settings.Code != http.StatusOK {
		t.Fatalf("GET /settings/system = %d, want 200", settings.Code)
	}
	for _, want := range []string{"webai-backup-import-action"} {
		if !strings.Contains(settings.Body.String(), want) {
			t.Errorf("Web AI backup import action missing %q", want)
		}
	}
	if strings.Contains(settings.Body.String(), "webai-key-action") {
		t.Error("Web AI session UI must not expose manual credential entry")
	}
	styles := get(t, s, "/static/style.css")
	if styles.Code != http.StatusOK {
		t.Fatalf("GET /static/style.css = %d, want 200", styles.Code)
	}
	for _, want := range []string{".product-media-dropzone", ".webai-section-actions", "white-space: nowrap"} {
		if !strings.Contains(styles.Body.String(), want) {
			t.Errorf("stylesheet missing layout rule %q", want)
		}
	}
}

func TestSaveOneCommandProductMedia(t *testing.T) {
	t.Run("multiple images", func(t *testing.T) {
		r := oneCommandMultipartRequest(t, []oneCommandTestFile{
			{name: "front.jpg", content: "image-one"},
			{name: "side.png", content: "image-two"},
		})
		media, err := saveOneCommandProductMedia(r, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if len(media.photos) != 2 || len(media.clips) != 0 {
			t.Fatalf("media = %+v, want two photos and no clips", media)
		}
		for i, want := range []string{"image-one", "image-two"} {
			got, err := os.ReadFile(media.photos[i])
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Errorf("photo %d content = %q, want %q", i, got, want)
			}
		}
	})

	t.Run("multiple videos", func(t *testing.T) {
		r := oneCommandMultipartRequest(t, []oneCommandTestFile{
			{name: "first.mp4", content: "clip-one"},
			{name: "second.mov", content: "clip-two"},
		})
		media, err := saveOneCommandProductMedia(r, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if len(media.clips) != 2 || len(media.photos) != 0 {
			t.Fatalf("media = %+v, want two clips and no photos", media)
		}
	})

	t.Run("mixed types rejected", func(t *testing.T) {
		r := oneCommandMultipartRequest(t, []oneCommandTestFile{
			{name: "front.jpg", content: "image-one"},
			{name: "clip.mp4", content: "clip-one"},
		})
		if _, err := saveOneCommandProductMedia(r, t.TempDir()); err == nil || !strings.Contains(err.Error(), "không trộn") {
			t.Fatalf("mixed media error = %v, want mixed-type rejection", err)
		}
	})
}

type oneCommandTestFile struct {
	name    string
	content string
}

func oneCommandMultipartRequest(t *testing.T, files []oneCommandTestFile) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := writer.CreateFormFile("product_media", file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, file.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/create-one-command", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	})
	return r
}

func TestSaveOneCommandProductMediaRejectsTooManyFiles(t *testing.T) {
	files := make([]oneCommandTestFile, 0, 9)
	for i := 0; i < 9; i++ {
		files = append(files, oneCommandTestFile{name: fmt.Sprintf("%d.jpg", i), content: "image"})
	}
	r := oneCommandMultipartRequest(t, files)
	if _, err := saveOneCommandProductMedia(r, t.TempDir()); err == nil || !strings.Contains(err.Error(), "tối đa 8") {
		t.Fatalf("too many files error = %v, want max-count rejection", err)
	}
}
