package web

import (
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/harness"
	"net/http"
	"strings"
	"time"
)

// harnessCycleView hiển thị chi tiết một chu kỳ OODA.
type harnessCycleView struct {
	CycleID             string
	StartedAt           string
	FinishedAt          string
	Duration            string
	Diagnosis           string
	ActionCount         int
	Status              string
	ChannelCount        int
	ProductCount        int
	QueueCount          int
	Decisions           []harness.OODADecisionTask
	Observations        *harness.OODAPerception
	RawJSON             string
	RawObservationsJSON string
	RawDecisionsJSON    string
}

// harnessMemoryView hiển thị một bài học/kinh nghiệm thực chiến.
type harnessMemoryView struct {
	ID            string
	Domain        string
	DomainVN      string
	Channel       string
	Niche         string
	FormatID      string
	Subject       string
	Insight       string
	Confidence    string
	ImpactScore   float64
	ImpactPercent string
	SampleSize    int
	SourceRef     string
	ScoreColor    string
	IsWarning     bool
	CreatedAt     string
	RawJSON       string
}

// harnessView là view model cho trang buồng lái Harness Agent (/harness).
type harnessView struct {
	HarnessOK            bool
	AutonomyLevel        int
	AutonomyLevelName    string
	AutonomyLevelBadge   string
	AutonomyDesc         string
	LatestCycle          *harnessCycleView
	RecentCycles         []*harnessCycleView
	Memories             []*harnessMemoryView
	TotalMemories        int
	TotalCycles          int
	AccountNames         []string
	ConfiguredNiches     []string
	DefaultPromptContext string
	Notice               string
	Error                string
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

	// Lấy kênh và ngách đã cấu hình.
	var promptChannel, promptNiche string
	if s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, a := range accounts {
				if a != nil && a.Username != "" {
					hv.AccountNames = append(hv.AccountNames, a.Username)
					if niche := strings.TrimSpace(a.Niche); niche != "" {
						hv.ConfiguredNiches = append(hv.ConfiguredNiches, niche)
						if promptNiche == "" {
							promptChannel, promptNiche = a.Username, niche
						}
					}
				}
			}
		}
	}

	if s.Harness == nil {
		hv.HarnessOK = false
		hv.AutonomyLevelName = "Chưa kết nối (Disconnected)"
		hv.AutonomyLevelBadge = "warn"
		hv.AutonomyDesc = "Bộ điều khiển Harness Agent chưa được khởi tạo trong cấu hình hiện tại."
		s.render(w, "harness", s.ctx("Harness", hv))
		return
	}

	hv.HarnessOK = true
	level := s.Harness.AutonomyLevel()
	hv.AutonomyLevel = int(level)

	switch level {
	case harness.AutonomyAssisted:
		hv.AutonomyLevelName = "Cấp độ 1: Trợ lý đề xuất (Assisted)"
		hv.AutonomyLevelBadge = "info"
		hv.AutonomyDesc = "OODA hiện ghi số kênh và job; chưa nối metrics từng video nên chưa thể đề xuất dựa trên hiệu quả. Sản xuất và đăng vẫn thủ công."
	case harness.AutonomySemiAutonomous:
		hv.AutonomyLevelName = "Cấp độ 2: Bán tự động (chưa hỗ trợ)"
		hv.AutonomyLevelBadge = "warn"
		hv.AutonomyDesc = "Mức này chưa tạo video nháp. Mọi đề xuất sản xuất hiện vẫn chờ đủ đầu vào và phê duyệt."
	case harness.AutonomyFullyAutonomous:
		hv.AutonomyLevelName = "Cấp độ 3: Tự hành 100% (chưa hỗ trợ)"
		hv.AutonomyLevelBadge = "warn"
		hv.AutonomyDesc = "Mức này chưa tự săn sản phẩm, sản xuất video hoặc đăng bài. Các action vẫn chờ đầu vào thủ công."
	default:
		hv.AutonomyLevelName = "Cấp độ 1: Trợ lý đề xuất (Assisted)"
		hv.AutonomyLevelBadge = "info"
		hv.AutonomyDesc = "Agent chỉ ghi nhận dữ liệu hiện có và đề xuất action; chưa tự tạo hoặc đăng video."
	}

	// Chu kỳ gần nhất
	if latest := s.Harness.LatestCycle(); latest != nil {
		hv.LatestCycle = mapCycleToView(latest)
	}

	// Lịch sử các chu kỳ
	cycles := s.Harness.ListRecentCycles(15)
	hv.TotalCycles = len(cycles)
	for _, c := range cycles {
		hv.RecentCycles = append(hv.RecentCycles, mapCycleToView(c))
	}

	// Ký ức thực chiến (Episodic Memory Bank)
	if memStore := s.Harness.Memory(); memStore != nil {
		mems, err := memStore.ListAllMemories(r.Context(), 50)
		if err == nil {
			hv.TotalMemories = len(mems)
			for _, m := range mems {
				mCopy := m
				hv.Memories = append(hv.Memories, mapMemoryToView(&mCopy))
			}
		}

		// Chỉ dựng prompt context khi có ngách do người dùng cấu hình.
		if promptNiche != "" {
			hv.DefaultPromptContext = memStore.GeneratePromptContext(r.Context(), promptNiche, promptChannel)
		}
	}

	s.render(w, "harness", s.ctx("Harness", hv))
}

// handleHarnessRunCycle kích hoạt chu kỳ nhận thức OODA lập tức.
func mapCycleToView(c *harness.OODACycleResult) *harnessCycleView {
	if c == nil {
		return nil
	}

	var durStr string
	tStart, err1 := time.Parse("2006-01-02 15:04:05", c.StartedAt)
	tEnd, err2 := time.Parse("2006-01-02 15:04:05", c.FinishedAt)
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

	rawBytes, _ := json.MarshalIndent(c, "", "  ")
	obsBytes, _ := json.MarshalIndent(c.Observations, "", "  ")
	decBytes, _ := json.MarshalIndent(c.Decisions, "", "  ")

	return &harnessCycleView{
		CycleID:             c.CycleID,
		StartedAt:           c.StartedAt,
		FinishedAt:          c.FinishedAt,
		Duration:            durStr,
		Diagnosis:           c.Diagnosis,
		ActionCount:         c.ActionCount,
		Status:              c.Status,
		ChannelCount:        channelCount,
		ProductCount:        productCount,
		QueueCount:          queueCount,
		Decisions:           c.Decisions,
		Observations:        &c.Observations,
		RawJSON:             string(rawBytes),
		RawObservationsJSON: string(obsBytes),
		RawDecisionsJSON:    string(decBytes),
	}
}

func mapMemoryToView(m *harness.AgentMemory) *harnessMemoryView {
	if m == nil {
		return nil
	}

	domainVN := string(m.MemoryType)
	switch m.MemoryType {
	case harness.MemHookPerformance:
		domainVN = "Giữ chân 3s (Hook)"
	case harness.MemConversionInsight:
		domainVN = "Chuyển đổi Affiliate"
	case harness.MemFormatReflection:
		domainVN = "Định dạng Video"
	case harness.MemChannelLearning:
		domainVN = "Khán giả Kênh"
	}

	isWarning := m.ImpactScore < 0
	scoreColor := "ok"
	if isWarning {
		scoreColor = "err"
	} else if m.ConfidenceScore < 0.6 {
		scoreColor = "warn"
	}

	rawBytes, _ := json.MarshalIndent(m, "", "  ")

	return &harnessMemoryView{
		ID:            m.ID,
		Domain:        string(m.MemoryType),
		DomainVN:      domainVN,
		Channel:       m.ChannelID,
		Niche:         m.Niche,
		FormatID:      m.FormatID,
		Subject:       m.Title,
		Insight:       m.Insight,
		Confidence:    fmt.Sprintf("%.0f%%", m.ConfidenceScore*100),
		ImpactScore:   m.ImpactScore,
		ImpactPercent: fmt.Sprintf("%+.0f%%", m.ImpactScore*100),
		SampleSize:    m.SampleSize,
		SourceRef:     m.SourceRef,
		ScoreColor:    scoreColor,
		IsWarning:     isWarning,
		CreatedAt:     m.CreatedAt,
		RawJSON:       string(rawBytes),
	}
}
