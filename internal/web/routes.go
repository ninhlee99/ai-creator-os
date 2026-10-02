package web

// Routing table and server-level plumbing for the dashboard: every
// route registration, the panic-recovery middleware, redirect helpers and
// the embedded static CSS handler.

import (
	"log"
	"net/http"
)

// Routes wires every dashboard route. POST redirects use 303 See Other,
// mirroring the FastAPI RedirectResponse(status_code=303) behavior.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /static/style.css", s.handleStaticCSS)
	mux.HandleFunc("GET /static/app.js", s.handleStaticJS)
	mux.HandleFunc("GET /static/agents/{file}", s.handleAgentAvatar)
	mux.HandleFunc("GET /team", s.handleTeam)

	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("GET /accounts", s.handleAccounts)
	mux.HandleFunc("GET /accounts/new", s.handleAccountNew)
	mux.HandleFunc("POST /accounts", s.handleAccountCreate)
	mux.HandleFunc("GET /accounts/{id}", s.handleAccountDetail)
	mux.HandleFunc("POST /accounts/{id}/transition", s.handleAccountTransition)
	mux.HandleFunc("POST /accounts/{id}/youtube", s.handleAccountYoutube)
	mux.HandleFunc("POST /accounts/{id}/onboard", s.handleAccountOnboard)
	mux.HandleFunc("POST /accounts/{id}/replan", s.handleAccountReplan)
	mux.HandleFunc("POST /accounts/{id}/live-topic", s.handleAccountLiveTopic)

	// Affiliate autopilot: theme policy, hands-off toggle, model library.
	mux.HandleFunc("POST /accounts/{id}/theme", s.handleAccountTheme)
	mux.HandleFunc("POST /accounts/{id}/autopilot", s.handleAccountAutopilot)
	mux.HandleFunc("POST /accounts/{id}/autopilot/run", s.handleAccountAutopilotRun)
	mux.HandleFunc("POST /accounts/{id}/models/upload", s.handleAccountModelUpload)
	mux.HandleFunc("POST /accounts/{id}/models/{photoID}/delete", s.handleAccountModelDelete)
	mux.HandleFunc("GET /models/{account}/{file}", s.handleModelPhoto)

	mux.HandleFunc("GET /schedule", s.handleSchedule)
	mux.HandleFunc("POST /schedule/build", s.handleScheduleBuild)

	// Video chữ động (kinetic) — tab trong Studio (gộp từ trang /content cũ).
	mux.HandleFunc("POST /studio/kinetic", s.handleStudioKineticCreate)
	mux.HandleFunc("GET /media/{name}", s.handleMedia)

	// Studio: AI video creation (affiliate / short film) + VN trends.
	mux.HandleFunc("GET /studio", s.handleStudio)
	mux.HandleFunc("POST /studio/affiliate", s.handleStudioAffiliateCreate)
	mux.HandleFunc("POST /studio/film", s.handleStudioFilmCreate)
	mux.HandleFunc("GET /studio/jobs", s.handleStudioJobs)
	mux.HandleFunc("GET /studio/jobs/{id}", s.handleStudioJobDetail)
	mux.HandleFunc("GET /studio/assets/{job}/{file}", s.handleStudioAsset)
	mux.HandleFunc("GET /studio/trends", s.handleStudioTrends)
	mux.HandleFunc("POST /studio/trends/refresh", s.handleStudioTrendsRefresh)
	mux.HandleFunc("GET /studio/mediagen/health", s.handleStudioMediaGenHealth)

	mux.HandleFunc("GET /publishers", s.handlePublishers)
	mux.HandleFunc("GET /publishers/tiktok/authorize", s.handleTikTokAuthorize)
	mux.HandleFunc("POST /publishers/tiktok/connect", s.handleTikTokConnect)
	mux.HandleFunc("GET /publishers/tiktok/callback", s.handleTikTokCallback)

	// Affiliate product discovery (theme search -> save into product store).
	// Tab "Kệ hàng" (gộp từ trang /shop cũ) nằm trong cùng trang này.
	mux.HandleFunc("GET /products", s.handleProducts)
	mux.HandleFunc("POST /products/search", s.handleProductsSearch)
	mux.HandleFunc("POST /products/add", s.handleProductsAdd)
	mux.HandleFunc("POST /products/shelf/add", s.handleProductsShelfAdd)
	mux.HandleFunc("POST /products/schedule", s.handleProductsSchedule)
	mux.HandleFunc("POST /products/music", s.handleAutopilotMusicUpload)

	mux.HandleFunc("GET /growth", s.handleGrowth)
	mux.HandleFunc("POST /growth/sync", s.handleGrowthSync)
	mux.HandleFunc("POST /growth/accounts/{id}/plan", s.handleGrowthPlan)
	mux.HandleFunc("POST /growth/production", s.handleGrowthProductionToggle)
	mux.HandleFunc("POST /growth/production/run", s.handleGrowthProductionRun)
	mux.HandleFunc("POST /growth/thresholds", s.handleGrowthThresholds)

	// Cài đặt tách 5 trang con (R2-W3, theo UI_UX_BLUEPRINT §2): mỗi trang
	// trả lời đúng một câu hỏi. /settings giữ làm lối vào → redirect.
	mux.HandleFunc("GET /settings", s.handleSettingsIndex)
	mux.HandleFunc("GET /settings/he-thong", s.handleSettingsHeThong)
	mux.HandleFunc("GET /settings/nha-cung-cap", s.handleSettingsNhaCungCap)
	mux.HandleFunc("GET /settings/model-local", s.handleSettingsModelLocal)
	mux.HandleFunc("GET /settings/nhan-vat", s.handleSettingsNhanVat)
	mux.HandleFunc("GET /settings/an-toan", s.handleSettingsAnToan)
	mux.HandleFunc("POST /settings/dryrun", s.handleSettingsDryRun)
	mux.HandleFunc("POST /settings/env", s.handleSettingsEnvSave)
	mux.HandleFunc("POST /settings/master", s.handleSettingsMaster)
	mux.HandleFunc("POST /settings/api-budget", s.handleSettingsAPIBudget)
	mux.HandleFunc("POST /kill", s.handleKill)
	mux.HandleFunc("POST /unkill", s.handleUnkill)

	// provider chain configuration (new requirement)
	mux.HandleFunc("GET /settings/chain", s.handleChainGet)
	mux.HandleFunc("POST /settings/chain", s.handleChainSave)
	mux.HandleFunc("POST /settings/chain/move", s.handleChainMove)
	mux.HandleFunc("POST /settings/chain/health", s.handleChainHealth)

	// per-key keyring management (add / delete / status / test), all JSON
	// except the redirect-free form posts; keys are never rendered raw.
	mux.HandleFunc("POST /settings/keys/add", s.handleKeyAdd)
	mux.HandleFunc("POST /settings/keys/delete", s.handleKeyDelete)
	mux.HandleFunc("GET /settings/keys/status", s.handleKeyStatus)
	mux.HandleFunc("POST /settings/keys/test", s.handleKeyTest)

	// VieNeu sidecar panel
	mux.HandleFunc("GET /settings/vieneu/status", s.handleVieneuStatus)
	mux.HandleFunc("POST /settings/vieneu/ensure", s.handleVieneuEnsure)
	mux.HandleFunc("GET /settings/vieneu/progress", s.handleVieneuProgress)
	mux.HandleFunc("POST /settings/vieneu/restart", s.handleVieneuRestart)
	mux.HandleFunc("POST /settings/vieneu/voice", s.handleVieneuVoice)

	// Avatar: characters, render test, sidecar
	mux.HandleFunc("GET /avatars/{file}", s.handleAvatarImage)
	mux.HandleFunc("GET /settings/avatar/characters", s.handleAvatarCharacters)
	mux.HandleFunc("POST /settings/avatar/characters", s.handleAvatarCharacterCreate)
	mux.HandleFunc("POST /settings/avatar/characters/{id}/delete", s.handleAvatarCharacterDelete)
	mux.HandleFunc("POST /settings/avatar/render-test", s.handleAvatarRenderTest)
	mux.HandleFunc("GET /settings/avatar-sidecar/status", s.handleAvatarSidecarStatus)
	mux.HandleFunc("POST /settings/avatar-sidecar/ensure", s.handleAvatarSidecarEnsure)
	mux.HandleFunc("GET /settings/avatar-sidecar/progress", s.handleAvatarSidecarProgress)
	mux.HandleFunc("POST /settings/avatar-sidecar/restart", s.handleAvatarSidecarRestart)

	// legacy JSON API
	mux.HandleFunc("GET /api/stats", s.handleAPIStats)
	mux.HandleFunc("GET /api/products", s.handleAPIProducts)
	mux.HandleFunc("GET /api/decisions", s.handleAPIDecisions)

	// Đợt 2 (gộp trang): URL cũ của các trang đã gộp redirect 303 sang tab
	// thay thế, để bookmark/form cũ vẫn tới đúng chỗ. Trang /analytics đã
	// tan hẳn (doanh thu ở Trang chủ, chi phí API ở Cài đặt, số liệu video
	// ở /growth) nên không giữ redirect.
	for _, legacy := range []struct{ pattern, target string }{
		{"GET /content", "/studio?tab=chu"},
		{"POST /content", "/studio?tab=chu"},
		{"GET /shop", "/products?tab=ke"},
		{"POST /shop/add", "/products?tab=ke"},
	} {
		mux.HandleFunc(legacy.pattern, redirectTo(legacy.target))
	}

	return s.recoverer(mux)
}

// recoverer keeps a panicking handler from taking the server down.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("web: panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// redirectTo returns a handler that 303-redirects every request to loc.
// Used for the legacy page URLs replaced by tabs in the Đợt 2 merge.
func redirectTo(loc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seeOther(w, r, loc)
	}
}

// ------------------------------------------------------------------ static

// handleStaticCSS serves the embedded dashboard stylesheet.
func (s *Server) handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(staticCSS)
}

// handleStaticJS serves the embedded shared client script (toast, confirm
// modal, studio polling — R2-W3: một nguồn JS dùng chung toàn app).
func (s *Server) handleStaticJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(staticJS)
}

// --------------------------------------------------------------- dashboard
