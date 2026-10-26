package studio

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func viralFlowEditPrompt(totalDuration float64, segment viralEditSegment, index, count int) string {
	return fmt.Sprintf(`Cinematic vertical 9:16 video of the adult character in the reference images performing an energetic modern dance choreography segment (%d of %d). Fluid body movement, dynamic rhythmic dance gestures, natural poses, expressive smiling face, atmospheric lighting, smooth motion, high fidelity, 4k quality. Single subject, preserve consistent character appearance from references, no extra people, no text, no subtitles, no blur.`, index, count)
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
	if ready, reason := s.ViralCharacterEditorStatus(ctx); !ready {
		fail(fmt.Errorf("Flow video edit chưa sẵn sàng: %s", reason))
		return
	}
	if _, ok := s.mg.(visualImageAnalyzer); !ok || s.mg == nil || !s.mg.Healthy(ctx) {
		fail(fmt.Errorf("media engine phân tích frame chưa sẵn sàng để kiểm tra nhận diện nhân vật"))
		return
	}
	s.appendLog(jobID, "SOURCE_VALIDATION_PASS: video nguồn, ảnh kênh, Google Flow edit và visual analyzer sẵn sàng")

	jobDir := filepath.Join(s.workRoot, jobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		fail(fmt.Errorf("tạo thư mục làm việc thất bại: %w", err))
		return
	}
	if err := os.MkdirAll(s.outDir, 0o755); err != nil {
		fail(fmt.Errorf("tạo thư mục đầu ra thất bại: %w", err))
		return
	}
	rawVideo := filepath.Join(jobDir, "viral-concat.mp4")
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
	modelPhotos = validCharacterPhotos(modelPhotos, 3)
	segments, err := splitViralSourceForEditing(ctx, p.SampleVideo, sourceDuration, jobDir)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(jobID, fmt.Sprintf("CHARACTER_REPLACEMENT_START: thay toàn bộ nhân vật qua %d đoạn, tối đa 10s mỗi lần edit", len(segments)))
	editedSegments := make([]string, 0, len(segments))
	segmentDurations := make([]float64, 0, len(segments))
	flowProjectID := ""
	for i, segment := range segments {
		edited := filepath.Join(jobDir, fmt.Sprintf("character-%02d.mp4", i+1))
		prompt := viralFlowEditPrompt(sourceDuration, segment, i+1, len(segments))

		// Sử dụng ảnh nhân vật kênh làm reference ingredients cho Flow (tối đa 3 ảnh)
		segmentIngredients := make([]string, 0, 3)
		for _, pth := range modelPhotos {
			if len(segmentIngredients) < 3 {
				segmentIngredients = append(segmentIngredients, pth)
			}
		}

		projectID, err := s.GenerateWebAIVideo(ctx, AffiliateVideoEngineGoogleFlow, WebAIVideoRequest{
			Mode: "ingredients", Prompt: prompt, AspectRatio: "VIDEO_ASPECT_RATIO_PORTRAIT",
			ImagePaths: segmentIngredients, ProjectID: flowProjectID,
		}, edited, func(line string) {
			if strings.TrimSpace(line) != "" {
				s.appendLog(jobID, line)
			}
		})
		if err != nil {
			fail(fmt.Errorf("Flow edit đoạn %d/%d: %w", i+1, len(segments), err))
			return
		}
		if projectID != "" {
			flowProjectID = projectID
		}
		got := probeDuration(ctx, edited)
		if got <= 0 {
			fail(fmt.Errorf("Flow không tạo được video hợp lệ cho đoạn %d", i+1))
			return
		}

		// Đồng bộ thời lượng đoạn edit khớp với đoạn nguồn để choreography và audio không lệch
		conformedClip := edited
		if math.Abs(got-segment.Duration) > 0.05 {
			conformedClip = filepath.Join(jobDir, fmt.Sprintf("character-conformed-%02d.mp4", i+1))
			if err := ffmpegRun(ctx, "-y", "-i", edited, "-t", fmt.Sprintf("%.3f", segment.Duration), "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p", conformedClip); err != nil {
				fail(fmt.Errorf("đồng bộ thời lượng đoạn %d: %w", i+1, err))
				return
			}
		}
		withSourceAudio := filepath.Join(jobDir, fmt.Sprintf("character-audio-%02d.mp4", i+1))
		if err := ffmpegRun(ctx, "-y", "-i", conformedClip, "-i", segment.Path, "-map", "0:v:0", "-map", "1:a?", "-c:v", "copy", "-c:a", "aac", "-shortest", withSourceAudio); err != nil {
			fail(fmt.Errorf("giữ audio nguồn đoạn %d: %w", i+1, err))
			return
		}
		editedSegments = append(editedSegments, withSourceAudio)
		segmentDurations = append(segmentDurations, segment.Duration)
		s.setStatus(jobID, StatusRunning, 5+int(70*float64(i+1)/float64(len(segments))))
	}
	resolution, err := NormalizeVideoOutputResolution(p.OutputResolution)
	if err != nil {
		fail(err)
		return
	}
	s.appendLog(jobID, fmt.Sprintf("REMASTER_START: ghép các đoạn và upscale một lượt lên %s · giữ toàn bộ khung hình", strings.ToUpper(resolution)))
	if err := ConcatClipsToMaster(ctx, editedSegments, segmentDurations, "9:16", resolution, rawVideo, true); err != nil {
		fail(fmt.Errorf("nối các đoạn nhân vật đã thay: %w", err))
		return
	}
	s.appendLog(jobID, "CHARACTER_REPLACEMENT_PASS: toàn bộ đoạn đã edit và audio nguồn đã ghép lại")
	s.appendLog(jobID, "OUTPUT_VALIDATION_START: đối chiếu thời lượng và kích thước với nguồn")
	if err := verifyVideoDuration(ctx, rawVideo, sourceDuration); err != nil {
		fail(fmt.Errorf("video đầu ra không giữ đúng thời lượng nguồn: %w", err))
		return
	}
	if err := UpscaleVideo(ctx, rawVideo, finalVideo, "9:16", resolution); err != nil {
		fail(fmt.Errorf("upscale master %s: %w", strings.ToUpper(resolution), err))
		return
	}
	if !ProbeHasAudio(ctx, finalVideo) {
		fail(fmt.Errorf("đầu ra viral bị mất audio từ video Radar"))
		return
	}
	outWidth, outHeight, err := probeViralDimensions(ctx, finalVideo)
	wantWidth, wantHeight, _ := VideoOutputDims("9:16", resolution)
	if err != nil || outWidth != wantWidth || outHeight != wantHeight || math.Abs(float64(outWidth)/float64(outHeight)-9.0/16.0) > 0.015 {
		fail(fmt.Errorf("đầu ra không đạt %s 9:16 (%dx%d)", strings.ToUpper(resolution), outWidth, outHeight))
		return
	}
	s.appendLog(jobID, fmt.Sprintf("OUTPUT_VALIDATION_PASS: thời lượng khớp nguồn; master %s %dx%d 9:16", strings.ToUpper(resolution), outWidth, outHeight))

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
