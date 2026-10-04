package web

// Reup Douyin (Đợt D+E): trang /reup — nguồn Douyin, quản lý yt-dlp sidecar,
// hàng đợi tải, thêm URL trực tiếp, transform 2 mức, bài đăng + before/after,
// công tắc tick.
//
// Fail-closed mọi nơi: kho chưa mở → trang báo rõ thay vì sập; yt-dlp
// chưa tải → nút tải + trạng thái (tick tự Ensure theo mẫu sidecar).

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/reup"
)

// reupTimeout là timeout cho mỗi thao tác reup từ UI.
const reupTimeout = 5 * time.Minute

// reupView gom dữ liệu cho template trang Reup.
type reupView struct {
	StoreOK         bool
	Sources         []reup.Source
	Videos          []reup.Video
	Stats           reup.VideoStats
	YtDlpInstalled  bool
	YtDlpVersion    string
	YtDlpBusy       bool
	DiscoverOn      bool
	DiscoverHours   int
	PerSource       int
	DiscoverLastRun string
	// Đợt E: transform + post + kill.
	Posts          []reupPostView
	AccountNames   []string
	PostAccount    string
	TransformOn    bool
	TransformLevel int
	VoiceoverOn    bool
	PostOn         bool
	VideosPerDay   int
	KillOn         bool
	KillN          int
	WarmupOn       bool
	MusicOK        bool
	// Level2IDs: video downloaded nào đủ ≥3 clip cùng nguồn để UI hiện
	// option Mức 2 (tránh bấm rồi fail).
	Level2IDs map[int64]bool
	Error     string
	Notice    string
}

// reupDirs trả về (binDir, workDir) cho yt-dlp + video đã tải.
func (s *Server) reupDirs() (string, string) {
	dataDir := s.DataDir
	if dataDir == "" && s.Cfg != nil {
		dataDir = filepath.Dir(s.Cfg.DatabasePath)
	}
	if dataDir == "" || dataDir == "." {
		dataDir = "."
	}
	return filepath.Join(dataDir, "bin"), filepath.Join(dataDir, "reup")
}

// reupDownlader dựng downloader từ state hiện tại; nil khi kho chưa mở.
func (s *Server) reupDownloader() *reup.Downloader {
	if s.Reup == nil {
		return nil
	}
	binDir, workDir := s.reupDirs()
	return reup.NewDownloader(reup.NewManager(binDir), reup.NewTikWM(), s.Reup, workDir)
}

// reupPageData đọc dữ liệu cho trang; lỗi đọc chỉ log, UI hiện trạng
// thái trung thực thay vì sập trang.
func (s *Server) reupPageData() reupView {
	v := reupView{
		StoreOK:       s.Reup != nil,
		DiscoverOn:    s.atSettingOn(automation.KeyReupDiscoverEnabled, true),
		DiscoverHours: s.atSettingInt(automation.KeyReupDiscoverIntervalH, 6),
		PerSource:     s.atSettingInt(automation.KeyReupVideosPerSource, 3),
	}
	if s.Ledger != nil {
		if lr, ok, _ := s.Ledger.GetSetting(automation.KeyReupDiscoverLastRun); ok {
			v.DiscoverLastRun = lr
		}
	}
	if s.Reup == nil {
		return v
	}
	if srcs, err := s.Reup.ListSources(); err != nil {
		log.Printf("web: reup sources: %v", err)
	} else {
		v.Sources = srcs
	}
	if vs, err := s.Reup.ListVideos(50); err != nil {
		log.Printf("web: reup videos: %v", err)
	} else {
		v.Videos = vs
	}
	if st, err := s.Reup.Stats(); err != nil {
		log.Printf("web: reup stats: %v", err)
	} else {
		v.Stats = st
	}
	// Mức 2 (compilation) cần ≥3 clip downloaded cùng nguồn: đánh dấu để
	// UI chỉ hiện option Mức 2 cho video đủ điều kiện.
	bySource := map[int64]int{}
	for _, vd := range v.Videos {
		if vd.Status == reup.StatusDownloaded {
			bySource[vd.SourceID]++
		}
	}
	v.Level2IDs = map[int64]bool{}
	for _, vd := range v.Videos {
		if vd.Status == reup.StatusDownloaded && bySource[vd.SourceID] >= 3 {
			v.Level2IDs[vd.ID] = true
		}
	}
	binDir, _ := s.reupDirs()
	mgr := reup.NewManager(binDir)
	if ver, err := mgr.Version(context.Background()); err == nil {
		v.YtDlpInstalled = true
		v.YtDlpVersion = ver
	}
	s.reupYtDlpMu.Lock()
	v.YtDlpBusy = s.reupYtDlpBusy
	s.reupYtDlpMu.Unlock()
	s.reupPageDataTransform(&v)
	return v
}

// handleReup render trang Reup.
func (s *Server) handleReup(w http.ResponseWriter, r *http.Request) {
	data := s.ctx()
	v := s.reupPageData()
	if !v.StoreOK {
		v.Error = "Kho Reup chưa sẵn sàng — kiểm tra log khởi động app."
	}
	data["Reup"] = v
	data["ok"] = r.URL.Query().Get("ok")
	data["err"] = r.URL.Query().Get("err")
	s.render(w, "reup", data)
}

// reupRedirect chuyển về /reup kèm toast (app.js tự đọc ?ok=/?err=).
func reupRedirect(w http.ResponseWriter, r *http.Request, okMsg, errMsg string) {
	loc := "/reup"
	if errMsg != "" {
		loc += "?err=" + url.QueryEscape(errMsg)
	} else if okMsg != "" {
		loc += "?ok=" + url.QueryEscape(okMsg)
	}
	seeOther(w, r, loc)
}

// handleReupSourceAdd thêm nguồn Douyin (form POST).
func (s *Server) handleReupSourceAdd(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	kind := r.FormValue("kind")
	value := r.FormValue("value")
	name := r.FormValue("name")
	src, err := s.Reup.AddSource(kind, value, name)
	if err != nil {
		reupRedirect(w, r, "", err.Error())
		return
	}
	reupRedirect(w, r, fmt.Sprintf("Đã thêm nguồn %s (%s).", src.DisplayName, src.KindLabel()), "")
}

// handleReupSourceToggle bật/tắt nguồn.
func (s *Server) handleReupSourceToggle(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		reupRedirect(w, r, "", "ID nguồn không hợp lệ.")
		return
	}
	// Đọc trạng thái hiện tại qua ListSources (tránh race bật/tắt).
	var cur *reup.Source
	if srcs, lerr := s.Reup.ListSources(); lerr == nil {
		for i := range srcs {
			if srcs[i].ID == id {
				cur = &srcs[i]
				break
			}
		}
	}
	if cur == nil {
		reupRedirect(w, r, "", fmt.Sprintf("Không tìm thấy nguồn #%d.", id))
		return
	}
	if err := s.Reup.SetSourceEnabled(id, !cur.Enabled); err != nil {
		reupRedirect(w, r, "", err.Error())
		return
	}
	state := "bật"
	if cur.Enabled {
		state = "tắt"
	}
	reupRedirect(w, r, fmt.Sprintf("Đã %s nguồn %s.", state, cur.DisplayName), "")
}

// handleReupSourceDelete xoá nguồn (video đã tải giữ lại).
func (s *Server) handleReupSourceDelete(w http.ResponseWriter, r *http.Request) {
	if s.Reup == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		reupRedirect(w, r, "", "ID nguồn không hợp lệ.")
		return
	}
	if err := s.Reup.DeleteSource(id); err != nil {
		reupRedirect(w, r, "", err.Error())
		return
	}
	reupRedirect(w, r, "Đã xoá nguồn (video đã tải giữ lại).", "")
}

// handleReupYtDlpStatus trả JSON trạng thái yt-dlp (UI poll sau khi bấm tải).
func (s *Server) handleReupYtDlpStatus(w http.ResponseWriter, r *http.Request) {
	binDir, _ := s.reupDirs()
	mgr := reup.NewManager(binDir)
	s.reupYtDlpMu.Lock()
	busy := s.reupYtDlpBusy
	s.reupYtDlpMu.Unlock()
	ver, err := mgr.Version(r.Context())
	writeJSON(w, map[string]any{
		"ok": true, "installed": err == nil, "version": ver, "busy": busy,
	})
}

// reupYtDlpRun chạy Ensure/Update trong background (tải binary ~30MB có
// thể mất vài phút — không giữ request).
func (s *Server) reupYtDlpRun(update bool) bool {
	s.reupYtDlpMu.Lock()
	if s.reupYtDlpBusy {
		s.reupYtDlpMu.Unlock()
		return false
	}
	s.reupYtDlpBusy = true
	s.reupYtDlpMu.Unlock()
	go func() {
		defer func() {
			s.reupYtDlpMu.Lock()
			s.reupYtDlpBusy = false
			s.reupYtDlpMu.Unlock()
		}()
		binDir, _ := s.reupDirs()
		mgr := reup.NewManager(binDir)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		var err error
		if update {
			err = mgr.Update(ctx)
		} else {
			err = mgr.Ensure(ctx)
		}
		if err != nil {
			log.Printf("web: reup yt-dlp ensure/update: %v", err)
		} else {
			log.Printf("web: reup yt-dlp sẵn sàng")
		}
	}()
	return true
}

// handleReupYtDlpEnsure bấm "Tải yt-dlp" (AJAX).
func (s *Server) handleReupYtDlpEnsure(w http.ResponseWriter, r *http.Request) {
	if !s.reupYtDlpRun(false) {
		writeJSON(w, map[string]any{"ok": true, "started": false, "note": "đang tải rồi"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "started": true})
}

// handleReupYtDlpUpdate bấm "Cập nhật yt-dlp" (AJAX, khi extractor gãy).
func (s *Server) handleReupYtDlpUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.reupYtDlpRun(true) {
		writeJSON(w, map[string]any{"ok": true, "started": false, "note": "đang tải rồi"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "started": true})
}

// handleReupScan quét tay (AJAX): discover + download ngay, trả JSON.
func (s *Server) handleReupScan(w http.ResponseWriter, r *http.Request) {
	auto := s.automation()
	if auto.Reup == nil {
		writeJSONErr(w, "Kho Reup chưa sẵn sàng.", http.StatusPreconditionFailed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), reupTimeout)
	defer cancel()
	// Tái dùng logic tick (không copy vòng discover+download).
	res := auto.RunReupScan(ctx)
	if s.Ledger != nil {
		_ = s.Ledger.SetSetting(automation.KeyReupDiscoverLastRun, time.Now().Format(time.RFC3339))
	}
	writeJSON(w, map[string]any{
		"ok": true, "found": res.Found, "downloaded": res.OK,
		"dup": res.Dup, "failed": res.Failed, "notes": res.Notes,
	})
}

// handleReupVideoAdd thêm video bằng URL trực tiếp (form POST → tải ngay).
func (s *Server) handleReupVideoAdd(w http.ResponseWriter, r *http.Request) {
	dl := s.reupDownloader()
	if dl == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	rawURL := strings.TrimSpace(r.FormValue("url"))
	if rawURL == "" {
		reupRedirect(w, r, "", "Nhập URL video Douyin.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), reupTimeout)
	defer cancel()
	v, err := dl.DownloadVideo(ctx, rawURL)
	if err != nil {
		reupRedirect(w, r, "", "Tải thất bại: "+err.Error())
		return
	}
	if v.Status == reup.StatusDownloaded {
		reupRedirect(w, r, fmt.Sprintf("Đã tải video #%d (%s).", v.ID, v.WatermarkLabel()), "")
		return
	}
	reupRedirect(w, r, "", fmt.Sprintf("Video #%d: %s", v.ID, v.FailReason))
}

// handleReupVideoRetry tải lại video lỗi.
func (s *Server) handleReupVideoRetry(w http.ResponseWriter, r *http.Request) {
	dl := s.reupDownloader()
	if dl == nil {
		reupRedirect(w, r, "", "Kho Reup chưa sẵn sàng.")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		reupRedirect(w, r, "", "ID video không hợp lệ.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), reupTimeout)
	defer cancel()
	v, err := dl.Retry(ctx, id)
	if err != nil {
		reupRedirect(w, r, "", "Tải lại thất bại: "+err.Error())
		return
	}
	if v.Status == reup.StatusDownloaded {
		reupRedirect(w, r, fmt.Sprintf("Đã tải lại video #%d.", v.ID), "")
		return
	}
	reupRedirect(w, r, "", fmt.Sprintf("Video #%d: %s", v.ID, v.FailReason))
}
