package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/growth"
)

// Đợt H2 (2026-10-04): Agent Team định nghĩa lại quanh 3 pipeline
// (PIVOT_REDESIGN.md §1, Ninh duyệt): thay vì team phục vụ live (đã park),
// mỗi trụ có một team chuyên trách hiển thị dạng cây trạng thái.
//
// TRUNG THỰC: trạng thái suy ra từ dữ liệu thật (studio jobs, kho AT/reup,
// tick) — đây là "bảng sức khỏe pipeline", KHÔNG phải agent thật đang chạy.
// Trang ghi rõ điều này, không bịa trạng thái.

type pipeStage struct {
	Key      string // hunter, producer, ...
	Label    string // tiếng Việt, ngắn
	Icon     string // emoji
	Status   string // ok | working | waiting | fail | idle
	Detail   string // 1 dòng số liệu thật
	Link     string // đường dẫn tới công cụ thực tế
	RoleDesc string // giải thích nhiệm vụ ngắn gọn
}

type pipeTeam struct {
	Key         string
	Name        string
	Icon        string
	Description string
	ActionLink  string
	ActionLabel string
	Stages      []pipeStage
}

// stage statuses
const (
	tmOK      = "ok"      // xong / đang tốt
	tmWorking = "working" // đang chạy
	tmWaiting = "waiting" // chờ việc
	tmFail    = "fail"    // lỗi cần xem
	tmIdle    = "idle"    // chưa cấu hình
)

// latestJobByKind returns the newest studio job of a kind, or nil.
func (s *Server) latestJobByKind(kind string) *studioJobLite {
	if s.Studio == nil {
		return nil
	}
	for _, j := range s.Studio.ListJobs(30) {
		if j.Kind == kind {
			full, ok := s.Studio.GetJob(j.ID)
			if !ok {
				return &studioJobLite{ID: j.ID, Status: j.Status, Progress: j.Progress}
			}
			qcStatus, _ := pipelineMarkerState(full.Log, "VISUAL_QC_START:", "VISUAL_QC_PASS:", "VISUAL_QC_FAIL:")
			return &studioJobLite{ID: full.ID, Status: full.Status, Progress: full.Progress, QCStatus: qcStatus, Log: full.Log, Output: full.Output}
		}
	}
	return nil
}

type studioJobLite struct {
	ID       string
	Status   string
	Progress int
	QCStatus string
	Log      string
	Output   string
}

func visualQCStage(label, icon string, j *studioJobLite, link, roleDesc string) pipeStage {
	stage := pipeStage{Key: "qc", Label: label, Icon: icon, Link: link, RoleDesc: roleDesc, Status: tmWaiting, Detail: "chờ job chạy visual QC"}
	if j == nil {
		stage.Detail = "chưa có job"
		return stage
	}
	switch j.QCStatus {
	case "pass":
		stage.Status = tmOK
		stage.Detail = visualQCLogDetail(j.Log, "VISUAL_QC_PASS:", "đã qua visual QC")
	case "fail":
		stage.Status = tmFail
		stage.Detail = visualQCLogDetail(j.Log, "VISUAL_QC_FAIL:", "visual QC chặn job")
	case "running":
		stage.Status = tmWorking
		stage.Detail = "đang phân tích frame"
	default:
		if j.Status == "done" {
			stage.Detail = "job cũ; chưa có bằng chứng visual QC"
		} else if j.Status == "failed" {
			stage.Detail = "job lỗi; không ghi nhận kết quả visual QC"
		}
	}
	return stage
}

func visualQCLogDetail(log, marker, fallback string) string {
	for _, line := range strings.Split(log, "\n") {
		if i := strings.Index(line, marker); i >= 0 {
			if detail := strings.TrimSpace(line[i+len(marker):]); detail != "" {
				return detail
			}
		}
	}
	return fallback
}

// jobStage maps a studio job to a producer stage chip.
func jobStage(label, icon string, j *studioJobLite, link, roleDesc string) pipeStage {
	res := pipeStage{Key: "producer", Label: label, Icon: icon, Link: link, RoleDesc: roleDesc}
	if j == nil {
		res.Status = tmWaiting
		res.Detail = "chưa có job"
		return res
	}
	switch j.Status {
	case "running":
		res.Status = tmWorking
		res.Detail = "đang render"
	case "done":
		res.Status = tmOK
		res.Detail = "job mới nhất xong"
	case "failed":
		res.Status = tmFail
		res.Detail = "job mới nhất lỗi"
	default:
		res.Status = tmWaiting
		res.Detail = "chờ"
	}
	return res
}

// countJobsByKind counts studio jobs of a kind with the given status.
func (s *Server) countJobsByKind(kind, status string) int64 {
	if s.Studio == nil {
		return 0
	}
	var n int64
	for _, j := range s.Studio.ListJobs(200) {
		if j.Kind == kind && (status == "" || j.Status == status) {
			n++
		}
	}
	return n
}

func (s *Server) affiliateTeam() pipeTeam {
	stages := []pipeStage{}
	// Hunter: quét AT datafeed → kho sản phẩm.
	if s.AT == nil || s.Products == nil {
		stages = append(stages, pipeStage{Key: "hunter", Label: "Hunter", Icon: "🔎", Status: tmIdle, Detail: "chưa cấu hình Accesstrade", Link: "/products", RoleDesc: "Cào & săn deal sản phẩm hoa hồng cao từ sàn"})
	} else {
		n, _ := s.Products.Count()
		camps, _ := s.AT.CachedCampaigns()
		st := tmWaiting
		if n > 0 {
			st = tmOK
		}
		stages = append(stages, pipeStage{Key: "hunter", Label: "Hunter", Icon: "🔎", Status: st,
			Detail: formatCount("sản phẩm", n) + " · " + formatCount("chiến dịch", int64(len(camps))),
			Link:   "/products", RoleDesc: "Cào & săn deal sản phẩm hoa hồng cao từ sàn"})
	}
	// These roles map to real, sequential job artifacts and log markers.
	// They are pipeline stages, not independent agents running in parallel.
	affiliateJob := s.latestJobByKind("affiliate")
	stages = append(stages, affiliateMarkedStage("Product Analyst", "🔬", affiliateJob,
		"PRODUCT_ANALYSIS_START:", "PRODUCT_ANALYSIS_PASS:", "PRODUCT_ANALYSIS_FAIL:",
		"Đọc ảnh listing, phân loại mặt hàng và ghi lại chi tiết nhìn thấy; không bịa claims."))
	stages = append(stages, affiliateMarkedStage("Director", "🎥", affiliateJob,
		"DIRECTOR_START:", "DIRECTOR_PASS:", "DIRECTOR_FAIL:",
		"Lập shot list theo cách dùng, focus sản phẩm, coverage và story arc."))
	stages = append(stages, affiliateMarkedStage("Producer / Editor", "🎬", affiliateJob,
		"PRODUCTION_START:", "PRODUCTION_PASS:", "PRODUCTION_FAIL:",
		"Chuẩn hóa clip chuyển động người vận hành cung cấp, nối cảnh và hòa âm; không gọi video API tính phí."))
	stages = append(stages, visualQCStage("QC thị giác", "🛡️", affiliateJob, "/studio/jobs?tab=running", "Chặn footage đứng hình ở 2 fps; đối chiếu sản phẩm, nhân vật, cảm xúc, nét, bố cục và continuity trên ba frame mỗi shot"))
	// Output: video đã render; publish không được coi là đã xong nếu chưa có post record.
	done := s.countJobsByKind("affiliate", "done")
	verified := s.countQCVerifiedJobsByKind("affiliate")
	outputStatus := tmWaiting
	if verified > 0 {
		outputStatus = tmOK
	}
	stages = append(stages, pipeStage{Key: "output", Label: "Đầu ra", Icon: "📤", Status: outputStatus,
		Detail: fmt.Sprintf("%d video qua QC · %d chưa xác minh", verified, done-verified), Link: "/studio/jobs?tab=done",
		RoleDesc: "Job hoàn tất chưa có nghĩa video đã được đăng lên kênh"})
	stages = append(stages, s.growthAnalyticsStage())
	// Analyst: đối soát hoa hồng AT.
	if s.AT == nil {
		stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: tmIdle, Detail: "chưa cấu hình Accesstrade", Link: "/products?tab=orders", RoleDesc: "Đối soát đơn hàng, hoa hồng và doanh thu"})
	} else {
		os_, _ := s.AT.GetOrderStats()
		st := tmWaiting
		if os_.ApprovedCount+os_.PendingCount > 0 {
			st = tmOK
		}
		stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: st,
			Detail: formatCount("đơn duyệt", os_.ApprovedCount) + " · " + formatCount("đơn chờ", os_.PendingCount),
			Link:   "/products?tab=orders", RoleDesc: "Đối soát đơn hàng, hoa hồng và doanh thu"})
	}
	return pipeTeam{
		Key:         "affiliate",
		Name:        "Affiliate",
		Icon:        "🛒",
		Description: "Trạng thái từng vai trò lấy từ artifact/log job thật. Đây là các chặng tuần tự, không giả lập agent độc lập hoặc kết quả tăng trưởng.",
		ActionLink:  "/studio",
		ActionLabel: "🎬 Dựng video mới",
		Stages:      stages,
	}
}

func (s *Server) countQCVerifiedJobsByKind(kind string) int64 {
	if s.Studio == nil {
		return 0
	}
	var count int64
	for _, job := range s.Studio.ListJobs(200) {
		if job.Kind != kind || job.Status != "done" {
			continue
		}
		full, ok := s.Studio.GetJob(job.ID)
		if ok && studioOutputQCVerified(full) {
			count++
		}
	}
	return count
}

func (s *Server) growthAnalyticsStage() pipeStage {
	stage := pipeStage{Key: "growth-analyst", Label: "Growth Analyst", Icon: "📈", Status: tmIdle,
		Detail: "chưa có analytics video được liên kết format; chưa kết luận tăng trưởng", Link: "/growth",
		RoleDesc: "Đọc views, shares, average view %, subscriber gain từ YouTube Analytics; không suy diễn khi thiếu dữ liệu"}
	if s.Growth == nil || s.Mgr == nil {
		return stage
	}
	accounts, err := s.Mgr.List()
	if err != nil {
		stage.Detail = "không đọc được tài khoản để đối chiếu analytics"
		return stage
	}
	var videos, winners int
	for _, account := range accounts {
		stats, err := s.Growth.ListFormatStats(account.ID)
		if err != nil {
			continue
		}
		for _, stat := range stats {
			videos += stat.Videos
			if stat.Verdict == growth.VerdictWinner {
				winners++
			}
		}
	}
	if videos > 0 {
		stage.Status = tmOK
		stage.Detail = fmt.Sprintf("%d video có analytics thật · %d format được đánh dấu winner", videos, winners)
	}
	return stage
}

func affiliateMarkedStage(label, icon string, job *studioJobLite, startMarker, passMarker, failMarker, roleDesc string) pipeStage {
	key := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(label, " / ", "-"), " ", "-"))
	stage := pipeStage{Key: key, Label: label, Icon: icon,
		Link: "/studio/jobs?tab=running", RoleDesc: roleDesc, Status: tmWaiting, Detail: "chờ job affiliate"}
	if job == nil {
		stage.Detail = "chưa có job"
		return stage
	}
	if strings.Contains(job.Log, passMarker) {
		stage.Status = tmOK
		stage.Detail = visualQCLogDetail(job.Log, passMarker, "đã hoàn tất")
		return stage
	}
	if strings.Contains(job.Log, failMarker) {
		stage.Status = tmFail
		stage.Detail = visualQCLogDetail(job.Log, failMarker, "bước này bị chặn")
		return stage
	}
	if strings.Contains(job.Log, startMarker) {
		if job.Status == "failed" {
			stage.Status = tmFail
			stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "job lỗi tại bước này")
		} else if job.Status == "running" {
			stage.Status = tmWorking
			stage.Detail = "đang chạy"
		}
		return stage
	}
	if job.Status == "done" {
		stage.Detail = "job cũ; chưa có bằng chứng cho chặng này"
	} else if job.Status == "failed" {
		stage.Detail = "job lỗi trước khi chặng này bắt đầu"
	}
	return stage
}

func (s *Server) viralTeam() pipeTeam {
	latest := s.latestJobByKind("viral")
	stages := []pipeStage{
		{Key: "input", Label: "Nguồn & kênh", Icon: "📥", Link: "/viral", RoleDesc: "Xác nhận video dọc, ảnh nhân vật và kênh đã chọn"},
		{Key: "identity", Label: "Thay toàn nhân vật", Icon: "👤", Link: "/viral", RoleDesc: "Edit toàn thân trên video nguồn; face-only không đạt yêu cầu"},
		{Key: "media", Label: "Kiểm tra video", Icon: "🎞️", Link: "/studio/jobs?tab=running", RoleDesc: "Ghép các đoạn edit, khôi phục audio nguồn, kiểm tra thời lượng và khung 9:16"},
		{Key: "qc", Label: "QC thị giác", Icon: "🛡️", Link: "/studio/jobs?tab=running", RoleDesc: "So sánh frame nguồn/đầu ra: nhân vật, cảnh, hành động và camera"},
		{Key: "output", Label: "Đầu ra", Icon: "📤", Link: "/studio/jobs?tab=done", RoleDesc: "Job hoàn tất sau khi kiểm tra media và visual QC đạt"},
	}
	if latest == nil {
		for i := range stages {
			stages[i].Status = tmIdle
			stages[i].Detail = "chưa có job"
		}
	} else {
		stages[0] = markedPipelineStage(stages[0], latest, "SOURCE_VALIDATION_START:", "SOURCE_VALIDATION_PASS:")
		stages[1] = markedPipelineStage(stages[1], latest, "CHARACTER_REPLACEMENT_START:", "CHARACTER_REPLACEMENT_PASS:")
		stages[2] = markedPipelineStage(stages[2], latest, "OUTPUT_VALIDATION_START:", "OUTPUT_VALIDATION_PASS:")
		stages[3] = visualQCStage(stages[3].Label, stages[3].Icon, latest, stages[3].Link, stages[3].RoleDesc)
		if latest.Status == "done" && latest.Output != "" && latest.QCStatus == "pass" {
			stages[4].Status = tmOK
			stages[4].Detail = "video đầu ra đã qua visual QC"
		} else if latest.Status == "done" && latest.Output != "" {
			stages[4].Status = tmWaiting
			stages[4].Detail = "render xong; chưa có bằng chứng visual QC"
		} else if latest.Status == "failed" {
			stages[4].Status = tmFail
			stages[4].Detail = "job lỗi; không có đầu ra hoàn chỉnh"
		} else {
			stages[4].Status = tmWaiting
			stages[4].Detail = "chờ các bước kiểm tra"
		}
	}
	return pipeTeam{
		Key:         "viral",
		Name:        "Video Viral",
		Icon:        "🔥",
		Description: "Các chặng lấy trạng thái từ log job; không hiển thị agent, voice hoặc đăng kênh nếu không có worker tương ứng.",
		ActionLink:  "/viral",
		ActionLabel: "🔥 Xưởng Video Viral",
		Stages:      stages,
	}
}

func markedPipelineStage(stage pipeStage, job *studioJobLite, startMarker, passMarker string) pipeStage {
	failMarker := strings.TrimSuffix(passMarker, "PASS:") + "FAIL:"
	state, _ := pipelineMarkerState(job.Log, startMarker, passMarker, failMarker)
	if state == "pass" {
		stage.Status = tmOK
		stage.Detail = visualQCLogDetail(job.Log, passMarker, "đã hoàn tất")
		return stage
	}
	if state == "fail" {
		stage.Status = tmFail
		stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "job lỗi tại bước này")
		return stage
	}
	if state == "running" {
		switch job.Status {
		case "running":
			stage.Status = tmWorking
			stage.Detail = "đang chạy"
		case "failed":
			stage.Status = tmFail
			stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "job lỗi tại bước này")
		default:
			stage.Status = tmWaiting
			stage.Detail = "đang chờ"
		}
		return stage
	}
	if job.Status == "failed" && !strings.Contains(job.Log, "SOURCE_VALIDATION_PASS:") {
		stage.Status = tmFail
		stage.Detail = visualQCLogDetail(job.Log, "LỖI:", "đầu vào không hợp lệ")
		return stage
	}
	stage.Status = tmWaiting
	stage.Detail = "chờ bước trước"
	return stage
}

func (s *Server) storyTeam() pipeTeam {
	stages := []pipeStage{}
	j := s.latestJobByKind("story")
	if j == nil {
		for _, st := range []pipeStage{
			{Key: "writer", Label: "Writer", Icon: "✍️", Link: "/stories", RoleDesc: "Viết kịch bản cốt truyện lôi cuốn"},
			{Key: "illustrator", Label: "Illustrator", Icon: "🎨", Link: "/stories", RoleDesc: "Vẽ tranh minh họa từng cảnh bằng AI"},
			{Key: "voice", Label: "Voice", Icon: "🎙️", Link: "/stories", RoleDesc: "Đọc giọng truyền cảm xúc bằng AI Voice TTS"},
			{Key: "producer", Label: "Producer", Icon: "🎬", Link: "/stories", RoleDesc: "Ghép cảnh, hòa âm và dựng video 16:9"},
			{Key: "publisher", Label: "Publisher", Icon: "📤", Link: "/accounts", RoleDesc: "Xuất bản video lên YouTube"},
		} {
			st.Status = tmWaiting
			st.Detail = "chưa có truyện"
			stages = append(stages, st)
		}
		return pipeTeam{
			Key:         "story",
			Name:        "Kể chuyện",
			Icon:        "📖",
			Description: "Tự động sáng tác truyện dài, vẽ minh họa từng cảnh và đọc giọng truyền cảm xúc.",
			ActionLink:  "/stories",
			ActionLabel: "📖 Viết truyện mới",
			Stages:      stages,
		}
	}
	// Ánh xạ tiến độ job → chặng đang làm (ước lượng, ghi rõ trong UI).
	cur := storyStageFor(j)
	keys := []pipeStage{
		{Key: "writer", Label: "Writer", Icon: "✍️", Link: "/stories", RoleDesc: "Viết kịch bản cốt truyện lôi cuốn"},
		{Key: "illustrator", Label: "Illustrator", Icon: "🎨", Link: "/stories", RoleDesc: "Vẽ tranh minh họa từng cảnh bằng AI"},
		{Key: "voice", Label: "Voice", Icon: "🎙️", Link: "/stories", RoleDesc: "Đọc giọng truyền cảm xúc bằng AI Voice TTS"},
		{Key: "producer", Label: "Producer", Icon: "🎬", Link: "/stories", RoleDesc: "Ghép cảnh, hòa âm và dựng video 16:9"},
		{Key: "publisher", Label: "Publisher", Icon: "📤", Link: "/accounts", RoleDesc: "Xuất bản video lên YouTube"},
	}
	for i := range keys {
		switch {
		case j.Status == "failed":
			keys[i].Status = tmFail
			keys[i].Detail = "job lỗi"
		case j.Status == "done":
			keys[i].Status = tmOK
			keys[i].Detail = "xong"
		case i < cur:
			keys[i].Status = tmOK
			keys[i].Detail = "xong"
		case i == cur:
			keys[i].Status = tmWorking
			keys[i].Detail = "đang làm (ước lượng)"
		default:
			keys[i].Status = tmWaiting
			keys[i].Detail = "chờ"
		}
		stages = append(stages, keys[i])
	}
	return pipeTeam{
		Key:         "story",
		Name:        "Kể chuyện",
		Icon:        "📖",
		Description: "Tự động sáng tác truyện dài, vẽ minh họa từng cảnh và đọc giọng truyền cảm xúc.",
		ActionLink:  "/stories",
		ActionLabel: "📖 Viết truyện mới",
		Stages:      stages,
	}
}

// storyStageFor ước lượng chặng hiện tại từ tiến độ job (0-100).
func storyStageFor(j *studioJobLite) int {
	switch {
	case j.Progress < 20:
		return 0 // writer
	case j.Progress < 45:
		return 1 // illustrator
	case j.Progress < 65:
		return 2 // voice
	case j.Progress < 90:
		return 3 // producer
	default:
		return 4 // publisher
	}
}

func formatCount(label string, n int64) string {
	return label + ": " + strconv.FormatInt(n, 10)
}

func (s *Server) handleTeamPipelines(w http.ResponseWriter, r *http.Request) {
	pipelines := []pipeTeam{
		s.affiliateTeam(),
		s.viralTeam(),
		s.storyTeam(),
	}
	s.render(w, "team", s.ctx(
		"Pipelines", pipelines,
		"Notice", r.URL.Query().Get("ok"),
		"Error", r.URL.Query().Get("err"),
	))
}
