package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	s.Routes().ServeHTTP(jsonRec, jsonReq)

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
