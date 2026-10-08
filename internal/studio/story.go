package studio

// ---------------------------------------------------------------------------
// Story pipeline (Đợt F — pillar 3 post-pivot): truyện ngôi thứ nhất +
// ảnh minh họa từng cảnh + giọng đọc, KHÔNG quay video.
//
//   - Viết truyện: LLM (tái dùng prompts/story.txt) → JSON {title, logline, text}.
//   - Chia cảnh: LLM → JSON {scenes:[{text, image_prompt}]} (3–12 cảnh).
//   - Ảnh minh họa: MediaGen (Gemini, xoay key) — 1 ảnh 16:9 / cảnh.
//   - Giọng đọc: Narrator (chuỗi TTS Gemini→VieNeu→Edge) — 1 wav / cảnh.
//   - Dựng: tái dùng AssemblePhotoList (Ken Burns) từng cảnh → ConcatClips →
//     mux audio TTS → MuxSubtitles (SRT từ thời lượng audio thật) →
//     MixMusicBed (tùy chọn) → 16:9 MP4.
//   - QC: 1920x1080 + có audio + dài > 30s. Rớt → failed + lý do tiếng Việt.
//
// Fail-closed: thiếu LLM / thiếu ảnh / thiếu giọng đọc → không dựng video
// câm hoặc video thiếu cảnh. Không đụng code cinematic/Veo (đã parked).
// ---------------------------------------------------------------------------

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// KindStory là job kind cho pipeline kể chuyện.

// StoryLogTail trả về dòng log cuối cùng của job — cho bảng /stories
// (ListJobs không select cột log để nhẹ).
func (s *Studio) StoryLogTail(id string) string {
	var lg string
	if err := s.db.QueryRow(`SELECT log FROM studio_jobs WHERE id=?`, id).Scan(&lg); err != nil {
		return ""
	}
	lg = strings.TrimSpace(lg)
	if lg == "" {
		return ""
	}
	lines := strings.Split(lg, "\n")
	return lines[len(lines)-1]
}

// DeleteStoryJob xoá 1 job kể chuyện: assets, file output, thư mục work,
// các row DB. Chỉ xoá job kind=story (không đụng job kind khác).
func (s *Studio) DeleteStoryJob(id string) error {
	j, ok := s.GetJob(id)
	if !ok {
		return fmt.Errorf("không tìm thấy job %s", id)
	}
	if j.Kind != KindStory {
		return fmt.Errorf("job %s không phải truyện kể", id)
	}
	for _, a := range s.ListAssets(id) {
		os.Remove(a.Path)
	}
	if j.Output != "" {
		os.Remove(j.Output)
	}
	os.RemoveAll(filepath.Join(s.workRoot, id))
	if _, err := s.db.Exec(`DELETE FROM studio_assets WHERE job_id=?`, id); err != nil {
		return fmt.Errorf("xoá assets: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM studio_jobs WHERE id=? AND kind=?`, id, KindStory); err != nil {
		return fmt.Errorf("xoá job: %w", err)
	}
	return nil
}

const KindStory = "story"

// StoryParams mô tả 1 job kể chuyện.
type StoryParams struct {
	Topic     string `json:"topic"`
	Genre     string `json:"genre"`
	Words     int    `json:"words"`      // độ dài truyện mục tiêu (từ)
	Scenes    int    `json:"scenes"`     // số cảnh mục tiêu (3–12)
	MusicOn   bool   `json:"music_on"`   // lồng nhạc nền
	MusicPath string `json:"music_path"` // file nhạc local ("" = không nhạc)
	AccountID int64  `json:"account_id,omitempty"`
	// Attempt: lần thử thứ mấy (0 = lần đầu). Đợt P: retry tự động tăng
	// dần; đạt story.retry_max thì dừng để không đốt quota vô ích.
	Attempt int `json:"attempt,omitempty"`
}

// storyScene là 1 cảnh: đoạn văn kể + prompt vẽ ảnh.
type storyScene struct {
	Text        string `json:"text"`
	ImagePrompt string `json:"image_prompt"`
}

// storyDoc là truyện đã viết.
type storyDoc struct {
	Title   string `json:"title"`
	Logline string `json:"logline"`
	Text    string `json:"text"`
}

// CreateStoryJob kiểm tra tham số, xếp job kể chuyện và chạy nền.
func (s *Studio) CreateStoryJob(p StoryParams) (string, error) {
	if strings.TrimSpace(p.Topic) == "" {
		return "", fmt.Errorf("cần chủ đề truyện")
	}
	if p.Words < 200 {
		p.Words = 450
	}
	if p.Words > 1200 {
		p.Words = 1200
	}
	if p.Scenes < 3 {
		p.Scenes = 5
	}
	if p.Scenes > 12 {
		p.Scenes = 12
	}
	title := "Kể chuyện: " + strings.TrimSpace(p.Topic)
	if len(title) > 90 {
		title = string([]rune(title)[:90]) + "…"
	}
	id, err := s.insertJob(KindStory, title, p)
	if err != nil {
		return "", err
	}
	go s.runStory(id, p)
	return id, nil
}

// GenerateTopics nhờ LLM nghĩ ra n chủ đề truyện mới theo thể loại.
// Dùng để tự refill hàng đợi khi Ninh không nhập tay (zero-touch).
// Nil LLM → lỗi rõ ràng (caller giữ note trung thực, không bịa chủ đề).
func (s *Studio) GenerateTopics(ctx context.Context, genre string, n int) ([]string, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("cần LLM để nghĩ chủ đề (chưa cấu hình)")
	}
	if n < 1 {
		n = 5
	}
	if n > 20 {
		n = 20
	}
	genre = strings.TrimSpace(genre)
	if genre == "" {
		genre = "tâm lý"
	}
	text, err := s.llm.Complete(ctx,
		"Bạn là biên tập truyện Việt Nam. Chỉ trả lời JSON thuần, không giải thích.",
		fmt.Sprintf(`Nghĩ ra đúng %d chủ đề truyện ngắn ngôi thứ nhất, thể loại %s, `+
			`hợp gu khán giả Việt Nam xem YouTube (đời thường, cảm xúc, kịch tính vừa phải, `+
			`không kinh dị máu me, không chính trị). Mỗi chủ đề 1 dòng ngắn gọn, cụ thể, `+
			`khác nhau, không trùng lặp ý tưởng.\n\nChỉ trả JSON: {"topics": ["...", ...]}`,
			n, genre))
	if err != nil {
		return nil, fmt.Errorf("nghĩ chủ đề: %w", err)
	}
	var raw struct {
		Topics []string `json:"topics"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, fmt.Errorf("nghĩ chủ đề: %w", err)
	}
	var out []string
	for _, t := range raw.Topics {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nghĩ chủ đề: LLM không trả chủ đề nào")
	}
	return out, nil
}

// StoryJobs trả về các job kể chuyện mới nhất.
func (s *Studio) StoryJobs(limit int) []Job {
	var out []Job
	for _, j := range s.ListJobs(limit * 3) {
		if j.Kind == KindStory {
			out = append(out, j)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// writeStory dùng LLM viết truyện ngôi thứ nhất (tái dùng story.txt).
func (s *Studio) writeStory(ctx context.Context, p StoryParams) (*storyDoc, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("cần LLM để viết truyện (chưa cấu hình)")
	}
	prompt, err := directorPrompt("story.txt", promptData{
		Topic: p.Topic, Genre: p.Genre, StoryWords: p.Words,
		Orient: "Viết ở NGÔI THỨ NHẤT (xưng \"tôi\") từ đầu đến cuối",
		// Seconds chỉ để LLM ước lượng "độ thịt" truyện; thời lượng thật
		// đo từ audio TTS sau này.
		Seconds: p.Words / 3,
	})
	if err != nil {
		return nil, err
	}
	text, err := s.llm.Complete(ctx,
		"Bạn là nhà văn Việt Nam. Chỉ trả lời JSON thuần, không giải thích.", prompt)
	if err != nil {
		return nil, fmt.Errorf("viết truyện: %w", err)
	}
	var doc storyDoc
	if err := parseDirectorJSON(text, &doc); err != nil {
		return nil, fmt.Errorf("viết truyện: %w", err)
	}
	doc.Title = strings.TrimSpace(doc.Title)
	doc.Text = strings.TrimSpace(doc.Text)
	if doc.Text == "" {
		return nil, fmt.Errorf("viết truyện: truyện rỗng")
	}
	return &doc, nil
}

// splitScenes chia truyện thành các cảnh (đoạn văn + prompt ảnh).
func (s *Studio) splitScenes(ctx context.Context, p StoryParams, doc *storyDoc) ([]storyScene, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("cần LLM để chia cảnh (chưa cấu hình)")
	}
	sys := "Bạn là biên tập storyboard Việt Nam. Chỉ trả lời JSON thuần, không giải thích."
	prompt := fmt.Sprintf(`Chia truyện sau thành đúng %d cảnh theo trình tự thời gian.
Mỗi cảnh: 1 đoạn văn kể ở ngôi thứ nhất (lấy nguyên văn hoặc tóm sát từ truyện, 40–120 từ),
kèm 1 image_prompt tiếng Anh mô tả 1 khung hình minh họa điện ảnh cho cảnh đó
(ánh sáng, màu sắc, bố cục 16:9, nhân vật Việt Nam nếu có; không chữ trong ảnh).

Truyện: %s

Chỉ trả JSON: {"scenes": [{"text": "...", "image_prompt": "..."}, ...]}`,
		p.Scenes, doc.Text)
	text, err := s.llm.Complete(ctx, sys, prompt)
	if err != nil {
		return nil, fmt.Errorf("chia cảnh: %w", err)
	}
	var raw struct {
		Scenes []storyScene `json:"scenes"`
	}
	if err := parseDirectorJSON(text, &raw); err != nil {
		return nil, fmt.Errorf("chia cảnh: %w", err)
	}
	var scenes []storyScene
	for _, sc := range raw.Scenes {
		sc.Text = strings.TrimSpace(sc.Text)
		sc.ImagePrompt = strings.TrimSpace(sc.ImagePrompt)
		if sc.Text == "" || sc.ImagePrompt == "" {
			continue
		}
		scenes = append(scenes, sc)
	}
	if len(scenes) < 3 {
		return nil, fmt.Errorf("chia cảnh: chỉ được %d cảnh hợp lệ (cần ≥3)", len(scenes))
	}
	if len(scenes) > 12 {
		scenes = scenes[:12]
	}
	return scenes, nil
}

// concatWAVs nối các file wav thành 1 track audio m4a (aac).
func concatWAVs(ctx context.Context, wavs []string, outPath string) error {
	if len(wavs) == 0 {
		return fmt.Errorf("không có track giọng đọc")
	}
	if dir := filepath.Dir(outPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	var args []string
	var fc []string
	for i, w := range wavs {
		args = append(args, "-i", w)
		fc = append(fc, fmt.Sprintf("[%d:a]", i))
	}
	fc = append(fc, fmt.Sprintf("concat=n=%d:v=0:a=1[aout]", len(wavs)))
	args = append(args, "-filter_complex", strings.Join(fc, ""), "-map", "[aout]",
		"-c:a", "aac", "-b:a", "160k", outPath)
	return ffmpegRun(ctx, args...)
}

// buildSRT dựng SRT từ văn bản từng cảnh và thời lượng audio thật.
func buildSRT(scenes []storyScene, durs []float64) string {
	var sb strings.Builder
	t := 0.0
	n := 1
	for i, sc := range scenes {
		d := 2.0
		if i < len(durs) && durs[i] > 0.5 {
			d = durs[i]
		}
		fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n", n, storySRTTime(t), storySRTTime(t+d), sc.Text)
		t += d
		n++
	}
	return sb.String()
}

func storySRTTime(sec float64) string {
	ms := int(sec*1000 + 0.5)
	h, ms := ms/3600000, ms%3600000
	m, ms := ms/60000, ms%60000
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms%1000)
}

// runStory chạy pipeline kể chuyện trong goroutine nền.
func (s *Studio) runStory(id string, p StoryParams) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, id)
		s.mu.Unlock()
	}()

	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(id, StatusFailed, 100)
		s.appendLog(id, "LỖI: job bị hủy khi đang chờ lượt render")
		return
	}

	s.setStatus(id, StatusRunning, 5)
	fail := func(err error) {
		s.appendLog(id, "LỖI: "+err.Error())
		s.setStatus(id, StatusFailed, 100)
	}

	if s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media-gen chưa sẵn sàng (thiếu GEMINI_API_KEYS hoặc key lỗi)"))
		return
	}
	if s.narrator == nil {
		fail(fmt.Errorf("TTS chưa sẵn sàng (không có chuỗi giọng đọc)"))
		return
	}

	work := filepath.Join(s.workRoot, id)
	if err := os.MkdirAll(work, 0o755); err != nil {
		fail(err)
		return
	}

	// 1. Viết truyện.
	s.appendLog(id, "Nhà văn đang viết truyện…")
	doc, err := s.writeStory(ctx, p)
	if err != nil {
		fail(err)
		return
	}
	title := strings.TrimSpace(doc.Title)
	if title != "" {
		s.db.Exec(`UPDATE studio_jobs SET title=? WHERE id=?`, "Kể chuyện: "+title, id)
	}
	s.appendLog(id, fmt.Sprintf("Truyện: %s (%d ký tự)", firstLine(title, doc.Text), len(doc.Text)))
	s.setStatus(id, StatusRunning, 15)

	// 2. Chia cảnh.
	s.appendLog(id, "Chia cảnh + viết prompt ảnh…")
	scenes, err := s.splitScenes(ctx, p, doc)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(id, fmt.Sprintf("Storyboard: %d cảnh", len(scenes)))
	s.setStatus(id, StatusRunning, 25)

	// 3+4. Vẽ ảnh + đọc giọng từng cảnh (fail-closed từng cảnh).
	var clips []string
	var wavs []string
	var durs []float64
	for i, sc := range scenes {
		aid := s.addAsset(id, i, "photo", sc.ImagePrompt)
		s.setAsset(aid, StatusRunning, "")

		img := filepath.Join(work, fmt.Sprintf("scene-%02d.png", i))
		imgPrompt := sc.ImagePrompt + ". Cinematic illustration, 16:9 widescreen, " +
			"Vietnamese characters if any, no text, no watermark."
		s.appendLog(id, fmt.Sprintf("Vẽ ảnh cảnh %d/%d…", i+1, len(scenes)))
		if err := s.mg.GenerateImage(ctx, imgPrompt, nil, img); err != nil {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: vẽ ảnh lỗi (%v) — dừng, không dựng thiếu cảnh", i+1, err))
			return
		}

		s.appendLog(id, fmt.Sprintf("Đọc giọng cảnh %d/%d…", i+1, len(scenes)))
		wavBytes, err := s.narrator(ctx, sc.Text)
		if err != nil {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: TTS lỗi (%v) — dừng, không tạo video câm", i+1, err))
			return
		}
		wav := filepath.Join(work, fmt.Sprintf("scene-%02d.wav", i))
		if err := os.WriteFile(wav, wavBytes, 0o644); err != nil {
			s.setAsset(aid, StatusFailed, "")
			fail(err)
			return
		}
		dur := storyProbeDuration(ctx, wav)
		if dur < 1 {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: audio giọng đọc quá ngắn (%.1fs)", i+1, dur))
			return
		}
		wavs = append(wavs, wav)
		durs = append(durs, dur)

		// Clip Ken Burns cho cảnh (tái dùng AssemblePhotoList, khổ 16:9).
		clip := filepath.Join(work, fmt.Sprintf("scene-%02d.mp4", i))
		if err := AssemblePhotoList(ctx, []string{img}, dur, "", 0, clip, "16:9"); err != nil {
			s.setAsset(aid, StatusFailed, "")
			fail(fmt.Errorf("cảnh %d: dựng clip lỗi: %w", i+1, err))
			return
		}
		s.setAsset(aid, StatusDone, img)
		clips = append(clips, clip)
		s.setStatus(id, StatusRunning, 25+int(45*float64(i+1)/float64(len(scenes))))
	}

	// 5. Dựng: nối clip → mux audio TTS → subtitle → nhạc nền.
	s.appendLog(id, "Nối các cảnh…")
	videoOnly := filepath.Join(work, "video-only.mp4")
	if err := ConcatClips(ctx, clips, "16:9", videoOnly); err != nil {
		fail(fmt.Errorf("nối cảnh: %w", err))
		return
	}
	s.appendLog(id, "Ghép giọng đọc…")
	voice := filepath.Join(work, "voice.m4a")
	if err := concatWAVs(ctx, wavs, voice); err != nil {
		fail(fmt.Errorf("nối giọng đọc: %w", err))
		return
	}
	withAudio := filepath.Join(work, "with-audio.mp4")
	if err := ffmpegRun(ctx, "-i", videoOnly, "-i", voice,
		"-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "aac",
		"-shortest", withAudio); err != nil {
		fail(fmt.Errorf("ghép audio: %w", err))
		return
	}
	s.appendLog(id, "Thêm phụ đề…")
	subbed := filepath.Join(work, "subbed.mp4")
	if err := MuxSubtitles(ctx, withAudio, buildSRT(scenes, durs), subbed); err != nil {
		fail(fmt.Errorf("phụ đề: %w", err))
		return
	}
	final := filepath.Join(s.outDir, "studio-"+id+".mp4")
	if p.MusicOn && strings.TrimSpace(p.MusicPath) != "" {
		s.appendLog(id, "Lồng nhạc nền…")
		if err := MixMusicBed(ctx, subbed, p.MusicPath, final); err != nil {
			s.appendLog(id, "Nhạc nền lỗi: "+err.Error()+" (giữ bản không nhạc)")
			if err := os.Rename(subbed, final); err != nil {
				fail(err)
				return
			}
		}
	} else {
		if err := os.Rename(subbed, final); err != nil {
			fail(err)
			return
		}
	}

	// 6. QC: 1920x1080 + có audio + dài > 30s (số thật đo từ file).
	s.appendLog(id, "Kiểm tra chất lượng…")
	if w, h := ProbeDims(ctx, final); w != 1920 || h != 1080 {
		fail(fmt.Errorf("QC: khổ video %dx%d, cần 1920x1080", w, h))
		return
	}
	if !ProbeHasAudio(ctx, final) {
		fail(fmt.Errorf("QC: video thiếu audio"))
		return
	}
	if d := storyProbeDuration(ctx, final); d < 30 {
		fail(fmt.Errorf("QC: video chỉ dài %.0fs (cần > 30s)", d))
		return
	}
	dur := storyProbeDuration(ctx, final)
	s.appendLog(id, fmt.Sprintf("QC đạt: 1920x1080, có audio, %.0fs, %d cảnh", dur, len(scenes)))

	s.setOutput(id, final)
	s.setStatus(id, StatusDone, 100)
	s.appendLog(id, fmt.Sprintf("Xong: %s (%.0fs, %d cảnh)", filepath.Base(final), dur, len(scenes)))
	s.fireOnDone(id)
}

// probeDuration trả về thời lượng giây của file media (0 khi lỗi).
// (Bản unexported: ProbeDuration của film_shots.go nằm sau build tag
// parked — không đụng vào để tránh trùng symbol khi build -tags parked.)
func storyProbeDuration(ctx context.Context, path string) float64 {
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

func firstLine(title, text string) string {
	if title != "" {
		return title
	}
	if i := strings.Index(text, "\n"); i > 0 {
		return text[:i]
	}
	if len(text) > 60 {
		return string([]rune(text)[:60]) + "…"
	}
	return text
}
