package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
)

var (
	flowCancelMu sync.Mutex
	flowCancels  = make(map[string]context.CancelFunc)
)

func (s *Server) handleStudioFlowView(w http.ResponseWriter, r *http.Request) {
	var graphs []flow.FlowSummary
	var activeGraph *flow.FlowGraph

	preset := r.URL.Query().Get("preset")

	if s.Studio != nil {
		if fStore, err := s.Studio.GetFlowStore(); err == nil {
			graphs, _ = fStore.ListGraphs()
			if preset != "" {
				activeGraph, _ = fStore.GetGraph(preset)
			}
			if activeGraph == nil && len(graphs) > 0 {
				activeGraph, _ = fStore.GetGraph(graphs[0].ID)
				preset = graphs[0].ID
			}
		}
	}

	if activeGraph == nil {
		activeGraph = &flow.FlowGraph{
			ID:          "flow_draft",
			Name:        "Workflow mới tùy chỉnh",
			Description: "Workflow trống - bấm '+' để thêm các node và kết nối luồng",
			Category:    "custom",
			IsPreset:    false,
			Nodes:       []flow.FlowNode{},
			Edges:       []flow.FlowEdge{},
		}
	}

	nodeDefs := flow.ListDefinitions()

	data := map[string]any{
		"ActiveTab":      "flow",
		"Graphs":         graphs,
		"ActiveGraph":    activeGraph,
		"NodeDefs":       nodeDefs,
		"PresetID":       preset,
		"IsWorkflowPage": true,
	}

	s.render(w, "studio_flow", data)
}

func (s *Server) handleAPIFlowDefinitions(w http.ResponseWriter, r *http.Request) {
	defs := flow.ListDefinitions()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"defs": defs,
	})
}

func (s *Server) handleAPIFlowProducts(w http.ResponseWriter, r *http.Request) {
	type flowProduct struct {
		ID           int64  `json:"id"`
		Title        string `json:"title"`
		Category     string `json:"category,omitempty"`
		ImageURL     string `json:"image_url,omitempty"`
		ReviewStatus string `json:"review_status"`
	}
	items := []flowProduct{}
	if s.Products != nil {
		catalog, err := s.Products.All(100)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		for _, product := range catalog {
			if !product.ReadyForAffiliateProduction() {
				continue
			}
			imageURL := ""
			if len(product.ImageURLs) > 0 {
				imageURL = product.ImageURLs[0]
			}
			items = append(items, flowProduct{
				ID: product.ID, Title: product.Title, Category: product.Category,
				ImageURL: imageURL, ReviewStatus: product.ReviewStatus,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "products": items})
}

func (s *Server) handleAPIFlowGraphs(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Error(w, `{"ok":false,"error":"Studio unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	fStore, err := s.Studio.GetFlowStore()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodPost {
		var g flow.FlowGraph
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			http.Error(w, `{"ok":false,"error":"Invalid JSON payload"}`, http.StatusBadRequest)
			return
		}
		if g.ID == "" {
			g.ID = fmt.Sprintf("flow_%d", time.Now().UnixNano())
		}
		if err := fStore.SaveUserGraph(&g); err != nil {
			http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    true,
			"graph": g,
		})
		return
	}

	// GET: List graphs
	list, err := fStore.ListGraphs()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"graphs": list,
	})
}

func (s *Server) handleAPIFlowGraphDetail(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Error(w, `{"ok":false,"error":"Studio unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	fStore, err := s.Studio.GetFlowStore()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	id := r.PathValue("id")

	if r.Method == http.MethodDelete {
		if err := fStore.DeleteGraph(id); err != nil {
			http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}

	g, err := fStore.GetGraph(id)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusNotFound)
		return
	}
	workflowError := flow.WorkflowReusableError(g)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                true,
		"graph":             g,
		"workflow_reusable": workflowError == "",
		"workflow_error":    workflowError,
	})
}

func (s *Server) handleAPIFlowExecute(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Error(w, `{"ok":false,"error":"Studio unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	fStore, err := s.Studio.GetFlowStore()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	var req struct {
		GraphID                       string          `json:"graph_id"`
		Graph                         *flow.FlowGraph `json:"graph"`
		Inputs                        map[string]any  `json:"inputs"`
		MuseAccountEmail              string          `json:"muse_account_email"`
		MuseAccountName               string          `json:"muse_account_name"`
		MuseCreditsAcknowledged       bool            `json:"muse_credits_acknowledged"`
		GoogleFlowCreditsAcknowledged bool            `json:"google_flow_credits_acknowledged"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"ok":false,"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	targetGraph := req.Graph
	if targetGraph == nil && req.GraphID != "" {
		targetGraph, err = fStore.GetGraph(req.GraphID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusNotFound)
			return
		}
	}

	if targetGraph == nil || len(targetGraph.Nodes) == 0 {
		http.Error(w, `{"ok":false,"error":"Không có đồ thị hợp lệ để thực thi"}`, http.StatusBadRequest)
		return
	}
	if err := flow.ValidateWorkflowRunInputs(targetGraph, req.Inputs); err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	if flowGraphRequiresMuseCredits(targetGraph, fStore, map[string]bool{}) && !req.MuseCreditsAcknowledged {
		http.Error(w, `{"ok":false,"error":"Workflow có thể dùng Muse AI credits; xác nhận bằng muse_credits_acknowledged=true trước khi chạy."}`, http.StatusBadRequest)
		return
	}
	if flowGraphRequiresGoogleFlowCredits(targetGraph, fStore, map[string]bool{}) && !req.GoogleFlowCreditsAcknowledged {
		http.Error(w, `{"ok":false,"error":"Workflow có thể dùng Google Flow credits; xác nhận bằng google_flow_credits_acknowledged=true trước khi chạy."}`, http.StatusBadRequest)
		return
	}

	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())

	// Đăng ký context hủy
	ctx, cancel := context.WithCancel(context.Background())
	flowCancelMu.Lock()
	flowCancels[runID] = cancel
	flowCancelMu.Unlock()

	// Chạy luồng nền
	go func() {
		defer func() {
			flowCancelMu.Lock()
			delete(flowCancels, runID)
			flowCancelMu.Unlock()
			cancel()
		}()
		services := s.flowExecutionServices()
		services.MuseAccountEmail = strings.TrimSpace(req.MuseAccountEmail)
		services.MuseAccountName = strings.TrimSpace(req.MuseAccountName)
		services.MuseCreditsAcknowledged = req.MuseCreditsAcknowledged
		services.GoogleFlowCreditsAcknowledged = req.GoogleFlowCreditsAcknowledged
		_, _ = s.Studio.ExecuteFlowGraphWithInputs(ctx, runID, targetGraph, req.Inputs, services)
	}()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"run_id": runID,
		"status": "running",
	})
}

// handleAPIFlowRunAPI cho phép các ứng dụng ngoài gọi Webhook API để tự động chạy workflow
// và truyền tham số đầu vào (prompt, product_title, model, video_url,...)
func (s *Server) handleAPIFlowRunAPI(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Error(w, `{"ok":false,"error":"Studio unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	fStore, err := s.Studio.GetFlowStore()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	var req struct {
		GraphID                       string                 `json:"graph_id"`
		Graph                         *flow.FlowGraph        `json:"graph"`
		Params                        map[string]interface{} `json:"params"`
		MuseAccountEmail              string                 `json:"muse_account_email"`
		MuseAccountName               string                 `json:"muse_account_name"`
		MuseCreditsAcknowledged       bool                   `json:"muse_credits_acknowledged"`
		GoogleFlowCreditsAcknowledged bool                   `json:"google_flow_credits_acknowledged"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"ok":false,"error":"Dữ liệu JSON không hợp lệ"}`, http.StatusBadRequest)
		return
	}

	// Hỗ trợ truyền ID qua đường dẫn /api/flow/webhook/{id}
	if req.GraphID == "" {
		req.GraphID = r.PathValue("id")
	}

	targetGraph := req.Graph
	if targetGraph == nil && req.GraphID != "" {
		targetGraph, err = fStore.GetGraph(req.GraphID)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"ok":false,"error":"Không tìm thấy workflow: %s"}`, req.GraphID), http.StatusNotFound)
			return
		}
	}

	if targetGraph == nil || len(targetGraph.Nodes) == 0 {
		http.Error(w, `{"ok":false,"error":"Không có đồ thị workflow hợp lệ để thực thi"}`, http.StatusBadRequest)
		return
	}

	// Nhân bản đồ thị để nạp các tham số runtime mà không làm hỏng bản lưu gốc
	clonedGraph := *targetGraph
	clonedNodes := make([]flow.FlowNode, len(targetGraph.Nodes))
	for i, n := range targetGraph.Nodes {
		clonedNodes[i] = n
		if n.Params != nil {
			clonedParams := make(map[string]any)
			for k, v := range n.Params {
				clonedParams[k] = v
			}
			clonedNodes[i].Params = clonedParams
		} else {
			clonedNodes[i].Params = make(map[string]any)
		}
	}
	clonedGraph.Nodes = clonedNodes

	// Tự động tiêm tham số (Inject params) vào các node tương ứng của workflow
	if len(req.Params) > 0 {
		for i := range clonedGraph.Nodes {
			node := &clonedGraph.Nodes[i]

			// 1. Tiêm theo ID cụ thể của node: params["node_id"] = { ... }
			if nodeOverride, ok := req.Params[node.ID].(map[string]any); ok {
				for pk, pv := range nodeOverride {
					node.Params[pk] = pv
				}
			}

			// 2. Tiêm theo tên trường tham số phổ biến
			for paramKey, paramVal := range req.Params {
				switch paramKey {
				case "prompt":
					if _, hasPrompt := node.Params["prompt"]; hasPrompt || node.Category == flow.CategoryAI || node.Type == "custom_ai_prompt" || node.Type == "ai_script_writer" {
						node.Params["prompt"] = paramVal
					}
				case "model", "provider":
					if _, hasModel := node.Params["model"]; hasModel || node.Category == flow.CategoryAI {
						node.Params["model"] = paramVal
						node.Params["provider"] = paramVal
					}
				case "product_name", "product_title":
					if _, hasTitle := node.Params["product_title"]; hasTitle || node.Type == "product_input" || node.Type == "asset_product_info" {
						node.Params["product_title"] = paramVal
					}
				case "character_image_url", "avatar_url":
					if node.Type == "character_input" || node.Type == "asset_model_profile" {
						node.Params["image_url"] = paramVal
						node.Params["source_image"] = paramVal
						node.Params["character_image"] = paramVal
					}
				case "product_image_url", "fabric_image_url":
					if node.Type == "product_input" || node.Type == "asset_product_info" || node.Type == "garment_segmentation" || strings.Contains(node.Type, "product") || strings.Contains(node.Type, "garment") {
						node.Params["image_url"] = paramVal
						node.Params["source_image"] = paramVal
						node.Params["product_image"] = paramVal
					}
				case "video_url", "source_video", "source_video_url", "motion_video_url":
					if _, hasVideo := node.Params["video_url"]; hasVideo || node.Type == "video_source" || node.Type == "asset_video_input" {
						node.Params["video_url"] = paramVal
						node.Params["source_video"] = paramVal
					}
				case "image_url":
					if _, hasImage := node.Params["image_url"]; hasImage || node.Type == "character_input" || node.Type == "asset_model_profile" {
						node.Params["image_url"] = paramVal
						node.Params["source_image"] = paramVal
					}
				default:
					if _, exists := node.Params[paramKey]; exists {
						node.Params[paramKey] = paramVal
					}
				}
			}
		}

		// Named workflow boundary inputs form the public request contract.
		for i := range clonedGraph.Nodes {
			node := &clonedGraph.Nodes[i]
			if node.Type != "workflow_input" || node.Params == nil {
				continue
			}
			key, _ := node.Params["key"].(string)
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if value, ok := req.Params[key]; ok {
				node.Params["value"] = value
			}
		}

		// A node-ID override stays highest priority for advanced integrations.
		for i := range clonedGraph.Nodes {
			node := &clonedGraph.Nodes[i]
			if override, ok := req.Params[node.ID].(map[string]any); ok {
				for key, value := range override {
					node.Params[key] = value
				}
			}
		}
	}
	if err := flow.ValidateWorkflowRunInputs(&clonedGraph, nil); err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	if flowGraphRequiresMuseCredits(&clonedGraph, fStore, map[string]bool{}) && !req.MuseCreditsAcknowledged {
		http.Error(w, `{"ok":false,"error":"Workflow có thể dùng Muse AI credits; xác nhận bằng muse_credits_acknowledged=true trước khi chạy."}`, http.StatusBadRequest)
		return
	}
	if flowGraphRequiresGoogleFlowCredits(&clonedGraph, fStore, map[string]bool{}) && !req.GoogleFlowCreditsAcknowledged {
		http.Error(w, `{"ok":false,"error":"Workflow có thể dùng Google Flow credits; xác nhận bằng google_flow_credits_acknowledged=true trước khi chạy."}`, http.StatusBadRequest)
		return
	}

	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	flowCancelMu.Lock()
	flowCancels[runID] = cancel
	flowCancelMu.Unlock()

	go func() {
		defer func() {
			flowCancelMu.Lock()
			delete(flowCancels, runID)
			flowCancelMu.Unlock()
			cancel()
		}()
		services := s.flowExecutionServices()
		services.MuseAccountEmail = strings.TrimSpace(req.MuseAccountEmail)
		services.MuseAccountName = strings.TrimSpace(req.MuseAccountName)
		services.MuseCreditsAcknowledged = req.MuseCreditsAcknowledged
		services.GoogleFlowCreditsAcknowledged = req.GoogleFlowCreditsAcknowledged
		_, _ = s.Studio.ExecuteFlowGraph(ctx, runID, &clonedGraph, services)
	}()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":              true,
		"run_id":          runID,
		"graph_id":        targetGraph.ID,
		"graph_name":      targetGraph.Name,
		"status":          "running",
		"message":         "Workflow đã tự động khởi chạy thành công qua API Webhook",
		"injected_params": req.Params,
		"status_url":      "/api/flow/runs/" + runID,
	})
}

func flowGraphRequiresMuseCredits(graph *flow.FlowGraph, store *flow.Store, visited map[string]bool) bool {
	return flowGraphRequiresProviderCredits(graph, store, visited, "museai")
}

func flowGraphRequiresGoogleFlowCredits(graph *flow.FlowGraph, store *flow.Store, visited map[string]bool) bool {
	return flowGraphRequiresProviderCredits(graph, store, visited, "googleflow")
}

func flowGraphRequiresProviderCredits(graph *flow.FlowGraph, store *flow.Store, visited map[string]bool, provider string) bool {
	if graph == nil {
		return false
	}
	for _, node := range graph.Nodes {
		if flowNodeRequiresProviderCredits(node, provider) {
			return true
		}
		if node.Type != "workflow_map" && node.Type != "workflow_call" && !strings.HasPrefix(node.Type, "workflow_call_") {
			continue
		}
		workflowID, _ := node.Params["workflow_id"].(string)
		workflowID = strings.TrimSpace(workflowID)
		if workflowID == "" || visited[workflowID] || store == nil {
			continue
		}
		visited[workflowID] = true
		child, err := store.GetGraph(workflowID)
		if err == nil && flowGraphRequiresProviderCredits(child, store, visited, provider) {
			return true
		}
	}
	return false
}

func flowNodeRequiresProviderCredits(node flow.FlowNode, provider string) bool {
	switch node.Type {
	case "muse_ai_video", "muse_ai_neural_render":
		return provider == "museai"
	case "google_flow_video":
		return provider == "googleflow"
	case "custom_web_ai":
		if flowHandlerParamString(node.Params, "output_type", "video") != "video" {
			return false
		}
		return normalizeFlowProvider(flowHandlerParamString(node.Params, "provider_name", "googleflow")) == provider
	case "custom_ai_prompt", "custom_universal_ai":
		if flowHandlerParamString(node.Params, "output_type", "text") != "video" {
			return false
		}
		return normalizeFlowProvider(flowHandlerParamString(node.Params, "provider", "googleflow")) == provider
	default:
		return false
	}
}

func flowHandlerParamString(params map[string]any, key, fallback string) string {
	value, ok := params[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeFlowProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "googleflow", "google flow", "veo", "veo 2", "veo 3":
		return "googleflow"
	case "muse", "museai", "muse ai":
		return "museai"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func (s *Server) handleAPIFlowRunDetail(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Error(w, `{"ok":false,"error":"Studio unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	fStore, err := s.Studio.GetFlowStore()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	id := r.PathValue("id")
	run, err := fStore.GetRun(id)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":  true,
		"run": run,
	})
}

func (s *Server) handleAPIFlowRunCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	flowCancelMu.Lock()
	cancel, ok := flowCancels[id]
	if ok && cancel != nil {
		cancel()
		delete(flowCancels, id)
	}
	flowCancelMu.Unlock()

	if s.Studio != nil {
		if fStore, err := s.Studio.GetFlowStore(); err == nil {
			if run, err := fStore.GetRun(id); err == nil && run != nil {
				run.Status = "cancelled"
				_ = fStore.SaveRun(run)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
	})
}

func (s *Server) handleAPIFlowRuns(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Error(w, `{"ok":false,"error":"Studio unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	fStore, err := s.Studio.GetFlowStore()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	runs, err := fStore.ListRuns(30)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"runs": runs,
	})
}

// handleAPIFlowUpload lưu ảnh, video và audio cho các node trong workflow.
func (s *Server) handleAPIFlowUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(50 << 20); err != nil { // max 50MB
		http.Error(w, `{"ok":false,"error":"Dung lượng file vượt quá giới hạn hoặc định dạng không hợp lệ"}`, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"ok":false,"error":"Không tìm thấy file tải lên"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	supportedExt := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
		".mp4": true, ".mov": true, ".webm": true, ".mkv": true, ".avi": true,
		".mp3": true, ".aac": true, ".wav": true, ".m4a": true, ".ogg": true, ".flac": true,
	}
	if !supportedExt[ext] {
		http.Error(w, `{"ok":false,"error":"Định dạng file không được hỗ trợ"}`, http.StatusBadRequest)
		return
	}

	uploadDir := "data/uploads"
	if s.Studio != nil && s.Studio.UploadDir() != "" {
		uploadDir = s.Studio.UploadDir()
	}
	_ = os.MkdirAll(uploadDir, 0o755)

	safeBasename := filepath.Base(header.Filename)
	safeBasename = strings.ReplaceAll(safeBasename, " ", "_")
	targetFilename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), safeBasename)
	targetPath := filepath.Join(uploadDir, targetFilename)

	dst, err := os.Create(targetPath)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	absPath, _ := filepath.Abs(targetPath)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"file_path": absPath,
		"filename":  targetFilename,
		"url":       "/media/" + targetFilename,
	})
}

func (s *Server) handleAPIFlowFetchURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"ok":false,"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL  string `json:"url"`
		Type string `json:"type"` // "video", "image", or "auto"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		http.Error(w, `{"ok":false,"error":"Thiếu thông tin URL hợp lệ"}`, http.StatusBadRequest)
		return
	}

	uploadDir := "data/uploads"
	if s.Studio != nil && s.Studio.UploadDir() != "" {
		uploadDir = s.Studio.UploadDir()
	}
	_ = os.MkdirAll(uploadDir, 0o755)

	destPath, mediaType, err := flow.DownloadMediaResource(r.Context(), req.URL, uploadDir, req.Type)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": fmt.Sprintf("Không thể tải tài nguyên từ URL: %v", err),
		})
		return
	}

	absPath, _ := filepath.Abs(destPath)
	filename := filepath.Base(destPath)

	resp := map[string]any{
		"ok":        true,
		"file_path": absPath,
		"filename":  filename,
		"url":       "/media/" + filename,
		"type":      mediaType,
	}

	// Nếu là video, tạo thêm ảnh thumbnail đại diện
	if mediaType == "video" {
		thumbName := fmt.Sprintf("%s_thumb.jpg", strings.TrimSuffix(filename, filepath.Ext(filename)))
		thumbPath := filepath.Join(uploadDir, thumbName)
		if err := flow.ExtractFrameFromVideo(r.Context(), destPath, thumbPath, 1.8); err == nil {
			resp["thumbnail_url"] = "/media/" + thumbName
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
