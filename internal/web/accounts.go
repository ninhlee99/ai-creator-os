package web

import (
	"database/sql"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
		"HasMetaClient", s.facebookOAuthConfigured(),
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
	brandDNAStatus := "Growth store chưa sẵn sàng; không đọc được hồ sơ."
	if s.Growth != nil {
		storedDNA, dnaErr := s.Growth.GetBrandDNA(id)
		if dnaErr != nil {
			log.Printf("web: load Brand DNA for account %d: %v", id, dnaErr)
			brandDNAStatus = "Không đọc được hồ sơ đã lưu; mẫu gợi ý chưa được nạp."
		} else if storedDNA == nil {
			brandDNA = growth.DefaultBrandDNA(acct.Niche, id)
			brandDNAStatus = "Đây là mẫu gợi ý theo ngách; chưa được lưu cho kênh."
		} else {
			brandDNA = storedDNA
			brandDNAStatus = "Hồ sơ đã lưu cho kênh."
		}
	}

	detailCtx := s.ctx(
		"Acct", s.wrapAccount(acct),
		"Tab", s.accountTab(r),
		"Allowed", allowed,
		"BrandDNA", brandDNA,
		"BrandDNAStatus", brandDNAStatus,
		"Decisions", decisions,
		"Publishers", pubNames,
		"Gifts", gifts,
		"Themes", themeOptions(),
		"AutopilotConfigured", network.AutopilotReady(acct),
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
