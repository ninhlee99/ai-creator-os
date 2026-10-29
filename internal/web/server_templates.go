package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/sysmon"
	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure Go, no cgo)
)

//go:embed templates/*.html templates/*/*.html
var templateFS embed.FS

//go:embed static/tailwind.css
var staticCSS []byte

//go:embed static/htmx.min.js
var staticHTMX []byte

//go:embed static/app.js
var staticJS []byte

//go:embed static/webai-login.js
var staticWebAILoginJS []byte

//go:embed static/flow-canvas.js
var staticFlowCanvasJS []byte

//go:embed static/preview.mp3
var staticPreviewAudio []byte

// assetVersion là hash nội dung CSS+JS nhúng (Đợt AK): URL asset đổi mỗi khi
// nội dung đổi, nên cập nhật app là trình duyệt lấy giao diện mới ngay —
// trước đây cache 1 giờ khiến người dùng vẫn thấy giao diện cũ.
var assetVersion = func() string {
	assets := append([]byte{}, staticCSS...)
	assets = append(assets, staticJS...)
	assets = append(assets, staticHTMX...)
	assets = append(assets, staticWebAILoginJS...)
	assets = append(assets, staticFlowCanvasJS...)
	h := sha256.Sum256(assets)
	return hex.EncodeToString(h[:])[:10]
}()

// pageFiles maps a page key to the template file rendered inside base.
var pageFiles = map[string]string{
	"dashboard":            "dashboard.html",
	"accounts":             "accounts.html",
	"account_new":          "account_new.html",
	"account_edit":         "account_edit.html",
	"account_detail":       "account_detail.html",
	"studio":               "studio.html",
	"studio_jobs":          "studio_jobs.html",
	"studio_trends":        "studio_trends.html",
	"studio_flow":          "studio_flow.html",
	"publishers":           "publishers.html",
	"products":             "products.html",
	"video_create":         "video_create.html",
	"viral":                "video_create.html",
	"growth":               "growth.html",
	"settings_system":      "settings/system.html",
	"settings_providers":   "settings/providers.html",
	"settings_safety":      "settings/safety.html",
	"settings_accesstrade": "settings/accesstrade.html",
	"onboard":              "onboard.html",
}

func (s *Server) parseTemplates() error {
	s.templates = make(map[string]*template.Template, len(pageFiles))
	for key, file := range pageFiles {
		files := []string{
			"templates/base.html",
			"templates/partials/shared.html",
			"templates/" + file,
		}
		if key == "settings_system" {
			files = append(files,
				"templates/settings/system_webai.html",
				"templates/settings/system_apikeys.html",
				"templates/settings/system_social.html",
				"templates/settings/system_operations.html",
			)
		}
		t, err := template.New("base").Funcs(templateFuncs).ParseFS(
			templateFS, files...)
		if err != nil {
			return fmt.Errorf("parse template %s: %w", file, err)
		}
		s.templates[key] = t
	}
	// Đợt T: trang đăng nhập standalone (không sidebar) cho authGate.
	t, err := template.New("login").Funcs(templateFuncs).ParseFS(
		templateFS, "templates/login.html")
	if err != nil {
		return fmt.Errorf("parse template login.html: %w", err)
	}
	s.templates["login"] = t

	return nil
}

// ctx builds the template context: base flags (Kill/DryRun) plus page data.
func (s *Server) ctx(kv ...any) map[string]any {
	m := map[string]any{
		"Kill":     s.Cfg.KillSwitch(),
		"DryRun":   s.Cfg.DryRun(),
		"MasterOn": automation.MasterOn(s.settings()),
		"AuthOn":   s.authEnabled(),
		"Metrics":  sysmon.CollectFast(),
	}
	if s.WebAI != nil {
		m["WebAICount"] = s.WebAI.CountConnected()
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	return m
}

// render executes a page inside the base layout. A template/handler error
// never crashes the server: it is logged and answered with a 500.
func (s *Server) render(w http.ResponseWriter, page string, data map[string]any) {
	t, ok := s.templates[page]
	if !ok {
		http.Error(w, "unknown page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("web: render %s: %v", page, err)
	}
}

// --------------------------------------------------------------- view types
