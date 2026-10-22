package web

// Affiliate product catalog: product search and discovery, manual add,
// the merged shelf tab, autopilot scheduling policy and music upload.

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// autopilotMusicPath is where the UI-uploaded music bed lives.
func autopilotMusicPath(databasePath string) string {
	dir := "."
	if databasePath != "" {
		dir = filepath.Dir(databasePath)
	}
	return filepath.Join(dir, "autopilot-music.m4a")
}

// scheduleView reads the current schedule state for the products page.
func (s *Server) scheduleView() (enabled bool, hours int, lastRun, music string, autoPublish, youTube bool) {
	// Automation switches read from the single settings facade (R2-W4):
	// ledger settings. Switches default ON when unset (Đợt 3); a stored
	// "0" is an explicit operator choice and always wins.
	st := s.settings()
	enabled = automation.AutopilotEnabled(st)
	hours = automation.AutopilotIntervalHours(st)
	lastRun = automation.AutopilotLastRun(st)
	music = automation.AutopilotMusicName(st)
	autoPublish = automation.AutopilotAutoPublish(st)
	youTube = automation.AutopilotYouTubeEnabled(st)
	return
}

// handleProductsSchedule saves the autopilot background schedule from the
// web UI: enable toggle + interval in hours + auto-publish toggle.
func (s *Server) handleProductsSchedule(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		s.fail(w, fmt.Errorf("kho cài đặt chưa khởi tạo"), "save schedule")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse schedule form")
		return
	}
	enabled := "0"
	if r.PostFormValue("enabled") == "on" {
		enabled = "1"
	}
	autoPub := "0"
	if r.PostFormValue("auto_publish") == "on" {
		autoPub = "1"
	}
	youTube := "0"
	if r.PostFormValue("youtube_enabled") == "on" {
		youTube = "1"
	}
	hours := 6
	if h, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("interval_hours"))); err == nil && h >= 1 && h <= 168 {
		hours = h
	}
	st := s.settings()
	if err := st.Set(SettingAutopilotEnabled, enabled); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotInterval, strconv.Itoa(hours)); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotAutoPublish, autoPub); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	if err := st.Set(SettingAutopilotYouTube, youTube); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	meta := "0"
	if r.PostFormValue("meta_enabled") == "on" {
		meta = "1"
	}
	if err := st.Set(SettingAutopilotMeta, meta); err != nil {
		s.fail(w, err, "save schedule")
		return
	}
	msg := "Đã tắt lịch autopilot."
	if enabled == "1" {
		msg = fmt.Sprintf("Đã bật lịch autopilot — chạy mỗi %d giờ cho các account đã sẵn sàng.", hours)
	}
	if autoPub == "1" {
		msg += " Video xong sẽ tự đăng TikTok (dạng nháp)."
	}
	seeOther(w, r, "/products?ok="+url.QueryEscape(msg))
}

// handleAutopilotMusicUpload replaces the autopilot music bed from the UI.
// Stored next to the database; the scheduler picks it up automatically.
func (s *Server) handleAutopilotMusicUpload(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		s.fail(w, fmt.Errorf("kho sản phẩm chưa khởi tạo"), "upload music")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20) // 25 MB cap
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Error", "File nhạc quá lớn hoặc lỗi upload (tối đa 25MB).",
		))
		return
	}
	f, hdr, err := r.FormFile("music")
	if err != nil {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Error", "Chưa chọn file nhạc.",
		))
		return
	}
	defer f.Close()
	if !audioFileRe.MatchString(hdr.Filename) {
		s.render(w, "products", s.ctx(
			"AT", s.atPageData(),
			"Themes", themeOptions(),
			"Error", "Chỉ nhận file nhạc mp3/m4a/wav/ogg/aac.",
		))
		return
	}
	dst := autopilotMusicPath(s.Cfg.DatabasePath)
	if dir := filepath.Dir(dst); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			s.fail(w, err, "upload music")
			return
		}
	}
	out, err := os.Create(dst)
	if err != nil {
		s.fail(w, err, "upload music")
		return
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		_ = os.Remove(dst)
		s.fail(w, err, "upload music")
		return
	}
	out.Close()
	_ = s.settings().Set(SettingAutopilotMusic, hdr.Filename)
	if s.Autopilot != nil {
		s.Autopilot.MusicPath = dst
	}
	seeOther(w, r, "/products?ok="+url.QueryEscape("Đã cập nhật nhạc nền autopilot: "+hdr.Filename))
}

var audioFileRe = regexp.MustCompile(`(?i)\.(mp3|m4a|wav|ogg|aac)$`)

// The autopilot background schedule is managed in the web UI (not via env
// or CLI). R2-W4: these keys live in the single settings facade (ledger
// settings table) — see internal/automation. Exported so cmd/aicos (the
// scheduler wiring) and internal/automation read the same keys the UI
// writes.
const (
	SettingAutopilotEnabled     = "autopilot_enabled"
	SettingAutopilotInterval    = "autopilot_interval_hours"
	SettingAutopilotLastRun     = "autopilot_last_run"
	SettingAutopilotMusic       = "autopilot_music_name"
	SettingAutopilotAutoPublish = "autopilot_auto_publish"
	SettingAutopilotYouTube     = "autopilot_youtube_enabled"
	SettingAutopilotMeta        = "autopilot_meta_enabled"
)
