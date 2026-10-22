package web

import (
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func (s *Server) importFromJSON(w http.ResponseWriter, r *http.Request, raw []byte) {
	var exp EnvExportJSON
	if err := json.Unmarshal(raw, &exp); err != nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("File JSON không hợp lệ: "+err.Error()))
		return
	}

	envCount := 0
	geminiCount := 0
	atCount := 0
	webaiCount := 0

	for _, k := range exp.GeminiAPIKeys {
		if s.addGeminiKey(k) {
			geminiCount++
		}
	}

	if exp.AccesstradeKey != "" {
		_ = s.Ledger.SetSetting(accesstrade.KeySetting, exp.AccesstradeKey)
		atCount++
	}
	if exp.TelegramBot != "" {
		_ = s.Ledger.SetSetting(automation.KeyNotifyTelegramBot, exp.TelegramBot)
		envCount++
	}
	if exp.TelegramChat != "" {
		_ = s.Ledger.SetSetting(automation.KeyNotifyTelegramChat, exp.TelegramChat)
		envCount++
	}
	if exp.WebhookURL != "" {
		_ = s.Ledger.SetSetting(automation.KeyNotifyWebhookURL, exp.WebhookURL)
		envCount++
	}

	for k, v := range exp.EnvVars {
		if k == "" || v == "" {
			continue
		}
		os.Setenv(k, v)
		if err := s.Ledger.SetSetting(envSettingKey(k), v); err == nil {
			envCount++
		}
	}

	if s.WebAI != nil && len(exp.WebAIAccounts) > 0 {
		for _, acc := range exp.WebAIAccounts {
			if !acc.TierConfirmed {
				acc.Tier = "unknown"
			}
			if _, err := s.WebAI.AddAccount(acc); err == nil {
				webaiCount++
			}
		}
	}

	s.Cfg.RefreshEnv()
	s.applyPersistedEnv()
	s.SyncGeminiKeys()
	_ = s.Ledger.Decide("human", "env_import_json", nil, fmt.Sprintf("Import JSON: %d biến, %d key Gemini, %d WebAI", envCount, geminiCount, webaiCount), nil)

	msg := fmt.Sprintf("✓ Đã nhập thành công từ JSON: %d biến môi trường, %d key Gemini, %d tài khoản WebAI.", envCount, geminiCount, webaiCount)
	log.Printf("web: %s", msg)
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(msg))
}

// addGeminiKey thêm 1 API key vào cả hai chuỗi (llm và tts) và kích hoạt đồng bộ.
func (s *Server) addGeminiKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || !looksLikeGeminiKey(key) {
		return false
	}
	added := false
	for _, chain := range []string{"llm", "tts"} {
		_, err := s.mutateKeys(chain, "gemini", func(keys []string) []string {
			for _, k := range keys {
				if k == key {
					return keys
				}
			}
			added = true
			return append(keys, key)
		})
		if err != nil {
			log.Printf("web: addGeminiKey to %s: %v", chain, err)
		}
	}
	if added {
		s.SyncGeminiKeys()
	}
	return added
}
