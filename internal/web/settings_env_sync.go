package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

// EnvExportJSON định nghĩa cấu trúc sao lưu cấu hình JSON.
type EnvExportJSON struct {
	Version        string              `json:"version"`
	ExportedAt     string              `json:"exported_at"`
	GeminiAPIKeys  []string            `json:"gemini_api_keys"`
	AccesstradeKey string              `json:"accesstrade_key,omitempty"`
	TelegramBot    string              `json:"telegram_bot,omitempty"`
	TelegramChat   string              `json:"telegram_chat,omitempty"`
	WebhookURL     string              `json:"webhook_url,omitempty"`
	EnvVars        map[string]string   `json:"env_vars"`
	WebAIAccounts  []webai.Account     `json:"web_ai_accounts,omitempty"`
}

// handleEnvExport xuất toàn bộ cấu hình ra file định dạng standard .env (aicos.env).
func (s *Server) handleEnvExport(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	buf.WriteString("# ==============================================================================\n")
	buf.WriteString("# AI Creator OS — Environment Configuration Backup\n")
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
func (s *Server) handleEnvImport(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/settings/he-thong?err="+url.QueryEscape("Kho dữ liệu chưa sẵn sàng."))
		return
	}

	// Hỗ trợ tối đa 10MB upload
	if err := r.ParseMultipartForm(10 << 20); err != nil && err != http.ErrNotMultipart {
		if err := r.ParseForm(); err != nil {
			seeOther(w, r, "/settings/he-thong?err="+url.QueryEscape("Không đọc được dữ liệu nhập: "+err.Error()))
			return
		}
	}

	var rawBytes []byte
	file, _, err := r.FormFile("env_file")
	if err == nil && file != nil {
		defer file.Close()
		rawBytes, _ = io.ReadAll(file)
	} else {
		rawBytes = []byte(r.FormValue("env_content"))
	}

	rawBytes = bytes.TrimSpace(rawBytes)
	if len(rawBytes) == 0 {
		seeOther(w, r, "/settings/he-thong?err="+url.QueryEscape("Chưa có nội dung hoặc file cấu hình nào được chọn."))
		return
	}

	// Kiểm tra nếu là JSON
	if rawBytes[0] == '{' {
		s.importFromJSON(w, r, rawBytes)
		return
	}

	// Mặc định: Phân tích cú pháp dòng .env
	s.importFromEnv(w, r, rawBytes)
}

func (s *Server) importFromEnv(w http.ResponseWriter, r *http.Request, raw []byte) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	envCount := 0
	geminiCount := 0
	atCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		// Bỏ ngoặc kép hoặc ngoặc đơn nếu có
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}

		if key == "" || val == "" {
			continue
		}

		switch key {
		case "GEMINI_API_KEYS", "GEMINI_API_KEY":
			keys := strings.Split(val, ",")
			for _, k := range keys {
				if s.addGeminiKey(k) {
					geminiCount++
				}
			}
		case "ACCESSTRADE_API_KEY", "ACCESSTRADE_KEY":
			_ = s.Ledger.SetSetting(accesstrade.KeySetting, val)
			atCount++
		case "NOTIFY_TELEGRAM_BOT":
			_ = s.Ledger.SetSetting(automation.KeyNotifyTelegramBot, val)
			envCount++
		case "NOTIFY_TELEGRAM_CHAT":
			_ = s.Ledger.SetSetting(automation.KeyNotifyTelegramChat, val)
			envCount++
		case "NOTIFY_WEBHOOK_URL":
			_ = s.Ledger.SetSetting(automation.KeyNotifyWebhookURL, val)
			envCount++
		case "NOTIFY_TELEGRAM_ON":
			_ = s.Ledger.SetSetting(automation.KeyNotifyTelegramOn, val)
			envCount++
		default:
			os.Setenv(key, val)
			if err := s.Ledger.SetSetting(envSettingKey(key), val); err == nil {
				envCount++
			}
		}
	}

	s.Cfg.RefreshEnv()
	s.applyPersistedEnv()
	_ = s.Ledger.Decide("human", "env_import", nil, fmt.Sprintf("Import .env: %d biến, %d key Gemini, %d Accesstrade", envCount, geminiCount, atCount), nil)

	msg := fmt.Sprintf("✓ Đã nhập thành công: %d biến môi trường, %d key Gemini, %d Accesstrade.", envCount, geminiCount, atCount)
	log.Printf("web: %s", msg)
	seeOther(w, r, "/settings/he-thong?ok="+url.QueryEscape(msg))
}

func (s *Server) importFromJSON(w http.ResponseWriter, r *http.Request, raw []byte) {
	var exp EnvExportJSON
	if err := json.Unmarshal(raw, &exp); err != nil {
		seeOther(w, r, "/settings/he-thong?err="+url.QueryEscape("File JSON không hợp lệ: "+err.Error()))
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
			if _, err := s.WebAI.AddAccount(acc); err == nil {
				webaiCount++
			}
		}
	}

	s.Cfg.RefreshEnv()
	s.applyPersistedEnv()
	_ = s.Ledger.Decide("human", "env_import_json", nil, fmt.Sprintf("Import JSON: %d biến, %d key Gemini, %d WebAI", envCount, geminiCount, webaiCount), nil)

	msg := fmt.Sprintf("✓ Đã nhập thành công từ JSON: %d biến môi trường, %d key Gemini, %d tài khoản WebAI.", envCount, geminiCount, webaiCount)
	log.Printf("web: %s", msg)
	seeOther(w, r, "/settings/he-thong?ok="+url.QueryEscape(msg))
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
