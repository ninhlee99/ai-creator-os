package studio

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

func (s *Studio) runViralFullCharacterEdit(jobID string, p ViralParams) {
	s.appendLog(jobID, "SOURCE_VALIDATION_START: kiểm tra video nguồn, ảnh nhân vật, Muse AI và visual QC")
	sourceDuration := probeDuration(context.Background(), p.SampleVideo)
	if sourceDuration <= 0 {
		s.failViralJob(jobID, fmt.Errorf("không đọc được thời lượng video nguồn"))
		return
	}
	width, height, err := probeViralDimensions(context.Background(), p.SampleVideo)
	if err != nil {
		s.failViralJob(jobID, err)
		return
	}
	if math.Abs(float64(width)/float64(height)-9.0/16.0) > 0.015 {
		s.failViralJob(jobID, fmt.Errorf("video nguồn %dx%d không phải khung dọc 9:16", width, height))
		return
	}
	if !fileExists(p.SampleVideo) || !fileExists(p.ModelPhoto) {
		s.failViralJob(jobID, fmt.Errorf("thiếu video nguồn hoặc ảnh nhân vật của kênh"))
		return
	}
	if ready, reason := s.ViralCharacterEditorStatus(context.Background()); !ready {
		s.failViralJob(jobID, fmt.Errorf("Muse AI chưa sẵn sàng: %s", reason))
		return
	}
	if !ProbeHasAudio(context.Background(), p.SampleVideo) {
		s.failViralJob(jobID, fmt.Errorf("video nguồn Radar không có track âm thanh"))
		return
	}

	timeout := 20*time.Minute + time.Duration(sourceDuration*10)*time.Second
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
		s.failViralJob(jobID, fmt.Errorf("job bị hủy khi chờ lượt render"))
		return
	}

	jobDir := filepath.Join(s.workRoot, jobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		s.failViralJob(jobID, fmt.Errorf("tạo thư mục làm việc: %w", err))
		return
	}
	photos := append([]string{p.ModelPhoto}, p.ModelPhotos...)
	photos = validCharacterPhotos(photos, 3)
	if len(photos) == 0 {
		s.failViralJob(jobID, fmt.Errorf("không tìm thấy ảnh nhân vật hợp lệ"))
		return
	}
	resolution, err := NormalizeVideoOutputResolution(p.OutputResolution)
	if err != nil {
		s.failViralJob(jobID, err)
		return
	}
	if store, err := s.GetFlowStore(); err != nil {
		s.failViralJob(jobID, err)
		return
	} else {
		graph, err := store.GetGraph("preset_viral_dance")
		if err != nil {
			s.failViralJob(jobID, fmt.Errorf("không tải được preset Viral Workflow: %w", err))
			return
		}
		for i := range graph.Nodes {
			node := &graph.Nodes[i]
			if node.Params == nil {
				node.Params = map[string]any{}
			}
			switch node.ID {
			case "video":
				node.Params["video_path"] = p.SampleVideo
				// EnqueueViral đã xác thực nguồn với Radar trước khi tạo job.
				node.Params["require_radar_dance"] = false
			case "character":
				node.Params["account_id"] = p.AccountID
				node.Params["model_path"] = photos[0]
				node.Params["model_paths"] = photos
				node.Params["model_name"] = p.Account
			case "export":
				node.Params["title"] = p.Topic
			case "resolution_input":
				node.Params["value"] = resolution
			}
		}
		s.setStatus(jobID, StatusRunning, 10)
		s.appendLog(jobID, fmt.Sprintf("WORKFLOW_START: chạy preset Viral qua Workflow API · Muse AI · %.2fs · %s", sourceDuration, resolution))
		flowRun, err := s.ExecuteFlowGraphWithInputs(ctx, "viral-"+jobID, graph, nil)
		if err != nil {
			s.failViralJob(jobID, fmt.Errorf("Workflow API Viral thất bại: %w", err))
			return
		}
		if flowRun == nil || flowRun.Status != "completed" {
			status := "không có trạng thái"
			if flowRun != nil {
				status = flowRun.Status
			}
			s.failViralJob(jobID, fmt.Errorf("Workflow API Viral kết thúc với trạng thái %s", status))
			return
		}
		workflowVideo, ok := flowRun.Outputs["video"].(string)
		if !ok || !fileExists(workflowVideo) || !isViralVideoFile(workflowVideo) {
			s.failViralJob(jobID, fmt.Errorf("Workflow API không trả video output hợp lệ"))
			return
		}
		finalVideo := filepath.Join(s.outDir, fmt.Sprintf("viral_%s.mp4", jobID))
		_ = os.Remove(finalVideo)
		if err := copyStudioFile(workflowVideo, finalVideo); err != nil {
			s.failViralJob(jobID, fmt.Errorf("chép video thành phẩm: %w", err))
			return
		}
		if err := verifyVideoDuration(ctx, finalVideo, sourceDuration); err != nil {
			_ = os.Remove(finalVideo)
			s.failViralJob(jobID, fmt.Errorf("đầu ra không giữ đúng thời lượng nguồn: %w", err))
			return
		}
		if !ProbeHasAudio(ctx, finalVideo) {
			_ = os.Remove(finalVideo)
			s.failViralJob(jobID, fmt.Errorf("đầu ra Viral bị mất audio nguồn"))
			return
		}
		outWidth, outHeight, err := probeViralDimensions(ctx, finalVideo)
		wantWidth, wantHeight, _ := VideoOutputDims("9:16", resolution)
		if err != nil || outWidth != wantWidth || outHeight != wantHeight {
			_ = os.Remove(finalVideo)
			s.failViralJob(jobID, fmt.Errorf("đầu ra không đạt %s 9:16 (%dx%d)", resolution, outWidth, outHeight))
			return
		}
		s.appendLog(jobID, fmt.Sprintf("WORKFLOW_OUTPUT_PASS: %s · %.2fs · %dx%d · audio giữ nguyên", filepath.Base(finalVideo), probeDuration(ctx, finalVideo), outWidth, outHeight))
		s.setStatus(jobID, StatusRunning, 85)
		if err := s.runViralVisualGate(ctx, jobID, finalVideo, jobDir, p, photos); err != nil {
			_ = os.Remove(finalVideo)
			s.failViralJob(jobID, err)
			return
		}
		s.setCaption(jobID, p.Topic)
		s.setOutput(jobID, finalVideo)
		s.setStatus(jobID, StatusDone, 100)
		s.appendLog(jobID, "Hoàn tất: workflow Muse AI, thời lượng, âm thanh và visual QC đều đạt.")
		s.fireOnDone(jobID)
	}
}

func (s *Studio) failViralJob(jobID string, err error) {
	s.appendLog(jobID, "LỖI: "+err.Error())
	s.setStatus(jobID, StatusFailed, 100)
}
