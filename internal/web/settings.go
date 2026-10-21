package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch and provider chain configuration storage.

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/backup"
	"github.com/ninhlee99/ai-creator-os/internal/engines/gmodels"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/notify"
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
	"TTS_API_KEY":             {"Key Gemini cho giọng đọc", "Google Gemini"},
	"GEMINI_TEXT_MODEL":       {"Model viết chữ (tùy chọn)", "Google Gemini"},
	"GEMINI_IMAGE_MODEL":      {"Model tạo ảnh (tùy chọn)", "Google Gemini"},
	"GEMINI_TTS_MODEL":        {"Model giọng đọc (tùy chọn)", "Google Gemini"},
	"GEMINI_VIDEO_MODEL":      {"Model tạo video Veo (tùy chọn)", "Google Gemini"},
	"OPENAI_API_KEY":          {"Key OpenAI (ChatGPT / GPT-6 Luna)", "OpenAI & Claude & AI Khác"},
	"ANTHROPIC_API_KEY":       {"Key Anthropic (Claude 5.5 Sonnet)", "OpenAI & Claude & AI Khác"},
	"ELEVENLABS_API_KEY":      {"Key ElevenLabs (Giọng đọc siêu thực)", "OpenAI & Claude & AI Khác"},
	"TIKTOK_CLIENT_KEY":       {"Client Key ứng dụng TikTok", "TikTok"},
	"TIKTOK_CLIENT_SECRET":    {"Client Secret ứng dụng TikTok", "TikTok"},
	"TIKTOK_REDIRECT_URI":     {"Redirect URI (Callback TikTok)", "TikTok"},
	"YOUTUBE_CLIENT_ID":       {"Client ID Google (nối YouTube)", "YouTube"},
	"YOUTUBE_CLIENT_SECRET":   {"Client Secret Google", "YouTube"},
	"YOUTUBE_API_KEY":         {"Key đọc số liệu YouTube", "YouTube"},
	"YOUTUBE_DEFAULT_PRIVACY": {"Chế độ hiển thị khi đăng", "YouTube"},
	"YOUTUBE_GOLDEN_HOUR":     {"Hẹn giờ vàng công khai", "YouTube"},
	"META_APP_ID":             {"App ID Meta (nối Facebook)", "Facebook · Instagram · Threads"},
	"META_APP_SECRET":         {"App Secret Meta", "Facebook · Instagram · Threads"},
	"META_REDIRECT_URI":       {"Redirect URI (Callback Facebook)", "Facebook · Instagram · Threads"},
	"FB_PAGE_ID":              {"ID Trang Facebook", "Facebook · Instagram · Threads"},
	"META_PAGE_TOKEN":         {"Mã truy cập Trang (Meta)", "Facebook · Instagram · Threads"},
	"FB_DRAFT_ONLY":           {"Facebook: chỉ tải lên chưa công khai", "Facebook · Instagram · Threads"},
	"IG_USER_ID":              {"ID tài khoản Instagram", "Facebook · Instagram · Threads"},
	"IG_AUTO_PUBLISH":         {"Instagram: tự đăng công khai", "Facebook · Instagram · Threads"},
	"THREADS_USER_ID":         {"ID tài khoản Threads", "Facebook · Instagram · Threads"},
	"THREADS_TOKEN":           {"Mã truy cập Threads", "Facebook · Instagram · Threads"},
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

func (s *Server) chainKey(name string) (string, bool) {
	switch name {
	case "tts":
		return ledger.SettingTTSChain, true
	case "llm":
		return ledger.SettingLLMChain, true
	}
	return "", false
}

// loadChain returns the stored chain config, falling back to defaults when
// nothing was saved yet.
func (s *Server) loadChain(name string) ChainConfig {
	key, ok := s.chainKey(name)
	if !ok {
		return ChainConfig{}
	}
	raw, found, err := s.Ledger.GetSetting(key)
	if err == nil && found && raw != "" {
		if c := UnmarshalChain(raw); len(c.Order) > 0 {
			previous := c.Order
			c.Order = withoutRetiredProviders(name, previous)
			retiredRemoved := len(c.Order) != len(previous)
			if retiredRemoved && !hasEnabledProvider(c.Order) {
				if name == "tts" {
					return defaultChainWithSavedGeminiKeys(DefaultTTSConfig(s.Cfg.GeminiAPIKeys), c.Order)
				}
				return defaultChainWithSavedGeminiKeys(DefaultLLMConfig(s.Cfg.GeminiAPIKeys), c.Order)
			}
			if len(c.Order) > 0 {
				return c
			}
		}
	}
	if name == "tts" {
		return DefaultTTSConfig(s.Cfg.GeminiAPIKeys)
	}
	return DefaultLLMConfig(s.Cfg.GeminiAPIKeys)
}

// saveChain persists the config and applies it to the live chain
// immediately (no restart).
func (s *Server) saveChain(name string, c ChainConfig) error {
	key, ok := s.chainKey(name)
	if !ok {
		return fmt.Errorf("unknown chain %q", name)
	}
	raw, err := MarshalChain(c)
	if err != nil {
		return err
	}
	if err := s.Ledger.SetSetting(key, raw); err != nil {
		return err
	}
	s.applyChain(name, ChainConfigJSON(raw))
	return nil
}

func (s *Server) applyChain(name string, raw ChainConfigJSON) {
	switch name {
	case "tts":
		if s.TTS != nil {
			s.TTS.SetConfig(raw)
		}
	case "llm":
		// LLM is the narrow LLMClient interface; the real chain (injected
		// by the wiring worker) also implements SetConfig.
		if sc, ok := s.LLM.(interface{ SetConfig(ChainConfigJSON) }); ok && s.LLM != nil {
			sc.SetConfig(raw)
		}
	}
}

// settingsData builds the full settings context once; each sub-page picks
// the keys it needs (R2-W3: một trang = một mối quan tâm).
func (s *Server) settingsData(r *http.Request) map[string]any {
	envs := make([]envRow, 0, len(envNames))
	for _, e := range envNames {
		v, ok := s.effectiveEnv(e)
		lg := envLabels[e]
		row := envRow{Name: e, Ok: ok, Help: envHelp[e], Label: lg[0], Group: lg[1]}
		if row.Label == "" {
			row.Label = e
		}
		row.NewGroup = len(envs) == 0 || envs[len(envs)-1].Group != row.Group
		if ok {
			row.Masked = maskSecret(v)
		}
		if s.Ledger != nil {
			if _, inDB, err := s.Ledger.GetSetting(envSettingKey(e)); err == nil && inDB {
				row.FromDB = true
			}
		}
		envs = append(envs, row)
	}
	// Chi phí API (khối trong "Trạng thái hệ thống") — fail-soft: lỗi truy
	// vấn chỉ ghi log, không làm sập trang Cài đặt.
	usage, uerr := s.queryUsage()
	if uerr != nil {
		log.Printf("web: settings usage: %v", uerr)
	}
	var spend float64
	if s.Ledger != nil {
		if v, err := s.Ledger.DailySpendUSD(); err == nil {
			spend = v
		}
	}
	webAIViews, webAIConnected := s.webAIPlatformViews()
	return map[string]any{
		"EnvStatus":      envs,
		"WebAIPlatforms": webAIViews,
		"WebAIConnected": webAIConnected,
		"EnvSaved":       r.URL.Query().Get("envsaved"),
		"EnvErr":         r.URL.Query().Get("enverr"), // Đợt AA: lỗi validate biến
		"Ok":             r.URL.Query().Get("ok"),     // Đợt AA: toast chung trang system
		"Err":            r.URL.Query().Get("err"),
		"DbPath":         s.Cfg.DatabasePath,
		"Usage":          usage,
		"Spend":          spend,
		"TTSChain":       s.loadChain("tts"),
		"LLMChain":       s.loadChain("llm"),
		"TTSKeys":        s.keyRingStatuses("tts", "gemini"),
		"LLMKeys":        s.keyRingStatuses("llm", "gemini"),
		// R2-W4: persisted master switch + daily API budget, editable in
		// Settings · Hệ thống.
		"MasterOn":   automation.MasterOn(s.settings()),
		"APIBudget":  automation.APIBudgetUSD(s.settings(), s.Cfg.DailyAPIBudgetUSD),
		"BudgetFrom": s.apiBudgetFrom(),
		// Accesstrade (Đợt B): key + trạng thái kết nối cho tab Cài đặt · Accesstrade.
		"ATKeySet":    s.atKeySet(),
		"ATMasked":    s.atMasked(),
		"ATCampaigns": s.atCachedCount(),
		"ATStoreOK":   s.AT != nil,
		// Accesstrade automation (Đợt G): interval tick cho tab Cài đặt ·
		// Accesstrade (công tắc on/off nằm ở trang Affiliate, POST /at/settings).
		"ATHunterHours": s.atSettingInt(automation.KeyATHunterIntervalHrs, 24),
		// Đợt AK: bật/tắt tick AT gộp về đây (trước nằm rời ở trang Affiliate).
		"ATAuto": map[string]any{
			"HunterOn":      s.atSettingOn(automation.KeyATHunterEnabled, true),
			"OrderSyncOn":   s.atSettingOn(automation.KeyATOrderSyncEnabled, true),
			"CampaignChkOn": s.atSettingOn(automation.KeyATCampaignCheckOn, true),
			"HunterVideos":  s.atSettingInt(automation.KeyATHunterVideos, 3),
		},
		"ATOrderSyncMins": s.atSettingInt(automation.KeyATOrderSyncIntervalM, 30),
		// R2-W7: legacy /api/* switch (default OFF), app version, runtime
		// probes, pending-restore flag — all shown in Settings · Hệ thống.
		"APIEnabled":     automation.APIEnabled(s.settings()),
		"Version":        s.Cfg.Version,
		"RuntimeTools":   probeRuntimeTools(),
		"AICapabilities": s.aiCapabilityViews(),
		"RestorePending": backup.PendingRestore(s.DataDir),
		"DataDir":        s.DataDir,
		// Đợt R+S: heartbeat các tick + trạng thái ngắt mạch quota.
		"TickBeats":    s.tickBeatViews(),
		"QuotaCircuit": s.quotaCircuitView(),
		// Đợt L: % đĩa đã dùng cho thẻ Hệ thống (disk guard tự dọn khi đầy).
		"DiskUsage": s.diskUsageView(),
		// Đợt M2: lần sao lưu tự động gần nhất.
		"BackupLastOK": automation.BackupLastOK(s.settings()),
		// Đợt V: thư mục offsite cho bản sao lưu.
		"BackupOffsiteDir": s.atSettingStr(automation.KeyBackupOffsiteDir, ""),
		// Đợt O2: cấu hình Telegram (token không bao giờ hiện lại ra UI).
		"NotifyOn":      s.atSettingOn(automation.KeyNotifyTelegramOn, false),
		"NotifyChat":    s.atSettingStr(automation.KeyNotifyTelegramChat, ""),
		"NotifyWebhook": s.atSettingStr(automation.KeyNotifyWebhookURL, ""),
		"NotifyHasBot": func() bool {
			v, ok, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramBot)
			return ok && strings.TrimSpace(v) != ""
		}(),
		"GeminiModels": s.geminiModelsView(),
	}
}

// diskUsageView trả về % đĩa đã dùng tại DataDir cho thẻ Hệ thống.
func (s *Server) diskUsageView() map[string]any {
	pct, err := automation.DiskUsagePct(s.DataDir)
	if err != nil || s.DataDir == "" {
		return map[string]any{"Pct": -1, "Badge": "badge-draft"}
	}
	return map[string]any{"Pct": pct, "Badge": diskBadge(pct)}
}

// diskBadge: đỏ từ 85% (ngưỡng tự dọn mặc định của self-heal), cam từ 75%.
// Đợt AK: trước đây ≥90% dùng badge-no (xám, nghĩa "tắt") — đĩa sắp đầy mà
// trông như bình thường.
func diskBadge(pct int) string {
	switch {
	case pct >= 85:
		return "badge-err"
	case pct >= 75:
		return "badge-warn"
	}
	return "badge-ok"
}

// apiBudgetFrom reports where the API budget value comes from (honest
// label for the UI): "ui" when a persisted override exists, "env" when
// it falls back to DAILY_API_BUDGET_USD.
func (s *Server) apiBudgetFrom() string {
	if _, ok := s.settings().Get(automation.KeyAPIBudgetUSD); ok {
		return "ui"
	}
	return "env"
}

// GeminiKeys tập hợp tất cả các API key của Google Gemini từ cấu hình, DB và biến môi trường.
func (s *Server) GeminiKeys() []string {
	var keys []string
	seen := make(map[string]bool)
	add := func(list []string) {
		for _, k := range list {
			k = strings.TrimSpace(k)
			if k != "" && !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	add(s.storedKeys("llm", "gemini"))
	add(s.storedKeys("tts", "gemini"))
	add(s.Cfg.GeminiAPIKeys)
	if env, ok := s.effectiveEnv("GEMINI_API_KEYS"); ok {
		add(strings.Split(env, ","))
	}
	if env, ok := s.effectiveEnv("GEMINI_API_KEY"); ok {
		add([]string{env})
	}
	if env, ok := s.effectiveEnv("TTS_API_KEY"); ok {
		add([]string{env})
	}
	return keys
}

// geminiModelsView chuẩn bị dữ liệu hiển thị khối cập nhật model Gemini ở Cài đặt · Hệ thống.
func (s *Server) geminiModelsView() map[string]any {
	report := gmodels.LastReport()
	lastCheckStr := "Chưa quét. Bấm nút bên phải để kiểm tra & cập nhật model mới nhất."
	if report != nil {
		if report.Success {
			lastCheckStr = fmt.Sprintf("Lúc %s (Thành công, %d models khả dụng)",
				report.CheckedAt.Format("15:04 02/01/2006"), report.TotalModels)
		} else {
			lastCheckStr = fmt.Sprintf("Lúc %s (Lỗi: %s)",
				report.CheckedAt.Format("15:04 02/01/2006"), report.Error)
		}
	}

	modelItem := func(k gmodels.Kind, label, desc string) map[string]any {
		cur := gmodels.Resolve(k)
		def := gmodels.Default(k)
		envVal, hasEnv := s.effectiveEnv(gmodels.EnvName(k))
		source := "Mặc định hệ thống"
		badge := "badge-info"
		if hasEnv && envVal != "" {
			source = "Ghi đè thủ công (" + gmodels.EnvName(k) + ")"
			badge = "badge-warn"
		} else if cur != def {
			source = "Đã cập nhật từ Google API"
			badge = "badge-ok"
		}
		return map[string]any{
			"Kind":    label,
			"Desc":    desc,
			"Model":   cur,
			"Default": def,
			"Source":  source,
			"Badge":   badge,
			"EnvVar":  gmodels.EnvName(k),
		}
	}

	return map[string]any{
		"LastCheck": lastCheckStr,
		"Status":    "🛠️ Cập nhật thủ công theo yêu cầu (Bấm nút để quét & nâng cấp toàn bộ model)",
		"Report":    report,
		"Items": []map[string]any{
			modelItem(gmodels.Text, "Văn bản & Kịch bản (LLM)", "Sáng tạo kịch bản video viral, hook, cốt truyện audio"),
			modelItem(gmodels.Image, "Tạo ảnh & Phân cảnh (Image)", "Nano Banana 2.1 — Tạo nhân vật Thần tiên tỉ tỉ, sản phẩm affiliate, cảnh truyện"),
			modelItem(gmodels.TTS, "Giọng đọc truyền cảm (TTS)", "Giọng đọc truyện audio đa cảm xúc (gemini-3.8-flash-tts)"),
			modelItem(gmodels.Video, "Tạo video chuyển động (Veo)", "Google Veo 3.1 tạo video chuyển động điện ảnh chân thực"),
		},
	}
}

// handleModelsUpdate kích hoạt kiểm tra và nâng cấp model Gemini tức thì từ giao diện web.
func (s *Server) handleModelsUpdate(w http.ResponseWriter, r *http.Request) {
	keys := s.GeminiKeys()
	if len(keys) == 0 {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Chưa có API key Gemini để kiểm tra cập nhật model."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rep, err := gmodels.CheckAndUpdate(ctx, nil, "", keys, s.DataDir)
	if err != nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Kiểm tra model thất bại: "+err.Error()))
		return
	}
	msg := fmt.Sprintf("✓ Đã kiểm tra Gemini API (%d models khả dụng). Text: %s | Image: %s | TTS: %s | Video: %s",
		rep.TotalModels, gmodels.Resolve(gmodels.Text), gmodels.Resolve(gmodels.Image), gmodels.Resolve(gmodels.TTS), gmodels.Resolve(gmodels.Video))
	if rep.Updated && len(rep.Changes) > 0 {
		msg = "🚀 Phát hiện & nâng cấp lên model mới: "
		for k, chg := range rep.Changes {
			msg += fmt.Sprintf("[%s: %s] ", k, chg)
		}
	}
	_ = s.Ledger.Decide("human", "model_update_check", nil, msg, map[string]any{"updated": rep.Updated, "total": rep.TotalModels})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(msg))
}

// settingsPage renders one settings sub-page with its nav key and the
// requested data keys.
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request, page, key string, keys ...string) {
	data := s.settingsData(r)
	if page == "accesstrade" {
		var accounts any
		if s.Mgr != nil {
			accounts, _ = s.Mgr.List()
		}
		var accountID int64
		if s.Ledger != nil {
			if raw, ok, _ := s.Ledger.GetSetting(automation.KeyATHunterAccountID); ok {
				accountID, _ = strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			}
		}
		data["ATHunterAccounts"] = accounts
		data["ATHunterAccountID"] = accountID
	}
	ctx := s.ctx("SettingsPage", page)
	for _, k := range keys {
		ctx[k] = data[k]
	}
	s.render(w, key, ctx)
}

// handleSettingsIndex keeps the old entry point working: it redirects to
// the first sub-page.
func (s *Server) handleSettingsIndex(w http.ResponseWriter, r *http.Request) {
	seeOther(w, r, "/settings/system")
}

// handleAuthPasswordSet đặt/đổi mật khẩu dashboard (Đợt T).
func (s *Server) handleAuthPasswordSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse password")
		return
	}
	// Đợt AF: đã có mật khẩu thì phải nhập đúng mật khẩu hiện tại mới được
	// đổi — chống người khác đổi mật khẩu khóa chủ máy (máy dùng chung).
	if s.authEnabled() {
		cur := r.PostFormValue("current_password")
		stored, ok, _ := s.Ledger.GetSetting(KeyAuthPasswordHash)
		if !ok || !checkAuthPassword(cur, stored) {
			seeOther(w, r, "/settings/system?err="+url.QueryEscape("Mật khẩu hiện tại không đúng."))
			return
		}
	}
	pw := r.PostFormValue("password")
	if err := s.setAuthPassword(pw); err != nil {
		seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Lỗi: "+err.Error()))
		return
	}
	_ = s.Ledger.Decide("human", "auth_password", nil, "Đặt/đổi mật khẩu truy cập dashboard.", nil)
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã đặt mật khẩu. Từ giờ truy cập dashboard cần đăng nhập."))
}

// handleAuthPasswordDisable tắt mật khẩu dashboard (Đợt T).
func (s *Server) handleAuthPasswordDisable(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse password")
		return
	}
	// Đợt AF: tắt cũng cần nhập đúng mật khẩu hiện tại.
	cur := r.PostFormValue("current_password")
	stored, ok, _ := s.Ledger.GetSetting(KeyAuthPasswordHash)
	if !ok || !checkAuthPassword(cur, stored) {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Mật khẩu hiện tại không đúng."))
		return
	}
	if err := s.clearAuthPassword(); err != nil {
		seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Lỗi: "+err.Error()))
		return
	}
	_ = s.Ledger.Decide("human", "auth_password", nil, "Tắt mật khẩu truy cập dashboard.", map[string]any{"off": true})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã tắt mật khẩu."))
}

// handleSettingsHeThong: "App đang chạy chế độ gì, còn thiếu biến nào?"
func (s *Server) handleSettingsHeThong(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "system", "settings_system",
		"EnvStatus", "EnvSaved", "EnvErr", "Ok", "Err", "DbPath", "Usage", "Spend",
		"MasterOn", "APIBudget", "BudgetFrom",
		"APIEnabled", "Version", "RestorePending", "DataDir", "DiskUsage",
		"BackupLastOK", "BackupOffsiteDir", "NotifyOn", "NotifyWebhook", "NotifyChat", "NotifyHasBot",
		"TickBeats", "QuotaCircuit", "WebAIPlatforms", "WebAIConnected", "GeminiModels")
}

// handleSettingsNhaCungCap: "AI dùng nhà cung cấp nào trước, key nào còn sống?"
func (s *Server) handleSettingsNhaCungCap(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "providers", "settings_providers",
		"TTSChain", "TTSKeys", "LLMChain", "LLMKeys")
}

// handleSettingsAPIToggle opens/closes the legacy /api/* JSON endpoints
// (R2-W7, R2-09). Default OFF; the operator turns it on explicitly.
func (s *Server) handleSettingsAPIToggle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse api toggle")
		return
	}
	on := r.PostFormValue("value") == "1"
	if err := automation.SetAPIEnabled(s.settings(), on); err != nil {
		s.fail(w, err, "save api switch")
		return
	}
	msg := "Đã TẮT API /api/*."
	if on {
		msg = "Đã BẬT API /api/* — chỉ dùng nội bộ; cân nhắc rủi ro khi mở app ra mạng LAN."
	}
	_ = s.Ledger.Decide("human", "api_switch", nil, msg, map[string]any{"on": on})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(msg))
}

// aiCapabilityView là một dòng trong bảng "Khả năng AI".
type aiCapabilityView struct {
	Label     string
	Status    string // ok | fail | unknown
	Badge     string // badge-ok | badge-err | badge-warn
	Text      string // nhãn tiếng Việt
	CheckedAt string
	Detail    string
}

// aiCapabilityViews đọc trạng thái khả năng AI từ studio (Film Wave 3).
func (s *Server) aiCapabilityViews() []aiCapabilityView {
	if s.Studio == nil {
		return nil
	}
	var out []aiCapabilityView
	for _, c := range s.Studio.GetCapabilities() {
		v := aiCapabilityView{Label: c.Label, Status: c.Status,
			CheckedAt: c.CheckedAt, Detail: c.Detail}
		switch c.Status {
		case "ok":
			v.Badge, v.Text = "badge-ok", "Hoạt động"
		case "fail":
			v.Badge, v.Text = "badge-err", "Không hoạt động"
		default:
			v.Badge, v.Text = "badge-warn", "Chưa kiểm tra"
		}
		out = append(out, v)
	}
	return out
}

// handleSettingsAnToan: "Dừng khẩn cấp bằng cách nào?"
func (s *Server) handleSettingsAnToan(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "safety", "settings_safety")
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

// handleSettingsMaster flips the persisted network-daemon master switch
// (R2-W4, R2-08). Default OFF; the network daemon reads the gate live,
// so no restart is needed. Every change is written to the decision log.
func (s *Server) handleSettingsMaster(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse master form")
		return
	}
	on := r.PostFormValue("value") == "1" || r.PostFormValue("on") == "1"
	if err := automation.SetMasterOn(s.settings(), on); err != nil {
		s.fail(w, err, "save master switch")
		return
	}
	msg := "Đã TẮT chế độ tự động: hệ thống đứng yên."
	if on {
		msg = "Đã BẬT chế độ tự động: hệ thống chạy theo lịch."
	}
	_ = s.Ledger.Decide("system", "master_switch", nil, msg, map[string]any{"on": on})
	// Đợt AG: quay lại trang đã bấm nút (dashboard hoặc system).
	back := r.PostFormValue("back")
	if back == "" || !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/settings/system"
	}
	seeOther(w, r, back+"?ok="+url.QueryEscape(msg))
}

// handleSettingsAPIBudget saves the UI-tuned daily API spend ceiling
// (R2-W4, R2-05). The persisted value wins over DAILY_API_BUDGET_USD.
func (s *Server) handleSettingsAPIBudget(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse budget form")
		return
	}
	raw := strings.TrimSpace(r.PostFormValue("value"))
	usd, err := strconv.ParseFloat(raw, 64)
	if err != nil || usd < 0 || usd > 10000 {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Ngân sách API không hợp lệ (0–10000 USD)."))
		return
	}
	if err := automation.SetAPIBudgetUSD(s.settings(), usd); err != nil {
		s.fail(w, err, "save api budget")
		return
	}
	_ = s.Ledger.Decide("system", "api_budget", nil,
		fmt.Sprintf("ngân sách API/ngày đặt thành $%.2f qua UI", usd),
		map[string]any{"usd": usd})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(fmt.Sprintf("Đã lưu ngân sách API: $%.2f/ngày.", usd)))
}

func (s *Server) handleSettingsDryRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse dry-run form")
		return
	}
	// NOTE: the required bugfix — dry_run is ON exactly when value == "on".
	on := r.PostFormValue("value") == "on"
	value := "0"
	if on {
		value = "1"
	}
	if err := s.Ledger.SetSetting("ops.dry_run", value); err != nil {
		s.fail(w, err, "save dry-run setting")
		return
	}
	s.Cfg.SetDryRun(on)
	if err := s.Ledger.Decide("human", "dry_run", nil,
		fmt.Sprintf("set dry_run=%v via dashboard", on), map[string]any{}); err != nil {
		log.Printf("web: decide dry_run: %v", err)
	}
	seeOther(w, r, "/settings/safety")
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	s.Cfg.SetKillSwitch(true)
	if err := s.Ledger.Decide("human", "kill_switch", nil,
		"engaged via dashboard", map[string]any{}); err != nil {
		log.Printf("web: decide kill: %v", err)
	}
	seeOther(w, r, "/")
}

func (s *Server) handleUnkill(w http.ResponseWriter, r *http.Request) {
	s.Cfg.SetKillSwitch(false)
	if err := s.Ledger.Decide("human", "kill_switch", nil,
		"released via dashboard", map[string]any{}); err != nil {
		log.Printf("web: decide unkill: %v", err)
	}
	seeOther(w, r, "/")
}

// ------------------------------------------------------- chain settings API

// envNames is the allowlist of environment variables editable in Settings.
// Values apply to the running process immediately and persist in the
// settings table (applied at startup), so they survive restarts.
var envNames = []string{
	"TTS_API_KEY", "GEMINI_TEXT_MODEL", "GEMINI_IMAGE_MODEL", "GEMINI_TTS_MODEL",
	"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "ELEVENLABS_API_KEY",
	"TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET", "TIKTOK_REDIRECT_URI",
	"YOUTUBE_CLIENT_ID", "YOUTUBE_CLIENT_SECRET", "YOUTUBE_API_KEY",
	"YOUTUBE_DEFAULT_PRIVACY", "YOUTUBE_GOLDEN_HOUR",
	"META_APP_ID", "META_APP_SECRET", "META_REDIRECT_URI",
	"FB_PAGE_ID", "META_PAGE_TOKEN", "FB_DRAFT_ONLY", "IG_USER_ID", "IG_AUTO_PUBLISH",
	"THREADS_USER_ID", "THREADS_TOKEN"}

// envHelp giải thích ngắn gọn từng biến cho người không rành (Đợt AA,
// finding C3) — hiện dưới tên biến ở trang Cài đặt · Hệ thống.
var envHelp = map[string]string{
	"TTS_API_KEY":             "Key Gemini dùng cho giọng đọc (Text-to-Speech). Chưa có thì app dùng giọng đọc miễn phí của máy.",
	"OPENAI_API_KEY":          "Key OpenAI từ platform.openai.com — Dùng cho kịch bản ChatGPT GPT-6 Luna và sinh nội dung bán hàng.",
	"ANTHROPIC_API_KEY":       "Key Anthropic từ console.anthropic.com — Dùng cho đạo diễn phân cảnh điện ảnh Claude 5.5 Sonnet.",
	"ELEVENLABS_API_KEY":      "Key ElevenLabs từ elevenlabs.io — Lồng tiếng voiceover siêu thực đa cảm xúc.",
	"TIKTOK_CLIENT_KEY":       "Client Key của app TikTok (tạo ở developers.tiktok.com) — để nối tài khoản TikTok.",
	"TIKTOK_CLIENT_SECRET":    "Client Secret đi kèm Client Key TikTok.",
	"TIKTOK_REDIRECT_URI":     "Mặc định http://127.0.0.1:8080/publishers/tiktok/callback. Phải khớp 100% với Redirect URI trên developers.tiktok.com.",
	"YOUTUBE_CLIENT_ID":       "Client ID OAuth Google (loại “Desktop app”) — để bấm “Nối YouTube” ở trang Đa nền tảng.",
	"YOUTUBE_CLIENT_SECRET":   "Client Secret đi kèm Client ID Google.",
	"META_APP_ID":             "App ID trong Meta for Developers — bật Facebook Login và pages_show_list, pages_manage_posts, pages_read_engagement.",
	"META_APP_SECRET":         "App Secret Meta; lưu kín. Facebook OAuth dùng để chọn Trang và lưu Page access token cho kênh.",
	"META_REDIRECT_URI":       "Tùy chọn. Phải khớp Valid OAuth Redirect URI trong Meta App; mặc định callback loopback của AICOS.",
	"FB_PAGE_ID":              "ID Trang Facebook — để đăng bài lên Trang. Bỏ trống thì không đăng Facebook.",
	"YOUTUBE_API_KEY":         "Key YouTube Data API (riêng với OAuth) — để đọc số liệu Analytics. Không bắt buộc.",
	"YOUTUBE_DEFAULT_PRIVACY": "Chế độ riêng tư mặc định khi đăng video. Chỉ nhận: private, unlisted, public.",
	"YOUTUBE_GOLDEN_HOUR":     "1 = video YouTube (private) tự hẹn giờ công khai vào khung giờ vàng VN kế tiếp (11:30 hoặc 19:30). Trống = giữ private để bạn tự duyệt.",
	"META_PAGE_TOKEN":         "Page access token Meta (developers.facebook.com → Graph API Explorer, quyền pages_manage_posts + instagram_content_publish). Dùng chung cho Facebook Page và Instagram.",
	"FB_DRAFT_ONLY":           "1 (mặc định) = video Facebook tải lên ở trạng thái chưa công khai để bạn duyệt; 0 = đăng công khai luôn.",
	"IG_USER_ID":              "ID tài khoản Instagram Business/Creator đã nối với Trang Facebook — để đăng Reels.",
	"IG_AUTO_PUBLISH":         "Instagram không có nháp qua API. 1 = đăng Reels công khai luôn; trống/0 = chỉ tải lên + xử lý (an toàn), không đăng.",
	"THREADS_USER_ID":         "ID tài khoản Threads (developers.facebook.com → Threads API) — để đăng bài kèm link sản phẩm/video.",
	"THREADS_TOKEN":           "Access token Threads (quyền threads_basic + threads_content_publish).",
	"GEMINI_TEXT_MODEL":       "Không bắt buộc. Trống = app tự chọn model Gemini viết chữ (mặc định " + gmodels.Default(gmodels.Text) + "; Google tắt model thì app tự dò model mới).",
	"GEMINI_IMAGE_MODEL":      "Không bắt buộc. Trống = app tự chọn model Gemini tạo ảnh (mặc định " + gmodels.Default(gmodels.Image) + "; tự dò model mới khi model cũ bị tắt).",
	"GEMINI_TTS_MODEL":        "Không bắt buộc. Trống = app tự chọn model Gemini giọng đọc (mặc định " + gmodels.Default(gmodels.TTS) + "; tự dò model mới khi model cũ bị tắt).",
}

// envSettingKey namespaces a UI-saved env value inside the settings table.

// ------------------------------------------------------- notify (Đợt O2)

// handleNotifySave lưu cấu hình Telegram (bot token, chat ID, bật/tắt).
func (s *Server) handleNotifySave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse notify form")
		return
	}
	set := func(k, v string) {
		_ = s.Ledger.SetSetting(k, v)
	}
	set(automation.KeyNotifyTelegramOn, map[bool]string{true: "1", false: "0"}[r.PostFormValue("notify_on") == "1"])
	if bot := strings.TrimSpace(r.PostFormValue("notify_bot")); bot != "" {
		set(automation.KeyNotifyTelegramBot, bot) // chỉ ghi đè khi nhập mới
	}
	set(automation.KeyNotifyTelegramChat, strings.TrimSpace(r.PostFormValue("notify_chat")))
	set(automation.KeyNotifyWebhookURL, strings.TrimSpace(r.PostFormValue("notify_webhook")))
	_ = s.Ledger.Decide("human", "notify_save", nil, "Đã lưu cấu hình thông báo Telegram.", map[string]any{})
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã lưu cấu hình Telegram."))
}

// handleNotifyTest gửi tin nhắn thử qua Telegram với cấu hình hiện tại.
func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	bot, _, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramBot)
	chat, _, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramChat)
	if err := notify.SendTelegram(bot, chat, "✅ AI Creator OS đã nối Telegram. Từ giờ cảnh báo sẽ gửi về đây."); err != nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Gửi thử thất bại: "+err.Error()))
		return
	}
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã gửi tin nhắn thử — kiểm tra Telegram."))
}

// handleNotifyDiscordTest gửi tin nhắn thử qua Discord Webhook.
func (s *Server) handleNotifyDiscordTest(w http.ResponseWriter, r *http.Request) {
	webhook, _, _ := s.Ledger.GetSetting(automation.KeyNotifyWebhookURL)
	if strings.TrimSpace(webhook) == "" {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Chưa cấu hình Discord Webhook URL."))
		return
	}
	if err := notify.SendDiscord(webhook, "🔔 Kiểm tra Discord Webhook", "✅ Kết nối thành công! AI Creator OS đã liên kết với kênh Discord này. Từ giờ các sự kiện render video, xuất bản và cảnh báo sẽ gửi về đây."); err != nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Gửi Discord thất bại: "+err.Error()))
		return
	}
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape("Đã gửi tin nhắn thử thành công tới Discord!"))
}
