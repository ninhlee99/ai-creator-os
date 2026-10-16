package web

// Routing table and server-level plumbing for the dashboard: every
// route registration, the panic-recovery middleware, redirect helpers and
// the embedded static CSS handler.

import (
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Routes wires every dashboard route. POST redirects use 303 See Other,
// mirroring the FastAPI RedirectResponse(status_code=303) behavior.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /static/style.css", s.handleStaticCSS)
	mux.HandleFunc("GET /static/app.js", s.handleStaticJS)
	mux.HandleFunc("GET /static/preview.mp3", s.handleStaticPreviewAudio)

	// Đợt S: health endpoint cho watchdog ngoài — không qua onboardGate.
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("POST /automation/switch", s.handleAutoSwitch)
	mux.HandleFunc("GET /accounts", s.handleAccounts)
	mux.HandleFunc("GET /accounts/new", s.handleAccountNew)
	mux.HandleFunc("POST /accounts", s.handleAccountCreate)
	mux.HandleFunc("GET /accounts/{id}", s.handleAccountDetail)
	mux.HandleFunc("POST /accounts/{id}/transition", s.handleAccountTransition)
	mux.HandleFunc("POST /accounts/{id}/youtube", s.handleAccountYoutube)
	mux.HandleFunc("POST /accounts/{id}/onboard", s.handleAccountOnboard)
	mux.HandleFunc("POST /accounts/{id}/replan", s.handleAccountReplan)

	// Affiliate autopilot: theme policy, hands-off toggle, model library.
	mux.HandleFunc("POST /accounts/{id}/theme", s.handleAccountTheme)
	mux.HandleFunc("POST /accounts/{id}/autopilot", s.handleAccountAutopilot)
	mux.HandleFunc("POST /accounts/{id}/autopilot/run", s.handleAccountAutopilotRun)
	mux.HandleFunc("POST /accounts/{id}/models/upload", s.handleAccountModelUpload)
	mux.HandleFunc("POST /accounts/{id}/models/{photoID}/delete", s.handleAccountModelDelete)
	// Brand DNA (Bản sắc thương hiệu cho kênh — Organic growth)
	mux.HandleFunc("POST /accounts/{id}/brand-dna", s.handleAccountBrandDNASave)
	mux.HandleFunc("POST /accounts/{id}/brand-dna/reset", s.handleAccountBrandDNAReset)

	// AI Mission Control
	mux.HandleFunc("POST /missions/{id}/cancel", s.handleMissionCancel)
	mux.HandleFunc("GET /api/missions/{id}", s.handleMissionDetailJSON)

	mux.HandleFunc("GET /media/{name}", s.handleMedia)

	// Studio: AI video creation (affiliate / short film) + VN trends.
	mux.HandleFunc("GET /studio", s.handleStudio)
	mux.HandleFunc("POST /studio/affiliate", s.handleStudioAffiliateCreate)
	mux.HandleFunc("GET /studio/jobs", s.handleStudioJobs)
	mux.HandleFunc("GET /studio/jobs/{id}", s.handleStudioJobDetail)
	mux.HandleFunc("POST /studio/jobs/{id}/cancel", s.handleStudioJobCancel)
	mux.HandleFunc("POST /studio/jobs/{id}/delete", s.handleStudioJobDelete)
	mux.HandleFunc("POST /studio/jobs/{id}/retry", s.handleStudioJobRetry)
	mux.HandleFunc("POST /studio/jobs/{id}/capcut", s.handleStudioJobCapCut)
	mux.HandleFunc("POST /studio/jobs/{id}/remix", s.handleStudioJobRemix)
	mux.HandleFunc("GET /studio/assets/{job}/{file}", s.handleStudioAsset)
	mux.HandleFunc("GET /studio/trends", s.handleStudioTrends)
	mux.HandleFunc("POST /studio/trends/refresh", s.handleStudioTrendsRefresh)
	mux.HandleFunc("GET /studio/mediagen/health", s.handleStudioMediaGenHealth)
	mux.HandleFunc("POST /studio/ad-creative", s.handleStudioAdCreative)
	mux.HandleFunc("GET /studio/uploads/{file}", s.handleStudioUpload)
	mux.HandleFunc("POST /studio/generate-image", s.handleStudioGenerateImage)
	mux.HandleFunc("POST /studio/schedule", s.handleStudioScheduleSave)
	mux.HandleFunc("POST /studio/schedule/run-now", s.handleStudioScheduleRunNow)

	// Web AI Hub (Claude Web, ChatGPT Web, Gemini Web, Muse AI login & workspace)
	mux.HandleFunc("GET /web-ai", s.handleWebAI)
	mux.HandleFunc("POST /web-ai/connect", s.handleWebAIConnect)
	mux.HandleFunc("POST /web-ai/disconnect", s.handleWebAIDisconnect)
	mux.HandleFunc("POST /web-ai/accounts/add", s.handleWebAIAccountAdd)
	mux.HandleFunc("POST /web-ai/accounts/delete", s.handleWebAIAccountDelete)
	mux.HandleFunc("POST /web-ai/accounts/test", s.handleWebAIAccountTest)
	mux.HandleFunc("POST /web-ai/accounts/reset-cooldown", s.handleWebAIAccountResetCooldown)
	mux.HandleFunc("POST /web-ai/prompt-generate", s.handleWebAIPromptGenerate)
	mux.HandleFunc("GET /web-ai/proxy/", s.handleWebAIProxy)


	mux.HandleFunc("GET /publishers", s.handlePublishers)
	mux.HandleFunc("GET /publishers/tiktok/authorize", s.handleTikTokAuthorize)
	mux.HandleFunc("POST /publishers/tiktok/connect", s.handleTikTokConnect)
	mux.HandleFunc("GET /publishers/tiktok/callback", s.handleTikTokCallback)
	// Đợt AA: YouTube OAuth trong app (trước đây chỉ có script Python).
	mux.HandleFunc("GET /publishers/youtube/authorize", s.handleYouTubeAuthorize)
	mux.HandleFunc("GET /publishers/youtube/callback", s.handleYouTubeCallback)

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
	mux.HandleFunc("GET /settings/an-toan", s.handleSettingsAnToan)
	mux.HandleFunc("POST /settings/dryrun", s.handleSettingsDryRun)
	mux.HandleFunc("POST /settings/env", s.handleSettingsEnvSave)
	mux.HandleFunc("GET /settings/env/export", s.handleEnvExport)
	mux.HandleFunc("GET /settings/env/export-json", s.handleEnvExportJSON)
	mux.HandleFunc("POST /settings/env/import", s.handleEnvImport)
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
	mux.HandleFunc("POST /kill", s.handleKill)
	mux.HandleFunc("POST /unkill", s.handleUnkill)

	mux.HandleFunc("GET /settings/accesstrade", s.handleSettingsAccesstrade)
	mux.HandleFunc("POST /settings/accesstrade/key", s.handleATKeySave)
	mux.HandleFunc("POST /settings/accesstrade/test", s.handleATTest)
	mux.HandleFunc("POST /settings/accesstrade/automation", s.handleATAutomationSave)
	// Accesstrade (Đợt B): sync campaign + tạo tracking link.
	mux.HandleFunc("POST /at/campaigns/sync", s.handleATSyncCampaigns)
	mux.HandleFunc("POST /at/links/create", s.handleATCreateLink)
	// Accesstrade (Đợt C): hunter + order sync + công tắc tick.
	mux.HandleFunc("POST /at/hunt", s.handleATHunt)
	mux.HandleFunc("POST /at/orders/sync", s.handleATOrderSync)
	// Đợt F: Kể chuyện YouTube (truyện ngôi thứ nhất + ảnh 16:9 + TTS).
	mux.HandleFunc("GET /stories", s.handleStories)
	mux.HandleFunc("POST /stories", s.handleStoryCreate)
	mux.HandleFunc("POST /stories/{id}/publish", s.handleStoryPublish)
	mux.HandleFunc("POST /stories/{id}/delete", s.handleStoryDelete)
	mux.HandleFunc("POST /stories/{id}/retry", s.handleStoryRetry) // đợt Y: thử lại tay
	mux.HandleFunc("GET /stories/{id}/log", s.handleStoryLog)      // đợt Y: nhật ký đầy đủ
	mux.HandleFunc("POST /stories/settings", s.handleStorySettings)

	// Video Viral AI 100% Gốc (Trendjack Spotlight Shorts 9:16).
	mux.HandleFunc("GET /viral", s.handleViral)
	mux.HandleFunc("POST /viral/create", s.handleViralCreate)
	mux.HandleFunc("POST /viral/quick", s.handleViralQuick)

	// Master Autonomous Harness Agent (OODA loop, experiential memory, portfolio control).
	mux.HandleFunc("GET /harness", s.handleHarness)
	mux.HandleFunc("POST /harness/cycle", s.handleHarnessRunCycle)
	mux.HandleFunc("POST /harness/autonomy", s.handleHarnessAutonomy)
	mux.HandleFunc("POST /harness/memory", s.handleHarnessMemoryAdd)
	// Đợt H2: Agent Team định nghĩa lại quanh 3 pipeline (thay team live đã park).
	mux.HandleFunc("GET /team", s.handleTeamPipelines)
	mux.HandleFunc("POST /team/agents/create", s.handleAgentCreate)
	mux.HandleFunc("POST /team/agents/toggle", s.handleAgentToggleAutopilot)
	mux.HandleFunc("POST /team/agents/delete", s.handleAgentDelete)
	mux.HandleFunc("POST /team/departments/assign", s.handleDeptAssign)
	mux.HandleFunc("POST /team/dispatch", s.handleTeamDispatchGoal)
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
		{"GET /shop", "/products?tab=ke"},
		{"POST /shop/add", "/products?tab=ke"},
	} {
		mux.HandleFunc(legacy.pattern, redirectTo(legacy.target))
	}

	return s.recoverer(s.originGate(s.authGate(s.onboardGate(mux))))
}

// originGate chống CSRF đơn giản mà bao phủ mọi POST (Đợt AA, finding C4):
// với POST/PUT/PATCH/DELETE, nếu request mang header Origin (hoặc Referer)
// mà host không khớp host của app → 403. Không có Origin/Referer (curl,
// gọi nội bộ, healthcheck) thì cho qua như cũ — không cần sửa 24 template.
func (s *Server) originGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !sameOrigin(r) {
				log.Printf("web: từ chối %s %s — origin không khớp (chống CSRF)", r.Method, r.URL.Path)
				http.Error(w, "origin không khớp — từ chối để chống CSRF", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if ref := r.Header.Get("Referer"); ref != "" {
			origin = ref
		} else {
			return true // curl / gọi nội bộ: không có header để kiểm
		}
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, "null") {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	// Đợt AK: sau tunnel/proxy (cloudflared, Tailscale serve) Host có thể bị
	// ghi đè thành 127.0.0.1 — so thêm host gốc proxy chuyển tiếp. Form CSRF
	// từ site lạ không tự đặt được header này.
	fh := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	return fh != "" && strings.EqualFold(u.Host, fh)
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

// setAssetCache: URL có ?v=<hash> khớp bản hiện tại → cache lâu (nội dung
// không bao giờ đổi dưới URL đó); không có/không khớp → bắt trình duyệt hỏi
// lại mỗi lần, tránh giao diện cũ sau khi cập nhật app (Đợt AK).
func setAssetCache(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("v") == assetVersion {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

// handleStaticCSS serves the embedded dashboard stylesheet.
func (s *Server) handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	setAssetCache(w, r)
	_, _ = w.Write(staticCSS)
}

// handleStaticJS serves the embedded shared client script (toast, confirm
// modal, studio polling — R2-W3: một nguồn JS dùng chung toàn app).
func (s *Server) handleStaticJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	setAssetCache(w, r)
	_, _ = w.Write(staticJS)
}

// handleStaticPreviewAudio serves the bundled sample preview music.
func (s *Server) handleStaticPreviewAudio(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(staticPreviewAudio)
}

// --------------------------------------------------------------- dashboard
