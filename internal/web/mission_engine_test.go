package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMissionEngineDispatchAndExecution(t *testing.T) {
	srv := newTestServer(t)

	// 1. Dispatch an affiliate product mission
	goalAffiliate := "Săn deal gia dụng và tạo video bán hàng cho nồi chiên không dầu"
	m1, err := srv.dispatchAIMission(context.Background(), goalAffiliate)
	if err != nil {
		t.Fatalf("failed to dispatch affiliate mission: %v", err)
	}
	if m1.Intent != "affiliate_campaign" {
		t.Errorf("expected intent affiliate_campaign, got %s", m1.Intent)
	}
	if m1.Status != "completed" {
		t.Errorf("expected status completed, got %s", m1.Status)
	}
	if m1.Progress != 100 {
		t.Errorf("expected progress 100, got %d", m1.Progress)
	}
	if len(m1.Steps) < 3 {
		t.Errorf("expected at least 3 execution steps, got %d", len(m1.Steps))
	}
	if m1.ResultSummary == "" {
		t.Error("expected non-empty result summary")
	}

	// 2. Dispatch a storytelling mission
	goalStory := "Tạo video kể chuyện lịch sử Việt Nam bí ẩn và sâu lắng"
	m2, err := srv.dispatchAIMission(context.Background(), goalStory)
	if err != nil {
		t.Fatalf("failed to dispatch story mission: %v", err)
	}
	if m2.Intent != "story_campaign" {
		t.Errorf("expected intent story_campaign, got %s", m2.Intent)
	}
	if m2.Status != "completed" {
		t.Errorf("expected status completed, got %s", m2.Status)
	}
	if m2.ArtifactURL != "/stories" {
		t.Errorf("expected artifact url /stories, got %s", m2.ArtifactURL)
	}

	// 3. Dispatch a trend intelligence mission
	goalTrend := "Tự động quét xu hướng Douyin và âm thanh TikTok hot"
	m3, err := srv.dispatchAIMission(context.Background(), goalTrend)
	if err != nil {
		t.Fatalf("failed to dispatch trend mission: %v", err)
	}
	if m3.Intent != "trend_campaign" {
		t.Errorf("expected intent trend_campaign, got %s", m3.Intent)
	}
	if len(m3.Steps) < 3 {
		t.Errorf("expected at least 3 steps in trend mission, got %d", len(m3.Steps))
	}

	// 4. Verify listAIMissions retrieves recent missions
	missions := srv.listAIMissions(10)
	if len(missions) < 3 {
		t.Fatalf("expected at least 3 stored missions, got %d", len(missions))
	}

	foundAffiliate := false
	for _, m := range missions {
		if m.ID == m1.ID {
			foundAffiliate = true
			if len(m.Steps) != len(m1.Steps) {
				t.Errorf("step count mismatch in retrieved mission")
			}
			break
		}
	}
	if !foundAffiliate {
		t.Error("newly dispatched affiliate mission not found in listAIMissions")
	}

	// 5. Verify HTTP dispatch handler
	form := url.Values{
		"goal": {"Tạo video review serum dưỡng trắng mờ nám 7 ngày"},
		"back": {"/"},
	}
	req := httptest.NewRequest("POST", "/team/dispatch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.handleTeamDispatchGoal(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect 303, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "ok=") {
		t.Errorf("expected redirect location to contain ok message, got %s", loc)
	}
}
