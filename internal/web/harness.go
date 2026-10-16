package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/harness"
)

// harnessCycleView hiển thị chi tiết một chu kỳ OODA.
type harnessCycleView struct {
	CycleID      string
	StartedAt    string
	FinishedAt   string
	Duration     string
	Diagnosis    string
	ActionCount  int
	Status       string
	ChannelCount int
	ProductCount int
	QueueCount   int
	Decisions    []harness.OODADecisionTask
}

// harnessMemoryView hiển thị một bài học/kinh nghiệm thực chiến.
type harnessMemoryView struct {
	ID         string
	Domain     string
	DomainVN   string
	Channel    string
	Subject    string
	Insight    string
	Confidence string
	ScoreColor string
	IsWarning  bool
	CreatedAt  string
}

// harnessView là view model cho trang buồng lái Harness Agent (/harness).
type harnessView struct {
	HarnessOK          bool
	AutonomyLevel      int
	AutonomyLevelName  string
	AutonomyLevelBadge string
	AutonomyDesc       string
	LatestCycle        *harnessCycleView
	RecentCycles       []*harnessCycleView
	Memories           []*harnessMemoryView
	TotalMemories      int
	TotalCycles        int
	Notice             string
	Error              string
}

// handleHarness render dashboard điều phối nhận thức OODA của Harness Agent.
func (s *Server) handleHarness(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	notice := q.Get("notice")
	errMsg := q.Get("err")

	hv := &harnessView{
		Notice: notice,
		Error:  errMsg,
	}

	if s.Harness == nil {
		hv.HarnessOK = false
		hv.AutonomyLevelName = "Chưa kết nối"
		hv.AutonomyLevelBadge = "warn"
		hv.AutonomyDesc = "Master Harness Agent chưa được khởi tạo trong cấu hình máy chủ."
		s.render(w, "harness", s.ctx("Harness", hv))
		return
	}

	hv.HarnessOK = true
	level := s.Harness.AutonomyLevel()
	hv.AutonomyLevel = int(level)

	switch level {
	case harness.AutonomyAssisted:
		hv.AutonomyLevelName = "Cấp độ 1: Trợ lý tư vấn (Assisted)"
		hv.AutonomyLevelBadge = "info"
		hv.AutonomyDesc = "Agent chỉ quan sát, chẩn đoán và đề xuất kế hoạch. Cần con người bấm duyệt trước khi bấm máy sản xuất hoặc đăng bài."
	case harness.AutonomySemiAutonomous:
		hv.AutonomyLevelName = "Cấp độ 2: Bán tự động (Semi-Autonomous)"
		hv.AutonomyLevelBadge = "running"
		hv.AutonomyDesc = "Agent tự động nghiên cứu xu hướng và render video demo. Chỉ cần con người duyệt bước đăng bài lên kênh chính thức."
	case harness.AutonomyFullyAutonomous:
		hv.AutonomyLevelName = "Cấp độ 3: Tự hành toàn diện (Fully Autonomous)"
		hv.AutonomyLevelBadge = "ok"
		hv.AutonomyDesc = "Agent chủ động 100%: Quản lý danh mục kênh, săn sản phẩm affiliate, sinh video, đăng bài, đo lường và tự sửa sai."
	default:
		hv.AutonomyLevelName = "Tự hành toàn diện"
		hv.AutonomyLevelBadge = "ok"
	}

	// Chu kỳ gần nhất
	if latest := s.Harness.LatestCycle(); latest != nil {
		hv.LatestCycle = mapCycleToView(latest)
	}

	// Lịch sử các chu kỳ
	cycles := s.Harness.ListRecentCycles(10)
	hv.TotalCycles = len(cycles)
	for _, c := range cycles {
		hv.RecentCycles = append(hv.RecentCycles, mapCycleToView(c))
	}

	// Ký ức thực chiến
	if memStore := s.Harness.Memory(); memStore != nil {
		mems, err := memStore.ListAllMemories(r.Context(), 30)
		if err == nil {
			hv.TotalMemories = len(mems)
			for _, m := range mems {
				mCopy := m
				hv.Memories = append(hv.Memories, mapMemoryToView(&mCopy))
			}
		}
	}

	s.render(w, "harness", s.ctx("Harness", hv))
}

// handleHarnessRunCycle kích hoạt chu kỳ nhận thức OODA lập tức.
func (s *Server) handleHarnessRunCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Harness == nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Harness agent chưa khởi tạo"), http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	cycle, err := s.Harness.RunCycle(ctx)
	if err != nil {
		log.Printf("web: harness run cycle error: %v", err)
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Chu kỳ nhận thức gặp lỗi: "+err.Error()), http.StatusSeeOther)
		return
	}

	msg := fmt.Sprintf("Chu kỳ %s hoàn tất thành công! Agent đã đưa ra %d quyết định hành động.", cycle.CycleID, cycle.ActionCount)
	http.Redirect(w, r, "/harness?notice="+url.QueryEscape(msg), http.StatusSeeOther)
}

// handleHarnessAutonomy cập nhật cấp độ tự trị.
func (s *Server) handleHarnessAutonomy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Harness == nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Harness agent chưa khởi tạo"), http.StatusSeeOther)
		return
	}

	levelStr := strings.TrimSpace(r.FormValue("level"))
	var lvl harness.AutonomyLevel
	switch levelStr {
	case "1", "assisted":
		lvl = harness.AutonomyAssisted
	case "2", "semi_autonomous":
		lvl = harness.AutonomySemiAutonomous
	case "3", "fully_autonomous":
		lvl = harness.AutonomyFullyAutonomous
	default:
		lvl = harness.AutonomyFullyAutonomous
	}

	s.Harness.SetAutonomyLevel(lvl)
	if s.Ledger != nil {
		target := "autonomy_level"
		_ = s.Ledger.Decide("harness", "harness_autonomy", &target, fmt.Sprintf("cập nhật cấp độ tự hành: %d", lvl), nil)
	}

	http.Redirect(w, r, "/harness?notice="+url.QueryEscape("Đã cập nhật cấp độ tự chủ thành công!"), http.StatusSeeOther)
}

// handleHarnessMemoryAdd cho phép người quản trị huấn luyện hoặc nạp thêm kinh nghiệm cho Agent.
func (s *Server) handleHarnessMemoryAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.Harness == nil || s.Harness.Memory() == nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Kho ký ức chưa khả dụng"), http.StatusSeeOther)
		return
	}

	domain := strings.TrimSpace(r.FormValue("domain"))
	channel := strings.TrimSpace(r.FormValue("channel"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	insight := strings.TrimSpace(r.FormValue("insight"))
	scoreStr := strings.TrimSpace(r.FormValue("score"))

	if domain == "" || insight == "" {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Vui lòng điền lĩnh vực và nội dung bài học"), http.StatusSeeOther)
		return
	}

	score, err := strconv.ParseFloat(scoreStr, 64)
	if err != nil || score == 0 {
		score = 0.8
	}

	item := harness.AgentMemory{
		MemoryType:      harness.MemoryType(domain),
		ChannelID:       channel,
		Title:           subject,
		Insight:         insight,
		ConfidenceScore: score,
		ImpactScore:     score,
		SampleSize:      1,
		CreatedAt:       time.Now().Format("2006-01-02 15:04:05"),
		UpdatedAt:       time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := s.Harness.Memory().RecordInsight(r.Context(), item); err != nil {
		http.Redirect(w, r, "/harness?err="+url.QueryEscape("Lỗi lưu ký ức: "+err.Error()), http.StatusSeeOther)
		return
	}

	if s.Ledger != nil {
		target := subject
		_ = s.Ledger.Decide("harness", "harness_memory", &target, fmt.Sprintf("ghi nhận bài học: [%s] %s", domain, subject), nil)
	}

	http.Redirect(w, r, "/harness?notice="+url.QueryEscape("Đã nạp kiến thức mới vào bộ não Agent thành công!"), http.StatusSeeOther)
}

func mapCycleToView(c *harness.OODACycleResult) *harnessCycleView {
	if c == nil {
		return nil
	}

	var durStr string
	tStart, err1 := time.Parse(time.RFC3339, c.StartedAt)
	tEnd, err2 := time.Parse(time.RFC3339, c.FinishedAt)
	if err1 == nil && err2 == nil {
		durStr = fmt.Sprintf("%.2fs", tEnd.Sub(tStart).Seconds())
	} else {
		durStr = "1.2s"
	}

	channelCount := len(c.Observations.Channels)
	productCount := 0
	if c.Observations.HighROIProduct != "" {
		productCount = 1
	}
	queueCount := c.Observations.ActiveJobsCount

	return &harnessCycleView{
		CycleID:      c.CycleID,
		StartedAt:    c.StartedAt,
		FinishedAt:   c.FinishedAt,
		Duration:     durStr,
		Diagnosis:    c.Diagnosis,
		ActionCount:  c.ActionCount,
		Status:       c.Status,
		ChannelCount: channelCount,
		ProductCount: productCount,
		QueueCount:   queueCount,
		Decisions:    c.Decisions,
	}
}

func mapMemoryToView(m *harness.AgentMemory) *harnessMemoryView {
	if m == nil {
		return nil
	}

	domainVN := string(m.MemoryType)
	switch m.MemoryType {
	case harness.MemHookPerformance:
		domainVN = "Tỷ lệ giữ chân (Hook)"
	case harness.MemConversionInsight:
		domainVN = "Chuyển đổi Affiliate"
	case harness.MemFormatReflection:
		domainVN = "Định dạng Video"
	case harness.MemChannelLearning:
		domainVN = "Định vị Kênh"
	}

	isWarning := m.ImpactScore < 0
	scoreColor := "ok"
	if isWarning {
		scoreColor = "err"
	} else if m.ConfidenceScore < 0.6 {
		scoreColor = "warn"
	}

	return &harnessMemoryView{
		ID:         m.ID,
		Domain:     string(m.MemoryType),
		DomainVN:   domainVN,
		Channel:    m.ChannelID,
		Subject:    m.Title,
		Insight:    m.Insight,
		Confidence: fmt.Sprintf("%.0f%%", m.ConfidenceScore*100),
		ScoreColor: scoreColor,
		IsWarning:  isWarning,
		CreatedAt:  m.CreatedAt,
	}
}
