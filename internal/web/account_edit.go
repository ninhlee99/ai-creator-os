package web

import (
	"database/sql"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func editableAccountStatuses(current string) []string {
	order := []string{
		"onboarding", "researching", "persona_assigned", "growing",
		"paused", "penalized", "retired",
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
	persona := current.Persona
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = current.Status
	}
	if status != current.Status && !network.TRANSITIONS[current.Status][status] {
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Không thể chuyển tài khoản sang giai đoạn này theo quy trình hiện tại.")))
		return
	}
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

	_, err = s.Mgr.UpdateAccount(id, username, niche, nicheHint, persona, theme, autopilot, minComm)
	if err != nil {
		log.Printf("web: update account: %v", err)
		seeOther(w, r, fmt.Sprintf("/accounts/%d/edit?err=%s", id, url.QueryEscape("Lỗi khi lưu thông tin: "+err.Error())))
		return
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
		"autopilot": autopilot,
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
