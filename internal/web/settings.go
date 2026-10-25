package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

type envRow struct {
	Name string
	// Label/Group: tên dễ hiểu + nhóm nền tảng (biến kỹ thuật chỉ hiện phụ).
	Label, Group string
	NewGroup     bool // hàng đầu tiên của nhóm → in tiêu đề nhóm
	Ok           bool
	// Masked is the secret-safe tail display ("••••abcd"); empty when unset.
	Masked string
	// FromDB marks values saved through the settings UI (persisted in the
	// settings table) as opposed to process environment variables.
	FromDB bool
	// Help là giải thích ngắn cho người không rành (Đợt AA).
	Help string
}

// envLabels: biến → {tên dễ hiểu, nhóm}. Trang Cài đặt hiện tên dễ hiểu
// làm chính, tên biến chỉ là chữ nhỏ phụ (để đối chiếu tài liệu).
var envLabels = map[string][2]string{
	"TTS_API_KEY":                    {"Key Gemini dự phòng cho giọng đọc", "Google Gemini"},
	"GEMINI_TEXT_MODEL":              {"Model viết kịch bản video", "Google Gemini"},
	"GEMINI_TTS_MODEL":               {"Model giọng đọc video", "Google Gemini"},
	"TIKTOK_CLIENT_KEY":              {"Client Key ứng dụng TikTok", "TikTok"},
	"TIKTOK_CLIENT_SECRET":           {"Client Secret ứng dụng TikTok", "TikTok"},
	"TIKTOK_REDIRECT_URI":            {"Redirect URI (Callback TikTok)", "TikTok"},
	"TIKTOK_DISCOVERY_ACCESS_TOKEN":  {"Access Token TikTok Business Discovery", "TikTok Business Discovery"},
	"TIKTOK_DISCOVERY_ADVERTISER_ID": {"Advertiser ID TikTok Discovery", "TikTok Business Discovery"},
	"APIFY_API_TOKEN":                {"API Token Apify cho Radar TikTok", "Apify"},
}

// envSettingKey namespaces a UI-saved env value inside the settings table.
func envSettingKey(name string) string { return "env:" + name }

// maskSecret renders "••••" + last 4 chars; never reveals more of a secret.
func maskSecret(v string) string {
	if len(v) <= 4 {
		return "••••"
	}
	return "••••" + v[len(v)-4:]
}

// effectiveEnv resolves a variable's current value: UI-saved (settings
// table) wins over the process environment.
func (s *Server) effectiveEnv(name string) (string, bool) {
	return s.EffectiveEnv(name)
}

// EffectiveEnv resolves a config variable's current value: UI-saved
// values win over the process environment. It implements
// automation.EnvProvider — the automation layer must never read
// read-once config structs for values the UI can change (R2-W4, R2-05).
func (s *Server) EffectiveEnv(name string) (string, bool) {
	if s.Ledger != nil {
		if v, ok, err := s.Ledger.GetSetting(envSettingKey(name)); err == nil && ok {
			return v, v != ""
		}
	}
	v := getenv(name, "")
	return v, v != ""
}

// applyPersistedEnv re-applies UI-saved env values into the process
// environment at startup so call-time getenv consumers see them after a
// restart. Called from NewServer.
func (s *Server) applyPersistedEnv() {
	if s.Ledger == nil {
		return
	}
	all, err := s.Ledger.AllSettings()
	if err != nil {
		return
	}
	for k, v := range all {
		if name, ok := strings.CutPrefix(k, "env:"); ok {
			if v == "" {
				os.Unsetenv(name)
			} else {
				os.Setenv(name, v)
			}
		}
	}
}

// applyPersistedDryRun restores the dashboard's live/safe mode at startup.
// Stored settings take precedence over DRY_RUN so an operator's choice
// survives a process restart.
func (s *Server) applyPersistedDryRun() {
	if s.Ledger == nil || s.Cfg == nil {
		return
	}
	v, ok, err := s.Ledger.GetSetting("ops.dry_run")
	if err != nil {
		log.Printf("web: load dry-run setting: %v", err)
		return
	}
	if ok {
		s.Cfg.SetDryRun(v != "0")
	}
}

// handleSettingsEnvSave saves one environment variable from the Settings
// UI: it applies to the running process immediately and persists in the
// settings table, so it is re-applied at startup. An empty value clears
// the variable.
func (s *Server) handleSettingsEnvSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse env form")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	allowed := false
	for _, n := range envNames {
		if n == name {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "biến không được phép sửa tại đây", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if value == "" {
		os.Unsetenv(name)
	} else {
		os.Setenv(name, value)
	}
	if s.Ledger != nil {
		if err := s.Ledger.SetSetting(envSettingKey(name), value); err != nil {
			s.fail(w, err, "save env setting")
			return
		}
	}
	if strings.HasPrefix(name, "TIKTOK_DISCOVERY_") || name == "APIFY_API_TOKEN" {
		studio.InvalidateTikTokRadarCache()
	}
	// R2-W4 (R2-05): the runtime Config was read once at startup — refresh
	// it so the value just saved through the UI takes effect now.
	s.Cfg.RefreshEnv()
	redirect := "/settings/system?envsaved=" + name
	if strings.HasPrefix(name, "TIKTOK_") || name == "APIFY_API_TOKEN" {
		redirect += "#tab-social"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// envNames is the allowlist of environment variables editable in Settings.
// Values apply to the running process immediately and persist in the
// settings table (applied at startup), so they survive restarts.
var envNames = []string{
	"TTS_API_KEY", "GEMINI_TEXT_MODEL", "GEMINI_TTS_MODEL",
	"TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET", "TIKTOK_REDIRECT_URI",
	"TIKTOK_DISCOVERY_ACCESS_TOKEN", "TIKTOK_DISCOVERY_ADVERTISER_ID",
	"APIFY_API_TOKEN",
}

// envHelp giải thích ngắn gọn từng biến cho người không rành (Đợt AA,
// finding C3) — hiện dưới tên biến ở trang Cài đặt · Hệ thống.
var envHelp = map[string]string{
	"TTS_API_KEY":                    "Key Gemini dự phòng cho giọng đọc. Có thể để trống nếu đã lưu key trong mục quản lý API key.",
	"GEMINI_TEXT_MODEL":              "Không bắt buộc. Để trống để app tự chọn model viết kịch bản video.",
	"GEMINI_TTS_MODEL":               "Không bắt buộc. Để trống để app tự chọn model giọng đọc video.",
	"TIKTOK_CLIENT_KEY":              "Client Key của ứng dụng TikTok dùng để kết nối kênh.",
	"TIKTOK_CLIENT_SECRET":           "Client Secret đi kèm Client Key TikTok.",
	"TIKTOK_REDIRECT_URI":            "TikTok Desktop Login Kit: chỉ dùng http://localhost hoặc 127.0.0.1, port đúng với app và đường dẫn /publishers/tiktok/callback. Mặc định: http://127.0.0.1:8080/publishers/tiktok/callback. Đăng ký đúng URI trong TikTok Developer Portal.",
	"TIKTOK_DISCOVERY_ACCESS_TOKEN":  "Access Token TikTok API for Business đã cấp quyền Discovery; khác token đăng nhập TikTok Desktop ở trên.",
	"TIKTOK_DISCOVERY_ADVERTISER_ID": "Advertiser ID được cấp quyền cho TikTok Business Discovery API.",
	"APIFY_API_TOKEN":                "Token Apify dùng lấy top video xu hướng Việt Nam hoặc tìm tối đa 3 chủ đề, kèm view/tym. Mỗi lần thu thập tối đa 60 video; Apify tính phí theo kết quả.",
}

// envSettingKey namespaces a UI-saved env value inside the settings table.

// ------------------------------------------------------- notify (Đợt O2)
