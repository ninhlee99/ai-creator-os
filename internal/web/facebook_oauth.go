package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	facebookOAuthHTTP = &http.Client{Timeout: 30 * time.Second}
	facebookGraphBase = "https://graph.facebook.com/v21.0"
	facebookPageIDRe  = regexp.MustCompile(`^[0-9]+$`)
)

type facebookPageOption struct {
	ID   string
	Name string
}

type facebookOAuthPage struct {
	facebookPageOption
	AccessToken string
}

type facebookPendingAccount struct {
	Username string
	Pages    []facebookOAuthPage
	Created  time.Time
}

func (s *Server) facebookOAuthCredentials() (string, string) {
	appID, _ := s.EffectiveEnv("META_APP_ID")
	secret, _ := s.EffectiveEnv("META_APP_SECRET")
	return strings.TrimSpace(appID), strings.TrimSpace(secret)
}

func (s *Server) facebookOAuthConfigured() bool {
	appID, secret := s.facebookOAuthCredentials()
	return appID != "" && secret != ""
}

func (s *Server) facebookRedirectURI(r *http.Request) (string, error) {
	if configured, ok := s.EffectiveEnv("META_REDIRECT_URI"); ok {
		u, err := url.Parse(strings.TrimSpace(configured))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "/publishers/facebook/callback" {
			return "", errors.New("META_REDIRECT_URI phải là URL hợp lệ kết thúc bằng /publishers/facebook/callback")
		}
		return u.String(), nil
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", fmt.Errorf("mở MiuNova qua http://127.0.0.1 rồi nối Facebook, hoặc đặt META_REDIRECT_URI")
	}
	return "http://" + r.Host + "/publishers/facebook/callback", nil
}

func (s *Server) handleFacebookAuthorize(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		username = "__new_facebook__"
	} else if !s.accountExists(username) {
		publishersMsg(w, r, "err", "Chọn kênh MiuNova hợp lệ trước khi nối Facebook.")
		return
	}
	appID, secret := s.facebookOAuthCredentials()
	if appID == "" || secret == "" {
		s.facebookOAuthError(w, r, username, "Nhập Meta App ID và App Secret trong Cài đặt → Hệ thống trước.")
		return
	}
	redirect, err := s.facebookRedirectURI(r)
	if err != nil {
		s.facebookOAuthError(w, r, username, err.Error())
		return
	}
	query := url.Values{
		"client_id":     {appID},
		"redirect_uri":  {redirect},
		"response_type": {"code"},
		"scope":         {"pages_show_list,pages_manage_posts,pages_read_engagement"},
		"state":         {s.oauth.issueWithRedirect(username, redirect)},
	}
	http.Redirect(w, r, "https://www.facebook.com/v21.0/dialog/oauth?"+query.Encode(), http.StatusFound)
}

func (s *Server) handleFacebookCallback(w http.ResponseWriter, r *http.Request) {
	state, ok := s.oauth.take(r.URL.Query().Get("state"))
	if !ok || state.username == "" || state.redirectURI == "" {
		publishersMsg(w, r, "err", "Facebook OAuth state không khớp hoặc đã hết hạn; hãy nối lại.")
		return
	}
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		s.facebookOAuthError(w, r, state.username, "Facebook từ chối quyền truy cập: "+providerError)
		return
	}
	if state.username != "__new_facebook__" && !s.accountExists(state.username) {
		s.facebookOAuthError(w, r, state.username, "Kênh không còn tồn tại; mở lại trang Kênh rồi nối Facebook.")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		s.facebookOAuthError(w, r, state.username, "Facebook không trả authorization code; hãy nối lại.")
		return
	}
	appID, secret := s.facebookOAuthCredentials()
	if appID == "" || secret == "" {
		s.facebookOAuthError(w, r, state.username, "Meta App ID hoặc App Secret chưa được cấu hình.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	userToken, err := facebookExchangeCode(ctx, appID, secret, state.redirectURI, code)
	if err != nil {
		s.facebookOAuthError(w, r, state.username, "Không đổi được Facebook authorization code; kiểm tra callback URL và cấu hình Meta App.")
		return
	}
	longToken, err := facebookLongLivedToken(ctx, appID, secret, userToken)
	if err != nil {
		s.facebookOAuthError(w, r, state.username, "Facebook không cấp được phiên dài hạn; thử nối lại.")
		return
	}
	pages, err := facebookManagedPages(ctx, longToken)
	if err != nil {
		s.facebookOAuthError(w, r, state.username, "Không đọc được danh sách Facebook Page; kiểm tra quyền pages_show_list.")
		return
	}
	if len(pages) == 0 {
		s.facebookOAuthError(w, r, state.username, "Tài khoản Facebook không quản lý Page nào có quyền cấp cho MiuNova.")
		return
	}
	if len(pages) == 1 {
		username, accountID, err := s.connectFacebookPage(state.username, pages[0])
		if err != nil {
			s.facebookOAuthError(w, r, state.username, "Không lưu được Facebook Page vào kênh.")
			return
		}
		if accountID > 0 {
			seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=connections&ok=%s", accountID, url.QueryEscape("Đã nối Facebook Page “"+pages[0].Name+"” cho "+username+".")))
			return
		}
		publishersMsg(w, r, "msg", "Đã nối Facebook Page “"+pages[0].Name+"” cho "+username+".")
		return
	}
	selectionID, err := randomFacebookState()
	if err != nil {
		s.facebookOAuthError(w, r, state.username, "Không tạo được phiên chọn Facebook Page; thử lại.")
		return
	}
	s.storeFacebookPending(selectionID, facebookPendingAccount{Username: state.username, Pages: pages, Created: time.Now()})
	choices := make([]facebookPageOption, 0, len(pages))
	for _, page := range pages {
		choices = append(choices, page.facebookPageOption)
	}
	s.render(w, "facebook_pages", s.ctx("Username", state.username, "State", selectionID, "Pages", choices))
}

func (s *Server) handleFacebookPageSelect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		publishersMsg(w, r, "err", "Không đọc được lựa chọn Facebook Page.")
		return
	}
	state := strings.TrimSpace(r.PostFormValue("state"))
	pageID := strings.TrimSpace(r.PostFormValue("page_id"))
	pending, ok := s.takeFacebookPending(state)
	if !ok || !facebookPageIDRe.MatchString(pageID) {
		publishersMsg(w, r, "err", "Phiên chọn Facebook Page đã hết hạn; hãy nối lại.")
		return
	}
	for _, page := range pending.Pages {
		if page.ID == pageID {
			username, accountID, err := s.connectFacebookPage(pending.Username, page)
			if err != nil {
				s.facebookOAuthError(w, r, pending.Username, "Không lưu được Facebook Page vào kênh.")
				return
			}
			if accountID > 0 {
				seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=connections&ok=%s", accountID, url.QueryEscape("Đã nối Facebook Page “"+page.Name+"” cho "+username+".")))
				return
			}
			publishersMsg(w, r, "msg", "Đã nối Facebook Page “"+page.Name+"” cho "+username+".")
			return
		}
	}
	publishersMsg(w, r, "err", "Facebook Page đã chọn không thuộc phiên OAuth này.")
}

func (s *Server) facebookOAuthError(w http.ResponseWriter, r *http.Request, username, message string) {
	if username == "__new_facebook__" {
		seeOther(w, r, "/accounts/new?err="+url.QueryEscape(message))
		return
	}
	publishersMsg(w, r, "err", message)
}

// connectFacebookPage creates an AICOS account when OAuth starts from the
// new-channel page, then stores page credentials in that account's settings.
func (s *Server) connectFacebookPage(username string, page facebookOAuthPage) (string, int64, error) {
	if username != "__new_facebook__" {
		if err := s.saveFacebookPage(username, page); err != nil {
			return "", 0, err
		}
		return username, 0, nil
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		return "", 0, err
	}
	for _, account := range accounts {
		if configuredID, ok := s.EffectiveEnv(accountEnvName("FB_PAGE_ID", account.Username)); ok && configuredID == page.ID {
			if err := s.saveFacebookPage(account.Username, page); err != nil {
				return "", 0, err
			}
			return account.Username, account.ID, nil
		}
	}
	username = "facebook_" + page.ID
	base := username
	for suffix := 2; s.accountExists(username); suffix++ {
		username = fmt.Sprintf("%s_%d", base, suffix)
	}
	account, err := s.Mgr.Add(username, "Facebook Page: "+page.Name, "")
	if err != nil {
		return "", 0, err
	}
	if err := s.saveFacebookPage(username, page); err != nil {
		return "", 0, err
	}
	s.advanceOnboarding(account.ID)
	s.autoGeneratePlan(account, "Tự sinh cho Facebook Page: "+page.Name)
	return username, account.ID, nil
}

func randomFacebookState() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func (s *Server) storeFacebookPending(id string, value facebookPendingAccount) {
	s.facebookPendingMu.Lock()
	defer s.facebookPendingMu.Unlock()
	if s.facebookPending == nil {
		s.facebookPending = map[string]facebookPendingAccount{}
	}
	for key, pending := range s.facebookPending {
		if time.Since(pending.Created) > oauthStateTTL {
			delete(s.facebookPending, key)
		}
	}
	s.facebookPending[id] = value
}

func (s *Server) takeFacebookPending(id string) (facebookPendingAccount, bool) {
	s.facebookPendingMu.Lock()
	defer s.facebookPendingMu.Unlock()
	value, ok := s.facebookPending[id]
	delete(s.facebookPending, id)
	return value, ok && time.Since(value.Created) <= oauthStateTTL
}

func (s *Server) saveFacebookPage(username string, page facebookOAuthPage) error {
	if !s.accountExists(username) || !facebookPageIDRe.MatchString(page.ID) || page.AccessToken == "" {
		return errors.New("invalid Facebook page credentials")
	}
	if err := s.saveAccountEnv(username, "META_PAGE_TOKEN", page.AccessToken); err != nil {
		return err
	}
	if err := s.saveAccountEnv(username, "FB_PAGE_ID", page.ID); err != nil {
		_ = s.saveAccountEnv(username, "META_PAGE_TOKEN", "")
		return err
	}
	return nil
}

func (s *Server) saveAccountEnv(username, key, value string) error {
	if key != "FB_PAGE_ID" && key != "META_PAGE_TOKEN" {
		return errors.New("unsupported account environment key")
	}
	name := accountEnvName(key, username)
	if s.Ledger != nil {
		if err := s.Ledger.SetSetting(envSettingKey(name), value); err != nil {
			return err
		}
	}
	if value == "" {
		os.Unsetenv(name)
	} else {
		os.Setenv(name, value)
	}
	return nil
}

func accountEnvName(key, username string) string {
	var suffix strings.Builder
	for _, r := range username {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			suffix.WriteRune(r)
		} else {
			suffix.WriteRune('_')
		}
	}
	return key + "_" + strings.ToUpper(suffix.String())
}
