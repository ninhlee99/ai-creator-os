package web

// Settings sub-domain for provider chains and secrets: chain CRUD and
// health and per-key keyring management.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/freeonly"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

func (s *Server) handleGeminiFreeTierSave(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho cài đặt chưa sẵn sàng"})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse Gemini free-tier confirmation")
		return
	}
	confirmed := r.PostFormValue("confirm_free") == "1"
	value := ""
	message := "Đã khóa Gemini API calls."
	if confirmed {
		keys := s.geminiProviderKeys()
		if len(keys) == 0 {
			s.RefreshGeminiFreeTierGate()
			seeOther(w, r, "/settings/providers?err="+url.QueryEscape("Chưa có Gemini API key để xác nhận."))
			return
		}
		value = freeonly.GeminiKeysFingerprint(keys)
		message = fmt.Sprintf("Đã xác nhận free-tier cho %d Gemini API key hiện tại.", len(keys))
	} else {
		message = "Đã thu hồi xác nhận free-tier; Gemini API calls bị khóa."
	}
	if err := s.Ledger.SetSetting(ledger.SettingGeminiFreeTierFingerprint, value); err != nil {
		s.fail(w, err, "save Gemini free-tier confirmation")
		return
	}
	s.RefreshGeminiFreeTierGate()
	_ = s.Ledger.Decide("human", "gemini_free_tier", nil, message, map[string]any{"confirmed": confirmed})
	seeOther(w, r, "/settings/providers?ok="+url.QueryEscape(message))
}

// handleChainGet returns the current chain config as JSON (stored config,
// or defaults when nothing was saved yet). API keys are masked — the raw
// values never leave the server.
func (s *Server) handleChainGet(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if _, ok := s.chainKey(name); !ok {
		http.Error(w, "unknown chain (name=tts|llm)", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	raw, _ := MarshalChain(maskChainKeys(s.loadChain(name)))
	_, _ = w.Write([]byte(raw))
}

// maskChainKeys returns a copy of cfg with raw API keys replaced by masked
// display values. The settings JSON API never leaks secrets.
func maskChainKeys(cfg ChainConfig) ChainConfig {
	out := ChainConfig{Order: make([]ProviderEntry, len(cfg.Order))}
	for i, e := range cfg.Order {
		e.APIKey = ""
		if len(e.APIKeys) > 0 {
			masked := make([]string, len(e.APIKeys))
			for j, k := range e.APIKeys {
				masked[j] = MaskKey(k)
			}
			e.APIKeys = masked
		}
		out.Order[i] = e
	}
	return out
}

// handleChainSave stores the whole chain form: repeated pname / enabled
// (checked indices) / apikey / timeout (sec) / retries fields, aligned by
// row index. Applies to the live chain immediately.
func (s *Server) handleChainSave(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if _, ok := s.chainKey(name); !ok {
		http.Error(w, "unknown chain (name=tts|llm)", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse chain form")
		return
	}
	names := r.PostForm["pname"]
	apikeys := r.PostForm["apikey"]
	timeouts := r.PostForm["timeout"]
	retries := r.PostForm["retries"]
	enabled := map[int]bool{}
	for _, v := range r.PostForm["enabled"] {
		if i, err := strconv.Atoi(v); err == nil {
			enabled[i] = true
		}
	}
	cfg := ChainConfig{}
	stored := s.loadChain(name) // key preservation for keyring-managed rows
	for i, pname := range names {
		e := ProviderEntry{Name: strings.TrimSpace(pname), Enabled: enabled[i]}
		if e.Name == "" {
			continue
		}
		if len(withoutRetiredProviders(name, []ProviderEntry{e})) == 0 {
			continue
		}
		if e.Name == "gemini" {
			// Keys of gemini rows are managed by the keyring UI through
			// the /settings/keys/* endpoints. The row posts an empty
			// apikey placeholder to keep the repeated-field alignment;
			// an empty value preserves the stored keys instead of
			// wiping them. A non-empty apikey is still honored as the
			// legacy single-key path.
			if i < len(apikeys) && strings.TrimSpace(apikeys[i]) != "" {
				e.SetAPIKeysText(apikeys[i])
			} else if se := findChainEntry(stored, e.Name); se != nil {
				e.APIKeys, e.APIKey = se.APIKeys, se.APIKey
			}
		} else if i < len(apikeys) {
			e.APIKey = strings.TrimSpace(apikeys[i])
		}
		if i < len(timeouts) {
			if v, err := strconv.Atoi(strings.TrimSpace(timeouts[i])); err == nil && v > 0 {
				e.TimeoutSec = v
			}
		}
		if i < len(retries) {
			if v, err := strconv.Atoi(strings.TrimSpace(retries[i])); err == nil && v >= 0 {
				e.Retries = v
			}
		}
		cfg.Order = append(cfg.Order, e)
	}
	if len(cfg.Order) == 0 {
		http.Error(w, "empty chain", http.StatusBadRequest)
		return
	}
	if err := s.saveChain(name, cfg); err != nil {
		s.fail(w, err, "save chain")
		return
	}
	seeOther(w, r, "/settings/providers?ok="+url.QueryEscape("Đã lưu cấu hình nhà cung cấp."))
}

// handleChainMove reorders one provider up/down (replaces drag-drop).
func (s *Server) handleChainMove(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	dir := r.URL.Query().Get("dir")
	i, _ := strconv.Atoi(r.URL.Query().Get("i"))
	if _, ok := s.chainKey(name); !ok {
		http.Error(w, "unknown chain", http.StatusBadRequest)
		return
	}
	cfg := s.loadChain(name)
	if i >= 0 && i < len(cfg.Order) {
		j := i - 1
		if dir == "down" {
			j = i + 1
		}
		if j >= 0 && j < len(cfg.Order) {
			cfg.Order[i], cfg.Order[j] = cfg.Order[j], cfg.Order[i]
			if err := s.saveChain(name, cfg); err != nil {
				s.fail(w, err, "move chain entry")
				return
			}
		}
	}
	seeOther(w, r, "/settings/providers?ok="+url.QueryEscape("Đã đổi thứ tự nhà cung cấp."))
}

// handleChainHealth probes one provider ("Kiểm tra kết nối") and returns
// {"ok": true|false} as JSON.
func (s *Server) handleChainHealth(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	provider := r.URL.Query().Get("provider")
	w.Header().Set("Content-Type", "application/json")
	checker := s.Health[name+":"+provider]
	if checker == nil {
		checker = s.Health[provider]
	}
	ok := false
	if checker != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		func() {
			defer func() {
				_ = recover() // a bad checker must not crash the dashboard
			}()
			ok = checker.Healthy(ctx)
		}()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": ok, "provider": provider})
}

// ------------------------------------------------------- keyring (API keys)
