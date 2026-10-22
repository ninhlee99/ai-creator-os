package web

import (
	"database/sql"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type personaChoice struct {
	Key   string
	Label string
	Style string
}

func personaOptions() []personaChoice {
	out := make([]personaChoice, 0, len(network.PERSONAS))
	for k, p := range network.PERSONAS {
		out = append(out, personaChoice{
			Key:   k,
			Label: p.Label,
			Style: p.LiveStyle,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Label < out[j].Label
	})
	return out
}

func (s *Server) handleAccountEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	acct, err := s.Mgr.GetWithAutopilot(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		s.fail(w, err, "get account for edit")
		return
	}

	statuses := []string{
		"onboarding", "researching", "persona_assigned", "growing",
		"live_ready", "live", "paused", "retired",
	}

	s.render(w, "account_edit", s.ctx(
		"Acct", s.wrapAccount(acct),
		"RawAcct", acct,
		"Personas", personaOptions(),
		"Statuses", statuses,
		"Themes", themeOptions(),
		"Error", r.URL.Query().Get("err"),
		"Notice", r.URL.Query().Get("ok"),
	))
}

func (s *Server) handleAccountUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse account edit form")
		return
	}

	username := strings.TrimSpace(r.PostFormValue("username"))
	username = strings.TrimLeft(username, "@")
	if username == "" {
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Tên tài khoản không được để trống.")))
		return
	}

	niche := strings.TrimSpace(r.PostFormValue("niche"))
	nicheHint := strings.TrimSpace(r.PostFormValue("niche_hint"))
	persona := strings.TrimSpace(r.PostFormValue("persona"))
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "growing"
	}
	youtubeChannel := strings.TrimSpace(r.PostFormValue("youtube_channel"))
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	autopilot := r.PostFormValue("autopilot") == "1" || r.PostFormValue("autopilot") == "on"

	minCommRaw := strings.TrimSpace(r.PostFormValue("min_commission"))
	minComm := 0.10
	if minCommRaw != "" {
		if v, err := strconv.ParseFloat(minCommRaw, 64); err == nil && v >= 0 {
			if v > 1.0 {
				v = v / 100.0 // allow user entering 10 for 10%
			}
			minComm = v
		}
	}

	followersRaw := strings.TrimSpace(r.PostFormValue("followers"))
	var followers int64
	if followersRaw != "" {
		followers, _ = strconv.ParseInt(followersRaw, 10, 64)
	}

	_, err := s.Mgr.UpdateAccount(id, username, niche, nicheHint, persona, status, youtubeChannel, theme, autopilot, minComm, followers)
	if err != nil {
		log.Printf("web: update account: %v", err)
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Lỗi khi lưu thông tin: "+err.Error())))
		return
	}

	_ = s.Ledger.Decide("human", "account_edit", &username,
		fmt.Sprintf("Cập nhật thông tin kênh %s thành công", username),
		map[string]any{"id": id, "username": username, "status": status, "niche": niche, "autopilot": autopilot})

	seeOther(w, r, fmt.Sprintf("/accounts/%d?ok=%s", id, url.QueryEscape("Đã cập nhật thông tin kênh thành công!")))
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	acct, err := s.Mgr.Get(id)
	uname := ""
	if err == nil {
		uname = acct.Username
	}

	if err := s.Mgr.DeleteAccount(id); err != nil {
		log.Printf("web: delete account: %v", err)
		seeOther(w, r, fmt.Sprintf("/accounts/%d?err=%s", id, url.QueryEscape("Không xóa được kênh: "+err.Error())))
		return
	}

	if uname != "" {
		_ = s.Ledger.Decide("human", "account_delete", &uname,
			fmt.Sprintf("Xóa kênh %s khỏi hệ thống", uname),
			map[string]any{"id": id, "username": uname})
	}

	seeOther(w, r, "/accounts?ok="+url.QueryEscape("Đã xóa kênh thành công."))
}
