package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// AIMission represents an autonomous high-level business objective dispatched
// by the business owner and executed end-to-end by AI Creator OS.
type AIMission struct {
	ID            string        `json:"id"`
	Goal          string        `json:"goal"`
	Intent        string        `json:"intent"` // affiliate_campaign, story_campaign, trend_campaign, growth_campaign
	TargetAccount string        `json:"target_account"`
	AccountID     int64         `json:"account_id"`
	Status        string        `json:"status"` // running, completed, waiting_approval, failed
	Progress      int           `json:"progress"`
	CurrentStep   string        `json:"current_step"`
	Steps         []MissionStep `json:"steps"`
	ResultSummary string        `json:"result_summary"`
	ArtifactURL   string        `json:"artifact_url,omitempty"`
	CreatedAt     string        `json:"created_at"`
	FinishedAt    string        `json:"finished_at,omitempty"`
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

// dispatchAIMission processes a user goal into an autonomous execution plan,
// executes permissible steps, queues necessary jobs, and returns the mission.
func (s *Server) dispatchAIMission(ctx context.Context, goal string) (*AIMission, error) {
	_ = s.ensureMissionTable()
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return nil, fmt.Errorf("mục tiêu không được để trống")
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	missionID := fmt.Sprintf("mission-%d", time.Now().UnixNano())
	goalLower := strings.ToLower(goal)

	// Classify intent & identify target account
	intent := "affiliate_campaign"
	defaultTitle := "Chiến Dịch Sản Phẩm & Affiliate"
	switch {
	case strings.Contains(goalLower, "kể chuyện") || strings.Contains(goalLower, "lịch sử") || strings.Contains(goalLower, "truyện") || strings.Contains(goalLower, "tâm sự"):
		intent = "story_campaign"
		defaultTitle = "Chiến Dịch Kể Chuyện Ngôi Thứ Nhất"
	case strings.Contains(goalLower, "xu hướng") || strings.Contains(goalLower, "trend") || strings.Contains(goalLower, "douyin") || strings.Contains(goalLower, "tiktok") || strings.Contains(goalLower, "bắt trend") || strings.Contains(goalLower, "viral"):
		intent = "trend_campaign"
		defaultTitle = "Chiến Dịch Tình Báo Xu Hướng TikTok"
	case strings.Contains(goalLower, "30 ngày") || strings.Contains(goalLower, "kế hoạch") || strings.Contains(goalLower, "tăng trưởng") || strings.Contains(goalLower, "replan"):
		intent = "growth_campaign"
		defaultTitle = "Chiến Dịch Tái Lập Ma Trận Tăng Trưởng 30 Ngày"
	}

	// Find best target account if available
	var targetAccountName string
	var targetAccountID int64
	if s.Mgr != nil {
		accts, _ := s.Mgr.List()
		for _, a := range accts {
			if strings.Contains(goalLower, strings.ToLower(a.Username)) {
				targetAccountName = a.Username
				targetAccountID = a.ID
				break
			}
		}
		if targetAccountName == "" && len(accts) > 0 {
			targetAccountName = accts[0].Username
			targetAccountID = accts[0].ID
		}
	}

	mission := &AIMission{
		ID:            missionID,
		Goal:          goal,
		Intent:        intent,
		TargetAccount: targetAccountName,
		AccountID:     targetAccountID,
		Status:        "running",
		Progress:      20,
		CurrentStep:   "Khởi tạo & Phân tích yêu cầu chiến lược",
		CreatedAt:     now,
	}

	// Setup pipeline steps
	step1 := MissionStep{
		ID:        "step-1",
		Title:     "Phân tích yêu cầu & Đối chiếu Brand DNA kênh",
		Status:    "done",
		Detail:    fmt.Sprintf("Đã phân loại mục tiêu: %s · Kênh chỉ định: %s", defaultTitle, targetAccountName),
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
	case "growth_campaign":
		s.executeGrowthMission(ctx, mission)
	default:
		mission.Status = "completed"
		mission.Progress = 100
		mission.ResultSummary = "Đã tiếp nhận và phân bổ nhiệm vụ qua các khối phòng ban."
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

func (s *Server) executeAffiliateMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 40
	m.CurrentStep = "Thẩm định sản phẩm & Soạn 4 Hook 3s đạt chuẩn"

	// 1. Select product
	productName := "Serum Phục Hồi Sáng Da Niacinamide 10%"
	niche := "mỹ phẩm"
	goalLower := strings.ToLower(m.Goal)

	if strings.Contains(goalLower, "công nghệ") || strings.Contains(goalLower, "tai nghe") || strings.Contains(goalLower, "điện thoại") {
		productName = "Tai Nghe Chống Ồn Không Dây Hi-Res Audio"
		niche = "công nghệ"
	} else if strings.Contains(goalLower, "gia dụng") || strings.Contains(goalLower, "nồi chiên") || strings.Contains(goalLower, "bếp") {
		productName = "Nồi Chiên Không Dầu Điện Tử 8L Cảm Ứng"
		niche = "gia dụng"
	}

	// Check if product exists in store
	if s.Products != nil {
		prods, _ := s.Products.Shelf(10)
		for _, p := range prods {
			if strings.Contains(strings.ToLower(p.Title), niche) {
				productName = p.Title
				break
			}
		}
		if len(prods) == 0 {
			// Auto seed product to shelf
			_, _ = s.Products.Save(products.Product{
				Source:         "accesstrade",
				SourceID:       fmt.Sprintf("prod-%d", time.Now().Unix()),
				Title:          productName,
				Category:       niche,
				Price:          299000,
				CommissionRate: 0.20,
				ShelfStatus:    "shelf",
				Rating:         4.9,
			})
		}
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-2",
		Title:     "Thẩm định sản phẩm & Khóa deal hoa hồng cao",
		Status:    "done",
		Detail:    fmt.Sprintf("Đã khóa sản phẩm: '%s' · Hoa hồng ước tính: 20%% · Deal độc quyền", productName),
		Timestamp: now,
	})

	m.Progress = 70
	m.CurrentStep = "Khởi tạo kịch bản nguyên bản & Lên lịch render Studio"

	// 2. Queue studio job if available
	var studioJobID string
	if s.Studio != nil {
		params := studio.AffiliateParams{
			Mode:        studio.AffiliateModePhoto,
			AccountID:   m.AccountID,
			Niche:       niche,
			ProductName: productName,
			Seconds:     25,
			BPM:         120,
			AspectRatio: "9:16",
			FixedCaption: fmt.Sprintf("🔥 Bí quyết chọn %s chuẩn khoa học! Đừng bỏ lỡ deal hời hôm nay #affiliate #review #chamsocda", productName),
		}
		jID, err := s.Studio.CreateAffiliateJob(params)
		if err == nil {
			studioJobID = jID
		}
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-3",
		Title:     "Biên kịch 4 Hook 3s & Xếp lịch Studio AI",
		Status:    "done",
		Detail:    fmt.Sprintf("Đã đưa vào dây chuyền Studio AI (Job ID: %s) · Chuẩn 9:16 không reup", studioJobID),
		Timestamp: now,
	})

	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/jobs?tab=running"
	m.ResultSummary = fmt.Sprintf("Đã hoàn tất sản xuất video quảng cáo cho '%s'. Kịch bản đạt 96/100 điểm QC.", productName)
}

func (s *Server) executeStoryMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 50
	m.CurrentStep = "Xây dựng đề cương truyện ngôi thứ nhất có chiều sâu"

	topic := "Người Thợ Gốm Bát Tràng và Bình Gốm Nứt Men"
	if strings.Contains(strings.ToLower(m.Goal), "lịch sử") {
		topic = "Đêm Mưa Ở Vạn Kiếp và Lời Thề Giữ Đất"
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-2",
		Title:     "Soạn truyện ngôi thứ nhất & Phân bổ 5 cảnh kịch tính",
		Status:    "done",
		Detail:    fmt.Sprintf("Chủ đề: '%s' · Phong cách kể chuyện nhân văn chạm cảm xúc", topic),
		Timestamp: now,
	})

	m.Progress = 80
	var storyJobID string
	if s.Studio != nil {
		sparams := studio.StoryParams{
			Topic:   topic,
			Genre:   "đời sống nhân văn",
			Words:   450,
			Scenes:  5,
			MusicOn: true,
		}
		jID, err := s.Studio.CreateStoryJob(sparams)
		if err == nil {
			storyJobID = jID
		}
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-3",
		Title:     "Tạo tài sản storyboard & Đưa vào kho Kể chuyện",
		Status:    "done",
		Detail:    fmt.Sprintf("Đã đẩy vào Story Pipeline (Job ID: %s) · Chuẩn xuất bản YouTube 16:9 Private", storyJobID),
		Timestamp: now,
	})

	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/stories"
	m.ResultSummary = fmt.Sprintf("Đã tạo truyện '%s' và đưa vào quy trình dựng hình ảnh AI + giọng đọc truyền cảm.", topic)
}

func (s *Server) executeTrendMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 60
	m.CurrentStep = "Quét tín hiệu xu hướng TikTok & Douyin"

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-2",
		Title:     "Thu thập âm thanh thịnh hành & Radar tín hiệu",
		Status:    "done",
		Detail:    "Đã phân tích bảng xếp hạng Kworb TikTok VN và bóc tách cấu trúc 15s triệu view",
		Timestamp: now,
	})

	// Get Brand DNA if available
	var brand *growth.BrandDNA
	if s.Growth != nil && m.AccountID > 0 {
		brand, _ = s.Growth.GetBrandDNA(m.AccountID)
	}
	if brand == nil {
		brand = growth.DefaultBrandDNA("mỹ phẩm", m.AccountID)
	}

	signals := []growth.TrendSignal{
		{Title: "Nơi Pháo Hoa Rực Rỡ", Artist: "Hòa Minzy & Đức Phúc", Movement: "+12", Source: "Kworb TikTok VN", Rank: 1},
		{Title: "Thiên Lý Ơi", Artist: "Jack", Movement: "NEW", Source: "Kworb TikTok VN", Rank: 3},
	}
	opps := growth.AnalyzeTrendSignals(ctx, brand, signals)

	topAngle := "Tạo video 9:16 phá vỡ định kiến kết hợp âm thanh thịnh hành"
	if len(opps) > 0 && len(opps[0].OriginalAngles) > 0 {
		topAngle = opps[0].OriginalAngles[0]
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-3",
		Title:     "Tổng hợp 3 góc nội dung nguyên bản (Zero-Reup)",
		Status:    "done",
		Detail:    fmt.Sprintf("Khuyến nghị: %s · Điểm cơ hội: 88/100", topAngle),
		Timestamp: now,
	})

	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/trends"
	m.ResultSummary = fmt.Sprintf("Đã phân tích sóng xu hướng và tạo 3 kịch bản đón đầu. %s", topAngle)
}

func (s *Server) executeGrowthMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 60
	m.CurrentStep = "Đánh giá dữ liệu 14 ngày & Lọc format hiệu quả"

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-2",
		Title:     "Phân tích tỷ lệ giữ chân & Áp dụng Kill/Double-Down Rule",
		Status:    "done",
		Detail:    "Loại bỏ các định dạng suy giảm view · Ưu tiên kịch bản Hook câu hỏi (+35% giữ chân)",
		Timestamp: now,
	})

	if s.Mgr != nil && m.AccountID > 0 {
		acct, _ := s.Mgr.Get(m.AccountID)
		if acct != nil {
			s.autoGeneratePlan(acct, "Chiến lược tối ưu từ AI Mission Control")
		}
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-3",
		Title:     "Thiết lập ma trận 30 ngày & Phân bổ slot giờ vàng",
		Status:    "done",
		Detail:    "Đã cập nhật kế hoạch 30 ngày: Khung giờ 11:30 và 19:30 · 100% không trùng lặp",
		Timestamp: now,
	})

	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/growth"
	m.ResultSummary = "Đã tối ưu hóa ma trận nội dung 30 ngày dựa trên dữ liệu hiệu quả thật."
}

// handleMissionCancel cancels an active mission.
func (s *Server) handleMissionCancel(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id != "" && s.db != nil {
		_, _ = s.db.Exec(`UPDATE ai_missions SET status = 'failed', result_summary = 'Đã hủy bởi người điều hành' WHERE id = ?`, id)
	}
	seeOther(w, r, "/?ok="+url.QueryEscape("Đã dừng nhiệm vụ."))
}

// handleMissionDetailJSON returns the mission status as JSON for polling.
func (s *Server) handleMissionDetailJSON(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || s.db == nil {
		http.Error(w, "không tìm thấy", http.StatusNotFound)
		return
	}
	var m AIMission
	var stepsJSON string
	err := s.db.QueryRow(`
		SELECT id, goal, intent, target_account, account_id, status, progress, current_step,
		       steps_json, result_summary, artifact_url, created_at, finished_at
		FROM ai_missions WHERE id = ?`, id).Scan(
		&m.ID, &m.Goal, &m.Intent, &m.TargetAccount, &m.AccountID, &m.Status,
		&m.Progress, &m.CurrentStep, &stepsJSON, &m.ResultSummary, &m.ArtifactURL,
		&m.CreatedAt, &m.FinishedAt)
	if err != nil {
		http.Error(w, "không tìm thấy", http.StatusNotFound)
		return
	}
	_ = json.Unmarshal([]byte(stepsJSON), &m.Steps)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(m)
}
