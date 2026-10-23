package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
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
	"TTS_API_KEY":              {"Key Gemini cho giọng đọc", "Google Gemini"},
	"GEMINI_TEXT_MODEL":        {"Model viết chữ (tùy chọn)", "Google Gemini"},
	"GEMINI_IMAGE_MODEL":       {"Model tạo ảnh (tùy chọn)", "Google Gemini"},
	"GEMINI_TTS_MODEL":         {"Model giọng đọc (tùy chọn)", "Google Gemini"},
	"GEMINI_VIDEO_MODEL":       {"Model tạo video Veo (tùy chọn)", "Google Gemini"},
	"OPENAI_API_KEY":           {"Key OpenAI (ChatGPT / GPT-6 Luna)", "OpenAI & Claude & AI Khác"},
	"ANTHROPIC_API_KEY":        {"Key Anthropic (Claude 5.5 Sonnet)", "OpenAI & Claude & AI Khác"},
	"ELEVENLABS_API_KEY":       {"Key ElevenLabs (Giọng đọc siêu thực)", "OpenAI & Claude & AI Khác"},
	"TIKTOK_CLIENT_KEY":        {"Client Key ứng dụng TikTok", "TikTok"},
	"TIKTOK_CLIENT_SECRET":     {"Client Secret ứng dụng TikTok", "TikTok"},
	"TIKTOK_REDIRECT_URI":      {"Redirect URI (Callback TikTok)", "TikTok"},
	"YOUTUBE_CLIENT_ID":        {"Client ID Google (nối YouTube)", "YouTube"},
	"YOUTUBE_CLIENT_SECRET":    {"Client Secret Google", "YouTube"},
	"YOUTUBE_REDIRECT_URI":     {"Redirect URI (Callback YouTube VPS)", "YouTube"},
	"BIO_GOOGLE_CLIENT_ID":     {"Client ID Google (đăng nhập thử đồ)", "Bio · Google Login"},
	"BIO_GOOGLE_CLIENT_SECRET": {"Client Secret Google (đăng nhập thử đồ)", "Bio · Google Login"},
	"BIO_GOOGLE_REDIRECT_URI":  {"Redirect URI (Callback Google Bio)", "Bio · Google Login"},
	"YOUTUBE_API_KEY":          {"Key đọc số liệu YouTube", "YouTube"},
	"YOUTUBE_DEFAULT_PRIVACY":  {"Chế độ hiển thị khi đăng", "YouTube"},
	"YOUTUBE_GOLDEN_HOUR":      {"Hẹn giờ vàng công khai", "YouTube"},
	"META_APP_ID":              {"App ID Meta (nối Facebook)", "Facebook · Instagram · Threads"},
	"META_APP_SECRET":          {"App Secret Meta", "Facebook · Instagram · Threads"},
	"META_REDIRECT_URI":        {"Redirect URI (Callback Facebook)", "Facebook · Instagram · Threads"},
	"FB_PAGE_ID":               {"ID Trang Facebook", "Facebook · Instagram · Threads"},
	"META_PAGE_TOKEN":          {"Mã truy cập Trang (Meta)", "Facebook · Instagram · Threads"},
	"FB_DRAFT_ONLY":            {"Facebook: chỉ tải lên chưa công khai", "Facebook · Instagram · Threads"},
	"IG_USER_ID":               {"ID tài khoản Instagram", "Facebook · Instagram · Threads"},
	"IG_AUTO_PUBLISH":          {"Instagram: tự đăng công khai", "Facebook · Instagram · Threads"},
	"THREADS_USER_ID":          {"ID tài khoản Threads", "Facebook · Instagram · Threads"},
	"THREADS_TOKEN":            {"Mã truy cập Threads", "Facebook · Instagram · Threads"},
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
	// Đợt AA: validate giá trị các biến có miền hợp lệ — tránh gõ sai rồi
	// chỉ phát hiện lúc đăng video mới lỗi khó hiểu.
	if name == "YOUTUBE_DEFAULT_PRIVACY" && value != "" {
		switch strings.ToLower(value) {
		case "private", "unlisted", "public":
			value = strings.ToLower(value)
		default:
			seeOther(w, r, "/settings/system?enverr="+url.QueryEscape("YOUTUBE_DEFAULT_PRIVACY chỉ nhận: private, unlisted hoặc public."))
			return
		}
	}
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
	// R2-W4 (R2-05): the runtime Config was read once at startup — refresh
	// it so the value just saved through the UI takes effect now.
	s.Cfg.RefreshEnv()
	http.Redirect(w, r, "/settings/system?envsaved="+name, http.StatusSeeOther)
}

// envNames is the allowlist of environment variables editable in Settings.
// Values apply to the running process immediately and persist in the
// settings table (applied at startup), so they survive restarts.
var envNames = []string{
	"TTS_API_KEY", "GEMINI_TEXT_MODEL", "GEMINI_IMAGE_MODEL", "GEMINI_TTS_MODEL",
	"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "ELEVENLABS_API_KEY",
	"TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET", "TIKTOK_REDIRECT_URI",
	"YOUTUBE_CLIENT_ID", "YOUTUBE_CLIENT_SECRET", "YOUTUBE_REDIRECT_URI", "YOUTUBE_API_KEY",
	"BIO_GOOGLE_CLIENT_ID", "BIO_GOOGLE_CLIENT_SECRET", "BIO_GOOGLE_REDIRECT_URI",
	"YOUTUBE_DEFAULT_PRIVACY", "YOUTUBE_GOLDEN_HOUR",
	"META_APP_ID", "META_APP_SECRET", "META_REDIRECT_URI",
	"FB_PAGE_ID", "META_PAGE_TOKEN", "FB_DRAFT_ONLY", "IG_USER_ID", "IG_AUTO_PUBLISH",
	"THREADS_USER_ID", "THREADS_TOKEN"}

// envHelp giải thích ngắn gọn từng biến cho người không rành (Đợt AA,
// finding C3) — hiện dưới tên biến ở trang Cài đặt · Hệ thống.
var envHelp = map[string]string{
	"TTS_API_KEY":              "Key Gemini dùng cho giọng đọc (Text-to-Speech). Chưa có thì app dùng giọng đọc miễn phí của máy.",
	"OPENAI_API_KEY":           "Key OpenAI từ platform.openai.com — Dùng cho kịch bản ChatGPT GPT-6 Luna và sinh nội dung bán hàng.",
	"ANTHROPIC_API_KEY":        "Key Anthropic từ console.anthropic.com — Dùng cho đạo diễn phân cảnh điện ảnh Claude 5.5 Sonnet.",
	"ELEVENLABS_API_KEY":       "Key ElevenLabs từ elevenlabs.io — Lồng tiếng voiceover siêu thực đa cảm xúc.",
	"TIKTOK_CLIENT_KEY":        "Client Key của app TikTok (tạo ở developers.tiktok.com) — để nối tài khoản TikTok.",
	"TIKTOK_CLIENT_SECRET":     "Client Secret đi kèm Client Key TikTok.",
	"TIKTOK_REDIRECT_URI":      "Để trống = Desktop/loopback local. VPS: nhập URI HTTPS đã đăng ký trong TikTok Web Login Kit, dạng https://<host>/publishers/tiktok/callback.",
	"YOUTUBE_CLIENT_ID":        "Client ID OAuth Google. Local dùng loại Desktop app; VPS dùng loại Web application.",
	"YOUTUBE_CLIENT_SECRET":    "Client Secret đi kèm Google OAuth Client ID; với VPS, dùng Web application client.",
	"YOUTUBE_REDIRECT_URI":     "Tùy chọn cho VPS: URL HTTPS đã đăng ký trong Google Web OAuth, dạng https://<host>/publishers/youtube/callback. Để trống thì dùng callback loopback local.",
	"BIO_GOOGLE_CLIENT_ID":     "OAuth Client ID cho khách đăng nhập Google để thử đồ. Nếu cả cặp BIO_GOOGLE_CLIENT_ID/SECRET để trống, app dùng cặp YOUTUBE_CLIENT_ID/SECRET.",
	"BIO_GOOGLE_CLIENT_SECRET": "OAuth Client Secret cho khách đăng nhập Google để thử đồ. Nếu cả cặp BIO_GOOGLE_CLIENT_ID/SECRET để trống, app dùng cặp YOUTUBE_CLIENT_ID/SECRET.",
	"BIO_GOOGLE_REDIRECT_URI":  "Callback HTTPS đã đăng ký trong Google Cloud OAuth, dạng https://<host>/bio/google/callback. Bắt buộc trên VPS; local loopback tự tạo callback.",
	"META_APP_ID":              "App ID trong Meta for Developers — bật Facebook Login và pages_show_list, pages_manage_posts, pages_read_engagement.",
	"META_APP_SECRET":          "App Secret Meta; lưu kín. Facebook OAuth dùng để chọn Trang và lưu Page access token cho kênh.",
	"META_REDIRECT_URI":        "Tùy chọn. Phải khớp Valid OAuth Redirect URI trong Meta App; mặc định callback loopback của MiuNova.",
	"FB_PAGE_ID":               "ID Trang Facebook — để đăng bài lên Trang. Bỏ trống thì không đăng Facebook.",
	"YOUTUBE_API_KEY":          "Key YouTube Data API (riêng với OAuth) — để đọc số liệu Analytics. Không bắt buộc.",
	"YOUTUBE_DEFAULT_PRIVACY":  "Chế độ riêng tư mặc định khi đăng video. Chỉ nhận: private, unlisted, public.",
	"YOUTUBE_GOLDEN_HOUR":      "1 = video YouTube (private) tự hẹn giờ công khai vào khung giờ vàng VN kế tiếp (11:30 hoặc 19:30). Trống = giữ private để bạn tự duyệt.",
	"META_PAGE_TOKEN":          "Page access token Meta (developers.facebook.com → Graph API Explorer, quyền pages_manage_posts + instagram_content_publish). Dùng chung cho Facebook Page và Instagram.",
	"FB_DRAFT_ONLY":            "1 (mặc định) = video Facebook tải lên ở trạng thái chưa công khai để bạn duyệt; 0 = đăng công khai luôn.",
	"IG_USER_ID":               "ID tài khoản Instagram Business/Creator đã nối với Trang Facebook — để đăng Reels.",
	"IG_AUTO_PUBLISH":          "Instagram không có nháp qua API. 1 = đăng Reels công khai luôn; trống/0 = chỉ tải lên + xử lý (an toàn), không đăng.",
	"THREADS_USER_ID":          "ID tài khoản Threads (developers.facebook.com → Threads API) — để đăng bài kèm link sản phẩm/video.",
	"THREADS_TOKEN":            "Access token Threads (quyền threads_basic + threads_content_publish).",
	"GEMINI_TEXT_MODEL":        "Không bắt buộc. Trống = app tự chọn model Gemini viết chữ (mặc định " + gmodels.Default(gmodels.Text) + "; Google tắt model thì app tự dò model mới).",
	"GEMINI_IMAGE_MODEL":       "Không bắt buộc. Trống = app tự chọn model Gemini tạo ảnh (mặc định " + gmodels.Default(gmodels.Image) + "; tự dò model mới khi model cũ bị tắt).",
	"GEMINI_TTS_MODEL":         "Không bắt buộc. Trống = app tự chọn model Gemini giọng đọc (mặc định " + gmodels.Default(gmodels.TTS) + "; tự dò model mới khi model cũ bị tắt).",
}

// envSettingKey namespaces a UI-saved env value inside the settings table.

// ------------------------------------------------------- notify (Đợt O2)
