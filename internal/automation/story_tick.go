package automation

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"log"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Story automation tick (Đợt F) — zero-touch, Ninh không chạm.
//
// StoryTick: đến hạn (mặc định 24h/lần) → lấy 1 chủ đề từ hàng đợi
// (story.topics, mỗi dòng 1 chủ đề) → tạo job kể chuyện (truyện ngôi thứ
// nhất → ảnh 16:9 → TTS → dựng 16:9 + phụ đề) → QC.
//
// Sau QC: mặc định TỰ ĐĂNG private-first (story.auto_publish=1, Đợt K) —
// Ninh xem lại trên YouTube Studio; tắt ở trang Kể chuyện để duyệt tay.
// Không bao giờ tự public.
//
// Cùng cổng an toàn như các tick khác: kill switch + DRY-RUN chặn;
// công tắc riêng mặc định TẮT; chỉ chạy sau khi chủ kênh bật cấu hình.
// ---------------------------------------------------------------------------

// Setting keys cho story automation (ledger settings).
const (
	KeyStoryEnabled           = "story.enabled"
	KeyStoryIntervalH         = "story.interval_hours"
	KeyStoryLastRun           = "story.last_run"
	KeyStoryTopics            = "story.topics"
	KeyStoryWords             = "story.words"
	KeyStoryScenes            = "story.scenes"
	KeyStoryGenre             = "story.genre"
	KeyStoryMusicOn           = "story.music_on"
	KeyStoryAutoPublish       = "story.auto_publish"
	KeyStoryAccount           = "story.account"
	KeyStoryPublishedPrefix   = "story.published."
	KeyStoryNeedsReviewPrefix = "story.needs_review."
	// KeyStoryPublishingPrefix: marker "đang đăng" — đặt TRƯỚC khi upload,
	// xóa khi xong/lỗi (Đợt S). Crash/mất điện giữa upload và marker
	// published để lại dấu vết: vòng sau thấy publishing mà chưa published
	// thì biết lần trước đăng dở (có thể đã lên 1 bản private) thay vì im
	// lặng đăng trùng.
	KeyStoryPublishingPrefix = "story.publishing."
	KeyStoryTopicsAutofill   = "story.topics_autofill"   // tự nghĩ chủ đề khi hàng đợi trống (mặc định tắt)
	KeyStoryTopicsAutofillN  = "story.topics_autofill_n" // số chủ đề mỗi lần refill (mặc định 10)
	// KeyStoryPublicAfterHours: số giờ private trước khi tự chuyển public
	// (đợt N). 0 = tắt (mặc định: giữ private để Ninh duyệt tay).
	KeyStoryPublicAfterHours = "story.public_after_hours"
	// KeyStoryPendingPublic: JSON map jobID -> "ISOTime|videoID" chờ chuyển public.
	KeyStoryPendingPublic = "story.pending_public"
	defaultStoryIntervalH = 24
)

// StoryRunner tạo và tra cứu job kể chuyện. *studio.Studio thỏa mãn.
type StoryRunner interface {
	CreateStoryJob(p studio.StoryParams) (string, error)
	GetJob(id string) (studio.Job, bool)
	AppendLog(id, line string)
}

type freshStoryJobCapability interface {
	CanCreateFreshStoryJobs() bool
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
	if !atOn(s.Settings, KeyStoryEnabled, false) {
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
	if capability, ok := s.Story.(freshStoryJobCapability); ok && !capability.CanCreateFreshStoryJobs() {
		// Keep the topic queue intact and avoid autofill LLM calls while the
		// configured image provider cannot keep characters consistent.
		stampRun(s.Settings, KeyStoryLastRun)
		notes := []string{"story: tạm dừng vì chưa có provider ảnh tham chiếu miễn phí; giữ nguyên hàng đợi chủ đề"}
		return append(notes, s.storyAutoPublish(ctx)...)
	}
	topic := s.popTopic()
	if topic == "" {
		// Hàng đợi trống → tự refill bằng LLM (zero-touch), thay vì chờ
		// Ninh nhập tay. Refill lỗi (chưa cấu hình LLM) → note trung thực.
		if refillNote := s.refillStoryTopics(ctx); refillNote != "" {
			stampRun(s.Settings, KeyStoryLastRun)
			return []string{refillNote}
		}
		topic = s.popTopic()
	}
	if topic == "" {
		stampRun(s.Settings, KeyStoryLastRun)
		return []string{"story: hàng đợi chủ đề trống — thêm chủ đề ở trang Kể chuyện"}
	}
	p := studio.StoryParams{
		Topic:   topic,
		Genre:   atStr(s.Settings, KeyStoryGenre, ""),
		Words:   atInt(s.Settings, KeyStoryWords, 600),
		Scenes:  atInt(s.Settings, KeyStoryScenes, 6),
		MusicOn: atOn(s.Settings, KeyStoryMusicOn, false),
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

// topicGenerator là khả năng tự nghĩ chủ đề (studio.Studio có, interface
// StoryRunner không bắt buộc — assert mềm để không vỡ implement khác).
type topicGenerator interface {
	GenerateTopics(ctx context.Context, genre string, n int) ([]string, error)
}

// refillStoryTopics tự nghĩ chủ đề mới khi hàng đợi trống. Trả về ""
// khi refill xong (caller pop lại), hoặc note trung thực khi không làm
// được (tắt autofill / thiếu LLM / LLM lỗi).
func (s *Service) refillStoryTopics(ctx context.Context) string {
	if !atOn(s.Settings, KeyStoryTopicsAutofill, false) {
		return "story: hàng đợi chủ đề trống (tự refill đang tắt) — thêm chủ đề ở trang Kể chuyện"
	}
	gen, ok := s.Story.(topicGenerator)
	if !ok || gen == nil {
		return "story: hàng đợi chủ đề trống — thêm chủ đề ở trang Kể chuyện"
	}
	n := atInt(s.Settings, KeyStoryTopicsAutofillN, 10)
	genre := atStr(s.Settings, KeyStoryGenre, "")
	topics, err := gen.GenerateTopics(ctx, genre, n)
	if err != nil {
		return fmt.Sprintf("story: hàng đợi chủ đề trống — tự refill lỗi (%v)", err)
	}
	raw, _ := s.Settings.Get(KeyStoryTopics)
	raw = strings.TrimSpace(raw)
	if raw != "" {
		raw += "\n"
	}
	_ = s.Settings.Set(KeyStoryTopics, raw+strings.Join(topics, "\n"))
	log.Printf("story: tự refill %d chủ đề mới (thể loại %q)", len(topics), genre)
	return ""
}

// storyAutoPublish tự đăng private các job đã xong khi bật auto_publish.
// (Đợt K: mặc định TẮT — private-first nên an toàn, Ninh xem lại trên
// YouTube Studio; tắt ở trang Kể chuyện nếu muốn duyệt tay.)
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
	// Đợt Y: duyệt theo batch cho đến khi hết job done-chưa-đăng (trước đây
	// chỉ xét 10 job mới nhất → job cũ thứ 11+ bị bỏ qua vĩnh viễn).
	var pending []studio.Job
	for limit := 50; ; limit *= 2 {
		batch := st.StoryJobs(limit)
		pending = pending[:0]
		for _, j := range batch {
			if j.Status != studio.StatusDone || j.Output == "" {
				continue
			}
			if s.Settings != nil {
				if _, done := s.Settings.Get(KeyStoryPublishedPrefix + j.ID); done {
					continue
				}
			}
			pending = append(pending, j)
		}
		if len(batch) < limit || limit >= 1000 {
			break
		}
	}
	for _, j := range pending {
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
