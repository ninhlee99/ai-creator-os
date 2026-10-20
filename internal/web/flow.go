package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/flow"
)

type flowViewData struct {
	Recipes    []flow.RecipeCatalogItem `json:"recipes"`
	ActiveFlow *flow.Graph              `json:"active_flow,omitempty"`
}

// handleFlow renders the main ComfyUI-like Flow Studio dashboard.
func (s *Server) handleFlow(w http.ResponseWriter, r *http.Request) {
	data := s.ctx()
	recipes := flow.ListRecipes()

	s.flowMu.RLock()
	var latestFlow *flow.Graph
	// Get most recently created flow
	for _, g := range s.activeFlows {
		if latestFlow == nil || g.CreatedAt.After(latestFlow.CreatedAt) {
			latestFlow = g
		}
	}
	s.flowMu.RUnlock()

	// If no flow yet, create a default Cute Fashion Affiliate flow for demonstration
	if latestFlow == nil {
		defaultFlow, err := flow.BuildRecipe(flow.RecipeKeyLookbookOutfit, map[string]interface{}{
			"category": "thời trang nữ dễ thương xinh xắn",
		})
		if err == nil {
			s.flowMu.Lock()
			s.activeFlows[defaultFlow.ID] = defaultFlow
			s.flowMu.Unlock()
			latestFlow = defaultFlow
		}
	}

	var modelPhotos []string
	if s.Studio != nil {
		if ml, err := s.Studio.ModelLibrary(); err == nil {
			if photos, err := ml.List(1); err == nil && len(photos) > 0 {
				for _, p := range photos {
					base := filepath.Base(p.Path)
					modelPhotos = append(modelPhotos, fmt.Sprintf("/studio/models/1/%s", base))
				}
			}
		}
	}
	if len(modelPhotos) == 0 {
		dir := filepath.Join(s.OutDir, "studio", "models", "1")
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && (strings.HasSuffix(e.Name(), ".jpg") || strings.HasSuffix(e.Name(), ".jpeg") || strings.HasSuffix(e.Name(), ".png")) {
					modelPhotos = append(modelPhotos, fmt.Sprintf("/studio/models/1/%s", e.Name()))
				}
			}
		}
	}
	data["CharacterPhotos"] = modelPhotos

	data["Flow"] = flowViewData{
		Recipes:    recipes,
		ActiveFlow: latestFlow,
	}

	s.render(w, "flow", data)
}

// handleAPIFlowRecipes returns the list of available recipes.
func (s *Server) handleAPIFlowRecipes(w http.ResponseWriter, r *http.Request) {
	recipes := flow.ListRecipes()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"recipes": recipes,
	})
}

type compileFlowRequest struct {
	Prompt    string                 `json:"prompt"`
	RecipeKey string                 `json:"recipe_key"`
	Params    map[string]interface{} `json:"params"`
}

// handleAPIFlowCompile compiles a natural language prompt or recipe key into a Graph.
func (s *Server) handleAPIFlowCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req compileFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("invalid JSON payload: %v", err),
		})
		return
	}

	var g *flow.Graph
	var err error

	if req.RecipeKey != "" {
		g, err = flow.BuildRecipe(req.RecipeKey, req.Params)
	} else if req.Prompt != "" {
		if s.FlowCompiler == nil {
			s.FlowCompiler = flow.NewCompiler(nil)
		}
		g, err = s.FlowCompiler.Compile(r.Context(), req.Prompt, req.Params)
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "cần cung cấp prompt hoặc recipe_key",
		})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("lỗi biên dịch Flow: %v", err),
		})
		return
	}

	s.flowMu.Lock()
	s.activeFlows[g.ID] = g
	s.flowMu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"graph": g,
	})
}

type runFlowRequest struct {
	ID string `json:"id"`
}

// handleAPIFlowRun executes the specified Flow Graph.
func (s *Server) handleAPIFlowRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req runFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "cần cung cấp id của flow",
		})
		return
	}

	s.flowMu.RLock()
	g, ok := s.activeFlows[req.ID]
	s.flowMu.RUnlock()

	if !ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("không tìm thấy flow id: %s", req.ID),
		})
		return
	}

	if s.FlowExecutor == nil {
		if s.FlowBridge == nil {
			s.FlowBridge = flow.NewAdaptersBridge(s.WebAI, nil)
		}
		s.FlowExecutor = flow.NewExecutor(s.FlowBridge)
	}
	if s.FlowBridge != nil {
		if s.TTS != nil {
			s.FlowBridge.SetTTSSynthesizer(s.TTS.Synthesize)
		}
		s.FlowBridge.SetOutDir(s.OutDir)
	}

	// Run synchronously or trigger in background
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	err := s.FlowExecutor.Execute(ctx, g)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("thực thi flow thất bại: %v", err),
			"graph": g,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"graph": g,
	})
}

// handleAPIFlowStatus returns the live status and node execution details of a Flow.
func (s *Server) handleAPIFlowStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "thiếu tham số id",
		})
		return
	}

	s.flowMu.RLock()
	g, ok := s.activeFlows[id]
	s.flowMu.RUnlock()

	if !ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "không tìm thấy flow",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"graph": g,
	})
}
