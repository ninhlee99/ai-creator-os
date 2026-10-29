package web

import (
	"net/url"
	"strings"
	"testing"
)

// Bảng tự động trên Trang chủ ghi đúng khóa setting của trang gốc, và từ
// chối khóa lạ (không cho ghi setting tùy ý qua form).
func TestAutoPanelToggle(t *testing.T) {
	s := newTestServer(t)
	body := get(t, s, "/dashboard").Body.String()
	if !strings.Contains(body, "Tự động hóa") || !strings.Contains(body, "Quét sản phẩm Accesstrade") {
		t.Fatal("Trang chủ thiếu bảng tự động hóa")
	}
	rec := postForm(t, s, "/automation/switch", url.Values{"key": {"autopilot_enabled"}, "on": {"0"}})
	if rec.Code != 303 {
		t.Fatalf("toggle = %d", rec.Code)
	}
	if s.atSettingOn("autopilot_enabled", true) {
		t.Fatal("affiliate autopilot switch must be off")
	}
	if rec := postForm(t, s, "/automation/switch", url.Values{"key": {"master.switch"}, "on": {"1"}}); rec.Code != 400 {
		t.Fatalf("khóa ngoài danh sách phải bị từ chối, got %d", rec.Code)
	}
}

// Bảng kiếm tiền nói đúng trạng thái: chưa nối gì thì không báo "Sẵn sàng".
func TestMoneyReadinessHonest(t *testing.T) {
	s := newTestServer(t)
	for _, k := range []string{"YOUTUBE_CLIENT_ID", "TIKTOK_CLIENT_KEY", "FB_PAGE_ID", "META_PAGE_TOKEN", "IG_USER_ID", "THREADS_USER_ID", "THREADS_TOKEN"} {
		t.Setenv(k, "")
	}
	for _, m := range s.moneyReadiness() {
		if m.Level == "ok" {
			t.Fatalf("%s báo sẵn sàng khi chưa nối gì", m.Name)
		}
		if m.Next == "" || m.Link == "" {
			t.Fatalf("%s thiếu bước tiếp theo", m.Name)
		}
	}
	if !strings.Contains(get(t, s, "/dashboard").Body.String(), "Trạng thái kiếm tiền") {
		t.Fatal("Trang chủ thiếu bảng kiếm tiền")
	}
}
