package web

import (
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) accountID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

// accountTab returns the current account-detail tab from ?tab=, defaulting
// to "overview".
func (s *Server) accountTab(r *http.Request) string {
	return accountTabName(r.URL.Query().Get("tab"))
}

func accountTabName(tab string) string {
	switch tab {
	case "overview", "autopilot", "connections", "brand-dna", "comments", "model-photos":
		return tab
	case "tong-quan":
		return "overview"
	case "ket-noi":
		return "connections"
	case "nhan-vat":
		return "model-photos"
	default:
		return "overview"
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
	s.accountBack(w, r, id, "connections", errMsg)
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
	s.accountBack(w, r, id, "connections", errMsg)
}

func (s *Server) handleAccountOnboard(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	s.advanceOnboarding(id)
	s.accountBack(w, r, id, "connections", "")
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
	s.accountBack(w, r, id, "connections", errMsg)
}
