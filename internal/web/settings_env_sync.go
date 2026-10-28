package web

import (
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"time"
)

// ConfigExportJSON định nghĩa cấu trúc sao lưu cấu hình JSON.
type ConfigExportJSON struct {
	Version        string            `json:"version"`
	ExportedAt     string            `json:"exported_at"`
	GeminiAPIKeys  []string          `json:"gemini_api_keys,omitempty"`
	AccesstradeKey string            `json:"accesstrade_key,omitempty"`
	TelegramBot    string            `json:"telegram_bot,omitempty"`
	TelegramChat   string            `json:"telegram_chat,omitempty"`
	WebhookURL     string            `json:"webhook_url,omitempty"`
	EnvVars        map[string]string `json:"env_vars,omitempty"`
	WebAIAccounts  []webai.Account   `json:"web_ai_accounts,omitempty"` // import compatibility; exports never include sessions
}

// handleConfigJSONExport exports configuration without provider session credentials.
func (s *Server) handleConfigJSONExport(w http.ResponseWriter, r *http.Request) {
	exp := ConfigExportJSON{
		Version:    "2.0",
		ExportedAt: time.Now().Format(time.RFC3339),
		EnvVars:    make(map[string]string),
	}
	// Portable JSON stays secret-free: API keys, notification tokens, webhook
	// URLs, env values, and browser sessions are credentials. Import remains
	// backward compatible for older exports, but new exports never disclose them.

	data, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		s.fail(w, err, "marshal export json")
		return
	}

	filename := fmt.Sprintf("aicos_config_%s.json", time.Now().Format("20060102_1504"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_, _ = w.Write(data)
}
