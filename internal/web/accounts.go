package web

// Account pages: list, creation form, onboarding state machine and the
// per-account detail page with its action POSTs (transition, YouTube link,
// replan, live topic).

import (
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

// ytAnalyticsStatus: "ok" (có token file), "chuakenh" (chưa gắn kênh),
// "chuacotoken" (chưa có token). Trung thực — chỉ kiểm tra file tồn tại,
// không đoán scope (thiếu scope → API báo 403 lúc sync).
func ytAnalyticsStatus(username, channel string) string {
	if strings.TrimSpace(channel) == "" {
		return "chuakenh"
	}
	if _, err := os.Stat(publishers.YouTubeTokenPath(username)); err != nil {
		return "chuacotoken"
	}
	return "ok"
}

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
			seeOther(w, r, "/accounts/"+strconv.FormatInt(acct.ID, 10)+"?err="+url.QueryEscape("Không lưu được kênh YouTube: "+err.Error()))
			return
		}
	}
	s.advanceOnboarding(acct.ID)
	// Zero-touch (Đợt 3): the first 30-day growth plan writes itself.
	s.autoGeneratePlan(acct, "Tự sinh khi tạo tài khoản")
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
	detailCtx := s.ctx(
		"Acct", s.wrapAccount(acct),
		"Tab", s.accountTab(r),
		"Allowed", allowed,
		"Persona", persona,
		"Decisions", decisions,
		"Publishers", pubNames,
		"Gifts", gifts,
		"Themes", themeOptions(),
		"AutopilotReady", network.AutopilotReady(acct),
		"ModelPhotos", s.modelPhotoViews(id),
		"StudioOK", s.Studio != nil,
		"YTAnalytics", ytAnalyticsStatus(acct.Username, acct.YoutubeChannel),
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

// accountTab returns the current account-detail tab from ?tab=, defaulting
// to "tong-quan".
func (s *Server) accountTab(r *http.Request) string {
	switch r.URL.Query().Get("tab") {
	case "autopilot", "ket-noi":
		return r.URL.Query().Get("tab")
	default:
		return "tong-quan"
	}
}

// accountBack redirects to the account detail page preserving the given tab
// and surfacing errMsg as a toast (R2-13) instead of a silent redirect.
func (s *Server) accountBack(w http.ResponseWriter, r *http.Request, id int64, tab, errMsg string) {
	u := "/accounts/" + strconv.FormatInt(id, 10) + "?tab=" + tab
	if errMsg != "" {
		u += "&err=" + url.QueryEscape(errMsg)
	}
	seeOther(w, r, u)
}

func (s *Server) handleAccountTransition(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	errMsg := ""
	if to := r.PostFormValue("to"); to != "" {
		if _, err := s.Mgr.Transition(id, to, nil); err != nil {
			errMsg = err.Error()
			log.Printf("web: transition: %v", err)
		}
	}
	s.accountBack(w, r, id, "ket-noi", errMsg)
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
	errMsg := ""
	if _, err := s.Mgr.SetYoutube(id, channel, kinds); err != nil {
		errMsg = err.Error()
		log.Printf("web: set youtube: %v", err)
	}
	s.accountBack(w, r, id, "ket-noi", errMsg)
}

func (s *Server) handleAccountOnboard(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	s.advanceOnboarding(id)
	s.accountBack(w, r, id, "ket-noi", "")
}

func (s *Server) handleAccountReplan(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	errMsg := ""
	acct, err := s.Mgr.Get(id)
	if err == nil {
		research := network.MakeTopicResearch(s.Mgr, id, s.LLM)
		if _, err := research(acct.NicheHint); err != nil {
			errMsg = err.Error()
			log.Printf("web: replan: %v", err)
		}
	} else {
		errMsg = err.Error()
	}
	s.accountBack(w, r, id, "ket-noi", errMsg)
}

// ---------------------------------------------------------------- schedule
