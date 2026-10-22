package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AIMission records a requested objective and the supported steps the system completed.
type AIMission struct {
	ID                      string        `json:"id"`
	Goal                    string        `json:"goal"`
	Intent                  string        `json:"intent"` // affiliate_campaign, story_campaign, viral_source_replace, trend_campaign, growth_campaign
	TargetAccount           string        `json:"target_account"`
	AccountID               int64         `json:"account_id"`
	Status                  string        `json:"status"` // running, completed, waiting_approval, failed
	Progress                int           `json:"progress"`
	CurrentStep             string        `json:"current_step"`
	Steps                   []MissionStep `json:"steps"`
	ResultSummary           string        `json:"result_summary"`
	ArtifactURL             string        `json:"artifact_url,omitempty"`
	CreatedAt               string        `json:"created_at"`
	FinishedAt              string        `json:"finished_at,omitempty"`
	FlowCreditsAcknowledged bool          `json:"-"`
}

// MissionStep is one discrete operational milestone inside a mission.
type MissionStep struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"` // pending, running, done, failed, skipped
	Detail    string `json:"detail"`
	Timestamp string `json:"timestamp"`
}

func (s *Server) ensureMissionTable() error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_missions (
			id TEXT PRIMARY KEY,
			goal TEXT NOT NULL,
			intent TEXT NOT NULL,
			target_account TEXT NOT NULL DEFAULT '',
			account_id INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'running',
			progress INTEGER NOT NULL DEFAULT 0,
			current_step TEXT NOT NULL DEFAULT '',
			steps_json TEXT NOT NULL DEFAULT '[]',
			result_summary TEXT NOT NULL DEFAULT '',
			artifact_url TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_ai_missions_created ON ai_missions(created_at);
	`)
	return err
}

// listAIMissions retrieves recent missions from SQLite.
func (s *Server) listAIMissions(limit int) []AIMission {
	_ = s.ensureMissionTable()
	if s.db == nil {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT id, goal, intent, target_account, account_id, status, progress, current_step,
		       steps_json, result_summary, artifact_url, created_at, finished_at
		FROM ai_missions
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []AIMission
	for rows.Next() {
		var m AIMission
		var stepsJSON string
		if err := rows.Scan(&m.ID, &m.Goal, &m.Intent, &m.TargetAccount, &m.AccountID,
			&m.Status, &m.Progress, &m.CurrentStep, &stepsJSON, &m.ResultSummary,
			&m.ArtifactURL, &m.CreatedAt, &m.FinishedAt); err != nil {
			continue
		}
		_ = json.Unmarshal([]byte(stepsJSON), &m.Steps)
		out = append(out, m)
	}
	return out
}

// dispatchAIMission classifies a user goal, runs supported steps, queues jobs,
// and records missing inputs instead of inventing them.
func (s *Server) dispatchAIMission(ctx context.Context, goal string) (*AIMission, error) {
	return s.dispatchAIMissionWithFlowConsent(ctx, goal, false)
}

func (s *Server) dispatchAIMissionWithFlowConsent(ctx context.Context, goal string, flowCreditsAcknowledged bool) (*AIMission, error) {
	_ = s.ensureMissionTable()
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return nil, fmt.Errorf("mục tiêu không được để trống")
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	missionID := fmt.Sprintf("mission-%d", time.Now().UnixNano())
	goalLower := strings.ToLower(goal)

	// Classify intent & identify target account
	intent := "unclassified"
	defaultTitle := "Yêu cầu chưa phân loại"
	switch {
	case strings.Contains(goalLower, "kể chuyện") || strings.Contains(goalLower, "lịch sử") || strings.Contains(goalLower, "truyện") || strings.Contains(goalLower, "tâm sự"):
		intent = "story_campaign"
		defaultTitle = "Chiến Dịch Kể Chuyện Ngôi Thứ Nhất"
	case strings.Contains(goalLower, "video viral") || strings.Contains(goalLower, "viral video") || strings.Contains(goalLower, "thay nhân vật"):
		intent = "viral_source_replace"
		defaultTitle = "Thay nhận diện trên video nguồn"
	case strings.Contains(goalLower, "xu hướng") || strings.Contains(goalLower, "trend") || strings.Contains(goalLower, "douyin") || strings.Contains(goalLower, "bắt trend"):
		intent = "trend_campaign"
		defaultTitle = "Chiến Dịch Tình Báo Xu Hướng TikTok"
	case strings.Contains(goalLower, "30 ngày") || strings.Contains(goalLower, "kế hoạch") || strings.Contains(goalLower, "tăng trưởng") || strings.Contains(goalLower, "replan"):
		intent = "growth_campaign"
		defaultTitle = "Chiến Dịch Tái Lập Ma Trận Tăng Trưởng 30 Ngày"
	case strings.Contains(goalLower, "affiliate") || strings.Contains(goalLower, "sản phẩm") || strings.Contains(goalLower, "quảng cáo") || strings.Contains(goalLower, "bán hàng"):
		intent = "affiliate_campaign"
		defaultTitle = "Chiến Dịch Sản Phẩm & Affiliate"
	}

	// Find best target account if available
	var targetAccountName string
	var targetAccountID int64
	if s.Mgr != nil {
		accts, _ := s.Mgr.List()
		matchedLength := 0
		for _, a := range accts {
			username := strings.ToLower(strings.TrimSpace(a.Username))
			if username != "" && strings.Contains(goalLower, username) && len(username) > matchedLength {
				targetAccountName = a.Username
				targetAccountID = a.ID
				matchedLength = len(username)
			}
		}
		if targetAccountName == "" && len(accts) == 1 {
			targetAccountName = accts[0].Username
			targetAccountID = accts[0].ID
		}
	}

	mission := &AIMission{
		ID:                      missionID,
		Goal:                    goal,
		Intent:                  intent,
		TargetAccount:           targetAccountName,
		AccountID:               targetAccountID,
		Status:                  "running",
		Progress:                20,
		CurrentStep:             "Khởi tạo & Phân tích yêu cầu chiến lược",
		CreatedAt:               now,
		FlowCreditsAcknowledged: flowCreditsAcknowledged,
	}

	// Setup pipeline steps
	step1 := MissionStep{
		ID:        "step-1",
		Title:     "Phân loại yêu cầu theo từ khóa và cấu hình kênh",
		Status:    "done",
		Detail:    fmt.Sprintf("Nhóm yêu cầu: %s · Kênh khớp trực tiếp: %s", defaultTitle, targetAccountName),
		Timestamp: now,
	}
	mission.Steps = append(mission.Steps, step1)

	// Execute intent-specific actions
	switch intent {
	case "affiliate_campaign":
		s.executeAffiliateMission(ctx, mission)
	case "story_campaign":
		s.executeStoryMission(ctx, mission)
	case "trend_campaign":
		s.executeTrendMission(ctx, mission)
	case "viral_source_replace":
		s.executeViralSourceReplaceMission(mission)
	case "growth_campaign":
		s.executeGrowthMission(ctx, mission)
	default:
		mission.Status = "waiting_approval"
		mission.Progress = 20
		mission.CurrentStep = "Chọn loại mục tiêu"
		mission.Steps = append(mission.Steps, MissionStep{
			ID: "step-2", Title: "Xác định loại công việc", Status: "pending",
			Detail: "Nêu rõ mục tiêu: Affiliate, truyện, xu hướng hoặc kế hoạch tăng trưởng.", Timestamp: now,
		})
		mission.ResultSummary = "Chưa phân loại được mục tiêu; chưa tạo job."
	}

	mission.FinishedAt = time.Now().Format("2006-01-02 15:04:05")

	// Save to database
	if s.db != nil {
		stepsJSON, _ := json.Marshal(mission.Steps)
		_, _ = s.db.Exec(`
			INSERT OR REPLACE INTO ai_missions (
				id, goal, intent, target_account, account_id, status, progress, current_step,
				steps_json, result_summary, artifact_url, created_at, finished_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, mission.ID, mission.Goal, mission.Intent, mission.TargetAccount, mission.AccountID,
			mission.Status, mission.Progress, mission.CurrentStep, string(stepsJSON),
			mission.ResultSummary, mission.ArtifactURL, mission.CreatedAt, mission.FinishedAt)
	}

	// Record in audit ledger
	if s.Ledger != nil {
		_ = s.Ledger.Decide("AI Mission Control", "dispatch_mission", &mission.ID, goal, map[string]any{
			"intent":         mission.Intent,
			"target_account": mission.TargetAccount,
			"result":         mission.ResultSummary,
		})
	}

	return mission, nil
}

// handleTeamDispatchGoal accepts a natural-language mission from callers and
// forwards it to the active mission engine. Agent-management routes are retired.
func (s *Server) handleTeamDispatchGoal(w http.ResponseWriter, r *http.Request) {
	goal := strings.TrimSpace(r.FormValue("goal"))
	if goal == "" {
		seeOther(w, r, "/studio")
		return
	}

	mission, err := s.dispatchAIMissionWithFlowConsent(r.Context(), goal, r.FormValue("confirm_flow_credits") == "1")
	if err != nil {
		seeOther(w, r, "/?err="+url.QueryEscape("Không thể thực thi mục tiêu: "+err.Error()))
		return
	}
	if mission.Intent == "story_campaign" {
		statusKey := "ok"
		if mission.Status == "failed" {
			statusKey = "err"
		}
		seeOther(w, r, "/stories?"+statusKey+"="+url.QueryEscape(mission.ResultSummary))
		return
	}
	if mission.Intent == "viral_source_replace" {
		seeOther(w, r, "/viral?notice="+url.QueryEscape(mission.ResultSummary))
		return
	}

	msg := "Đã tạo chiến dịch từ yêu cầu: " + mission.ResultSummary
	if r.FormValue("back") == "/" {
		seeOther(w, r, "/?ok="+url.QueryEscape(msg))
		return
	}
	seeOther(w, r, "/studio?msg="+url.QueryEscape(msg))
}
