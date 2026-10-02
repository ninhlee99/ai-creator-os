package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

var mediaNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.mp4$`)

// Routes wires every dashboard route. POST redirects use 303 See Other,
// mirroring the FastAPI RedirectResponse(status_code=303) behavior.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /static/style.css", s.handleStaticCSS)
	mux.HandleFunc("GET /static/agents/{file}", s.handleAgentAvatar)
	mux.HandleFunc("GET /team", s.handleTeam)

	mux.HandleFunc("GET /", s.handleDashboard)
	mux.HandleFunc("GET /accounts", s.handleAccounts)
	mux.HandleFunc("GET /accounts/new", s.handleAccountNew)
	mux.HandleFunc("POST /accounts", s.handleAccountCreate)
	mux.HandleFunc("GET /accounts/{id}", s.handleAccountDetail)
	mux.HandleFunc("POST /accounts/{id}/transition", s.handleAccountTransition)
	mux.HandleFunc("POST /accounts/{id}/followers", s.handleAccountFollowers)
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

	mux.HandleFunc("GET /content", s.handleContent)
	mux.HandleFunc("POST /content", s.handleContentCreate)
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

	mux.HandleFunc("GET /shop", s.handleShop)
	mux.HandleFunc("POST /shop/add", s.handleShopAdd)

	// Affiliate product discovery (theme search -> save into product store).
	mux.HandleFunc("GET /products", s.handleProducts)
	mux.HandleFunc("POST /products/search", s.handleProductsSearch)
	mux.HandleFunc("POST /products/add", s.handleProductsAdd)
	mux.HandleFunc("POST /products/schedule", s.handleProductsSchedule)
	mux.HandleFunc("POST /products/music", s.handleAutopilotMusicUpload)

	mux.HandleFunc("GET /analytics", s.handleAnalytics)
	mux.HandleFunc("GET /growth", s.handleGrowth)
	mux.HandleFunc("POST /growth/sync", s.handleGrowthSync)
	mux.HandleFunc("POST /growth/accounts/{id}/plan", s.handleGrowthPlan)

	mux.HandleFunc("GET /settings", s.handleSettings)
	mux.HandleFunc("POST /settings/dryrun", s.handleSettingsDryRun)
	mux.HandleFunc("POST /settings/env", s.handleSettingsEnvSave)
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

func seeOther(w http.ResponseWriter, r *http.Request, loc string) {
	http.Redirect(w, r, loc, http.StatusSeeOther)
}

func (s *Server) fail(w http.ResponseWriter, err error, what string) {
	log.Printf("web: %s: %v", what, err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// ------------------------------------------------------------------ static

func (s *Server) handleStaticCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(staticCSS)
}

// --------------------------------------------------------------- dashboard

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	eligible := 0
	for _, a := range accounts {
		if a.Status == "live_ready" || a.Status == "live" {
			eligible++
		}
	}
	revenue := s.scalarFloat("SELECT COALESCE(SUM(commission),0) FROM orders")
	today := s.today()
	slots, err := s.Ledger.GetSlots(today)
	if err != nil {
		s.fail(w, err, "get slots")
		return
	}
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions ORDER BY id DESC LIMIT 8")
	if err != nil {
		s.fail(w, err, "recent decisions")
		return
	}
	s.render(w, "dashboard", s.ctx(
		"Accounts", accounts,
		"EligibleCount", eligible,
		"ByStatus", sortedStatusCounts(accounts),
		"Revenue", revenue,
		"Slots", s.slotViews(slots),
		"Decisions", decisions,
		"Today", today,
		"NJobs", s.Jobs.Count(),
		// Bắt đầu nhanh checklist state (trang chủ khi chưa có tài khoản).
		"HasKeys", s.hasAnyAPIKey(),
		"HasStudioJobs", s.Studio != nil && len(s.Studio.ListJobs(1)) > 0,
	))
}

// hasAnyAPIKey reports whether any Gemini key is configured (keyring in the
// database or the GEMINI_API_KEYS environment variable).
func (s *Server) hasAnyAPIKey() bool {
	if len(s.keyRingStatuses("tts", "gemini")) > 0 ||
		len(s.keyRingStatuses("llm", "gemini")) > 0 {
		return true
	}
	return getenv("GEMINI_API_KEYS", "") != "" || getenv("TTS_API_KEY", "") != ""
}

// ---------------------------------------------------------------- accounts

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.Mgr.ListWithAutopilot()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	views := make([]*accountView, 0, len(accounts))
	for _, a := range accounts {
		views = append(views, s.wrapAccount(a))
	}
	s.render(w, "accounts", s.ctx(
		"Accounts", views,
		"FollowersNeed", network.FollowersToLive,
	))
}

func (s *Server) handleAccountNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "account_new", s.ctx())
}

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse account form")
		return
	}
	username := strings.TrimSpace(r.PostFormValue("username"))
	username = strings.TrimLeft(username, "@")
	if username == "" {
		seeOther(w, r, "/accounts/new")
		return
	}
	acct, err := s.Mgr.Add(username,
		strings.TrimSpace(r.PostFormValue("niche_hint")),
		strings.TrimSpace(r.PostFormValue("rtmp_key_ref")))
	if err != nil {
		s.fail(w, err, "add account")
		return
	}
	if ch := strings.TrimSpace(r.PostFormValue("youtube_channel")); ch != "" {
		if err := s.Ledger.SetAccountStatus(acct.ID, acct.Status,
			map[string]any{"youtube_channel": ch}); err != nil {
			log.Printf("web: set youtube channel: %v", err)
		}
	}
	s.advanceOnboarding(acct.ID)
	seeOther(w, r, "/accounts/"+strconv.FormatInt(acct.ID, 10))
}

// advanceOnboarding ports _advance_onboarding: run OnboardStep up to 8
// times until the status stops changing.
func (s *Server) advanceOnboarding(accountID int64) {
	for i := 0; i < 8; i++ {
		acct, err := s.Mgr.Get(accountID)
		if err != nil {
			return
		}
		before := acct.Status
		var next string
		if s.LLM != nil {
			next, err = network.OnboardStepWithLLM(s.Mgr, accountID, s.LLM)
		} else {
			next, err = network.OnboardStep(s.Mgr, accountID)
		}
		if err != nil {
			log.Printf("web: onboard step: %v", err)
			return
		}
		if next == before {
			return
		}
	}
}

type lastTopicView struct {
	Topic     string
	Reason    string
	CreatedAt string
}

func (s *Server) handleAccountDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	acct, err := s.Mgr.GetWithAutopilot(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		s.fail(w, err, "get account")
		return
	}
	allowed := make([]string, 0)
	for to := range network.TRANSITIONS[acct.Status] {
		allowed = append(allowed, to)
	}
	sort.Strings(allowed)

	var persona *network.Persona
	if p, ok := network.PERSONAS[acct.Persona]; ok {
		cp := p
		persona = &cp
	}
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions WHERE target=? ORDER BY id DESC LIMIT 20",
		acct.Username)
	if err != nil {
		s.fail(w, err, "account decisions")
		return
	}
	pubNames := []string{}
	for _, p := range publishers.BuildPublishers(acct.Username, acct.YoutubeChannel, acct.YoutubeContentTypes) {
		pubNames = append(pubNames, p.Name())
	}
	gifts, err := s.Ledger.AccountGiftUSD(id)
	if err != nil {
		s.fail(w, err, "account gifts")
		return
	}
	var lastTopic *lastTopicView
	rows, err := s.queryDecisions(
		`SELECT id, agent, action, target, reason, created_at FROM decisions
		 WHERE agent='live_planner' AND action='session_topic' AND target LIKE ? ORDER BY id DESC LIMIT 1`,
		acct.Username+":%")
	if err != nil {
		s.fail(w, err, "last topic")
		return
	}
	if len(rows) > 0 {
		topic := rows[0].Target
		if i := strings.Index(topic, ":"); i >= 0 {
			topic = topic[i+1:]
		}
		lastTopic = &lastTopicView{Topic: topic, Reason: rows[0].Reason, CreatedAt: rows[0].CreatedAt}
	}
	detailCtx := s.ctx(
		"Acct", s.wrapAccount(acct),
		"Allowed", allowed,
		"Persona", persona,
		"Decisions", decisions,
		"Publishers", pubNames,
		"Gifts", gifts,
		"LastTopic", lastTopic,
		"Themes", themeOptions(),
		"AutopilotReady", network.AutopilotReady(acct),
		"ModelPhotos", s.modelPhotoViews(id),
		"StudioOK", s.Studio != nil,
		"Error", r.URL.Query().Get("err"),
		"Notice", r.URL.Query().Get("ok"),
	)
	for k, v := range s.accountGrowthView(acct) {
		detailCtx[k] = v
	}
	s.render(w, "account_detail", detailCtx)
}

func (s *Server) accountID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

func (s *Server) handleAccountTransition(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	if to := r.PostFormValue("to"); to != "" {
		if _, err := s.Mgr.Transition(id, to, nil); err != nil {
			log.Printf("web: transition: %v", err)
		}
	}
	seeOther(w, r, "/accounts/"+strconv.FormatInt(id, 10))
}

func (s *Server) handleAccountFollowers(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	n, _ := strconv.ParseInt(r.PostFormValue("n"), 10, 64)
	if n < 0 {
		n = 0
	}
	if err := s.Ledger.SetFollowers(id, n); err != nil {
		log.Printf("web: set followers: %v", err)
	}
	seeOther(w, r, "/accounts/"+strconv.FormatInt(id, 10))
}

func (s *Server) handleAccountYoutube(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	channel := strings.TrimSpace(r.PostFormValue("channel"))
	var kinds []string
	for _, k := range strings.Split(r.PostFormValue("content_types"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			kinds = append(kinds, k)
		}
	}
	if _, err := s.Mgr.SetYoutube(id, channel, kinds); err != nil {
		log.Printf("web: set youtube: %v", err)
	}
	seeOther(w, r, "/accounts/"+strconv.FormatInt(id, 10))
}

func (s *Server) handleAccountOnboard(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	s.advanceOnboarding(id)
	seeOther(w, r, "/accounts/"+strconv.FormatInt(id, 10))
}

func (s *Server) handleAccountReplan(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	acct, err := s.Mgr.Get(id)
	if err == nil {
		research := network.MakeTopicResearch(s.Mgr, id, s.LLM)
		if _, err := research(acct.NicheHint); err != nil {
			log.Printf("web: replan: %v", err)
		}
	}
	seeOther(w, r, "/accounts/"+strconv.FormatInt(id, 10))
}

func (s *Server) handleAccountLiveTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if _, _, err := s.Mgr.PlanLiveTopic(s.LLM, id); err != nil {
		log.Printf("web: plan live topic: %v", err)
	}
	seeOther(w, r, "/accounts/"+strconv.FormatInt(id, 10))
}

// ---------------------------------------------------------------- schedule

var weekdayNames = []string{"Thứ 2", "Thứ 3", "Thứ 4", "Thứ 5", "Thứ 6", "Thứ 7", "Chủ nhật"}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	today := s.today()
	slots, err := s.Ledger.GetSlots(today)
	if err != nil {
		s.fail(w, err, "get slots")
		return
	}
	eligible := 0
	if accts, err := s.Mgr.List("live_ready", "live"); err == nil {
		eligible = len(accts)
	}
	// Go Sunday=0; Python weekday() Monday=0.
	wd := (int(time.Now().In(s.location()).Weekday()) + 6) % 7
	s.render(w, "schedule", s.ctx(
		"Slots", s.slotViews(slots),
		"Today", today,
		"EligibleCount", eligible,
		"DayName", weekdayNames[wd],
	))
}

func (s *Server) handleScheduleBuild(w http.ResponseWriter, r *http.Request) {
	today := s.today()
	accts, err := s.Ledger.ListAccounts(nil)
	if err != nil {
		s.fail(w, err, "list accounts for schedule")
		return
	}
	wd := (int(time.Now().In(s.location()).Weekday()) + 6) % 7
	slots := network.BuildSchedule(accts, wd)
	for i := range slots {
		slots[i].SlotDate = today
		slots[i].Status = "planned"
	}
	if _, err := s.db.Exec("DELETE FROM live_slots WHERE slot_date=?", today); err != nil {
		s.fail(w, err, "clear old slots")
		return
	}
	if err := s.Ledger.SaveSlots(slots); err != nil {
		s.fail(w, err, "save slots")
		return
	}
	if err := s.Ledger.Decide("scheduler", "build_schedule", &today,
		fmt.Sprintf("%d slots", len(slots)), map[string]any{}); err != nil {
		log.Printf("web: decide build_schedule: %v", err)
	}
	seeOther(w, r, "/schedule")
}

// ----------------------------------------------------------------- content

func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	s.render(w, "content", s.ctx("Jobs", s.Jobs.All()))
}

func (s *Server) handleContentCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse content form")
		return
	}
	var captions []string
	for _, c := range strings.Split(r.PostFormValue("captions"), "\n") {
		if c = strings.TrimSpace(c); c != "" {
			captions = append(captions, c)
		}
	}
	job := Job{
		ID:        NewJobID(),
		Title:     strings.TrimSpace(r.PostFormValue("title")),
		Captions:  captions,
		Narration: strings.TrimSpace(r.PostFormValue("narration")),
		Status:    "queued",
		CreatedAt: s.nowISO(),
		Log:       "Đang chờ…",
	}
	if job.Title == "" {
		seeOther(w, r, "/content")
		return
	}
	s.Jobs.Add(job)
	go s.runJob(job.ID) // never blocks the dashboard; panics are contained
	seeOther(w, r, "/content")
}

// runJob renders one content job in the background and records the
// decision. It never crashes the dashboard.
func (s *Server) runJob(jobID string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("web: job %s panic: %v", jobID, rec)
			s.Jobs.Update(jobID, func(j *Job) {
				j.Status = "failed"
				j.FinishedAt = s.nowISO()
			})
		}
	}()
	job, ok := s.Jobs.Get(jobID)
	if !ok {
		return
	}
	s.Jobs.Update(jobID, func(j *Job) {
		j.Status = "running"
		j.Log = "Bắt đầu dựng video…"
	})
	ctx := context.Background()
	res, err := s.renderer.Render(ctx, job, s.TTS, func(line string) {
		s.Jobs.AppendLog(jobID, line)
	})
	if err != nil {
		log.Printf("web: render job %s: %v", jobID, err)
		s.Jobs.Update(jobID, func(j *Job) {
			j.Status = "failed"
			j.FinishedAt = s.nowISO()
		})
		s.Jobs.AppendLog(jobID, fmt.Sprintf("Lỗi: %v", err))
		return
	}
	title := job.Title
	if err := s.Ledger.Decide("content", "video_rendered", &title,
		fmt.Sprintf("kinetic video %.1fs", res.Seconds),
		map[string]any{"job": job.ID, "voice": res.HasAudio}); err != nil {
		log.Printf("web: decide video_rendered: %v", err)
	}
	s.Jobs.Update(jobID, func(j *Job) {
		j.Status = "done"
		j.Output = res.Output
		j.FinishedAt = s.nowISO()
	})
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Strict allowlist: a single path segment, .mp4 only. Anything else
	// (including "..") is a 404.
	if !mediaNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, s.OutDir+"/"+name)
}

// --------------------------------------------------------------- publishers

func (s *Server) handlePublishers(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	rows := make([]publisherRow, 0, len(accounts))
	for _, a := range accounts {
		var plats []string
		for _, p := range publishers.BuildPublishers(a.Username, a.YoutubeChannel, a.YoutubeContentTypes) {
			plats = append(plats, p.Name())
		}
		rows = append(rows, publisherRow{
			Username:     a.Username,
			Status:       a.Status,
			TiktokToken:  fileExists(publishers.TikTokTokenPath(a.Username)),
			TiktokClient: publishers.NewTikTokPublisher(a.Username).HasClient(),
			YoutubeToken: fileExists(publishers.YouTubeTokenPath(a.Username)),
			FbPage:       strings.TrimSpace(getenv("FB_PAGE_ID_"+strings.ToUpper(a.Username), "")) != "",
			Rtmp:         a.RtmpKey() != "",
			Platforms:    plats,
		})
	}
	anyClient := false
	for _, row := range rows {
		anyClient = anyClient || row.TiktokClient
	}
	s.render(w, "publishers", s.ctx("Rows", rows, "AnyTiktokClient", anyClient,
		"Msg", r.URL.Query().Get("msg"), "Err", r.URL.Query().Get("err"),
		"RedirectURI", publishers.TikTokRedirectURI(), "RedirectLocal", publishers.TikTokRedirectIsLocal()))
}

// --------------------------------------------------------------------- shop

type productView struct {
	Title           string
	PlatformPID     string
	Category        string
	Price           float64
	CommissionRate  float64
	CommissionValue float64
	Score           float64
	Status          string
}

func (s *Server) handleShop(w http.ResponseWriter, r *http.Request) {
	products, err := s.Ledger.ShelfProducts(50)
	if err != nil {
		s.fail(w, err, "shelf products")
		return
	}
	views := make([]productView, 0, len(products))
	for _, p := range products {
		views = append(views, productView{
			Title: p.Title, PlatformPID: p.PlatformPID,
			Category: nullStr(p.Category), Price: p.Price,
			CommissionRate: p.CommissionRate, CommissionValue: p.CommissionValue,
			Score: p.Score, Status: p.Status,
		})
	}
	s.render(w, "shop", s.ctx("Products", views))
}

func (s *Server) handleShopAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse shop form")
		return
	}
	pid := strings.TrimSpace(r.PostFormValue("platform_pid"))
	title := strings.TrimSpace(r.PostFormValue("title"))
	price, _ := strconv.ParseFloat(r.PostFormValue("price"), 64)
	rate, _ := strconv.ParseFloat(r.PostFormValue("commission_rate"), 64)
	if pid == "" || title == "" {
		seeOther(w, r, "/shop")
		return
	}
	category := strings.TrimSpace(r.PostFormValue("category"))
	fields := map[string]any{
		"title": title, "price": price, "commission_rate": rate,
		"commission_value": price * rate,
		"category":         nil,
		"status":           "shelf", "score": 0.0,
	}
	if category != "" {
		fields["category"] = category
	}
	if _, err := s.Ledger.UpsertProduct(pid, fields); err != nil {
		log.Printf("web: upsert product: %v", err)
	}
	seeOther(w, r, "/shop")
}

// ---------------------------------------------------------------- analytics

type giftRow struct {
	Username string
	USD      float64
}

type usageRow struct {
	Engine   string
	Provider string
	N        int64
	Cost     float64
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	revenue := s.scalarFloat("SELECT COALESCE(SUM(commission),0) FROM orders")
	commissions, err := s.Ledger.TotalRevenue()
	if err != nil {
		s.fail(w, err, "total revenue")
		return
	}
	sessions := s.scalarInt("SELECT COUNT(*) FROM live_sessions")

	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	var gifts []giftRow
	for _, a := range accounts {
		if g, err := s.Ledger.AccountGiftUSD(a.ID); err == nil && g > 0 {
			gifts = append(gifts, giftRow{Username: a.Username, USD: g})
		}
	}
	usage, err := s.queryUsage()
	if err != nil {
		s.fail(w, err, "usage")
		return
	}
	spend, err := s.Ledger.DailySpendUSD()
	if err != nil {
		s.fail(w, err, "daily spend")
		return
	}
	s.render(w, "analytics", s.ctx(
		"Revenue", revenue,
		"Commissions", commissions,
		"Sessions", sessions,
		"GiftRows", gifts,
		"Usage", usage,
		"Spend", spend,
	))
}

func (s *Server) queryUsage() ([]usageRow, error) {
	rows, err := s.db.Query(
		`SELECT engine, provider, COUNT(*) n, COALESCE(SUM(cost_usd),0) cost
		 FROM api_usage GROUP BY engine, provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []usageRow
	for rows.Next() {
		var u usageRow
		if err := rows.Scan(&u.Engine, &u.Provider, &u.N, &u.Cost); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------- settings

type envRow struct {
	Name string
	Ok   bool
	// Masked is the secret-safe tail display ("••••abcd"); empty when unset.
	Masked string
	// FromDB marks values saved through the settings UI (persisted in the
	// settings table) as opposed to process environment variables.
	FromDB bool
}

// envNames is the allowlist of environment variables editable in Settings.
// Values apply to the running process immediately and persist in the
// settings table (applied at startup), so they survive restarts.
var envNames = []string{"TTS_API_KEY", "TIKTOK_SHOP_APP_KEY", "TIKTOK_SHOP_APP_SECRET",
	"TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET",
	"YOUTUBE_CLIENT_ID", "YOUTUBE_CLIENT_SECRET", "FB_PAGE_ID", "YOUTUBE_API_KEY"}

// envSettingKey namespaces a UI-saved env value inside the settings table.
func envSettingKey(name string) string { return "env:" + name }

// maskSecret renders "••••" + last 4 chars; never reveals more of a secret.
func maskSecret(v string) string {
	if len(v) <= 4 {
		return "••••"
	}
	return "••••" + v[len(v)-4:]
}

// effectiveEnv resolves a variable's current value: UI-saved (settings
// table) wins over the process environment.
func (s *Server) effectiveEnv(name string) (string, bool) {
	if s.Ledger != nil {
		if v, ok, err := s.Ledger.GetSetting(envSettingKey(name)); err == nil && ok {
			return v, v != ""
		}
	}
	v := getenv(name, "")
	return v, v != ""
}

// applyPersistedEnv re-applies UI-saved env values into the process
// environment at startup so call-time getenv consumers see them after a
// restart. Called from NewServer.
func (s *Server) applyPersistedEnv() {
	if s.Ledger == nil {
		return
	}
	all, err := s.Ledger.AllSettings()
	if err != nil {
		return
	}
	for k, v := range all {
		if name, ok := strings.CutPrefix(k, "env:"); ok {
			if v == "" {
				os.Unsetenv(name)
			} else {
				os.Setenv(name, v)
			}
		}
	}
}

type rtmpRow struct {
	Username string
	Ref      string
	Ok       bool
}

func (s *Server) chainKey(name string) (string, bool) {
	switch name {
	case "tts":
		return ledger.SettingTTSChain, true
	case "llm":
		return ledger.SettingLLMChain, true
	case "avatar":
		return ledger.SettingAvatarChain, true
	}
	return "", false
}

// loadChain returns the stored chain config, falling back to defaults when
// nothing was saved yet.
func (s *Server) loadChain(name string) ChainConfig {
	key, ok := s.chainKey(name)
	if !ok {
		return ChainConfig{}
	}
	raw, found, err := s.Ledger.GetSetting(key)
	if err == nil && found && raw != "" {
		if c := UnmarshalChain(raw); len(c.Order) > 0 {
			return c
		}
	}
	if name == "tts" {
		return DefaultTTSConfig(s.Cfg.GeminiAPIKeys)
	}
	if name == "avatar" {
		return DefaultAvatarConfig()
	}
	return DefaultLLMConfig(s.Cfg.GeminiAPIKeys)
}

// saveChain persists the config and applies it to the live chain
// immediately (no restart).
func (s *Server) saveChain(name string, c ChainConfig) error {
	key, ok := s.chainKey(name)
	if !ok {
		return fmt.Errorf("unknown chain %q", name)
	}
	raw, err := MarshalChain(c)
	if err != nil {
		return err
	}
	if err := s.Ledger.SetSetting(key, raw); err != nil {
		return err
	}
	s.applyChain(name, ChainConfigJSON(raw))
	return nil
}

func (s *Server) applyChain(name string, raw ChainConfigJSON) {
	switch name {
	case "tts":
		if s.TTS != nil {
			s.TTS.SetConfig(raw)
		}
	case "llm":
		// LLM is the narrow LLMClient interface; the real chain (injected
		// by the wiring worker) also implements SetConfig.
		if sc, ok := s.LLM.(interface{ SetConfig(ChainConfigJSON) }); ok && s.LLM != nil {
			sc.SetConfig(raw)
		}
	case "avatar":
		if s.Avatar != nil {
			s.Avatar.SetConfig(raw)
		}
	}
}

type vieneuView struct {
	Connected bool
	State     string
	Detail    string
	Voices    []string
}

func (s *Server) vieneuView() vieneuView {
	v := vieneuView{}
	if s.VieNeu == nil {
		return v
	}
	v.Connected = true
	v.State, v.Detail = s.VieNeu.Status()
	v.Voices = s.VieNeu.Voices()
	if len(v.Voices) == 0 {
		v.Voices = VieNeuVoiceFallback
	}
	return v
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	envs := make([]envRow, 0, len(envNames))
	for _, e := range envNames {
		v, ok := s.effectiveEnv(e)
		row := envRow{Name: e, Ok: ok}
		if ok {
			row.Masked = maskSecret(v)
		}
		if s.Ledger != nil {
			if _, inDB, err := s.Ledger.GetSetting(envSettingKey(e)); err == nil && inDB {
				row.FromDB = true
			}
		}
		envs = append(envs, row)
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	rtmps := make([]rtmpRow, 0, len(accounts))
	for _, a := range accounts {
		rtmps = append(rtmps, rtmpRow{Username: a.Username, Ref: a.RtmpKeyRef, Ok: a.RtmpKey() != ""})
	}
	s.render(w, "settings", s.ctx(
		"EnvStatus", envs,
		"EnvSaved", r.URL.Query().Get("envsaved"),
		"RtmpRows", rtmps,
		"DbPath", s.Cfg.DatabasePath,
		"TTSChain", s.loadChain("tts"),
		"LLMChain", s.loadChain("llm"),
		"TTSKeys", s.keyRingStatuses("tts", "gemini"),
		"LLMKeys", s.keyRingStatuses("llm", "gemini"),
		"VieNeu", s.vieneuView(),
		"AvatarChain", s.loadChain("avatar"),
		"AvatarRealtime", s.avatarRealtime(),
		"Characters", s.characterViews(),
		"AvatarSidecar", s.avatarSidecarView(),
	))
}

// handleSettingsEnvSave saves one environment variable from the Settings
// UI: it applies to the running process immediately and persists in the
// settings table, so it is re-applied at startup. An empty value clears
// the variable.
func (s *Server) handleSettingsEnvSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse env form")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	allowed := false
	for _, n := range envNames {
		if n == name {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "biến không được phép sửa tại đây", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.FormValue("value"))
	if value == "" {
		os.Unsetenv(name)
	} else {
		os.Setenv(name, value)
	}
	if s.Ledger != nil {
		if err := s.Ledger.SetSetting(envSettingKey(name), value); err != nil {
			s.fail(w, err, "save env setting")
			return
		}
	}
	http.Redirect(w, r, "/settings?envsaved="+name+"#bien-moi-truong", http.StatusSeeOther)
}

// avatarRealtime reports whether a CLOUD avatar tier (HeyGen/D-ID) is
// enabled — the only tiers that can truly stream in realtime. The local
// MuseTalk tier renders offline only (ước tính 6–10 phút cho clip 60
// giây trên Mac M1) and must never count toward this badge, even though
// its contract exposes a stream interface (see docs/MAC_M1_CORE_AUDIT.md).
func (s *Server) avatarRealtime() bool {
	for _, p := range s.loadChain("avatar").Order {
		if p.Enabled && p.Name != "local" {
			return true
		}
	}
	return false
}

func (s *Server) handleSettingsDryRun(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	// NOTE: the required bugfix — dry_run is ON exactly when value == "on".
	on := r.PostFormValue("value") == "on"
	s.Cfg.SetDryRun(on)
	if err := s.Ledger.Decide("human", "dry_run", nil,
		fmt.Sprintf("set dry_run=%v via dashboard", on), map[string]any{}); err != nil {
		log.Printf("web: decide dry_run: %v", err)
	}
	seeOther(w, r, "/settings")
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	s.Cfg.SetKillSwitch(true)
	if err := s.Ledger.Decide("human", "kill_switch", nil,
		"engaged via dashboard", map[string]any{}); err != nil {
		log.Printf("web: decide kill: %v", err)
	}
	seeOther(w, r, "/")
}

func (s *Server) handleUnkill(w http.ResponseWriter, r *http.Request) {
	s.Cfg.SetKillSwitch(false)
	if err := s.Ledger.Decide("human", "kill_switch", nil,
		"released via dashboard", map[string]any{}); err != nil {
		log.Printf("web: decide unkill: %v", err)
	}
	seeOther(w, r, "/")
}

// ------------------------------------------------------- chain settings API

// handleChainGet returns the current chain config as JSON (stored config,
// or defaults when nothing was saved yet). API keys are masked — the raw
// values never leave the server.
func (s *Server) handleChainGet(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if _, ok := s.chainKey(name); !ok {
		http.Error(w, "unknown chain (name=tts|llm|avatar)", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	raw, _ := MarshalChain(maskChainKeys(s.loadChain(name)))
	_, _ = w.Write([]byte(raw))
}

// maskChainKeys returns a copy of cfg with raw API keys replaced by masked
// display values. The settings JSON API never leaks secrets.
func maskChainKeys(cfg ChainConfig) ChainConfig {
	out := ChainConfig{Order: make([]ProviderEntry, len(cfg.Order))}
	for i, e := range cfg.Order {
		e.APIKey = ""
		if len(e.APIKeys) > 0 {
			masked := make([]string, len(e.APIKeys))
			for j, k := range e.APIKeys {
				masked[j] = MaskKey(k)
			}
			e.APIKeys = masked
		}
		out.Order[i] = e
	}
	return out
}

// handleChainSave stores the whole chain form: repeated pname / enabled
// (checked indices) / apikey / timeout (sec) / retries fields, aligned by
// row index. Applies to the live chain immediately.
func (s *Server) handleChainSave(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if _, ok := s.chainKey(name); !ok {
		http.Error(w, "unknown chain (name=tts|llm|avatar)", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse chain form")
		return
	}
	names := r.PostForm["pname"]
	apikeys := r.PostForm["apikey"]
	timeouts := r.PostForm["timeout"]
	retries := r.PostForm["retries"]
	enabled := map[int]bool{}
	for _, v := range r.PostForm["enabled"] {
		if i, err := strconv.Atoi(v); err == nil {
			enabled[i] = true
		}
	}
	cfg := ChainConfig{}
	stored := s.loadChain(name) // key preservation for keyring-managed rows
	for i, pname := range names {
		e := ProviderEntry{Name: strings.TrimSpace(pname), Enabled: enabled[i]}
		if e.Name == "" {
			continue
		}
		if e.Name == "gemini" {
			// Keys of gemini rows are managed by the keyring UI through
			// the /settings/keys/* endpoints. The row posts an empty
			// apikey placeholder to keep the repeated-field alignment;
			// an empty value preserves the stored keys instead of
			// wiping them. A non-empty apikey is still honored as the
			// legacy single-key path.
			if i < len(apikeys) && strings.TrimSpace(apikeys[i]) != "" {
				e.SetAPIKeysText(apikeys[i])
			} else if se := findChainEntry(stored, e.Name); se != nil {
				e.APIKeys, e.APIKey = se.APIKeys, se.APIKey
			}
		} else if i < len(apikeys) {
			e.APIKey = strings.TrimSpace(apikeys[i])
		}
		if i < len(timeouts) {
			if v, err := strconv.Atoi(strings.TrimSpace(timeouts[i])); err == nil && v > 0 {
				e.TimeoutSec = v
			}
		}
		if i < len(retries) {
			if v, err := strconv.Atoi(strings.TrimSpace(retries[i])); err == nil && v >= 0 {
				e.Retries = v
			}
		}
		cfg.Order = append(cfg.Order, e)
	}
	if len(cfg.Order) == 0 {
		http.Error(w, "empty chain", http.StatusBadRequest)
		return
	}
	if err := s.saveChain(name, cfg); err != nil {
		s.fail(w, err, "save chain")
		return
	}
	seeOther(w, r, "/settings")
}

// handleChainMove reorders one provider up/down (replaces drag-drop).
func (s *Server) handleChainMove(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	dir := r.URL.Query().Get("dir")
	i, _ := strconv.Atoi(r.URL.Query().Get("i"))
	if _, ok := s.chainKey(name); !ok {
		http.Error(w, "unknown chain", http.StatusBadRequest)
		return
	}
	cfg := s.loadChain(name)
	if i >= 0 && i < len(cfg.Order) {
		j := i - 1
		if dir == "down" {
			j = i + 1
		}
		if j >= 0 && j < len(cfg.Order) {
			cfg.Order[i], cfg.Order[j] = cfg.Order[j], cfg.Order[i]
			if err := s.saveChain(name, cfg); err != nil {
				s.fail(w, err, "move chain entry")
				return
			}
		}
	}
	seeOther(w, r, "/settings")
}

// handleChainHealth probes one provider ("Kiểm tra kết nối") and returns
// {"ok": true|false} as JSON.
func (s *Server) handleChainHealth(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	provider := r.URL.Query().Get("provider")
	w.Header().Set("Content-Type", "application/json")
	checker := s.Health[name+":"+provider]
	if checker == nil {
		checker = s.Health[provider]
	}
	ok := false
	if checker != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		func() {
			defer func() {
				_ = recover() // a bad checker must not crash the dashboard
			}()
			ok = checker.Healthy(ctx)
		}()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": ok, "provider": provider})
}

// ------------------------------------------------------- keyring (API keys)

// keyChainParams resolves ?chain=tts|llm + ?provider= for the key endpoints.
func (s *Server) keyChainParams(r *http.Request) (chainName, provider string, ok bool) {
	chainName = r.URL.Query().Get("chain")
	provider = strings.TrimSpace(r.URL.Query().Get("provider"))
	if _, ok = s.chainKey(chainName); !ok {
		return "", "", false
	}
	if provider == "" {
		return "", "", false
	}
	return chainName, provider, true
}

func writeJSONErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

// keyRingStatuses returns the render-ready per-key state for a provider.
// It prefers the live keyring (real cooldown/invalid state); when the
// chain adapter is not wired (or doesn't rotate keys) it falls back to
// the stored config's keys, all reported "ok". Raw keys never leave the
// server — every entry is masked.
func (s *Server) keyRingStatuses(chainName, provider string) []KeyStatus {
	var src any
	switch chainName {
	case "tts":
		src = s.TTS
	case "llm":
		src = s.LLM
	}
	if src != nil {
		if kp, ok := src.(KeyStatusProvider); ok {
			if live := kp.KeyStatus(provider); len(live) > 0 {
				for i := range live {
					live[i].Total = len(live)
					live[i].FillDerived()
				}
				return live
			}
		}
	}
	// Fallback: stored config keys, reported healthy.
	cfg := s.loadChain(chainName)
	for _, e := range cfg.Order {
		if e.Name != provider {
			continue
		}
		keys := e.APIKeys
		if len(keys) == 0 && e.APIKey != "" {
			keys = []string{e.APIKey}
		}
		out := make([]KeyStatus, 0, len(keys))
		for i, k := range keys {
			st := KeyStatus{Index: i, Last4: last4(k), State: "ok", Total: len(keys)}
			st.FillDerived()
			out = append(out, st)
		}
		return out
	}
	return []KeyStatus{}
}

// storedKeys returns the provider row's keys from the stored chain config.
func (s *Server) storedKeys(chainName, provider string) []string {
	cfg := s.loadChain(chainName)
	for _, e := range cfg.Order {
		if e.Name != provider {
			continue
		}
		if len(e.APIKeys) > 0 {
			return append([]string(nil), e.APIKeys...)
		}
		if e.APIKey != "" {
			return []string{e.APIKey}
		}
		return nil
	}
	return nil
}

// mutateKeys loads the stored chain, applies fn to the provider row's key
// list, then saves — persisting to the DB and applying to the live chain
// immediately (no restart). Returns the updated key list.
func (s *Server) mutateKeys(chainName, provider string, fn func(keys []string) []string) ([]string, error) {
	cfg := s.loadChain(chainName)
	idx := -1
	for i, e := range cfg.Order {
		if e.Name == provider {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("unknown provider %q in chain %q", provider, chainName)
	}
	keys := s.storedKeys(chainName, provider)
	keys = fn(keys)
	cfg.Order[idx].SetAPIKeysText(strings.Join(keys, "\n"))
	if err := s.saveChain(chainName, cfg); err != nil {
		return nil, err
	}
	return keys, nil
}

// handleKeyAdd appends one API key to the provider's keyring (deduplicated)
// and applies it to the live chain immediately. JSON, no page reload.
func (s *Server) handleKeyAdd(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONErr(w, "không đọc được form", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(r.PostFormValue("key"))
	if key == "" {
		writeJSONErr(w, "key trống — hãy dán API key vào ô nhập", http.StatusBadRequest)
		return
	}
	duplicate := false
	keys, err := s.mutateKeys(chainName, provider, func(keys []string) []string {
		for _, k := range keys {
			if k == key {
				duplicate = true
				return keys
			}
		}
		return append(keys, key)
	})
	if err != nil {
		writeJSONErr(w, "không lưu được key: "+err.Error(), http.StatusInternalServerError)
		return
	}
	masked := MaskKey(key)
	if err := s.Ledger.Decide("human", "api_key_add", nil,
		fmt.Sprintf("thêm API key %s vào %s/%s%s", masked, chainName, provider,
			map[bool]string{true: " (đã có)", false: ""}[duplicate]),
		map[string]any{"chain": chainName, "provider": provider, "masked": masked}); err != nil {
		log.Printf("web: decide api_key_add: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "duplicate": duplicate, "masked": masked, "total": len(keys),
	})
}

// handleKeyDelete removes one API key by index and applies immediately.
func (s *Server) handleKeyDelete(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONErr(w, "không đọc được form", http.StatusBadRequest)
		return
	}
	idx, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("idx")))
	if err != nil {
		writeJSONErr(w, "thiếu idx", http.StatusBadRequest)
		return
	}
	before := s.storedKeys(chainName, provider)
	if idx < 0 || idx >= len(before) {
		writeJSONErr(w, "key không tồn tại", http.StatusBadRequest)
		return
	}
	masked := MaskKey(before[idx])
	keys, err := s.mutateKeys(chainName, provider, func(keys []string) []string {
		return append(keys[:idx], keys[idx+1:]...)
	})
	if err != nil {
		writeJSONErr(w, "không xóa được key: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.Ledger.Decide("human", "api_key_delete", nil,
		fmt.Sprintf("xóa API key %s khỏi %s/%s", masked, chainName, provider),
		map[string]any{"chain": chainName, "provider": provider, "masked": masked}); err != nil {
		log.Printf("web: decide api_key_delete: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "masked": masked, "total": len(keys),
	})
}

// handleKeyStatus returns the masked per-key state as JSON for the keyring
// UI (initial page render uses the same data server-side).
func (s *Server) handleKeyStatus(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	keys := s.keyRingStatuses(chainName, provider)
	if keys == nil {
		keys = []KeyStatus{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "keys": keys})
}

// handleKeyTest probes one key (ValidateKey) and returns the result as
// JSON. The key stays masked in the response; error text comes from the
// provider and contains only the masked form.
func (s *Server) handleKeyTest(w http.ResponseWriter, r *http.Request) {
	chainName, provider, ok := s.keyChainParams(r)
	if !ok {
		writeJSONErr(w, "unknown chain (chain=tts|llm) hoặc thiếu provider", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONErr(w, "không đọc được form", http.StatusBadRequest)
		return
	}
	idx, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("idx")))
	if err != nil {
		writeJSONErr(w, "thiếu idx", http.StatusBadRequest)
		return
	}
	if keys := s.storedKeys(chainName, provider); idx < 0 || idx >= len(keys) {
		writeJSONErr(w, "key không tồn tại", http.StatusBadRequest)
		return
	}
	var tester KeyTester
	switch chainName {
	case "tts":
		tester, _ = s.TTS.(KeyTester)
	case "llm":
		tester, _ = s.LLM.(KeyTester)
	}
	if tester == nil {
		writeJSONErr(w, "provider chưa hỗ trợ kiểm tra key", http.StatusNotImplemented)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var testErr error
	func() {
		defer func() { _ = recover() }()
		testErr = tester.ValidateKey(ctx, provider, idx)
	}()
	w.Header().Set("Content-Type", "application/json")
	if testErr != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": testErr.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// ------------------------------------------------------------- VieNeu panel

func (s *Server) vieneuRequired(w http.ResponseWriter, r *http.Request) bool {
	if s.VieNeu == nil {
		http.Error(w, "VieNeu sidecar chưa được kết nối", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *Server) handleVieneuStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.VieNeu == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"connected": false})
		return
	}
	state, detail := s.VieNeu.Status()
	voices := s.VieNeu.Voices()
	if len(voices) == 0 {
		voices = VieNeuVoiceFallback
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"connected": true, "state": state, "detail": detail, "voices": voices,
	})
}

// handleVieneuEnsure starts the model download in the background; progress
// is polled via /settings/vieneu/progress.
func (s *Server) handleVieneuEnsure(w http.ResponseWriter, r *http.Request) {
	if !s.vieneuRequired(w, r) {
		return
	}
	s.vieneuMu.Lock()
	s.vieneuDownloaded, s.vieneuTotal, s.vieneuErr = 0, 0, ""
	s.vieneuDone = false
	s.vieneuMu.Unlock()
	go func() {
		err := s.VieNeu.EnsureModel(context.Background(), func(downloaded, total int64) {
			s.vieneuMu.Lock()
			s.vieneuDownloaded, s.vieneuTotal = downloaded, total
			s.vieneuMu.Unlock()
		})
		s.vieneuMu.Lock()
		s.vieneuDone = true
		if err != nil {
			s.vieneuErr = err.Error()
		}
		s.vieneuMu.Unlock()
	}()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"started": true})
}

func (s *Server) handleVieneuProgress(w http.ResponseWriter, r *http.Request) {
	s.vieneuMu.Lock()
	defer s.vieneuMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"connected":  s.VieNeu != nil,
		"downloaded": s.vieneuDownloaded,
		"total":      s.vieneuTotal,
		"done":       s.vieneuDone,
		"error":      s.vieneuErr,
	})
}

func (s *Server) handleVieneuRestart(w http.ResponseWriter, r *http.Request) {
	if !s.vieneuRequired(w, r) {
		return
	}
	if err := s.VieNeu.Restart(r.Context()); err != nil {
		log.Printf("web: vieneu restart: %v", err)
	}
	seeOther(w, r, "/settings")
}

func (s *Server) handleVieneuVoice(w http.ResponseWriter, r *http.Request) {
	if !s.vieneuRequired(w, r) {
		return
	}
	_ = r.ParseForm()
	if preset := strings.TrimSpace(r.PostFormValue("preset")); preset != "" {
		if err := s.VieNeu.SetVoice(preset); err != nil {
			log.Printf("web: vieneu set voice: %v", err)
		}
	}
	seeOther(w, r, "/settings")
}

// ----------------------------------------------------------- legacy JSON API

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("web: write json: %v", err)
	}
}

func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	spend, err := s.Ledger.DailySpendUSD()
	if err != nil {
		s.fail(w, err, "daily spend")
		return
	}
	writeJSON(w, map[string]any{
		"dry_run":          s.Cfg.DryRun(),
		"kill_switch":      s.Cfg.KillSwitch(),
		"total_commission": s.scalarFloat("SELECT COALESCE(SUM(commission),0) FROM orders"),
		"daily_spend_usd":  spend,
	})
}

type productJSON struct {
	ID              int64   `json:"id"`
	PlatformPID     string  `json:"platform_pid"`
	Title           string  `json:"title"`
	Category        string  `json:"category"`
	Price           float64 `json:"price"`
	CommissionRate  float64 `json:"commission_rate"`
	CommissionValue float64 `json:"commission_value"`
	Score           float64 `json:"score"`
	Status          string  `json:"status"`
}

func (s *Server) handleAPIProducts(w http.ResponseWriter, r *http.Request) {
	products, err := s.Ledger.GetProducts()
	if err != nil {
		s.fail(w, err, "get products")
		return
	}
	sort.Slice(products, func(i, j int) bool { return products[i].Score > products[j].Score })
	if len(products) > 50 {
		products = products[:50]
	}
	out := make([]productJSON, 0, len(products))
	for _, p := range products {
		out = append(out, productJSON{
			ID: p.ID, PlatformPID: p.PlatformPID, Title: p.Title,
			Category: nullStr(p.Category), Price: p.Price,
			CommissionRate: p.CommissionRate, CommissionValue: p.CommissionValue,
			Score: p.Score, Status: p.Status,
		})
	}
	writeJSON(w, out)
}

func (s *Server) handleAPIDecisions(w http.ResponseWriter, r *http.Request) {
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions ORDER BY id DESC LIMIT 100")
	if err != nil {
		s.fail(w, err, "api decisions")
		return
	}
	if decisions == nil {
		decisions = []decisionView{}
	}
	writeJSON(w, decisions)
}
