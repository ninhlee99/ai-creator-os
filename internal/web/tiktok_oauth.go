package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/tiktok"
)

// oauthStates remembers the CSRF state handed to each TikTok authorize
// redirect, so a pasted redirect URL is only accepted for the account
// that started it. Entries expire after 15 minutes (TikTok codes live
// far shorter anyway).
type oauthStates struct {
	mu sync.Mutex
	m  map[string]oauthState
}

type oauthState struct {
	username string
	verifier string // PKCE code_verifier ("" for the Web flow)
	// redirectURI: callback URL đã dùng lúc authorize (YouTube yêu cầu khớp
	// y hệt lúc đổi code; lưu ở đây để không phụ thuộc Host lúc callback).
	redirectURI string
	at          time.Time
}

const oauthStateTTL = 15 * time.Minute

func (o *oauthStates) issue(username, verifier string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	st := hex.EncodeToString(b)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.m == nil {
		o.m = map[string]oauthState{}
	}
	for k, v := range o.m {
		if time.Since(v.at) > oauthStateTTL {
			delete(o.m, k)
		}
	}
	o.m[st] = oauthState{username: username, verifier: verifier, at: time.Now()}
	return st
}

// issueWithRedirect như issue nhưng nhớ thêm redirect URI (dùng cho
// YouTube OAuth: Google yêu cầu redirect_uri lúc đổi code khớp y hệt
// lúc authorize).
func (o *oauthStates) issueWithRedirect(username, redirectURI string) string {
	st := o.issue(username, "")
	o.mu.Lock()
	defer o.mu.Unlock()
	v := o.m[st]
	v.redirectURI = redirectURI
	o.m[st] = v
	return st
}

// take consumes a fresh state, returning who started it (and the PKCE
// verifier). ok is false for unknown, reused or expired states.
func (o *oauthStates) take(state string) (oauthState, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.m[state]
	delete(o.m, state)
	return v, ok && time.Since(v.at) <= oauthStateTTL
}

// parseOAuthRedirect pulls code/state/error out of whatever was pasted:
// the full redirect URL, or just the query string.
func parseOAuthRedirect(pasted string) (code, state, oerr string) {
	pasted = strings.TrimSpace(pasted)
	raw := pasted
	if i := strings.Index(pasted, "?"); i >= 0 {
		raw = pasted[i+1:]
	}
	if i := strings.Index(raw, "#"); i >= 0 {
		raw = raw[:i]
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "", "", "không đọc được URL đã dán"
	}
	if e := q.Get("error"); e != "" {
		return "", "", e + " " + q.Get("error_description")
	}
	return q.Get("code"), q.Get("state"), ""
}

func (s *Server) accountExists(username string) bool {
	accounts, err := s.Mgr.List()
	if err != nil {
		return false
	}
	for _, a := range accounts {
		if a.Username == username {
			return true
		}
	}
	return false
}

func publishersMsg(w http.ResponseWriter, r *http.Request, key, text string) {
	seeOther(w, r, "/publishers?"+url.Values{key: {text}}.Encode())
}

// handleTikTokAuthorize sends the browser (already logged in to TikTok)
// to the Login Kit consent page for one account or to onboard a new real account.
func (s *Server) handleTikTokAuthorize(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	fromNew := false
	if username == "" {
		username = "__new_tiktok__"
		fromNew = true
	} else if !s.accountExists(username) {
		publishersMsg(w, r, "err", "Không có tài khoản "+username)
		return
	}
	p := publishers.NewTikTokPublisher(username)
	if !p.HasClient() {
		msg := "Thiếu Client Key/Secret TikTok — nhập ở Cài đặt · Hệ thống, mục Mã & biến cấu hình."
		if fromNew {
			seeOther(w, r, "/accounts/new?err="+url.QueryEscape(msg))
			return
		}
		publishersMsg(w, r, "err", msg)
		return
	}
	// PKCE always: required by TikTok's Desktop platform, harmless on Web
	// (the verifier always matches the challenge we sent).
	verifier, challenge := tiktok.NewPKCE()
	http.Redirect(w, r, p.AuthorizeURL(s.oauth.issue(username, verifier), challenge), http.StatusFound)
}

// handleTikTokCallback receives code/state after Allow: directly (local
// redirect URI) or forwarded by the https relay page
// docs/tiktok-app-site/callback.html, since TikTok rejects http redirects.
func (s *Server) handleTikTokCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		publishersMsg(w, r, "err", "TikTok từ chối: "+e+" "+q.Get("error_description"))
		return
	}
	s.finishTikTokConnect(w, r, "", q.Get("code"), q.Get("state"))
}

// handleTikTokConnect takes the pasted redirect URL, exchanges the code
// and writes tiktok_token_<username>.json. Tokens never reach the page.
func (s *Server) handleTikTokConnect(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	username := r.PostFormValue("username")
	if !s.accountExists(username) {
		publishersMsg(w, r, "err", "Không có tài khoản "+username)
		return
	}
	code, state, oerr := parseOAuthRedirect(r.PostFormValue("redirect"))
	if oerr != "" {
		publishersMsg(w, r, "err", "TikTok từ chối: "+oerr)
		return
	}
	s.finishTikTokConnect(w, r, username, code, state)
}

// finishTikTokConnect validates state (and, for the paste form, that it was
// issued for the chosen account), exchanges the code and writes the token.
func (s *Server) finishTikTokConnect(w http.ResponseWriter, r *http.Request, username, code, state string) {
	if code == "" {
		publishersMsg(w, r, "err", "Không có ?code= — bấm lại “Kết nối TikTok”.")
		return
	}
	st, ok := s.oauth.take(state)
	if !ok || (username != "" && st.username != username) {
		publishersMsg(w, r, "err", "state không khớp hoặc đã hết hạn (15 phút) — bấm lại “Kết nối TikTok”.")
		return
	}
	targetUsername := st.username
	var targetAccountID int64
	if targetUsername == "__new_tiktok__" {
		candidateName := fmt.Sprintf("tiktok_creator_%d", time.Now().Unix()%10000)
		acct, err := s.Mgr.Add(candidateName, "kênh TikTok thật đã xác thực", "")
		if err != nil {
			publishersMsg(w, r, "err", "Không tạo được kênh từ TikTok: "+err.Error())
			return
		}
		targetUsername = candidateName
		targetAccountID = acct.ID
		s.advanceOnboarding(acct.ID)
		s.autoGeneratePlan(acct, "Tự sinh cho kênh TikTok thật")
	}
	scope, err := publishers.NewTikTokPublisher(targetUsername).ExchangeCode(code, st.verifier)
	if err != nil {
		publishersMsg(w, r, "err", "Đổi code lấy token thất bại: "+err.Error())
		return
	}
	if s.Ledger != nil {
		_ = s.Ledger.DeleteSetting(tiktokTokenDeadKey(targetUsername))
	}
	if targetAccountID > 0 {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=ket-noi&ok=%s", targetAccountID, url.QueryEscape("Đã kết nối thành công kênh TikTok "+targetUsername+"!")))
		return
	}
	publishersMsg(w, r, "msg", "Đã kết nối TikTok cho "+targetUsername+" (scope: "+scope+"). Token lưu ở "+publishers.TikTokTokenPath(targetUsername)+".")
}

func tiktokTokenDeadKey(username string) string { return "tiktok_token_dead:" + username }

