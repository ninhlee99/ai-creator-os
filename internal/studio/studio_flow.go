package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
	"github.com/ninhlee99/ai-creator-os/internal/webai"
)

// GetFlowStore trả về bộ lưu trữ đồ thị ComfyUI của Studio (khởi tạo lười nếu cần).
func (s *Studio) GetFlowStore() (*flow.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flowStore != nil {
		if fs, ok := s.flowStore.(*flow.Store); ok {
			return fs, nil
		}
	}
	fs, err := flow.NewStore(s.db)
	if err != nil {
		return nil, fmt.Errorf("khởi tạo workflow store: %w", err)
	}
	if err := fs.SeedDefaultPresets(); err != nil {
		return nil, fmt.Errorf("khởi tạo workflow presets: %w", err)
	}
	s.flowStore = fs
	return fs, nil
}

// ExecuteFlowGraph thực thi đồ thị ComfyUI trong môi trường của Studio.
func (s *Studio) ExecuteFlowGraph(ctx context.Context, runID string, graph *flow.FlowGraph, serviceOptions ...flow.ExecutionServices) (*flow.FlowRun, error) {
	return s.ExecuteFlowGraphWithInputs(ctx, runID, graph, nil, serviceOptions...)
}

// StartFlowGraphWithInputs starts a detached workflow run and tracks its
// cancellation until completion so Studio shutdown can drain it safely.
func (s *Studio) StartFlowGraphWithInputs(ctx context.Context, runID string, graph *flow.FlowGraph, workflowInputs map[string]any, serviceOptions ...flow.ExecutionServices) error {
	if graph == nil {
		return fmt.Errorf("workflow graph is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if runID == "" {
		runID = fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	runCtx, cancel := context.WithCancel(ctx)
	if err := s.reserveFlowAdmission(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.shuttingDown || s.shutdownStarted.Load() {
			return fmt.Errorf("studio is shutting down")
		}
		if s.flowRuns == nil {
			s.flowRuns = make(map[string]context.CancelFunc)
		}
		if _, exists := s.flowRuns[runID]; exists {
			return fmt.Errorf("workflow run %q is already active", runID)
		}
		s.flowRuns[runID] = cancel
		return nil
	}); err != nil {
		cancel()
		return err
	}

	store, err := s.GetFlowStore()
	if err != nil {
		s.queueMu.Lock()
		if s.queuedFlows > 0 {
			s.queuedFlows--
		}
		s.queueMu.Unlock()
		s.mu.Lock()
		delete(s.flowRuns, runID)
		s.mu.Unlock()
		cancel()
		return err
	}
	queued := &flow.FlowRun{
		ID: runID, GraphID: graph.ID, GraphName: graph.Name, Status: "queued",
		NodeStates: make(map[string]flow.NodeState), Outputs: make(map[string]any),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	for _, node := range graph.Nodes {
		queued.NodeStates[node.ID] = flow.NodeState{Status: "queued"}
	}
	if err := store.SaveRun(queued); err != nil {
		s.queueMu.Lock()
		if s.queuedFlows > 0 {
			s.queuedFlows--
		}
		s.queueMu.Unlock()
		s.mu.Lock()
		delete(s.flowRuns, runID)
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("lưu trạng thái queue workflow: %w", err)
	}
	services := append([]flow.ExecutionServices(nil), serviceOptions...)
	inputs := cloneWorkflowInputs(workflowInputs)
	go func() {
		defer func() {
			s.queueMu.Lock()
			if s.queuedFlows > 0 {
				s.queuedFlows--
			}
			s.queueMu.Unlock()
			s.mu.Lock()
			delete(s.flowRuns, runID)
			s.mu.Unlock()
			cancel()
		}()
		defer func() {
			if recovered := recover(); recovered != nil {
				_, _ = s.recordFlowPanic(runID, graph, recovered)
			}
		}()
		_, runErr := s.ExecuteFlowGraphWithInputs(runCtx, runID, graph, inputs, services...)
		if runErr != nil && runCtx.Err() != nil {
			s.markQueuedFlowRunCancelled(runID)
		}
	}()
	return nil
}

func (s *Studio) markQueuedFlowRunCancelled(runID string) {
	store, err := s.GetFlowStore()
	if err != nil {
		return
	}
	run, err := store.GetRun(runID)
	if err != nil || run == nil || run.Status != "queued" {
		return
	}
	now := time.Now().Format(time.RFC3339)
	for id, state := range run.NodeStates {
		if state.Status == "queued" {
			state.Status = "cancelled"
			state.FinishedAt = now
			run.NodeStates[id] = state
		}
	}
	run.Status = "cancelled"
	run.FinishedAt = now
	run.Log += "Workflow bị hủy khi đang chờ lượt chạy.\n"
	_ = store.SaveRun(run)
}

func (s *Studio) recordFlowPanic(runID string, graph *flow.FlowGraph, recovered any) (run *flow.FlowRun, err error) {
	defer func() {
		if panicValue := recover(); panicValue != nil {
			run = nil
			err = fmt.Errorf("record workflow panic: %v", panicValue)
		}
	}()
	message := fmt.Sprintf("workflow panic: %v", recovered)
	store, err := s.GetFlowStore()
	if err != nil {
		return nil, err
	}
	run, err = store.GetRun(runID)
	if err != nil || run == nil {
		run = &flow.FlowRun{
			ID: runID, GraphID: graph.ID, GraphName: graph.Name,
			NodeStates: make(map[string]flow.NodeState), Outputs: make(map[string]any),
			CreatedAt: time.Now().Format(time.RFC3339),
		}
		for _, node := range graph.Nodes {
			run.NodeStates[node.ID] = flow.NodeState{Status: "queued"}
		}
	}
	for id, state := range run.NodeStates {
		if state.Status == "running" {
			state.Status = "failed"
			state.Error = message
			state.FinishedAt = time.Now().Format(time.RFC3339)
			run.NodeStates[id] = state
		}
	}
	run.Status = "failed"
	run.FinishedAt = time.Now().Format(time.RFC3339)
	if run.Log != "" && !strings.HasSuffix(run.Log, "\n") {
		run.Log += "\n"
	}
	run.Log += message + "\n"
	if err := store.SaveRun(run); err != nil {
		return run, err
	}
	return run, nil
}

// CancelFlowRun cancels an active detached workflow without removing its
// lifecycle record; shutdown keeps waiting until its executor exits.
func (s *Studio) CancelFlowRun(runID string) bool {
	s.mu.Lock()
	cancel, ok := s.flowRuns[runID]
	if ok && cancel != nil {
		cancel()
	}
	s.mu.Unlock()
	return ok
}

func cloneWorkflowInputs(inputs map[string]any) map[string]any {
	if inputs == nil {
		return nil
	}
	copy := make(map[string]any, len(inputs))
	for key, value := range inputs {
		copy[key] = value
	}
	return copy
}

// ExecuteFlowGraphWithInputs executes a workflow with values supplied for its named workflow_input nodes.
func (s *Studio) ExecuteFlowGraphWithInputs(ctx context.Context, runID string, graph *flow.FlowGraph, workflowInputs map[string]any, serviceOptions ...flow.ExecutionServices) (completedRun *flow.FlowRun, execErr error) {
	if graph == nil {
		return nil, fmt.Errorf("workflow graph is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sem, err := s.renderSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { <-sem }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runID == "" {
		runID = fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			completedRun, execErr = s.recordFlowPanic(runID, graph, recovered)
			if execErr == nil {
				execErr = fmt.Errorf("workflow panic: %v", recovered)
			} else {
				execErr = errors.Join(fmt.Errorf("workflow panic: %v", recovered), execErr)
			}
		}
	}()
	fStore, err := s.GetFlowStore()
	if err != nil {
		return nil, err
	}
	var services flow.ExecutionServices
	if len(serviceOptions) > 0 {
		services = serviceOptions[0]
	}

	workDir := filepath.Join(s.workRoot, "runs", runID)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("tạo thư mục workflow: %w", err)
	}
	if err := os.MkdirAll(s.outDir, 0o755); err != nil {
		return nil, fmt.Errorf("tạo thư mục output workflow: %w", err)
	}

	initialRun := &flow.FlowRun{
		ID:         runID,
		GraphID:    graph.ID,
		GraphName:  graph.Name,
		Status:     "running",
		Progress:   0,
		NodeStates: make(map[string]flow.NodeState),
		Outputs:    make(map[string]any),
		CreatedAt:  time.Now().Format(time.RFC3339),
	}
	for _, n := range graph.Nodes {
		initialRun.NodeStates[n.ID] = flow.NodeState{Status: "queued", Progress: 0}
	}
	if err := fStore.SaveRun(initialRun); err != nil {
		return nil, fmt.Errorf("lưu trạng thái khởi tạo workflow: %w", err)
	}

	var nodeStateMu sync.Mutex
	liveNodeStates := make(map[string]flow.NodeState, len(initialRun.NodeStates))
	for k, v := range initialRun.NodeStates {
		liveNodeStates[k] = v
	}

	analyzeImages := func(ctx context.Context, prompt string, imagePaths []string) (string, error) {
		if analyzer, ok := s.mg.(visualImageAnalyzer); ok && s.mg != nil && s.mg.Healthy(ctx) {
			refs := make([]ImageRef, 0, len(imagePaths))
			for _, path := range imagePaths {
				if !fileExists(path) {
					return "", fmt.Errorf("ảnh phân tích không tồn tại: %s", filepath.Base(path))
				}
				refs = append(refs, ImageRef{Path: path})
			}
			if out, err := analyzer.AnalyzeImages(ctx, prompt, refs); err == nil && strings.TrimSpace(out) != "" {
				return out, nil
			}
		}
		// Dự phòng sang tài khoản Web AI (Gemini, Claude, ChatGPT, Grok) khi MediaGen không sẵn sàng
		return s.AnalyzeImagesWithWebAI(ctx, prompt, imagePaths)
	}
	var museSessionOnce sync.Once
	var museSession *WebAISession
	var museSessionErr error
	museRender := func(renderCtx context.Context, isNewThread bool, threadURL, threadTitle, prompt string, files []string, outputPath string) (string, error) {
		services.MuseCreditsAcknowledged = true
		museSessionOnce.Do(func() {
			museSession, museSessionErr = s.GetWebAISessionForEmail("museai", services.MuseAccountEmail)
		})
		if museSessionErr != nil {
			return "", museSessionErr
		}
		accountName := services.MuseAccountName
		if strings.TrimSpace(accountName) == "" && museSession != nil {
			accountName = museSession.AccountName
		}
		return s.RenderMuseChat(renderCtx, museSession, MuseChatRequest{
			NewThread: isNewThread, ThreadURL: threadURL, Title: threadTitle,
			AccountName: accountName,
			Prompt:      prompt, Files: append([]string(nil), files...),
		}, outputPath, func(message string) {})
	}

	execCtx := &flow.ExecutionContext{
		Context:               ctx,
		RunID:                 runID,
		WorkDir:               workDir,
		OutDir:                s.outDir,
		WorkflowInputs:        workflowInputs,
		MuseAccountEmail:      services.MuseAccountEmail,
		MuseAccountName:       services.MuseAccountName,
		MuseRenderWithSession: museRender,
		Log: func(msg string) {
			log.Printf("[Run %s] %s", runID, msg)
		},
		NodeStatus: func(nodeID string, state flow.NodeState) {
			nodeStateMu.Lock()
			liveNodeStates[nodeID] = state
			snapshot := make(map[string]flow.NodeState, len(liveNodeStates))
			for k, v := range liveNodeStates {
				snapshot[k] = v
			}
			nodeStateMu.Unlock()
			_ = fStore.UpdateRunNodeStates(runID, snapshot)
		},
		EngineCall: func(ctx context.Context, platform string, prompt string, images []string, aspect, projectID, accountID, outPath string) (string, string, error) {
			services.MuseCreditsAcknowledged = true
			services.GoogleFlowCreditsAcknowledged = true
			req := WebAIVideoRequest{
				Mode:        "ingredients",
				Prompt:      prompt,
				AspectRatio: aspect,
				ImagePaths:  images,
				ProjectID:   projectID,
			}
			nextProjectID, nextAccountID, err := s.GenerateWebAIVideoForAccount(ctx, platform, accountID, req, outPath, func(line string) {})
			return nextProjectID, nextAccountID, err
		},
		GenerateText: func(ctx context.Context, systemPrompt, prompt string) (string, error) {
			if s.llm != nil && s.llm.Healthy(ctx) {
				if txt, err := s.llm.Complete(ctx, systemPrompt, prompt); err == nil && strings.TrimSpace(txt) != "" {
					return txt, nil
				}
			}
			return s.GenerateTextWithWebAI(ctx, systemPrompt, prompt)
		},
		WebAIOperation: func(ctx context.Context, platform, prompt string, files []string, outputType, outputPath, conversationURL, accountID string) (string, string, string, string, error) {
			var session *WebAISession
			var err error
			if accountID != "" {
				session, err = s.GetWebAISessionByID(platform, accountID)
			} else {
				session, err = s.GetWebAISession(platform)
			}
			if err != nil {
				return "", "", conversationURL, accountID, err
			}
			result, err := webai.UseWebAIInWebUI(ctx, platform, session.CookieHeader, session.CookieJar, webai.WebAIOperationRequest{
				Prompt: prompt, Files: files, OutputType: outputType, OutputPath: outputPath, ConversationURL: conversationURL,
			}, session.CookieUpdater)
			return result.Text, result.MediaPath, result.ConversationURL, session.AccountID, err
		},
		SynthesizeSpeech: func(ctx context.Context, text, voice string) ([]byte, error) {
			if s.narrator == nil {
				return nil, fmt.Errorf("TTS chưa được cấu hình")
			}
			return s.narrator(ctx, text, voice)
		},
		GenerateImage: func(ctx context.Context, prompt string, imagePaths []string, outPath string) error {
			if s.mg != nil && s.mg.Healthy(ctx) {
				refs := make([]ImageRef, 0, len(imagePaths))
				for _, path := range imagePaths {
					refs = append(refs, ImageRef{Path: path})
				}
				if err := s.mg.GenerateImage(ctx, prompt, refs, outPath); err == nil && fileExists(outPath) {
					return nil
				}
			}
			return s.GenerateImageWithWebAI(ctx, prompt, imagePaths, outPath)
		},
		AnalyzeImages: analyzeImages,
		VisionEval: func(ctx context.Context, prompt string, imagePath string) (string, error) {
			return analyzeImages(ctx, "Bạn là chuyên gia thị giác AI phân tích chi tiết ảnh. "+prompt, []string{imagePath})
		},
		CharacterPhotos: func(accountID int64) ([]string, error) {
			photos := s.ModelPhotosForAccount(accountID)
			if len(photos) == 0 {
				return nil, fmt.Errorf("kênh %d chưa có ảnh nhân vật trong thư viện", accountID)
			}
			return photos, nil
		},
		EvaluateVideo: func(ctx context.Context, videoPath, workDir string) (map[string]any, error) {
			evalReport, err := s.EvaluateVideoQuality(ctx, videoPath, workDir, analyzeImages)
			if err != nil {
				return nil, err
			}
			data, err := json.Marshal(evalReport)
			if err != nil {
				return nil, err
			}
			var result map[string]any
			if err := json.Unmarshal(data, &result); err != nil {
				return nil, err
			}
			return result, nil
		},
		ResolveProduct:    services.ResolveProduct,
		ResolveRadarVideo: services.ResolveRadarVideo,
		FFmpegRunner: func(ctx context.Context, args ...string) error {
			return ffmpegRun(ctx, args...)
		},
		ResolveWorkflow: func(id string) (*flow.FlowGraph, error) {
			return fStore.GetGraph(id)
		},
	}
	completedRun, execErr = flow.ExecuteGraph(execCtx, graph)
	if completedRun != nil {
		// Sau khi workflow hoàn thành và tạo video thành công, tự động phân tích và đánh giá chất lượng video
		if completedRun.Status == "completed" {
			videoPath := findFlowRunOutputVideo(completedRun)
			if videoPath != "" && fileExists(videoPath) {
				// Re-run technical QC after the graph without starting another
				// browser-backed vision request; browser startup failures must not
				// keep a finished workflow in "running" state.
				evalReport, evalErr := s.EvaluateVideoQuality(execCtx.Context, videoPath, workDir, nil)
				if evalErr == nil && evalReport != nil {
					if completedRun.Outputs == nil {
						completedRun.Outputs = make(map[string]any)
					}
					completedRun.Outputs["quality_evaluation"] = evalReport
					completedRun.Outputs["video_qc_score"] = evalReport.OverallScore
					completedRun.Outputs["video_qc_status"] = evalReport.Status
					completedRun.Outputs["video_qc_verdict"] = evalReport.Verdict
					evalLog := fmt.Sprintf("\n[VIDEO QUALITY QC] Điểm: %d/100 (%s) · %s · %dx%d (%s) · %.1fs · %s",
						evalReport.OverallScore, evalReport.Status, evalReport.Verdict,
						evalReport.Width, evalReport.Height, evalReport.AspectRatio, evalReport.DurationSec, evalReport.Summary)
					completedRun.Log += evalLog
				}
			}
		}

		if err := fStore.SaveRun(completedRun); err != nil {
			persistErr := fmt.Errorf("lưu trạng thái kết thúc workflow: %w", err)
			if execErr != nil {
				return completedRun, errors.Join(execErr, persistErr)
			}
			return completedRun, persistErr
		}
	}

	return completedRun, execErr
}

func findFlowRunOutputVideo(run *flow.FlowRun) string {
	if run == nil || run.Outputs == nil {
		return ""
	}
	for _, key := range []string{"video", "video_out", "result_out"} {
		if val, ok := run.Outputs[key].(string); ok && isViralVideoFile(val) {
			return val
		}
	}
	for _, val := range run.Outputs {
		if path, ok := val.(string); ok && isViralVideoFile(path) {
			return path
		}
	}
	return ""
}
