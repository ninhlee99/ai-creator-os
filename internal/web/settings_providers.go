package web

// Settings sub-domain for provider chains and secrets: chain CRUD and
// health, per-key keyring management, and the VieNeu/avatar sidecar panels.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleChainGet returns the current chain config as JSON (stored config,
// or defaults when nothing was saved yet). API keys are masked — the raw
// values never leave the server.
func (s *Server) handleChainGet(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if _, ok := s.chainKey(name); !ok {
		http.Error(w, "unknown chain (name=tts|llm|avatar)", http.StatusBadRequest)
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
		http.Error(w, "unknown chain (name=tts|llm|avatar)", http.StatusBadRequest)
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
	seeOther(w, r, "/settings/nha-cung-cap")
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
	seeOther(w, r, "/settings/nha-cung-cap")
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

// keyChainParams resolves ?chain=tts|llm + ?provider= for the key endpoints.
func (s *Server) keyChainParams(r *http.Request) (chainName, provider string, ok bool) {
	chainName = r.URL.Query().Get("chain")
	provider = strings.TrimSpace(r.URL.Query().Get("provider"))
	if _, ok = s.chainKey(chainName); !ok {
		return "", "", false
	}
	if provider == "" {
		return "", "", false
	}
	return chainName, provider, true
}

func writeJSONErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

// keyRingStatuses returns the render-ready per-key state for a provider.
// It prefers the live keyring (real cooldown/invalid state); when the
// chain adapter is not wired (or doesn't rotate keys) it falls back to
// the stored config's keys, all reported "ok". Raw keys never leave the
// server — every entry is masked.
func (s *Server) keyRingStatuses(chainName, provider string) []KeyStatus {
	var src any
	switch chainName {
	case "tts":
		src = s.TTS
	case "llm":
		src = s.LLM
	}
	if src != nil {
		if kp, ok := src.(KeyStatusProvider); ok {
			if live := kp.KeyStatus(provider); len(live) > 0 {
				for i := range live {
					live[i].Total = len(live)
					live[i].FillDerived()
				}
				return live
			}
		}
	}
	// Fallback: stored config keys, reported healthy.
	cfg := s.loadChain(chainName)
	for _, e := range cfg.Order {
		if e.Name != provider {
			continue
		}
		keys := e.APIKeys
		if len(keys) == 0 && e.APIKey != "" {
			keys = []string{e.APIKey}
		}
		out := make([]KeyStatus, 0, len(keys))
		for i, k := range keys {
			st := KeyStatus{Index: i, Last4: last4(k), State: "ok", Total: len(keys)}
			st.FillDerived()
			out = append(out, st)
		}
		return out
	}
	return []KeyStatus{}
}

// storedKeys returns the provider row's keys from the stored chain config.
func (s *Server) storedKeys(chainName, provider string) []string {
	cfg := s.loadChain(chainName)
	for _, e := range cfg.Order {
		if e.Name != provider {
			continue
		}
		if len(e.APIKeys) > 0 {
			return append([]string(nil), e.APIKeys...)
		}
		if e.APIKey != "" {
			return []string{e.APIKey}
		}
		return nil
	}
	return nil
}

// mutateKeys loads the stored chain, applies fn to the provider row's key
// list, then saves — persisting to the DB and applying to the live chain
// immediately (no restart). Returns the updated key list.
func (s *Server) mutateKeys(chainName, provider string, fn func(keys []string) []string) ([]string, error) {
	cfg := s.loadChain(chainName)
	idx := -1
	for i, e := range cfg.Order {
		if e.Name == provider {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("unknown provider %q in chain %q", provider, chainName)
	}
	keys := s.storedKeys(chainName, provider)
	keys = fn(keys)
	cfg.Order[idx].SetAPIKeysText(strings.Join(keys, "\n"))
	if err := s.saveChain(chainName, cfg); err != nil {
		return nil, err
	}
	return keys, nil
}

// handleKeyAdd appends one API key to the provider's keyring (deduplicated)
// and applies it to the live chain immediately. JSON, no page reload.
func (s *Server) handleKeyAdd(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONErr(w, "không đọc được form", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.PostFormValue("key"))
	if key == "" {
		writeJSONErr(w, "key trống — hãy dán API key vào ô nhập", http.StatusBadRequest)
		return
	}
	duplicate := false
	keys, err := s.mutateKeys(chainName, provider, func(keys []string) []string {
		for _, k := range keys {
			if k == key {
				duplicate = true
				return keys
			}
		}
		return append(keys, key)
	})
	if err != nil {
		writeJSONErr(w, "không lưu được key: "+err.Error(), http.StatusInternalServerError)
		return
	}
	masked := MaskKey(key)
	if err := s.Ledger.Decide("human", "api_key_add", nil,
		fmt.Sprintf("thêm API key %s vào %s/%s%s", masked, chainName, provider,
			map[bool]string{true: " (đã có)", false: ""}[duplicate]),
		map[string]any{"chain": chainName, "provider": provider, "masked": masked}); err != nil {
		log.Printf("web: decide api_key_add: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "duplicate": duplicate, "masked": masked, "total": len(keys),
	})
}

// handleKeyDelete removes one API key by index and applies immediately.
func (s *Server) handleKeyDelete(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONErr(w, "không đọc được form", http.StatusBadRequest)
		return
	}
	idx, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("idx")))
	if err != nil {
		writeJSONErr(w, "thiếu idx", http.StatusBadRequest)
		return
	}
	before := s.storedKeys(chainName, provider)
	if idx < 0 || idx >= len(before) {
		writeJSONErr(w, "key không tồn tại", http.StatusBadRequest)
		return
	}
	masked := MaskKey(before[idx])
	keys, err := s.mutateKeys(chainName, provider, func(keys []string) []string {
		return append(keys[:idx], keys[idx+1:]...)
	})
	if err != nil {
		writeJSONErr(w, "không xóa được key: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Ledger.Decide("human", "api_key_delete", nil,
		fmt.Sprintf("xóa API key %s khỏi %s/%s", masked, chainName, provider),
		map[string]any{"chain": chainName, "provider": provider, "masked": masked}); err != nil {
		log.Printf("web: decide api_key_delete: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "masked": masked, "total": len(keys),
	})
}

// handleKeyStatus returns the masked per-key state as JSON for the keyring
// UI (initial page render uses the same data server-side).
func (s *Server) handleKeyStatus(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	keys := s.keyRingStatuses(chainName, provider)
	if keys == nil {
		keys = []KeyStatus{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "keys": keys})
}

// handleKeyTest probes one key (ValidateKey) and returns the result as
// JSON. The key stays masked in the response; error text comes from the
// provider and contains only the masked form.
func (s *Server) handleKeyTest(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONErr(w, "không đọc được form", http.StatusBadRequest)
		return
	}
	idx, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("idx")))
	if err != nil {
		writeJSONErr(w, "thiếu idx", http.StatusBadRequest)
		return
	}
	if keys := s.storedKeys(chainName, provider); idx < 0 || idx >= len(keys) {
		writeJSONErr(w, "key không tồn tại", http.StatusBadRequest)
		return
	}
	var tester KeyTester
	switch chainName {
	case "tts":
		tester, _ = s.TTS.(KeyTester)
	case "llm":
		tester, _ = s.LLM.(KeyTester)
	}
	if tester == nil {
		writeJSONErr(w, "provider chưa hỗ trợ kiểm tra key", http.StatusNotImplemented)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var testErr error
	func() {
		defer func() { _ = recover() }()
		testErr = tester.ValidateKey(ctx, provider, idx)
	}()
	w.Header().Set("Content-Type", "application/json")
	if testErr != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": testErr.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ------------------------------------------------------------- VieNeu panel

func (s *Server) vieneuRequired(w http.ResponseWriter, r *http.Request) bool {
	if s.VieNeu == nil {
		http.Error(w, "VieNeu sidecar chưa được kết nối", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *Server) handleVieneuStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.VieNeu == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"connected": false})
		return
	}
	state, detail := s.VieNeu.Status()
	voices := s.VieNeu.Voices()
	if len(voices) == 0 {
		voices = VieNeuVoiceFallback
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"connected": true, "state": state, "detail": detail, "voices": voices,
	})
}

// handleVieneuEnsure starts the model download in the background; progress
// is polled via /settings/vieneu/progress.
func (s *Server) handleVieneuEnsure(w http.ResponseWriter, r *http.Request) {
	if !s.vieneuRequired(w, r) {
		return
	}
	s.kickVieneuEnsure()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"started": true})
}

func (s *Server) handleVieneuProgress(w http.ResponseWriter, r *http.Request) {
	s.vieneuMu.Lock()
	defer s.vieneuMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"connected":  s.VieNeu != nil,
		"downloaded": s.vieneuDownloaded,
		"total":      s.vieneuTotal,
		"done":       s.vieneuDone,
		"error":      s.vieneuErr,
	})
}

func (s *Server) handleVieneuRestart(w http.ResponseWriter, r *http.Request) {
	if !s.vieneuRequired(w, r) {
		return
	}
	if err := s.VieNeu.Restart(r.Context()); err != nil {
		log.Printf("web: vieneu restart: %v", err)
	}
	seeOther(w, r, "/settings/model-local")
}

func (s *Server) handleVieneuVoice(w http.ResponseWriter, r *http.Request) {
	if !s.vieneuRequired(w, r) {
		return
	}
	_ = r.ParseForm()
	if preset := strings.TrimSpace(r.PostFormValue("preset")); preset != "" {
		if err := s.VieNeu.SetVoice(preset); err != nil {
			log.Printf("web: vieneu set voice: %v", err)
		}
	}
	seeOther(w, r, "/settings/model-local")
}

// ----------------------------------------------------------- legacy JSON API
