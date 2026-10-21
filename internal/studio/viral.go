package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// KindViral là job thay nhận diện nhân vật trên video nguồn đã chọn.
const KindViral = "viral"

// ViralParams chứa cấu hình đầu vào để tạo 1 video viral.
type ViralParams struct {
	Topic            string               `json:"topic"`
	Genre            string               `json:"genre"`       // mystery, lifehack, trendjack, tech_fact, koc_review
	VisualMood       string               `json:"visual_mood"` // cinematic, vibrant_3d, realistic, cyber_neon
	Voice            string               `json:"voice"`       // TTS provider voice identifier
	Account          string               `json:"account"`     // username kênh để đọc Brand DNA
	AccountID        int64                `json:"account_id,omitempty"`
	ModelPhoto       string               `json:"model_photo,omitempty"`  // local file path to character face reference image
	ModelPhotos      []string             `json:"model_photos,omitempty"` // multi-angle face reference images for character locking
	TargetDuration   float64              `json:"target_duration"`        // exact source duration in seconds
	MusicURL         string               `json:"music_url"`              // Nhạc nền trending (tùy chọn)
	MusicFile        string               `json:"music_file"`
	SourceVideoURL   string               `json:"source_video_url,omitempty"`
	SampleVideo      string               `json:"sample_video,omitempty"` // local file path to sample motion video
	AIMode           string               `json:"ai_mode,omitempty"`      // full_character_edit replaces the complete person in source footage
	ReferenceSummary string               `json:"reference_summary,omitempty"`
	ReferenceBeats   []ViralReferenceBeat `json:"reference_beats,omitempty"`
}

// ViralReferenceBeat is one observed beat from sampled source frames.
type ViralReferenceBeat struct {
	TimeSec        float64 `json:"time_sec"`
	Framing        string  `json:"framing"`
	SubjectAction  string  `json:"subject_action"`
	BodyMovement   string  `json:"body_movement"`
	CameraMovement string  `json:"camera_movement"`
	EditTransition string  `json:"edit_transition"`
	Energy         string  `json:"energy"`
	Uncertainty    string  `json:"uncertainty,omitempty"`
}

// ViralScene mô tả 1 phân cảnh trong kịch bản video viral.
type ViralScene struct {
	Index          int     `json:"index"`
	Phase          string  `json:"phase"`        // source_beat
	VoiceText      string  `json:"voice_text"`   // Lời thoại lôi cuốn (tiếng Việt)
	ImagePrompt    string  `json:"image_prompt"` // Prompt tiếng Anh cho AI MediaGen vẽ ảnh 9:16
	VideoPrompt    string  `json:"video_prompt"` // Chuyển động tái hiện hành động/camera của nguồn
	VisualHook     string  `json:"visual_hook"`  // Text giật tít hiển thị trên màn hình
	DurationSec    float64 `json:"duration_sec"`
	SourceStartSec float64 `json:"source_start_sec"`
	SourceEndSec   float64 `json:"source_end_sec"`
}

// ViralScript là kịch bản hoàn chỉnh của một video viral.
type ViralScript struct {
	Title        string       `json:"title"`
	Hook3s       string       `json:"hook_3s"`
	CoreAngle    string       `json:"core_angle"`
	Hashtags     []string     `json:"hashtags"`
	CallToAction string       `json:"call_to_action"`
	Scenes       []ViralScene `json:"scenes"`
}

// EnqueueViral đưa một tác vụ tạo video viral vào hàng đợi render của Studio.
func (s *Studio) EnqueueViral(ctx context.Context, p ViralParams) (string, error) {
	return "", fmt.Errorf("%s", viralPaidVideoDisabledMessage)
}

// ListViralJobs trả về danh sách các job viral đã và đang sản xuất.
func (s *Studio) ListViralJobs(limit int) []Job {
	if limit <= 0 {
		limit = 30
	}
	jobs := s.ListJobs(limit)
	var out []Job
	for _, j := range jobs {
		if j.Kind == KindViral {
			out = append(out, j)
		}
	}
	return out
}

// GenerateViralScript writes a source-grounded storyboard. It refuses to create
// content when the source video or channel character evidence is missing.
func (s *Studio) GenerateViralScript(ctx context.Context, p ViralParams) (*ViralScript, error) {
	duration := p.TargetDuration
	if fileExists(p.SampleVideo) {
		measured := probeDuration(ctx, p.SampleVideo)
		if measured <= 0 {
			return nil, fmt.Errorf("không đọc được thời lượng video nguồn")
		}
		duration = measured
	}
	if duration <= 0 {
		return nil, fmt.Errorf("cần thời lượng video nguồn chính xác")
	}
	p.TargetDuration = duration

	if p.ModelPhoto == "" {
		for _, photo := range p.ModelPhotos {
			if fileExists(photo) {
				p.ModelPhoto = photo
				break
			}
		}
	}
	if !fileExists(p.ModelPhoto) {
		return nil, fmt.Errorf("cần ảnh nhân vật đã chọn của kênh để lập storyboard tái hiện")
	}
	if len(p.ReferenceBeats) == 0 || strings.TrimSpace(p.ReferenceSummary) == "" {
		summary, beats, err := s.analyzeViralReference(ctx, p.SampleVideo, duration)
		if err != nil {
			return nil, err
		}
		p.ReferenceSummary = summary
		p.ReferenceBeats = beats
	}
	if err := validateViralReferenceBeats(p.ReferenceBeats, duration); err != nil {
		return nil, err
	}
	if s.llm == nil || !s.llm.Healthy(ctx) {
		return nil, fmt.Errorf("không có LLM khả dụng để chuyển phân tích video nguồn thành storyboard")
	}

	beats, err := json.MarshalIndent(p.ReferenceBeats, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("không mã hóa được beat video nguồn: %w", err)
	}
	prompt, err := directorPrompt("viral_recreate.txt", promptData{
		Topic:            p.Topic,
		Niche:            p.Account,
		Seconds:          int(math.Round(duration)),
		Duration:         formatViralDuration(duration),
		ReferenceSummary: strings.TrimSpace(p.ReferenceSummary),
		ReferenceBeats:   string(beats),
	})
	if err != nil {
		return nil, err
	}
	response, err := s.llm.Complete(ctx, "Bạn là đạo diễn tái hiện video theo nguồn đã phân tích. Chỉ trả JSON thuần, không thêm nội dung.", prompt)
	if err != nil {
		return nil, fmt.Errorf("viết storyboard tái hiện nguồn: %w", err)
	}

	var script ViralScript
	if err := json.Unmarshal([]byte(extractJSON(response)), &script); err != nil {
		return nil, fmt.Errorf("storyboard trả JSON không hợp lệ: %w", err)
	}
	if strings.TrimSpace(script.Title) == "" {
		return nil, fmt.Errorf("storyboard thiếu tiêu đề mô tả nguồn")
	}
	if err := validateViralReferenceScenes(script.Scenes, duration); err != nil {
		return nil, err
	}

	// The available source evidence is visual-only. Never infer audio, text,
	// marketing copy, or engagement assets from sampled frames.
	script.Hook3s = ""
	script.CallToAction = ""
	script.Hashtags = nil
	for i := range script.Scenes {
		script.Scenes[i].VoiceText = ""
		script.Scenes[i].VisualHook = ""
	}
	return &script, nil
}

func (s *Studio) analyzeViralReference(ctx context.Context, videoPath string, duration float64) (string, []ViralReferenceBeat, error) {
	if !fileExists(videoPath) {
		return "", nil, fmt.Errorf("cần video nguồn để quan sát thứ tự cảnh và chuyển động")
	}
	if s.mg == nil || !s.mg.Healthy(ctx) {
		return "", nil, fmt.Errorf("media engine chưa sẵn sàng để phân tích video nguồn")
	}
	analyzer, ok := s.mg.(visualImageAnalyzer)
	if !ok {
		return "", nil, fmt.Errorf("media engine chưa hỗ trợ phân tích frame video nguồn")
	}
	workDir := filepath.Join(s.workRoot, "viral-reference-"+newID())
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("tạo thư mục phân tích video nguồn: %w", err)
	}
	defer os.RemoveAll(workDir)

	frames, err := sampleViralReferenceFrames(ctx, videoPath, workDir, duration)
	if err != nil {
		return "", nil, err
	}
	var prompt strings.Builder
	prompt.WriteString("Analyze these ordered samples from one source video. Images are point samples, not continuous footage. Describe only visible evidence; do not infer unseen choreography, speech, sound, text, or intent. Group observations into chronological beats. Give each beat a source time_sec and concise framing, subject_action, body_movement, camera_movement, edit_transition, energy, and uncertainty. Mark motion between samples uncertain when frames do not prove it. Return JSON only: {summary:string, beats:[{time_sec:number, framing:string, subject_action:string, body_movement:string, camera_movement:string, edit_transition:string, energy:string, uncertainty:string}]}\n")
	for i, frame := range frames {
		fmt.Fprintf(&prompt, "Image %d shows source time %.3f seconds.\n", i+1, frame.TimeSec)
	}
	refs := make([]ImageRef, 0, len(frames))
	for _, frame := range frames {
		refs = append(refs, ImageRef{Path: frame.Path})
	}
	raw, err := analyzer.AnalyzeImages(ctx, prompt.String(), refs)
	if err != nil {
		return "", nil, fmt.Errorf("phân tích frame video nguồn: %w", err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal([]byte(extractJSON(raw)), &result); err != nil {
		return "", nil, fmt.Errorf("phân tích video nguồn trả JSON lỗi: %w", err)
	}
	var summary string
	var beats []ViralReferenceBeat
	if err := json.Unmarshal(result["summary"], &summary); err != nil {
		return "", nil, fmt.Errorf("phân tích video nguồn thiếu summary hợp lệ: %w", err)
	}
	if err := json.Unmarshal(result["beats"], &beats); err != nil {
		return "", nil, fmt.Errorf("phân tích video nguồn thiếu beats hợp lệ: %w", err)
	}
	if strings.TrimSpace(summary) == "" {
		return "", nil, fmt.Errorf("phân tích video nguồn thiếu tóm tắt quan sát")
	}
	if err := validateViralReferenceBeats(beats, duration); err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(summary), beats, nil
}

func validateViralReferenceBeats(beats []ViralReferenceBeat, duration float64) error {
	if len(beats) == 0 {
		return fmt.Errorf("thiếu beat quan sát từ video nguồn")
	}
	lastTime := -1.0
	for i, beat := range beats {
		if beat.TimeSec < 0 || beat.TimeSec > duration || beat.TimeSec <= lastTime {
			return fmt.Errorf("beat nguồn %d có timestamp thiếu, ngoài video hoặc sai thứ tự", i+1)
		}
		if strings.TrimSpace(beat.Framing+beat.SubjectAction+beat.BodyMovement+beat.CameraMovement+beat.EditTransition+beat.Energy) == "" {
			return fmt.Errorf("beat nguồn %d thiếu mô tả hình ảnh quan sát được", i+1)
		}
		lastTime = beat.TimeSec
	}
	return nil
}

func validateViralReferenceScenes(scenes []ViralScene, target float64) error {
	if len(scenes) == 0 {
		return fmt.Errorf("storyboard nguồn không có cảnh")
	}
	if target < 4 || target > float64(len(scenes))*8 {
		return fmt.Errorf("%d cảnh không thể phủ đúng %.3f giây với clip 4-8 giây/cảnh", len(scenes), target)
	}
	cursor := 0.0
	for i := range scenes {
		scene := &scenes[i]
		if strings.TrimSpace(scene.ImagePrompt) == "" || strings.TrimSpace(scene.VideoPrompt) == "" {
			return fmt.Errorf("storyboard cảnh %d thiếu keyframe hoặc chuyển động theo nguồn", i+1)
		}
		if math.Abs(scene.SourceStartSec-cursor) > 0.01 {
			return fmt.Errorf("storyboard cảnh %d tạo khoảng trống hoặc chồng lấn với timeline nguồn", i+1)
		}
		if scene.SourceEndSec <= scene.SourceStartSec {
			return fmt.Errorf("storyboard cảnh %d có khoảng timestamp không hợp lệ", i+1)
		}
		sceneDuration := scene.SourceEndSec - scene.SourceStartSec
		if sceneDuration < 4-0.01 || sceneDuration > 8+0.01 {
			return fmt.Errorf("storyboard cảnh %d dài %.3f giây; giới hạn nguồn là 4-8 giây", i+1, sceneDuration)
		}
		scene.Index = i + 1
		scene.Phase = "source_beat"
		scene.DurationSec = sceneDuration
		cursor = scene.SourceEndSec
	}
	if math.Abs(cursor-target) > 0.01 {
		return fmt.Errorf("timeline storyboard kết thúc ở %.3f giây, nguồn dài %.3f giây", cursor, target)
	}
	return nil
}

func formatViralDuration(seconds float64) string {
	formatted := strconv.FormatFloat(seconds, 'f', 3, 64)
	return strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
}

// runViral runs full-character source editing on the selected viral video.
func (s *Studio) runViral(jobID string, p ViralParams) {
	defer func() {
		if r := recover(); r != nil {
			s.setStatus(jobID, StatusFailed, 100)
			s.appendLog(jobID, fmt.Sprintf("LỖI panic (%v) — job dừng an toàn.", r))
		}
	}()
	s.setStatus(jobID, StatusFailed, 100)
	s.appendLog(jobID, "LỖI: "+viralPaidVideoDisabledMessage)
}

func (s *Studio) runViralFullCharacterEdit(jobID string, p ViralParams) {
	s.appendLog(jobID, "SOURCE_VALIDATION_START: kiểm tra file nguồn, kênh và engine")
	sourceDuration := probeDuration(context.Background(), p.SampleVideo)
	if sourceDuration <= 0 {
		s.setStatus(jobID, StatusFailed, 100)
		s.appendLog(jobID, "LỖI: không đọc được thời lượng video nguồn")
		return
	}
	width, height, err := probeViralDimensions(context.Background(), p.SampleVideo)
	if err != nil {
		s.setStatus(jobID, StatusFailed, 100)
		s.appendLog(jobID, "LỖI: "+err.Error())
		return
	}
	if math.Abs(float64(width)/float64(height)-9.0/16.0) > 0.015 {
		s.setStatus(jobID, StatusFailed, 100)
		s.appendLog(jobID, fmt.Sprintf("LỖI: video nguồn %dx%d không phải khung dọc 9:16; dừng để giữ nguyên framing.", width, height))
		return
	}

	timeout := 20 * time.Minute
	durationBudget := time.Duration(sourceDuration*10)*time.Second + 20*time.Minute
	if durationBudget > timeout {
		timeout = durationBudget
	}
	if timeout > 6*time.Hour {
		timeout = 6 * time.Hour
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	s.mu.Lock()
	s.running[jobID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, jobID)
		s.mu.Unlock()
	}()

	sem := s.jobSlot()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		s.setStatus(jobID, StatusFailed, 100)
		s.appendLog(jobID, "LỖI: job bị hủy khi đang chờ hàng đợi render")
		return
	}

	fail := func(err error) {
		s.appendLog(jobID, "LỖI: "+err.Error())
		s.setStatus(jobID, StatusFailed, 100)
	}
	s.setStatus(jobID, StatusRunning, 5)
	s.appendLog(jobID, fmt.Sprintf("Viral full-character edit: giữ nguồn %dx%d, %.2fs; thay toàn bộ nhân vật và giữ chuỗi cảnh/chuyển động/camera.", width, height, sourceDuration))
	if !fileExists(p.SampleVideo) || !fileExists(p.ModelPhoto) {
		fail(fmt.Errorf("thiếu video nguồn hoặc ảnh nhân vật của kênh"))
		return
	}
	editor, editorReady := s.mg.(ViralCharacterVideoEditor)
	if !editorReady || s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("Gemini Omni video-edit adapter chưa sẵn sàng"))
		return
	}
	if _, ok := s.mg.(visualImageAnalyzer); !ok || s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media engine phân tích frame chưa sẵn sàng để kiểm tra nhận diện nhân vật"))
		return
	}
	s.appendLog(jobID, "SOURCE_VALIDATION_PASS: video nguồn, ảnh kênh, Gemini Omni adapter và visual analyzer sẵn sàng")

	jobDir := filepath.Join(s.workRoot, jobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		fail(fmt.Errorf("tạo thư mục làm việc thất bại: %w", err))
		return
	}
	if err := os.MkdirAll(s.outDir, 0o755); err != nil {
		fail(fmt.Errorf("tạo thư mục đầu ra thất bại: %w", err))
		return
	}
	finalVideo := filepath.Join(s.outDir, fmt.Sprintf("viral_%s.mp4", jobID))
	_ = os.Remove(finalVideo)
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(finalVideo)
		}
	}()

	modelPhotos := append([]string(nil), p.ModelPhotos...)
	if len(modelPhotos) == 0 {
		modelPhotos = []string{p.ModelPhoto}
	}
	modelPhotos = validCharacterPhotos(modelPhotos, 4)
	segments, err := splitViralSourceForEditing(ctx, p.SampleVideo, sourceDuration, jobDir)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(jobID, fmt.Sprintf("CHARACTER_REPLACEMENT_START: thay toàn bộ nhân vật qua %d đoạn, tối đa 10s mỗi lần edit", len(segments)))
	var editedSegments []string
	for i, segment := range segments {
		edited := filepath.Join(jobDir, fmt.Sprintf("character-%02d.mp4", i+1))
		if err := editor.EditCharacterVideoSegment(ctx, segment.Path, modelPhotos, edited); err != nil {
			fail(fmt.Errorf("Gemini Omni edit đoạn %d/%d: %w", i+1, len(segments), err))
			return
		}
		got := probeDuration(ctx, edited)
		if got <= 0 || math.Abs(got-segment.Duration) > 0.25 {
			fail(fmt.Errorf("Gemini Omni đổi thời lượng đoạn %d: nguồn %.2fs, đầu ra %.2fs; dừng để không lệch nhịp choreography", i+1, segment.Duration, got))
			return
		}
		withSourceAudio := filepath.Join(jobDir, fmt.Sprintf("character-audio-%02d.mp4", i+1))
		if err := ffmpegRun(ctx, "-y", "-i", edited, "-i", segment.Path, "-map", "0:v:0", "-map", "1:a?", "-c:v", "copy", "-c:a", "aac", "-shortest", withSourceAudio); err != nil {
			fail(fmt.Errorf("giữ audio nguồn đoạn %d: %w", i+1, err))
			return
		}
		editedSegments = append(editedSegments, withSourceAudio)
		s.setStatus(jobID, StatusRunning, 5+int(70*float64(i+1)/float64(len(segments))))
	}
	if err := ConcatClips(ctx, editedSegments, "9:16", finalVideo); err != nil {
		fail(fmt.Errorf("nối các đoạn nhân vật đã thay: %w", err))
		return
	}
	s.appendLog(jobID, "CHARACTER_REPLACEMENT_PASS: toàn bộ đoạn đã edit và audio nguồn đã ghép lại")
	s.appendLog(jobID, "OUTPUT_VALIDATION_START: đối chiếu thời lượng và kích thước với nguồn")
	if err := verifyVideoDuration(ctx, finalVideo, sourceDuration); err != nil {
		fail(fmt.Errorf("video đầu ra không giữ đúng thời lượng nguồn: %w", err))
		return
	}
	outWidth, outHeight, err := probeViralDimensions(ctx, finalVideo)
	if err != nil || math.Abs(float64(outWidth)/float64(outHeight)-9.0/16.0) > 0.015 {
		fail(fmt.Errorf("đầu ra không giữ khung dọc 9:16 (%dx%d)", outWidth, outHeight))
		return
	}
	s.appendLog(jobID, fmt.Sprintf("OUTPUT_VALIDATION_PASS: thời lượng khớp nguồn; khung đầu ra %dx%d 9:16", outWidth, outHeight))

	s.setStatus(jobID, StatusRunning, 85)
	if err := s.runViralVisualGate(ctx, jobID, finalVideo, jobDir, p, modelPhotos); err != nil {
		fail(err)
		return
	}
	s.setCaption(jobID, strings.TrimSpace(p.Topic))
	s.setOutput(jobID, finalVideo)
	s.setStatus(jobID, StatusDone, 100)
	s.appendLog(jobID, fmt.Sprintf("Hoàn tất full-character edit: %.2fs; visual QC cảnh, choreography và danh tính đã đạt.", sourceDuration))
	complete = true
	s.fireOnDone(jobID)
}

type viralEditSegment struct {
	Path     string
	Duration float64
}

func splitViralSourceForEditing(ctx context.Context, source string, duration float64, workDir string) ([]viralEditSegment, error) {
	if duration < 3 {
		return nil, fmt.Errorf("video nguồn phải dài ít nhất 3s để dùng Gemini Omni edit")
	}
	const maxEditSeconds = 10.0
	count := int(math.Ceil(duration / maxEditSeconds))
	if count < 1 {
		count = 1
	}
	segmentSeconds := duration / float64(count)
	if segmentSeconds < 3 || segmentSeconds > maxEditSeconds {
		return nil, fmt.Errorf("không chia được video %.2fs thành đoạn edit 3-10s", duration)
	}
	segments := make([]viralEditSegment, 0, count)
	for i := 0; i < count; i++ {
		start := float64(i) * segmentSeconds
		segmentDuration := segmentSeconds
		if i == count-1 {
			segmentDuration = duration - start
		}
		path := filepath.Join(workDir, fmt.Sprintf("source-segment-%02d.mp4", i+1))
		if err := ffmpegRun(ctx, "-y", "-ss", fmt.Sprintf("%.3f", start), "-i", source,
			"-t", fmt.Sprintf("%.3f", segmentDuration), "-map", "0:v:0", "-map", "0:a?",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "160k", "-avoid_negative_ts", "make_zero", path); err != nil {
			return nil, fmt.Errorf("tách đoạn nguồn %d/%d: %w", i+1, count, err)
		}
		segments = append(segments, viralEditSegment{Path: path, Duration: segmentDuration})
	}
	return segments, nil
}

func probeViralDimensions(ctx context.Context, videoPath string) (int, int, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height", "-of", "csv=s=x:p=0", videoPath)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("đọc kích thước video thất bại: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("ffprobe trả kích thước video không hợp lệ")
	}
	width, wErr := strconv.Atoi(parts[0])
	height, hErr := strconv.Atoi(parts[1])
	if wErr != nil || hErr != nil || width <= 0 || height <= 0 {
		return 0, 0, fmt.Errorf("ffprobe trả kích thước video không hợp lệ")
	}
	return width, height, nil
}

// isDanceOrPerformance kiểm tra chủ đề hoặc thể loại có phải là vũ đạo/nhảy/dance không.
func isDanceOrPerformance(topic, genre string) bool {
	t := strings.ToLower(topic)
	g := strings.ToLower(genre)
	return g == "dance" ||
		strings.Contains(t, "nhảy") ||
		strings.Contains(t, "dance") ||
		strings.Contains(t, "vũ đạo") ||
		strings.Contains(t, "sexy") ||
		strings.Contains(t, "choreo")
}

// createSolidColorPhoto tạo 1 ảnh nền mẫu chuẩn 9:16 hợp lệ (PNG) cho trường hợp offline, test hoặc quota hết.
func (s *Studio) createSolidColorPhoto(outPath string, w, h int, text string) error {
	_ = text
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 1920
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Đổ nền gradient dọc tinh tế từ xanh đậm sang đen tạo bối cảnh điện ảnh
	for y := 0; y < h; y++ {
		factor := float64(y) / float64(h)
		r := uint8(15 + factor*10)
		g := uint8(23 + factor*15)
		b := uint8(42 + factor*20)
		col := color.RGBA{R: r, G: g, B: b, A: 255}
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, col)
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func copyViralFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// ViralOriginalProfile lưu trữ đặc tính kỹ thuật & phong cách của video viral gốc để phản chiếu sang video AI.
type ViralOriginalProfile struct {
	Path           string  `json:"path"`
	Duration       float64 `json:"duration"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	HasAudio       bool    `json:"has_audio"`
	AudioExtracted string  `json:"audio_extracted,omitempty"`
	MotionVibe     string  `json:"motion_vibe"` // "energetic_beat", "smooth_sensual", "dynamic_street", "natural_lookbook"
	VisualMood     string  `json:"visual_mood"` // "neon_aesthetic", "warm_studio", "natural_daylight", "cinematic_dramatic"
	PacingSec      float64 `json:"pacing_sec"`
}

type viralSampleFrame struct {
	Path    string
	TimeSec float64
}

func isViralVideoFile(path string) bool {
	if path == "" || !fileExists(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mov", ".webm", ".mkv", ".avi":
		return true
	default:
		return false
	}
}

func sampleViralReferenceFrames(ctx context.Context, videoPath, outDir string, duration float64) ([]viralSampleFrame, error) {
	if duration <= 0 {
		return nil, fmt.Errorf("không đọc được thời lượng video")
	}
	count := int(math.Ceil(duration * 2)) // two ordered source samples per second
	if count < 8 {
		count = 8
	}
	if count > 120 {
		count = 120
	}
	frames := make([]viralSampleFrame, 0, count)
	for i := 0; i < count; i++ {
		at := duration * (float64(i) + 0.5) / float64(count)
		path := filepath.Join(outDir, fmt.Sprintf("viral-reference-%02d.jpg", i+1))
		if err := ffmpegRun(ctx, "-ss", fmt.Sprintf("%.3f", at), "-i", videoPath, "-vf", "scale=640:-2", "-frames:v", "1", "-q:v", "7", path); err != nil || !fileExists(path) {
			continue
		}
		frames = append(frames, viralSampleFrame{Path: path, TimeSec: at})
	}
	if len(frames) < 4 {
		return nil, fmt.Errorf("chỉ trích được %d frame; cần ít nhất 4", len(frames))
	}
	return frames, nil
}

// AnalyzeViralOriginal reads technical metadata. Motion, mood, and pacing
// remain unknown until an image/video analyzer measures them.
func AnalyzeViralOriginal(ctx context.Context, samplePath, outDir string) *ViralOriginalProfile {
	prof := &ViralOriginalProfile{
		Path:     samplePath,
		Duration: probeDuration(ctx, samplePath),
		HasAudio: ProbeHasAudio(ctx, samplePath),
	}

	w, h := ProbeDims(ctx, samplePath)
	prof.Width = w
	prof.Height = h

	// Trích xuất âm thanh gốc nếu video có audio stream
	if prof.HasAudio && outDir != "" {
		audioFile := filepath.Join(outDir, "viral_orig_audio.m4a")
		if err := ffmpegRun(ctx, "-i", samplePath, "-vn", "-c:a", "aac", "-b:a", "192k", audioFile); err == nil && fileExists(audioFile) {
			prof.AudioExtracted = audioFile
		}
	}

	return prof
}
