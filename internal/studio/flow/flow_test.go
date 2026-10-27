package flow

import (
	"context"
	"database/sql"
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
		FFmpegRunner: nil, // Chế độ test fallback nội bộ
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
		t.Fatalf("TC-09: Thực thi PresetViralDance thất bại: %v (Log: %s)", err, run.Log)
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

	// 1. Kiểm tra seed presets
	graphs, err := store.ListGraphs()
	if err != nil {
		t.Fatalf("ListGraphs lỗi: %v", err)
	}
	if len(graphs) < 3 {
		t.Errorf("Kỳ vọng ít nhất 3 preset được seed, có: %d", len(graphs))
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
