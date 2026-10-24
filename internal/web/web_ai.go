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
	Platform        webai.PlatformInfo
	Accounts        []webai.Account
	ActiveCount     int
	ReadyCount      int
	StudioSupported bool
	LoginSupported  bool
	CooldownCount   int
	TotalCount      int
}

func webAIPlatformSupportsStudio(platform string) bool {
	return platform == webai.PlatformMuseAI || platform == webai.PlatformGoogleFlow
}

// webAIPlatformViews returns saved-session counts and the subset usable by Studio.
func (s *Server) webAIPlatformViews() ([]webAIView, int, int) {
	platforms := webai.SupportedPlatforms()
	loginTargets := webai.SupportedWebLoginTargets()
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
	studioReady := 0
	for _, p := range platforms {
		var accs []webai.Account
		if allAccounts != nil {
			accs = allAccounts[p.ID]
		}
		ready, cooldown, activeCount, total := 0, 0, 0, len(accs)
		for _, a := range accs {
			if a.IsActive {
				activeCount++
			}
			if a.IsActive && (a.Status == "ok" || a.Status == "connected") {
				ready++
			} else if a.IsActive && a.Status == "cooldown" {
				cooldown++
			}
		}
		totalConnected += activeCount
		studioSupported := webAIPlatformSupportsStudio(p.ID)
		_, loginSupported := loginTargets[p.ID]
		if studioSupported {
			studioReady += ready
		}
		views = append(views, webAIView{
			Platform:        p,
			Accounts:        accs,
			ActiveCount:     activeCount,
			ReadyCount:      ready,
			StudioSupported: studioSupported,
			LoginSupported:  loginSupported,
			CooldownCount:   cooldown,
			TotalCount:      total,
		})
	}
	return views, totalConnected, studioReady
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

// handleWebAIAccountDelete deletes an account from the pool.
func (s *Server) handleWebAIAccountDelete(w http.ResponseWriter, r *http.Request) {
	redirectBack := func(param, msg string) {
		target := resolveWebAIRedirect(r, "/settings/system#tab-webai")
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
		target := resolveWebAIRedirect(r, "/settings/system#tab-webai")
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
