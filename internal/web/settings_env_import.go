package web

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/accesstrade"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func (s *Server) handleEnvImport(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Kho dữ liệu chưa sẵn sàng."))
		return
	}

	// Hỗ trợ tối đa 10MB upload
	if err := r.ParseMultipartForm(10 << 20); err != nil && err != http.ErrNotMultipart {
		if err := r.ParseForm(); err != nil {
			seeOther(w, r, "/settings/system?err="+url.QueryEscape("Không đọc được dữ liệu nhập: "+err.Error()))
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
		seeOther(w, r, "/settings/system?err="+url.QueryEscape("Chưa có nội dung hoặc file cấu hình nào được chọn."))
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
	s.SyncGeminiKeys()
	s.applyPersistedEnv()
	_ = s.Ledger.Decide("human", "env_import", nil, fmt.Sprintf("Import .env: %d biến, %d key Gemini, %d Accesstrade", envCount, geminiCount, atCount), nil)

	msg := fmt.Sprintf("✓ Đã nhập thành công: %d biến môi trường, %d key Gemini, %d Accesstrade.", envCount, geminiCount, atCount)
	log.Printf("web: %s", msg)
	seeOther(w, r, "/settings/system?ok="+url.QueryEscape(msg))
}
