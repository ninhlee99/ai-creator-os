package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
	"net/http"
	"net/url"
	"strings"
)

// webAIView holds template projection for Web AI Hub.
// webAIView holds template projection for Web AI Hub with multi-account keyring.
type webAIView struct {
	Platform          webai.PlatformInfo
	Accounts          []webai.Account
	HasGoogleAccounts bool
	Primary           webai.Account
	ReadyCount        int
	CooldownCount     int
	TotalCount        int
	HasAccounts       bool
	IsConnected       bool
}

// webAIPlatformViews computes template views for each platform and the total ready accounts count.
func (s *Server) webAIPlatformViews() ([]webAIView, int) {
	platforms := webai.SupportedPlatforms()
	var allAccounts map[string][]webai.Account
	if s.WebAI != nil {
		var err error
		allAccounts, err = s.WebAI.GetAllAccounts()
		if err != nil {
			logErr("webai: GetAllAccounts: %v", err)
		}
	}

	views := make([]webAIView, 0, len(platforms))
	totalConnected := 0
	for _, p := range platforms {
		var accs []webai.Account
		if allAccounts != nil {
			accs = allAccounts[p.ID]
		}
		ready, cooldown, total := 0, 0, len(accs)
		var primary webai.Account
		for _, a := range accs {
			if a.IsActive && a.IsConfirmedFree() && (a.Status == "ok" || a.Status == "connected") {
				ready++
			} else if a.IsActive && a.IsConfirmedFree() && a.Status == "cooldown" {
				cooldown++
			}
			if primary.ID == "" || a.IsActive {
				primary = a
			}
		}
		if primary.ID == "" {
			primary = webai.Account{
				Platform: p.ID,
				Status:   webai.StatusDisconnected,
				Tier:     "unknown",
			}
		}
		totalConnected += ready
		hasFreeGoogle := false
		for _, account := range allAccounts[webai.PlatformGemini] {
			if account.IsActive && account.IsConfirmedFree() && (account.Status == "ok" || account.Status == "connected") {
				hasFreeGoogle = true
				break
			}
		}
		views = append(views, webAIView{
			Platform:          p,
			Accounts:          accs,
			HasGoogleAccounts: hasFreeGoogle,
			Primary:           primary,
			ReadyCount:        ready,
			CooldownCount:     cooldown,
			TotalCount:        total,
			HasAccounts:       total > 0,
			IsConnected:       ready > 0,
		})
	}
	return views, totalConnected
}

func resolveWebAIRedirect(r *http.Request, defaultTarget string) string {
	back := strings.TrimSpace(r.FormValue("back"))
	if back != "" && strings.HasPrefix(back, "/") && !strings.HasPrefix(back, "//") {
		return back
	}
	ref := r.Header.Get("Referer")
	if strings.Contains(ref, "/settings") {
		return "/settings/system#tab-webai"
	}
	return defaultTarget
}

// handleWebAI renders the AI Web Hub page with multi-account rotation support.
func (s *Server) handleWebAI(w http.ResponseWriter, r *http.Request) {
	views, totalConnected := s.webAIPlatformViews()

	// Active tab inside the embedded workspace
	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = webai.PlatformChatGPT
	}

	s.render(w, "web_ai", s.ctx(
		"Title", "AI Web Hub — MiuNova",
		"Platforms", views,
		"ConnectedCount", totalConnected,
		"ActiveTab", activeTab,
		"Notice", r.URL.Query().Get("ok"),
		"Error", r.URL.Query().Get("err"),
	))
}

// handleWebAIAccountAdd rejects manually supplied sessions. Web sessions must
// enter the pool through the browser-login flow, which validates cookie and identity.
func (s *Server) handleWebAIAccountAdd(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/web-ai")
		frag := ""
		if idx := strings.Index(target, "#"); idx >= 0 {
			frag = target[idx:]
			target = target[:idx]
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		seeOther(w, r, target+sep+param+"="+url.QueryEscape(msg)+frag)
	}

	redirectBack("err", "Hãy đăng nhập bằng trình duyệt để hệ thống xác minh email và cookie phiên hoạt động.")
}

// handleWebAIAccountDelete deletes an account from the pool.
func (s *Server) handleWebAIAccountDelete(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/web-ai")
		frag := ""
		if idx := strings.Index(target, "#"); idx >= 0 {
			frag = target[idx:]
			target = target[:idx]
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		seeOther(w, r, target+sep+param+"="+url.QueryEscape(msg)+frag)
	}

	if s.WebAI == nil {
		redirectBack("err", "Kho Web AI chưa được khởi tạo")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		redirectBack("err", "Thiếu ID tài khoản")
		return
	}
	if err := s.WebAI.DeleteAccount(id); err != nil {
		redirectBack("err", "Lỗi xóa tài khoản: "+err.Error())
		return
	}
	redirectBack("ok", "Đã xóa tài khoản khỏi pool xoay tua")
}

// handleWebAIAccountResetCooldown manually clears cooldown for an account.
func (s *Server) handleWebAIAccountResetCooldown(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/web-ai")
		frag := ""
		if idx := strings.Index(target, "#"); idx >= 0 {
			frag = target[idx:]
			target = target[:idx]
		}
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		seeOther(w, r, target+sep+param+"="+url.QueryEscape(msg)+frag)
	}

	if s.WebAI == nil {
		redirectBack("err", "Kho Web AI chưa được khởi tạo")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		redirectBack("err", "Thiếu ID tài khoản")
		return
	}
	if err := s.WebAI.ResetCooldown(id); err != nil {
		redirectBack("err", "Lỗi đặt lại cooldown: "+err.Error())
		return
	}
	redirectBack("ok", "Đã đặt lại trạng thái — tài khoản đã sẵn sàng hoạt động trở lại!")
}

func logErr(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}
