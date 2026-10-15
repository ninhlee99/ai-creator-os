package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

// webAIView holds template projection for Web AI Hub.
// webAIView holds template projection for Web AI Hub with multi-account keyring.
type webAIView struct {
	Platform      webai.PlatformInfo
	Accounts      []webai.Account
	Primary       webai.Account
	ReadyCount    int
	CooldownCount int
	TotalCount    int
	HasAccounts   bool
	IsConnected   bool
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
			if a.Status == "ok" || a.Status == "connected" {
				ready++
			} else if a.Status == "cooldown" {
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
				Tier:     "free",
			}
		}
		totalConnected += ready
		views = append(views, webAIView{
			Platform:      p,
			Accounts:      accs,
			Primary:       primary,
			ReadyCount:    ready,
			CooldownCount: cooldown,
			TotalCount:    total,
			HasAccounts:   total > 0,
			IsConnected:   ready > 0,
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
		return "/settings/he-thong#tab-webai"
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
		"Title", "AI Web Hub — AI Creator OS",
		"Platforms", views,
		"ConnectedCount", totalConnected,
		"ActiveTab", activeTab,
		"Notice", r.URL.Query().Get("ok"),
		"Error", r.URL.Query().Get("err"),
	))
}

// handleWebAIAccountAdd adds an account to the pool for a platform.
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

	if s.WebAI == nil {
		redirectBack("err", "Kho Web AI chưa được khởi tạo")
		return
	}
	platform := strings.TrimSpace(r.FormValue("platform"))
	sessionKey := strings.TrimSpace(r.FormValue("session_key"))
	cookie := strings.TrimSpace(r.FormValue("cookie"))
	accountName := strings.TrimSpace(r.FormValue("account_name"))
	email := strings.TrimSpace(r.FormValue("email"))
	tier := strings.TrimSpace(r.FormValue("tier"))
	if tier == "" {
		tier = "free"
	}

	if sessionKey == "" && cookie == "" {
		redirectBack("err", "Vui lòng nhập Session Key, Cookie hoặc Token phiên làm việc.")
		return
	}

	if accountName == "" {
		if email != "" {
			accountName = email
		} else {
			accountName = fmt.Sprintf("%s acc %d", platform, time.Now().Unix()%1000)
		}
	}

	acc := webai.Account{
		Platform:    platform,
		AccountName: accountName,
		Email:       email,
		SessionKey:  sessionKey,
		Cookie:      cookie,
		Status:      "ok",
		Tier:        tier,
		LastChecked: time.Now().UTC(),
	}

	if _, err := s.WebAI.AddAccount(acc); err != nil {
		redirectBack("err", "Lỗi thêm tài khoản: "+err.Error())
		return
	}

	redirectBack("ok", "Đã thêm tài khoản web "+platform+" vào pool xoay tua thành công!")
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

// handleWebAIAccountTest validates account session credentials format.
func (s *Server) handleWebAIAccountTest(w http.ResponseWriter, r *http.Request) {
	if s.WebAI == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Kho Web AI chưa sẵn sàng"})
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	acc, err := s.WebAI.GetAccount(id)
	if err != nil || acc == nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không tìm thấy tài khoản"})
		return
	}
	// Check credentials presence
	if strings.TrimSpace(acc.SessionKey) == "" && strings.TrimSpace(acc.Cookie) == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Thiếu session key / cookie"})
		return
	}
	s.writeJSON(w, map[string]any{"ok": true, "status": acc.Status, "message": "Phiên làm việc hợp lệ ✓"})
}

// handleWebAIConnect maintains compatibility with legacy single-account connect calls.
func (s *Server) handleWebAIConnect(w http.ResponseWriter, r *http.Request) {
	s.handleWebAIAccountAdd(w, r)
}

// handleWebAIDisconnect disconnects an account and clears credentials.
func (s *Server) handleWebAIDisconnect(w http.ResponseWriter, r *http.Request) {
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
	platform := strings.TrimSpace(r.FormValue("platform"))
	if err := s.WebAI.DisconnectAccount(platform); err != nil {
		redirectBack("err", "Lỗi ngắt kết nối: "+err.Error())
		return
	}
	redirectBack("ok", "Đã ngắt kết nối "+platform)
}


// handleWebAIPromptGenerate returns synthesized prompts for Claude, ChatGPT, Gemini, and Muse AI.
func (s *Server) handleWebAIPromptGenerate(w http.ResponseWriter, r *http.Request) {
	productName := r.FormValue("product_name")
	niche := r.FormValue("niche")
	framework := r.FormValue("framework")
	targetAudience := r.FormValue("target_audience")
	keyBenefits := r.FormValue("key_benefits")
	cta := r.FormValue("cta")

	pkg := webai.GenerateAdScriptPackage(productName, niche, framework, targetAudience, keyBenefits, cta)
	s.writeJSON(w, pkg)
}

// handleWebAIProxy delegates requests to the ProxyRelay.
func (s *Server) handleWebAIProxy(w http.ResponseWriter, r *http.Request) {
	if s.WebAIRelay == nil {
		if s.WebAI != nil {
			s.WebAIRelay = webai.NewProxyRelay(s.WebAI)
		} else {
			http.Error(w, "Web AI relay not configured", http.StatusServiceUnavailable)
			return
		}
	}
	s.WebAIRelay.ServeHTTP(w, r)
}

// handleStudioAdCreative queues a high quality ad creative video job leveraging Muse AI clips,
// Gemini Imagen 3 photos, and AI voiceover.
func (s *Server) handleStudioAdCreative(w http.ResponseWriter, r *http.Request) {
	studioErr := func(msg string) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(msg))
	}
	if s.Studio == nil {
		studioErr("Studio chưa sẵn sàng — thử tải lại trang.")
		return
	}
	if err := r.ParseMultipartForm(120 << 20); err != nil {
		if perr := r.ParseForm(); perr != nil {
			studioErr("Không đọc được form — thử lại.")
			return
		}
	}

	upDir := s.Studio.UploadDir()
	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		studioErr("Ảnh sản phẩm lỗi: " + err.Error())
		return
	}
	modelPhoto, err := saveUpload(r, "model_photo", upDir)
	if err != nil {
		studioErr("Ảnh người mẫu lỗi: " + err.Error())
		return
	}
	extraPhotos, err := saveUploads(r, "extra_photos", upDir, 8)
	if err != nil {
		studioErr("Ảnh phụ lỗi: " + err.Error())
		return
	}

	// Download photos from URLs (e.g. Gemini Imagen 3, Web CDN)
	rawPhotoURLs := strings.TrimSpace(r.FormValue("photo_urls"))
	if rawPhotoURLs != "" {
		for i, line := range strings.Split(rawPhotoURLs, "\n") {
			u := strings.TrimSpace(line)
			if u == "" || !strings.HasPrefix(u, "http") {
				continue
			}
			dst := filepath.Join(upDir, fmt.Sprintf("%d-urlphoto-%d.png", time.Now().UnixNano(), i))
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
			if err := studio.DownloadImage(ctx, u, dst); err == nil {
				if productPhoto == "" {
					productPhoto = dst
				} else if len(extraPhotos) < 8 {
					extraPhotos = append(extraPhotos, dst)
				}
			}
			cancel()
		}
	}

	// Save video clips (from Muse AI or uploaded)
	var clips []string
	if r.MultipartForm != nil {
		for i, hdr := range r.MultipartForm.File["clip_files"] {
			if i >= 8 || hdr.Size == 0 {
				continue
			}
			ext := strings.ToLower(filepath.Ext(hdr.Filename))
			if ext != ".mp4" && ext != ".mov" && ext != ".webm" {
				continue
			}
			f, err := hdr.Open()
			if err != nil {
				continue
			}
			dst := filepath.Join(upDir, fmt.Sprintf("%d-clip-%d%s", time.Now().UnixNano(), i, ext))
			out, err := os.Create(dst)
			if err == nil {
				_, _ = io.Copy(out, f)
				out.Close()
				clips = append(clips, dst)
			}
			f.Close()
		}
	}

	// Download video clips from URLs (from Muse AI or CDN)
	rawClipURLs := strings.TrimSpace(r.FormValue("clip_urls"))
	if rawClipURLs != "" {
		for i, line := range strings.Split(rawClipURLs, "\n") {
			u := strings.TrimSpace(line)
			if u == "" || !strings.HasPrefix(u, "http") {
				continue
			}
			if len(clips) >= 8 {
				break
			}
			dst := filepath.Join(upDir, fmt.Sprintf("%d-urlclip-%d.mp4", time.Now().UnixNano(), i))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			if err := studio.DownloadVideo(ctx, u, dst); err == nil {
				clips = append(clips, dst)
			}
			cancel()
		}
	}

	// Save Voiceover audio or synthesize from voiceover_text
	voiceoverText := strings.TrimSpace(r.FormValue("voiceover_text"))
	voiceoverPath, _ := saveUpload(r, "voiceover_file", upDir)
	if voiceoverPath == "" && voiceoverText != "" && s.TTS != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		if audioBytes, err := s.TTS.Synthesize(ctx, voiceoverText, "default"); err == nil && len(audioBytes) > 0 {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-voiceover.mp3", time.Now().UnixNano()))
			if werr := os.WriteFile(dst, audioBytes, 0o644); werr == nil {
				voiceoverPath = dst
			}
		}
		cancel()
	}

	// Save Music
	musicPath, _ := saveUpload(r, "music_file", upDir)
	if musicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			defer cancel()
			if derr := studio.DownloadAudio(ctx, u, dst); derr == nil {
				musicPath = dst
			}
		}
	}

	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	seconds = clampInt(seconds, 10, 120)
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))

	bpm = clampInt(bpm, 60, 200)
	musicStart, _ := strconv.ParseFloat(r.FormValue("music_start"), 64)

	productName := strings.TrimSpace(r.FormValue("product_name"))
	if productName == "" {
		productName = "Video quảng cáo sản phẩm"
	}

	aspectRatio := strings.TrimSpace(r.FormValue("aspect_ratio"))
	if aspectRatio != "16:9" {
		aspectRatio = "9:16"
	}

	p := studio.AffiliateParams{
		Mode:          r.FormValue("mode"),
		Niche:         strings.TrimSpace(r.FormValue("niche")),
		ProductName:   productName,
		ProductPhoto:  productPhoto,
		ModelPhoto:    modelPhoto,
		ExtraPhotos:   extraPhotos,
		Clips:         clips,
		VoiceoverPath: voiceoverPath,
		VoiceoverText: strings.TrimSpace(r.FormValue("voiceover_text")),
		AspectRatio:   aspectRatio,
		Seconds:       seconds,
		BPM:           bpm,
		MusicPath:     musicPath,
		MusicStart:    musicStart,
		MusicTitle:    strings.TrimSpace(r.FormValue("music_title")),
		MusicArtist:   strings.TrimSpace(r.FormValue("music_artist")),
		FixedCaption:  strings.TrimSpace(r.FormValue("fixed_caption")),
		AffLink:       strings.TrimSpace(r.FormValue("aff_link")),
	}

	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		studioErr("Không tạo được video: " + err.Error())
		return
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape("Đang dựng video quảng cáo sản phẩm chất lượng cao…"))
}

func logErr(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}
