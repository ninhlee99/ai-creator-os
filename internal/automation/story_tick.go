package automation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ---------------------------------------------------------------------------
// Story automation tick (Đợt F) — zero-touch, Ninh không chạm.
//
// StoryTick: đến hạn (mặc định 24h/lần) → lấy 1 chủ đề từ hàng đợi
// (story.topics, mỗi dòng 1 chủ đề) → tạo job kể chuyện (truyện ngôi thứ
// nhất → ảnh 16:9 → TTS → dựng 16:9 + phụ đề) → QC.
//
// Sau QC: mặc định CHỜ Ninh duyệt ở trang Kể chuyện rồi bấm Đăng
// (story.auto_publish=0). Bật story.auto_publish=1 → tick tự đăng
// private-first (không bao giờ tự public).
//
// Cùng cổng an toàn như các tick khác: kill switch + DRY-RUN chặn;
// công tắc riêng mặc định BẬT (unset = bật — tiền lệ Đợt 3).
// ---------------------------------------------------------------------------

// Setting keys cho story automation (ledger settings).
const (
	KeyStoryEnabled         = "story.enabled"
	KeyStoryIntervalH       = "story.interval_hours"
	KeyStoryLastRun         = "story.last_run"
	KeyStoryTopics          = "story.topics"
	KeyStoryWords           = "story.words"
	KeyStoryScenes          = "story.scenes"
	KeyStoryGenre           = "story.genre"
	KeyStoryMusicOn         = "story.music_on"
	KeyStoryAutoPublish     = "story.auto_publish"
	KeyStoryAccount         = "story.account"
	KeyStoryPublishedPrefix = "story.published."
	defaultStoryIntervalH   = 24
)

// StoryRunner tạo và tra cứu job kể chuyện. *studio.Studio thỏa mãn.
type StoryRunner interface {
	CreateStoryJob(p studio.StoryParams) (string, error)
	GetJob(id string) (studio.Job, bool)
	AppendLog(id, line string)
}

// popTopic lấy chủ đề đầu tiên khỏi hàng đợi và ghi lại phần còn lại.
func (s *Service) popTopic() string {
	if s.Settings == nil {
		return ""
	}
	raw, ok := s.Settings.Get(KeyStoryTopics)
	if !ok || strings.TrimSpace(raw) == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	var rest []string
	topic := ""
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if topic == "" {
			topic = ln
			continue
		}
		rest = append(rest, ln)
	}
	_ = s.Settings.Set(KeyStoryTopics, strings.Join(rest, "\n"))
	return topic
}

// StoryTick chạy pass tạo truyện đến hạn. main.go gọi mỗi 5 phút
// (cùng vòng với các tick khác); mỗi tick tự kiểm tra đến hạn bên trong.
func (s *Service) StoryTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	if !atOn(s.Settings, KeyStoryEnabled, true) {
		return nil
	}
	if s.Story == nil {
		return nil // studio chưa nối — bỏ qua im lặng như các tick khác
	}
	interval := time.Duration(atInt(s.Settings, KeyStoryIntervalH,
		defaultStoryIntervalH)) * time.Hour
	if interval <= 0 {
		interval = defaultStoryIntervalH * time.Hour
	}
	if !dueSince(s.Settings, KeyStoryLastRun, interval) {
		// Chưa đến hạn tạo mới, nhưng vẫn kiểm tra auto-publish.
		return s.storyAutoPublish(ctx)
	}
	topic := s.popTopic()
	if topic == "" {
		stampRun(s.Settings, KeyStoryLastRun)
		return []string{"story: hàng đợi chủ đề trống — thêm chủ đề ở trang Kể chuyện"}
	}
	p := studio.StoryParams{
		Topic:   topic,
		Genre:   atStr(s.Settings, KeyStoryGenre, "tâm lý"),
		Words:   atInt(s.Settings, KeyStoryWords, 600),
		Scenes:  atInt(s.Settings, KeyStoryScenes, 6),
		MusicOn: atOn(s.Settings, KeyStoryMusicOn, true),
	}
	if p.MusicOn && s.StoryMusicPath != "" {
		p.MusicPath = s.StoryMusicPath
	}
	id, err := s.Story.CreateStoryJob(p)
	if err != nil {
		stampRun(s.Settings, KeyStoryLastRun)
		return []string{fmt.Sprintf("story: tạo job lỗi: %v", err)}
	}
	stampRun(s.Settings, KeyStoryLastRun)
	log.Printf("story: job %s đã tạo (chủ đề %q)", id, topic)
	notes := []string{fmt.Sprintf("story: đã xếp truyện %q vào hàng đợi", topic)}
	return append(notes, s.storyAutoPublish(ctx)...)
}

// storyAutoPublish tự đăng private các job đã xong khi bật auto_publish.
// Mặc định TẮT — Ninh duyệt ở trang Kể chuyện rồi bấm Đăng.
func (s *Service) storyAutoPublish(ctx context.Context) []string {
	if !atOn(s.Settings, KeyStoryAutoPublish, false) {
		return nil
	}
	account := atStr(s.Settings, KeyStoryAccount, "")
	if account == "" {
		return []string{"story: auto_publish bật nhưng chưa chọn kênh"}
	}
	st, ok := s.Story.(*studio.Studio)
	if !ok || st == nil {
		return nil
	}
	var notes []string
	for _, j := range st.StoryJobs(10) {
		if j.Status != studio.StatusDone || j.Output == "" {
			continue
		}
		if s.Settings != nil {
			if _, done := s.Settings.Get(KeyStoryPublishedPrefix + j.ID); done {
				continue
			}
		}
		note, ok := s.PublishStoryNow(ctx, j.ID, account)
		notes = append(notes, note)
		if !ok {
			break // lỗi đăng → dừng, thử lại vòng sau
		}
	}
	return notes
}

// PublishStoryNow đăng 1 truyện đã dựng lên YouTube (private-first).
// Dùng chung cho nút Đăng trên UI và auto-publish.
func (s *Service) PublishStoryNow(ctx context.Context, jobID, username string) (string, bool) {
	if s.killed() || s.dryRun() {
		return "story: bị chặn bởi kill switch / dry-run", false
	}
	if s.Story == nil {
		return "story: studio chưa sẵn sàng", false
	}
	j, ok := s.Story.GetJob(jobID)
	if !ok {
		return fmt.Sprintf("story: không tìm thấy job %s", jobID), false
	}
	if j.Kind != studio.KindStory {
		return fmt.Sprintf("story: job %s không phải truyện kể", jobID), false
	}
	if j.Status != studio.StatusDone || j.Output == "" {
		return fmt.Sprintf("story %q: chưa dựng xong (trạng thái: %s)", j.Title, j.Status), false
	}
	if s.Accounts == nil {
		return "story: chưa nối quản lý kênh", false
	}
	var acct *network.Account
	if accts, lerr := s.Accounts.List(); lerr == nil {
		for _, a := range accts {
			if a.Username == username {
				acct = a
				break
			}
		}
	}
	if acct == nil {
		return fmt.Sprintf("story: không tìm thấy kênh %q", username), false
	}
	if s.Uploader == nil {
		return "story: chưa nối YouTube uploader", false
	}
	title := j.Title
	if title == "" {
		title = "Kể chuyện"
	}
	desc := title + "\n\n" + growth.DisclosureLine
	// "short_film" = Entertainment (24) — kind gần nhất cho truyện dài 16:9.
	res := s.Uploader.Upload(ctx, acct, j.Output, title, desc, "short_film")
	if !res.Ok {
		s.Story.AppendLog(jobID, "Đăng YouTube thất bại: "+res.Error)
		return fmt.Sprintf("story %q: đăng thất bại — %s", j.Title, res.Error), false
	}
	s.Story.AppendLog(jobID, "Đã đăng YouTube (private): "+res.RemoteID)
	if s.Settings != nil {
		_ = s.Settings.Set(KeyStoryPublishedPrefix+jobID, "1")
	}
	note := fmt.Sprintf("story %q: đã đăng YouTube (private)", j.Title)
	if res.URL != "" {
		note += " (" + res.URL + ")"
	}
	return note, true
}

// atStr đọc setting chuỗi (def khi chưa có).
func atStr(st Settings, key, def string) string {
	if st == nil {
		return def
	}
	if v, ok := st.Get(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}
