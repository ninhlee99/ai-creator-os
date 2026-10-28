package studio

import (
	"context"
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
	_ = fs.SeedDefaultPresets()
	s.flowStore = fs
	return fs, nil
}

// ExecuteFlowGraph thực thi đồ thị ComfyUI trong môi trường của Studio.
func (s *Studio) ExecuteFlowGraph(ctx context.Context, runID string, graph *flow.FlowGraph, serviceOptions ...flow.ExecutionServices) (*flow.FlowRun, error) {
	return s.ExecuteFlowGraphWithInputs(ctx, runID, graph, nil, serviceOptions...)
}

// ExecuteFlowGraphWithInputs executes a workflow with values supplied for its named workflow_input nodes.
func (s *Studio) ExecuteFlowGraphWithInputs(ctx context.Context, runID string, graph *flow.FlowGraph, workflowInputs map[string]any, serviceOptions ...flow.ExecutionServices) (*flow.FlowRun, error) {
	if runID == "" {
		runID = fmt.Sprintf("run_%d", time.Now().UnixNano())
	}
	fStore, err := s.GetFlowStore()
	if err != nil {
		return nil, err
	}
	var services flow.ExecutionServices
	if len(serviceOptions) > 0 {
		services = serviceOptions[0]
	}

	workDir := filepath.Join(s.workRoot, "runs", runID)
	_ = os.MkdirAll(workDir, 0o755)
	_ = os.MkdirAll(s.outDir, 0o755)

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
	_ = fStore.SaveRun(initialRun)
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
	completedRun, execErr := flow.ExecuteGraph(execCtx, graph)
	if completedRun != nil {
		_ = fStore.SaveRun(completedRun)
	}

	return completedRun, execErr
}
