package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/flow"
)

func TestFlowPageRenders(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/flow")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /flow = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Studio Sáng Tạo 3-Trong-1",
		"Affiliate Thời Trang Nữ Dễ Thương",
		"Trending Vũ Đạo Sexy Dance",
		"Truyện Audio Tình Yêu",
		"Mẫu Nhân Vật Của Bạn",
		"100% Miễn Phí (0đ Chi Phí)",
		"Google Flow & Muse AI",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trang /flow thiếu nội dung %q", want)
		}
	}
}

func TestFlowRecipesAPI(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/api/flow/recipes")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/flow/recipes = %d, want 200", rec.Code)
	}
	var res struct {
		OK      bool                     `json:"ok"`
		Recipes []flow.RecipeCatalogItem `json:"recipes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !res.OK || len(res.Recipes) < 4 {
		t.Fatalf("kỳ vọng ok=true và >=4 recipes, nhận: %+v", res)
	}
}

func TestFlowCompileAndRunAPI(t *testing.T) {
	s := newTestServer(t)

	// 1. Compile Recipe
	compilePayload := map[string]interface{}{
		"recipe_key": flow.RecipeKeyDanceTrendjack,
		"params": map[string]interface{}{
			"trend_url": "https://tiktok.com/@trend/123",
		},
	}
	bodyBytes, _ := json.Marshal(compilePayload)
	req := httptest.NewRequest("POST", "/api/flow/compile", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/flow/compile = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var compileRes struct {
		OK    bool        `json:"ok"`
		Graph *flow.Graph `json:"graph"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&compileRes); err != nil {
		t.Fatalf("decode compile json: %v", err)
	}
	if !compileRes.OK || compileRes.Graph == nil {
		t.Fatalf("biên dịch thất bại: %+v", compileRes)
	}

	flowID := compileRes.Graph.ID

	// 2. Query Status
	reqStatus := httptest.NewRequest("GET", "/api/flow/status?id="+flowID, nil)
	recStatus := httptest.NewRecorder()
	s.Routes().ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("GET /api/flow/status = %d, want 200", recStatus.Code)
	}

	// 3. Run Flow
	runPayload := map[string]interface{}{
		"id": flowID,
	}
	runBytes, _ := json.Marshal(runPayload)
	reqRun := httptest.NewRequest("POST", "/api/flow/run", bytes.NewReader(runBytes))
	reqRun.Header.Set("Content-Type", "application/json")
	recRun := httptest.NewRecorder()
	s.Routes().ServeHTTP(recRun, reqRun)

	if recRun.Code != http.StatusOK {
		t.Fatalf("POST /api/flow/run = %d, want 200: %s", recRun.Code, recRun.Body.String())
	}

	var runRes struct {
		OK    bool        `json:"ok"`
		Graph *flow.Graph `json:"graph"`
	}
	if err := json.NewDecoder(recRun.Body).Decode(&runRes); err != nil {
		t.Fatalf("decode run json: %v", err)
	}
	if !runRes.OK || runRes.Graph.Status != flow.GraphCompleted {
		t.Fatalf("chạy flow thất bại: %+v", runRes)
	}
	if runRes.Graph.QCScore < 85 {
		t.Errorf("kỳ vọng QCScore >= 85, nhận %d", runRes.Graph.QCScore)
	}
}

func TestFlowCompileNaturalPrompt(t *testing.T) {
	s := newTestServer(t)

	compilePayload := map[string]interface{}{
		"prompt": "Viết truyện kinh dị đêm mưa rùng rợn giọng VieNeu",
	}
	bodyBytes, _ := json.Marshal(compilePayload)
	req := httptest.NewRequest("POST", "/api/flow/compile", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/flow/compile = %d, want 200", rec.Code)
	}

	var res struct {
		OK    bool        `json:"ok"`
		Graph *flow.Graph `json:"graph"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !res.OK || res.Graph.RecipeKey != flow.RecipeKeyAudioStoryCinema {
		t.Fatalf("kỳ vọng RecipeKeyAudioStoryCinema, nhận: %+v", res.Graph)
	}
}
