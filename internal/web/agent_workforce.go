package web

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AIAgent represents an autonomous virtual creator employee.
type AIAgent struct {
	ID             string   `json:"id"`
	Code           string   `json:"code,omitempty"`
	Name           string   `json:"name"`
	Avatar         string   `json:"avatar"`
	Role           string   `json:"role"`
	Niche          string   `json:"niche"`
	Tone           string   `json:"tone"`
	Voice          string   `json:"voice"`
	Level          int      `json:"level"`
	SLA            string   `json:"sla,omitempty"`
	KPI            string   `json:"kpi,omitempty"`
	VideosProduced int      `json:"videos_produced"`
	TotalViews     int64    `json:"total_views"`
	Status         string   `json:"status"` // "working", "idle", "learning"
	CurrentTask    string   `json:"current_task"`
	BestPractices  []string `json:"best_practices"`
	Autopilot      bool     `json:"autopilot"`
	IsCustom       bool     `json:"is_custom"`
	CreatedAt      string   `json:"created_at"`
	Departments    []string `json:"departments"`
}

// LiveOperationEvent represents a real-time event happening in one of the 8 enterprise departments.
type LiveOperationEvent struct {
	Time        string `json:"time"`
	DeptCode    string `json:"dept_code"`
	DeptName    string `json:"dept_name"`
	AgentName   string `json:"agent_name"`
	AgentAvatar string `json:"agent_avatar"`
	Action      string `json:"action"`
	Detail      string `json:"detail"`
	Tag         string `json:"tag"`
	BadgeClass  string `json:"badge_class"`
}

// AgentDeptTask represents a task assigned to an agent in a specific department.
type AgentDeptTask struct {
	AgentID      string `json:"agent_id"`
	DeptID       string `json:"dept_id"`
	RoleInDept   string `json:"role_in_dept"`
	Rank         string `json:"rank"`         // "lead", "senior", "specialist", "qc"
	StepOrder    int    `json:"step_order"`   // 1, 2, 3, 4 (thứ tự trong dây chuyền)
	ReceivesFrom string `json:"receives_from"`// Nhận đầu vào từ ai
	HandsOffTo   string `json:"hands_off_to"` // Bàn giao thành phẩm cho ai
	CurrentTask  string `json:"current_task"`
	TaskStatus   string `json:"task_status"` // "working", "completed", "review", "idle"
	Progress     int    `json:"progress"`    // 0-100%
	Deliverable  string `json:"deliverable"`
	LastUpdated  string `json:"last_updated"`
}

// DeptMember pairs an AIAgent with their specific task in a department.
type DeptMember struct {
	Agent        AIAgent `json:"agent"`
	RoleInDept   string  `json:"role_in_dept"`
	Rank         string  `json:"rank"`
	RankLabel    string  `json:"rank_label"`
	RankTierNum  int     `json:"rank_tier_num"`
	StepOrder    int     `json:"step_order"`
	ReceivesFrom string  `json:"receives_from"`
	HandsOffTo   string  `json:"hands_off_to"`
	CurrentTask  string  `json:"current_task"`
	TaskStatus   string  `json:"task_status"`
	Progress     int     `json:"progress"`
	Deliverable  string  `json:"deliverable"`
	LastUpdated  string  `json:"last_updated"`
	PosX         int     `json:"pos_x"`
	PosY         int     `json:"pos_y"`
}

// DeptRankTier represents a hierarchical rank level within a department org tree.
type DeptRankTier struct {
	TierNumber int          `json:"tier_number"` // 1, 2, 3, 4
	TierTitle  string       `json:"tier_title"`  // Tiêu đề cấp bậc
	BadgeClass string       `json:"badge_class"` // "lead", "senior", "specialist", "qc"
	Members    []DeptMember `json:"members"`
}

// NetworkLink represents a data transmission channel between two agent nodes in the network topology.
type NetworkLink struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Label    string `json:"label"`
	Color    string `json:"color"`
	Duration string `json:"duration"`
}

// AIDepartment represents an automated business unit / workflow department.
type AIDepartment struct {
	ID          string         `json:"id"`
	Code        string         `json:"code"`        // Mã phòng ban: DEP-01, DEP-02...
	Tier        int            `json:"tier"`        // Cấp bậc phòng ban: 1..8
	TierName    string         `json:"tier_name"`   // "Cấp 1 · Ban Điều Hành & Quản Trị Rủi Ro", v.v.
	Name        string         `json:"name"`
	LeaderName  string         `json:"leader_name"` // Tên trưởng phòng
	Icon        string         `json:"icon"`
	Workflow    string         `json:"workflow"`
	Description string         `json:"description"`
	ActionLink  string         `json:"action_link"`
	ActionLabel string         `json:"action_label"`
	SLA         string         `json:"sla,omitempty"`
	KPI         string         `json:"kpi,omitempty"`
	Throughput  string         `json:"throughput,omitempty"`
	Active247   bool           `json:"active_247,omitempty"`
	Members     []DeptMember   `json:"members"`
	Lead        *DeptMember    `json:"lead,omitempty"`
	Specialists []DeptMember   `json:"specialists,omitempty"`
	QC          *DeptMember    `json:"qc,omitempty"`
	RankTiers   []DeptRankTier `json:"rank_tiers,omitempty"` // Các tầng cấp bậc từ Cấp 1 -> Cấp 2 -> Cấp 3 -> Cấp 4
	Links       []NetworkLink  `json:"links,omitempty"`
}

var (
	agentMu           sync.RWMutex
	fallbackAgents    []AIAgent
	fallbackDeptTasks []AgentDeptTask
)

func defaultEliteAgents() []AIAgent {
	return []AIAgent{
		{
			ID:             "agent-exec-director",
			Code:           "AGENT-EXEC-01",
			Name:           "Alex Vũ (Giám Đốc Vận Hành & Điều Phối)",
			Avatar:         "🏛️",
			Role:           "Giám Đốc Điều Hành Chiến Lược & An Toàn Kênh",
			Niche:          "điều phối & quản trị rủi ro",
			Tone:           "Quyết đoán, chuẩn mực, phân bổ tài nguyên GPU/API và bảo vệ an toàn kênh 24/7",
			Voice:          "Nam miền Bắc đĩnh đạc (VieNeu-TTS)",
			Level:          10,
			SLA:            "< 2s điều phối",
			KPI:            "99.9% Uptime · 0 vi phạm chính sách",
			VideosProduced: 128,
			TotalViews:     5840000,
			Status:         "working",
			CurrentTask:    "Giám sát phân bổ quota API Gemini và cân bằng tải 8 phòng ban tác chiến 24/7",
			BestPractices: []string{
				"Phân luồng quota API theo thứ tự ưu tiên giúp hệ thống không bao giờ bị nghẽn lệnh xuất bản.",
				"Khóa an toàn bản quyền trước khi phân phối giúp triệt tiêu 100% rủi ro bị quét gậy nền tảng.",
				"Tự động ngắt render nếu nhiệt độ GPU hoặc chi phí token vượt ngưỡng an toàn.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-08-01",
		},
		{
			ID:             "agent-trend-hunter",
			Code:           "AGENT-RADAR-02",
			Name:           "Radar Kai (Trưởng Ban Săn Xu Hướng Viral)",
			Avatar:         "📡",
			Role:           "Trưởng Ban Tình Báo Xu Hướng & Săn Viral Douyin/TikTok",
			Niche:          "quét xu hướng & âm thanh viral",
			Tone:           "Nhạy bén, phân tích dữ liệu thuật toán theo thời gian thực",
			Voice:          "Nam miền Nam công nghệ (VieNeu-TTS)",
			Level:          9,
			SLA:            "15 phút / chu kỳ quét",
			KPI:            "Phát hiện trend trước 24h · 92% audio hot",
			VideosProduced: 96,
			TotalViews:     4280000,
			Status:         "working",
			CurrentTask:    "Quét radar Douyin bắt audio #CinematicBeat và bóc tách cấu trúc 15s triệu view",
			BestPractices: []string{
				"Âm thanh Douyin tăng trưởng >300% trong 2h đầu thường đạt đỉnh trên TikTok VN sau 24-48h.",
				"Hook thị giác biến đổi đồ vật trong 1.5s đầu có tỷ lệ giữ chân khán giả cao gấp 3.2 lần.",
				"Lọc bỏ các audio dính bản quyền thương mại trước khi đưa vào hàng đợi sản xuất.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-08-10",
		},
		{
			ID:             "agent-bep-me-bi",
			Code:           "AGENT-COMM-03",
			Name:           "Elena Bùi - Bếp Mẹ Bi (Trưởng Phòng Thẩm Định & Săn Deal)",
			Avatar:         "⚖️",
			Role:           "Trưởng Khối Thẩm Định Ý Tưởng & Săn Deal Chuyển Đổi Cao",
			Niche:          "thương mại & gia dụng thông minh",
			Tone:           "Sắc sảo, thực dụng, định lượng bằng tỷ lệ chuyển đổi ROI và hoa hồng ròng",
			Voice:          "Nữ miền Bắc truyền cảm (VieNeu-TTS)",
			Level:          8,
			SLA:            "< 30s thẩm định deal",
			KPI:            "Hoa hồng ròng > 15% · Tỷ lệ duyệt >= 85đ",
			VideosProduced: 68,
			TotalViews:     2450000,
			Status:         "working",
			CurrentTask:    "Thẩm định deal Serum và Nồi chiên không dầu hoa hồng 22% sàn Accesstrade",
			BestPractices: []string{
				"Sản phẩm có rating 4.8★ và trên 1.000 lượt bán tạo tỷ lệ chuyển đổi affiliate cao nhất.",
				"Video review có demo thử nghiệm tính năng thực tế giúp tỷ lệ bấm giỏ hàng tăng 2.3 lần.",
				"Ghim ưu đãi giảm giá có hạn giờ vàng 19h-21h triệt tiêu 85% sự chần chừ của người xem.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-09-10",
		},
		{
			ID:             "agent-content-strat",
			Code:           "AGENT-STRAT-04",
			Name:           "Daniel Trần (Trưởng Ban Kế Hoạch & Nội Dung)",
			Avatar:         "📅",
			Role:           "Trưởng Ban Chiến Lược Kênh & Lịch Biên Tập Giờ Vàng",
			Niche:          "ma trận nội dung & tăng trưởng kênh",
			Tone:           "Chiến lược dài hạn, cân bằng ma trận 60-30-10 và phân luồng kênh",
			Voice:          "Nam miền Nam đĩnh đạc (VieNeu-TTS)",
			Level:          9,
			SLA:            "Lập lịch 30 ngày tự động",
			KPI:            "0% trùng lặp nội dung · 100% khớp giờ vàng",
			VideosProduced: 82,
			TotalViews:     3300000,
			Status:         "working",
			CurrentTask:    "Lập ma trận nội dung 30 ngày cho kênh Mỹ Phẩm và phân bổ slot giờ vàng 19:30",
			BestPractices: []string{
				"Tỷ lệ vàng 60% viral kéo phễu - 30% chia sẻ giá trị - 10% bán hàng giúp kênh tăng trưởng bền vững.",
				"Xếp lịch phát hành cách nhau tối thiểu 3 giờ để không tự bóp tương tác lẫn nhau.",
				"Kênh dưới 10k follower cần tối thiểu 2 video/ngày vào khung 11h30 và 19h30.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-08-12",
		},
		{
			ID:             "agent-minh-thu",
			Code:           "AGENT-COPY-05",
			Name:           "Minh Thư (Trưởng Ban Biên Kịch Triệu View)",
			Avatar:         "✍️",
			Role:           "Giám Đốc Sáng Tạo Kịch Bản & Tâm Lý Học Khán Giả",
			Niche:          "mỹ phẩm & thời trang",
			Tone:           "Thấu hiểu tâm lý, chuyên gia Hook 3s giật gân, nhịp kịch bản 3 hồi kịch tính",
			Voice:          "Nữ miền Nam ngọt ngào (VieNeu-TTS)",
			Level:          9,
			SLA:            "< 18s / kịch bản 5 cảnh",
			KPI:            "Tỷ lệ giữ chân 3s > 84% · Điểm QC kịch bản > 90",
			VideosProduced: 88,
			TotalViews:     3950000,
			Status:         "working",
			CurrentTask:    "Soạn kịch bản #KB-204 với Hook 3s giữ chân 86% và phân cảnh chuyển hóa cảm xúc",
			BestPractices: []string{
				"Hook dạng cảnh báo da dầu mụn bết dính giữ chân 84% người xem trong 3s đầu.",
				"Cảnh demo macro chất kem tan trên da giúp tỷ lệ bấm giỏ hàng tăng 2.3 lần.",
				"CTA chỉ tay góc trái nhận voucher tạo tỷ lệ chuyển đổi cao nhất lúc 19h30.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-09-01",
		},
		{
			ID:             "agent-duc-huy",
			Code:           "AGENT-ART-06",
			Name:           "Đức Huy (Giám Đốc Kỹ Thuật Studio & Dựng Video)",
			Avatar:         "🎬",
			Role:           "Đạo Diễn Kỹ Xảo Hình Ảnh, Âm Thanh & Video Nhịp Nhanh 4K",
			Niche:          "công nghệ & kỹ xảo điện ảnh",
			Tone:           "Chính xác từng khung hình, kiểm soát ánh sáng điện ảnh và tự động cân bằng âm thanh -14 LUFS",
			Voice:          "Nam miền Bắc trầm ấm (VieNeu-TTS)",
			Level:          9,
			SLA:            "< 45s hoàn tất render video 4K",
			KPI:            "100% đồng nhất nhân vật · Cân bằng âm thanh tự động -14 LUFS",
			VideosProduced: 96,
			TotalViews:     4350000,
			Status:         "working",
			CurrentTask:    "Render 4K Imagen 3 phân cảnh tai nghe chống ồn, tự động cân bằng âm lượng -14 LUFS",
			BestPractices: []string{
				"So sánh độ trễ chơi game giữa tai nghe thường và chip mới tạo tương tác bình luận bùng nổ.",
				"Mô phỏng 3D vi mạch công nghệ bên trong giúp thời gian xem trung bình đạt 24s/30s.",
				"Cam kết bảo hành 1 đổi 1 chính hãng triệt tiêu 90% sự do dự của khách hàng.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-08-15",
		},
		{
			ID:             "agent-son-bach",
			Code:           "AGENT-QC-07",
			Name:           "Sơn Bách (Trưởng Ban Kiểm Soát Chất Lượng QC)",
			Avatar:         "🛡️",
			Role:           "Thanh Tra Trưởng Thẩm Định Chất Lượng Độc Lập & Vùng An Toàn Hiển Thị",
			Niche:          "thẩm định QC & kiểm định phát hành",
			Tone:           "Nghiêm ngặt, khách quan, không thỏa hiệp với video lỗi, soi từng khung hình và tần số âm thanh",
			Voice:          "Nam phát thanh viên kể chuyện (VieNeu-TTS)",
			Level:          9,
			SLA:            "< 5s thẩm định toàn diện video",
			KPI:            "100% video đạt chuẩn vùng an toàn & chính sách nền tảng",
			VideosProduced: 112,
			TotalViews:     5100000,
			Status:         "working",
			CurrentTask:    "Cấp chứng nhận QC 96/100 cho Video #VD-108, xác nhận chuẩn vùng an toàn hiển thị TikTok",
			BestPractices: []string{
				"Mở đầu bằng câu hỏi lật ngược tâm lý giữ chân 88% người xem không lướt qua.",
				"Vùng an toàn 9:16 phải chừa mép dưới 120px và mép phải 80px để tránh bị nút TikTok che.",
				"Kiểm tra mức âm lượng thoại luôn nằm trong khoảng -14 LUFS đến -16 LUFS, không bị rè méo tiếng.",
			},
			Autopilot: false,
			IsCustom:  false,
			CreatedAt: "2026-08-20",
		},
		{
			ID:             "agent-growth-analyst",
			Code:           "AGENT-DIST-08",
			Name:           "Marcus Lê (Trưởng Khối Tăng Trưởng & Phân Phối Đa Kênh)",
			Avatar:         "📱",
			Role:           "Chuyên Gia Tăng Trưởng Kênh, Phân Phối Đa Nền Tảng & Phễu Chuyển Đổi",
			Niche:          "học máy & tối ưu hóa tự động",
			Tone:           "Định lượng dữ liệu, mổ xẻ đường cong giữ chân và tối ưu hóa giỏ hàng theo thời gian thực",
			Voice:          "Nam miền Bắc trẻ trung (VieNeu-TTS)",
			Level:          9,
			SLA:            "Khớp 100% slot giờ vàng",
			KPI:            "Tỷ lệ nhấp giỏ hàng CTR > 4.5% · Tăng trưởng view +35%",
			VideosProduced: 79,
			TotalViews:     3420000,
			Status:         "working",
			CurrentTask:    "Lên lịch phát sóng 19:30 trên TikTok & Shorts, gắn link bio và tracking link Accesstrade",
			BestPractices: []string{
				"Gắn link bio và tracking link Accesstrade ngay khi đăng bài giúp tối ưu tỷ lệ chuyển đổi đơn hàng.",
				"Chống trùng bản quyền chéo bằng cách thay đổi mã băm video (hash MD5) và vi chỉnh tốc độ 1.02x.",
				"Mổ xẻ đồ thị rớt người xem: nếu tụt mạnh ở giây 3, chuyển phản hồi ngay cho biên kịch đổi verbal hook.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-08-25",
		},
		{
			ID:             "agent-hoang-nam",
			Code:           "AGENT-ANALYTICS-09",
			Name:           "Dr. Hoàng Nam (Trưởng Ban Phân Tích & Đổi Mới Ý Tưởng)",
			Avatar:         "📊",
			Role:           "Trưởng Phòng Phân Tích Dữ Liệu, Đo Lường & Đổi Mới Ý Tưởng",
			Niche:          "thuật toán phân phối & tối ưu hóa kịch bản",
			Tone:           "Định lượng chính xác, tư duy khoa học dữ liệu, phát hiện sớm mẫu hình tăng trưởng",
			Voice:          "Nam miền Bắc đĩnh đạc (VieNeu-TTS)",
			Level:          9,
			SLA:            "< 10 phút sau khi có dữ liệu view",
			KPI:            "Tỷ lệ giữ chân tăng +20% · 100% video đột phá được nhân bản",
			VideosProduced: 105,
			TotalViews:     4850000,
			Status:         "working",
			CurrentTask:    "Phân tích đồ thị drop-off video #VD-108, đề xuất Phòng Sáng Tạo đổi hook mở đầu từ giây 3",
			BestPractices: []string{
				"Phân tích điểm gãy người xem tại giây thứ 3 và thứ 15 để đưa phản hồi ngược về cho Phòng Nội Dung.",
				"Khi video đạt hệ số view > 2.5x so với median, ngay lập tức gửi tín hiệu cho Phòng Kế Hoạch nhân bản series 3 tập theo format đó.",
				"Nếu tỷ lệ xem hết < 25%, gắn cờ cảnh báo yêu cầu dừng sản xuất format để tối ưu lại kịch bản.",
			},
			Autopilot: true,
			IsCustom:  false,
			CreatedAt: "2026-08-01",
		},
	}
}

func (s *Server) ensureAgentTable() error {
	if s.db == nil {
		return nil
	}
	agentMu.Lock()
	defer agentMu.Unlock()

	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			avatar TEXT NOT NULL,
			role TEXT NOT NULL,
			niche TEXT NOT NULL,
			tone TEXT NOT NULL,
			voice TEXT NOT NULL,
			level INTEGER NOT NULL DEFAULT 1,
			videos_produced INTEGER NOT NULL DEFAULT 0,
			total_views INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'idle',
			current_task TEXT NOT NULL DEFAULT '',
			best_practices_json TEXT NOT NULL DEFAULT '[]',
			autopilot INTEGER NOT NULL DEFAULT 1,
			is_custom INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			code TEXT NOT NULL DEFAULT '',
			sla TEXT NOT NULL DEFAULT '',
			kpi TEXT NOT NULL DEFAULT ''
		);
	`)
	if err != nil {
		return err
	}

	// Auto-migrate new columns
	_, _ = s.db.Exec(`ALTER TABLE ai_agents ADD COLUMN code TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE ai_agents ADD COLUMN sla TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE ai_agents ADD COLUMN kpi TEXT NOT NULL DEFAULT ''`)

	// Seed enterprise default agents only if table is empty (preserving runtime state)
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ai_agents`).Scan(&count)
	if count == 0 {
		for _, ag := range defaultEliteAgents() {
			bpBytes, _ := json.Marshal(ag.BestPractices)
			apVal := 0
			if ag.Autopilot {
				apVal = 1
			}
			_, _ = s.db.Exec(`
				INSERT INTO ai_agents (id, name, avatar, role, niche, tone, voice, level, videos_produced, total_views, status, current_task, best_practices_json, autopilot, is_custom, created_at, code, sla, kpi)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)
			`, ag.ID, ag.Name, ag.Avatar, ag.Role, ag.Niche, ag.Tone, ag.Voice, ag.Level, ag.VideosProduced, ag.TotalViews, ag.Status, ag.CurrentTask, string(bpBytes), apVal, ag.CreatedAt, ag.Code, ag.SLA, ag.KPI)
		}
	}
	return nil
}

// listAgents returns all active AI agents (from DB or fallback).
func (s *Server) listAgents() []AIAgent {
	if err := s.ensureAgentTable(); err == nil && s.db != nil {
		rows, err := s.db.Query(`
			SELECT id, name, avatar, role, niche, tone, voice, level, videos_produced, total_views, status, current_task, best_practices_json, autopilot, is_custom, created_at, code, sla, kpi
			FROM ai_agents
			ORDER BY level DESC, videos_produced DESC
		`)
		if err == nil {
			defer rows.Close()
			var list []AIAgent
			for rows.Next() {
				var ag AIAgent
				var bpJSON string
				var apVal, custVal int
				var code, sla, kpi string
				if scanErr := rows.Scan(&ag.ID, &ag.Name, &ag.Avatar, &ag.Role, &ag.Niche, &ag.Tone, &ag.Voice, &ag.Level, &ag.VideosProduced, &ag.TotalViews, &ag.Status, &ag.CurrentTask, &bpJSON, &apVal, &custVal, &ag.CreatedAt, &code, &sla, &kpi); scanErr == nil {
					ag.Autopilot = apVal == 1
					ag.IsCustom = custVal == 1
					ag.Code = code
					ag.SLA = sla
					ag.KPI = kpi
					_ = json.Unmarshal([]byte(bpJSON), &ag.BestPractices)
					list = append(list, ag)
				}
			}
			if len(list) > 0 {
				return list
			}
		}
	}

	agentMu.RLock()
	defer agentMu.RUnlock()
	if len(fallbackAgents) == 0 {
		return defaultEliteAgents()
	}
	return fallbackAgents
}

// handleAgentCreate handles user creation of a new custom AI agent.
func (s *Server) handleAgentCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		seeOther(w, r, "/team?err="+url.QueryEscape("Vui lòng nhập tên nhân sự AI"))
		return
	}
	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		role = "Chuyên Gia Sáng Tạo Kịch Bản"
	}
	niche := strings.TrimSpace(r.FormValue("niche"))
	if niche == "" {
		niche = "tổng hợp"
	}
	tone := strings.TrimSpace(r.FormValue("tone"))
	if tone == "" {
		tone = "Tự nhiên, chân thành, tập trung vào lợi ích khách hàng"
	}
	voice := strings.TrimSpace(r.FormValue("voice"))
	if voice == "" {
		voice = "default"
	}
	avatar := strings.TrimSpace(r.FormValue("avatar"))
	if avatar == "" {
		avatar = "🤖"
	}

	agentID := fmt.Sprintf("custom-%d", time.Now().UnixNano())
	now := time.Now().Format("2006-01-02")
	initialPractices := []string{
		fmt.Sprintf("Được đào tạo cho phong cách: %s", tone),
		fmt.Sprintf("Chuyên sâu ngành hàng: %s", niche),
	}
	bpBytes, _ := json.Marshal(initialPractices)

	if s.db != nil {
		_ = s.ensureAgentTable()
		_, _ = s.db.Exec(`
			INSERT INTO ai_agents (id, name, avatar, role, niche, tone, voice, level, videos_produced, total_views, status, current_task, best_practices_json, autopilot, is_custom, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 1, 0, 0, 'idle', 'Sẵn sàng nhận nhiệm vụ đầu tiên', ?, 1, 1, ?)
		`, agentID, name, avatar, role, niche, tone, voice, string(bpBytes), now)
	} else {
		agentMu.Lock()
		if len(fallbackAgents) == 0 {
			fallbackAgents = defaultEliteAgents()
		}
		fallbackAgents = append(fallbackAgents, AIAgent{
			ID:             agentID,
			Name:           name,
			Avatar:         avatar,
			Role:           role,
			Niche:          niche,
			Tone:           tone,
			Voice:          voice,
			Level:          1,
			VideosProduced: 0,
			TotalViews:     0,
			Status:         "idle",
			CurrentTask:    "Sẵn sàng nhận nhiệm vụ đầu tiên",
			BestPractices:  initialPractices,
			Autopilot:      true,
			IsCustom:       true,
			CreatedAt:      now,
		})
		agentMu.Unlock()
	}

	seeOther(w, r, "/team?ok="+url.QueryEscape("Đã bổ sung nhân sự AI mới: "+name))
}

// handleAgentToggleAutopilot toggles 24/7 autopilot for an agent.
func (s *Server) handleAgentToggleAutopilot(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id == "" {
		seeOther(w, r, "/team")
		return
	}

	if s.db != nil {
		_ = s.ensureAgentTable()
		_, _ = s.db.Exec(`UPDATE ai_agents SET autopilot = CASE WHEN autopilot = 1 THEN 0 ELSE 1 END WHERE id = ?`, id)
	} else {
		agentMu.Lock()
		for i := range fallbackAgents {
			if fallbackAgents[i].ID == id {
				fallbackAgents[i].Autopilot = !fallbackAgents[i].Autopilot
				break
			}
		}
		agentMu.Unlock()
	}

	seeOther(w, r, "/team?ok="+url.QueryEscape("Đã cập nhật trạng thái tự hành."))
}

// handleAgentDelete deletes a custom agent.
func (s *Server) handleAgentDelete(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id == "" {
		seeOther(w, r, "/team")
		return
	}

	if s.db != nil {
		_ = s.ensureAgentTable()
		_, _ = s.db.Exec(`DELETE FROM ai_agents WHERE id = ? AND is_custom = 1`, id)
	} else {
		agentMu.Lock()
		var nList []AIAgent
		for _, ag := range fallbackAgents {
			if ag.ID != id || !ag.IsCustom {
				nList = append(nList, ag)
			}
		}
		fallbackAgents = nList
		agentMu.Unlock()
	}

	seeOther(w, r, "/team?ok="+url.QueryEscape("Đã xóa nhân sự AI."))
}

// handleTeamDispatchGoal takes an Omni-Goal from Dashboard or Team page,
// dispatches an autonomous mission via Mission Engine, and forwards to Studio/Dashboard.
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

	goalLower := strings.ToLower(goal)
	matchedNiche := "mỹ phẩm"
	productSample := "Serum Trắng Da Mờ Thâm Nám"

	if strings.Contains(goalLower, "công nghệ") || strings.Contains(goalLower, "tai nghe") || strings.Contains(goalLower, "điện tử") || strings.Contains(goalLower, "sạc") {
		matchedNiche = "công nghệ"
		productSample = "Tai Nghe Chống Ồn Sony WH-1000XM5"
	} else if strings.Contains(goalLower, "gia dụng") || strings.Contains(goalLower, "nồi") || strings.Contains(goalLower, "bếp") || strings.Contains(goalLower, "nhà") {
		matchedNiche = "gia dụng"
		productSample = "Nồi Chiên Không Dầu Điện Tử 8L"
	} else if strings.Contains(goalLower, "kể chuyện") || strings.Contains(goalLower, "lịch sử") || strings.Contains(goalLower, "truyện") {
		seeOther(w, r, "/stories?ok="+url.QueryEscape("Đã khởi tạo kịch bản kể chuyện: "+mission.ResultSummary))
		return
	}

	if r.FormValue("back") == "/" {
		seeOther(w, r, "/?ok="+url.QueryEscape("Đã kích hoạt chiến dịch AI: "+mission.ResultSummary))
		return
	}

	targetURL := fmt.Sprintf("/studio?product_name=%s&niche=%s&msg=%s",
		url.QueryEscape(productSample),
		url.QueryEscape(matchedNiche),
		url.QueryEscape("Đã kích hoạt chiến dịch AI thành công: "+mission.ResultSummary),
	)
	seeOther(w, r, targetURL)
}

func defaultDeptTasks() []AgentDeptTask {
	return []AgentDeptTask{
		// 1. Dept Exec (DEP-01) · Ban Điều Hành & Quản Trị Rủi Ro
		{
			AgentID:      "agent-exec-director",
			DeptID:       "dept-exec",
			RoleInDept:   "Giám Đốc Điều Phối & Quản Trị Hệ Thống 24/7",
			Rank:         "lead",
			StepOrder:    1,
			ReceivesFrom: "Mục tiêu chiến lược từ Ban Lãnh Đạo & Dashboard Chỉ Huy",
			HandsOffTo:   "Chuyên Viên Quản Trị Rủi Ro Marcus Lê (Khâu 2)",
			CurrentTask:  "Phân bổ hạn ngạch quota Gemini API và điều phối 8 phòng ban tác chiến 24/7",
			TaskStatus:   "working",
			Progress:     95,
			Deliverable:  "Bảng phân bổ tài nguyên tác chiến và chỉ đạo sản xuất thời gian thực",
			LastUpdated:  "Vừa cập nhật",
		},
		{
			AgentID:      "agent-growth-analyst",
			DeptID:       "dept-exec",
			RoleInDept:   "Chuyên Viên Quản Trị Tuân Thủ & An Toàn Kênh",
			Rank:         "senior",
			StepOrder:    2,
			ReceivesFrom: "Lệnh phân bổ từ Alex Vũ (Khâu 1)",
			HandsOffTo:   "Các Khối Phòng Ban Nghiên Cứu & Sản Xuất",
			CurrentTask:  "Kiểm soát chính sách nền tảng TikTok/Meta/YouTube, chống gậy bản quyền và bảo vệ an toàn tài khoản",
			TaskStatus:   "completed",
			Progress:     100,
			Deliverable:  "Báo cáo an toàn tài khoản 100% · 0 rủi ro chính sách",
			LastUpdated:  "12 phút trước",
		},

		// 2. Dept Intelligence (DEP-02) · Khối Tình Báo Thị Trường & Săn Viral
		{
			AgentID:      "agent-trend-hunter",
			DeptID:       "dept-intelligence",
			RoleInDept:   "Trưởng Ban Tình Báo Xu Hướng & Săn Viral Douyin/TikTok",
			Rank:         "lead",
			StepOrder:    1,
			ReceivesFrom: "Dữ liệu thời gian thực từ Douyin, TikTok Creative Center & YouTube Trends",
			HandsOffTo:   "Chuyên Viên Bóc Tách Format Viral Đức Huy (Khâu 2)",
			CurrentTask:  "Quét radar Douyin bắt audio #CinematicBeat và bóc tách cấu trúc 15s triệu view",
			TaskStatus:   "working",
			Progress:     90,
			Deliverable:  "Top 5 audio thịnh hành và 3 video có tốc độ tăng trưởng >300%/h",
			LastUpdated:  "5 phút trước",
		},
		{
			AgentID:      "agent-duc-huy",
			DeptID:       "dept-intelligence",
			RoleInDept:   "Chuyên Viên Bóc Tách Format & Phân Tích Đối Thủ",
			Rank:         "senior",
			StepOrder:    2,
			ReceivesFrom: "Danh sách tín hiệu hot từ Radar Kai (Khâu 1)",
			HandsOffTo:   "Khối Thẩm Định Ý Tưởng & Thương Mại Affiliate",
			CurrentTask:  "Phân tích cấu trúc video đối thủ đầu ngành, trích xuất khung hình và hiệu ứng giữ chân",
			TaskStatus:   "working",
			Progress:     85,
			Deliverable:  "Hồ sơ bóc tách kỹ xảo và bản đồ phân tích đối thủ cạnh tranh",
			LastUpdated:  "14 phút trước",
		},

		// 3. Dept Affiliate (DEP-03) · Khối Thẩm Định Ý Tưởng & Thương Mại Affiliate
		{
			AgentID:      "agent-bep-me-bi",
			DeptID:       "dept-affiliate",
			RoleInDept:   "Trưởng Khối Thẩm Định Ý Tưởng & Săn Deal Sàn",
			Rank:         "lead",
			StepOrder:    1,
			ReceivesFrom: "Sàn thương mại điện tử qua đối tác Accesstrade (Shopee, Lazada, Tiki)",
			HandsOffTo:   "Chuyên Viên Kỹ Thuật Đức Huy (Khâu 2)",
			CurrentTask:  "Quét top 10 sản phẩm đồ gia dụng thông minh có hoa hồng trên 15% và đánh giá 4.8★",
			TaskStatus:   "working",
			Progress:     85,
			Deliverable:  "Kệ hàng 10 deal gia dụng bán chạy nhất kèm link affiliate",
			LastUpdated:  "5 phút trước",
		},
		{
			AgentID:      "agent-duc-huy",
			DeptID:       "dept-affiliate",
			RoleInDept:   "Chuyên Viên Phân Tích Thông Số & Đối Thủ",
			Rank:         "specialist",
			StepOrder:    2,
			ReceivesFrom: "Danh sách 10 deal gia dụng từ Bếp Mẹ Bi (Khâu 1)",
			HandsOffTo:   "Chuyên Viên Insight Khách Hàng Minh Thư (Khâu 3)",
			CurrentTask:  "Lập bảng so sánh độ trễ âm thanh bluetooth giữa các dòng tai nghe chống ồn dưới 1 triệu",
			TaskStatus:   "working",
			Progress:     70,
			Deliverable:  "Bảng thông số kỹ thuật thực tế dùng làm luận cứ chốt đơn",
			LastUpdated:  "18 phút trước",
		},
		{
			AgentID:      "agent-minh-thu",
			DeptID:       "dept-affiliate",
			RoleInDept:   "Chuyên Viên Insight Khách Hàng & Thông Điệp Bán Hàng",
			Rank:         "senior",
			StepOrder:    3,
			ReceivesFrom: "Bảng thông số và tính năng từ Đức Huy (Khâu 2)",
			HandsOffTo:   "Khối Chiến Lược Kênh & Lịch Biên Tập",
			CurrentTask:  "Nghiên cứu tệp khách hàng Gen Z, tối ưu thông điệp bán hàng và CTA chốt đơn",
			TaskStatus:   "completed",
			Progress:     100,
			Deliverable:  "Bộ 3 mẫu CTA giữ chân và kích thích bấm giỏ hàng ngay lập tức",
			LastUpdated:  "30 phút trước",
		},

		// 4. Dept Story (DEP-04) · Khối Chiến Lược Kênh & Lịch Biên Tập
		{
			AgentID:      "agent-content-strat",
			DeptID:       "dept-story",
			RoleInDept:   "Trưởng Ban Chiến Lược Kênh & Lịch Biên Tập Giờ Vàng",
			Rank:         "lead",
			StepOrder:    1,
			ReceivesFrom: "Ý tưởng thẩm định từ Khối Thẩm Định Deal",
			HandsOffTo:   "Chuyên Viên Cốt Truyện Sơn Bách (Khâu 2)",
			CurrentTask:  "Lập ma trận nội dung 30 ngày (60% viral, 30% giá trị, 10% chuyển đổi) và phân luồng kênh",
			TaskStatus:   "working",
			Progress:     92,
			Deliverable:  "Bảng kế hoạch xuất bản 30 ngày phủ sóng khung giờ vàng 11h30 và 19h30",
			LastUpdated:  "6 phút trước",
		},
		{
			AgentID:      "agent-son-bach",
			DeptID:       "dept-story",
			RoleInDept:   "Chuyên Gia Cốt Truyện & Lời Thoại",
			Rank:         "senior",
			StepOrder:    2,
			ReceivesFrom: "Khung ma trận nội dung từ Daniel Trần (Khâu 1)",
			HandsOffTo:   "Chuyên Viên Cấu Trúc Khung Kịch Bản Đức Huy (Khâu 3)",
			CurrentTask:  "Soạn kịch bản truyện 3 hồi, tạo cao trào hồi hộp ở phút thứ 2 và câu hỏi lật mở bí ẩn ở kết thúc",
			TaskStatus:   "working",
			Progress:     75,
			Deliverable:  "Cốt truyện 3 hồi đạt điểm giữ chân người xem 88%",
			LastUpdated:  "12 phút trước",
		},
		{
			AgentID:      "agent-duc-huy",
			DeptID:       "dept-story",
			RoleInDept:   "Chuyên Viên Cấu Trúc Khung & Prompts 4K",
			Rank:         "specialist",
			StepOrder:    3,
			ReceivesFrom: "Cốt truyện từ Sơn Bách (Khâu 2)",
			HandsOffTo:   "Khối Sáng Tạo Nội Dung & Biên Kịch",
			CurrentTask:  "Chuyển đổi từng phân cảnh kịch bản thành các câu lệnh Prompt Imagen 4K",
			TaskStatus:   "completed",
			Progress:     100,
			Deliverable:  "Bộ Prompt phân cảnh chuẩn cấu trúc 9:16",
			LastUpdated:  "22 phút trước",
		},

		// 5. Dept Creative (DEP-05) · Khối Sáng Tạo Nội Dung & Biên Kịch
		{
			AgentID:      "agent-minh-thu",
			DeptID:       "dept-creative",
			RoleInDept:   "Giám Đốc Sáng Tạo Kịch Bản & Hook 3s Nghẹt Thở",
			Rank:         "lead",
			StepOrder:    1,
			ReceivesFrom: "Đề bài brief và ma trận nội dung từ Ban Biên Tập",
			HandsOffTo:   "Chuyên Gia Chuyển Thể Cốt Truyện Sơn Bách (Khâu 2)",
			CurrentTask:  "Soạn 4 Hook 3s dạng cảnh báo giữ chân 86%+ và kịch bản 5 phân cảnh theo tâm lý học hành vi",
			TaskStatus:   "working",
			Progress:     90,
			Deliverable:  "Kịch bản 5 phân cảnh hoàn chỉnh kèm lời thoại và chỉ dẫn diễn xuất",
			LastUpdated:  "8 phút trước",
		},
		{
			AgentID:      "agent-son-bach",
			DeptID:       "dept-creative",
			RoleInDept:   "Chuyên Gia Việt Hóa Kịch Bản & Cốt Truyện Douyin",
			Rank:         "senior",
			StepOrder:    2,
			ReceivesFrom: "Kịch bản nháp từ Minh Thư (Khâu 1)",
			HandsOffTo:   "Studio Sản Xuất Video & Âm Thanh AI",
			CurrentTask:  "Việt hóa câu chuyện Douyin triệu view, tinh chỉnh ngữ cảnh văn hóa và tâm lý người Việt",
			TaskStatus:   "completed",
			Progress:     100,
			Deliverable:  "Kịch bản chuyển thể hoàn hảo đạt 94/100 điểm cảm xúc",
			LastUpdated:  "15 phút trước",
		},

		// 6. Dept Studio (DEP-06) · Studio Sản Xuất Video & Âm Thanh AI
		{
			AgentID:      "agent-minh-thu",
			DeptID:       "dept-studio",
			RoleInDept:   "Chuyên Viên Phối Màu & Moodboard Visual",
			Rank:         "senior",
			StepOrder:    1,
			ReceivesFrom: "Kịch bản phân cảnh từ Khối Sáng Tạo Nội Dung",
			HandsOffTo:   "Đạo Diễn Hình Ảnh Đức Huy (Khâu 2)",
			CurrentTask:  "Thiết kế moodboard bảng màu pastel và storyboard ánh sáng macro cho sản phẩm serum",
			TaskStatus:   "working",
			Progress:     85,
			Deliverable:  "Bảng định hướng phong cách hình ảnh và Prompts Imagen 4K",
			LastUpdated:  "Vừa cập nhật",
		},
		{
			AgentID:      "agent-duc-huy",
			DeptID:       "dept-studio",
			RoleInDept:   "Trưởng Phòng Sản Xuất & Đạo Diễn Hình Ảnh 4K",
			Rank:         "lead",
			StepOrder:    2,
			ReceivesFrom: "Định hướng visual từ Minh Thư (Khâu 1)",
			HandsOffTo:   "Chuyên Viên Dàn Dựng Bếp Mẹ Bi (Khâu 3)",
			CurrentTask:  "Render 5 ảnh phân cảnh 4K chuẩn 9:16 bằng Gemini Imagen 3 cho video Tai nghe Sony",
			TaskStatus:   "working",
			Progress:     80,
			Deliverable:  "Bộ 5 ảnh bối cảnh 4K độ phân giải cao và Storyboard dàn dựng",
			LastUpdated:  "5 phút trước",
		},
		{
			AgentID:      "agent-bep-me-bi",
			DeptID:       "dept-studio",
			RoleInDept:   "Chuyên Viên Dàn Dựng & Cắt Cúp Nhịp Nhàng",
			Rank:         "specialist",
			StepOrder:    3,
			ReceivesFrom: "Bộ ảnh phân cảnh từ Đức Huy (Khâu 2)",
			HandsOffTo:   "Giám Sát Kỹ Thuật Sơn Bách (Khâu 4)",
			CurrentTask:  "Dàn dựng chuyển cảnh mượt mà, zoom macro texture sản phẩm",
			TaskStatus:   "completed",
			Progress:     100,
			Deliverable:  "Bản dựng video thô tỷ lệ 9:16 hoàn chỉnh",
			LastUpdated:  "12 phút trước",
		},
		{
			AgentID:      "agent-son-bach",
			DeptID:       "dept-studio",
			RoleInDept:   "Giám Sát Kỹ Thuật Frame & Độ Nét 4K",
			Rank:         "qc",
			StepOrder:    4,
			ReceivesFrom: "Bản dựng thô từ Bếp Mẹ Bi (Khâu 3)",
			HandsOffTo:   "Hội Đồng Thẩm Định Chất Lượng Độc Lập",
			CurrentTask:  "Kiểm tra độ sắc nét từng khung hình và độ đồng nhất gương mặt nhân vật AI",
			TaskStatus:   "completed",
			Progress:     96,
			Deliverable:  "Bảng đánh giá hình ảnh đạt chuẩn 96/100",
			LastUpdated:  "10 phút trước",
		},

		// 7. Dept Audio (DEP-07) · Hội Đồng Kiểm Định Chất Lượng Độc Lập
		{
			AgentID:      "agent-bep-me-bi",
			DeptID:       "dept-audio",
			RoleInDept:   "Chuyên Viên Đánh Giá Âm Lượng & Voiceover",
			Rank:         "specialist",
			StepOrder:    1,
			ReceivesFrom: "Lời thoại & Bản dựng từ Studio Sản Xuất Video",
			HandsOffTo:   "Chuyên Viên Kiểm Soát Tiêu Chuẩn Đức Huy (Khâu 2)",
			CurrentTask:  "Kiểm tra chất lượng giọng đọc VieNeu-TTS và tự động cân bằng âm lượng",
			TaskStatus:   "working",
			Progress:     90,
			Deliverable:  "Báo cáo thẩm định âm thanh sạch tạp âm, chuẩn độ vang phòng thu",
			LastUpdated:  "7 phút trước",
		},
		{
			AgentID:      "agent-duc-huy",
			DeptID:       "dept-audio",
			RoleInDept:   "Chuyên Viên Kiểm Soát Vùng An Toàn & Tỷ Lệ Khung Hình",
			Rank:         "specialist",
			StepOrder:    2,
			ReceivesFrom: "Báo cáo âm thanh từ Bếp Mẹ Bi (Khâu 1)",
			HandsOffTo:   "Giám Đốc Thẩm Định QC Sơn Bách (Khâu 3)",
			CurrentTask:  "Kiểm tra kích thước tỷ lệ khung hình 9:16 và vùng an toàn hiển thị cho icon TikTok/Reels",
			TaskStatus:   "working",
			Progress:     85,
			Deliverable:  "Báo cáo tuân thủ vùng an toàn hiển thị 100%",
			LastUpdated:  "14 phút trước",
		},
		{
			AgentID:      "agent-son-bach",
			DeptID:       "dept-audio",
			RoleInDept:   "Giám Đốc Thẩm Định Kiểm Định Chất Lượng & Tiêu Chuẩn Triệu View",
			Rank:         "lead",
			StepOrder:    3,
			ReceivesFrom: "Báo cáo vùng an toàn từ Đức Huy (Khâu 2)",
			HandsOffTo:   "Khối Phân Phối Đa Nền Tảng & Tối Ưu Chuyển Đổi",
			CurrentTask:  "Đo lường chỉ số Hook retention 3s và cấp chứng nhận kiểm định QC 96/100",
			TaskStatus:   "completed",
			Progress:     100,
			Deliverable:  "Chứng nhận kiểm định chất lượng video xuất sắc 96/100",
			LastUpdated:  "20 phút trước",
		},

		// 8. Dept Reup (DEP-08) · Khối Phân Phối Đa Nền Tảng & Tối Ưu Chuyển Đổi
		{
			AgentID:      "agent-minh-thu",
			DeptID:       "dept-reup",
			RoleInDept:   "Trưởng Phòng Phân Phối Kênh & Lịch Giờ Vàng",
			Rank:         "lead",
			StepOrder:    1,
			ReceivesFrom: "Video hoàn chỉnh và chứng nhận QC từ Hội Đồng Thẩm Định",
			HandsOffTo:   "Chuyên Viên Quét Bản Quyền ContentID Đức Huy (Khâu 2)",
			CurrentTask:  "Lên lịch phát hành đa nền tảng TikTok, Reels & Shorts giờ vàng",
			TaskStatus:   "working",
			Progress:     90,
			Deliverable:  "Lịch đăng tải đồng bộ đa nền tảng vào khung giờ vàng",
			LastUpdated:  "15 phút trước",
		},
		{
			AgentID:      "agent-duc-huy",
			DeptID:       "dept-reup",
			RoleInDept:   "Chuyên Viên Bản Quyền ContentID & Hashtag Trending",
			Rank:         "specialist",
			StepOrder:    2,
			ReceivesFrom: "Lịch phát hành từ Minh Thư (Khâu 1)",
			HandsOffTo:   "Chuyên Viên Phân Tích Tăng Trưởng Sơn Bách (Khâu 3)",
			CurrentTask:  "Quét đối chiếu thuật toán bản quyền âm thanh & tối ưu bộ hashtag trending",
			TaskStatus:   "working",
			Progress:     85,
			Deliverable:  "Chứng nhận an toàn bản quyền ContentID và bộ 8 hashtag viral",
			LastUpdated:  "18 phút trước",
		},
		{
			AgentID:      "agent-son-bach",
			DeptID:       "dept-reup",
			RoleInDept:   "Chuyên Viên Phân Tích Tương Tác & Tăng Trưởng View",
			Rank:         "senior",
			StepOrder:    3,
			ReceivesFrom: "Báo cáo tối ưu từ Đức Huy (Khâu 2)",
			HandsOffTo:   "Ban Giám Đốc & Dashboard Tác Chiến",
			CurrentTask:  "Theo dõi số lượt xem, tỷ lệ giữ chân và bình luận khán giả trên các kênh",
			TaskStatus:   "completed",
			Progress:     100,
			Deliverable:  "Báo cáo tăng trưởng view và chuyển đổi đơn hàng hàng ngày",
			LastUpdated:  "25 phút trước",
		},
		// =========================================================================
		// DEP-09: PHÒNG PHÂN TÍCH DỮ LIỆU, ĐO LƯỜNG & ĐỔI MỚI Ý TƯỞNG
		// =========================================================================
		{
			AgentID:      "agent-hoang-nam",
			DeptID:       "dept-analytics",
			RoleInDept:   "Trưởng Phòng Phân Tích & Đổi Mới Ý Tưởng Thuật Toán",
			Rank:         "lead",
			StepOrder:    1,
			ReceivesFrom: "Dữ liệu view, retention và chuyển đổi từ Phòng Quản Trị Kênh (DEP-08)",
			HandsOffTo:   "Phòng Tìm Kiếm Ý Tưởng (DEP-02) & Phòng Sáng Tạo (DEP-05)",
			CurrentTask:  "Mổ xẻ đồ thị rớt người xem (Retention Drop-off) và phát hiện mẫu hình video viral đột phá",
			TaskStatus:   "working",
			Progress:     92,
			Deliverable:  "Báo cáo tối ưu hóa thuật toán và khuyến nghị thay đổi 3 Hook mở đầu",
			LastUpdated:  "Vừa xong",
		},
		{
			AgentID:      "agent-growth-analyst",
			DeptID:       "dept-analytics",
			RoleInDept:   "Chuyên Viên Thử Nghiệm A/B Testing & Tối Ưu Chuyển Đổi",
			Rank:         "senior",
			StepOrder:    2,
			ReceivesFrom: "Đề xuất tối ưu từ Dr. Hoàng Nam (Khâu 1)",
			HandsOffTo:   "Phòng Kế Hoạch Sản Xuất (DEP-04) để nhân bản series",
			CurrentTask:  "Thử nghiệm A/B Testing giữa 2 định dạng kịch bản 'So sánh trực diện' vs 'Kể chuyện drama'",
			TaskStatus:   "working",
			Progress:     85,
			Deliverable:  "Kết quả A/B Test chứng minh định dạng so sánh trực diện tăng CTR +28%",
			LastUpdated:  "8 phút trước",
		},
	}
}

func (s *Server) ensureDeptTasksTable() error {
	if s.db == nil {
		return nil
	}
	agentMu.Lock()
	defer agentMu.Unlock()

	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_dept_tasks (
			agent_id TEXT NOT NULL,
			dept_id TEXT NOT NULL,
			role_in_dept TEXT NOT NULL,
			rank TEXT NOT NULL DEFAULT 'specialist',
			step_order INTEGER NOT NULL DEFAULT 1,
			receives_from TEXT NOT NULL DEFAULT '',
			hands_off_to TEXT NOT NULL DEFAULT '',
			current_task TEXT NOT NULL,
			task_status TEXT NOT NULL DEFAULT 'working',
			progress INTEGER NOT NULL DEFAULT 50,
			deliverable TEXT NOT NULL DEFAULT '',
			last_updated TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (agent_id, dept_id)
		);
	`)
	if err != nil {
		return err
	}

	// Auto-migrate new columns if table already existed
	_, _ = s.db.Exec(`ALTER TABLE ai_dept_tasks ADD COLUMN rank TEXT NOT NULL DEFAULT 'specialist'`)
	_, _ = s.db.Exec(`ALTER TABLE ai_dept_tasks ADD COLUMN step_order INTEGER NOT NULL DEFAULT 1`)
	_, _ = s.db.Exec(`ALTER TABLE ai_dept_tasks ADD COLUMN receives_from TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE ai_dept_tasks ADD COLUMN hands_off_to TEXT NOT NULL DEFAULT ''`)

	// Seed or update tasks for all departments
	for _, dt := range defaultDeptTasks() {
		_, _ = s.db.Exec(`
			INSERT OR REPLACE INTO ai_dept_tasks (agent_id, dept_id, role_in_dept, rank, step_order, receives_from, hands_off_to, current_task, task_status, progress, deliverable, last_updated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, dt.AgentID, dt.DeptID, dt.RoleInDept, dt.Rank, dt.StepOrder, dt.ReceivesFrom, dt.HandsOffTo, dt.CurrentTask, dt.TaskStatus, dt.Progress, dt.Deliverable, dt.LastUpdated)
	}
	return nil
}

func (s *Server) listDeptTasks() []AgentDeptTask {
	if err := s.ensureDeptTasksTable(); err == nil && s.db != nil {
		rows, err := s.db.Query(`
			SELECT agent_id, dept_id, role_in_dept, rank, step_order, receives_from, hands_off_to, current_task, task_status, progress, deliverable, last_updated
			FROM ai_dept_tasks
			ORDER BY dept_id ASC, step_order ASC
		`)
		if err == nil {
			defer rows.Close()
			var list []AgentDeptTask
			for rows.Next() {
				var dt AgentDeptTask
				if scanErr := rows.Scan(&dt.AgentID, &dt.DeptID, &dt.RoleInDept, &dt.Rank, &dt.StepOrder, &dt.ReceivesFrom, &dt.HandsOffTo, &dt.CurrentTask, &dt.TaskStatus, &dt.Progress, &dt.Deliverable, &dt.LastUpdated); scanErr == nil {
					list = append(list, dt)
				}
			}
			if len(list) > 0 {
				return list
			}
		}
	}

	agentMu.RLock()
	defer agentMu.RUnlock()
	if len(fallbackDeptTasks) == 0 {
		return defaultDeptTasks()
	}
	return fallbackDeptTasks
}

// listLiveOperations returns real-time activities across corporate departments,
// prioritizing real operations from AI Missions, Studio video jobs, and Ledger decisions.
func (s *Server) listLiveOperations() []LiveOperationEvent {
	var events []LiveOperationEvent

	// 1. Collect real operations from AI Missions
	missions := s.listAIMissions(4)
	for _, m := range missions {
		deptCode := "DEP-01"
		deptName := "Ban Điều Hành"
		agentName := "Alex Vũ"
		avatar := "🎯"
		tag := "CHIẾN DỊCH AI"
		badge := "badge-ok"
		if m.Status == "running" {
			badge = "badge-info"
		} else if m.Status == "failed" {
			badge = "badge-err"
		}

		switch m.Intent {
		case "affiliate_campaign":
			deptCode = "DEP-03"
			deptName = "Thẩm Định Deal"
			agentName = "Elena Bùi"
			avatar = "⚖️"
			tag = "AFFILIATE DEAL"
		case "story_campaign":
			deptCode = "DEP-04"
			deptName = "Kể Chuyện"
			agentName = "Lâm Phong"
			avatar = "📖"
			tag = "KHO TRUYỆN"
		case "trend_campaign":
			deptCode = "DEP-02"
			deptName = "Săn Xu Hướng"
			agentName = "Radar Kai"
			avatar = "📡"
			tag = "XU HƯỚNG NÓNG"
		case "growth_campaign":
			deptCode = "DEP-09"
			deptName = "Phân Tích & Tăng Trưởng"
			agentName = "Dr. Hoàng Nam"
			avatar = "📊"
			tag = "MA TRẬN 30 NGÀY"
		}

		tStr := m.CreatedAt
		if len(tStr) >= 16 {
			tStr = tStr[11:16]
		}
		events = append(events, LiveOperationEvent{
			Time:        tStr,
			DeptCode:    deptCode,
			DeptName:    deptName,
			AgentName:   agentName,
			AgentAvatar: avatar,
			Action:      "Thực Thi: " + m.Goal,
			Detail:      m.ResultSummary,
			Tag:         tag,
			BadgeClass:  badge,
		})
	}

	// 2. Collect real operations from Studio Jobs
	if s.Studio != nil {
		jobs := s.Studio.ListJobs(4)
		for _, j := range jobs {
			tStr := j.CreatedAt
			if len(tStr) >= 16 {
				tStr = tStr[11:16]
			}
			badge := "badge-info"
			actionText := "Đang Render Video"
			if j.Status == "done" {
				badge = "badge-ok"
				actionText = "Hoàn Tất Video"
			} else if j.Status == "failed" {
				badge = "badge-err"
				actionText = "Render Thất Bại"
			}
			captionPreview := j.Caption
			if len(captionPreview) > 80 {
				captionPreview = captionPreview[:80] + "..."
			}
			events = append(events, LiveOperationEvent{
				Time:        tStr,
				DeptCode:    "DEP-06",
				DeptName:    "Studio Sản Xuất Video",
				AgentName:   "Đức Huy",
				AgentAvatar: "🎬",
				Action:      fmt.Sprintf("%s: %s", actionText, j.Title),
				Detail:      fmt.Sprintf("Tiến độ: %d%% · %s", j.Progress, captionPreview),
				Tag:         strings.ToUpper(j.Status),
				BadgeClass:  badge,
			})
		}
	}

	// 3. Collect real operations from Decisions
	if s.Ledger != nil {
		decs, err := s.queryDecisions("SELECT id, agent, action, target, reason, created_at FROM decisions WHERE action != 'onboard' ORDER BY id DESC LIMIT 3")
		if err == nil {
			for _, d := range decs {
				tStr := d.CreatedAt
				if len(tStr) >= 16 {
					tStr = tStr[11:16]
				}
				events = append(events, LiveOperationEvent{
					Time:        tStr,
					DeptCode:    "DEP-01",
					DeptName:    "Ban Điều Hành & Tuân Thủ",
					AgentName:   d.Agent,
					AgentAvatar: "🛡️",
					Action:      "Quyết Định: " + d.Action + " ➔ " + d.Target,
					Detail:      d.Reason,
					Tag:         "QUYẾT ĐỊNH",
					BadgeClass:  "badge-draft",
				})
			}
		}
	}

	if len(events) > 0 {
		if len(events) > 8 {
			events = events[:8]
		}
		return events
	}

	return baselineOperationEvents()
}

func baselineOperationEvents() []LiveOperationEvent {
	return []LiveOperationEvent{
		{
			Time:        "16:28",
			DeptCode:    "DEP-09",
			DeptName:    "Phân Tích & Đổi Mới",
			AgentName:   "Dr. Hoàng Nam",
			AgentAvatar: "📊",
			Action:      "Sẵn Sàng Phân Tích Mẫu Video Đột Phá",
			Detail:      "Hệ thống đo lường đồ thị giữ chân và cấu trúc Hook 3s · Tự động nhân bản biến thể hiệu quả",
			Tag:         "SẴN SÀNG",
			BadgeClass:  "badge-ok",
		},
		{
			Time:        "16:24",
			DeptCode:    "DEP-02",
			DeptName:    "Săn Ý Tưởng Viral",
			AgentName:   "Radar Kai",
			AgentAvatar: "📡",
			Action:      "Bắt Tín Hiệu Âm Thanh Xu Hướng TikTok VN",
			Detail:      "Quét bảng xếp hạng Kworb TikTok VN định kỳ 6h · Phân tích tốc độ tăng trưởng và bóc tách cấu trúc 15s",
			Tag:         "RADAR XU HƯỚNG",
			BadgeClass:  "badge-ok",
		},
		{
			Time:        "16:20",
			DeptCode:    "DEP-03",
			DeptName:    "Thẩm Định Deal",
			AgentName:   "Elena Bùi",
			AgentAvatar: "⚖️",
			Action:      "Thẩm Định Deal Hoa Hồng Cao Sàn Accesstrade",
			Detail:      "Lọc chiến dịch Shopee/Lazada/Tiki hoa hồng >15% · Tỷ lệ duyệt >= 85đ · Tự động gắn tracking link",
			Tag:         "ACCESSTRADE",
			BadgeClass:  "badge-ok",
		},
		{
			Time:        "16:15",
			DeptCode:    "DEP-05",
			DeptName:    "Nội Dung & Kịch Bản",
			AgentName:   "Minh Thư",
			AgentAvatar: "✍️",
			Action:      "Quy Trình Soạn Kịch Bản 4 Hook 3s Nguyên Bản",
			Detail:      "Áp dụng nguyên tắc Brand DNA của kênh · Cấu trúc tâm lý 3 hồi · Tuyệt đối không reup vi phạm",
			Tag:         "CHUẨN NGUYÊN BẢN",
			BadgeClass:  "badge-info",
		},
		{
			Time:        "16:10",
			DeptCode:    "DEP-06",
			DeptName:    "Studio Sản Xuất Video",
			AgentName:   "Đức Huy",
			AgentAvatar: "🎬",
			Action:      "Dây Chuyền Render Video 9:16 & 16:9 Tự Động",
			Detail:      "Ken Burns chuyển động mượt mà · Chuẩn âm thanh -14 LUFS · Xuất gói CapCut 1-click",
			Tag:         "STUDIO AI",
			BadgeClass:  "badge-draft",
		},
		{
			Time:        "16:05",
			DeptCode:    "DEP-07",
			DeptName:    "Kiểm Soát Chất Lượng (QC)",
			AgentName:   "Sơn Bách",
			AgentAvatar: "🛡️",
			Action:      "Gác Cổng 10 Tiêu Chí Chất Lượng Video",
			Detail:      "Kiểm tra vùng an toàn 9:16 · Không lỗi âm thanh · 0 từ cấm chính sách nền tảng",
			Tag:         "QC 10 TIÊU CHÍ",
			BadgeClass:  "badge-ok",
		},
		{
			Time:        "15:58",
			DeptCode:    "DEP-08",
			DeptName:    "Quản Trị Kênh Social",
			AgentName:   "Marcus Lê",
			AgentAvatar: "📱",
			Action:      "Lên Lịch Xuất Bản Khung Giờ Vàng Đa Nền Tảng",
			Detail:      "Hẹn giờ đăng 11:30 & 19:30 trên TikTok, YouTube Shorts & Facebook Reels",
			Tag:         "KHUNG GIỜ VÀNG",
			BadgeClass:  "badge-info",
		},
		{
			Time:        "15:45",
			DeptCode:    "DEP-01",
			DeptName:    "Ban Điều Hành",
			AgentName:   "Alex Vũ",
			AgentAvatar: "🏛️",
			Action:      "Kiểm Soát An Toàn & Quota API 24/7",
			Detail:      "Xoay key Gemini tự động · Tự ngắt mạch khi quá tải · Master Switch bảo vệ hệ thống",
			Tag:         "AN TOÀN HỆ THỐNG",
			BadgeClass:  "badge-ok",
		},
	}
}

func rankMeta(rank string) (int, string) {
	switch rank {
	case "lead":
		return 1, "👑 Cấp 1: Ban Điều Hành / Trưởng Khối"
	case "senior":
		return 2, "💎 Cấp 2: Chuyên Viên Cao Cấp (Senior)"
	case "specialist":
		return 3, "⭐ Cấp 3: Chuyên Viên Thực Thi"
	case "qc":
		return 4, "🛡️ Cấp 4: Thẩm Định QC & Kiểm Soát"
	default:
		return 3, "⭐ Cấp 3: Chuyên Viên Thực Thi"
	}
}

func (s *Server) listDepartments(agents []AIAgent) []AIDepartment {
	depts := []AIDepartment{
		{
			ID:          "dept-exec",
			Code:        "DEP-01",
			Tier:        1,
			TierName:    "Cấp 1 · Ban Điều Hành & Quản Trị Chiến Lược",
			Name:        "Ban Điều Hành & Quản Trị Chiến Lược",
			LeaderName:  "Alex Vũ (Giám Đốc Vận Hành & Điều Phối)",
			Icon:        "🏛️",
			Workflow:    "Phân Bổ Tài Nguyên GPU/API ➔ Kiểm Soát Ngân Sách ➔ Quản Trị Rủi Ro & Kill Switch ➔ Cấp Phép Chiến Dịch",
			Description: "Bộ não chỉ huy toàn công ty, phân bổ ngân sách sản xuất, giám sát chi phí token API, bảo vệ an toàn kênh và kích hoạt chỉ thị chiến lược.",
			ActionLink:  "/settings",
			ActionLabel: "Trung Tâm Điều Hành 🏛️",
			SLA:         "< 2s điều phối",
			KPI:         "99.9% Uptime · 0 vi phạm chính sách",
			Throughput:  "24/7 Không ngừng nghỉ",
			Active247:   true,
		},
		{
			ID:          "dept-intelligence",
			Code:        "DEP-02",
			Tier:        2,
			TierName:    "Cấp 2 · Phòng Tìm Kiếm & Thu Thập Ý Tưởng Viral",
			Name:        "Phòng Tìm Kiếm & Thu Thập Ý Tưởng Viral",
			LeaderName:  "Radar Kai (Trưởng Ban Săn Xu Hướng Viral)",
			Icon:        "📡",
			Workflow:    "Quét Radar 24/7 Douyin/TikTok ➔ Bóc Tách Audio Thịnh Hành ➔ Phân Tích Cấu Trúc Hook Triệu View ➔ Chuyển Giao Phòng Đánh Giá",
			Description: "Trinh sát thị trường hoạt động 24/7, phát hiện sớm các xu hướng bùng nổ, bóc tách âm thanh hot và cấu trúc nội dung viral trên các nền tảng.",
			ActionLink:  "/studio/trends",
			ActionLabel: "Radar Xu Hướng 📡",
			SLA:         "15 phút / chu kỳ quét",
			KPI:         "Phát hiện trend trước 24h · 92% audio hot",
			Throughput:  "120 tín hiệu viral / ngày",
			Active247:   true,
		},
		{
			ID:          "dept-affiliate",
			Code:        "DEP-03",
			Tier:        3,
			TierName:    "Cấp 3 · Phòng Thẩm Định & Đánh Giá Ý Tưởng / Sản Phẩm",
			Name:        "Phòng Thẩm Định & Đánh Giá Ý Tưởng / Sản Phẩm",
			LeaderName:  "Elena Bùi (Trưởng Phòng Thẩm Định & Săn Deal)",
			Icon:        "⚖️",
			Workflow:    "Chấm Điểm Viral Score (0-100) ➔ Thẩm Định Tính Khả Thi ➔ Lọc Deal Hoa Hồng Ròng >15% ➔ Duyệt Ý Tưởng >= 85 Điểm",
			Description: "Bộ lọc tử thần đánh giá tiềm năng lan tỏa của ý tưởng, kiểm tra tính khả thi chuyển đổi và thẩm định chất lượng deal trước khi đưa vào sản xuất.",
			ActionLink:  "/products",
			ActionLabel: "Kệ Hàng Săn Deal ⚖️",
			SLA:         "< 30s thẩm định deal",
			KPI:         "Hoa hồng > 15% · Tỷ lệ duyệt >= 85đ",
			Throughput:  "50 deal & ý tưởng được lọc / ngày",
			Active247:   true,
		},
		{
			ID:          "dept-story",
			Code:        "DEP-04",
			Tier:        4,
			TierName:    "Cấp 4 · Phòng Kế Hoạch & Điều Phối Sản Xuất",
			Name:        "Phòng Kế Hoạch & Điều Phối Sản Xuất",
			LeaderName:  "Daniel Trần (Trưởng Ban Kế Hoạch & Nội Dung)",
			Icon:        "📅",
			Workflow:    "Ma Trận Nội Dung 30 Ngày (60-30-10) ➔ Phân Luồng Kênh Theo Tệp Khán Giả ➔ Cân Đối Giờ Vàng Phát Sóng ➔ Cấp Lệnh Sản Xuất",
			Description: "Ban biên tập chiến lược, phân bổ ma trận nội dung theo tỷ lệ vàng (60% viral kéo phễu, 30% giữ chân, 10% bán hàng) và khóa slot giờ vàng.",
			ActionLink:  "/growth",
			ActionLabel: "Lịch Biên Tập & Kênh 📅",
			SLA:         "Lập kế hoạch 30 ngày tự động",
			KPI:         "0% trùng lặp nội dung · 100% khớp giờ vàng",
			Throughput:  "30 ngày kế hoạch / kênh",
			Active247:   true,
		},
		{
			ID:          "dept-creative",
			Code:        "DEP-05",
			Tier:        5,
			TierName:    "Cấp 5 · Phòng Nội Dung & Sáng Tạo Kịch Bản Triệu View",
			Name:        "Phòng Nội Dung & Sáng Tạo Kịch Bản Triệu View",
			LeaderName:  "Minh Thư (Trưởng Ban Biên Kịch Triệu View)",
			Icon:        "✍️",
			Workflow:    "Thiết Kế Hook 3s Giữ Chân ➔ Xây Dựng Cốt Truyện 3 Hồi & Open Loops ➔ Bản Địa Hóa Kịch Bản Viral ➔ Soạn Prompt Hình Ảnh Chi Tiết",
			Description: "Phòng sáng tạo kịch bản đỉnh cao, ứng dụng tâm lý học hành vi giữ chân khán giả, bản địa hóa nội dung tự nhiên và tạo câu thoại đắt giá.",
			ActionLink:  "/stories",
			ActionLabel: "Kho Kịch Bản & Cốt Truyện ✍️",
			SLA:         "< 18s / kịch bản 5 cảnh",
			KPI:         "Tỷ lệ giữ chân 3s > 84% · Điểm QC kịch bản > 90",
			Throughput:  "45 kịch bản / ngày",
			Active247:   true,
		},
		{
			ID:          "dept-studio",
			Code:        "DEP-06",
			Tier:        6,
			TierName:    "Cấp 6 · Studio Sản Xuất Video & Âm Thanh AI",
			Name:        "Studio Sản Xuất Video & Âm Thanh AI",
			LeaderName:  "Đức Huy (Giám Đốc Kỹ Thuật Studio & Dựng Video)",
			Icon:        "🎬",
			Workflow:    "Khóa Nhân Vật & Visual 4K Imagen 3 ➔ Thu Âm Giọng Đọc AI -14 LUFS ➔ Chuyển Cảnh, Bắt Nhịp Nhạc & Phụ Đề Động ➔ Xuất Video Hoàn Chỉnh",
			Description: "Bộ phận sản xuất tự động 24/7, biến kịch bản thành video hoàn chỉnh chuẩn 9:16 chất lượng điện ảnh, âm thanh chuẩn hóa và hiệu ứng bắt mắt.",
			ActionLink:  "/studio",
			ActionLabel: "Studio AI 🎬",
			SLA:         "< 45s / video render 4K",
			KPI:         "100% đồng nhất nhân vật · Tự động hạ nhạc nền -14 LUFS",
			Throughput:  "60 video hoàn thiện / ngày",
			Active247:   true,
		},
		{
			ID:          "dept-audio",
			Code:        "DEP-07",
			Tier:        7,
			TierName:    "Cấp 7 · Phòng Kiểm Soát Chất Lượng Độc Lập & Pháp Lý Bản Quyền",
			Name:        "Phòng Kiểm Soát Chất Lượng Độc Lập & Pháp Lý Bản Quyền",
			LeaderName:  "Sơn Bách (Trưởng Ban Kiểm Soát Chất Lượng QC)",
			Icon:        "🛡️",
			Workflow:    "Soi Từng Khung Hình Vùng An Toàn ➔ Kiểm Tra Khẩu Hình & Tần Số Âm ➔ Quét Từ Cấm Chính Sách Nền Tảng ➔ Cấp Chứng Nhận Đạt Chuẩn QC",
			Description: "Hội đồng thẩm định độc lập gác cổng phát hành, chấm điểm khách quan không thiên vị. Đảm bảo mọi video xuất bản đều đạt chuẩn triệu view và an toàn 100%.",
			ActionLink:  "/studio",
			ActionLabel: "Thẩm Định Chất Lượng QC 🛡️",
			SLA:         "< 5s / video thẩm định",
			KPI:         "100% video xuất bản đạt chuẩn >= 85 điểm",
			Throughput:  "100% video qua cửa kiểm định",
			Active247:   true,
		},
		{
			ID:          "dept-reup",
			Code:        "DEP-08",
			Tier:        8,
			TierName:    "Cấp 8 · Phòng Quản Trị Mạng Lưới Kênh & Tăng Trưởng Social (Growth Marketing)",
			Name:        "Phòng Quản Trị Mạng Lưới Kênh & Tăng Trưởng Social (Growth Marketing)",
			LeaderName:  "Marcus Lê (Trưởng Khối Tăng Trưởng & Phân Phối Đa Kênh)",
			Icon:        "📱",
			Workflow:    "Tiếp Nhận Video Đạt QC ➔ Tối Ưu Phễu Chuyển Đổi & Ghim Giỏ Hàng ➔ Phân Phối Đa Kênh Giờ Vàng ➔ Quản Trị Tương Tác & Tăng Trưởng Kênh",
			Description: "Khối tiếp thị và tăng trưởng đa nền tảng (TikTok, Facebook Reels, YouTube Shorts). Tự động điều phối lịch đăng, tối ưu hóa tỷ lệ chuyển đổi link bio/affiliate, và mở rộng quy mô mạng lưới kênh 24/7.",
			ActionLink:  "/growth",
			ActionLabel: "Quản Trị Kênh & Tăng Trưởng 📱",
			SLA:         "Khớp 100% slot giờ vàng",
			KPI:         "Tỷ lệ nhấp giỏ hàng CTR > 4.5% · An toàn kênh 100%",
			Throughput:  "Phân phối tới toàn bộ mạng lưới kênh",
			Active247:   true,
		},
		{
			ID:          "dept-analytics",
			Code:        "DEP-09",
			Tier:        9,
			TierName:    "Cấp 9 · Phòng Phân Tích Dữ Liệu, Đo Lường & Đổi Mới Ý Tưởng",
			Name:        "Phòng Phân Tích Dữ Liệu, Đo Lường & Đổi Mới Ý Tưởng",
			LeaderName:  "Dr. Hoàng Nam (Trưởng Ban Phân Tích & Đổi Mới Ý Tưởng)",
			Icon:        "📊",
			Workflow:    "Đo Lường View/CTR/Retention ➔ Mổ Xẻ Đồ Thị Rớt Người Xem ➔ Phát Hiện Format Đột Phá ➔ Phản Hồi Ngược Về Phòng Ý Tưởng & Kịch Bản",
			Description: "Bộ não tối ưu hóa thuật toán 24/7. Đo lường tỷ lệ giữ chân, phân tích doanh thu affiliate và tạo vòng lặp phản hồi thông minh giúp video sau luôn tốt hơn video trước.",
			ActionLink:  "/growth",
			ActionLabel: "Báo Cáo Tăng Trưởng & Đổi Mới 📊",
			SLA:         "< 10 phút sau khi có số liệu",
			KPI:         "Tỷ lệ giữ chân tăng +20% · 100% video đột phá được nhân bản",
			Throughput:  "Phân tích liên tục 24/7 mọi video xuất bản",
			Active247:   true,
		},
	}

	tasks := s.listDeptTasks()
	agentMap := make(map[string]AIAgent)
	for _, ag := range agents {
		agentMap[ag.ID] = ag
	}

	tasksByDept := make(map[string][]AgentDeptTask)
	agentDeptBadges := make(map[string][]string)

	for _, t := range tasks {
		tasksByDept[t.DeptID] = append(tasksByDept[t.DeptID], t)
		var deptName string
		switch t.DeptID {
		case "dept-exec":
			deptName = "🏛️ Ban Điều Hành"
		case "dept-intelligence":
			deptName = "📡 Săn Ý Tưởng Viral"
		case "dept-affiliate":
			deptName = "⚖️ Đánh Giá Ý Tưởng & Deal"
		case "dept-story":
			deptName = "📅 Kế Hoạch Sản Xuất"
		case "dept-creative":
			deptName = "✍️ Nội Dung & Kịch Bản"
		case "dept-studio":
			deptName = "🎬 Studio Sản Xuất Video"
		case "dept-audio":
			deptName = "🛡️ Kiểm Soát QC"
		case "dept-reup":
			deptName = "📱 Quản Trị Kênh Social"
		case "dept-analytics":
			deptName = "📊 Phân Tích & Đổi Mới"
		default:
			deptName = "🏢 " + t.DeptID
		}
		hasBadge := false
		for _, b := range agentDeptBadges[t.AgentID] {
			if b == deptName {
				hasBadge = true
				break
			}
		}
		if !hasBadge {
			agentDeptBadges[t.AgentID] = append(agentDeptBadges[t.AgentID], deptName)
		}
	}

	for i := range depts {
		dTasks := tasksByDept[depts[i].ID]
		var tier1Leads, tier2Seniors, tier3Specialists, tier4QC []DeptMember

		for _, dt := range dTasks {
			ag, ok := agentMap[dt.AgentID]
			if !ok {
				continue
			}
			tNum, rLabel := rankMeta(dt.Rank)
			member := DeptMember{
				Agent:        ag,
				RoleInDept:   dt.RoleInDept,
				Rank:         dt.Rank,
				RankLabel:    rLabel,
				RankTierNum:  tNum,
				StepOrder:    dt.StepOrder,
				ReceivesFrom: dt.ReceivesFrom,
				HandsOffTo:   dt.HandsOffTo,
				CurrentTask:  dt.CurrentTask,
				TaskStatus:   dt.TaskStatus,
				Progress:     dt.Progress,
				Deliverable:  dt.Deliverable,
				LastUpdated:  dt.LastUpdated,
			}
			depts[i].Members = append(depts[i].Members, member)

			switch dt.Rank {
			case "lead":
				tier1Leads = append(tier1Leads, member)
			case "senior":
				tier2Seniors = append(tier2Seniors, member)
			case "qc":
				tier4QC = append(tier4QC, member)
			default:
				tier3Specialists = append(tier3Specialists, member)
			}
		}

		// Ensure at least one lead exists
		if len(tier1Leads) == 0 && len(depts[i].Members) > 0 {
			first := depts[i].Members[0]
			first.Rank = "lead"
			first.RankTierNum, first.RankLabel = rankMeta("lead")
			tier1Leads = append(tier1Leads, first)
			if len(tier2Seniors) > 0 && tier2Seniors[0].Agent.ID == first.Agent.ID {
				tier2Seniors = tier2Seniors[1:]
			} else if len(tier3Specialists) > 0 && tier3Specialists[0].Agent.ID == first.Agent.ID {
				tier3Specialists = tier3Specialists[1:]
			}
		}

		// Backward compatibility pointers
		if len(tier1Leads) > 0 {
			mCopy := tier1Leads[0]
			depts[i].Lead = &mCopy
		}
		if len(tier4QC) > 0 {
			mCopy := tier4QC[0]
			depts[i].QC = &mCopy
		}
		depts[i].Specialists = append(tier2Seniors, tier3Specialists...)

		// Build structured RankTiers for multi-tier org tree
		if len(tier1Leads) > 0 {
			depts[i].RankTiers = append(depts[i].RankTiers, DeptRankTier{
				TierNumber: 1,
				TierTitle:  "👑 BAN ĐIỀU HÀNH & TRƯỞNG KHỐI (CẤP 1)",
				BadgeClass: "lead",
				Members:    tier1Leads,
			})
		}
		if len(tier2Seniors) > 0 {
			depts[i].RankTiers = append(depts[i].RankTiers, DeptRankTier{
				TierNumber: 2,
				TierTitle:  "💎 CHUYÊN VIÊN CAO CẤP (SENIOR - CẤP 2)",
				BadgeClass: "senior",
				Members:    tier2Seniors,
			})
		}
		if len(tier3Specialists) > 0 {
			depts[i].RankTiers = append(depts[i].RankTiers, DeptRankTier{
				TierNumber: 3,
				TierTitle:  "⭐ CHUYÊN VIÊN THỰC THI (SPECIALIST - CẤP 3)",
				BadgeClass: "specialist",
				Members:    tier3Specialists,
			})
		}
		if len(tier4QC) > 0 {
			depts[i].RankTiers = append(depts[i].RankTiers, DeptRankTier{
				TierNumber: 4,
				TierTitle:  "🛡️ THẨM ĐỊNH QC & AN TOÀN NỀN TẢNG (CẤP 4)",
				BadgeClass: "qc",
				Members:    tier4QC,
			})
		}

		// Build network diagram topology and data links
		buildDepartmentNetworkTopology(&depts[i])
	}

	// Update departments slice in agents
	for i := range agents {
		if badges, ok := agentDeptBadges[agents[i].ID]; ok {
			agents[i].Departments = badges
		}
	}

	return depts
}

// buildDepartmentNetworkTopology calculates coordinates (X, Y) for compact employee cards
// and SVG paths for animated data transmission packets between agents on an 860x380 canvas.
func buildDepartmentNetworkTopology(dept *AIDepartment) {
	n := len(dept.Members)
	if n == 0 {
		return
	}

	if dept.ID == "dept-studio" && n >= 4 {
		// Specific 4-node diamond network for Studio
		for i := range dept.Members {
			switch dept.Members[i].StepOrder {
			case 1: // Minh Thư (Scripting / Ingestion)
				dept.Members[i].PosX = 145
				dept.Members[i].PosY = 190
			case 2: // Đức Huy (Lead / Visual Director)
				dept.Members[i].PosX = 430
				dept.Members[i].PosY = 68
			case 3: // Bếp Mẹ Bi (Audio / Video Editor)
				dept.Members[i].PosX = 430
				dept.Members[i].PosY = 312
			case 4: // Sơn Bách (QC Gateway)
				dept.Members[i].PosX = 715
				dept.Members[i].PosY = 190
			default:
				dept.Members[i].PosX = 430
				dept.Members[i].PosY = 190
			}
		}
		dept.Links = []NetworkLink{
			{ID: "nl-1", Path: "M 145,190 Q 270,95 430,68", Label: "Kịch bản 5 cảnh & Prompts", Color: "#38bdf8", Duration: "2.2s"},
			{ID: "nl-2", Path: "M 145,190 Q 270,285 430,312", Label: "Lời thoại Voiceover", Color: "#818cf8", Duration: "2.8s"},
			{ID: "nl-3", Path: "M 430,68 L 430,312", Label: "Trục dựng: Visual Imagen 4K", Color: "#3b82f6", Duration: "2.0s"},
			{ID: "nl-4", Path: "M 430,312 Q 590,285 715,190", Label: "Bản dựng video hoàn chỉnh", Color: "#10b981", Duration: "2.4s"},
			{ID: "nl-5", Path: "M 430,68 Q 590,95 715,190", Label: "Tiêu chuẩn kiểm định", Color: "#06b6d4", Duration: "2.6s"},
			{ID: "nl-6", Path: "M 715,190 Q 570,160 430,68", Label: "Nghiệm thu QC 96/100", Color: "#10b981", Duration: "3.2s"},
		}
		return
	}

	if n >= 3 {
		// 3-node triangular network
		for i := range dept.Members {
			switch dept.Members[i].StepOrder {
			case 1: // Lead Node (Top)
				dept.Members[i].PosX = 430
				dept.Members[i].PosY = 75
			case 2: // Specialist 1 (Bottom Left)
				dept.Members[i].PosX = 210
				dept.Members[i].PosY = 295
			case 3: // Specialist 2 (Bottom Right)
				dept.Members[i].PosX = 650
				dept.Members[i].PosY = 295
			default:
				dept.Members[i].PosX = 430
				dept.Members[i].PosY = 190
			}
		}
		dept.Links = []NetworkLink{
			{ID: "nl-1", Path: "M 430,75 Q 300,165 210,295", Label: "Chỉ đạo & Bàn giao tác vụ", Color: "#f59e0b", Duration: "2.2s"},
			{ID: "nl-2", Path: "M 210,295 L 650,295", Label: "Chuyển giao thành phẩm chuyên môn", Color: "#38bdf8", Duration: "2.0s"},
			{ID: "nl-3", Path: "M 650,295 Q 560,165 430,75", Label: "Nghiệm thu & Hoàn tất khâu", Color: "#10b981", Duration: "2.4s"},
		}
		return
	}

	if n == 2 {
		dept.Members[0].PosX = 230
		dept.Members[0].PosY = 190
		dept.Members[1].PosX = 630
		dept.Members[1].PosY = 190

		label1 := "Bàn giao tác vụ & tài nguyên"
		if dept.Members[0].Deliverable != "" {
			label1 = dept.Members[0].Deliverable
		}
		dept.Links = []NetworkLink{
			{ID: "nl-1", Path: "M 230,170 Q 430,110 630,170", Label: label1, Color: "#38bdf8", Duration: "2.2s"},
			{ID: "nl-2", Path: "M 630,210 Q 430,270 230,210", Label: "Nghiệm thu & Phản hồi", Color: "#10b981", Duration: "2.5s"},
		}
		return
	}

	// Generic topology for any arbitrary member count
	for i := range dept.Members {
		theta := float64(i)*2*math.Pi/float64(n) - math.Pi/2
		dept.Members[i].PosX = 430 + int(260*math.Cos(theta))
		dept.Members[i].PosY = 190 + int(115*math.Sin(theta))
	}
	for i := range dept.Members {
		next := (i + 1) % n
		p := fmt.Sprintf("M %d,%d L %d,%d", dept.Members[i].PosX, dept.Members[i].PosY, dept.Members[next].PosX, dept.Members[next].PosY)
		lbl := "Trao đổi dữ liệu"
		if dept.Members[i].Deliverable != "" {
			lbl = dept.Members[i].Deliverable
		}
		dept.Links = append(dept.Links, NetworkLink{
			ID:       fmt.Sprintf("nl-%d", i),
			Path:     p,
			Label:    lbl,
			Color:    "#38bdf8",
			Duration: "2.4s",
		})
	}
}

func (s *Server) handleDeptAssign(w http.ResponseWriter, r *http.Request) {
	agentID := strings.TrimSpace(r.FormValue("agent_id"))
	deptID := strings.TrimSpace(r.FormValue("dept_id"))
	role := strings.TrimSpace(r.FormValue("role_in_dept"))
	rank := strings.TrimSpace(r.FormValue("rank"))
	if rank == "" {
		rank = "specialist"
	}
	task := strings.TrimSpace(r.FormValue("current_task"))
	receivesFrom := strings.TrimSpace(r.FormValue("receives_from"))
	handsOffTo := strings.TrimSpace(r.FormValue("hands_off_to"))
	deliverable := strings.TrimSpace(r.FormValue("deliverable"))

	if agentID == "" || deptID == "" {
		seeOther(w, r, "/team?err="+url.QueryEscape("Vui lòng chọn nhân sự và phòng ban"))
		return
	}
	if role == "" {
		role = "Chuyên viên thực thi quy trình"
	}
	if task == "" {
		task = "Đang nhận nhiệm vụ mới trong quy trình phòng ban"
	}
	if deliverable == "" {
		deliverable = "Báo cáo tiến độ và kết quả công việc"
	}

	if s.db != nil {
		_ = s.ensureDeptTasksTable()
		_, _ = s.db.Exec(`
			INSERT INTO ai_dept_tasks (agent_id, dept_id, role_in_dept, rank, step_order, receives_from, hands_off_to, current_task, task_status, progress, deliverable, last_updated)
			VALUES (?, ?, ?, ?, 9, ?, ?, ?, 'working', 50, ?, ?)
			ON CONFLICT(agent_id, dept_id) DO UPDATE SET
				role_in_dept = excluded.role_in_dept,
				rank = excluded.rank,
				receives_from = excluded.receives_from,
				hands_off_to = excluded.hands_off_to,
				current_task = excluded.current_task,
				deliverable = excluded.deliverable,
				last_updated = excluded.last_updated
		`, agentID, deptID, role, rank, receivesFrom, handsOffTo, task, deliverable, "Vừa cập nhật")
	} else {
		agentMu.Lock()
		if len(fallbackDeptTasks) == 0 {
			fallbackDeptTasks = defaultDeptTasks()
		}
		found := false
		for i := range fallbackDeptTasks {
			if fallbackDeptTasks[i].AgentID == agentID && fallbackDeptTasks[i].DeptID == deptID {
				fallbackDeptTasks[i].RoleInDept = role
				fallbackDeptTasks[i].Rank = rank
				fallbackDeptTasks[i].ReceivesFrom = receivesFrom
				fallbackDeptTasks[i].HandsOffTo = handsOffTo
				fallbackDeptTasks[i].CurrentTask = task
				fallbackDeptTasks[i].Deliverable = deliverable
				fallbackDeptTasks[i].LastUpdated = "Vừa cập nhật"
				found = true
				break
			}
		}
		if !found {
			fallbackDeptTasks = append(fallbackDeptTasks, AgentDeptTask{
				AgentID:      agentID,
				DeptID:       deptID,
				RoleInDept:   role,
				Rank:         rank,
				StepOrder:    9,
				ReceivesFrom: receivesFrom,
				HandsOffTo:   handsOffTo,
				CurrentTask:  task,
				TaskStatus:   "working",
				Progress:     50,
				Deliverable:  deliverable,
				LastUpdated:  "Vừa cập nhật",
			})
		}
		agentMu.Unlock()
	}

	seeOther(w, r, "/team?view=departments&dept="+deptID+"&ok="+url.QueryEscape("Đã liên kết nhân sự vào phòng ban thành công."))
}
