package flow

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed presets/*.json
var presetFiles embed.FS

// GetPresetGraphs loads immutable workflow templates from JSON data files.
func GetPresetGraphs() []FlowGraph {
	paths, err := fs.Glob(presetFiles, "presets/*.json")
	if err != nil {
		panic(fmt.Errorf("list workflow presets: %w", err))
	}
	sort.Strings(paths)
	graphs := make([]FlowGraph, 0, len(paths))
	for _, path := range paths {
		graph := loadPreset(path)
		graphs = append(graphs, graph)
	}
	return graphs
}

func loadPreset(path string) FlowGraph {
	data, err := presetFiles.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("read workflow preset %q: %w", path, err))
	}
	var graph FlowGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		panic(fmt.Errorf("decode workflow preset %q: %w", path, err))
	}
	graph.IsPreset = true
	return graph
}

func presetByID(id string) FlowGraph {
	for _, graph := range GetPresetGraphs() {
		if graph.ID == id {
			return graph
		}
	}
	panic(fmt.Errorf("workflow preset %q not found", id))
}

// Exported constructors remain as compatibility helpers; preset content lives in JSON.
func PresetViralDance() FlowGraph         { return presetByID("preset_viral_dance") }
func PresetAffiliateShowcase() FlowGraph  { return presetByID("preset_affiliate_showcase") }
func PresetFashionTryOnAngles() FlowGraph { return presetByID("preset_fashion_tryon") }
