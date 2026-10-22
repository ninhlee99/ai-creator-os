package web

import (
	"database/sql"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
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

func editableAccountStatuses(current string) []string {
	order := []string{
		"onboarding", "researching", "persona_assigned", "growing",
		"live_ready", "live", "paused", "penalized", "retired",
	}
	out := []string{current}
	for _, status := range order {
		if status != current && network.TRANSITIONS[current][status] {
			out = append(out, status)
		}
	}
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

	s.render(w, "account_edit", s.ctx(
		"Acct", s.wrapAccount(acct),
		"RawAcct", acct,
		"Personas", personaOptions(),
		"Statuses", editableAccountStatuses(acct.Status),
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
	current, err := s.Mgr.Get(id)
	if err != nil {
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		s.fail(w, err, "get account for update")
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
		status = current.Status
	}
	if status != current.Status && !network.TRANSITIONS[current.Status][status] {
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Không thể chuyển tài khoản sang giai đoạn này theo quy trình hiện tại.")))
		return
	}
	if persona != "" {
		if _, ok := network.PERSONAS[persona]; !ok {
			seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Tính cách AI không hợp lệ.")))
			return
		}
	}
	youtubeChannel := strings.TrimSpace(r.PostFormValue("youtube_channel"))
	theme := strings.TrimSpace(r.PostFormValue("theme"))
	if theme != "" {
		if _, ok := products.DescribeTheme(theme); !ok {
			seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Chủ đề affiliate không hợp lệ.")))
			return
		}
	}
	autopilot := r.PostFormValue("autopilot") == "1" || r.PostFormValue("autopilot") == "on"
	if autopilot && theme == "" {
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Chọn chủ đề affiliate trước khi bật Autopilot.")))
		return
	}

	verifyTikTokFollowers := r.PostFormValue("verify_tiktok_followers") == "1"
	tiktokFollowersRaw := strings.TrimSpace(r.PostFormValue("tiktok_followers"))
	var followers int64
	if verifyTikTokFollowers {
		value, err := strconv.ParseInt(tiktokFollowersRaw, 10, 64)
		if tiktokFollowersRaw == "" || err != nil || value < 0 {
			seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Nhập số TikTok followers hợp lệ và không âm.")))
			return
		}
		followers = value
	} else if tiktokFollowersRaw != "" {
		value, err := strconv.ParseInt(tiktokFollowersRaw, 10, 64)
		if err != nil || value != current.Followers || current.FollowersSource != network.FollowersSourceTikTokManual {
			seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Tick xác nhận TikTok trước khi lưu thay đổi số followers.")))
			return
		}
	}
	candidate := *current
	if verifyTikTokFollowers {
		candidate.Followers = followers
		candidate.FollowersSource = network.FollowersSourceTikTokManual
	}
	if status != current.Status && (status == "live_ready" || status == "live") && !candidate.TikTokLiveThresholdMet() {
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Cần xác minh ít nhất 1.000 TikTok followers trước khi chuyển sang trạng thái livestream.")))
		return
	}

	minCommRaw := strings.TrimSpace(r.PostFormValue("min_commission"))
	minComm := 0.10
	if minCommRaw != "" {
		v, err := strconv.ParseFloat(minCommRaw, 64)
		if err != nil || v < 0 || v > 100 {
			seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Ngưỡng hoa hồng phải từ 0 đến 100%.")))
			return
		}
		minComm = v / 100
	}

	_, err = s.Mgr.UpdateAccount(id, username, niche, nicheHint, persona, youtubeChannel, theme, autopilot, minComm)
	if err != nil {
		log.Printf("web: update account: %v", err)
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Lỗi khi lưu thông tin: "+err.Error())))
		return
	}
	if verifyTikTokFollowers {
		if err := s.Ledger.SetFollowers(id, followers); err != nil {
			log.Printf("web: verify TikTok followers: %v", err)
			seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Không lưu được số TikTok followers: "+err.Error())))
			return
		}
	}
	if status != current.Status {
		if _, err := s.Mgr.Transition(id, status, nil); err != nil {
			log.Printf("web: account edit transition: %v", err)
			seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Không chuyển được giai đoạn: "+err.Error())))
			return
		}
	}

	decision := map[string]any{
		"id": id, "username": username, "status": status, "niche": niche,
		"autopilot": autopilot, "tiktok_followers_verified": verifyTikTokFollowers,
	}
	if verifyTikTokFollowers {
		decision["tiktok_followers"] = followers
	}
	_ = s.Ledger.Decide("human", "account_edit", &username,
		fmt.Sprintf("Cập nhật thông tin kênh %s thành công", username), decision)

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
