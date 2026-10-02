package web

// Reup transform + đăng (Đợt E): UI chạy transform 2 mức, xem trước
// before/after, đăng bài, cài đặt transform/post/kill.
//
// TRUNG THỰC: mọi chữ trong UI về transform chỉ ghi "giảm rủi ro",
// không bao giờ hứa "an toàn bản quyền".

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// reupPostView gom dữ liệu bài đăng cho template.
type reupPostView struct {
	Post     reup.Post
	OrigFile string // id video gốc (cho preview before) — "" nếu compilation
	HasFile  bool
}

// reupTransformer dựng transformer cho UI (TTS/LLM chain của app).
func (s *Server) reupTransformer() *reup.Transformer {
	if s.Reup == nil {
		return nil
	}
	_, workDir := s.reupDirs()
	music := ""
	if s.DataDir != "" {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			music = mp
		}
	}
	var voice reup.VoiceSynth
	if s.TTS != nil {
		voice = s.TTS // TTSChainAPI.Synthesize khớp hợp đồng (VieNeu local, không tốn key)
	}
	var llm reup.Commentator
	if s.LLM != nil {
		llm = s.LLM
	}
	return &reup.Transformer{
		Store:     s.Reup,
		Voice:     voice,
		LLM:       llm,
		MusicPath: music,
		WorkDir:   filepath.Join(workDir, "transform"),
	}
}

// reupPageDataTransform bổ sung dữ liệu transform/post cho trang Reup.
func (s *Server) reupPageDataTransform(v *reupView) {
	v.TransformOn = s.atSettingOn(automation.KeyReupTransformEnabled, true)
	v.TransformLevel = s.atSettingInt(automation.KeyReupTransformLevel, 1)
	v.VoiceoverOn = s.atSettingOn(automation.KeyReupVoiceoverEnabled, true)
	v.PostOn = s.atSettingOn(automation.KeyReupPostEnabled, true)
	v.VideosPerDay = s.atSettingInt(automation.KeyReupVideosPerDay, 3)
	v.KillOn = s.atSettingOn(automation.KeyReupKillEnabled, true)
	v.KillN = s.atSettingInt(automation.KeyReupKillZeroN, 5)
	v.WarmupOn = s.atSettingOn(automation.KeyReupWarmupEnabled, true)
	if s.Ledger != nil {
		if val, ok, _ := s.Ledger.GetSetting(automation.KeyReupPostAccount); ok {
			v.PostAccount = val
		}
	}
	if s.DataDir != "" {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			v.MusicOK = true
		}
	}
	if s.Mgr != nil {
		if accts, err := s.Mgr.List(); err == nil {
			for _, a := range accts {
				v.AccountNames = append(v.AccountNames, a.Username)
			}
		}
	}
	if s.Reup == nil {
		return
	}
	posts, err := s.Reup.ListPosts(30)
	if err != nil {
		return
	}
	for _, p := range posts {
		pv := reupPostView{Post: p, HasFile: p.FilePath != "" && fileExists(p.FilePath)}
		if len(p.VideoIDs) == 1 {
			pv.OrigFile = fmt.Sprintf("%d", p.VideoIDs[0])
		}
		v.Posts = append(v.Posts, pv)
	}
}

// handleReupTransform (POST /reup/videos/{id}/transform, AJAX): tạo bài và
// chạy transform nền. Mức 2 cần 3 clip cùng nguồn (lấy thêm 2 video cùng
// nguồn đã tải, ưu tiên play_count cao).
func (s *Server) handleReupTransform(w http.ResponseWriter, r *http.Request) {
	tr := s.reupTransformer()
	if tr == nil {
		writeJSONErr(w, "Kho Reup chưa sẵn sàng.", http.StatusPreconditionFailed)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSONErr(w, "ID video không hợp lệ.", http.StatusBadRequest)
		return
	}
	v, err := s.Reup.GetVideo(id)
	if err != nil {
		writeJSONErr(w, "Không tìm thấy video.", http.StatusNotFound)
		return
	}
	if v.Status != reup.StatusDownloaded {
		writeJSONErr(w, "Video chưa tải xong — không transform được.", http.StatusPreconditionFailed)
		return
	}
	var in struct {
		Level *int `json:"level"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in) // body trống → dùng mức mặc định
	level := s.atSettingInt(automation.KeyReupTransformLevel, 1)
	if in.Level != nil && (*in.Level == reup.Level1 || *in.Level == reup.Level2) {
		level = *in.Level
	}

	// Chuẩn bị clip cho mức đã chọn.
	var clips []reup.Video
	var ids []int64
	if level == reup.Level2 {
		more, merr := s.Reup.VideosForCompilation(v.SourceID, 3)
		if merr != nil || len(more) < 3 {
			writeJSONErr(w, "Mức 2 cần 3 video cùng nguồn đã tải — chưa đủ.",
				http.StatusPreconditionFailed)
			return
		}
		// Đảm bảo video đang chọn nằm trong compilation.
		found := false
		for _, m := range more {
			if m.ID == v.ID {
				found = true
			}
		}
		if !found {
			more[2] = v
		}
		clips = more
		for _, c := range clips {
			ids = append(ids, c.ID)
		}
	} else {
		clips = []reup.Video{v}
		ids = []int64{v.ID}
	}
	// Chặn transform trùng từ UI: video đã có bài còn hiệu lực (khác
	// failed) thì không tạo bài mới. (Tick tự động đã an toàn nhờ
	// VideosNeedingTransform.)
	if live, err := s.Reup.LivePostForVideo(v.ID); err != nil {
		writeJSONErr(w, "Kiểm tra bài trùng: "+err.Error(), http.StatusInternalServerError)
		return
	} else if live != nil {
		writeJSONErr(w, fmt.Sprintf("Video đã có bài #%d (%s) — không tạo trùng.", live.ID, live.StatusLabel()), http.StatusPreconditionFailed)
		return
	}
	post, err := s.Reup.CreatePost(ids, level)
	if err != nil {
		writeJSONErr(w, "Tạo bài: "+err.Error(), http.StatusInternalServerError)
		return
	}
	voiceover := s.atSettingOn(automation.KeyReupVoiceoverEnabled, true)
	title := v.Title
	if level == reup.Level2 {
		title = "Tuyển tập"
	}
	go s.runReupTransform(post.ID, tr, clips, reup.TransformOptions{
		Level:     level,
		Seed:      v.ID,
		Title:     title,
		Voiceover: voiceover,
	})
	writeJSON(w, map[string]any{"ok": true, "post_id": post.ID})
}

// runReupTransform chạy transform nền (serialize 1 lượt — nặng CPU).
func (s *Server) runReupTransform(postID int64, tr *reup.Transformer, clips []reup.Video, opts reup.TransformOptions) {
	s.reupTransformMu.Lock()
	defer s.reupTransformMu.Unlock()
	_ = s.Reup.SetPostStatus(postID, reup.PostTransforming, "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var out string
	var err error
	if opts.Level == reup.Level2 {
		out, err = tr.TransformCompilation(ctx, clips, opts)
	} else {
		out, err = tr.TransformVideo(ctx, clips[0], opts)
	}
	if err != nil {
		_ = s.Reup.SetPostStatus(postID, reup.PostFailed, err.Error())
		return
	}
	_ = s.Reup.SetPostTransformed(postID, out)
}

// handleReupPostStatus (GET /reup/posts/{id}/status, AJAX): UI poll tiến độ.
func (s *Server) handleReupPostStatus(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		writeJSONErr(w, "Kho Reup chưa sẵn sàng.", http.StatusPreconditionFailed)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSONErr(w, "ID bài không hợp lệ.", http.StatusBadRequest)
		return
	}
	p, err := s.Reup.GetPost(id)
	if err != nil {
		writeJSONErr(w, "Không tìm thấy bài.", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "status": p.Status, "label": p.StatusLabel(),
		"fail_reason": p.FailReason, "file": p.FilePath != "",
		"views": p.ViewsLabel(),
	})
}

// serveReupFile serve file trong dir cho trước, chặn path traversal.
func (s *Server) serveReupFile(w http.ResponseWriter, r *http.Request, dir, path string) {
	if path == "" {
		http.NotFound(w, r)
		return
	}
	clean := filepath.Clean(path)
	absDir, _ := filepath.Abs(dir)
	abs, _ := filepath.Abs(clean)
	if !strings.HasPrefix(abs, absDir+string(filepath.Separator)) && abs != absDir {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, abs)
}

// handleReupFile (GET /reup/file/{id}): file transform của bài (preview after).
func (s *Server) handleReupFile(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := s.Reup.GetPost(id)
	if err != nil || p.FilePath == "" {
		http.NotFound(w, r)
		return
	}
	_, workDir := s.reupDirs()
	s.serveReupFile(w, r, filepath.Join(workDir, "transform"), p.FilePath)
}

// handleReupVideoFile (GET /reup/videos/{id}/file): file gốc (preview before).
func (s *Server) handleReupVideoFile(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	v, err := s.Reup.GetVideo(id)
	if err != nil || v.FilePath == "" {
		http.NotFound(w, r)
		return
	}
	_, workDir := s.reupDirs()
	s.serveReupFile(w, r, workDir, v.FilePath)
}

// handleReupPostPublish (POST /reup/posts/{id}/publish): đăng bài đã
// transform lên kênh đã chọn (draft-first/private-first như cũ).
func (s *Server) handleReupPostPublish(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		reupRedirect(w, r, "", "ID bài không hợp lệ.")
		return
	}
	account := strings.TrimSpace(r.FormValue("account"))
	if account == "" {
		reupRedirect(w, r, "", "Chọn kênh để đăng.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), reupTimeout)
	defer cancel()
	note, ok := s.automation().PublishReupPostNow(ctx, id, account)
	if !ok {
		reupRedirect(w, r, "", note)
		return
	}
	reupRedirect(w, r, note, "")
}

// handleReupSettings lưu cài đặt transform/post/kill (AJAX JSON) — mở rộng
// handler Đợt D (các key cũ giữ nguyên).
func (s *Server) handleReupSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DiscoverOn  *bool   `json:"discover_on"`
		Hours       *int    `json:"interval_hours"`
		PerSource   *int    `json:"per_source"`
		TransformOn *bool   `json:"transform_on"`
		Level       *int    `json:"transform_level"`
		VoiceoverOn *bool   `json:"voiceover_on"`
		PostOn      *bool   `json:"post_on"`
		VideosPerDay *int   `json:"videos_per_day"`
		PostAccount *string `json:"post_account"`
		KillOn      *bool   `json:"kill_on"`
		KillN       *int    `json:"kill_n"`
		WarmupOn    *bool   `json:"warmup_on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONErr(w, "JSON không hợp lệ.", http.StatusBadRequest)
		return
	}
	if s.Ledger == nil {
		writeJSONErr(w, "Ledger chưa sẵn sàng.", http.StatusPreconditionFailed)
		return
	}
	setBool := func(key string, b *bool) {
		if b == nil {
			return
		}
		_ = s.Ledger.SetSetting(key, map[bool]string{true: "1", false: "0"}[*b])
	}
	clamp := func(n *int, lo, hi int) {
		if n == nil {
			return
		}
		if *n < lo {
			*n = lo
		}
		if *n > hi {
			*n = hi
		}
	}
	setBool(automation.KeyReupDiscoverEnabled, in.DiscoverOn)
	clamp(in.Hours, 1, 168)
	if in.Hours != nil {
		_ = s.Ledger.SetSetting(automation.KeyReupDiscoverIntervalH, strconv.Itoa(*in.Hours))
	}
	clamp(in.PerSource, 1, 10)
	if in.PerSource != nil {
		_ = s.Ledger.SetSetting(automation.KeyReupVideosPerSource, strconv.Itoa(*in.PerSource))
	}
	setBool(automation.KeyReupTransformEnabled, in.TransformOn)
	if in.Level != nil && (*in.Level == 1 || *in.Level == 2) {
		_ = s.Ledger.SetSetting(automation.KeyReupTransformLevel, strconv.Itoa(*in.Level))
	}
	setBool(automation.KeyReupVoiceoverEnabled, in.VoiceoverOn)
	setBool(automation.KeyReupPostEnabled, in.PostOn)
	clamp(in.VideosPerDay, 1, 20)
	if in.VideosPerDay != nil {
		_ = s.Ledger.SetSetting(automation.KeyReupVideosPerDay, strconv.Itoa(*in.VideosPerDay))
	}
	if in.PostAccount != nil {
		_ = s.Ledger.SetSetting(automation.KeyReupPostAccount, strings.TrimSpace(*in.PostAccount))
	}
	setBool(automation.KeyReupKillEnabled, in.KillOn)
	clamp(in.KillN, 2, 20)
	if in.KillN != nil {
		_ = s.Ledger.SetSetting(automation.KeyReupKillZeroN, strconv.Itoa(*in.KillN))
	}
	setBool(automation.KeyReupWarmupEnabled, in.WarmupOn)
	writeJSON(w, map[string]any{"ok": true})
}
// reupSettingsView gom cấu hình reup cho tab Cài đặt · Reup.
type reupSettingsView struct {
	TransformOn    bool
	TransformLevel int
	VoiceoverOn    bool
	PostOn         bool
	VideosPerDay   int
	PostAccount    string
	AccountNames   []string
	KillOn         bool
	KillN          int
	WarmupOn       bool
	MusicOK        bool
	StoreOK        bool
}

func (s *Server) reupSettingsCfg() reupSettingsView {
	v := reupSettingsView{
		TransformOn:    s.atSettingOn(automation.KeyReupTransformEnabled, true),
		TransformLevel: s.atSettingInt(automation.KeyReupTransformLevel, 1),
		VoiceoverOn:    s.atSettingOn(automation.KeyReupVoiceoverEnabled, true),
		PostOn:         s.atSettingOn(automation.KeyReupPostEnabled, true),
		VideosPerDay:   s.atSettingInt(automation.KeyReupVideosPerDay, 3),
		KillOn:         s.atSettingOn(automation.KeyReupKillEnabled, true),
		KillN:          s.atSettingInt(automation.KeyReupKillZeroN, 5),
		WarmupOn:       s.atSettingOn(automation.KeyReupWarmupEnabled, true),
		StoreOK:        s.Reup != nil,
	}
	if s.Ledger != nil {
		if val, ok, _ := s.Ledger.GetSetting(automation.KeyReupPostAccount); ok {
			v.PostAccount = val
		}
	}
	if s.DataDir != "" {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			v.MusicOK = true
		}
	}
	if s.Mgr != nil {
		if accts, err := s.Mgr.List(); err == nil {
			for _, a := range accts {
				v.AccountNames = append(v.AccountNames, a.Username)
			}
		}
	}
	if v.TransformLevel != 2 {
		v.TransformLevel = 1
	}
	return v
}

// handleSettingsReup: tab Cài đặt · Reup (Đợt E).
func (s *Server) handleSettingsReup(w http.ResponseWriter, r *http.Request) {
	s.settingsPage(w, r, "reup", "settings_reup", "ReupCfg")
}

// handleSettingsReupSave lưu form tab Reup (POST, redirect 303).
func (s *Server) handleSettingsReupSave(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/settings/reup?err="+url.QueryEscape("Kho dữ liệu chưa sẵn sàng."))
		return
	}
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/settings/reup?err="+url.QueryEscape("Không đọc được form."))
		return
	}
	f := r.PostForm
	setBool := func(key, field string) {
		_ = s.Ledger.SetSetting(key, map[bool]string{true: "1", false: "0"}[f.Get(field) == "1"])
	}
	setBool(automation.KeyReupTransformEnabled, "transform_on")
	setBool(automation.KeyReupVoiceoverEnabled, "voiceover_on")
	setBool(automation.KeyReupPostEnabled, "post_on")
	setBool(automation.KeyReupKillEnabled, "kill_on")
	setBool(automation.KeyReupWarmupEnabled, "warmup_on")
	level, _ := strconv.Atoi(f.Get("transform_level"))
	if level != 2 {
		level = 1
	}
	_ = s.Ledger.SetSetting(automation.KeyReupTransformLevel, strconv.Itoa(level))
	perDay, _ := strconv.Atoi(f.Get("videos_per_day"))
	if perDay < 1 {
		perDay = 1
	}
	if perDay > 20 {
		perDay = 20
	}
	_ = s.Ledger.SetSetting(automation.KeyReupVideosPerDay, strconv.Itoa(perDay))
	killN, _ := strconv.Atoi(f.Get("kill_n"))
	if killN < 2 {
		killN = 2
	}
	if killN > 20 {
		killN = 20
	}
	_ = s.Ledger.SetSetting(automation.KeyReupKillZeroN, strconv.Itoa(killN))
	_ = s.Ledger.SetSetting(automation.KeyReupPostAccount, strings.TrimSpace(f.Get("post_account")))
	_ = s.Ledger.Decide("human", "reup_settings_save", nil, "Lưu cài đặt Reup.", nil)
	seeOther(w, r, "/settings/reup?ok="+url.QueryEscape("Đã lưu cài đặt Reup."))
}
