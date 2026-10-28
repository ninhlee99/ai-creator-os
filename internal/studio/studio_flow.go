package studio

import (
	"context"
	"errors"
	"fmt"
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
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("studio is shutting down")
	}
	if s.flowRuns == nil {
		s.flowRuns = make(map[string]context.CancelFunc)
	}
	if _, exists := s.flowRuns[runID]; exists {
		s.mu.Unlock()
		cancel()
		return fmt.Errorf("workflow run %q is already active", runID)
	}
	s.flowRuns[runID] = cancel
	s.mu.Unlock()

	services := append([]flow.ExecutionServices(nil), serviceOptions...)
	inputs := cloneWorkflowInputs(workflowInputs)
	go func() {
		defer func() {
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
		_, _ = s.ExecuteFlowGraphWithInputs(runCtx, runID, graph, inputs, services...)
	}()
	return nil
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
	analyzeImages := func(ctx context.Context, prompt string, imagePaths []string) (string, error) {
		analyzer, ok := s.mg.(visualImageAnalyzer)
		if !ok || s.mg == nil || !s.mg.Healthy(ctx) {
			return "", fmt.Errorf("media engine chưa sẵn sàng phân tích ảnh")
		}
		refs := make([]ImageRef, 0, len(imagePaths))
		for _, path := range imagePaths {
			if !fileExists(path) {
				return "", fmt.Errorf("ảnh phân tích không tồn tại: %s", filepath.Base(path))
			}
			refs = append(refs, ImageRef{Path: path})
		}
		return analyzer.AnalyzeImages(ctx, prompt, refs)
	}
	var museSessionOnce sync.Once
	var museSession *WebAISession
	var museSessionErr error
	museRender := func(renderCtx context.Context, isNewThread bool, threadURL, threadTitle, prompt string, files []string, outputPath string) (string, error) {
		if !services.MuseCreditsAcknowledged {
			return "", fmt.Errorf("cần xác nhận workflow có thể dùng credits Muse AI trước khi tạo video")
		}
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
			// Callback ghi log
		},
		NodeStatus: func(nodeID string, state flow.NodeState) {
			run, err := fStore.GetRun(runID)
			if err == nil && run != nil {
				run.NodeStates[nodeID] = state
				_ = fStore.SaveRun(run)
			}
		},
		EngineCall: func(ctx context.Context, platform string, prompt string, images []string, aspect string, outPath string) (string, error) {
			switch strings.ToLower(strings.TrimSpace(platform)) {
			case webai.PlatformMuseAI:
				if !services.MuseCreditsAcknowledged {
					return "", fmt.Errorf("cần xác nhận workflow có thể dùng Muse AI credits trước khi tạo video")
				}
			case webai.PlatformGoogleFlow:
				if !services.GoogleFlowCreditsAcknowledged {
					return "", fmt.Errorf("cần xác nhận workflow có thể dùng Google Flow credits trước khi tạo video")
				}
			}
			req := WebAIVideoRequest{
				Mode:        "ingredients",
				Prompt:      prompt,
				AspectRatio: aspect,
				ImagePaths:  images,
			}
			return s.GenerateWebAIVideo(ctx, platform, req, outPath, func(line string) {})
		},
		GenerateText: func(ctx context.Context, systemPrompt, prompt string) (string, error) {
			if s.llm == nil || !s.llm.Healthy(ctx) {
				return "", fmt.Errorf("LLM chưa sẵn sàng")
			}
			return s.llm.Complete(ctx, systemPrompt, prompt)
		},
		SynthesizeSpeech: func(ctx context.Context, text, voice string) ([]byte, error) {
			if s.narrator == nil {
				return nil, fmt.Errorf("TTS chưa được cấu hình")
			}
			return s.narrator(ctx, text, voice)
		},
		GenerateImage: func(ctx context.Context, prompt string, imagePaths []string, outPath string) error {
			if s.mg == nil || !s.mg.Healthy(ctx) {
				return fmt.Errorf("image generator chưa sẵn sàng")
			}
			refs := make([]ImageRef, 0, len(imagePaths))
			for _, path := range imagePaths {
				refs = append(refs, ImageRef{Path: path})
			}
			return s.mg.GenerateImage(ctx, prompt, refs, outPath)
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
