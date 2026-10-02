package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch, provider chain configuration storage and the VieNeu view.

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
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

type envRow struct {
	Name string
	Ok   bool
	// Masked is the secret-safe tail display ("••••abcd"); empty when unset.
	Masked string
	// FromDB marks values saved through the settings UI (persisted in the
	// settings table) as opposed to process environment variables.
	FromDB bool
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

type rtmpRow struct {
	Username string
	Ref      string
	Ok       bool
}

func (s *Server) chainKey(name string) (string, bool) {
	switch name {
	case "tts":
		return ledger.SettingTTSChain, true
	case "llm":
		return ledger.SettingLLMChain, true
	case "avatar":
		return ledger.SettingAvatarChain, true
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
			return c
		}
	}
	if name == "tts" {
		return DefaultTTSConfig(s.Cfg.GeminiAPIKeys)
	}
	if name == "avatar" {
		return DefaultAvatarConfig()
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
	case "avatar":
		if s.Avatar != nil {
			s.Avatar.SetConfig(raw)
		}
	}
}

type vieneuView struct {
	Connected bool
	State     string
	Detail    string
	Voices    []string
}

func (s *Server) vieneuView() vieneuView {
	v := vieneuView{}
	if s.VieNeu == nil {
		return v
	}
	v.Connected = true
	v.State, v.Detail = s.VieNeu.Status()
	v.Voices = s.VieNeu.Voices()
	if len(v.Voices) == 0 {
		v.Voices = VieNeuVoiceFallback
	}
	return v
}

// settingsData builds the full settings context once; each sub-page picks
// the keys it needs (R2-W3: một trang = một mối quan tâm).
func (s *Server) settingsData(r *http.Request) map[string]any {
	envs := make([]envRow, 0, len(envNames))
	for _, e := range envNames {
		v, ok := s.effectiveEnv(e)
		row := envRow{Name: e, Ok: ok}
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
	accounts, err := s.Mgr.List()
	if err != nil {
		log.Printf("web: settings list accounts: %v", err)
	}
	rtmps := make([]rtmpRow, 0, len(accounts))
	for _, a := range accounts {
		rtmps = append(rtmps, rtmpRow{Username: a.Username, Ref: a.RtmpKeyRef, Ok: a.RtmpKey() != ""})
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
	return map[string]any{
		"EnvStatus":      envs,
		"EnvSaved":       r.URL.Query().Get("envsaved"),
		"RtmpRows":       rtmps,
		"DbPath":         s.Cfg.DatabasePath,
		"Usage":          usage,
		"Spend":          spend,
		"TTSChain":       s.loadChain("tts"),
		"LLMChain":       s.loadChain("llm"),
		"TTSKeys":        s.keyRingStatuses("tts", "gemini"),
		"LLMKeys":        s.keyRingStatuses("llm", "gemini"),
		"VieNeu":         s.vieneuView(),
		"AvatarChain":    s.loadChain("avatar"),
		"AvatarRealtime": s.avatarRealtime(),
		"Characters":     s.characterViews(),
		"AvatarSidecar":  s.avatarSidecarView(),
		// R2-W4: persisted master switch + daily API budget, editable in
		// Settings · Hệ thống.
		"MasterOn":   automation.MasterOn(s.settings()),
		"APIBudget":  automation.APIBudgetUSD(s.settings(), s.Cfg.DailyAPIBudgetUSD),
		"BudgetFrom": s.apiBudgetFrom(),
		// R2-W7: legacy /api/* switch (default OFF), app version, runtime
		// probes, pending-restore flag — all shown in Settings · Hệ thống.
		"APIEnabled":     automation.APIEnabled(s.settings()),
		"Version":        s.Cfg.Version,
		"RuntimeTools":   probeRuntimeTools(),
		"AICapabilities": s.aiCapabilityViews(),
		"RestorePending": backup.PendingRestore(s.DataDir),
		"DataDir":        s.DataDir,
	}
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

// settingsPage renders one settings sub-page with its nav key and the
// requested data keys.
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request, page, key string, keys ...string) {
	data := s.settingsData(r)
	ctx := s.ctx("SettingsPage", page)
	for _, k := range keys {
		ctx[k] = data[k]
	}
	s.render(w, key, ctx)
}

// handleSettingsIndex keeps the old entry point working: it redirects to
// the first sub-page.
func (s *Server) handleSettingsIndex(w http.ResponseWriter, r *http.Request) {
	seeOther(w, r, "/settings/he-thong")
}

// handleSettingsHeThong: "App đang chạy chế độ gì, còn thiếu biến nào?"
func (s *Server) handleSettingsHeThong(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "he-thong", "settings_he_thong",
		"EnvStatus", "EnvSaved", "RtmpRows", "DbPath", "Usage", "Spend",
		"MasterOn", "APIBudget", "BudgetFrom",
		"APIEnabled", "Version", "RestorePending", "DataDir")
}

// handleSettingsNhaCungCap: "AI dùng nhà cung cấp nào trước, key nào còn sống?"
func (s *Server) handleSettingsNhaCungCap(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "nha-cung-cap", "settings_nha_cung_cap",
		"TTSChain", "TTSKeys", "LLMChain", "LLMKeys", "AvatarChain", "AvatarRealtime")
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
	seeOther(w, r, "/settings/he-thong?ok="+url.QueryEscape(msg))
}

// handleSettingsModelLocal: "Model local đã sẵn sàng chưa?"
func (s *Server) handleSettingsModelLocal(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "model-local", "settings_model_local",
		"VieNeu", "AvatarSidecar", "RuntimeTools", "AICapabilities")
}

// aiCapabilityView là một dòng trong bảng "Khả năng AI".
type aiCapabilityView struct {
	Label     string
	Status    string // ok | fail | unknown
	Badge     string // badge-ok | badge-no | badge-warn
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
			v.Badge, v.Text = "badge-no", "Không hoạt động"
		default:
			v.Badge, v.Text = "badge-warn", "Chưa kiểm tra"
		}
		out = append(out, v)
	}
	return out
}

// handleProbeVideo quay thử đúng 1 clip 8s bằng Veo — TỐN TIỀN THẬT nên UI
// hiện modal cảnh báo trước (data-confirm), và job chạy nền để không treo
// request (Veo poll vài phút).
func (s *Server) handleProbeVideo(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		_ = s.Studio.ProbeVideoGen(ctx)
	}()
	seeOther(w, r, "/settings/model-local?ok="+url.QueryEscape(
		"Đang kiểm tra quay video (~8s Veo, tốn phí thật) — quay lại sau ít phút để xem kết quả."))
}

// handleSettingsNhanVat: "Có những khuôn mặt AI nào, render thử ra sao?"
func (s *Server) handleSettingsNhanVat(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "nhan-vat", "settings_nhan_vat", "Characters")
}

// handleSettingsAnToan: "Dừng khẩn cấp bằng cách nào?"
func (s *Server) handleSettingsAnToan(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "an-toan", "settings_an_toan")
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
	// R2-W4 (R2-05): the runtime Config was read once at startup — refresh
	// it so the value just saved through the UI takes effect now.
	s.Cfg.RefreshEnv()
	http.Redirect(w, r, "/settings/he-thong?envsaved="+name, http.StatusSeeOther)
}

// handleSettingsMaster flips the persisted network-daemon master switch
// (R2-W4, R2-08). Default OFF; the network daemon reads the gate live,
// so no restart is needed. Every change is written to the decision log.
func (s *Server) handleSettingsMaster(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse master form")
		return
	}
	on := r.PostFormValue("value") == "1"
	if err := automation.SetMasterOn(s.settings(), on); err != nil {
		s.fail(w, err, "save master switch")
		return
	}
	msg := "Đã TẮT công tắc chính: daemon mạng đứng yên (không tự mở live)."
	if on {
		msg = "Đã BẬT công tắc chính: daemon mạng được phép chạy theo lịch (kill switch / DRY-RUN vẫn chặn như thường)."
	}
	_ = s.Ledger.Decide("system", "master_switch", nil, msg, map[string]any{"on": on})
	seeOther(w, r, "/settings/he-thong?ok="+url.QueryEscape(msg))
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
		seeOther(w, r, "/settings/he-thong?err="+url.QueryEscape("Ngân sách API không hợp lệ (0–10000 USD)."))
		return
	}
	if err := automation.SetAPIBudgetUSD(s.settings(), usd); err != nil {
		s.fail(w, err, "save api budget")
		return
	}
	_ = s.Ledger.Decide("system", "api_budget", nil,
		fmt.Sprintf("ngân sách API/ngày đặt thành $%.2f qua UI", usd),
		map[string]any{"usd": usd})
	seeOther(w, r, "/settings/he-thong?ok="+url.QueryEscape(fmt.Sprintf("Đã lưu ngân sách API: $%.2f/ngày.", usd)))
}

// avatarRealtime reports whether a CLOUD avatar tier (HeyGen/D-ID) is
// enabled — the only tiers that can truly stream in realtime. The local
// MuseTalk tier renders offline only (ước tính 6–10 phút cho clip 60
// giây trên Mac M1) and must never count toward this badge, even though
// its contract exposes a stream interface (see docs/RESEARCH.md §6).
func (s *Server) avatarRealtime() bool {
	for _, p := range s.loadChain("avatar").Order {
		if p.Enabled && p.Name != "local" {
			return true
		}
	}
	return false
}

func (s *Server) handleSettingsDryRun(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	// NOTE: the required bugfix — dry_run is ON exactly when value == "on".
	on := r.PostFormValue("value") == "on"
	s.Cfg.SetDryRun(on)
	if err := s.Ledger.Decide("human", "dry_run", nil,
		fmt.Sprintf("set dry_run=%v via dashboard", on), map[string]any{}); err != nil {
		log.Printf("web: decide dry_run: %v", err)
	}
	seeOther(w, r, "/settings/an-toan")
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
var envNames = []string{"TTS_API_KEY", "TIKTOK_SHOP_APP_KEY", "TIKTOK_SHOP_APP_SECRET",
	"TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET",
	"YOUTUBE_CLIENT_ID", "YOUTUBE_CLIENT_SECRET", "FB_PAGE_ID", "YOUTUBE_API_KEY",
	"YOUTUBE_DEFAULT_PRIVACY"}

// envSettingKey namespaces a UI-saved env value inside the settings table.
