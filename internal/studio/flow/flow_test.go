package flow

import (
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestTC01_NodeRegistry(t *testing.T) {
	defs := ListDefinitions()
	if len(defs) < 17 {
		t.Fatalf("TC-01: Mong muốn ít nhất 17 node types, nhận được: %d", len(defs))
	}

	requiredTypes := []string{
		"character_input", "product_input", "video_source", "audio_source",
		"prompt_template", "product_extractor", "image_composite",
		"dance_motion_extractor", "dance_retarget", "google_flow_video",
		"muse_ai_video", "gemini_vision", "custom_web_ai", "audio_merge",
		"video_upscale", "concat_scenes", "video_output", "image_output",
	}

	for _, reqType := range requiredTypes {
		def, ok := GetNodeDefinition(reqType)
		if !ok {
			t.Errorf("TC-01: Chưa đăng ký node type: %s", reqType)
			continue
		}
		if def.Title == "" {
			t.Errorf("TC-01: Node %s thiếu Title", reqType)
		}
		if def.Category == "" {
			t.Errorf("TC-01: Node %s thiếu Category", reqType)
		}
		if def.Executor == nil {
			t.Errorf("TC-01: Node %s thiếu hàm Executor", reqType)
		}
	}
}

func TestTC02_CycleDetection(t *testing.T) {
	// Tạo đồ thị có chu trình: N1 -> N2 -> N3 -> N1
	g := &FlowGraph{
		ID:   "cycle_graph",
		Name: "Cycle Test",
		Nodes: []FlowNode{
			{ID: "n1", Type: "prompt_template"},
			{ID: "n2", Type: "prompt_template"},
			{ID: "n3", Type: "prompt_template"},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "n1", FromPort: "prompt", ToNode: "n2", ToPort: "character_desc"},
			{ID: "e2", FromNode: "n2", FromPort: "prompt", ToNode: "n3", ToPort: "character_desc"},
			{ID: "e3", FromNode: "n3", FromPort: "prompt", ToNode: "n1", ToPort: "character_desc"},
		},
	}

	_, err := ValidateGraph(g)
	if err == nil {
		t.Fatalf("TC-02: Kỳ vọng lỗi chu trình tuần hoàn, nhưng ValidateGraph không trả về lỗi")
	}
	if !strings.Contains(err.Error(), "chu trình") && !strings.Contains(err.Error(), "circular") {
		t.Errorf("TC-02: Thông báo lỗi chu trình không chuẩn: %v", err)
	}
}

func TestTC03_PortTypeSafety(t *testing.T) {
	// Kết nối không tương thích: video -> text
	g := &FlowGraph{
		ID:   "bad_port_graph",
		Name: "Bad Port Test",
		Nodes: []FlowNode{
			{ID: "n_vid", Type: "video_source"},
			{ID: "n_prompt", Type: "prompt_template"},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "n_vid", FromPort: "video_path", ToNode: "n_prompt", ToPort: "character_desc"},
		},
	}

	_, err := ValidateGraph(g)
	if err == nil {
		t.Fatalf("TC-03: Kỳ vọng lỗi kiểu dữ liệu cổng không khớp (video -> text)")
	}
	if !strings.Contains(err.Error(), "không khớp") && !strings.Contains(err.Error(), "kiểu dữ liệu") {
		t.Errorf("TC-03: Lỗi không nhắc đến sự không tương thích cổng: %v", err)
	}
}

func TestTC04_TopologicalOrder(t *testing.T) {
	// Đồ thị nhánh hợp lệ: (Char, Prod) -> Extractor -> Composite -> Prompt
	g := &FlowGraph{
		ID:   "dag_graph",
		Name: "DAG Test",
		Nodes: []FlowNode{
			{ID: "n_composite", Type: "image_composite"},
			{ID: "n_char", Type: "character_input"},
			{ID: "n_prod", Type: "product_input"},
			{ID: "n_extract", Type: "product_extractor"},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "n_prod", FromPort: "product_image", ToNode: "n_extract", ToPort: "source_image"},
			{ID: "e2", FromNode: "n_char", FromPort: "character_image", ToNode: "n_composite", ToPort: "character_image"},
			{ID: "e3", FromNode: "n_extract", FromPort: "cutout_image", ToNode: "n_composite", ToPort: "product_image"},
		},
	}

	order, err := ValidateGraph(g)
	if err != nil {
		t.Fatalf("TC-04: ValidateGraph trả về lỗi: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("TC-04: Thứ tự topo mong muốn 4 node, nhận: %d", len(order))
	}

	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}

	if pos["n_prod"] > pos["n_extract"] {
		t.Errorf("TC-04: n_prod phải chạy trước n_extract")
	}
	if pos["n_extract"] > pos["n_composite"] {
		t.Errorf("TC-04: n_extract phải chạy trước n_composite")
	}
	if pos["n_char"] > pos["n_composite"] {
		t.Errorf("TC-04: n_char phải chạy trước n_composite")
	}
}

func TestTC05_PresetsValidation(t *testing.T) {
	presets := GetPresetGraphs()
	if len(presets) < 3 {
		t.Fatalf("TC-05: Cần ít nhất 3 preset mẫu, có: %d", len(presets))
	}

	for _, p := range presets {
		order, err := ValidateGraph(&p)
		if err != nil {
			t.Errorf("TC-05: Preset %s không hợp lệ: %v", p.ID, err)
		}
		if len(order) != len(p.Nodes) {
			t.Errorf("TC-05: Preset %s số lượng node topo không khớp: got %d, want %d", p.ID, len(order), len(p.Nodes))
		}
	}
}

func setupTestContext(t *testing.T) (*ExecutionContext, string) {
	tmpDir, err := os.MkdirTemp("", "flow-test-*")
	if err != nil {
		t.Fatalf("tạo temp dir: %v", err)
	}

	workDir := filepath.Join(tmpDir, "work")
	outDir := filepath.Join(tmpDir, "out")
	_ = os.MkdirAll(workDir, 0o755)
	_ = os.MkdirAll(outDir, 0o755)

	dummyChar := filepath.Join(workDir, "default_char.jpg")
	_ = writeDummyImage(dummyChar, 736, 982, "CHARACTER")

	dummyProd := filepath.Join(workDir, "default_prod.jpg")
	_ = writeDummyImage(dummyProd, 800, 800, "PRODUCT")

	dummyVid := filepath.Join(workDir, "sample_916.mp4")
	_ = writeDummyVideo(&ExecutionContext{WorkDir: workDir}, dummyVid, 10.0)

	dummyAudio := filepath.Join(workDir, "preview.mp3")
	_ = writeDummyAudio(&ExecutionContext{WorkDir: workDir}, dummyAudio, 10.0)

	execCtx := &ExecutionContext{
		Context: context.Background(),
		RunID:   "test_run_123",
		WorkDir: workDir,
		OutDir:  outDir,
		Log:     func(msg string) {},
		EngineCall: func(ctx context.Context, platform string, prompt string, images []string, aspect string, outPath string) (string, error) {
			return "test_project", writeDummyVideo(&ExecutionContext{WorkDir: workDir}, outPath, 3.0)
		},
		VisionEval: func(ctx context.Context, prompt string, imagePath string) (string, error) {
			return "Asian female model wearing elegant outfit", nil
		},
		AnalyzeImages: func(ctx context.Context, prompt string, imagePaths []string) (string, error) {
			return "Asian female model dancing gracefully with sexy choreography", nil
		},
		GenerateText: func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
			return "Cinematic shot of elegant dancer in modern studio", nil
		},
		CharacterPhotos: func(accountID int64) ([]string, error) {
			return []string{dummyChar}, nil
		},
		ResolveProduct: func(ctx context.Context, id int64) (FlowProduct, error) {
			return FlowProduct{ID: id, Title: "Sản phẩm thời trang mẫu", ImageURLs: []string{dummyProd}}, nil
		},
		ResolveRadarVideo: func(ctx context.Context, sourceURL string, requireDance bool) (FlowRadarVideo, error) {
			return FlowRadarVideo{VideoURL: dummyVid, SoundTitle: "Dance Beat", Author: "@dancer"}, nil
		},
		FFmpegRunner: func(ctx context.Context, args ...string) error {
			if len(args) > 0 {
				lastArg := args[len(args)-1]
				if isVideoFile(lastArg) {
					return writeDummyVideo(&ExecutionContext{WorkDir: workDir}, lastArg, 5.0)
				}
				if isImageFile(lastArg) {
					return writeDummyImage(lastArg, 1080, 1920, "TEST")
				}
				if isAudioFile(lastArg) {
					return writeDummyAudio(&ExecutionContext{WorkDir: workDir}, lastArg, 5.0)
				}
			}
			return nil
		},
	}

	return execCtx, tmpDir
}

func TestTC06_ExecuteAssetNodes(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	// Character Node
	charNode := &FlowNode{ID: "c1", Type: "character_input", Params: map[string]any{"model_name": "Test Char"}}
	charOut, err := executeCharacterInput(ctx, charNode, nil)
	if err != nil {
		t.Fatalf("executeCharacterInput lỗi: %v", err)
	}
	if charOut["character_image"] == "" || !fileExists(charOut["character_image"].(string)) {
		t.Errorf("character_image không hợp lệ: %v", charOut["character_image"])
	}

	// Product Node
	prodNode := &FlowNode{ID: "p1", Type: "product_input", Params: map[string]any{"product_title": "Son Môi Kem Miu"}}
	prodOut, err := executeProductInput(ctx, prodNode, nil)
	if err != nil {
		t.Fatalf("executeProductInput lỗi: %v", err)
	}
	if prodOut["product_image"] == "" || !fileExists(prodOut["product_image"].(string)) {
		t.Errorf("product_image không hợp lệ: %v", prodOut["product_image"])
	}

	// Prompt Template Node
	promptNode := &FlowNode{ID: "pr1", Type: "prompt_template", Params: map[string]any{
		"template": "Video of {character} with {product}",
	}}
	promptOut, err := executePromptTemplate(ctx, promptNode, map[string]any{
		"character_desc": "Miu Model",
		"product_desc":   "Lipstick",
	})
	if err != nil {
		t.Fatalf("executePromptTemplate lỗi: %v", err)
	}
	promptStr := promptOut["prompt"].(string)
	if promptStr != "Video of Miu Model with Lipstick" {
		t.Errorf("Prompt không khớp: %s", promptStr)
	}
}

func TestTC07_ExecuteProcessNodes(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	dummyImg := filepath.Join(ctx.WorkDir, "dummy.jpg")
	_ = writeDummyImage(dummyImg, 800, 800, "TEST")

	// 1. Product Extractor
	extNode := &FlowNode{ID: "ext1", Type: "product_extractor"}
	extOut, err := executeProductExtractor(ctx, extNode, map[string]any{
		"source_image": dummyImg,
	})
	if err != nil {
		t.Fatalf("executeProductExtractor lỗi: %v", err)
	}
	if !fileExists(extOut["cutout_image"].(string)) {
		t.Errorf("cutout_image không tồn tại: %v", extOut["cutout_image"])
	}

	// 2. Image Composite
	compNode := &FlowNode{ID: "comp1", Type: "image_composite"}
	compOut, err := executeImageComposite(ctx, compNode, map[string]any{
		"character_image": dummyImg,
		"product_image":   extOut["cutout_image"],
	})
	if err != nil {
		t.Fatalf("executeImageComposite lỗi: %v", err)
	}
	if !fileExists(compOut["composite_image"].(string)) {
		t.Errorf("composite_image không tồn tại: %v", compOut["composite_image"])
	}

	// 3. Dance Retarget
	retargetNode := &FlowNode{ID: "retarget1", Type: "dance_retarget"}
	retargetOut, err := executeDanceRetarget(ctx, retargetNode, map[string]any{
		"character_image": dummyImg,
		"motion_data":     map[string]any{"bpm": 120},
	})
	if err != nil {
		t.Fatalf("executeDanceRetarget lỗi: %v", err)
	}
	if retargetOut["prompt_choreography"] == "" {
		t.Errorf("prompt_choreography rỗng")
	}
}

func TestTC08_ExecuteAINodes(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	dummyImg := filepath.Join(ctx.WorkDir, "dummy.jpg")
	_ = writeDummyImage(dummyImg, 800, 800, "TEST")

	// 1. Google Flow Video Node
	flowNode := &FlowNode{ID: "flow1", Type: "google_flow_video"}
	flowOut, err := executeGoogleFlowVideo(ctx, flowNode, map[string]any{
		"prompt":      "A beautiful dancing video",
		"ingredients": []string{dummyImg},
	})
	if err != nil {
		t.Fatalf("executeGoogleFlowVideo lỗi: %v", err)
	}
	if !fileExists(flowOut["video_out"].(string)) {
		t.Errorf("video_out từ Google Flow không tồn tại")
	}

	// 2. Muse AI Video Node
	museNode := &FlowNode{ID: "muse1", Type: "muse_ai_video"}
	museOut, err := executeMuseAIVideo(ctx, museNode, map[string]any{
		"prompt":          "Product showcase video",
		"reference_image": dummyImg,
	})
	if err != nil {
		t.Fatalf("executeMuseAIVideo lỗi: %v", err)
	}
	if !fileExists(museOut["video_out"].(string)) {
		t.Errorf("video_out từ Muse AI không tồn tại")
	}

	// 3. Gemini Vision Node
	geminiNode := &FlowNode{ID: "gemini1", Type: "gemini_vision"}
	geminiOut, err := executeGeminiVision(ctx, geminiNode, map[string]any{
		"image_in": dummyImg,
	})
	if err != nil {
		t.Fatalf("executeGeminiVision lỗi: %v", err)
	}
	if geminiOut["optimized_prompt"] == "" {
		t.Errorf("optimized_prompt rỗng")
	}
}

func TestTC09_FullPresetViralDanceExecution(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	preset := PresetViralDance()
	run, err := ExecuteGraph(ctx, &preset)
	if err != nil {
		logMsg := ""
		if run != nil {
			logMsg = run.Log
		}
		t.Fatalf("TC-09: Thực thi PresetViralDance thất bại: %v (Log: %s)", err, logMsg)
	}

	if run.Status != "completed" {
		t.Errorf("TC-09: Trạng thái mong muốn 'completed', nhận: %s", run.Status)
	}
	if run.Progress != 100 {
		t.Errorf("TC-09: Tiến độ mong muốn 100%%, nhận: %d", run.Progress)
	}
	if run.FinalOutput == "" || !fileExists(run.FinalOutput) {
		t.Errorf("TC-09: FinalOutput video không tồn tại: %s", run.FinalOutput)
	}

	// Kiểm tra trạng thái từng node đều là completed
	for _, n := range preset.Nodes {
		st, ok := run.NodeStates[n.ID]
		if !ok || st.Status != "completed" {
			t.Errorf("TC-09: Node %s chưa hoàn thành (status=%s)", n.ID, st.Status)
		}
	}
}

func TestTC10_FullPresetAffiliateShowcaseExecution(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	preset := PresetAffiliateShowcase()
	run, err := ExecuteGraph(ctx, &preset)
	if err != nil {
		t.Fatalf("TC-10: Thực thi PresetAffiliateShowcase thất bại: %v (Log: %s)", err, run.Log)
	}

	if run.Status != "completed" {
		t.Errorf("TC-10: Trạng thái mong muốn 'completed', nhận: %s", run.Status)
	}
	if run.Progress != 100 {
		t.Errorf("TC-10: Tiến độ mong muốn 100%%, nhận: %d", run.Progress)
	}
	if run.FinalOutput == "" || !fileExists(run.FinalOutput) {
		t.Errorf("TC-10: FinalOutput video không tồn tại: %s", run.FinalOutput)
	}
}

func TestTC13_FullPresetFashionTryOnExecution(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	preset := PresetFashionTryOnAngles()
	run, err := ExecuteGraph(ctx, &preset)
	if err != nil {
		t.Fatalf("TC-13: Thực thi PresetFashionTryOnAngles thất bại: %v (Log: %s)", err, run.Log)
	}

	if run.Status != "completed" {
		t.Errorf("TC-13: Trạng thái mong muốn 'completed', nhận: %s", run.Status)
	}
	if run.Progress != 100 {
		t.Errorf("TC-13: Tiến độ mong muốn 100%%, nhận: %d", run.Progress)
	}
	if run.FinalOutput == "" || !fileExists(run.FinalOutput) {
		t.Errorf("TC-13: FinalOutput video không tồn tại: %s", run.FinalOutput)
	}

	for _, n := range preset.Nodes {
		st, ok := run.NodeStates[n.ID]
		if !ok || st.Status != "completed" {
			t.Errorf("TC-13: Node %s (%s) chưa hoàn thành (status=%s)", n.ID, n.Type, st.Status)
		}
	}
}

func TestTC11_RunCancellation(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	cancelCtx, cancel := context.WithCancel(context.Background())
	ctx.Context = cancelCtx

	// Hủy context ngay lập tức
	cancel()

	preset := PresetViralDance()
	run, err := ExecuteGraph(ctx, &preset)
	if err == nil {
		t.Fatalf("TC-11: Kỳ vọng lỗi hủy bỏ context, nhưng không nhận được")
	}
	if run.Status != "cancelled" {
		t.Errorf("TC-11: Trạng thái mong muốn 'cancelled', nhận: %s", run.Status)
	}
}

func TestTC12_StoreCRUD(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("mở in-memory db: %v", err)
	}
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore lỗi: %v", err)
	}

	// 1. Kiểm tra ban đầu không tự động seed preset (theo yêu cầu hệ thống sạch)
	graphs, err := store.ListGraphs()
	if err != nil {
		t.Fatalf("ListGraphs lỗi: %v", err)
	}
	if len(graphs) != 0 {
		t.Errorf("Kỳ vọng ban đầu không tự động seed preset, có: %d", len(graphs))
	}
	// Kiểm tra gọi thủ công SeedDefaultPresets khi được yêu cầu lưu thành flow hệ thống
	if err := store.SeedDefaultPresets(); err != nil {
		t.Fatalf("SeedDefaultPresets lỗi: %v", err)
	}
	graphs, _ = store.ListGraphs()
	if len(graphs) < 3 {
		t.Errorf("Kỳ vọng ít nhất 3 preset sau khi gọi SeedDefaultPresets, có: %d", len(graphs))
	}

	// 2. Lưu graph tùy chỉnh
	custom := &FlowGraph{
		ID:          "custom_g1",
		Name:        "Test Custom Flow",
		Description: "Mô tả",
		Category:    "custom",
		Nodes: []FlowNode{
			{ID: "n1", Type: "character_input"},
		},
	}
	if err := store.SaveGraph(custom); err != nil {
		t.Fatalf("SaveGraph lỗi: %v", err)
	}

	// 3. Lấy lại graph
	retrieved, err := store.GetGraph("custom_g1")
	if err != nil {
		t.Fatalf("GetGraph lỗi: %v", err)
	}
	if retrieved.Name != "Test Custom Flow" {
		t.Errorf("Tên graph không khớp: %s", retrieved.Name)
	}

	// 4. Lưu và lấy FlowRun
	run := &FlowRun{
		ID:        "run_test_99",
		GraphID:   "custom_g1",
		GraphName: "Test Custom Flow",
		Status:    "running",
		Progress:  50,
	}
	if err := store.SaveRun(run); err != nil {
		t.Fatalf("SaveRun lỗi: %v", err)
	}
	retRun, err := store.GetRun("run_test_99")
	if err != nil {
		t.Fatalf("GetRun lỗi: %v", err)
	}
	if retRun.Progress != 50 {
		t.Errorf("Progress run không khớp: %d", retRun.Progress)
	}

	// 5. Xóa graph tùy chỉnh
	if err := store.DeleteGraph("custom_g1"); err != nil {
		t.Fatalf("DeleteGraph lỗi: %v", err)
	}
	_, err = store.GetGraph("custom_g1")
	if err == nil {
		t.Errorf("Kỳ vọng lỗi khi lấy graph đã xóa")
	}

	// 6. Xóa preset phải bị từ chối
	if err := store.DeleteGraph("preset_viral_dance"); err == nil {
		t.Errorf("Kỳ vọng không cho phép xóa preset mẫu")
	}
}

func TestTC14_MultiDependencyExecution(t *testing.T) {
	workDir, err := os.MkdirTemp("", "flow-dep-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(workDir)

	outDir := filepath.Join(workDir, "out")
	_ = os.MkdirAll(outDir, 0o755)
	_ = writeDummyImage(filepath.Join(workDir, "default_char.jpg"), 736, 982, "CHARACTER")
	_ = writeDummyImage(filepath.Join(workDir, "default_prod.jpg"), 800, 800, "PRODUCT")

	// Đồ thị: n_char và n_prod đều nối vào n_composite -> n_out
	graph := &FlowGraph{
		ID:   "multi_dep_graph",
		Name: "Multi Dependency Test",
		Nodes: []FlowNode{
			{ID: "n_char", Type: "character_input", Title: "Nhân vật"},
			{ID: "n_prod", Type: "product_input", Title: "Sản phẩm"},
			{ID: "n_comp", Type: "image_composite", Title: "Ghép phối cảnh"},
			{ID: "n_out", Type: "image_output", Title: "Ảnh xuất"},
		},
		Edges: []FlowEdge{
			{ID: "e1", FromNode: "n_char", FromPort: "character_image", ToNode: "n_comp", ToPort: "character_image"},
			{ID: "e2", FromNode: "n_prod", FromPort: "product_image", ToNode: "n_comp", ToPort: "product_image"},
			{ID: "e3", FromNode: "n_comp", FromPort: "composite_image", ToNode: "n_out", ToPort: "image_in"},
		},
	}

	ctx := &ExecutionContext{
		Context: context.Background(),
		RunID:   "run_dep_test",
		WorkDir: workDir,
		OutDir:  outDir,
		Log:     func(msg string) {},
		FFmpegRunner: func(ctx context.Context, args ...string) error {
			if len(args) > 0 {
				lastArg := args[len(args)-1]
				if isImageFile(lastArg) {
					return writeDummyImage(lastArg, 1080, 1920, "COMPOSITE")
				}
			}
			return nil
		},
	}

	run, err := ExecuteGraph(ctx, graph)
	if err != nil {
		t.Fatalf("ExecuteGraph lỗi: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("Kỳ vọng completed, nhận: %s (log: %s)", run.Status, run.Log)
	}

	// Đảm bảo cả 4 node đều có status completed
	for _, n := range graph.Nodes {
		st, ok := run.NodeStates[n.ID]
		if !ok || st.Status != "completed" {
			t.Errorf("Node %s chưa hoàn thành: %+v", n.ID, st)
		}
	}
}

func TestTC17_ProductExtractorFromVideoAndImage(t *testing.T) {
	workDir := filepath.Join(os.TempDir(), "aicos_test_tc17_work")
	outDir := filepath.Join(os.TempDir(), "aicos_test_tc17_out")
	_ = os.RemoveAll(workDir)
	_ = os.RemoveAll(outDir)
	_ = os.MkdirAll(workDir, 0o755)
	_ = os.MkdirAll(outDir, 0o755)
	defer os.RemoveAll(workDir)
	defer os.RemoveAll(outDir)

	// Tạo một ảnh mẫu RGBA để test bóc tách
	testImgPath := filepath.Join(workDir, "test_prod.png")
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if x >= 25 && x <= 75 && y >= 25 && y <= 75 {
				img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 240, G: 240, B: 240, A: 255})
			}
		}
	}
	f, err := os.Create(testImgPath)
	if err != nil {
		t.Fatalf("tạo test img lỗi: %v", err)
	}
	_ = png.Encode(f, img)
	_ = f.Close()

	graph := &FlowGraph{
		ID:   "extractor_test_graph",
		Name: "Product Extractor Test",
		Nodes: []FlowNode{
			{
				ID:   "ext_node",
				Type: "product_extractor",
				Params: map[string]any{
					"source_image": testImgPath,
				},
			},
		},
	}

	ctx := &ExecutionContext{
		Context: context.Background(),
		RunID:   "run_ext_test",
		WorkDir: workDir,
		OutDir:  outDir,
		Log:     func(msg string) {},
	}

	run, err := ExecuteGraph(ctx, graph)
	if err != nil {
		t.Fatalf("ExecuteGraph lỗi: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("Kỳ vọng completed, nhận: %s (log: %s)", run.Status, run.Log)
	}

	st, ok := run.NodeStates["ext_node"]
	if !ok || st.Status != "completed" {
		t.Fatalf("Node ext_node chưa hoàn thành: %+v", st)
	}

	if st.OutputPreview == "" || !fileExists(st.OutputPreview) {
		t.Errorf("Kỳ vọng OutputPreview tồn tại, nhận: %v", st.OutputPreview)
	}
}

func TestTC18_MediaDownloaderHTMLScraper(t *testing.T) {
	// Giả lập một trang web HTML có thẻ OpenGraph hình ảnh và video
	htmlPayload := `
<!DOCTYPE html>
<html>
<head>
  <meta property="og:title" content="Sản phẩm Test" />
  <meta property="og:image" content="https://example.com/assets/banner_product.jpg" />
  <meta property="og:video" content="https://example.com/assets/sample_video.mp4" />
</head>
<body>
  <h1>Chi tiết sản phẩm</h1>
  <img src="/img/detail.png" />
</body>
</html>
`
	videoURL := extractMediaURLFromHTML(htmlPayload, "https://example.com/item/123", "video")
	if videoURL != "https://example.com/assets/sample_video.mp4" {
		t.Errorf("Kỳ vọng lấy được video URL, nhận: %s", videoURL)
	}

	imageURL := extractMediaURLFromHTML(htmlPayload, "https://example.com/item/123", "image")
	if imageURL != "https://example.com/assets/banner_product.jpg" {
		t.Errorf("Kỳ vọng lấy được image URL, nhận: %s", imageURL)
	}
}

func TestTC21_PreviewMediaNode(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "out")
	_ = os.MkdirAll(outDir, 0755)

	testImg := filepath.Join(tmpDir, "test_sample.png")
	if err := writeDummyImage(testImg, 400, 400, "Sample Image for Preview"); err != nil {
		t.Fatalf("writeDummyImage failed: %v", err)
	}

	g := FlowGraph{
		ID:   "test_preview_graph",
		Name: "Test Preview Node Execution",
		Nodes: []FlowNode{
			{
				ID:    "char_node",
				Type:  "character_input",
				Title: "Nhân vật",
				Params: map[string]any{
					"model_path": testImg,
				},
			},
			{
				ID:    "preview_node",
				Type:  "preview_media",
				Title: "Xem trước kết quả",
			},
		},
		Edges: []FlowEdge{
			{
				ID:       "e_char_preview",
				FromNode: "char_node",
				FromPort: "character_image",
				ToNode:   "preview_node",
				ToPort:   "media_in",
			},
		},
	}

	ctx := &ExecutionContext{
		Context: context.Background(),
		WorkDir: tmpDir,
		OutDir:  outDir,
		Log:     func(msg string) {},
	}
	run, err := ExecuteGraph(ctx, &g)
	if err != nil {
		t.Fatalf("ExecuteGraph failed: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("Expected run completed, got: %s (log: %s)", run.Status, run.Log)
	}

	st, ok := run.NodeStates["preview_node"]
	if !ok || st.Status != "completed" {
		t.Fatalf("preview_node did not complete: %+v", st)
	}
	if st.OutputPreview == "" || !fileExists(st.OutputPreview) {
		t.Errorf("Expected preview_node OutputPreview to exist, got: %s", st.OutputPreview)
	}
}

func TestTC22_UniversalCustomNodes(t *testing.T) {
	ctx, tmpDir := setupTestContext(t)
	defer os.RemoveAll(tmpDir)

	// 1. Custom Media Input Node
	mediaNode := &FlowNode{
		ID:   "c_media",
		Type: "custom_media_input",
		Params: map[string]any{
			"media_title": "Video Nhảy Xu Hướng",
		},
	}
	mediaOut, err := executeCustomMediaInput(ctx, mediaNode, nil)
	if err != nil {
		t.Fatalf("executeCustomMediaInput failed: %v", err)
	}
	if mediaOut["media_out"] == "" || !fileExists(mediaOut["media_out"].(string)) {
		t.Errorf("media_out must be a valid file: %v", mediaOut["media_out"])
	}

	// 2. Custom Universal AI Multi-Modal Node
	aiNode := &FlowNode{
		ID:   "c_ai",
		Type: "custom_universal_ai",
		Params: map[string]any{
			"model":       "gemini_2_5_flash",
			"prompt":      "Tạo video sáng tạo dựa trên: {{text_in}}",
			"output_type": "video",
		},
	}
	aiOut, err := executeCustomUniversalAI(ctx, aiNode, map[string]any{
		"text_in":  "Kịch bản nhảy triệu view TikTok",
		"video_in": mediaOut["media_out"],
	})
	if err != nil {
		t.Fatalf("executeCustomUniversalAI failed: %v", err)
	}
	if aiOut["result_out"] == "" {
		t.Errorf("result_out cannot be empty: %v", aiOut)
	}
	if aiOut["video_out"] == "" || !fileExists(aiOut["video_out"].(string)) {
		t.Errorf("video_out must exist: %v", aiOut["video_out"])
	}

	// 3. Custom Vision Analyzer Node
	visNode := &FlowNode{
		ID:   "c_vis",
		Type: "custom_vision_analyzer",
		Params: map[string]any{
			"analysis_task": "comprehensive_breakdown",
			"model":         "gemini_2_5_pro",
		},
	}
	visOut, err := executeCustomVisionAnalyzer(ctx, visNode, map[string]any{
		"video_in": mediaOut["media_out"],
	})
	if err != nil {
		t.Fatalf("executeCustomVisionAnalyzer failed: %v", err)
	}
	if visOut["analysis_text"] == "" || visOut["prompt_out"] == "" {
		t.Errorf("vision analyzer outputs missing: %v", visOut)
	}

	// 4. Custom Media Processor Node
	procNode := &FlowNode{
		ID:   "c_proc",
		Type: "custom_media_processor",
		Params: map[string]any{
			"operation":     "resize_aspect",
			"aspect_ratio":  "9:16",
			"filter_preset": "cyber_neon",
		},
	}
	procOut, err := executeCustomMediaProcessor(ctx, procNode, map[string]any{
		"media_in": mediaOut["media_out"],
	})
	if err != nil {
		t.Fatalf("executeCustomMediaProcessor failed: %v", err)
	}
	if procOut["media_out"] == "" || !fileExists(procOut["media_out"].(string)) {
		t.Errorf("processed media_out must exist: %v", procOut["media_out"])
	}

	// 5. Custom Script & Logic Node
	scriptNode := &FlowNode{
		ID:   "c_script",
		Type: "custom_script_runner",
		Params: map[string]any{
			"template": "Video {{input_1}} đạt chuẩn chất lượng với độ tương đồng {{input_2}}",
		},
	}
	scriptOut, err := executeCustomScriptRunner(ctx, scriptNode, map[string]any{
		"input_1": "Viral Sexy Dance",
		"input_2": "99.8%",
	})
	if err != nil {
		t.Fatalf("executeCustomScriptRunner failed: %v", err)
	}
	if !strings.Contains(scriptOut["result_text"].(string), "Viral Sexy Dance") {
		t.Errorf("unexpected script output: %v", scriptOut["result_text"])
	}

	// 6. Custom API Webhook Node
	webhookNode := &FlowNode{
		ID:   "c_hook",
		Type: "custom_api_webhook",
		Params: map[string]any{
			"method": "POST",
		},
	}
	hookOut, err := executeCustomAPIWebhook(ctx, webhookNode, map[string]any{
		"message_text": "Thông báo hoàn thành workflow",
	})
	if err != nil {
		t.Fatalf("executeCustomAPIWebhook failed: %v", err)
	}
	if hookOut["status_code"] != 200 {
		t.Errorf("status_code must be 200: %v", hookOut["status_code"])
	}

	// 7. Custom Export & Publish Node
	exportNode := &FlowNode{
		ID:   "c_export",
		Type: "custom_export_publish",
		Params: map[string]any{
			"title":           "Video TikTok Master Xuất Bản",
			"filename_prefix": "tiktok_master",
		},
	}
	exportOut, err := executeCustomExportPublish(ctx, exportNode, map[string]any{
		"media_in": procOut["media_out"],
	})
	if err != nil {
		t.Fatalf("executeCustomExportPublish failed: %v", err)
	}
	if exportOut["final_file"] == "" || !fileExists(exportOut["final_file"].(string)) {
		t.Errorf("final_file must exist: %v", exportOut["final_file"])
	}
}
