package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// AIMission records a requested objective and the supported steps the system completed.
type AIMission struct {
	ID            string        `json:"id"`
	Goal          string        `json:"goal"`
	Intent        string        `json:"intent"` // affiliate_campaign, story_campaign, viral_source_replace, trend_campaign, growth_campaign
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

// dispatchAIMission classifies a user goal, runs supported steps, queues jobs,
// and records missing inputs instead of inventing them.
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

	mission, err := s.dispatchAIMission(r.Context(), goal)
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

func (s *Server) executeAffiliateMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 40
	m.CurrentStep = "Chọn sản phẩm có thật và kiểm tra đầu vào"
	goalLower := strings.ToLower(m.Goal)
	fail := func(stepID, title, detail string) {
		m.Steps = append(m.Steps, MissionStep{ID: stepID, Title: title, Status: "failed", Detail: detail, Timestamp: now})
		m.Progress = 40
		m.Status = "failed"
		m.CurrentStep = title
		m.ResultSummary = detail
	}
	if s.Products == nil {
		fail("step-2", "Chọn sản phẩm thật", "Kho sản phẩm chưa sẵn sàng; chưa tạo video.")
		return
	}
	if s.Studio == nil {
		fail("step-2", "Kiểm tra đầu vào Studio", "Studio chưa sẵn sàng; chưa tạo video.")
		return
	}
	if ready, reason := s.Studio.AutoAffiliateProductionReady(); !ready {
		fail("step-2", "Affiliate cần shot plan và footage", reason+"; mission chưa tải ảnh sản phẩm hoặc tạo job.")
		return
	}
	if m.AccountID <= 0 {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Chưa có kênh đích để khóa ảnh nhân vật; chưa tạo video.")
		return
	}

	shelf, err := s.Products.Shelf(100)
	if err != nil {
		fail("step-2", "Chọn sản phẩm thật", "Không đọc được Kệ hàng: "+err.Error())
		return
	}
	var selected *products.Product
	var exactMatches []*products.Product
	var categoryMatches []*products.Product
	for i := range shelf {
		candidate := &shelf[i]
		if strings.TrimSpace(candidate.Title) == "" || len(candidate.ImageURLs) == 0 || strings.TrimSpace(candidate.ImageURLs[0]) == "" {
			continue
		}
		if missionProductMatchesGoal(*candidate, goalLower) {
			categoryMatches = append(categoryMatches, candidate)
		}
		title := strings.ToLower(strings.TrimSpace(candidate.Title))
		if title != "" && strings.Contains(goalLower, title) {
			exactMatches = append(exactMatches, candidate)
		}
	}
	if len(exactMatches) == 1 {
		selected = exactMatches[0]
	} else if len(exactMatches) > 1 {
		fail("step-2", "Chọn sản phẩm cụ thể", "Tên sản phẩm khớp nhiều mục trong Kệ hàng; chỉnh tên hoặc yêu cầu cụ thể hơn.")
		return
	} else if len(categoryMatches) == 1 {
		selected = categoryMatches[0]
	} else if len(categoryMatches) > 1 {
		fail("step-2", "Chọn sản phẩm cụ thể", "Có nhiều sản phẩm cùng khớp ngành hàng; nêu chính xác tên sản phẩm để tránh chọn nhầm.")
		return
	}
	if selected == nil {
		fail("step-2", "Chọn sản phẩm thật", "Không có sản phẩm phù hợp kèm ảnh thật trong Kệ hàng; thêm sản phẩm rồi thử lại.")
		return
	}

	library, err := s.Studio.ModelLibrary()
	if err != nil {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Không đọc được thư viện nhân vật của kênh: "+err.Error())
		return
	}
	modelPhotos, err := library.List(m.AccountID)
	if err != nil {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Không đọc được ảnh nhân vật của kênh: "+err.Error())
		return
	}
	var modelPhoto string
	var channelModelPhotos []string
	for _, photo := range modelPhotos {
		if fileExists(photo.Path) {
			channelModelPhotos = append(channelModelPhotos, photo.Path)
			if modelPhoto == "" {
				modelPhoto = photo.Path
			}
		}
	}
	if modelPhoto == "" {
		fail("step-2", "Chọn kênh và ảnh nhân vật", "Kênh chưa có ảnh nhân vật đã lưu; thêm ảnh đúng kênh rồi thử lại.")
		return
	}

	uploadDir := s.Studio.UploadDir()
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		fail("step-2", "Tải ảnh sản phẩm thật", "Không tạo được thư mục ảnh sản phẩm: "+err.Error())
		return
	}
	productPhotos := make([]string, 0, 4)
	for i, imageURL := range selected.ImageURLs {
		if i >= 4 {
			break
		}
		dst := filepath.Join(uploadDir, fmt.Sprintf("mission-product-%d-%d.jpg", time.Now().UnixNano(), i))
		downloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		downloadErr := studio.DownloadImage(downloadCtx, imageURL, dst)
		cancel()
		if downloadErr == nil && fileExists(dst) {
			productPhotos = append(productPhotos, dst)
		}
	}
	if len(productPhotos) == 0 {
		fail("step-2", "Tải ảnh sản phẩm thật", "Không tải được ảnh listing sản phẩm; chưa tạo video.")
		return
	}

	productName := selected.Title
	niche := selected.Category
	commissionDetail := "hoa hồng chưa có dữ liệu"
	if selected.CommissionRate > 0 {
		commissionDetail = fmt.Sprintf("hoa hồng theo kho %.1f%%", selected.CommissionRate*100)
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Chọn sản phẩm và khóa ảnh tham chiếu", Status: "done",
		Detail: fmt.Sprintf("Sản phẩm trong Kệ hàng: '%s' · %s · ảnh sản phẩm và ảnh nhân vật kênh đã xác nhận.", productName, commissionDetail), Timestamp: now,
	})

	m.Progress = 70
	m.CurrentStep = "Xếp job Studio với sản phẩm và nhân vật đã xác nhận"
	params := studio.AffiliateParams{
		Mode:         studio.AffiliateModeShots,
		AccountID:    m.AccountID,
		ModelPhoto:   modelPhoto,
		ModelPhotos:  channelModelPhotos,
		Niche:        niche,
		ProductName:  productName,
		ProductPhoto: productPhotos[0],
		ExtraPhotos:  productPhotos[1:],
		Seconds:      30,
		BPM:          120,
		AspectRatio:  "9:16",
		AffLink:      selected.ProductURL,
	}
	if selected.Price > 0 {
		params.ProductPrice = fmt.Sprintf("%.0f ₫", selected.Price)
	}
	studioJobID, queueErr := s.Studio.CreateAffiliateJob(params)
	if queueErr != nil {
		detail := "Không xếp được job Studio: " + queueErr.Error()
		fail("step-3", "Xếp job Studio", detail)
		return
	}

	m.Steps = append(m.Steps, MissionStep{
		ID:        "step-3",
		Title:     "Xếp job Affiliate vào Studio",
		Status:    "done",
		Detail:    fmt.Sprintf("Đã xếp job %s; video chưa hoàn tất và chưa đăng.", studioJobID),
		Timestamp: now,
	})

	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/jobs?tab=running"
	m.ResultSummary = fmt.Sprintf("Đã đưa job video cho '%s' vào hàng đợi (job %s). Video chưa hoàn tất; visual QC sẽ chạy trước khi job được đánh dấu xong.", productName, studioJobID)
}

func missionProductMatchesGoal(p products.Product, goal string) bool {
	haystack := strings.ToLower(strings.TrimSpace(p.Title + " " + p.Category))
	if title := strings.ToLower(strings.TrimSpace(p.Title)); title != "" && strings.Contains(goal, title) {
		return true
	}
	categories := [][]string{
		{"mỹ phẩm", "serum", "skincare", "son", "kem dưỡng", "beauty", "cosmetic"},
		{"công nghệ", "tai nghe", "điện thoại", "laptop", "tech", "electronics", "headphone"},
		{"gia dụng", "nồi", "bếp", "máy hút", "home", "kitchen", "appliance"},
		{"thời trang", "áo", "quần", "váy", "cardigan", "túi", "fashion", "apparel"},
	}
	for _, terms := range categories {
		requested := false
		matchesProduct := false
		for _, term := range terms {
			requested = requested || strings.Contains(goal, term)
			matchesProduct = matchesProduct || strings.Contains(haystack, term)
		}
		if requested {
			return matchesProduct
		}
	}
	return false
}

func (s *Server) executeStoryMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.CurrentStep = "Xếp job truyện theo yêu cầu"
	if s.Studio == nil {
		m.Status = "failed"
		m.CurrentStep = "Studio chưa sẵn sàng"
		m.ResultSummary = "Studio chưa sẵn sàng; chưa tạo job truyện."
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: m.CurrentStep, Status: "failed", Detail: m.ResultSummary, Timestamp: now})
		return
	}
	topic := strings.TrimSpace(m.Goal)
	jobID, err := s.Studio.CreateStoryJob(studio.StoryParams{
		Topic: topic, Words: 450, Scenes: 5,
	})
	if err != nil {
		m.Status = "failed"
		m.CurrentStep = "Không xếp được job truyện"
		m.ResultSummary = "Không tạo được job truyện: " + err.Error()
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: m.CurrentStep, Status: "failed", Detail: m.ResultSummary, Timestamp: now})
		return
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Xếp job truyện vào Studio", Status: "done",
		Detail: fmt.Sprintf("Đã xếp job %s theo yêu cầu gốc; truyện/video chưa hoàn tất.", jobID), Timestamp: now,
	})
	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/jobs?tab=running"
	m.ResultSummary = fmt.Sprintf("Đã xếp job truyện %s theo yêu cầu; kết quả sẽ xuất hiện khi job hoàn tất.", jobID)
}

func (s *Server) executeViralSourceReplaceMission(m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Status = "waiting_approval"
	m.Progress = 20
	m.CurrentStep = "Chọn video nguồn và ảnh nhân vật"
	m.ArtifactURL = "/viral"
	m.ResultSummary = "Cần người vận hành chọn video nguồn 9:16, kênh đích và ảnh nhân vật thuộc đúng kênh tại /viral. Chưa tạo job."
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Chờ đầu vào source_replace", Status: "pending",
		Detail: m.ResultSummary, Timestamp: now,
	})
}

func (s *Server) executeTrendMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 60
	m.CurrentStep = "Quét tín hiệu xu hướng TikTok & Douyin"
	fail := func(title, detail string) {
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: title, Status: "failed", Detail: detail, Timestamp: now})
		m.Status = "failed"
		m.CurrentStep = title
		m.ResultSummary = detail
	}
	sounds, err := studio.FetchVNTrending(ctx)
	if err != nil || len(sounds) == 0 {
		if err == nil {
			err = fmt.Errorf("nguồn không trả bài hát")
		}
		fail("Đọc bảng xu hướng hiện tại", "Không lấy được tín hiệu xu hướng trực tiếp: "+err.Error())
		return
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Đọc bảng âm thanh xu hướng hiện tại", Status: "done",
		Detail: fmt.Sprintf("Đã nhận %d bài từ %s.", len(sounds), studio.TrendsSource), Timestamp: now,
	})

	var brand *growth.BrandDNA
	if s.Growth != nil && m.AccountID > 0 {
		brand, _ = s.Growth.GetBrandDNA(m.AccountID)
	}
	signals := make([]growth.TrendSignal, 0, len(sounds))
	for _, sound := range sounds {
		signals = append(signals, growth.TrendSignal{
			Title: sound.Title, Artist: sound.Artist, Movement: sound.Movement,
			Source: studio.TrendsSource, Rank: sound.Rank,
		})
	}
	opps := growth.AnalyzeTrendSignals(ctx, brand, signals)
	var topAngle string
	if len(opps) > 0 && len(opps[0].OriginalAngles) > 0 {
		topAngle = opps[0].OriginalAngles[0]
	}
	detail := fmt.Sprintf("Gợi ý heuristic từ dữ liệu hiện tại: %s", topAngle)
	if topAngle == "" {
		detail = "Đã lưu tín hiệu thực; chưa tạo được góc nội dung từ bộ phân tích hiện tại."
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-3", Title: "Tạo gợi ý nội dung từ tín hiệu thật", Status: "done",
		Detail: detail, Timestamp: now,
	})
	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/studio/trends"
	m.ResultSummary = fmt.Sprintf("Đã đọc %d tín hiệu hiện tại từ %s. Gợi ý nội dung mang tính heuristic, không phải dự báo lượt xem.", len(sounds), studio.TrendsSource)
}

func (s *Server) executeGrowthMission(ctx context.Context, m *AIMission) {
	now := time.Now().Format("15:04:05")
	m.Progress = 60
	m.CurrentStep = "Tạo lịch nội dung theo cấu hình kênh"
	fail := func(title, detail string) {
		m.Steps = append(m.Steps, MissionStep{ID: "step-2", Title: title, Status: "failed", Detail: detail, Timestamp: now})
		m.Status = "failed"
		m.CurrentStep = title
		m.ResultSummary = detail
	}
	if s.Mgr == nil || m.AccountID <= 0 {
		fail("Chọn kênh đích", "Không có kênh để lập kế hoạch.")
		return
	}
	if s.Growth == nil {
		fail("Kiểm tra kho tăng trưởng", "Kho tăng trưởng chưa sẵn sàng; chưa lập kế hoạch.")
		return
	}
	acct, err := s.Mgr.Get(m.AccountID)
	if err != nil || acct == nil {
		fail("Đọc cấu hình kênh", "Không đọc được kênh đích.")
		return
	}
	stats, statsErr := s.Growth.ListFormatStats(m.AccountID)
	if statsErr != nil {
		fail("Đọc thống kê format", "Không đọc được dữ liệu format: "+statsErr.Error())
		return
	}
	killed := 0
	for _, stat := range stats {
		if stat.Verdict == growth.VerdictKilled {
			killed++
		}
	}
	count, err := s.generatePlanFor(acct, "Lịch nội dung theo ngách và giai đoạn kênh")
	if err != nil {
		fail("Tạo lịch nội dung", "Không lập được lịch: "+err.Error())
		return
	}
	snapshot, snapshotErr := s.Growth.LatestSnapshot(m.AccountID)
	snapshotDetail := "Chưa có snapshot chỉ số kênh."
	if snapshotErr == nil && snapshot != nil {
		snapshotDetail = "Có snapshot chỉ số kênh; lịch vẫn dựa trên ngách, giai đoạn và format đã đánh dấu dừng."
	}
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-2", Title: "Đọc cấu hình tăng trưởng hiện có", Status: "done",
		Detail: fmt.Sprintf("%d format đã đánh dấu dừng. %s Không dự báo mức tăng view/retention.", killed, snapshotDetail), Timestamp: now,
	})
	m.Steps = append(m.Steps, MissionStep{
		ID: "step-3", Title: "Tạo kế hoạch nội dung 30 ngày", Status: "done",
		Detail: fmt.Sprintf("Đã tạo %d vị trí theo ngách và giai đoạn kênh; loại %d format đã đánh dấu dừng.", count, killed), Timestamp: now,
	})
	m.Progress = 100
	m.Status = "completed"
	m.ArtifactURL = "/growth"
	m.ResultSummary = fmt.Sprintf("Đã tạo lịch 30 ngày cho @%s từ cấu hình kênh và format đã đánh dấu dừng. Không khẳng định mức tăng hiệu quả.", acct.Username)
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
