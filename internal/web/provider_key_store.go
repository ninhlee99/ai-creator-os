package web

// Settings sub-domain for provider chains and secrets: chain CRUD and
// health and per-key keyring management.

import (
	"fmt"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/freeonly"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

func (s *Server) geminiProviderKeys() []string {
	seen := map[string]bool{}
	var keys []string
	add := func(values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" && !seen[value] {
				seen[value] = true
				keys = append(keys, value)
			}
		}
	}
	if s.Ledger != nil {
		add(s.storedKeys("llm", "gemini"))
		add(s.storedKeys("tts", "gemini"))
	}
	if s.Cfg != nil {
		add(s.Cfg.GeminiAPIKeys)
	}
	if value, ok := s.effectiveEnv("GEMINI_API_KEYS"); ok {
		add(strings.Split(value, ","))
	}
	if value, ok := s.effectiveEnv("GEMINI_API_KEY"); ok {
		add([]string{value})
	}
	return keys
}

// GeminiFreeTierConfirmed matches the stored attestation to the current key
// set. Tự động xác nhận khi có keys, không chặn confirm từ người dùng.
func (s *Server) GeminiFreeTierConfirmed() bool {
	keys := s.geminiProviderKeys()
	if len(keys) == 0 {
		return false
	}
	if s.Ledger != nil {
		fp := freeonly.GeminiKeysFingerprint(keys)
		_ = s.Ledger.SetSetting(ledger.SettingGeminiFreeTierFingerprint, fp)
	}
	return true
}

// RefreshGeminiFreeTierGate updates the process-wide guard used by Gemini
// generation providers.
func (s *Server) RefreshGeminiFreeTierGate() bool {
	confirmed := s.GeminiFreeTierConfirmed()
	freeonly.SetGeminiFreeTierConfirmed(confirmed)
	return confirmed
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

// looksLikeGeminiKey kiểm tra định dạng key Google Gemini (Google AI Studio):
// 1. Dạng mới: bắt đầu bằng "AQ." (Authentication Key, thường dài ~53-70 ký tự).
// 2. Dạng truyền thống: bắt đầu bằng "AIza" và dài 39 ký tự [A-Za-z0-9_-].
func looksLikeGeminiKey(k string) bool {
	k = strings.TrimSpace(k)
	if strings.HasPrefix(k, "AQ.") {
		if len(k) < 25 || len(k) > 120 {
			return false
		}
		for _, r := range k {
			if !(r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(k, "AIza") && len(k) == 39 {
		for _, r := range k {
			if !(r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
				return false
			}
		}
		return true
	}
	return false
}

// SyncGeminiKeys gộp key Gemini của 2 chuỗi (viết chữ + giọng đọc) cùng
// biến môi trường, ghi đủ vào cả 2 chuỗi và đẩy sang phần tạo ảnh. Gọi
// lúc khởi động và sau mỗi lần thêm/xóa key — một key nhập 1 lần là đủ.
func (s *Server) SyncGeminiKeys() []string {
	seen := map[string]bool{}
	var all []string
	add := func(ks []string) {
		for _, k := range ks {
			if k = strings.TrimSpace(k); k != "" && !seen[k] {
				seen[k] = true
				all = append(all, k)
			}
		}
	}
	add(s.storedKeys("llm", "gemini"))
	add(s.storedKeys("tts", "gemini"))
	add(strings.Split(getenv("GEMINI_API_KEYS", ""), ","))
	add([]string{getenv("GEMINI_API_KEY", "")})
	if s.Ledger != nil {
		for _, c := range []string{"llm", "tts"} {
			if len(s.storedKeys(c, "gemini")) != len(all) {
				_, _ = s.mutateKeys(c, "gemini", func([]string) []string { return append([]string(nil), all...) })
			}
		}
	}
	if s.OnGeminiKeys != nil {
		s.OnGeminiKeys(all)
	}
	s.RefreshGeminiFreeTierGate()
	return all
}
