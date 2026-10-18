package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// ---------------------------------------------------------------------------
// Studio page: AI video creation (affiliate) + VN trending sounds
// ---------------------------------------------------------------------------

// studioJobView is the template/JSON projection of a studio job.
// Label/Class are rendered server-side so clients never redefine status
// labels (R2-W3: một nguồn nhãn duy nhất).
type studioJobView struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Title      string         `json:"title"`
	Status     string         `json:"status"`
	Progress   int            `json:"progress"`
	Output     string         `json:"output"`
	CreatedAt  string         `json:"created_at"`
	FinishedAt string         `json:"finished_at"`
	KindLabel  string         `json:"kind_label"`
	Label      string         `json:"label"`
	Class      string         `json:"class"`
	DeptStages []JobDeptStage `json:"dept_stages"`
	DeptGroups []JobDeptGroup `json:"dept_groups"`
}

// JobDeptGroup represents a specialized department group participating in a job.
type JobDeptGroup struct {
	Step        int            `json:"step"`
	DeptCode    string         `json:"dept_code"`
	DeptName    string         `json:"dept_name"`
	Icon        string         `json:"icon"`
	Status      string         `json:"status"` // "done", "active", "waiting", "failed"
	StatusLabel string         `json:"status_label"`
	HandoffNote string         `json:"handoff_note"`
	FlowType    string         `json:"flow_type"` // "two_way", "one_way"
	Agents      []JobAgentNode `json:"agents"`
}

// JobAgentNode represents an individual agent participating in a job within a department.
type JobAgentNode struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Avatar     string `json:"avatar"`
	Role       string `json:"role"`
	Task       string `json:"task"`
	IsActive   bool   `json:"is_active"`
	IsDone     bool   `json:"is_done"`
	StateText  string `json:"state_text"`
	ActionType string `json:"action_type"`
}

func buildJobDeptGroups(j studio.Job) []JobDeptGroup {
	// Job đã hoàn thành -> Không hiển thị sơ đồ nhân sự, chỉ hiển thị cho job đang làm
	if j.Status == "done" {
		return nil
	}

	allMasterGroups := []JobDeptGroup{
		{
			Step:        1,
			DeptCode:    "DEP-01",
			DeptName:    "Khối Tình Báo & Thẩm Định Deal",
			Icon:        "⚖️",
			FlowType:    "one_way",
			HandoffNote: "Bàn giao brief insight & thông số ➔ Khối Biên Kịch",
			Agents: []JobAgentNode{
				{
					ID:     "agent-bep-me-bi",
					Name:   "Elena Bùi",
					Avatar: "⚖️",
					Role:   "Trưởng Khối Thương Mại & Deal Hunter",
					Task:   "Săn deal hoa hồng cao >20% & đối soát thị trường",
				},
				{
					ID:     "agent-duc-huy",
					Name:   "Đức Huy",
					Avatar: "👨‍💻",
					Role:   "Chuyên Viên Thông Số",
					Task:   "Lập bảng thông số kỹ thuật & USP",
				},
			},
		},
		{
			Step:        2,
			DeptCode:    "DEP-02",
			DeptName:    "Khối Sáng Tạo & Biên Kịch",
			Icon:        "✍️",
			FlowType:    "two_way",
			HandoffNote: "Bàn giao kịch bản 5 cảnh & Prompts 4K ➔ Studio Sản Xuất",
			Agents: []JobAgentNode{
				{
					ID:     "agent-minh-thu",
					Name:   "Minh Thư",
					Avatar: "✍️",
					Role:   "Giám Đốc Sáng Tạo Kịch Bản",
					Task:   "Soạn 4 Hook 3s giữ chân 86%+ & 5 phân cảnh",
				},
				{
					ID:     "agent-son-bach",
					Name:   "Sơn Bách",
					Avatar: "📖",
					Role:   "Chuyên Gia Cốt Truyện",
					Task:   "Tối ưu hóa nhịp 3 hồi và chuyển hóa cảm xúc",
				},
			},
		},
		{
			Step:        3,
			DeptCode:    "DEP-03",
			DeptName:    "Studio Sản Xuất Video AI",
			Icon:        "🎬",
			FlowType:    "one_way",
			HandoffNote: "Bàn giao bản dựng video 4K ➔ Kiểm Soát Chất Lượng (QC)",
			Agents: []JobAgentNode{
				{
					ID:     "agent-duc-huy",
					Name:   "Đức Huy",
					Avatar: "🎬",
					Role:   "Đạo Diễn Visual & VFX 4K",
					Task:   "Render 5 ảnh 4K Imagen 3 đồng nhất nhân vật",
				},
				{
					ID:     "agent-minh-thu",
					Name:   "Minh Thư",
					Avatar: "👩‍💼",
					Role:   "Chuyên Viên Phối Màu",
					Task:   "Duyệt bảng màu pastel & storyboard",
				},
			},
		},
		{
			Step:        4,
			DeptCode:    "DEP-04",
			DeptName:    "Hội Đồng Kiểm Định Chất Lượng Độc Lập",
			Icon:        "🛡️",
			FlowType:    "one_way",
			HandoffNote: "Cấp chứng nhận QC 96/100 ➔ Khối Phân Phối Kênh",
			Agents: []JobAgentNode{
				{
					ID:     "agent-son-bach",
					Name:   "Sơn Bách",
					Avatar: "🛡️",
					Role:   "Thanh Tra Trưởng Kiểm Định Chất Lượng",
					Task:   "Đo lường tỷ lệ giữ chân 3s & vùng an toàn hiển thị TikTok/Reels",
				},
				{
					ID:     "agent-duc-huy",
					Name:   "Đức Huy",
					Avatar: "👨‍💻",
					Role:   "Kỹ Sư Kiểm Soát Video",
					Task:   "Kiểm tra tỷ lệ 9:16 & âm lượng chuẩn -14 LUFS",
				},
			},
		},
		{
			Step:        5,
			DeptCode:    "DEP-05",
			DeptName:    "Khối Phân Phối Đa Nền Tảng",
			Icon:        "📱",
			FlowType:    "one_way",
			HandoffNote: "Lên lịch phát hành đa nền tảng TikTok/Reels/Shorts giờ vàng",
			Agents: []JobAgentNode{
				{
					ID:     "agent-minh-thu",
					Name:   "Minh Thư",
					Avatar: "📱",
					Role:   "Điều Phối Phân Phối Kênh",
					Task:   "Lên lịch phát hành đa nền tảng giờ vàng",
				},
				{
					ID:     "agent-son-bach",
					Name:   "Sơn Bách",
					Avatar: "📖",
					Role:   "Chuyên Viên Tương Tác & Chuyển Đổi",
					Task:   "Quét ContentID bản quyền & tối ưu view",
				},
			},
		},
	}

	// Dynamic filtering based on assigned departments for this job
	var p studio.AffiliateParams
	_ = json.Unmarshal([]byte(j.Params), &p)

	groups := allMasterGroups
	if len(p.AssignedDepts) > 0 {
		assignedMap := make(map[string]bool)
		for _, d := range p.AssignedDepts {
			assignedMap[strings.ToUpper(strings.TrimSpace(d))] = true
			assignedMap[strings.ToLower(strings.TrimSpace(d))] = true
		}
		var filtered []JobDeptGroup
		for _, g := range allMasterGroups {
			match := assignedMap[strings.ToUpper(g.DeptCode)]
			switch g.DeptCode {
			case "DEP-01":
				if assignedMap["dept-affiliate"] || assignedMap["affiliate"] || assignedMap["marketing"] {
					match = true
				}
			case "DEP-02":
				if assignedMap["dept-story"] || assignedMap["story"] || assignedMap["script"] {
					match = true
				}
			case "DEP-03":
				if assignedMap["dept-studio"] || assignedMap["studio"] || assignedMap["production"] {
					match = true
				}
			case "DEP-04":
				if assignedMap["dept-audio"] || assignedMap["audio"] || assignedMap["qc"] {
					match = true
				}
			case "DEP-05":
				if assignedMap["dept-reup"] || assignedMap["reup"] || assignedMap["social"] {
					match = true
				}
			}
			if match {
				filtered = append(filtered, g)
			}
		}
		if len(filtered) > 0 {
			groups = filtered
		}
	}

	for i := range groups {
		groups[i].Step = i + 1
	}

	numGroups := len(groups)
	if numGroups == 0 {
		return nil
	}

	switch j.Status {
	case "running":
		stageSpan := 100.0 / float64(numGroups)
		activeIdx := int(float64(j.Progress) / stageSpan)
		if activeIdx >= numGroups {
			activeIdx = numGroups - 1
		}

		for i := 0; i < numGroups; i++ {
			if i < activeIdx {
				groups[i].Status = "done"
				groups[i].StatusLabel = "✓ Đã xong & Bàn giao"
				for a := range groups[i].Agents {
					groups[i].Agents[a].IsDone = true
					groups[i].Agents[a].IsActive = false
					groups[i].Agents[a].StateText = "Đã hoàn thành và bàn giao"
					groups[i].Agents[a].ActionType = "done"
				}
			} else if i == activeIdx {
				groups[i].Status = "active"
				groups[i].StatusLabel = "⚡ Đang xử lý khâu"
				if groups[i].FlowType == "two_way" && len(groups[i].Agents) >= 2 {
					groups[i].Agents[0].IsActive = true
					groups[i].Agents[0].StateText = "Đang soạn thảo kịch bản & Hook 3s"
					groups[i].Agents[0].ActionType = "exchanging"
					groups[i].Agents[1].IsActive = true
					groups[i].Agents[1].StateText = "Đang phản hồi cốt truyện & cấu trúc"
					groups[i].Agents[1].ActionType = "exchanging"
				} else {
					for a := range groups[i].Agents {
						if a == 0 {
							groups[i].Agents[a].IsActive = true
							groups[i].Agents[a].StateText = "Đang trực tiếp thực hiện nhiệm vụ"
							groups[i].Agents[a].ActionType = "working"
						} else {
							groups[i].Agents[a].IsActive = false
							groups[i].Agents[a].StateText = "Hỗ trợ và phối hợp nghiệp vụ"
							groups[i].Agents[a].ActionType = "working"
						}
					}
				}
			} else {
				groups[i].Status = "waiting"
				groups[i].StatusLabel = "⏳ Chờ bàn giao"
				for a := range groups[i].Agents {
					groups[i].Agents[a].IsDone = false
					groups[i].Agents[a].IsActive = false
					groups[i].Agents[a].StateText = "Chờ khâu trước hoàn tất"
					groups[i].Agents[a].ActionType = "waiting"
				}
			}
		}

	case "failed":
		for i := range groups {
			if i == 0 {
				groups[i].Status = "failed"
				groups[i].StatusLabel = "⚠️ Tạm dừng xử lý"
				for a := range groups[i].Agents {
					groups[i].Agents[a].IsDone = false
					groups[i].Agents[a].IsActive = false
					groups[i].Agents[a].StateText = "Tạm dừng xử lý"
					groups[i].Agents[a].ActionType = "failed"
				}
			} else {
				groups[i].Status = "waiting"
				groups[i].StatusLabel = "Tạm dừng"
				for a := range groups[i].Agents {
					groups[i].Agents[a].IsDone = false
					groups[i].Agents[a].IsActive = false
					groups[i].Agents[a].StateText = "Chờ xử lý"
					groups[i].Agents[a].ActionType = "waiting"
				}
			}
		}

	default: // queued
		groups[0].Status = "active"
		groups[0].StatusLabel = "⚡ Đang tiếp nhận đề bài"
		groups[0].Agents[0].IsActive = true
		groups[0].Agents[0].StateText = "Đang nhận đề bài từ Studio"
		groups[0].Agents[0].ActionType = "working"
		for i := 1; i < len(groups); i++ {
			groups[i].Status = "waiting"
			groups[i].StatusLabel = "⏳ Đang chờ lượt"
			for a := range groups[i].Agents {
				groups[i].Agents[a].IsDone = false
				groups[i].Agents[a].IsActive = false
				groups[i].Agents[a].StateText = "Chờ phân công"
				groups[i].Agents[a].ActionType = "waiting"
			}
		}
	}

	return groups
}

// JobDeptStage represents one specialized department participating in a studio job.
type JobDeptStage struct {
	Step        int    `json:"step"`
	DeptCode    string `json:"dept_code"`
	DeptName    string `json:"dept_name"`
	Icon        string `json:"icon"`
	AgentName   string `json:"agent_name"`
	AgentAvatar string `json:"agent_avatar"`
	AgentRole   string `json:"agent_role"`
	TaskName    string `json:"task_name"`
	Deliverable string `json:"deliverable"`
	Status      string `json:"status"` // "done", "running", "waiting", "failed"
	StatusLabel string `json:"status_label"`
	Progress    int    `json:"progress"`
}

func buildJobDeptStages(j studio.Job) []JobDeptStage {
	stages := []JobDeptStage{
		{
			Step:        1,
			DeptCode:    "DEP-01",
			DeptName:    "P. Nghiên Cứu",
			Icon:        "🔍",
			AgentName:   "Bếp Mẹ Bi",
			AgentAvatar: "🍳",
			AgentRole:   "Trưởng Phòng Nghiên Cứu",
			TaskName:    "Phân tích USP sản phẩm & insight đối thủ",
			Deliverable: "Bản brief sản phẩm & thông số bán chạy",
		},
		{
			Step:        2,
			DeptCode:    "DEP-02",
			DeptName:    "P. Biên Kịch",
			Icon:        "✍️",
			AgentName:   "Minh Thư",
			AgentAvatar: "👩‍💼",
			AgentRole:   "Trưởng Phòng Biên Kịch",
			TaskName:    "Biên soạn kịch bản 5 cảnh & 4 Hook 3s giữ chân 84%+",
			Deliverable: "Kịch bản phân cảnh hoàn chỉnh & prompts 4K",
		},
		{
			Step:        3,
			DeptCode:    "DEP-03",
			DeptName:    "P. Mỹ Thuật AI",
			Icon:        "🎨",
			AgentName:   "Đức Huy",
			AgentAvatar: "👨‍💻",
			AgentRole:   "Đạo Diễn Hình Ảnh",
			TaskName:    "Render 5 ảnh phân cảnh 4K chuẩn 9:16 đồng nhất",
			Deliverable: "Bộ 5 ảnh bối cảnh 4K độ nét cao",
		},
		{
			Step:        4,
			DeptCode:    "DEP-04",
			DeptName:    "P. Âm Thanh",
			Icon:        "🎙️",
			AgentName:   "Bếp Mẹ Bi",
			AgentAvatar: "🍳",
			AgentRole:   "Chuyên Viên Lồng Tiếng",
			TaskName:    "Lồng tiếng VieNeu-TTS & tự động cân bằng âm lượng",
			Deliverable: "File master audio đồng bộ nhịp nhạc",
		},
		{
			Step:        5,
			DeptCode:    "DEP-05",
			DeptName:    "P. Thẩm Định QC",
			Icon:        "🛡️",
			AgentName:   "Sơn Bách",
			AgentAvatar: "📖",
			AgentRole:   "Giám Đốc Thẩm Định QC",
			TaskName:    "Chấm điểm QC Hook/Text/Audio & quét ContentID",
			Deliverable: "Điểm QC 96/100 — Đạt chuẩn phát hành",
		},
		{
			Step:        6,
			DeptCode:    "DEP-06",
			DeptName:    "P. Phân Phối",
			Icon:        "🚀",
			AgentName:   "Tự hành 24/7",
			AgentAvatar: "🤖",
			AgentRole:   "Hệ Thống Đăng Kênh",
			TaskName:    "Xuất bản TikTok, Reels, YouTube Shorts",
			Deliverable: "Link video trực tuyến & báo cáo view",
		},
	}

	switch j.Status {
	case "done":
		for i := range stages {
			stages[i].Status = "done"
			stages[i].Progress = 100
			if i == 5 {
				stages[i].StatusLabel = "Sẵn sàng đăng"
			} else {
				stages[i].StatusLabel = "✓ Hoàn tất"
			}
		}
	case "running":
		p := j.Progress
		switch {
		case p < 20:
			stages[0].Status = "done"
			stages[0].Progress = 100
			stages[0].StatusLabel = "✓ Hoàn tất"
			stages[1].Status = "running"
			stages[1].Progress = p * 5
			stages[1].StatusLabel = "⚡ Đang viết kịch bản"
			for i := 2; i < len(stages); i++ {
				stages[i].Status = "waiting"
				stages[i].Progress = 0
				stages[i].StatusLabel = "⏳ Chờ bàn giao"
			}
		case p < 50:
			stages[0].Status = "done"
			stages[0].Progress = 100
			stages[0].StatusLabel = "✓ Hoàn tất"
			stages[1].Status = "done"
			stages[1].Progress = 100
			stages[1].StatusLabel = "✓ Hoàn tất"
			stages[2].Status = "running"
			stages[2].Progress = (p - 20) * 100 / 30
			stages[2].StatusLabel = "⚡ Đang render ảnh 4K"
			for i := 3; i < len(stages); i++ {
				stages[i].Status = "waiting"
				stages[i].Progress = 0
				stages[i].StatusLabel = "⏳ Chờ nhận ảnh"
			}
		case p < 80:
			for i := 0; i < 3; i++ {
				stages[i].Status = "done"
				stages[i].Progress = 100
				stages[i].StatusLabel = "✓ Hoàn tất"
			}
			stages[3].Status = "running"
			stages[3].Progress = (p - 50) * 100 / 30
			stages[3].StatusLabel = "⚡ Đang thu voice & mix"
			for i := 4; i < len(stages); i++ {
				stages[i].Status = "waiting"
				stages[i].Progress = 0
				stages[i].StatusLabel = "⏳ Chờ âm thanh"
			}
		default: // 80 <= p < 100
			for i := 0; i < 4; i++ {
				stages[i].Status = "done"
				stages[i].Progress = 100
				stages[i].StatusLabel = "✓ Hoàn tất"
			}
			stages[4].Status = "running"
			stages[4].Progress = (p - 80) * 100 / 20
			stages[4].StatusLabel = "⚡ Đang thẩm định QC"
			stages[5].Status = "waiting"
			stages[5].Progress = 0
			stages[5].StatusLabel = "⏳ Chờ duyệt QC"
		}
	case "failed":
		for i := range stages {
			if i <= 2 {
				stages[i].Status = "done"
				stages[i].Progress = 100
				stages[i].StatusLabel = "✓ Hoàn tất"
			} else if i == 3 {
				stages[i].Status = "failed"
				stages[i].Progress = 50
				stages[i].StatusLabel = "⚠️ Lỗi xử lý"
			} else {
				stages[i].Status = "waiting"
				stages[i].Progress = 0
				stages[i].StatusLabel = "Tạm dừng"
			}
		}
	default: // queued
		stages[0].Status = "running"
		stages[0].Progress = 30
		stages[0].StatusLabel = "⚡ Đang tiếp nhận"
		for i := 1; i < len(stages); i++ {
			stages[i].Status = "waiting"
			stages[i].Progress = 0
			stages[i].StatusLabel = "⏳ Đang chờ lượt"
		}
	}
	return stages
}

func kindLabel(kind string) string {
	switch kind {
	case studio.KindAffiliate:
		return "Affiliate"
	default:
		return kind
	}
}

func (s *Server) studioJobViews(jobs []studio.Job) []studioJobView {
	out := make([]studioJobView, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, studioJobView{
			ID: j.ID, Kind: j.Kind, Title: j.Title, Status: j.Status,
			Progress: j.Progress, Output: mediaName(j.Output),
			CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt,
			KindLabel:  kindLabel(j.Kind),
			Label:      statusLabel(j.Status),
			Class:      statusClass(j.Status),
			DeptStages: buildJobDeptStages(j),
			DeptGroups: buildJobDeptGroups(j),
		})
	}
	return out
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

// handleStudio renders the Studio page: creation form, job links, trends.
// Đợt AK: tab "Video chữ động" (kinetic) đã gỡ — trái format affiliate đã
// chốt (ảnh + nhạc, không chữ) và không tự động hóa nào dùng.
func (s *Server) handleStudio(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.render(w, "studio", s.ctx(
			"Error", "Studio chưa được khởi tạo."))
		return
	}
	jobs := s.studioJobViews(s.Studio.ListJobs(30))
	// Zero-touch (Đợt 3 / A6): a stale chart refreshes itself in the
	// background — nobody has to remember the refresh button.
	if studio.TrendsStale() {
		studio.RefreshVNTrendingAsync()
	}
	sounds, trendsErr := studio.FetchVNTrending(r.Context())
	activeJobs, jobCount := 0, len(jobs)
	doneCount := 0
	for _, j := range jobs {
		if j.Status == "running" || j.Status == "queued" {
			activeJobs++
		} else if j.Status == "done" {
			doneCount++
		}
	}
	mgOK := false
	if mg := s.Studio.MediaGen(); mg != nil {
		mgOK = mg.Healthy(r.Context())
	}

	var accountNames []string
	if s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, a := range accounts {
				if a != nil && a.Username != "" {
					accountNames = append(accountNames, a.Username)
				}
			}
		}
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "affiliate"
	}
	notice := r.URL.Query().Get("notice")
	if notice == "" && r.URL.Query().Get("msg") == "created" {
		notice = "Tác vụ sản xuất video đã được đưa vào hàng đợi AI thành công!"
	}

	ttTrends, _ := studio.FetchTikTokTrendingVideos(r.Context())
	dramasVN, dramasCN, _ := studio.FetchSplitShowbizDrama(r.Context())
	combinedDramas := append([]studio.ShowbizDrama(nil), dramasVN...)
	combinedDramas = append(combinedDramas, dramasCN...)
	topicGroups := studio.GetConfiguredTopicGroups([]string{"nhay", "su_kien_vn", "bien_hinh", "hai_huoc"})
	presetTopicGroups := studio.GetAllPresetTopicGroups()

	var selectedModelID int64
	if midStr := r.URL.Query().Get("model_id"); midStr != "" {
		selectedModelID, _ = strconv.ParseInt(midStr, 10, 64)
	}
	availableModels := s.listModelPhotoCards()
	if selectedModelID == 0 && len(availableModels) > 0 {
		selectedModelID = availableModels[0].ID
	}

	s.render(w, "studio", s.ctx(
		"Jobs", jobs, // giữ field cho tương thích: danh sách job ở /studio/jobs
		"Shelf", s.shelfViews(),
		"JobCount", jobCount,
		"ActiveJobs", activeJobs,
		"RunningCount", activeJobs,
		"DoneCount", doneCount,
		"CurrentTab", "create",
		"InitialMode", mode,
		"Notice", notice,
		"Sounds", sounds,
		"TopSound", topSound(sounds),
		"TrendsSource", studio.TrendsSource,
		"TrendsErr", errText(trendsErr),
		"MediaGenOK", mgOK,
		"AccountNames", accountNames,
		"AvailableModels", availableModels,
		"SelectedModelID", selectedModelID,
		"ViralTrends", defaultViralTrends,
		"TikTokTrends", ttTrends,
		"TopicGroups", topicGroups,
		"PresetTopicGroups", presetTopicGroups,
		"ShowbizDramasVN", dramasVN,
		"ShowbizDramasCN", dramasCN,
		"ShowbizDramas", combinedDramas,
		"AudioStoryPlots", studio.GetCuratedAudioStories(""),
		"DefStoryGenre", s.atSettingStr(automation.KeyStoryGenre, "tâm lý"),
		"DefStoryWords", s.atSettingInt(automation.KeyStoryWords, 600),
		"DefStoryScenes", s.atSettingInt(automation.KeyStoryScenes, 6),
		"Error", r.URL.Query().Get("err"),
	))
}

// topSound returns "Artist – Title" of the top-ranked trending sound.
func topSound(sounds []studio.TrendingSound) string {
	if len(sounds) == 0 {
		return ""
	}
	return sounds[0].Artist + " – " + sounds[0].Title
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// saveUpload stores one multipart file into dir, returning its path.
func saveUpload(r *http.Request, field, dir string) (string, error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return "", nil // field absent is fine
	}
	defer f.Close()
	if hdr.Size == 0 {
		return "", nil
	}
	// Đợt AC: báo rõ khi file quá lớn thay vì cắt im lặng 50MB.
	if hdr.Size > 50<<20 {
		return "", fmt.Errorf("file %s quá lớn (%.1fMB, tối đa 50MB)", field, float64(hdr.Size)/(1<<20))
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".mp3", ".m4a", ".wav", ".ogg":
	default:
		return "", fmt.Errorf("file %s: định dạng %s không hỗ trợ", field, ext)
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), field, ext)
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, io.LimitReader(f, 50<<20)); err != nil {
		out.Close()
		return "", err
	}
	out.Close()
	return dst, nil
}

// saveUploads lưu tối đa max ảnh từ 1 ô chọn nhiều file (chỉ nhận ảnh).
func saveUploads(r *http.Request, field, dir string, max int) ([]string, error) {
	if r.MultipartForm == nil {
		return nil, nil
	}
	var out []string
	for i, hdr := range r.MultipartForm.File[field] {
		if i >= max {
			break
		}
		if hdr.Size == 0 {
			continue
		}
		if hdr.Size > 50<<20 {
			return nil, fmt.Errorf("ảnh %s quá lớn (tối đa 50MB)", hdr.Filename)
		}
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp":
		default:
			return nil, fmt.Errorf("ảnh %s: định dạng %s không hỗ trợ", hdr.Filename, ext)
		}
		f, err := hdr.Open()
		if err != nil {
			return nil, err
		}
		dst := filepath.Join(dir, fmt.Sprintf("%d-%s-%d%s", time.Now().UnixNano(), field, i, ext))
		out2, err := os.Create(dst)
		if err != nil {
			f.Close()
			return nil, err
		}
		_, err = io.Copy(out2, f)
		out2.Close()
		f.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, dst)
	}
	return out, nil
}

// handleStudioAffiliateCreate queues an affiliate video job.
func (s *Server) handleStudioAffiliateCreate(w http.ResponseWriter, r *http.Request) {
	studioErr := func(msg string) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(msg))
	}
	if s.Studio == nil {
		studioErr("Studio chưa sẵn sàng — thử tải lại trang.")
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		studioErr("Không đọc được form — thử lại.")
		return
	}
	upDir := s.Studio.UploadDir()
	var modelPhotos []string
	for _, key := range []string{"model_photos", "character_photos", "model_photo", "character_photo"} {
		if photos, _ := saveUploads(r, key, upDir, 10); len(photos) > 0 {
			modelPhotos = append(modelPhotos, photos...)
		}
	}
	if len(modelPhotos) == 0 {
		if p1, _ := saveUpload(r, "model_photo", upDir); p1 != "" {
			modelPhotos = append(modelPhotos, p1)
		} else if p2, _ := saveUpload(r, "character_photo", upDir); p2 != "" {
			modelPhotos = append(modelPhotos, p2)
		}
	}

	acctName := strings.TrimSpace(r.FormValue("account"))
	var acctID int64
	if rawID := strings.TrimSpace(r.FormValue("account_id")); rawID != "" {
		acctID, _ = strconv.ParseInt(rawID, 10, 64)
	}
	if acctID == 0 && acctName != "" && s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, a := range accounts {
				if a != nil && a.Username == acctName {
					acctID = a.ID
					break
				}
			}
		}
	}
	if len(modelPhotos) > 0 && acctID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil {
			for _, pth := range modelPhotos {
				_, _ = ms.Add(acctID, pth)
			}
		}
	}
	if len(modelPhotos) == 0 && acctID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil {
			if photos, err := ms.List(acctID); err == nil && len(photos) > 0 {
				for _, ph := range photos {
					modelPhotos = append(modelPhotos, ph.Path)
				}
			}
		}
	}
	if len(modelPhotos) == 0 {
		if midStr := strings.TrimSpace(r.FormValue("model_id")); midStr != "" {
			if mid, err := strconv.ParseInt(midStr, 10, 64); err == nil && mid > 0 {
				if photoPath, perr := s.findModelPhotoByID(mid); perr == nil && photoPath != "" {
					modelPhotos = append(modelPhotos, photoPath)
				}
			}
		}
	}
	var modelPhoto string
	if len(modelPhotos) > 0 {
		modelPhoto = modelPhotos[0]
	}
	prodName := strings.TrimSpace(r.FormValue("product_name"))
	niche := strings.TrimSpace(r.FormValue("niche"))
	priceVal := strings.TrimSpace(r.FormValue("product_price"))
	affLinkVal := strings.TrimSpace(r.FormValue("aff_link"))

	var remotePhotoURLs []string
	if pidStr := strings.TrimSpace(r.FormValue("product_id")); pidStr != "" && s.Products != nil {
		if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil && pid > 0 {
			if prod, perr := s.Products.GetByID(pid); perr == nil {
				if prodName == "" {
					prodName = prod.Title
				}
				if priceVal == "" && prod.Price > 0 {
					priceVal = fmt.Sprintf("%.0f ₫", prod.Price)
				}
				if niche == "" && prod.Category != "" {
					niche = prod.Category
				}
				if affLinkVal == "" && prod.ProductURL != "" {
					affLinkVal = prod.ProductURL
				}
				remotePhotoURLs = append(remotePhotoURLs, prod.ImageURLs...)
			}
		}
	}
	rawUrls := strings.TrimSpace(r.FormValue("photo_urls"))
	if rawUrls == "" {
		rawUrls = strings.TrimSpace(r.FormValue("photo_url"))
	}
	for _, u := range strings.Split(rawUrls, "\n") {
		u = strings.TrimSpace(u)
		if u != "" {
			remotePhotoURLs = append(remotePhotoURLs, u)
		}
	}

	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		studioErr("Ảnh sản phẩm lỗi: " + err.Error())
		return
	}
	if productPhoto == "" {
		if gen := strings.TrimSpace(r.FormValue("generated_photo_path")); gen != "" {
			if _, serr := os.Stat(gen); serr == nil {
				productPhoto = gen
			}
		}
	}

	extraPhotos, err := saveUploads(r, "extra_photos", upDir, 6)
	if err != nil {
		studioErr("Ảnh thêm lỗi: " + err.Error())
		return
	}

	// Tải ảnh sản phẩm thật từ URL (nếu người dùng nhập URL hoặc chọn từ kệ)
	for i, u := range remotePhotoURLs {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			continue
		}
		if productPhoto != "" && len(extraPhotos) >= 5 {
			break
		}
		dlDst := filepath.Join(upDir, fmt.Sprintf("%d-dl-prod-%d.jpg", time.Now().UnixNano(), i))
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		derr := studio.DownloadImage(ctx, u, dlDst)
		cancel()
		if derr == nil && fileExists(dlDst) {
			if productPhoto == "" {
				productPhoto = dlDst
			} else {
				extraPhotos = append(extraPhotos, dlDst)
			}
		}
	}

	if productPhoto == "" {
		genPath := filepath.Join(upDir, fmt.Sprintf("%d-autogen-prod.png", time.Now().UnixNano()))
		if prodName == "" {
			prodName = "Sản phẩm AI Creator"
		}
		if s.Studio != nil && s.Studio.MediaGen() != nil && s.Studio.MediaGen().Healthy(r.Context()) {
			prompt := fmt.Sprintf("Ultra-detailed professional commercial advertising photograph of %s, %s, 4K quality, tack-sharp focus, photorealistic, cinematic lighting, 9:16 vertical, no text, no watermark", prodName, niche)
			if gerr := s.Studio.MediaGen().GenerateImage(r.Context(), prompt, nil, genPath); gerr == nil {
				productPhoto = genPath
			}
		}
		if productPhoto == "" {
			if verr := studio.GenerateProductVisual(prodName, niche, "SAN PHAM CHINH HANG - DAT MUA NGAY", 1080, 1920, genPath); verr == nil {
				productPhoto = genPath
			}
		}
	}

	musicPath, err := saveUpload(r, "music_file", upDir)
	if err != nil {
		studioErr("File nhạc lỗi: " + err.Error())
		return
	}
	// Optional: direct audio URL for the trending sound.
	if musicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			defer cancel()
			if derr := studio.DownloadAudio(ctx, u, dst); derr != nil {
				studioErr("Không tải được nhạc từ link: " + derr.Error())
				return
			}
			musicPath = dst
		}
	}
	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))
	musicStart, _ := strconv.ParseFloat(r.FormValue("music_start"), 64)
	seconds = clampInt(seconds, 10, 120)
	bpm = clampInt(bpm, 60, 200)
	if musicStart < 0 {
		musicStart = 0
	}
	var assignedDepts []string
	if r.MultipartForm != nil {
		assignedDepts = r.MultipartForm.Value["assigned_depts"]
	}
	if len(assignedDepts) == 0 {
		assignedDepts = r.Form["assigned_depts"]
	}
	if len(assignedDepts) == 0 {
		assignedDepts = []string{"DEP-01", "DEP-02", "DEP-03", "DEP-04", "DEP-05"}
	}

	voText := strings.TrimSpace(r.FormValue("voiceover_text"))
	if voText == "" && prodName != "" {
		priceStr := ""
		if priceVal != "" {
			priceStr = ", giá ưu đãi chỉ " + priceVal
		}
		voText = fmt.Sprintf("Khám phá %s chính hãng. Thiết kế sang trọng, chất lượng cao cấp%s. Bấm vào giỏ hàng góc trái săn ưu đãi ngay hôm nay!", prodName, priceStr)
	}

	p := studio.AffiliateParams{
		Mode:          r.FormValue("mode"),
		Niche:         niche,
		ProductName:   prodName,
		ProductPrice:  priceVal,
		AffLink:       affLinkVal,
		FixedCaption:  strings.TrimSpace(r.FormValue("caption")),
		VoiceoverText: voText,
		ModelPhoto:    modelPhoto,
		ModelPhotos:   modelPhotos,
		ProductPhoto:  productPhoto,
		ExtraPhotos:   extraPhotos,
		Seconds:       seconds,
		BPM:           bpm,
		MusicPath:     musicPath,
		MusicStart:    musicStart,
		MusicTitle:    strings.TrimSpace(r.FormValue("music_title")),
		MusicArtist:   strings.TrimSpace(r.FormValue("music_artist")),
		AssignedDepts: assignedDepts,
	}
	if p.ProductName == "" {
		p.ProductName = "Sản phẩm"
	}

	sampleClip := strings.TrimSpace(r.FormValue("sample_clip"))
	if sampleClip != "" {
		candidates := []string{
			filepath.Join(s.DataDir, "clips", "fashion", sampleClip),
			filepath.Join(s.DataDir, "clips", "fashion", sampleClip+".mp4"),
			filepath.Join("data", "clips", "fashion", sampleClip),
			filepath.Join("data", "clips", "fashion", sampleClip+".mp4"),
		}
		for _, cand := range candidates {
			if fileExists(cand) {
				p.Clips = []string{cand}
				break
			}
		}
	}
	// Chỉ sử dụng clips nếu người dùng chọn rõ ràng sampleClip

	jobID, err := s.Studio.CreateAffiliateJob(p)
	if err != nil {
		studioErr("Không tạo được job: " + err.Error())
		return
	}
	seeOther(w, r, "/studio/jobs?tab=running&msg=created&job_id="+jobID)
}

// (R2-W3), JSON for polling when Accept: application/json.
func (s *Server) handleStudioJobs(w http.ResponseWriter, r *http.Request) {
	var allJobs []studioJobView
	if s.Studio != nil {
		allJobs = s.studioJobViews(s.Studio.ListJobs(50))
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{"jobs": allJobs})
		return
	}

	var runningJobs, doneJobs, failedJobs []studioJobView
	for _, j := range allJobs {
		switch j.Status {
		case "running", "queued":
			runningJobs = append(runningJobs, j)
		case "done":
			doneJobs = append(doneJobs, j)
		case "failed":
			failedJobs = append(failedJobs, j)
		}
	}

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		if len(runningJobs) > 0 {
			tab = "running"
		} else {
			tab = "done"
		}
	}

	var displayedJobs []studioJobView
	switch tab {
	case "running":
		displayedJobs = runningJobs
	case "done":
		displayedJobs = doneJobs
	case "failed":
		displayedJobs = failedJobs
	case "schedule":
		displayedJobs = runningJobs
	default:
		displayedJobs = allJobs
	}

	schedEnabled, schedHours, schedLastRun, schedMusic, schedAutoPub, schedYouTube := s.scheduleView()
	schedMeta := false
	if s.Ledger != nil {
		schedMeta = automation.AutopilotMetaEnabled(s.settings())
	}

	nextRunStr := "Đang chờ lịch kích hoạt"
	if schedEnabled {
		if schedLastRun != "" {
			if t, err := time.Parse(time.RFC3339, schedLastRun); err == nil {
				next := t.Add(time.Duration(schedHours) * time.Hour)
				if next.After(time.Now()) {
					nextRunStr = next.Format("15:04 ngày 02/01/2006")
				} else {
					nextRunStr = "Sắp chạy ngay trong vài phút tới"
				}
			}
		} else {
			nextRunStr = "Sắp kích hoạt trong chu kỳ tiếp theo"
		}
	} else {
		nextRunStr = "Tạm dừng (Lịch tự động đang tắt)"
	}

	s.render(w, "studio_jobs", s.ctx(
		"Title", "Quản lý Jobs & Lịch Studio",
		"Path", "/studio/jobs",
		"Jobs", displayedJobs,
		"AllJobs", allJobs,
		"RunningJobs", runningJobs,
		"DoneJobs", doneJobs,
		"FailedJobs", failedJobs,
		"RunningCount", len(runningJobs),
		"DoneCount", len(doneJobs),
		"TotalCount", len(allJobs),
		"Tab", tab,
		"CurrentTab", tab,
		"SchedEnabled", schedEnabled,
		"SchedHours", schedHours,
		"SchedLastRun", schedLastRun,
		"SchedNextRun", nextRunStr,
		"SchedMusic", schedMusic,
		"SchedAutoPub", schedAutoPub,
		"SchedYouTube", schedYouTube,
		"SchedMeta", schedMeta,
		"Msg", r.URL.Query().Get("msg"),
		"Err", r.URL.Query().Get("err"),
	))
}

// handleStudioScheduleSave saves the autopilot schedule settings directly from Studio.
func (s *Server) handleStudioScheduleSave(w http.ResponseWriter, r *http.Request) {
	if s.Ledger == nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Kho cài đặt chưa khởi tạo"))
		return
	}
	if err := r.ParseForm(); err != nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Không đọc được dữ liệu form: "+err.Error()))
		return
	}
	enabled := "0"
	if r.PostFormValue("enabled") == "on" {
		enabled = "1"
	}
	autoPub := "0"
	if r.PostFormValue("auto_publish") == "on" {
		autoPub = "1"
	}
	youTube := "0"
	if r.PostFormValue("youtube_enabled") == "on" {
		youTube = "1"
	}
	meta := "0"
	if r.PostFormValue("meta_enabled") == "on" {
		meta = "1"
	}
	hours := 6
	if h, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("interval_hours"))); err == nil && h >= 1 && h <= 168 {
		hours = h
	}

	st := s.settings()
	_ = st.Set(SettingAutopilotEnabled, enabled)
	_ = st.Set(SettingAutopilotInterval, strconv.Itoa(hours))
	_ = st.Set(SettingAutopilotAutoPublish, autoPub)
	_ = st.Set(SettingAutopilotYouTube, youTube)
	_ = st.Set(SettingAutopilotMeta, meta)

	msg := "Đã cập nhật lịch tự động: TẮT"
	if enabled == "1" {
		msg = fmt.Sprintf("Đã bật lịch tự động tạo video mỗi %d giờ.", hours)
	}
	seeOther(w, r, "/studio/jobs?tab=schedule&msg="+url.QueryEscape(msg))
}

// handleStudioScheduleRunNow triggers one immediate autopilot production cycle.
func (s *Server) handleStudioScheduleRunNow(w http.ResponseWriter, r *http.Request) {
	if s.Autopilot == nil {
		seeOther(w, r, "/studio/jobs?tab=schedule&err="+url.QueryEscape("Hệ thống Autopilot chưa được cấu hình runner."))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_ = s.Autopilot.RunAll(ctx)
	}()
	seeOther(w, r, "/studio/jobs?tab=running&msg="+url.QueryEscape("Đã phát lệnh kích hoạt dây chuyền AI sản xuất video ngay lập tức!"))
}

// handleStudioJobCancel cancels a running/queued job.
func (s *Server) handleStudioJobCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio != nil {
		s.Studio.CancelRunning(id)
	}
	seeOther(w, r, "/studio/jobs")
}

// handleStudioJobDelete removes a job and all its files.
func (s *Server) handleStudioJobDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio == nil {
		seeOther(w, r, "/studio/jobs")
		return
	}
	if err := s.Studio.DeleteJob(id); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không xoá được job: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs")
}

// handleStudioJobRetry creates a new affiliate job from an old job's params.
func (s *Server) handleStudioJobRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.Studio == nil {
		seeOther(w, r, "/studio/jobs")
		return
	}
	j, ok := s.Studio.GetJob(id)
	if !ok {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tìm thấy job "+id))
		return
	}
	if j.Kind != studio.KindAffiliate {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Chỉ thử lại được job affiliate"))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không đọc được tham số job cũ"))
		return
	}
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Không tạo được job mới: "+err.Error()))
		return
	}
	seeOther(w, r, "/studio/jobs")
}

// studioAssetView projects a storyboard asset for JSON.
// Label/Class are server-rendered so clients never redefine them (R2-W3).
type studioAssetView struct {
	ID      int64  `json:"id"`
	Idx     int    `json:"idx"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Prompt  string `json:"prompt"`
	Preview string `json:"preview"`
	Label   string `json:"label"`
	Class   string `json:"class"`
}

// handleStudioJobDetail returns one job + its storyboard assets as JSON.
func (s *Server) handleStudioJobDetail(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	j, ok := s.Studio.GetJob(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	assets := s.Studio.ListAssets(id)
	views := make([]studioAssetView, 0, len(assets))
	for _, a := range assets {
		v := studioAssetView{ID: a.ID, Idx: a.Idx, Kind: a.Kind, Status: a.Status, Prompt: a.Prompt,
			Label: statusLabel(a.Status), Class: statusClass(a.Status)}
		if a.Status == "done" && a.Path != "" {
			v.Preview = "/studio/assets/" + id + "/" + filepath.Base(a.Path)
		}
		views = append(views, v)
	}
	resp := map[string]any{
		"job":    j,
		"assets": views,
	}
	// Checklist lên xu hướng cho video affiliate đã xong (đo từ file thật).
	if j.Kind == studio.KindAffiliate && j.Status == studio.StatusDone && j.Output != "" {
		var p studio.AffiliateParams
		_ = json.Unmarshal([]byte(j.Params), &p)
		outPath := j.Output
		if s.Studio != nil {
			outPath = s.Studio.OutputPath(j.Output)
		}
		resp["trend"] = studio.TrendReadiness(r.Context(), outPath, p, j.Caption)
	}
	s.writeJSON(w, resp)
}

var studioAssetRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(png|jpg|jpeg|webp|mp4)$`)

func (s *Server) handleStudioAsset(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	job := r.PathValue("job")
	file := r.PathValue("file")
	if !studioAssetRe.MatchString(file) || strings.Contains(job, "..") || strings.Contains(job, "/") {
		http.NotFound(w, r)
		return
	}
	// The job id is a unix-nano timestamp; be lenient but safe.
	for _, c := range job {
		if c < '0' || c > '9' {
			http.NotFound(w, r)
			return
		}
	}
	path := filepath.Join(s.Studio.WorkDir(), job, file)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.Studio.WorkDir())) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, path)
}

// handleStudioTrends serves the VN trending chart: HTML page (R2-W3),
// JSON when Accept: application/json.
func (s *Server) handleStudioTrends(w http.ResponseWriter, r *http.Request) {
	sounds, err := studio.FetchVNTrending(r.Context())
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.writeJSON(w, map[string]any{
			"source": studio.TrendsSource,
			"sounds": sounds,
			"error":  errText(err),
		})
		return
	}
	s.render(w, "studio_trends", s.ctx(
		"Title", "Nhạc thịnh hành", "Path", "/studio/trends",
		"Sounds", sounds, "TrendsSource", studio.TrendsSource, "TrendsErr", errText(err),
	))
}

// handleStudioTrendsRefresh forces a chart refresh.
func (s *Server) handleStudioTrendsRefresh(w http.ResponseWriter, r *http.Request) {
	_, _ = studio.RefreshVNTrending(r.Context())
	http.Redirect(w, r, "/studio/trends", http.StatusSeeOther)
}

// handleStudioMediaGenHealth reports media-gen provider health as JSON.
func (s *Server) handleStudioMediaGenHealth(w http.ResponseWriter, r *http.Request) {
	healthy, keys, name := false, 0, ""
	if s.Studio != nil {
		if mg := s.Studio.MediaGen(); mg != nil {
			name = mg.Name()
			healthy = mg.Healthy(r.Context())
			if g, ok := mg.(interface{ KeyCount() int }); ok {
				keys = g.KeyCount()
			}
		}
	}
	s.writeJSON(w, map[string]any{
		"provider": name, "healthy": healthy, "keys": keys,
	})
}

var studioUploadFileRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+\.(png|jpg|jpeg|webp|mp4|mov|m4a|mp3)$`)

// handleStudioUpload serves uploaded and AI-generated assets from Studio's upload directory.
func (s *Server) handleStudioUpload(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	file := r.PathValue("file")
	if !studioUploadFileRe.MatchString(file) || strings.Contains(file, "..") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.Studio.UploadDir(), file)
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.Studio.UploadDir())) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, path)
}

// handleStudioGenerateImage generates a high-quality product image via Gemini Imagen or visual fallback.
func (s *Server) handleStudioGenerateImage(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		writeJSONErr(w, "Studio chưa sẵn sàng", http.StatusServiceUnavailable)
		return
	}
	productName := strings.TrimSpace(r.FormValue("product_name"))
	niche := strings.TrimSpace(r.FormValue("niche"))
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	aspect := strings.TrimSpace(r.FormValue("aspect_ratio"))
	if productName == "" {
		productName = "Sản phẩm AI Creator"
	}
	if niche == "" {
		niche = "Thương mại điện tử"
	}

	wPx, hPx := 1080, 1920
	if aspect == "16:9" {
		wPx, hPx = 1920, 1080
	} else if aspect == "1:1" {
		wPx, hPx = 1080, 1080
	}

	filename := fmt.Sprintf("%d-ai-prod.png", time.Now().UnixNano())
	outPath := filepath.Join(s.Studio.UploadDir(), filename)
	source := "studio_visual"

	// Try Gemini Imagen if keys are healthy
	if s.Studio.MediaGen() != nil && s.Studio.MediaGen().Healthy(r.Context()) {
		geminiPrompt := prompt
		if geminiPrompt == "" {
			geminiPrompt = fmt.Sprintf("Ultra-detailed professional commercial advertising photograph of %s, %s, 4K quality, tack-sharp focus, photorealistic, cinematic studio lighting, commercial photography, no text, no watermark", productName, niche)
		}
		if gerr := s.Studio.MediaGen().GenerateImage(r.Context(), geminiPrompt, nil, outPath); gerr == nil {
			source = "gemini"
		}
	}

	// If Gemini was not used or failed, generate high-quality visual card
	if source != "gemini" {
		sub := prompt
		if sub == "" {
			sub = "CHÍNH HÃNG 100% · ƯU ĐÃI ĐẶC BIỆT"
		}
		if err := studio.GenerateProductVisual(productName, niche, sub, wPx, hPx, outPath); err != nil {
			writeJSONErr(w, "Không tạo được ảnh: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	s.writeJSON(w, map[string]any{
		"ok":       true,
		"filename": filename,
		"path":     outPath,
		"url":      "/studio/uploads/" + filename,
		"source":   source,
	})
}

var mediaNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+\.(mp4|srt|vtt|mp3|wav|jpg|jpeg|png|webp)$`)

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// Strict allowlist: a single path segment, valid media extensions only. Anything else
	// (including "..") is a 404.
	if !mediaNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, filepath.Join(s.OutDir, name))
}

// handleClipMedia serves sample motion/lookbook video clips from data/clips/.
func (s *Server) handleClipMedia(w http.ResponseWriter, r *http.Request) {
	cat := r.PathValue("category")
	name := r.PathValue("name")
	if cat != "viral" && cat != "fashion" {
		http.NotFound(w, r)
		return
	}
	if !mediaNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	serveMediaFile(w, r, filepath.Join("data", "clips", cat, name))
}

// --------------------------------------------------------------- publishers

// capcutExportRoot là nơi xuất gói chỉnh sửa (~/Movies/AI Creator OS).
// Biến để test trỏ về thư mục tạm.
var capcutExportRoot = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Movies", "AI Creator OS")
}

// openOnMac chạy lệnh `open` của macOS (seam cho test).
var openOnMac = func(args ...string) error { return exec.Command("open", args...).Run() }

// handleStudioJobCapCut xuất gói chỉnh sửa, mở Finder tới thư mục và bật
// CapCut nếu máy có (CapCut không có API — người dùng kéo thả file vào).
func (s *Server) handleStudioJobCapCut(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.NotFound(w, r)
		return
	}
	dir, err := s.Studio.ExportForEditing(r.PathValue("id"), capcutExportRoot())
	if err != nil {
		seeOther(w, r, "/studio/jobs?err="+url.QueryEscape("Xuất cho CapCut lỗi: "+err.Error()))
		return
	}
	_ = openOnMac("-R", dir)
	msg := "Đã xuất vào " + dir + " — kéo các file vào timeline CapCut."
	if openOnMac("-a", "CapCut") != nil {
		msg += " (Chưa thấy app CapCut — cài từ capcut.com rồi mở thư mục này.)"
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape(msg))
}

// mediaName trả tên file để phát qua /media/{name} (route chỉ nhận 1
// đoạn tên). j.Output là đường dẫn đầy đủ — dùng thẳng sẽ ra URL hỏng.
func mediaName(output string) string {
	if output == "" {
		return ""
	}
	return filepath.Base(output)
}

// handleStudioJobRemix dựng lại 1 video affiliate với tham số đã sửa
// (thời lượng, nhịp, đoạn nhạc, file nhạc, thứ tự ảnh, chú thích). Bản cũ
// giữ nguyên — đây là "chỉnh sửa" trong app vì CapCut không có API.
func (s *Server) handleStudioJobRemix(w http.ResponseWriter, r *http.Request) {
	back := func(q string) { seeOther(w, r, "/studio/jobs?"+q) }
	if s.Studio == nil {
		back("err=" + url.QueryEscape("Studio chưa sẵn sàng."))
		return
	}
	j, ok := s.Studio.GetJob(r.PathValue("id"))
	if !ok || j.Kind != studio.KindAffiliate {
		back("err=" + url.QueryEscape("Chỉ chỉnh sửa được video affiliate đã có."))
		return
	}
	var p studio.AffiliateParams
	if err := json.Unmarshal([]byte(j.Params), &p); err != nil {
		back("err=" + url.QueryEscape("Không đọc được tham số video cũ."))
		return
	}
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		back("err=" + url.QueryEscape("Không đọc được form — thử lại."))
		return
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("seconds"))); err == nil {
		p.Seconds = clampInt(v, 10, 120)
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.FormValue("bpm"))); err == nil {
		p.BPM = clampInt(v, 60, 200)
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("music_start")), 64); err == nil && v >= 0 {
		p.MusicStart = v
	}
	music, err := saveUpload(r, "music_file", s.Studio.UploadDir())
	if err != nil {
		back("err=" + url.QueryEscape("File nhạc lỗi: "+err.Error()))
		return
	}
	if music != "" {
		p.MusicPath = music
	}
	p = reorderPhotos(p, r.FormValue("order"))
	p.FixedCaption = strings.TrimSpace(r.FormValue("caption"))
	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		back("err=" + url.QueryEscape("Không dựng lại được: "+err.Error()))
		return
	}
	back("msg=" + url.QueryEscape("Đang dựng bản chỉnh sửa — bản cũ vẫn giữ để so sánh."))
}

// reorderPhotos đổi thứ tự ảnh sản phẩm + ảnh thêm (ảnh người mẫu giữ
// nguyên vì là ảnh khóa khuôn mặt). order: "reverse" | "rotate" | "".
func reorderPhotos(p studio.AffiliateParams, order string) studio.AffiliateParams {
	var all []string
	if p.ProductPhoto != "" {
		all = append(all, p.ProductPhoto)
	}
	all = append(all, p.ExtraPhotos...)
	if len(all) < 2 {
		return p
	}
	switch order {
	case "reverse":
		for i, k := 0, len(all)-1; i < k; i, k = i+1, k-1 {
			all[i], all[k] = all[k], all[i]
		}
	case "rotate":
		all = append(all[1:], all[0])
	default:
		return p
	}
	p.ProductPhoto, p.ExtraPhotos = all[0], append([]string(nil), all[1:]...)
	return p
}

// handleStudioViralCreate nhận yêu cầu tạo video đu trend / viral shorts 9:16 từ Studio.
func (s *Server) handleStudioViralCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Studio chưa sẵn sàng — thử tải lại trang."))
		return
	}
	_ = r.ParseForm()
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	if topic == "" {
		seeOther(w, r, "/studio?mode=viral&err="+url.QueryEscape("Vui lòng nhập chủ đề hoặc ý tưởng video"))
		return
	}
	genre := r.PostFormValue("genre")
	if genre == "" {
		genre = "trendjack"
	}
	visualMood := r.PostFormValue("visual_mood")
	if visualMood == "" {
		visualMood = "cinematic"
	}
	voice := r.PostFormValue("voice")
	account := r.PostFormValue("account")

	duration, _ := strconv.Atoi(r.PostFormValue("duration"))
	if duration < 15 || duration > 60 {
		duration = 30
	}
	musicURL := r.PostFormValue("music_url")

	var accountID int64
	var uploadedPhotos []string
	if account != "" && s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, a := range accounts {
				if a != nil && a.Username == account {
					accountID = a.ID
					break
				}
			}
		}
	}
	if accountID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil {
			if photos, err := ms.List(accountID); err == nil && len(photos) > 0 {
				for _, ph := range photos {
					uploadedPhotos = append(uploadedPhotos, ph.Path)
				}
			}
		}
	}
	var modelPhoto string
	if len(uploadedPhotos) > 0 {
		modelPhoto = uploadedPhotos[0]
	}

	p := studio.ViralParams{
		Topic:          topic,
		Genre:          genre,
		VisualMood:     visualMood,
		Voice:          voice,
		Account:        account,
		AccountID:      accountID,
		ModelPhoto:     modelPhoto,
		ModelPhotos:    uploadedPhotos,
		TargetDuration: duration,
		MusicURL:       musicURL,
	}

	jobID, err := s.Studio.EnqueueViral(r.Context(), p)
	if err != nil {
		seeOther(w, r, "/studio?mode=viral&err="+url.QueryEscape("Lỗi tạo job: "+err.Error()))
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("ai_orchestrator", "viral_produce", nil, fmt.Sprintf("Studio điều phối video viral 9:16 job=%s genre=%s: %s", jobID, genre, topic), nil)
	}

	seeOther(w, r, "/studio/jobs?tab=running&notice="+url.QueryEscape(fmt.Sprintf("Đã đưa video đu trend '%s' vào dây chuyền sản xuất AI (Job: %s)!", topic, jobID)))
}

// handleStudioViralQuick khởi tạo tức thì từ mẫu xu hướng 1-chạm trong Studio.
func (s *Server) handleStudioViralQuick(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Studio chưa sẵn sàng — thử tải lại trang."))
		return
	}
	_ = r.ParseForm()
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	genre := r.PostFormValue("genre")
	if topic == "" {
		topic = "Bí quyết phát triển bản thân và tư duy tài chính thông minh"
	}
	if genre == "" {
		genre = "trendjack"
	}

	p := studio.ViralParams{
		Topic:          topic,
		Genre:          genre,
		VisualMood:     "cinematic",
		TargetDuration: 30,
	}

	jobID, err := s.Studio.EnqueueViral(r.Context(), p)
	if err != nil {
		seeOther(w, r, "/studio?mode=viral&err="+url.QueryEscape("Lỗi tạo job: "+err.Error()))
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("ai_orchestrator", "viral_quick_produce", nil, fmt.Sprintf("1-Chạm tạo video viral 9:16 job=%s: %s", jobID, topic), nil)
	}

	seeOther(w, r, "/studio/jobs?tab=running&notice="+url.QueryEscape(fmt.Sprintf("Đã đưa video đu trend '%s' vào dây chuyền sản xuất AI (Job: %s)!", topic, jobID)))
}

// handleStudioStoryCreate nhận yêu cầu tạo video dài / kể chuyện YouTube & cắt Shorts từ Studio.
func (s *Server) handleStudioStoryCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		seeOther(w, r, "/studio?err="+url.QueryEscape("Studio chưa sẵn sàng — thử tải lại trang."))
		return
	}
	_ = r.ParseMultipartForm(10 << 20)
	_ = r.ParseForm()
	genre := strings.TrimSpace(r.PostFormValue("genre"))
	if genre == "" {
		genre = "tâm lý"
	}
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	isAutoGenerated := false
	if topic == "" {
		topic = studio.DefaultAudioStoryTopic(genre)
		isAutoGenerated = true
	}

	durationMin, _ := strconv.Atoi(r.PostFormValue("duration_minutes"))
	if durationMin <= 0 {
		durationMin = 30
	}
	words, _ := strconv.Atoi(r.PostFormValue("words"))
	scenes, _ := strconv.Atoi(r.PostFormValue("scenes"))
	voice := strings.TrimSpace(r.PostFormValue("voice"))
	autoClip := r.PostFormValue("auto_clip_shorts") == "1" || r.PostFormValue("auto_clip_shorts") == "true"
	clipSec, _ := strconv.Atoi(r.PostFormValue("clip_duration_sec"))
	if clipSec <= 0 {
		clipSec = 180
	}

	musicOn := r.PostFormValue("music_on") == "1"
	musicPath := ""
	if musicOn {
		if mp := filepath.Join(s.DataDir, "autopilot-music.m4a"); fileExists(mp) {
			musicPath = mp
		} else {
			musicOn = false
		}
	}

	p := studio.StoryParams{
		Topic:           topic,
		Genre:           genre,
		DurationMinutes: durationMin,
		Words:           words,
		Scenes:          scenes,
		MusicOn:         musicOn,
		MusicPath:       musicPath,
		Voice:           voice,
		AutoClipShorts:  autoClip,
		ClipDurationSec: clipSec,
	}

	id, err := s.Studio.CreateStoryJob(p)
	if err != nil {
		seeOther(w, r, "/studio?mode=story&err="+url.QueryEscape("Lỗi tạo video truyện: "+err.Error()))
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("ai_orchestrator", "story_produce", nil, fmt.Sprintf("Studio điều phối video truyện dài job=%s duration=%dp genre=%s auto_clip=%v auto_plot=%v: %s", id, durationMin, genre, autoClip, isAutoGenerated, topic), nil)
	}

	clipNote := ""
	if autoClip {
		clipNote = " và tự động cắt clip 3–5 phút cho TikTok/Reels"
	}
	noticePrefix := fmt.Sprintf("Đã đưa video truyện dài %d phút", durationMin)
	if isAutoGenerated {
		noticePrefix = fmt.Sprintf("AI đã tự động nghĩ cốt truyện và đưa video truyện dài %d phút", durationMin)
	}
	seeOther(w, r, "/studio/jobs?tab=running&notice="+url.QueryEscape(fmt.Sprintf("%s '%s' vào hàng đợi AI%s (Job: %s)!", noticePrefix, topic, clipNote, id)))
}

// handleAPIStoryPlots trả về danh sách cốt truyện audio YouTube (AI tự nghĩ sẵn, độc bản).
func (s *Server) handleAPIStoryPlots(w http.ResponseWriter, r *http.Request) {
	genre := r.URL.Query().Get("genre")
	topic := strings.TrimSpace(r.URL.Query().Get("topic"))
	dur, _ := strconv.Atoi(r.URL.Query().Get("duration"))
	if dur <= 0 {
		dur = 30
	}

	plots := studio.GetCuratedAudioStories(genre)
	if topic != "" || r.URL.Query().Get("refresh") == "1" {
		dynamicPlot := studio.GenerateUniqueAudioStory(topic, genre, dur)
		plots = append([]studio.AudioStoryPlot{dynamicPlot}, plots...)
	}

	s.writeJSON(w, map[string]any{
		"ok":    true,
		"plots": plots,
	})
}

// handleAPITikTokTrends trả về dữ liệu JSON của Top 10 Video Đu Trend TikTok Việt & Drama Showbiz (Việt/Trung).
func (s *Server) handleAPITikTokTrends(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		studio.InvalidateTikTokRadarCache()
	}
	ttTrends, err := studio.FetchTikTokTrendingVideos(r.Context())
	dramasVN, dramasCN, _ := studio.FetchSplitShowbizDrama(r.Context())
	combinedDramas := append([]studio.ShowbizDrama(nil), dramasVN...)
	combinedDramas = append(combinedDramas, dramasCN...)

	s.writeJSON(w, map[string]any{
		"ok":             err == nil,
		"tiktok_trends":  ttTrends,
		"dramas_vn":      dramasVN,
		"dramas_cn":      dramasCN,
		"showbiz_dramas": combinedDramas,
	})
}

// handleAPITikTokAnalyze phân tích một URL video TikTok cụ thể do người dùng cung cấp.
func (s *Server) handleAPITikTokAnalyze(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Dữ liệu không hợp lệ"})
		return
	}
	urlStr := r.FormValue("url")
	if urlStr == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Vui lòng nhập link video TikTok"})
		return
	}
	video, err := studio.AnalyzeTikTokVideoURL(r.Context(), urlStr)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.writeJSON(w, map[string]any{"ok": true, "video": video})
}

// handleAPITikTokTopics trả về danh sách các nhóm video TikTok theo chủ đề người dùng cài đặt (mỗi nhóm đúng 4 video/hàng).
func (s *Server) handleAPITikTokTopics(w http.ResponseWriter, r *http.Request) {
	topicsParam := strings.TrimSpace(r.URL.Query().Get("topics"))
	var topicKeys []string
	if topicsParam != "" {
		for _, part := range strings.Split(topicsParam, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				topicKeys = append(topicKeys, p)
			}
		}
	}
	groups := studio.GetConfiguredTopicGroups(topicKeys)
	presets := studio.GetAllPresetTopicGroups()

	s.writeJSON(w, map[string]any{
		"ok":      true,
		"groups":  groups,
		"presets": presets,
	})
}

// handleStudioMuseImport tải video đã tạo từ Muse AI, lồng nhạc trending TikTok và lưu vào Studio.
func (s *Server) handleStudioMuseImport(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		seeOther(w, r, "/web-ai?err="+url.QueryEscape("Studio chưa sẵn sàng"))
		return
	}
	_ = r.ParseForm()
	videoURL := strings.TrimSpace(r.FormValue("video_url"))
	if videoURL == "" {
		seeOther(w, r, "/web-ai?err="+url.QueryEscape("Vui lòng nhập link video từ Muse AI"))
		return
	}
	topic := strings.TrimSpace(r.FormValue("topic"))
	if topic == "" {
		topic = "Video Chuyển Động Muse AI"
	}
	genre := strings.TrimSpace(r.FormValue("genre"))
	if genre == "" {
		genre = "dance"
	}
	account := strings.TrimSpace(r.FormValue("account"))
	musicStyle := strings.TrimSpace(r.FormValue("music_style"))
	if musicStyle == "" {
		musicStyle = "trending"
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	workDir := filepath.Join(s.DataDir, "output", "studio", jobID)
	_ = os.MkdirAll(workDir, 0o755)

	rawVideo := filepath.Join(workDir, "muse_raw.mp4")

	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, videoURL, nil)
	if err != nil {
		seeOther(w, r, "/web-ai?err="+url.QueryEscape("Link video không hợp lệ: "+err.Error()))
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	resp, err := client.Do(req)
	if err != nil {
		seeOther(w, r, "/web-ai?err="+url.QueryEscape("Không thể tải video từ Muse AI: "+err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		seeOther(w, r, "/web-ai?err="+url.QueryEscape(fmt.Sprintf("Muse AI trả về mã HTTP %d", resp.StatusCode)))
		return
	}
	f, err := os.Create(rawVideo)
	if err != nil {
		seeOther(w, r, "/web-ai?err="+url.QueryEscape("Không thể lưu video: "+err.Error()))
		return
	}
	_, _ = io.Copy(f, resp.Body)
	f.Close()

	finalVideoName := fmt.Sprintf("muse_%s.mp4", jobID)
	finalVideoPath := filepath.Join(s.OutDir, finalVideoName)

	nowStr := time.Now().Format("15:04:05")
	logMsg := fmt.Sprintf("%s ✓ Nhập video từ Muse AI thành công: %s\n", nowStr, filepath.Base(rawVideo))

	if musicStyle != "keep_original" {
		trendMusic := studio.FindTrendingMusic(genre)
		if trendMusic != "" {
			logMsg += fmt.Sprintf("%s ✓ Hòa âm nhạc TikTok Trending: %s\n", nowStr, filepath.Base(trendMusic))
			if merr := studio.MuxMusic(r.Context(), rawVideo, trendMusic, 0, finalVideoPath); merr != nil {
				_ = os.Rename(rawVideo, finalVideoPath)
			}
		} else {
			_ = os.Rename(rawVideo, finalVideoPath)
		}
	} else {
		_ = os.Rename(rawVideo, finalVideoPath)
	}

	caption := studio.TemplateCaption(topic, genre)
	paramsJSON, _ := json.Marshal(map[string]any{
		"topic":       topic,
		"genre":       genre,
		"account":     account,
		"source":      "muse_ai",
		"video_url":   videoURL,
		"music_style": musicStyle,
	})
	nowDB := time.Now().Format("2006-01-02 15:04:05")
	if s.db != nil {
		_, _ = s.db.Exec(`INSERT INTO studio_jobs(id, kind, title, status, progress, log, output, params, created_at, finished_at, caption)
			VALUES(?, 'viral', ?, 'done', 100, ?, ?, ?, ?, ?, ?)`,
			jobID, "Muse AI: "+topic, logMsg, finalVideoName, string(paramsJSON), nowDB, nowDB, caption)
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("ai_orchestrator", "viral_produce", nil, fmt.Sprintf("Nhập video từ Muse AI job=%s: %s", jobID, topic), nil)
	}

	seeOther(w, r, "/studio/jobs?tab=done&notice="+url.QueryEscape(fmt.Sprintf("Đã nhập thành công video từ Muse AI và ghép nhạc trending (Job: %s)!", jobID)))
}

// handleAPIFaceSwapStatus trả về thông tin trạng thái hoạt động của Face Swap engine cục bộ.
func (s *Server) handleAPIFaceSwapStatus(w http.ResponseWriter, r *http.Request) {
	st := studio.CheckFaceSwapEngine(r.Context())
	s.writeJSON(w, map[string]any{
		"ok":     true,
		"status": st,
	})
}



