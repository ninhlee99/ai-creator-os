package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleStorySettings(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		storiesRedirect(w, r, "", "Ledger chưa sẵn sàng.")
		return
	}
	settings := map[string]string{
		automation.KeyStoryTopics:         strings.TrimSpace(r.FormValue("topics")),
		automation.KeyStoryAccount:        strings.TrimSpace(r.FormValue("account")),
		automation.KeyStoryAutoPublish:    "0",
		automation.KeyStoryTopicsAutofill: "0",
	}
	if r.FormValue("auto_publish") == "1" {
		settings[automation.KeyStoryAutoPublish] = "1"
	}
	if r.FormValue("topics_autofill") == "1" {
		settings[automation.KeyStoryTopicsAutofill] = "1"
	}
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	if n, err := strconv.Atoi(r.FormValue("words")); err == nil {
		settings[automation.KeyStoryWords] = strconv.Itoa(clamp(n, 200, 1200))
	}
	if n, err := strconv.Atoi(r.FormValue("scenes")); err == nil {
		settings[automation.KeyStoryScenes] = strconv.Itoa(clamp(n, 3, 12))
	}
	if _, ok := r.Form["genre"]; ok {
		settings[automation.KeyStoryGenre] = strings.TrimSpace(r.FormValue("genre"))
	}
	if h, err := strconv.Atoi(r.FormValue("interval_hours")); err == nil {
		settings[automation.KeyStoryIntervalH] = strconv.Itoa(clamp(h, 1, 168))
	}
	if h, err := strconv.Atoi(r.FormValue("public_after_hours")); err == nil {
		h = clamp(h, 0, 168)
		settings[automation.KeyStoryPublicAfterHours] = strconv.Itoa(h)
		// Đợt Y: tắt tự-public → xóa luôn hàng chờ cũ, nếu không bật lại
		// sau này sẽ làm video cũ public bất ngờ.
		if h == 0 {
			if err := s.Ledger.DeleteSetting(automation.KeyStoryPendingPublic); err != nil {
				storiesRedirect(w, r, "", "Không xóa được lịch công khai cũ; một số cấu hình có thể đã cập nhật.")
				return
			}
		}
	}
	// Đợt P: tự thử lại job lỗi. Chỉ ghi khi form có các trường này.
	if _, ok := r.Form["retry_on"]; ok {
		settings[automation.KeyStoryRetryEnabled] = "0"
		if r.FormValue("retry_on") == "1" {
			settings[automation.KeyStoryRetryEnabled] = "1"
		}
	}
	if n, err := strconv.Atoi(r.FormValue("retry_max")); err == nil {
		settings[automation.KeyStoryRetryMax] = strconv.Itoa(clamp(n, 1, 10))
	}
	if h, err := strconv.Atoi(r.FormValue("retry_cooldown")); err == nil {
		settings[automation.KeyStoryRetryCooldown] = strconv.Itoa(clamp(h, 1, 72))
	}
	for key, value := range settings {
		if err := s.Ledger.SetSetting(key, value); err != nil {
			msg := "Không lưu đủ mặc định kể chuyện (" + key + "); một số giá trị có thể đã cập nhật. Tải lại và kiểm tra."
			storiesRedirect(w, r, "", msg)
			return
		}
	}
	storiesRedirect(w, r, "Đã lưu mặc định kể chuyện.", "")
}

// atSettingStr đọc setting dạng chuỗi (def khi chưa có).
func (s *Server) atSettingStr(key, def string) string {
	if s.Ledger == nil {
		return def
	}
	if v, ok, err := s.Ledger.GetSetting(key); err == nil && ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}
