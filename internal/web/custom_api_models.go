package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const settingKeyCustomAPIModels = "custom_api_models"

// CustomAPIModel đại diện cho cấu hình model API do người dùng tự đặt tên và cấu hình endpoint.
type CustomAPIModel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`     // Tên định danh (vd: "OpenAI GPT-4o Prod", "Gemini 2.5 Pro")
	Provider  string `json:"provider"` // "openai", "gemini", "anthropic", "custom"
	Endpoint  string `json:"endpoint"` // URL API endpoint (vd: "https://api.openai.com/v1")
	Model     string `json:"model"`    // Model ID (vd: "gpt-4.5-preview", "gemini-2.5-pro")
	Type      string `json:"type"`     // "text", "image", "video", "multimodal"
	APIKey    string `json:"api_key"`  // Secret API Key
	CreatedAt string `json:"created_at"`
}

var customAPIModelsMu sync.RWMutex

func (s *Server) listCustomAPIModels() ([]CustomAPIModel, error) {
	customAPIModelsMu.RLock()
	defer customAPIModelsMu.RUnlock()

	if s.Ledger == nil {
		return s.default2026APIModels(), nil
	}
	raw, found, err := s.Ledger.GetSetting(settingKeyCustomAPIModels)
	if err != nil || !found || strings.TrimSpace(raw) == "" {
		return s.default2026APIModels(), nil
	}
	var models []CustomAPIModel
	if err := json.Unmarshal([]byte(raw), &models); err != nil {
		return s.default2026APIModels(), nil
	}
	return models, nil
}

func (s *Server) saveCustomAPIModels(models []CustomAPIModel) error {
	customAPIModelsMu.Lock()
	defer customAPIModelsMu.Unlock()

	if s.Ledger == nil {
		return fmt.Errorf("ledger chưa khởi tạo")
	}
	data, err := json.Marshal(models)
	if err != nil {
		return err
	}
	return s.Ledger.SetSetting(settingKeyCustomAPIModels, string(data))
}

func (s *Server) default2026APIModels() []CustomAPIModel {
	return []CustomAPIModel{
		{
			ID:        "gemini-2-5-pro",
			Name:      "Gemini 2.5 Pro (Official API)",
			Provider:  "gemini",
			Endpoint:  "https://generativelanguage.googleapis.com/v1beta",
			Model:     "gemini-2.5-pro",
			Type:      "multimodal",
			APIKey:    "",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
		{
			ID:        "gemini-2-5-flash",
			Name:      "Gemini 2.5 Flash (Ultra Fast)",
			Provider:  "gemini",
			Endpoint:  "https://generativelanguage.googleapis.com/v1beta",
			Model:     "gemini-2.5-flash",
			Type:      "multimodal",
			APIKey:    "",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
		{
			ID:        "openai-gpt-4o",
			Name:      "OpenAI GPT-4o (Production)",
			Provider:  "openai",
			Endpoint:  "https://api.openai.com/v1",
			Model:     "gpt-4o",
			Type:      "multimodal",
			APIKey:    "",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
		{
			ID:        "claude-3-7-sonnet",
			Name:      "Claude 3.7 Sonnet (Anthropic)",
			Provider:  "anthropic",
			Endpoint:  "https://api.anthropic.com/v1",
			Model:     "claude-3-7-sonnet",
			Type:      "text",
			APIKey:    "",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
		{
			ID:        "runway-gen3",
			Name:      "Runway Gen-3 Alpha Turbo (Video API)",
			Provider:  "custom",
			Endpoint:  "https://api.runwayml.com/v1",
			Model:     "gen3a_turbo",
			Type:      "video",
			APIKey:    "",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
	}
}

// handleListCustomAPIModels: GET /api/custom-models
func (s *Server) handleListCustomAPIModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.listCustomAPIModels()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Ẩn secret key trước khi trả về browser
	safeList := make([]map[string]any, len(models))
	for i, m := range models {
		masked := ""
		if len(m.APIKey) > 8 {
			masked = m.APIKey[:4] + "••••" + m.APIKey[len(m.APIKey)-4:]
		} else if len(m.APIKey) > 0 {
			masked = "••••••••"
		}
		safeList[i] = map[string]any{
			"id":         m.ID,
			"name":       m.Name,
			"provider":   m.Provider,
			"endpoint":   m.Endpoint,
			"model":      m.Model,
			"type":       m.Type,
			"has_key":    m.APIKey != "",
			"masked_key": masked,
			"created_at": m.CreatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":     true,
		"models": safeList,
	})
}

// handleSaveCustomAPIModel: POST /api/custom-models
func (s *Server) handleSaveCustomAPIModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	provider := strings.ToLower(strings.TrimSpace(r.FormValue("provider")))
	endpoint := strings.TrimSpace(r.FormValue("endpoint"))
	model := strings.TrimSpace(r.FormValue("model"))
	modelType := strings.ToLower(strings.TrimSpace(r.FormValue("type")))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	id := strings.TrimSpace(r.FormValue("id"))

	if name == "" {
		http.Error(w, "Vui lòng nhập Tên Cấu Hình", http.StatusBadRequest)
		return
	}
	if model == "" {
		http.Error(w, "Vui lòng nhập Model ID", http.StatusBadRequest)
		return
	}
	if endpoint == "" {
		switch provider {
		case "openai":
			endpoint = "https://api.openai.com/v1"
		case "gemini":
			endpoint = "https://generativelanguage.googleapis.com/v1beta"
		case "anthropic":
			endpoint = "https://api.anthropic.com/v1"
		}
	}
	if modelType == "" {
		modelType = "multimodal"
	}

	models, _ := s.listCustomAPIModels()
	if id == "" {
		id = fmt.Sprintf("api_%d", time.Now().UnixNano())
	}

	updated := false
	for i := range models {
		if models[i].ID == id {
			models[i].Name = name
			models[i].Provider = provider
			models[i].Endpoint = endpoint
			models[i].Model = model
			models[i].Type = modelType
			if apiKey != "" {
				models[i].APIKey = apiKey
			}
			updated = true
			break
		}
	}

	if !updated {
		models = append(models, CustomAPIModel{
			ID:        id,
			Name:      name,
			Provider:  provider,
			Endpoint:  endpoint,
			Model:     model,
			Type:      modelType,
			APIKey:    apiKey,
			CreatedAt: time.Now().Format(time.RFC3339),
		})
	}

	if err := s.saveCustomAPIModels(models); err != nil {
		http.Error(w, "Lưu cấu hình thất bại: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Nếu request gửi từ form HTML thông thường, redirect lại
	if r.Header.Get("Accept") != "application/json" && !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		http.Redirect(w, r, "/settings?tab=apikeys&ok=1", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id})
}

// handleDeleteCustomAPIModel: POST /api/custom-models/delete
func (s *Server) handleDeleteCustomAPIModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		http.Error(w, "Thiếu ID", http.StatusBadRequest)
		return
	}

	models, _ := s.listCustomAPIModels()
	newModels := make([]CustomAPIModel, 0, len(models))
	for _, m := range models {
		if m.ID != id {
			newModels = append(newModels, m)
		}
	}

	if err := s.saveCustomAPIModels(newModels); err != nil {
		http.Error(w, "Xóa thất bại: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("Accept") != "application/json" {
		http.Redirect(w, r, "/settings?tab=apikeys&ok=1", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
