package web

// Account pages: list, creation form, onboarding state machine and the
// per-account detail page with its action POSTs (transition, YouTube link,
// replan, live topic).

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
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
	s.render(w, "account_new", s.ctx(
		"Error", r.URL.Query().Get("err"),
		"HasYouTubeClient", publishers.NewYouTubePublisher("", nil, "").HasClient(),
		"HasTikTokClient", publishers.NewTikTokPublisher("").HasClient(),
	))
}

func (s *Server) handleAccountCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(32 << 20)
	username := strings.TrimSpace(r.PostFormValue("username"))
	username = strings.TrimLeft(username, "@")
	if username == "" {
		seeOther(w, r, "/accounts/new?err="+url.QueryEscape("Chưa nhập username."))
		return
	}
	// Đợt AA: username đã tồn tại thì báo rõ thay vì im lặng "nhận vơ"
	// kênh cũ (và trigger onboarding/plan mới cho kênh đang chạy).
	if s.accountExists(username) {
		seeOther(w, r, "/accounts/new?err="+url.QueryEscape("Kênh \""+username+"\" đã tồn tại."))
		return
	}
	// Đợt AA: form đã gỡ field rtmp_key_ref (đợt W) — truyền rỗng, giữ
	// signature Mgr.Add vì network daemon + ledger vẫn dùng cột RTMP
	// (live engine, không thuộc phạm vi dọn UI).
	acct, err := s.Mgr.Add(username,
		strings.TrimSpace(r.PostFormValue("niche_hint")),
		"")
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

	// Gán nhân vật đại diện cho kênh:
	if r.MultipartForm != nil {
		for _, key := range []string{"photo", "model_photo", "character_photo"} {
			if fhs, ok := r.MultipartForm.File[key]; ok && len(fhs) > 0 && fhs[0] != nil {
				fh := fhs[0]
				ext := strings.ToLower(filepath.Ext(fh.Filename))
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
					f, err := fh.Open()
					if err == nil {
						tmp, err := os.CreateTemp("", "account-model-*"+ext)
						if err == nil {
							_, _ = io.Copy(tmp, f)
							tmp.Close()
							_ = s.AssignCharacterToAccount(acct.ID, tmp.Name())
							_ = os.Remove(tmp.Name())
						}
						f.Close()
					}
				}
				break
			}
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
	if rawTab := r.URL.Query().Get("tab"); rawTab != "" {
		if tab := accountTabName(rawTab); tab != rawTab {
			q := r.URL.Query()
			q.Set("tab", tab)
			u := *r.URL
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}
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
	var brandDNA *growth.BrandDNA
	if s.Growth != nil {
		brandDNA, _ = s.Growth.GetBrandDNA(id)
		if brandDNA == nil {
			brandDNA = growth.DefaultBrandDNA(acct.Niche, id)
		}
	}

	detailCtx := s.ctx(
		"Acct", s.wrapAccount(acct),
		"Tab", s.accountTab(r),
		"Allowed", allowed,
		"BrandDNA", brandDNA,
		"Decisions", decisions,
		"Publishers", pubNames,
		"Gifts", gifts,
		"Themes", themeOptions(),
		"AutopilotReady", network.AutopilotReady(acct),
		"ModelPhotos", s.modelPhotoViews(id),
		"StudioOK", s.Studio != nil,
		"YTAnalytics", ytAnalyticsStatus(acct.Username, acct.YoutubeChannel),
		"PubStatus", s.publisherRowFor(acct.Username, acct.YoutubeChannel, acct.YoutubeContentTypes, acct.Status),
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

func (s *Server) handleAccountBrandDNASave(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse form")
		return
	}
	if s.Growth == nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Hệ thống Growth chưa sẵn sàng")))
		return
	}

	splitLines := func(val string) []string {
		var out []string
		for _, line := range strings.Split(val, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				out = append(out, line)
			}
		}
		return out
	}

	dna := &growth.BrandDNA{
		AccountID:       id,
		Positioning:     strings.TrimSpace(r.FormValue("positioning")),
		AudiencePersona: strings.TrimSpace(r.FormValue("audience_persona")),
		ValuePromise:    strings.TrimSpace(r.FormValue("value_promise")),
		ToneOfVoice:     strings.TrimSpace(r.FormValue("tone_of_voice")),
		HookRules:       splitLines(r.FormValue("hook_rules")),
		ContentPillars:  splitLines(r.FormValue("content_pillars")),
		VisualStyle:     strings.TrimSpace(r.FormValue("visual_style")),
		AudioStyle:      strings.TrimSpace(r.FormValue("audio_style")),
		RedLines:        splitLines(r.FormValue("red_lines")),
		QCChecklist:     splitLines(r.FormValue("qc_checklist")),
	}

	if err := s.Growth.SaveBrandDNA(dna); err != nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Lỗi khi lưu Brand DNA: "+err.Error())))
		return
	}

	seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&ok=%s", id, url.QueryEscape("Đã lưu Bản sắc thương hiệu (Brand DNA) thành công!")))
}

func (s *Server) handleAccountBrandDNAReset(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	acct, err := s.Mgr.Get(id)
	if err != nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Không tìm thấy kênh")))
		return
	}
	if s.Growth == nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Hệ thống Growth chưa sẵn sàng")))
		return
	}

	defaultDNA := growth.DefaultBrandDNA(acct.Niche, id)
	_ = s.Growth.SaveBrandDNA(defaultDNA)

	seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&ok=%s", id, url.QueryEscape("Đã khôi phục Brand DNA chuẩn AI theo ngành hàng!")))
}

// ---------------------------------------------------------------- comment auto-reply

func (s *Server) handleAccountCommentSimulate(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	acct, err := s.Mgr.Get(id)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không tìm thấy kênh"})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Dữ liệu không hợp lệ"})
		return
	}

	userComment := strings.TrimSpace(r.FormValue("comment"))
	if userComment == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Vui lòng nhập nội dung bình luận để thử nghiệm"})
		return
	}

	low := strings.ToLower(userComment)
	intent := "Tương tác cộng đồng"
	intentCode := "engagement"
	action := "Mô phỏng: gợi ý phản hồi tự nhiên"
	delaySec := 45 + (len(userComment)*7)%75

	var reply string
	// Simulation never invents a marketplace deep link or performs an action.
	affLink := ""

	isPurchase := strings.Contains(low, "giá") || strings.Contains(low, "bao nhiêu") ||
		strings.Contains(low, "mua") || strings.Contains(low, "link") ||
		strings.Contains(low, "shop") || strings.Contains(low, "ship") ||
		strings.Contains(low, "inbox") || strings.Contains(low, "tư vấn") ||
		strings.Contains(low, "đâu") || strings.Contains(low, "chỗ nào")

	isPraise := strings.Contains(low, "hay") || strings.Contains(low, "đẹp") ||
		strings.Contains(low, "đỉnh") || strings.Contains(low, "chuẩn") ||
		strings.Contains(low, "tuyệt") || strings.Contains(low, "thích")

	isToxic := strings.Contains(low, "lừa đảo") || strings.Contains(low, "dở") ||
		strings.Contains(low, "xấu") || strings.Contains(low, "nhảm")

	if isPurchase {
		intent = "Câu hỏi về sản phẩm"
		intentCode = "purchase"
		action = "Mô phỏng: gợi ý phản hồi về sản phẩm"
		if affLink == "" {
			affLink = "Chưa cấu hình link liên kết."
		}
		switch acct.Persona {
		case "genz", "playful":
			reply = fmt.Sprintf("Dạ sốp gửi mình link săn sale giảm tới 45%% kèm mã freeship ở giỏ hàng góc trái video hoặc bấm vào đây nha: %s ✨ Tranh thủ kẻo hết deal nè bác ơi!", affLink)
		case "expert", "tech":
			reply = fmt.Sprintf("Chào bạn, dòng này hiệu năng thực tế rất ổn trong tầm giá. Bạn có thể xem chi tiết thông số và nhận ưu đãi bảo hành chính hãng ở link giỏ hàng này nhé: %s", affLink)
		default:
			reply = fmt.Sprintf("Dạ em gửi mình link săn ưu đãi chính hãng kèm freeship ở giỏ hàng góc trái video hoặc bấm vào đây xem nhé: %s 💖 Cần tư vấn thêm bạn cứ nhắn em nha!", affLink)
		}
	} else if isPraise {
		intent = "Khen ngợi / Đồng tình"
		intentCode = "praise"
		action = "Mô phỏng: cảm ơn và gợi ý theo dõi"
		reply = "Cảm ơn bạn nhiều nha! Follow kênh để đón xem video hữu ích tiếp theo vào 19h tối mai nhé! 🔥"
	} else if isToxic {
		intent = "Tiêu cực / Cần kiểm duyệt (Bảo vệ thương hiệu)"
		intentCode = "toxic"
		action = "Mô phỏng: đề xuất kiểm duyệt"
		reply = "Bình luận cần được người vận hành xem xét."
	} else {
		intent = "Thắc mắc & Tranh luận (Kích thích Watch Time)"
		intentCode = "inquiry"
		action = "Mô phỏng: gợi ý câu trả lời mở"
		reply = "Theo bạn thì yếu tố nào quan trọng nhất? Cùng để lại bình luận để mình bàn luận thêm ở video sau nha! 👇"
	}

	s.writeJSON(w, map[string]any{
		"ok":          true,
		"intent":      intent,
		"intent_code": intentCode,
		"action":      action,
		"reply":       reply,
		"delay_sec":   delaySec,
		"comment":     userComment,
	})
}

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
