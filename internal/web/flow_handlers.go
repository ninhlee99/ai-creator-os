package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	if preset == "" {
		preset = "preset_viral_dance"
	}

	if s.Studio != nil {
		if fStore, err := s.Studio.GetFlowStore(); err == nil {
			graphs, _ = fStore.ListGraphs()
			activeGraph, _ = fStore.GetGraph(preset)
			if activeGraph == nil && len(graphs) > 0 {
				activeGraph, _ = fStore.GetGraph(graphs[0].ID)
			}
		}
	}

	if activeGraph == nil {
		p := flow.PresetViralDance()
		activeGraph = &p
	}

	nodeDefs := flow.ListDefinitions()

	data := map[string]any{
		"ActiveTab":   "flow",
		"Graphs":      graphs,
		"ActiveGraph": activeGraph,
		"NodeDefs":    nodeDefs,
		"PresetID":    preset,
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
		if err := fStore.SaveGraph(&g); err != nil {
			http.Error(w, fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error()), http.StatusInternalServerError)
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    true,
		"graph": g,
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
		GraphID string          `json:"graph_id"`
		Graph   *flow.FlowGraph `json:"graph"`
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
		_, _ = s.Studio.ExecuteFlowGraph(ctx, runID, targetGraph)
	}()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"run_id": runID,
		"status": "running",
	})
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
