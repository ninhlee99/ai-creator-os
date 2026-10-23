package web

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// storyJobView là 1 job kể chuyện cho template.
type storyJobView struct {
	ID          string
	Title       string
	Status      string
	StatusVN    string
	Progress    int
	MediaURL    string // /media/<file> khi done
	Duration    string // "3:24" đo thật từ file, "" khi chưa có
	Scenes      int    // số asset ảnh
	LogTail     string
	Published   bool
	NeedsReview bool
	CreatedAt   string
	FinishedAt  string
}

// storiesView gom dữ liệu cho template trang Kể chuyện.
type storiesView struct {
	StudioOK     bool
	Jobs         []storyJobView
	AccountNames []string
	PostAccount  string
	// Mặc định form.
	DefTopic   string
	DefGenre   string
	DefWords   int
	DefScenes  int
	MusicOn    bool
	MusicOK    bool
	AutoOn     bool
	TickOn     bool
	AutofillOn bool // tự nghĩ chủ đề khi hàng đợi trống
	// PublicAfterH: giờ private trước khi tự chuyển public (đợt N, 0 = tắt)
	PublicAfterH int
	TickHours    int
	Topics       string // hàng đợi chủ đề (mỗi dòng 1 chủ đề)
	// Retry (đợt P): tự thử lại job lỗi.
	RetryOn       bool
	RetryMax      int
	RetryCooldown int
	Error         string
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
		StudioOK:      s.Studio != nil,
		DefGenre:      s.atSettingStr(automation.KeyStoryGenre, ""),
		DefWords:      s.atSettingInt(automation.KeyStoryWords, 600),
		DefScenes:     s.atSettingInt(automation.KeyStoryScenes, 6),
		MusicOn:       s.atSettingOn(automation.KeyStoryMusicOn, false),
		AutoOn:        s.atSettingOn(automation.KeyStoryAutoPublish, false),
		AutofillOn:    s.atSettingOn(automation.KeyStoryTopicsAutofill, false),
		PublicAfterH:  s.atSettingInt(automation.KeyStoryPublicAfterHours, 0),
		TickOn:        s.atSettingOn(automation.KeyStoryEnabled, false),
		TickHours:     s.atSettingInt(automation.KeyStoryIntervalH, 24),
		RetryOn:       s.atSettingOn(automation.KeyStoryRetryEnabled, false),
		RetryMax:      s.atSettingInt(automation.KeyStoryRetryMax, 3),
		RetryCooldown: s.atSettingInt(automation.KeyStoryRetryCooldown, 6),
		PostAccount:   s.atSettingStr(automation.KeyStoryAccount, ""),
	}
	if s.Ledger != nil {
		if t, ok, _ := s.Ledger.GetSetting(automation.KeyStoryTopics); ok {
			v.Topics = t
		}
	}
	if s.Studio == nil {
		return v
	}
	// Nhạc nền: cùng file autopilot Ninh upload (tái dùng cho kể chuyện) hoặc mặc định hệ thống.
	if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
		v.MusicOK = true
	} else if defM := studio.FindDefaultMusic(); defM != "" {
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
			outPath := j.Output
			if s.Studio != nil {
				outPath = s.Studio.OutputPath(j.Output)
			}
			if d := probeStoryDuration(outPath); d > 0 {
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
			if _, ok, _ := s.Ledger.GetSetting(automation.KeyStoryNeedsReviewPrefix + j.ID); ok {
				sv.NeedsReview = true
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
