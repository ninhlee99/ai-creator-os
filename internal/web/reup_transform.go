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
		Level          *int   `json:"level"`
		CharacterStyle string `json:"character_style"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in) // body trống → dùng mức mặc định
	level := s.atSettingInt(automation.KeyReupTransformLevel, 1)
	if in.Level != nil && (*in.Level == reup.Level1 || *in.Level == reup.Level2 || *in.Level == reup.Level3) {
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
	// failed) thì không tạo bài mới. Check + tạo bài bọc trong
	// Store.TransformMu chung với tick tự động — chặn cửa sổ race tick
	// vừa check VideosNeedingTransform xong thì UI chen CreatePost
	// (Đợt Y).
	s.Reup.TransformMu.Lock()
	live, lerr := s.Reup.LivePostForVideo(v.ID)
	var post reup.Post
	var perr error
	if lerr == nil && live == nil {
		post, perr = s.Reup.CreatePost(ids, level)
	}
	s.Reup.TransformMu.Unlock()
	if lerr != nil {
		writeJSONErr(w, "Kiểm tra bài trùng: "+lerr.Error(), http.StatusInternalServerError)
		return
	}
	if live != nil {
		writeJSONErr(w, fmt.Sprintf("Video đã có bài #%d (%s) — không tạo trùng.", live.ID, live.StatusLabel()), http.StatusPreconditionFailed)
		return
	}
	if perr != nil {
		writeJSONErr(w, "Tạo bài: "+perr.Error(), http.StatusInternalServerError)
		return
	}
	voiceover := s.atSettingOn(automation.KeyReupVoiceoverEnabled, true)
	go s.runReupTransform(post.ID, tr, clips, reup.TransformOptions{
		Level:          level,
		Seed:           v.ID,
		Voiceover:      voiceover,
		CharacterStyle: in.CharacterStyle,
	})
	writeJSON(w, map[string]any{"ok": true, "post_id": post.ID})
}

// runReupTransform chạy transform nền (serialize 1 lượt — nặng CPU).
// Dùng Store.TransformMu chung với tick tự động (automation) để không
// bao giờ có 2 ffmpeg chạy song song hay 2 bài cho 1 video (Đợt Y).
func (s *Server) runReupTransform(postID int64, tr *reup.Transformer, clips []reup.Video, opts reup.TransformOptions) {
	s.Reup.TransformMu.Lock()
	defer s.Reup.TransformMu.Unlock()
	_ = s.Reup.SetPostStatus(postID, reup.PostTransforming, "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var out string
	var err error
	if opts.Level == reup.Level3 {
		out, err = tr.TransformRemake(ctx, clips[0], opts)
	} else if opts.Level == reup.Level2 {
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
	serveMediaFile(w, r, abs)
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

// handleReupPostRetry (POST /reup/posts/{id}/retry): thử lại bài lỗi —
// chạy transform lại với cùng video/mức (serialize qua Store.TransformMu
// như handleReupTransform). Đợt Y.
func (s *Server) handleReupPostRetry(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		reupRedirect(w, r, "", "ID bài không hợp lệ.")
		return
	}
	p, err := s.Reup.GetPost(id)
	if err != nil {
		reupRedirect(w, r, "", "Không tìm thấy bài.")
		return
	}
	if p.Status != reup.PostFailed {
		reupRedirect(w, r, "", fmt.Sprintf("Bài #%d đang %s — chỉ thử lại bài lỗi.", id, p.StatusLabel()))
		return
	}
	tr := s.reupTransformer()
	if tr == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	var clips []reup.Video
	for _, vid := range p.VideoIDs {
		v, verr := s.Reup.GetVideo(vid)
		if verr != nil || v.Status != reup.StatusDownloaded {
			reupRedirect(w, r, "", fmt.Sprintf("Video #%d không còn để transform lại.", vid))
			return
		}
		clips = append(clips, v)
	}
	if len(clips) == 0 {
		reupRedirect(w, r, "", "Bài không còn video để transform lại.")
		return
	}
	voiceover := s.atSettingOn(automation.KeyReupVoiceoverEnabled, true)
	go s.runReupTransform(p.ID, tr, clips, reup.TransformOptions{
		Level:     p.TransformLevel,
		Seed:      clips[0].ID,
		Voiceover: voiceover,
	})
	reupRedirect(w, r, fmt.Sprintf("Đang thử lại bài #%d.", id), "")
}

// reupSettingsView gom cấu hình reup cho tab Cài đặt · Reup.
type reupSettingsView struct {
	DiscoverOn      bool
	DiscoverHours   int
	VideosPerSource int
	TransformOn     bool
	TransformLevel  int
	VoiceoverOn     bool
	PostOn          bool
	VideosPerDay    int
	PostAccount     string
	AccountNames    []string
	KillOn          bool
	KillN           int
	KillPerSource   bool
	WarmupOn        bool
	MusicOK         bool
	StoreOK         bool
}

func (s *Server) reupSettingsCfg() reupSettingsView {
	v := reupSettingsView{
		DiscoverOn:      s.atSettingOn(automation.KeyReupDiscoverEnabled, true),
		DiscoverHours:   s.atSettingInt(automation.KeyReupDiscoverIntervalH, 6),
		VideosPerSource: s.atSettingInt(automation.KeyReupVideosPerSource, 3),
		TransformOn:     s.atSettingOn(automation.KeyReupTransformEnabled, true),
		TransformLevel:  s.atSettingInt(automation.KeyReupTransformLevel, 1),
		VoiceoverOn:     s.atSettingOn(automation.KeyReupVoiceoverEnabled, true),
		PostOn:          s.atSettingOn(automation.KeyReupPostEnabled, true),
		VideosPerDay:    s.atSettingInt(automation.KeyReupVideosPerDay, 3),
		KillOn:          s.atSettingOn(automation.KeyReupKillEnabled, true),
		KillN:           s.atSettingInt(automation.KeyReupKillZeroN, 5),
		KillPerSource:   s.atSettingOn(automation.KeyReupKillPerSource, true),
		WarmupOn:        s.atSettingOn(automation.KeyReupWarmupEnabled, true),
		StoreOK:         s.Reup != nil,
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
	if v.TransformLevel != 2 && v.TransformLevel != 3 {
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
	setBool(automation.KeyReupKillPerSource, "kill_per_source")
	setBool(automation.KeyReupWarmupEnabled, "warmup_on")
	if f.Has("discover_on") {
		setBool(automation.KeyReupDiscoverEnabled, "discover_on")
	}
	// Chỉ ghi đè khi form có field (tránh form cũ/cache reset lặng lẽ).
	if f.Has("discover_hours") {
		discH, _ := strconv.Atoi(f.Get("discover_hours"))
		if discH < 1 {
			discH = 1
		}
		if discH > 72 {
			discH = 72
		}
		_ = s.Ledger.SetSetting(automation.KeyReupDiscoverIntervalH, strconv.Itoa(discH))
	}
	if f.Has("videos_per_source") {
		perSrc, _ := strconv.Atoi(f.Get("videos_per_source"))
		if perSrc < 1 {
			perSrc = 1
		}
		if perSrc > 10 {
			perSrc = 10
		}
		_ = s.Ledger.SetSetting(automation.KeyReupVideosPerSource, strconv.Itoa(perSrc))
	}
	level, _ := strconv.Atoi(f.Get("transform_level"))
	if level != 2 && level != 3 {
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
