package web

import (
	"bytes"
	"encoding/json"
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

	// 3. Affiliate intent routes to Studio until automatic production exists.
	jsonReq := httptest.NewRequest(http.MethodPost, "/api/create-one-command", strings.NewReader(url.Values{
		"command": {"Review serum chống lão hóa phục hồi da"},
	}.Encode()))
	jsonReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	jsonReq.Header.Set("Accept", "application/json")
	jsonRec := httptest.NewRecorder()
	serveLocalTestRoute(s, jsonRec, jsonReq)

	if jsonRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("API request expected 422 until affiliate automation is available, got %d, body: %s", jsonRec.Code, jsonRec.Body.String())
	}
	var res OneCommandResult
	if err := json.Unmarshal(jsonRec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if res.OK || res.Type != "affiliate" || !strings.Contains(res.RedirectURL, "/studio?") || !strings.Contains(res.Message, "chưa tạo video affiliate tự động") {
		t.Errorf("unexpected honest affiliate result: %+v", res)
	}
}

func TestDashboardQuickCreateCopy(t *testing.T) {
	s := newViralServer(t)
	page := get(t, s, "/")
	if page.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", page.Code)
	}
	for _, want := range []string{
		"Bắt đầu tạo nội dung",
		"Kể chuyện tạo job ngay",
		"chưa tự dựng từ câu lệnh",
		"name=\"command\"",
		"Tiếp tục",
	} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("quick content router missing %q", want)
		}
	}
	if strings.Contains(page.Body.String(), `name="product_media"`) || strings.Contains(page.Body.String(), "tạo footage chuyển động mới") {
		t.Error("quick content router promises or accepts media it cannot use")
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
	for _, want := range []string{".webai-section-actions", "white-space: nowrap"} {
		if !strings.Contains(styles.Body.String(), want) {
			t.Errorf("stylesheet missing layout rule %q", want)
		}
	}
}

func TestOneCommandUploadIsNotPersisted(t *testing.T) {
	s := newViralServer(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("command", "Review serum B5"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("product_media", "listing.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "image-bytes"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/create-one-command", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("upload request = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "media chưa được lưu") {
		t.Fatalf("upload response omitted non-persistence notice: %s", rec.Body.String())
	}
	files, err := os.ReadDir(s.Studio.UploadDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(files) > 0 {
		t.Errorf("quick create left unused upload files behind: %v", files)
	}
	if jobs := s.Studio.ListJobs(10); len(jobs) != 0 {
		t.Errorf("unsupported quick create unexpectedly queued jobs: %+v", jobs)
	}
}

func TestOneCommandRejectsLargeMultipart(t *testing.T) {
	s := newViralServer(t)
	large := strings.Repeat("x", oneCommandMaxRequestBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/api/create-one-command", strings.NewReader(large))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=not-a-real-form")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	serveLocalTestRoute(s, rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Luồng nhanh chỉ nhận mô tả bằng chữ") {
		t.Fatalf("oversize request = %d %s, want bounded 400 with text-only guidance", rec.Code, rec.Body.String())
	}
	entries, err := os.ReadDir(s.Studio.UploadDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("oversize request left files behind: %v", entries)
	}
}
