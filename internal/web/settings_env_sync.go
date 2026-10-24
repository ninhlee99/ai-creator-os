package web

import (
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"strings"
	"time"
)

// ConfigExportJSON định nghĩa cấu trúc sao lưu cấu hình JSON.
type ConfigExportJSON struct {
	Version        string            `json:"version"`
	ExportedAt     string            `json:"exported_at"`
	GeminiAPIKeys  []string          `json:"gemini_api_keys"`
	AccesstradeKey string            `json:"accesstrade_key,omitempty"`
	TelegramBot    string            `json:"telegram_bot,omitempty"`
	TelegramChat   string            `json:"telegram_chat,omitempty"`
	WebhookURL     string            `json:"webhook_url,omitempty"`
	EnvVars        map[string]string `json:"env_vars"`
	WebAIAccounts  []webai.Account   `json:"web_ai_accounts,omitempty"`
}

// handleConfigJSONExport xuất toàn bộ cấu hình dưới dạng JSON (bao gồm tài khoản WebAI).
func (s *Server) handleConfigJSONExport(w http.ResponseWriter, r *http.Request) {
	exp := ConfigExportJSON{
		Version:    "1.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		EnvVars:    make(map[string]string),
	}

	// Gemini Keys
	exp.GeminiAPIKeys = s.storedKeys("llm", "gemini")
	if len(exp.GeminiAPIKeys) == 0 && len(s.Cfg.GeminiAPIKeys) > 0 {
		exp.GeminiAPIKeys = s.Cfg.GeminiAPIKeys
	}

	// Accesstrade
	if s.Ledger != nil {
		if atKey, ok, _ := s.Ledger.GetSetting(accesstrade.KeySetting); ok && atKey != "" {
			exp.AccesstradeKey = atKey
		}
		exp.TelegramBot, _, _ = s.Ledger.GetSetting(automation.KeyNotifyTelegramBot)
		exp.TelegramChat, _, _ = s.Ledger.GetSetting(automation.KeyNotifyTelegramChat)
		exp.WebhookURL, _, _ = s.Ledger.GetSetting(automation.KeyNotifyWebhookURL)
	}

	// Env vars
	for _, name := range envNames {
		val, ok := s.EffectiveEnv(name)
		if ok && val != "" {
			exp.EnvVars[name] = val
		}
	}
	if s.Ledger != nil {
		if all, err := s.Ledger.AllSettings(); err == nil {
			for k, v := range all {
				if name, ok := strings.CutPrefix(k, "env:"); ok && v != "" {
					exp.EnvVars[name] = v
				}
			}
		}
	}

	// WebAI Accounts
	if s.WebAI != nil {
		if accountsMap, err := s.WebAI.GetAllAccounts(); err == nil {
			for _, accs := range accountsMap {
				exp.WebAIAccounts = append(exp.WebAIAccounts, accs...)
			}
		}
	}

	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		s.fail(w, err, "marshal export json")
		return
	}

	filename := fmt.Sprintf("aicos_config_%s.json", time.Now().Format("20060102_1504"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_, _ = w.Write(data)
}
