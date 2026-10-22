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
	set := func(key, val string) { _ = s.Ledger.SetSetting(key, val) }
	setBool := func(key string, on bool) {
		if on {
			set(key, "1")
		} else {
			set(key, "0")
		}
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
		set(automation.KeyStoryWords, strconv.Itoa(clamp(n, 200, 1200)))
	}
	if n, err := strconv.Atoi(r.FormValue("scenes")); err == nil {
		set(automation.KeyStoryScenes, strconv.Itoa(clamp(n, 3, 12)))
	}
	if g := strings.TrimSpace(r.FormValue("genre")); g != "" {
		set(automation.KeyStoryGenre, g)
	}
	if h, err := strconv.Atoi(r.FormValue("interval_hours")); err == nil {
		set(automation.KeyStoryIntervalH, strconv.Itoa(clamp(h, 1, 168)))
	}
	if a := strings.TrimSpace(r.FormValue("account")); a != "" {
		set(automation.KeyStoryAccount, a)
	}
	set(automation.KeyStoryTopics, strings.TrimSpace(r.FormValue("topics")))
	setBool(automation.KeyStoryMusicOn, r.FormValue("music_on") == "1")
	setBool(automation.KeyStoryAutoPublish, r.FormValue("auto_publish") == "1")
	setBool(automation.KeyStoryEnabled, r.FormValue("tick_on") == "1")
	setBool(automation.KeyStoryTopicsAutofill, r.FormValue("topics_autofill") == "1")
	if h, err := strconv.Atoi(r.FormValue("public_after_hours")); err == nil {
		h = clamp(h, 0, 168)
		set(automation.KeyStoryPublicAfterHours, strconv.Itoa(h))
		// Đợt Y: tắt tự-public → xóa luôn hàng chờ cũ, nếu không bật lại
		// sau này sẽ làm video cũ public bất ngờ.
		if h == 0 {
			_ = s.Ledger.DeleteSetting(automation.KeyStoryPendingPublic)
		}
	}
	// Đợt P: tự thử lại job lỗi.
	setBool(automation.KeyStoryRetryEnabled, r.FormValue("retry_on") == "1")
	if n, err := strconv.Atoi(r.FormValue("retry_max")); err == nil {
		set(automation.KeyStoryRetryMax, strconv.Itoa(clamp(n, 1, 10)))
	}
	if h, err := strconv.Atoi(r.FormValue("retry_cooldown")); err == nil {
		set(automation.KeyStoryRetryCooldown, strconv.Itoa(clamp(h, 1, 72)))
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
