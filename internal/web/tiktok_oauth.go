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
	verifier string // PKCE code_verifier for TikTok Desktop Login Kit
	at       time.Time
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

// take consumes a fresh state, returning who started it (and the PKCE
// verifier). ok is false for unknown, reused or expired states.
func (o *oauthStates) take(state string) (oauthState, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.m[state]
	delete(o.m, state)
	return v, ok && time.Since(v.at) <= oauthStateTTL
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
	if err := publishers.ValidateTikTokDesktopRedirectURI(); err != nil {
		publishersMsg(w, r, "err", err.Error())
		return
	}
	verifier, challenge := tiktok.NewPKCE()
	http.Redirect(w, r, p.AuthorizeURL(s.oauth.issue(username, verifier), challenge), http.StatusFound)
}

// handleTikTokCallback receives the code/state directly at the Desktop
// loopback redirect URI after the creator grants access.
func (s *Server) handleTikTokCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		publishersMsg(w, r, "err", "TikTok từ chối: "+e+" "+q.Get("error_description"))
		return
	}
	s.finishTikTokConnect(w, r, "", q.Get("code"), q.Get("state"))
}

// finishTikTokConnect validates state, exchanges the code and writes the token.
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
	if err := publishers.ValidateTikTokDesktopRedirectURI(); err != nil {
		publishersMsg(w, r, "err", err.Error())
		return
	}
	targetUsername := st.username
	isNewAccount := targetUsername == "__new_tiktok__"
	var targetAccountID int64
	if isNewAccount {
		for attempt := 0; attempt < 8; attempt++ {
			var suffix [16]byte
			if _, err := rand.Read(suffix[:]); err != nil {
				publishersMsg(w, r, "err", "Không tạo được mã kênh TikTok an toàn: "+err.Error())
				return
			}
			candidateName := "tiktok_creator_" + hex.EncodeToString(suffix[:])
			if s.accountExists(candidateName) {
				continue
			}
			acct, err := s.Mgr.Add(candidateName, "kênh TikTok thật đã xác thực", "")
			if err != nil {
				publishersMsg(w, r, "err", "Không tạo được kênh từ TikTok: "+err.Error())
				return
			}
			if acct.Username != candidateName {
				continue
			}
			targetUsername = candidateName
			targetAccountID = acct.ID
			break
		}
		if targetAccountID == 0 {
			publishersMsg(w, r, "err", "Không tạo được tên kênh TikTok mới; thử kết nối lại.")
			return
		}
	}
	_, err := publishers.NewTikTokPublisher(targetUsername).ExchangeCode(code, st.verifier)
	if err != nil {
		if targetAccountID > 0 {
			if cleanupErr := s.Mgr.DeleteAccount(targetAccountID); cleanupErr != nil {
				err = fmt.Errorf("%w; dọn kênh tạo dở thất bại: %v", err, cleanupErr)
			}
		}
		publishersMsg(w, r, "err", "Đổi code lấy token thất bại: "+err.Error())
		return
	}
	if s.Ledger != nil {
		_ = s.Ledger.DeleteSetting(tiktokTokenDeadKey(targetUsername))
	}
	if targetAccountID == 0 {
		accounts, listErr := s.Mgr.List()
		if listErr == nil {
			for _, acct := range accounts {
				if acct.Username == targetUsername {
					targetAccountID = acct.ID
					break
				}
			}
		}
	}
	if targetAccountID > 0 {
		if isNewAccount {
			if acct, getErr := s.Mgr.Get(targetAccountID); getErr == nil {
				s.advanceOnboarding(acct.ID)
				s.autoGeneratePlan(acct, "Tự sinh cho kênh TikTok thật")
			}
		}
		seeOther(w, r, "/accounts?ok="+url.QueryEscape("Đã kết nối thành công kênh TikTok "+targetUsername+"!"))
		return
	}
	seeOther(w, r, "/accounts?err="+url.QueryEscape("TikTok đã xác thực nhưng không tìm thấy kênh @"+targetUsername+" trong danh sách kênh."))
}

func tiktokTokenDeadKey(username string) string { return "tiktok_token_dead:" + username }
