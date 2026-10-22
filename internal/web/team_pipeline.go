package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"net/http"
	"strconv"
	"strings"
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
