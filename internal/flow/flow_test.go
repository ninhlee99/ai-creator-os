package flow

import (
	"context"
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := GlobalRegistry()
	list := reg.List()
	if len(list) < 10 {
		t.Fatalf("kỳ vọng ít nhất 10 node types, nhận %d", len(list))
	}

	node, err := reg.CreateNode("test_node", NodeTikTokTrend, "Custom Label")
	if err != nil {
		t.Fatalf("lỗi tạo node: %v", err)
	}
	if node.Label != "Custom Label" {
		t.Errorf("kỳ vọng label 'Custom Label', nhận '%s'", node.Label)
	}
	if node.Category != "input" {
		t.Errorf("kỳ vọng category 'input', nhận '%s'", node.Category)
	}
}

func TestRecipes(t *testing.T) {
	recipes := ListRecipes()
	if len(recipes) != 4 {
		t.Fatalf("kỳ vọng 4 recipes, nhận %d", len(recipes))
	}

	// 0. Chinese Cinema Drama
	gCinema, err := BuildRecipe(RecipeKeyChineseCinemaDrama, map[string]interface{}{
		"prompt": "Nghịch thiên tru ma lục tập 1",
		"genre":  "tien_hiep",
	})
	if err != nil {
		t.Fatalf("lỗi tạo recipe chinese cinema: %v", err)
	}
	if len(gCinema.Nodes) != 11 {
		t.Errorf("kỳ vọng 11 nodes cho Chinese Cinema Recipe, nhận %d", len(gCinema.Nodes))
	}

	// 1. Dance Trendjack
	gDance, err := BuildRecipe(RecipeKeyDanceTrendjack, map[string]interface{}{
		"trend_url": "https://tiktok.com/@daolephuonghoa/video/123",
	})
	if err != nil {
		t.Fatalf("lỗi tạo recipe dance: %v", err)
	}
	if len(gDance.Nodes) != 10 {
		t.Errorf("kỳ vọng 10 nodes cho Dance Recipe, nhận %d", len(gDance.Nodes))
	}

	// 2. Lookbook
	gLookbook, err := BuildRecipe(RecipeKeyLookbookOutfit, nil)
	if err != nil {
		t.Fatalf("lỗi tạo recipe lookbook: %v", err)
	}
	if len(gLookbook.Nodes) != 9 {
		t.Errorf("kỳ vọng 9 nodes cho Lookbook Recipe, nhận %d", len(gLookbook.Nodes))
	}

	// 3. Audio Story
	gStory, err := BuildRecipe(RecipeKeyAudioStoryCinema, map[string]interface{}{
		"genre": "kinh_di",
		"topic": "Bóng đè lúc canh ba",
	})
	if err != nil {
		t.Fatalf("lỗi tạo recipe story: %v", err)
	}
	if len(gStory.Nodes) != 8 {
		t.Errorf("kỳ vọng 8 nodes cho Story Recipe, nhận %d", len(gStory.Nodes))
	}
}

func TestCompiler(t *testing.T) {
	comp := NewCompiler(nil)
	ctx := context.Background()

	// Prompt 0: Chinese Cinema Drama
	g0, err := comp.Compile(ctx, "Làm một tập phim ngắn vi kịch kiếm hiệp tiên hiệp kiểu Trung Quốc bằng Kling AI", nil)
	if err != nil {
		t.Fatalf("lỗi compile prompt cinema: %v", err)
	}
	if g0.RecipeKey != RecipeKeyChineseCinemaDrama {
		t.Errorf("kỳ vọng RecipeKeyChineseCinemaDrama, nhận %s", g0.RecipeKey)
	}

	// Prompt 1: Dance
	g1, err := comp.Compile(ctx, "Lấy trend nhảy Đào Lê Phương Hoa mới nhất, đổi đồ đầm trắng và ghép mặt kênh", nil)
	if err != nil {
		t.Fatalf("lỗi compile prompt dance: %v", err)
	}
	if g1.RecipeKey != RecipeKeyDanceTrendjack {
		t.Errorf("kỳ vọng RecipeKeyDanceTrendjack, nhận %s", g1.RecipeKey)
	}

	// Prompt 2: Story
	g2, err := comp.Compile(ctx, "Viết truyện kinh dị audio đêm mưa rùng rợn 15 phút giọng VieNeu", nil)
	if err != nil {
		t.Fatalf("lỗi compile prompt story: %v", err)
	}
	if g2.RecipeKey != RecipeKeyAudioStoryCinema {
		t.Errorf("kỳ vọng RecipeKeyAudioStoryCinema, nhận %s", g2.RecipeKey)
	}

	// Prompt 3: Lookbook
	g3, err := comp.Compile(ctx, "Tạo video lookbook phối đồ thời trang mùa hè Shopee cho người mẫu", nil)
	if err != nil {
		t.Fatalf("lỗi compile prompt lookbook: %v", err)
	}
	if g3.RecipeKey != RecipeKeyLookbookOutfit {
		t.Errorf("kỳ vọng RecipeKeyLookbookOutfit, nhận %s", g3.RecipeKey)
	}
}

func TestExecutorCycleDetection(t *testing.T) {
	bridge := NewAdaptersBridge(nil, nil)
	exec := NewExecutor(bridge)
	ctx := context.Background()

	reg := GlobalRegistry()
	n1, _ := reg.CreateNode("n1", NodeTikTokTrend, "Node 1")
	n2, _ := reg.CreateNode("n2", NodeWebMotionTransfer, "Node 2")

	// Cyclic graph: n1 -> n2 -> n1
	g := &Graph{
		ID:     "cycle_test",
		Nodes:  []*Node{n1, n2},
		Status: GraphPending,
		Wires: []Wire{
			{FromNode: "n1", FromPin: "video_url", ToNode: "n2", ToPin: "reference_video"},
			{FromNode: "n2", FromPin: "motion_video", ToNode: "n1", ToPin: "url"},
		},
	}

	err := exec.Execute(ctx, g)
	if err == nil {
		t.Fatalf("kỳ vọng lỗi chu trình nhưng không có")
	}
	if g.Status != GraphFailed {
		t.Errorf("kỳ vọng trạng thái GraphFailed, nhận %s", g.Status)
	}
}

func TestExecutorEndToEnd(t *testing.T) {
	bridge := NewAdaptersBridge(nil, nil)
	exec := NewExecutor(bridge)
	ctx := context.Background()

	// Track events
	eventCount := 0
	exec.AddListener(func(graph *Graph, node *Node) {
		eventCount++
	})

	g, err := BuildRecipe(RecipeKeyDanceTrendjack, map[string]interface{}{
		"trend_url": "https://tiktok.com/@trend/123",
	})
	if err != nil {
		t.Fatalf("lỗi build recipe: %v", err)
	}

	err = exec.Execute(ctx, g)
	if err != nil {
		t.Fatalf("lỗi thực thi graph: %v", err)
	}

	if g.Status != GraphCompleted {
		t.Errorf("kỳ vọng GraphCompleted, nhận %s", g.Status)
	}
	if g.QCScore < 85 {
		t.Errorf("kỳ vọng QCScore >= 85, nhận %d", g.QCScore)
	}
	if g.FinalOutputURL == "" {
		t.Errorf("kỳ vọng có FinalOutputURL")
	}
	if eventCount == 0 {
		t.Errorf("kỳ vọng có event broadcast")
	}

	for _, n := range g.Nodes {
		if n.Status != StatusCompleted && n.Status != StatusFallback {
			t.Errorf("node %s chưa hoàn thành: %s", n.ID, n.Status)
		}
	}
}

func TestExecutorChineseCinemaEndToEnd(t *testing.T) {
	bridge := NewAdaptersBridge(nil, nil)
	exec := NewExecutor(bridge)
	ctx := context.Background()

	g, err := BuildRecipe(RecipeKeyChineseCinemaDrama, map[string]interface{}{
		"prompt": "Quyết chiến Côn Lôn Đỉnh tập 1",
	})
	if err != nil {
		t.Fatalf("lỗi build recipe chinese cinema: %v", err)
	}

	err = exec.Execute(ctx, g)
	if err != nil {
		t.Fatalf("lỗi thực thi graph: %v", err)
	}

	if g.Status != GraphCompleted {
		t.Errorf("kỳ vọng GraphCompleted, nhận %s", g.Status)
	}
	if g.QCScore < 85 {
		t.Errorf("kỳ vọng QCScore >= 85, nhận %d", g.QCScore)
	}
	if g.FinalOutputURL == "" {
		t.Errorf("kỳ vọng có FinalOutputURL")
	}
	for _, n := range g.Nodes {
		if n.Status != StatusCompleted && n.Status != StatusFallback {
			t.Errorf("node %s chưa hoàn thành: %s", n.ID, n.Status)
		}
	}
}
