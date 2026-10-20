package web

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

func TestAPITikTokTopics(t *testing.T) {
	s := newViralServer(t)

	// 1. Mặc định: không truyền topics -> trả về các nhóm mặc định
	rec := get(t, s, "/api/studio/tiktok-topics")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/studio/tiktok-topics = %d, want 200", rec.Code)
	}

	var res struct {
		OK      bool                    `json:"ok"`
		Groups  []studio.TopicVideoGroup `json:"groups"`
		Presets []studio.TopicVideoGroup `json:"presets"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !res.OK {
		t.Fatalf("res.OK = false")
	}
	if len(res.Groups) == 0 {
		t.Fatalf("res.Groups is empty")
	}
	for _, grp := range res.Groups {
		if len(grp.Videos) != 4 {
			t.Errorf("topic group %q videos count = %d, want exactly 4", grp.TopicName, len(grp.Videos))
		}
	}
	if len(res.Presets) < 5 {
		t.Errorf("res.Presets count = %d, want >= 5", len(res.Presets))
	}

	// 2. Tùy chỉnh chủ đề: Nhảy, Sự kiện Việt Nam và 1 chủ đề custom
	recCustom := get(t, s, "/api/studio/tiktok-topics?topics=nhay,su_kien_vn,Ẩm+thực+Hà+Nội")
	if recCustom.Code != http.StatusOK {
		t.Fatalf("GET /api/studio/tiktok-topics?topics=... = %d, want 200", recCustom.Code)
	}

	var resCustom struct {
		OK     bool                    `json:"ok"`
		Groups []studio.TopicVideoGroup `json:"groups"`
	}
	if err := json.NewDecoder(recCustom.Body).Decode(&resCustom); err != nil {
		t.Fatalf("decode custom json: %v", err)
	}
	if len(resCustom.Groups) != 3 {
		t.Fatalf("custom groups len = %d, want 3", len(resCustom.Groups))
	}
	for _, grp := range resCustom.Groups {
		if len(grp.Videos) != 4 {
			t.Errorf("custom topic group %q videos count = %d, want exactly 4", grp.TopicName, len(grp.Videos))
		}
	}
}

func TestViralPageTopicRendering(t *testing.T) {
	s := newTestServer(t)
	st, err := studio.New(filepath.Join(t.TempDir(), "studio.db"), nil, nil, nil, t.TempDir())
	if err != nil {
		t.Fatalf("studio.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	s.Studio = st

	rec := get(t, s, "/viral")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /viral = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// Kiểm tra các thành phần UI viral sexy dance tinh gọn
	for _, want := range []string{
		"Sexy Dance Realtime",
		"Chọn Điệu Nhảy Hoặc Dán Link TikTok Realtime",
		"Chọn Các Góc Mặt Nhân Vật Nữ Thực Hiện Điệu Nhảy",
		"BẮT ĐẦU TẠO VIDEO SEXY DANCE TRENDING",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("viral page thiếu chuỗi UI: %q", want)
		}
	}
}
