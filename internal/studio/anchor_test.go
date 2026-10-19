package studio

import (
	"strings"
	"testing"
)

func TestVisualAnchor(t *testing.T) {
	anchor := NewVisualAnchor("Review Kem Dưỡng Ẩm Trắng Da", "fashion", "skincare", "data/test_face.png")
	if anchor == nil {
		t.Fatalf("expected non-nil VisualAnchor")
	}

	if anchor.MasterPhotoPath != "data/test_face.png" {
		t.Errorf("expected MasterPhotoPath to be set, got %s", anchor.MasterPhotoPath)
	}

	if anchor.DeterministicSeed == 0 {
		t.Errorf("expected deterministic non-zero seed")
	}

	// 1. Test Master Portrait Prompt
	portraitPrompt := anchor.MasterPortraitPrompt("9:16")
	if !strings.Contains(portraitPrompt, anchor.ActorName) {
		t.Errorf("portrait prompt missing actor name: %s", portraitPrompt)
	}
	if !strings.Contains(portraitPrompt, anchor.Wardrobe) {
		t.Errorf("portrait prompt missing wardrobe: %s", portraitPrompt)
	}
	if !strings.Contains(portraitPrompt, "vertical 9:16 portrait composition") {
		t.Errorf("portrait prompt missing 9:16 composition")
	}
	if !strings.Contains(portraitPrompt, "no text") {
		t.Errorf("portrait prompt should enforce clean frame without text")
	}

	// 2. Test EnrichScenePrompt
	scenePrompt := anchor.EnrichScenePrompt("gently applying the moisturizing cream on her cheeks", "smooth dolly in", 2, 5)
	if !strings.Contains(scenePrompt, "scene 2 of 5") {
		t.Errorf("enriched scene prompt missing scene index: %s", scenePrompt)
	}
	if !strings.Contains(scenePrompt, anchor.FaceTraits) {
		t.Errorf("enriched scene prompt missing facial traits: %s", scenePrompt)
	}
	if !strings.Contains(scenePrompt, anchor.Wardrobe) {
		t.Errorf("enriched scene prompt missing signature wardrobe: %s", scenePrompt)
	}
	if !strings.Contains(scenePrompt, "smooth dolly in") {
		t.Errorf("enriched scene prompt missing camera motion: %s", scenePrompt)
	}

	// 3. Test FlowPrompt
	flowPrompt := anchor.FlowPrompt("smiling warmly at the camera with radiant skin", "slow orbital pan")
	if !strings.Contains(flowPrompt, anchor.ActorName) {
		t.Errorf("flow prompt missing actor name: %s", flowPrompt)
	}
	if !strings.Contains(flowPrompt, "slow orbital pan") {
		t.Errorf("flow prompt missing camera motion: %s", flowPrompt)
	}

	// 4. Test Vietnamese vs Chinese Ethnicity Auto-Detection
	anchorVN := NewVisualAnchor("Vũ Điệu Nóng Bỏng TikTok Việt Nam", "dance", "lifestyle", "")
	if anchorVN.Ethnicity != "Vietnamese" {
		t.Errorf("expected Vietnamese ethnicity for VN dance, got %s", anchorVN.Ethnicity)
	}

	anchorCN := NewVisualAnchor("Douyin Hot Dance 抖音卡点热舞 C-Biz", "dance", "cn", "")
	if anchorCN.Ethnicity != "Chinese" {
		t.Errorf("expected Chinese ethnicity for Douyin/CBiz dance, got %s", anchorCN.Ethnicity)
	}

	// 5. Test Uploaded Model Photo Strict Face Lock
	anchorWithUpload := NewVisualAnchor("Lookbook Thời Trang", "fashion", "apparel", "data/models/my_uploaded_face.png")
	if anchorWithUpload.MasterPhotoPath != "data/models/my_uploaded_face.png" {
		t.Errorf("expected MasterPhotoPath to be preserved, got %s", anchorWithUpload.MasterPhotoPath)
	}
	masterPrompt := anchorWithUpload.MasterPortraitPrompt("9:16")
	if !strings.Contains(masterPrompt, "Strictly conditioned on the uploaded character reference image") {
		t.Errorf("master portrait prompt missing uploaded reference lock: %s", masterPrompt)
	}
}
