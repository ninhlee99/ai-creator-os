package web

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
)

type affiliateFlowConfig struct {
	Topic           string
	ProductURL      string
	Prompt          string
	Resolution      string
	Voice           string
	Duration        float64
	ReferenceVideo  string
	ProductID       int64
	ProductPhotos   []string
	CharacterPhotos []string
}

func prepareAffiliateFlow(graph *flow.FlowGraph, cfg affiliateFlowConfig) map[string]any {
	inputs := map[string]any{
		"product_name":      cfg.Topic,
		"product_url":       cfg.ProductURL,
		"prompt":            cfg.Prompt,
		"resolution":        cfg.Resolution,
		"output_resolution": cfg.Resolution,
		"voice":             cfg.Voice,
		"duration":          cfg.Duration,
	}
	if cfg.ReferenceVideo != "" {
		inputs["sample_video"] = cfg.ReferenceVideo
		inputs["reference_video"] = cfg.ReferenceVideo
	}
	if len(cfg.ProductPhotos) > 0 {
		inputs["product_image"] = cfg.ProductPhotos[0]
		inputs["product_images"] = append([]string(nil), cfg.ProductPhotos...)
	}

	if graph == nil {
		return inputs
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		if node.Params == nil {
			node.Params = map[string]any{}
		}
		switch node.Type {
		case "product_input":
			if cfg.Topic != "" {
				node.Params["product_title"] = cfg.Topic
			}
			if cfg.ProductID > 0 {
				node.Params["product_id"] = strconv.FormatInt(cfg.ProductID, 10)
			}
			if len(cfg.ProductPhotos) > 0 {
				node.Params["product_image"] = cfg.ProductPhotos[0]
				node.Params["product_images"] = append([]string(nil), cfg.ProductPhotos...)
			}
			if cfg.ProductURL != "" {
				node.Params["affiliate_url"] = cfg.ProductURL
			}
		case "character_input":
			if len(cfg.CharacterPhotos) > 0 {
				node.Params["model_path"] = cfg.CharacterPhotos[0]
				node.Params["model_paths"] = append([]string(nil), cfg.CharacterPhotos...)
			}
		}
	}
	return inputs
}

type viralFlowConfig struct {
	SourceVideo     string
	Resolution      string
	CharacterPhotos []string
	RequireRadar    bool
}

func prepareViralFlow(graph *flow.FlowGraph, cfg viralFlowConfig) map[string]any {
	inputs := map[string]any{
		"video":             cfg.SourceVideo,
		"sample_video":      cfg.SourceVideo,
		"resolution":        cfg.Resolution,
		"output_resolution": cfg.Resolution,
	}
	if len(cfg.CharacterPhotos) > 0 {
		inputs["character"] = cfg.CharacterPhotos[0]
		inputs["model_photo"] = cfg.CharacterPhotos[0]
		inputs["character_photos"] = append([]string(nil), cfg.CharacterPhotos...)
	}
	if graph == nil {
		return inputs
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		if node.Params == nil {
			node.Params = map[string]any{}
		}
		switch node.Type {
		case "character_input":
			if len(cfg.CharacterPhotos) > 0 {
				node.Params["model_path"] = cfg.CharacterPhotos[0]
				node.Params["model_paths"] = append([]string(nil), cfg.CharacterPhotos...)
			}
		case "video_source":
			node.Params["video_path"] = cfg.SourceVideo
			node.Params["require_radar_dance"] = cfg.RequireRadar
		}
	}
	return inputs
}

func loadAffiliateFlow(st *studio.Studio, workflowID string) (*flow.FlowGraph, error) {
	if st == nil {
		return nil, fmt.Errorf("Studio chưa sẵn sàng")
	}
	fs, err := st.GetFlowStore()
	if err != nil {
		return nil, fmt.Errorf("không mở được kho workflow: %w", err)
	}
	if fs == nil {
		return nil, fmt.Errorf("kho workflow chưa sẵn sàng")
	}
	graph, err := fs.GetGraph(workflowID)
	if err != nil {
		return nil, fmt.Errorf("không tìm thấy workflow %s: %w", workflowID, err)
	}
	if graph == nil {
		return nil, fmt.Errorf("workflow %s không có dữ liệu", workflowID)
	}
	return graph, nil
}

func prioritizeSelectedCharacterPhotos(libraryPhotos, selectedPhotos []string) []string {
	if len(selectedPhotos) == 0 {
		return append([]string(nil), libraryPhotos...)
	}
	ordered := append([]string(nil), selectedPhotos...)
	selected := make(map[string]struct{}, len(selectedPhotos))
	for _, path := range selectedPhotos {
		selected[path] = struct{}{}
	}
	for _, path := range libraryPhotos {
		if _, exists := selected[path]; !exists {
			ordered = append(ordered, path)
		}
	}
	return ordered
}

func missingChannelCharacter(outputType string, characterPhotos []string) string {
	if outputType != "affiliate" && outputType != "viral" {
		return ""
	}
	for _, path := range characterPhotos {
		if strings.TrimSpace(path) != "" {
			return ""
		}
	}
	return "Kênh chưa có ảnh nhân vật đã tải lên. Tải ảnh nhân vật vào profile kênh trước khi tạo video."
}
