package web

import (
	"reflect"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/studio/flow"
)

func TestVideoCreateFlowRoutingScenarios(t *testing.T) {
	const (
		profileCharacter = "/profile/model.jpg"
		uploadCharacter  = "/upload/new-model.jpg"
		productPhoto     = "/product/listing.jpg"
		referenceVideo   = "/reference/sample.mp4"
	)

	tests := []struct {
		name            string
		kind            string
		characterPhotos []string
		referenceVideo  string
		wantCharacter   string
		wantReference   bool
		wantRadarSource bool
	}{
		{name: "product + profile character", kind: "affiliate", characterPhotos: []string{profileCharacter}, wantCharacter: profileCharacter},
		{name: "product + profile character + reference", kind: "affiliate", characterPhotos: []string{profileCharacter}, referenceVideo: referenceVideo, wantCharacter: profileCharacter, wantReference: true},
		{name: "product + uploaded character", kind: "affiliate", characterPhotos: []string{uploadCharacter, profileCharacter}, wantCharacter: uploadCharacter},
		{name: "product + uploaded character + reference", kind: "affiliate", characterPhotos: []string{uploadCharacter, profileCharacter}, referenceVideo: referenceVideo, wantCharacter: uploadCharacter, wantReference: true},
		{name: "viral TikTok + profile character + reference", kind: "viral", characterPhotos: []string{profileCharacter}, referenceVideo: referenceVideo, wantCharacter: profileCharacter, wantReference: true, wantRadarSource: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			photos := prioritizeSelectedCharacterPhotos([]string{profileCharacter}, tt.characterPhotos)
			if tt.kind == "affiliate" {
				graph := flow.PresetAffiliateShowcase()
				inputs := prepareAffiliateFlow(&graph, affiliateFlowConfig{
					Topic: "Demo product", ProductURL: "https://shop.example/product",
					Resolution: "1080p", Duration: 30, ReferenceVideo: tt.referenceVideo,
					ProductPhotos: []string{productPhoto}, CharacterPhotos: photos,
				})
				if got := inputs["product_image"]; got != productPhoto {
					t.Errorf("product_image=%v, want %q", got, productPhoto)
				}
				if got := inputs["reference_video"]; tt.wantReference && got != referenceVideo {
					t.Errorf("reference_video=%v, want %q", got, referenceVideo)
				} else if !tt.wantReference && got != nil {
					t.Errorf("reference_video=%v, want omitted", got)
				}
				assertFlowNodeParam(t, graph, "product_input", "product_image", productPhoto)
				assertFlowNodeParam(t, graph, "character_input", "model_path", tt.wantCharacter)
				assertFlowNodeParam(t, graph, "character_input", "model_paths", photos)
				return
			}

			graph := flow.PresetViralDance()
			inputs := prepareViralFlow(&graph, viralFlowConfig{
				SourceVideo: tt.referenceVideo, Resolution: "1080p",
				CharacterPhotos: photos, RequireRadar: tt.wantRadarSource,
			})
			if inputs["video"] != referenceVideo || inputs["sample_video"] != referenceVideo {
				t.Errorf("video inputs=%v/%v, want %q", inputs["video"], inputs["sample_video"], referenceVideo)
			}
			assertFlowNodeParam(t, graph, "character_input", "model_path", tt.wantCharacter)
			assertFlowNodeParam(t, graph, "character_input", "model_paths", photos)
			assertFlowNodeParam(t, graph, "video_source", "video_path", referenceVideo)
			assertFlowNodeParam(t, graph, "video_source", "require_radar_dance", true)
		})
	}
}

func assertFlowNodeParam(t *testing.T, graph flow.FlowGraph, nodeType, key string, want any) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Type == nodeType {
			got, ok := node.Params[key]
			if !ok {
				t.Fatalf("node %q missing param %q", node.ID, key)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("node %q param %q=%#v, want %#v", node.ID, key, got, want)
			}
			return
		}
	}
	t.Fatalf("graph %q missing node type %q", graph.ID, nodeType)
}

func TestPrioritizeSelectedCharacterPhotos(t *testing.T) {
	got := prioritizeSelectedCharacterPhotos(
		[]string{"/profile/old.jpg", "/upload/new.jpg", "/profile/other.jpg"},
		[]string{"/upload/new.jpg"},
	)
	want := []string{"/upload/new.jpg", "/profile/old.jpg", "/profile/other.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered photos=%v, want %v", got, want)
	}
}

func TestMissingChannelCharacterGuard(t *testing.T) {
	for _, outputType := range []string{"affiliate", "viral"} {
		if got := missingChannelCharacter(outputType, nil); got == "" {
			t.Errorf("output type %q must reject missing profile character", outputType)
		}
		if got := missingChannelCharacter(outputType, []string{"/profile/model.jpg"}); got != "" {
			t.Errorf("output type %q rejected profile character: %s", outputType, got)
		}
	}
	if got := missingChannelCharacter("workflow", nil); got != "" {
		t.Errorf("custom workflow unexpectedly requires profile character: %s", got)
	}
}
