package web

// Settings page core: persisted environment variables, dry-run toggle,
// kill switch, provider chain configuration storage and the VieNeu view.

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

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

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
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
		s.fail(w, err, "list accounts")
		return
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
	s.render(w, "settings", s.ctx(
		"EnvStatus", envs,
		"EnvSaved", r.URL.Query().Get("envsaved"),
		"RtmpRows", rtmps,
		"DbPath", s.Cfg.DatabasePath,
		"Usage", usage,
		"Spend", spend,
		"TTSChain", s.loadChain("tts"),
		"LLMChain", s.loadChain("llm"),
		"TTSKeys", s.keyRingStatuses("tts", "gemini"),
		"LLMKeys", s.keyRingStatuses("llm", "gemini"),
		"VieNeu", s.vieneuView(),
		"AvatarChain", s.loadChain("avatar"),
		"AvatarRealtime", s.avatarRealtime(),
		"Characters", s.characterViews(),
		"AvatarSidecar", s.avatarSidecarView(),
	))
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
	http.Redirect(w, r, "/settings?envsaved="+name+"#bien-moi-truong", http.StatusSeeOther)
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
	seeOther(w, r, "/settings")
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
