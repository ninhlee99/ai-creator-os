package web

// Settings sub-domain for provider chains and secrets: chain CRUD and
// health and per-key keyring management.

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
	if provider == "gemini" && !looksLikeGeminiKey(key) {
		writeJSONErr(w, "Đây không phải key Gemini: key Gemini (Google AI Studio) bắt đầu bằng “AIza” hoặc “AQ.”. "+
			"Lấy key miễn phí tại aistudio.google.com/apikey rồi dán lại.", http.StatusBadRequest)
		return
	}
	duplicate := false
	if provider == "gemini" {
		// 1 key Gemini dùng chung cho viết chữ + giọng đọc + tạo ảnh.
		defer s.SyncGeminiKeys()
		for _, other := range []string{"llm", "tts"} {
			if other == chainName {
				continue
			}
			_, _ = s.mutateKeys(other, provider, func(keys []string) []string {
				for _, k := range keys {
					if k == key {
						return keys
					}
				}
				return append(keys, key)
			})
		}
	}
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
	removed := before[idx]
	keys, err := s.mutateKeys(chainName, provider, func(keys []string) []string {
		return append(keys[:idx], keys[idx+1:]...)
	})
	if err == nil && provider == "gemini" {
		// Key dùng chung → xóa ở mọi chuỗi + phần tạo ảnh.
		for _, other := range []string{"llm", "tts"} {
			if other != chainName {
				_, _ = s.mutateKeys(other, provider, func(keys []string) []string {
					out := keys[:0]
					for _, k := range keys {
						if k != removed {
							out = append(out, k)
						}
					}
					return out
				})
			}
		}
		s.SyncGeminiKeys()
	}
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

// ----------------------------------------------------------- legacy JSON API
