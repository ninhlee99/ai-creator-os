package web

import (
	"net/http"
)

// Routes wires every dashboard route. POST redirects use 303 See Other,
// mirroring the FastAPI RedirectResponse(status_code=303) behavior.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /static/style.css", s.handleStaticCSS)
	mux.HandleFunc("GET /static/htmx.min.js", s.handleStaticHTMX)
	mux.HandleFunc("GET /static/app.js", s.handleStaticJS)
	mux.HandleFunc("GET /static/webai-login.js", s.handleStaticWebAILoginJS)
	mux.HandleFunc("GET /static/flow-canvas.js", s.handleStaticFlowCanvasJS)
	mux.HandleFunc("GET /static/preview.mp3", s.handleStaticPreviewAudio)
	mux.HandleFunc("GET /static/presets/{file}", s.handleStaticPresetImage)
	mux.HandleFunc("GET /static/sample_916.mp4", s.handleStaticSampleVideo)

	// Đợt S: health endpoint cho watchdog ngoài — không qua onboardGate.
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	// Trang chủ là màn hình Tạo video AI bằng 1 câu lệnh (Conversation Studio kiểu Gemini).
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.handleVideoCreate(w, r)
	})
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.handleVideoCreateSubmit(w, r)
	})
	mux.HandleFunc("GET /dashboard", s.handleDashboard)
	mux.HandleFunc("POST /automation/switch", s.handleAutoSwitch)
	mux.HandleFunc("GET /accounts", s.handleAccounts)
	mux.HandleFunc("GET /accounts/new", s.handleAccountNew)
	mux.HandleFunc("POST /accounts", s.handleAccountCreate)
	mux.HandleFunc("GET /accounts/{id}", s.handleAccountDetail)
	mux.HandleFunc("GET /accounts/{id}/edit", s.handleAccountEdit)
	mux.HandleFunc("POST /accounts/{id}/edit", s.handleAccountUpdate)
	mux.HandleFunc("POST /accounts/{id}/delete", s.handleAccountDelete)
	mux.HandleFunc("POST /accounts/{id}/transition", s.handleAccountTransition)
	mux.HandleFunc("POST /accounts/{id}/onboard", s.handleAccountOnboard)
	mux.HandleFunc("POST /accounts/{id}/replan", s.handleAccountReplan)

	// Affiliate autopilot: theme policy, hands-off toggle, model library.
	mux.HandleFunc("POST /accounts/{id}/theme", s.handleAccountTheme)
	mux.HandleFunc("POST /accounts/{id}/autopilot", s.handleAccountAutopilot)
	mux.HandleFunc("POST /accounts/{id}/autopilot/run", s.handleAccountAutopilotRun)
	mux.HandleFunc("POST /accounts/{id}/models/upload", s.handleAccountModelUpload)
	mux.HandleFunc("POST /accounts/{id}/models/{photoID}/delete", s.handleAccountModelDelete)
	mux.HandleFunc("GET /models/{account}/{file}", s.handleModelPhoto)

	// Điều hướng Thư Viện Nhân Vật -> Quản lý trực tiếp theo Kênh (/accounts)
	mux.HandleFunc("GET /models", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
	})

	// Đường dẫn tắt chuyên biệt cho Affiliate
	mux.HandleFunc("GET /affiliate", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/studio", http.StatusSeeOther)
	})
	// Brand DNA (Bản sắc thương hiệu cho kênh — Organic growth)
	mux.HandleFunc("POST /accounts/{id}/brand-dna", s.handleAccountBrandDNASave)
	mux.HandleFunc("POST /accounts/{id}/brand-dna/reset", s.handleAccountBrandDNAReset)
	mux.HandleFunc("GET /media/{name}", s.handleMedia)

	// Studio: TikTok affiliate and viral video creation + radar.
	mux.HandleFunc("GET /studio", s.handleStudio)
	mux.HandleFunc("POST /studio/affiliate/plan", s.handleStudioAffiliatePlan)
	mux.HandleFunc("POST /studio/affiliate", s.handleStudioAffiliateCreate)
	mux.HandleFunc("GET /studio/jobs", s.handleStudioJobs)
	mux.HandleFunc("GET /studio/create", redirectTo("/workflow"))
	mux.HandleFunc("POST /studio/generate", s.handleStudioAIGenerate)
	mux.HandleFunc("GET /studio/jobs/{id}", s.handleStudioJobDetail)
	mux.HandleFunc("POST /studio/jobs/{id}/cancel", s.handleStudioJobCancel)
	mux.HandleFunc("POST /studio/jobs/{id}/delete", s.handleStudioJobDelete)
	mux.HandleFunc("POST /studio/jobs/{id}/retry", s.handleStudioJobRetry)
	mux.HandleFunc("POST /studio/jobs/{id}/publish-review", s.handleStudioPublishReview)
	mux.HandleFunc("POST /studio/jobs/{id}/tiktok-draft", s.handleStudioTikTokDraft)
	mux.HandleFunc("POST /studio/jobs/{id}/tiktok-video", s.handleStudioTikTokVideoLink)
	mux.HandleFunc("POST /studio/jobs/{id}/capcut", s.handleStudioJobCapCut)
	mux.HandleFunc("POST /studio/jobs/{id}/remix", s.handleStudioJobRemix)
	mux.HandleFunc("GET /studio/assets/{job}/{file}", s.handleStudioAsset)
	mux.HandleFunc("GET /studio/trends", s.handleStudioTrends)
	mux.HandleFunc("POST /studio/trends/refresh", s.handleStudioTrendsRefresh)
	mux.HandleFunc("POST /studio/trends/topics", s.handleStudioTrendsAddTopic)
	mux.HandleFunc("POST /studio/trends/topics/delete", s.handleStudioTrendsDeleteTopic)
	mux.HandleFunc("POST /studio/trends/{videoID}/dance", s.handleStudioTrendDanceVerify)
	mux.HandleFunc("GET /studio/mediagen/health", s.handleStudioMediaGenHealth)
	mux.HandleFunc("GET /studio/uploads/{file}", s.handleStudioUpload)
	mux.HandleFunc("GET /studio/models/{account}/{file}", s.handleStudioModelPhoto)
	mux.HandleFunc("POST /studio/generate-image", s.handleStudioGenerateImage)
	mux.HandleFunc("POST /studio/schedule", s.handleStudioScheduleSave)
	mux.HandleFunc("POST /studio/schedule/run-now", s.handleStudioScheduleRunNow)

	// Visual Workflow Studio (ComfyUI / n8n Node Canvas)
	mux.HandleFunc("GET /workflow", s.handleStudioFlowView)
	mux.HandleFunc("GET /api/flow/definitions", s.handleAPIFlowDefinitions)
	mux.HandleFunc("GET /api/flow/products", s.handleAPIFlowProducts)
	mux.HandleFunc("GET /api/flow/graphs", s.handleAPIFlowGraphs)
	mux.HandleFunc("POST /api/flow/graphs", s.handleAPIFlowGraphs)
	mux.HandleFunc("GET /api/flow/graphs/{id}", s.handleAPIFlowGraphDetail)
	mux.HandleFunc("DELETE /api/flow/graphs/{id}", s.handleAPIFlowGraphDetail)
	mux.HandleFunc("POST /api/flow/execute", s.handleAPIFlowExecute)
	mux.HandleFunc("POST /api/flow/api-run", s.handleAPIFlowRunAPI)
	mux.HandleFunc("POST /api/flow/webhook", s.handleAPIFlowRunAPI)
	mux.HandleFunc("POST /api/flow/webhook/{id}", s.handleAPIFlowRunAPI)
	mux.HandleFunc("POST /api/flow/upload", s.handleAPIFlowUpload)
	mux.HandleFunc("POST /api/flow/fetch-url", s.handleAPIFlowFetchURL)
	mux.HandleFunc("GET /api/flow/runs", s.handleAPIFlowRuns)
	mux.HandleFunc("GET /api/flow/runs/{id}", s.handleAPIFlowRunDetail)
	mux.HandleFunc("POST /api/flow/runs/{id}/cancel", s.handleAPIFlowRunCancel)
	mux.HandleFunc("GET /api/custom-models", s.handleListCustomAPIModels)
	mux.HandleFunc("POST /api/custom-models", s.handleSaveCustomAPIModel)
	mux.HandleFunc("POST /api/custom-models/delete", s.handleDeleteCustomAPIModel)

	// Giám sát tài nguyên hệ thống (RAM, CPU, Workflow Nodes, Tiến trình nặng)
	mux.HandleFunc("GET /api/system/metrics", s.handleSystemMetrics)
	mux.HandleFunc("GET /api/system/sidebar-metrics", s.handleSystemSidebarMetrics)
	mux.HandleFunc("POST /api/system/kill-process", s.handleKillProcess)

	// Legacy Web AI prompt cockpit now redirects to account login settings.
	mux.HandleFunc("GET /web-ai", redirectTo("/settings/system#tab-webai"))
	mux.HandleFunc("POST /web-ai/accounts/delete", s.handleWebAIAccountDelete)
	mux.HandleFunc("POST /web-ai/accounts/test", s.handleWebAIAccountTest)
	mux.HandleFunc("POST /web-ai/accounts/reset-cooldown", s.handleWebAIAccountResetCooldown)
	mux.Handle("POST /web-ai/browser-login/start", s.browserLoginHTTPSGate(http.HandlerFunc(s.handleWebAIBrowserLoginStart)))
	mux.Handle("GET /web-ai/browser-login/status", s.browserLoginHTTPSGate(http.HandlerFunc(s.handleWebAIBrowserLoginStatus)))
	mux.Handle("GET /web-ai/browser-login/screen", s.browserLoginHTTPSGate(http.HandlerFunc(s.handleWebAIBrowserLoginScreen)))
	mux.Handle("POST /web-ai/browser-login/input", s.browserLoginHTTPSGate(http.HandlerFunc(s.handleWebAIBrowserLoginInput)))
	mux.Handle("POST /web-ai/browser-login/cancel", s.browserLoginHTTPSGate(http.HandlerFunc(s.handleWebAIBrowserLoginCancel)))

	mux.HandleFunc("GET /publishers", s.handlePublishers)
	mux.HandleFunc("GET /publishers/tiktok/authorize", s.handleTikTokAuthorize)
	mux.HandleFunc("GET /publishers/tiktok/callback", s.handleTikTokCallback)

	// Affiliate product discovery (theme search -> save into product store).
	// Tab "Kệ hàng" (gộp từ trang /shop cũ) nằm trong cùng trang này.
	mux.HandleFunc("GET /products", s.handleProducts)
	mux.HandleFunc("POST /products/import-link", s.handleProductsImportLink)
	mux.HandleFunc("POST /products/create-video", s.handleProductsCreateVideo)
	mux.HandleFunc("POST /products/shelf/add", s.handleProductsShelfAdd)
	mux.HandleFunc("POST /products/shelf/toggle", s.handleProductsShelfToggle)

	mux.HandleFunc("GET /growth", s.handleGrowth)
	mux.HandleFunc("POST /growth/sync", s.handleGrowthSync)
	mux.HandleFunc("POST /growth/accounts/{id}/plan", s.handleGrowthPlan)
	mux.HandleFunc("POST /growth/production", s.handleGrowthProductionToggle)
	mux.HandleFunc("POST /growth/production/run", s.handleGrowthProductionRun)
	mux.HandleFunc("POST /growth/thresholds", s.handleGrowthThresholds)
	mux.HandleFunc("POST /growth/tuning/rollback", s.handleGrowthTuningRollback)

	// Cài đặt tách 5 trang con (R2-W3, theo UI_UX_BLUEPRINT §2): mỗi trang
	// trả lời đúng một câu hỏi. /settings giữ làm lối vào → redirect.
	mux.HandleFunc("GET /settings", s.handleSettingsIndex)
	mux.HandleFunc("GET /settings/system", s.handleSettingsHeThong)
	mux.HandleFunc("GET /settings/providers", s.handleSettingsNhaCungCap)
	mux.HandleFunc("GET /settings/safety", s.handleSettingsAnToan)
	mux.HandleFunc("POST /settings/dryrun", s.handleSettingsDryRun)
	mux.HandleFunc("POST /settings/models/update", s.handleModelsUpdate)
	mux.HandleFunc("POST /settings/env", s.handleSettingsEnvSave)
	mux.HandleFunc("POST /settings/master", s.handleSettingsMaster)
	mux.HandleFunc("POST /settings/api-budget", s.handleSettingsAPIBudget)
	mux.HandleFunc("POST /settings/api", s.handleSettingsAPIToggle)
	mux.HandleFunc("POST /settings/backup", s.handleBackupCreate)
	mux.HandleFunc("POST /settings/backup/restore", s.handleBackupRestore)
	mux.HandleFunc("POST /settings/backup/offsite", s.handleBackupOffsite)
	// Đợt O2: cấu hình + gửi thử thông báo Telegram / Discord.
	mux.HandleFunc("POST /settings/notify", s.handleNotifySave)
	mux.HandleFunc("POST /settings/notify-test", s.handleNotifyTest)
	mux.HandleFunc("POST /settings/notify-discord-test", s.handleNotifyDiscordTest)
	mux.HandleFunc("POST /settings/tiktok/cookie/save", s.handleTikTokCookieSave)
	mux.HandleFunc("POST /settings/tiktok/cookie/delete", s.handleTikTokCookieDelete)
	mux.HandleFunc("POST /kill", s.handleKill)
	mux.HandleFunc("POST /unkill", s.handleUnkill)

	// Universal Video Creation Studio (vẫn hỗ trợ alias /workflow/create)
	mux.HandleFunc("GET /workflow/create", s.handleVideoCreate)
	mux.HandleFunc("POST /workflow/create", s.handleVideoCreateSubmit)
	// Điều hướng tương thích route cũ /viral sang /workflow/create
	mux.HandleFunc("GET /viral", func(w http.ResponseWriter, r *http.Request) {
		target := "/workflow/create"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
	mux.HandleFunc("POST /viral/create", s.handleVideoCreateSubmit)

	// provider chain configuration (new requirement)
	mux.HandleFunc("GET /settings/chain", s.handleChainGet)
	mux.HandleFunc("POST /settings/chain", s.handleChainSave)
	mux.HandleFunc("POST /settings/chain/move", s.handleChainMove)
	mux.HandleFunc("POST /settings/chain/health", s.handleChainHealth)
	mux.HandleFunc("POST /settings/gemini-free-tier", s.handleGeminiFreeTierSave)

	// per-key keyring management (add / delete / status / test), all JSON
	// except the redirect-free form posts; keys are never rendered raw.
	mux.HandleFunc("POST /settings/keys/add", s.handleKeyAdd)
	mux.HandleFunc("POST /settings/keys/delete", s.handleKeyDelete)
	mux.HandleFunc("GET /settings/keys/status", s.handleKeyStatus)
	mux.HandleFunc("POST /settings/keys/test", s.handleKeyTest)

	// legacy JSON API — mặc định TẮT (R2-W7, R2-09): không có UI nào gọi,
	// mở ra chỉ khi người vận hành bật công tắc ở Cài đặt · Hệ thống.
	mux.HandleFunc("GET /api/stats", s.requireAPI(s.handleAPIStats))
	mux.HandleFunc("GET /api/products", s.requireAPI(s.handleAPIProducts))
	mux.HandleFunc("GET /api/decisions", s.requireAPI(s.handleAPIDecisions))

	// First-run wizard (R2-W7): hiện khi thư mục dữ liệu chưa có ledger.db.
	mux.HandleFunc("GET /onboard", s.handleOnboard)
	mux.HandleFunc("POST /onboard", s.handleOnboard)
	// Đợt T: đăng nhập/đăng xuất dashboard (authGate cho /login đi qua).
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)
	// Đợt T: đặt/tắt mật khẩu dashboard.
	mux.HandleFunc("POST /settings/auth/password", s.handleAuthPasswordSet)
	mux.HandleFunc("POST /settings/auth/password/disable", s.handleAuthPasswordDisable)

	// Đợt 2 (gộp trang): URL cũ của các trang đã gộp redirect 303 sang tab
	// thay thế, để bookmark/form cũ vẫn tới đúng chỗ. Trang /analytics đã
	// tan hẳn (doanh thu ở Trang chủ, chi phí API ở Cài đặt, số liệu video
	// ở /growth) nên không giữ redirect.
	for _, legacy := range []struct{ pattern, target string }{
		{"GET /content", "/studio"},
		{"POST /content", "/studio"},
		{"GET /shop", "/products?tab=shelf"},
		{"POST /shop/add", "/products?tab=shelf"},
	} {
		mux.HandleFunc(legacy.pattern, redirectTo(legacy.target))
	}

	return s.railsRequestLogger(s.recoverer(s.originGate(s.authGate(s.onboardGate(mux)))))
}

// originGate chống CSRF đơn giản mà bao phủ mọi POST (Đợt AA, finding C4):
// với POST/PUT/PATCH/DELETE, nếu request mang header Origin (hoặc Referer)
// mà host không khớp host của app → 403. Không có Origin/Referer (curl,
// gọi nội bộ, healthcheck) thì cho qua như cũ — không cần sửa 24 template.
