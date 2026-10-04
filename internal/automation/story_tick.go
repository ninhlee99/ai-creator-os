package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
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
	KeyStoryTopicsAutofill  = "story.topics_autofill"   // tự nghĩ chủ đề khi hàng đợi trống (mặc định bật)
	KeyStoryTopicsAutofillN = "story.topics_autofill_n" // số chủ đề mỗi lần refill (mặc định 10)
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

// topicGenerator là khả năng tự nghĩ chủ đề (studio.Studio có, interface
// StoryRunner không bắt buộc — assert mềm để không vỡ implement khác).
type topicGenerator interface {
	GenerateTopics(ctx context.Context, genre string, n int) ([]string, error)
}

// refillStoryTopics tự nghĩ chủ đề mới khi hàng đợi trống. Trả về ""
// khi refill xong (caller pop lại), hoặc note trung thực khi không làm
// được (tắt autofill / thiếu LLM / LLM lỗi).
func (s *Service) refillStoryTopics(ctx context.Context) string {
	if !atOn(s.Settings, KeyStoryTopicsAutofill, true) {
		return "story: hàng đợi chủ đề trống (tự refill đang tắt) — thêm chủ đề ở trang Kể chuyện"
	}
	gen, ok := s.Story.(topicGenerator)
	if !ok || gen == nil {
		return "story: hàng đợi chủ đề trống — thêm chủ đề ở trang Kể chuyện"
	}
	n := atInt(s.Settings, KeyStoryTopicsAutofillN, 10)
	genre := atStr(s.Settings, KeyStoryGenre, "tâm lý")
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
// (Đợt K: mặc định BẬT — private-first nên an toàn, Ninh xem lại trên
// YouTube Studio; tắt ở trang Kể chuyện nếu muốn duyệt tay.)
func (s *Service) storyAutoPublish(ctx context.Context) []string {
	if !atOn(s.Settings, KeyStoryAutoPublish, true) {
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
	// Đợt M1: SEO + thumbnail tự động. LLM viết title/desc/tags tiếng Việt;
	// thiếu LLM thì dùng template trung thực. Thumbnail = ảnh cảnh đầu tiên.
	vmeta := s.storySEOMeta(ctx, j)
	vmeta.ThumbnailPath = s.storyThumbnailPath(j.ID)
	var res publishers.PublishResult
	if mu, ok := s.Uploader.(MetaUploader); ok && mu != nil {
		res = mu.UploadMeta(ctx, acct, j.Output, vmeta, "short_film")
	} else {
		res = s.Uploader.Upload(ctx, acct, j.Output, vmeta.Title, vmeta.Description, "short_film")
	}
	if !res.Ok {
		s.Story.AppendLog(jobID, "Đăng YouTube thất bại: "+res.Error)
		return fmt.Sprintf("story %q: đăng thất bại — %s", j.Title, res.Error), false
	}
	logLine := "Đã đăng YouTube (private): " + res.RemoteID
	if vmeta.ThumbnailPath != "" {
		if res.ThumbnailSet {
			logLine += " + thumbnail"
		} else if res.ThumbnailError != "" {
			logLine += " (thumbnail lỗi: " + res.ThumbnailError + ")"
		}
	}
	s.Story.AppendLog(jobID, logLine)
	if s.Settings != nil {
		_ = s.Settings.Set(KeyStoryPublishedPrefix+jobID, "1")
		// Đợt N: hẹn giờ chuyển public (nếu bật). Không có videoID thì bỏ qua.
		s.enqueuePublic(jobID, res.RemoteID)
	}
	note := fmt.Sprintf("story %q: đã đăng YouTube (private)", j.Title)
	if res.URL != "" {
		note += " (" + res.URL + ")"
	}
	if h := atInt(s.Settings, KeyStoryPublicAfterHours, 0); h > 0 && res.RemoteID != "" {
		note += fmt.Sprintf(" — tự chuyển công khai sau %d giờ", h)
	}
	return note, true
}

// enqueuePublic thêm video vào hàng chờ chuyển public (đợt N).
// Chỉ hẹn khi Ninh đã bật story.public_after_hours > 0.
func (s *Service) enqueuePublic(jobID, videoID string) {
	if s.Settings == nil || videoID == "" {
		return
	}
	if atInt(s.Settings, KeyStoryPublicAfterHours, 0) <= 0 {
		return
	}
	pending := s.pendingPublic()
	pending[jobID] = time.Now().Format("2006-01-02T15:04:05") + "|" + videoID
	s.savePendingPublic(pending)
}

// pendingPublic đọc hàng chờ chuyển public (map rỗng khi chưa có/lỗi).
func (s *Service) pendingPublic() map[string]string {
	out := map[string]string{}
	if s.Settings == nil {
		return out
	}
	raw, ok := s.Settings.Get(KeyStoryPendingPublic)
	if !ok || raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func (s *Service) savePendingPublic(m map[string]string) {
	if s.Settings == nil {
		return
	}
	raw, _ := json.Marshal(m)
	_ = s.Settings.Set(KeyStoryPendingPublic, string(raw))
}

// PrivacyUpdater: uploader đổi được quyền riêng tư video (soft-assert).
type PrivacyUpdater interface {
	UpdatePrivacy(ctx context.Context, a *network.Account, videoID, privacy string) error
}

// StoryPublicTick chuyển public các video private đã quá thời gian chờ
// (đợt N). 0 giờ = tắt. Tôn trọng kill switch + dry-run.
func (s *Service) StoryPublicTick(ctx context.Context) []string {
	if s.killed() || s.dryRun() {
		return nil
	}
	hours := atInt(s.Settings, KeyStoryPublicAfterHours, 0)
	if hours <= 0 || s.Settings == nil {
		return nil
	}
	pending := s.pendingPublic()
	if len(pending) == 0 {
		return nil
	}
	account := atStr(s.Settings, KeyStoryAccount, "")
	if account == "" || s.Accounts == nil || s.Uploader == nil {
		return nil
	}
	var acct *network.Account
	if accts, err := s.Accounts.List(); err == nil {
		for _, a := range accts {
			if a.Username == account {
				acct = a
				break
			}
		}
	}
	if acct == nil {
		return []string{fmt.Sprintf("story: không tìm thấy kênh %q để chuyển public", account)}
	}
	pu, ok := s.Uploader.(PrivacyUpdater)
	if !ok || pu == nil {
		return []string{"story: uploader không hỗ trợ đổi quyền riêng tư"}
	}
	// So sánh chuỗi theo format "2006-01-02T15:04:05" (cùng cách ghi khi
	// enqueue bằng time.Now().Format) — tránh bẫy timezone của time.Parse
	// (parse ra UTC trong khi Now() là giờ local).
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Format("2006-01-02T15:04:05")
	var notes []string
	changed := false
	for jobID, val := range pending {
		ts, vid, _ := strings.Cut(val, "|")
		if vid == "" || ts == "" || ts >= cutoff {
			continue // chưa tới giờ → giữ lại
		}
		if _, err := time.ParseInLocation("2006-01-02T15:04:05", ts, time.Local); err != nil {
			continue // dữ liệu hỏng → giữ lại, không crash tick
		}
		if err := pu.UpdatePrivacy(ctx, acct, vid, "public"); err != nil {
			notes = append(notes, fmt.Sprintf("story %s: chuyển public thất bại — %v (thử lại vòng sau)", jobID, err))
			continue
		}
		delete(pending, jobID)
		changed = true
		if s.Story != nil {
			s.Story.AppendLog(jobID, "Tự chuyển công khai sau "+strconv.Itoa(hours)+" giờ (đợt N)")
		}
		notes = append(notes, fmt.Sprintf("story %s: đã tự chuyển công khai", jobID))
	}
	if changed {
		s.savePendingPublic(pending)
	}
	return notes
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

// ---------------------------------------------------------------------------
// Đợt M1: SEO + thumbnail tự động khi đăng story.

// seoGenerator: Studio viết metadata YouTube bằng LLM (soft-assert).
type seoGenerator interface {
	GenerateSEOMeta(ctx context.Context, topic, genre, fallbackTitle string) (studio.SEOMeta, error)
}

// assetLister: Studio liệt kê asset của job (soft-assert).
type assetLister interface {
	ListAssets(jobID string) []studio.Asset
}

// MetaUploader: uploader hỗ trợ metadata đầy đủ (tags + thumbnail).
// Không bắt buộc — uploader cũ vẫn chạy qua Upload thường.
type MetaUploader interface {
	UploadMeta(ctx context.Context, a *network.Account, videoPath string, meta publishers.VideoMeta, kind string) publishers.PublishResult
}

// storySEOMeta dựng metadata đăng YouTube: LLM viết, lỗi/thiếu LLM thì
// dùng template trung thực (không bịa "tối ưu").
func (s *Service) storySEOMeta(ctx context.Context, j studio.Job) publishers.VideoMeta {
	var sp studio.StoryParams
	_ = json.Unmarshal([]byte(j.Params), &sp)
	topic := strings.TrimSpace(sp.Topic)
	if topic == "" {
		topic = strings.TrimSpace(j.Title)
	}
	if g, ok := s.Story.(seoGenerator); ok && g != nil {
		if m, err := g.GenerateSEOMeta(ctx, topic, sp.Genre, j.Title); err == nil && m.Title != "" {
			desc := strings.TrimSpace(m.Description)
			if desc != "" {
				desc += "\n\n"
			}
			return publishers.VideoMeta{
				Title:       m.Title,
				Description: desc + growth.DisclosureLine,
				Tags:        m.Tags,
			}
		}
		s.Story.AppendLog(j.ID, "SEO: không viết được bằng LLM — dùng tiêu đề/mô tả cơ bản")
	}
	t := studio.TemplateSEOMeta(topic, sp.Genre, growth.DisclosureLine)
	return publishers.VideoMeta{Title: t.Title, Description: t.Description, Tags: t.Tags}
}

// storyThumbnailPath chọn ảnh cảnh đầu tiên còn file làm thumbnail YouTube.
// "" = không đặt thumbnail tùy chỉnh (YouTube tự lấy khung hình).
func (s *Service) storyThumbnailPath(jobID string) string {
	al, ok := s.Story.(assetLister)
	if !ok || al == nil {
		return ""
	}
	for _, a := range al.ListAssets(jobID) {
		if a.Kind != "photo" || a.Status != studio.StatusDone || a.Path == "" {
			continue
		}
		if fi, err := os.Stat(a.Path); err == nil && !fi.IsDir() {
			return a.Path
		}
	}
	return ""
}
