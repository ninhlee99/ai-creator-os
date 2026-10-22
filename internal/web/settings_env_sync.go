package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"strings"
	"time"
)

// EnvExportJSON định nghĩa cấu trúc sao lưu cấu hình JSON.
type EnvExportJSON struct {
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

// handleEnvExport xuất toàn bộ cấu hình ra file định dạng standard .env (aicos.env).
func (s *Server) handleEnvExport(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	buf.WriteString("# ==============================================================================\n")
	buf.WriteString("# MiuNova — Environment Configuration Backup\n")
	buf.WriteString(fmt.Sprintf("# Xuất lúc: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	buf.WriteString("# Cách sử dụng trên máy mới:\n")
	buf.WriteString("# 1. Đặt file này vào thư mục app dưới tên .env, HOẶC\n")
	buf.WriteString("# 2. Vào Cài đặt · Hệ thống -> Nhập cấu hình và tải file này lên.\n")
	buf.WriteString("# ==============================================================================\n\n")

	// 1. Gemini API Keys
	geminiKeys := s.storedKeys("llm", "gemini")
	if len(geminiKeys) == 0 && len(s.Cfg.GeminiAPIKeys) > 0 {
		geminiKeys = s.Cfg.GeminiAPIKeys
	}
	if len(geminiKeys) > 0 {
		buf.WriteString("# --- Google Gemini AI (Keyring Pool) ---\n")
		buf.WriteString(fmt.Sprintf("GEMINI_API_KEYS=\"%s\"\n\n", strings.Join(geminiKeys, ",")))
	}

	// 2. Accesstrade
	if s.Ledger != nil {
		if atKey, ok, _ := s.Ledger.GetSetting(accesstrade.KeySetting); ok && atKey != "" {
			buf.WriteString("# --- Tiếp Thị Liên Kết (Affiliate) ---\n")
			buf.WriteString(fmt.Sprintf("ACCESSTRADE_API_KEY=\"%s\"\n\n", atKey))
		}
	}

	// 3. Telegram & Webhook
	if s.Ledger != nil {
		bot, _, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramBot)
		chat, _, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramChat)
		hook, _, _ := s.Ledger.GetSetting(automation.KeyNotifyWebhookURL)
		on, _, _ := s.Ledger.GetSetting(automation.KeyNotifyTelegramOn)
		if bot != "" || chat != "" || hook != "" {
			buf.WriteString("# --- Thông Báo & Cảnh Báo ---\n")
			if bot != "" {
				buf.WriteString(fmt.Sprintf("NOTIFY_TELEGRAM_BOT=\"%s\"\n", bot))
			}
			if chat != "" {
				buf.WriteString(fmt.Sprintf("NOTIFY_TELEGRAM_CHAT=\"%s\"\n", chat))
			}
			if hook != "" {
				buf.WriteString(fmt.Sprintf("NOTIFY_WEBHOOK_URL=\"%s\"\n", hook))
			}
			if on != "" {
				buf.WriteString(fmt.Sprintf("NOTIFY_TELEGRAM_ON=\"%s\"\n", on))
			}
			buf.WriteString("\n")
		}
	}

	// 4. Các biến môi trường khác (envNames + UI-saved env)
	buf.WriteString("# --- Biến Môi Trường & Khóa API Dịch Vụ ---\n")
	exported := map[string]bool{"GEMINI_API_KEYS": true, "ACCESSTRADE_API_KEY": true}
	for _, name := range envNames {
		if exported[name] {
			continue
		}
		val, ok := s.EffectiveEnv(name)
		if ok && val != "" {
			buf.WriteString(fmt.Sprintf("%s=\"%s\"\n", name, val))
			exported[name] = true
		}
	}

	// Any additional UI-saved env:* in Ledger
	if s.Ledger != nil {
		if all, err := s.Ledger.AllSettings(); err == nil {
			for k, v := range all {
				if name, ok := strings.CutPrefix(k, "env:"); ok && !exported[name] && v != "" {
					buf.WriteString(fmt.Sprintf("%s=\"%s\"\n", name, v))
					exported[name] = true
				}
			}
		}
	}

	filename := fmt.Sprintf("aicos_env_%s.env", time.Now().Format("20060102_1504"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_, _ = w.Write(buf.Bytes())
}

// handleEnvExportJSON xuất toàn bộ cấu hình dưới dạng JSON (bao gồm tài khoản WebAI).
func (s *Server) handleEnvExportJSON(w http.ResponseWriter, r *http.Request) {
	exp := EnvExportJSON{
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

// handleEnvImport nhập cấu hình từ file tải lên hoặc nội dung dán vào form.
