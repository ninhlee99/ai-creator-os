package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

func (s *Server) handleTikTokCookieSave(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		seeOther(w, r, "/settings/system?enverr=Kho+Web+AI+chưa+sẵn+sàng#tab-social")
		return
	}
	cookie := strings.TrimSpace(r.FormValue("cookie"))
	if cookie == "" {
		seeOther(w, r, "/settings/system?enverr=Vui+lòng+nhập+chuỗi+cookie#tab-social")
		return
	}

	sessionKey := "manual_" + time.Now().Format("20060102150405")
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "sessionid=") {
			sessionKey = strings.TrimPrefix(part, "sessionid=")
			break
		}
	}

	cookieJar := webai.ParseCookieHeaderToBrowserCookies(cookie, ".tiktok.com")
	accountName := "TikTok (Thủ công)"
	_, err := s.WebAI.UpsertAccountSessionWithCookieJar(webai.PlatformTikTok, sessionKey, cookie, cookieJar, accountName, "manual", "unknown")
	if err != nil {
		seeOther(w, r, "/settings/system?enverr="+strings.ReplaceAll(err.Error(), " ", "+")+"#tab-social")
		return
	}

	seeOther(w, r, "/settings/system?ok=Đã+lưu+cookie+TikTok+thành+công#tab-social")
}

func (s *Server) handleTikTokCookieDelete(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		seeOther(w, r, "/settings/system#tab-social")
		return
	}
	accounts, _ := s.WebAI.GetAccounts(webai.PlatformTikTok)
	for _, acc := range accounts {
		_ = s.WebAI.DeleteAccount(acc.ID)
	}
	seeOther(w, r, "/settings/system?ok=Đã+xóa+cookie+TikTok#tab-social")
}
