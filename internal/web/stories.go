package web

// Kể chuyện YouTube (Đợt F — pillar 3 post-pivot): trang /stories —
// truyện ngôi thứ nhất + ảnh minh họa từng cảnh + giọng đọc, KHÔNG quay
// video. Job chạy trên studio_jobs (kind=story), dựng bằng internal/studio.
//
// Fail-closed mọi nơi: Studio chưa khởi tạo → trang báo rõ; đăng YouTube
// private-first qua automation (kill switch / dry-run chặn).

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// storyJobView là 1 job kể chuyện cho template.
type storyJobView struct {
	ID         string
	Title      string
	Status     string
	StatusVN   string
	Progress   int
	MediaURL   string // /media/<file> khi done
	Duration   string // "3:24" đo thật từ file, "" khi chưa có
	Scenes     int    // số asset ảnh
	LogTail    string
	Published  bool
	CreatedAt  string
	FinishedAt string
}

// storiesView gom dữ liệu cho template trang Kể chuyện.
type storiesView struct {
	StudioOK     bool
	Jobs         []storyJobView
	AccountNames []string
	PostAccount  string
	// Mặc định form.
	DefTopic  string
	DefGenre  string
	DefWords  int
	DefScenes int
	MusicOn   bool
	MusicOK   bool
	AutoOn    bool
	TickOn    bool
	TickHours int
	Topics    string // hàng đợi chủ đề (mỗi dòng 1 chủ đề)
	Error     string
}

func storyStatusVN(st string) string {
	switch st {
	case studio.StatusQueued:
		return "đang chờ"
	case studio.StatusRunning:
		return "đang dựng"
	case studio.StatusDone:
		return "xong"
	case studio.StatusFailed:
		return "lỗi"
	}
	return st
}

// storiesPageData đọc dữ liệu cho trang; lỗi đọc chỉ log, UI hiện trạng
// thái trung thực thay vì sập trang.
func (s *Server) storiesPageData() storiesView {
	v := storiesView{
		StudioOK:    s.Studio != nil,
		DefGenre:    s.atSettingStr(automation.KeyStoryGenre, "tâm lý"),
		DefWords:    s.atSettingInt(automation.KeyStoryWords, 600),
		DefScenes:   s.atSettingInt(automation.KeyStoryScenes, 6),
		MusicOn:     s.atSettingOn(automation.KeyStoryMusicOn, true),
		AutoOn:      s.atSettingOn(automation.KeyStoryAutoPublish, false),
		TickOn:      s.atSettingOn(automation.KeyStoryEnabled, true),
		TickHours:   s.atSettingInt(automation.KeyStoryIntervalH, 24),
		PostAccount: s.atSettingStr(automation.KeyStoryAccount, ""),
	}
	if s.Ledger != nil {
		if t, ok, _ := s.Ledger.GetSetting(automation.KeyStoryTopics); ok {
			v.Topics = t
		}
	}
	if s.Studio == nil {
		return v
	}
	// Nhạc nền: cùng file autopilot Ninh upload (tái dùng cho kể chuyện).
	if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
		v.MusicOK = true
	}
	if s.Mgr != nil {
		if accts, err := s.Mgr.List(); err == nil {
			for _, a := range accts {
				v.AccountNames = append(v.AccountNames, a.Username)
			}
		}
	}
	for _, j := range s.Studio.StoryJobs(30) {
		sv := storyJobView{
			ID: j.ID, Title: j.Title, Status: j.Status,
			StatusVN: storyStatusVN(j.Status), Progress: j.Progress,
			CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
		}
		if j.Output != "" {
			sv.MediaURL = "/media/" + filepath.Base(j.Output)
			if d := probeStoryDuration(j.Output); d > 0 {
				m, sec := int(d)/60, int(d)%60
				sv.Duration = fmt.Sprintf("%d:%02d", m, sec)
			}
		}
		sv.Scenes = len(s.Studio.ListAssets(j.ID))
		sv.LogTail = s.Studio.StoryLogTail(j.ID)
		if s.Ledger != nil {
			if _, ok, _ := s.Ledger.GetSetting(automation.KeyStoryPublishedPrefix + j.ID); ok {
				sv.Published = true
			}
		}
		v.Jobs = append(v.Jobs, sv)
	}
	return v
}

// probeStoryDuration đo thời lượng thật của file output (0 khi lỗi).
func probeStoryDuration(path string) float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error", "-show_entries", "format=duration",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var d float64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d)
	if d < 0 {
		return 0
	}
	return d
}

func (s *Server) handleStories(w http.ResponseWriter, r *http.Request) {
	data := s.ctx()
	v := s.storiesPageData()
	if !v.StudioOK {
		v.Error = "Studio chưa được khởi tạo — kiểm tra log khởi động app."
	}
	data["Stories"] = v
	data["ok"] = r.URL.Query().Get("ok")
	data["err"] = r.URL.Query().Get("err")
	s.render(w, "stories", data)
}

// storiesRedirect chuyển về /stories kèm toast (app.js tự đọc ?ok=/?err=).
func storiesRedirect(w http.ResponseWriter, r *http.Request, okMsg, errMsg string) {
	loc := "/stories"
	if errMsg != "" {
		loc += "?err=" + url.QueryEscape(errMsg)
	} else if okMsg != "" {
		loc += "?ok=" + url.QueryEscape(okMsg)
	}
	seeOther(w, r, loc)
}

// handleStoryCreate tạo job kể chuyện mới (form POST).
func (s *Server) handleStoryCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		storiesRedirect(w, r, "", "Studio chưa sẵn sàng.")
		return
	}
	p := storyParamsFromForm(r)
	if strings.TrimSpace(p.Topic) == "" {
		storiesRedirect(w, r, "", "Nhập chủ đề truyện.")
		return
	}
	// Nhạc nền: cùng file autopilot (không bắt Ninh upload riêng).
	if p.MusicOn {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			p.MusicPath = mp
		} else {
			p.MusicOn = false
		}
	}
	id, err := s.Studio.CreateStoryJob(p)
	if err != nil {
		storiesRedirect(w, r, "", err.Error())
		return
	}
	log.Printf("web: story job %s đã tạo (chủ đề %q)", id, p.Topic)
	storiesRedirect(w, r, "Đã xếp truyện vào hàng đợi — theo dõi tiến trình bên dưới.", "")
}

// storyParamsFromForm đọc form tạo truyện + clamp giá trị.
func storyParamsFromForm(r *http.Request) studio.StoryParams {
	words, _ := strconv.Atoi(r.FormValue("words"))
	scenes, _ := strconv.Atoi(r.FormValue("scenes"))
	genre := strings.TrimSpace(r.FormValue("genre"))
	if genre == "" {
		genre = "tâm lý"
	}
	return studio.StoryParams{
		Topic:   strings.TrimSpace(r.FormValue("topic")),
		Genre:   genre,
		Words:   words,
		Scenes:  scenes,
		MusicOn: r.FormValue("music_on") == "1",
	}
}

// handleStoryPublish đăng 1 truyện đã dựng lên YouTube (private-first).
func (s *Server) handleStoryPublish(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	account := strings.TrimSpace(r.FormValue("account"))
	if account == "" {
		storiesRedirect(w, r, "", "Chọn kênh YouTube để đăng.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*60*1000000000)
	defer cancel()
	note, ok := s.automation().PublishStoryNow(ctx, id, account)
	if !ok {
		storiesRedirect(w, r, "", note)
		return
	}
	storiesRedirect(w, r, note, "")
}

// handleStoryDelete xoá 1 job kể chuyện (form POST, có confirm ở UI).
func (s *Server) handleStoryDelete(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		storiesRedirect(w, r, "", "Studio chưa sẵn sàng.")
		return
	}
	if err := s.Studio.DeleteStoryJob(r.PathValue("id")); err != nil {
		storiesRedirect(w, r, "", err.Error())
		return
	}
	storiesRedirect(w, r, "Đã xoá truyện.", "")
}

// handleStorySettings lưu mặc định kể chuyện (form POST).
func (s *Server) handleStorySettings(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		storiesRedirect(w, r, "", "Ledger chưa sẵn sàng.")
		return
	}
	set := func(key, val string) { _ = s.Ledger.SetSetting(key, val) }
	setBool := func(key string, on bool) {
		if on {
			set(key, "1")
		} else {
			set(key, "0")
		}
	}
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	if n, err := strconv.Atoi(r.FormValue("words")); err == nil {
		set(automation.KeyStoryWords, strconv.Itoa(clamp(n, 200, 1200)))
	}
	if n, err := strconv.Atoi(r.FormValue("scenes")); err == nil {
		set(automation.KeyStoryScenes, strconv.Itoa(clamp(n, 3, 12)))
	}
	if g := strings.TrimSpace(r.FormValue("genre")); g != "" {
		set(automation.KeyStoryGenre, g)
	}
	if h, err := strconv.Atoi(r.FormValue("interval_hours")); err == nil {
		set(automation.KeyStoryIntervalH, strconv.Itoa(clamp(h, 1, 168)))
	}
	if a := strings.TrimSpace(r.FormValue("account")); a != "" {
		set(automation.KeyStoryAccount, a)
	}
	set(automation.KeyStoryTopics, strings.TrimSpace(r.FormValue("topics")))
	setBool(automation.KeyStoryMusicOn, r.FormValue("music_on") == "1")
	setBool(automation.KeyStoryAutoPublish, r.FormValue("auto_publish") == "1")
	setBool(automation.KeyStoryEnabled, r.FormValue("tick_on") == "1")
	storiesRedirect(w, r, "Đã lưu mặc định kể chuyện.", "")
}

// atSettingStr đọc setting dạng chuỗi (def khi chưa có).
func (s *Server) atSettingStr(key, def string) string {
	if s.Ledger == nil {
		return def
	}
	if v, ok, err := s.Ledger.GetSetting(key); err == nil && ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}
