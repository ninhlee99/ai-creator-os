package web

import (
	"net/http"
	"strconv"
)

// Đợt H2 (2026-10-04): Agent Team định nghĩa lại quanh 3 pipeline
// (PIVOT_REDESIGN.md §1, Ninh duyệt): thay vì team phục vụ live (đã park),
// mỗi trụ có một team chuyên trách hiển thị dạng cây trạng thái.
//
// TRUNG THỰC: trạng thái suy ra từ dữ liệu thật (studio jobs, kho AT/reup,
// tick) — đây là "bảng sức khỏe pipeline", KHÔNG phải agent thật đang chạy.
// Trang ghi rõ điều này, không bịa trạng thái.

type pipeStage struct {
	Key    string // hunter, producer, ...
	Label  string // tiếng Việt, ngắn
	Icon   string // emoji
	Status string // ok | working | waiting | fail | idle
	Detail string // 1 dòng số liệu thật
}

type pipeTeam struct {
	Key    string
	Name   string
	Icon   string
	Stages []pipeStage
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
			jj := j
			return &studioJobLite{Status: jj.Status, Progress: jj.Progress}
		}
	}
	return nil
}

type studioJobLite struct {
	Status   string
	Progress int
}

// jobStage maps a studio job to a producer stage chip.
func jobStage(label, icon string, j *studioJobLite) pipeStage {
	if j == nil {
		return pipeStage{Key: "producer", Label: label, Icon: icon, Status: tmWaiting, Detail: "chưa có job"}
	}
	switch j.Status {
	case "running":
		return pipeStage{Key: "producer", Label: label, Icon: icon, Status: tmWorking, Detail: "đang render"}
	case "done":
		return pipeStage{Key: "producer", Label: label, Icon: icon, Status: tmOK, Detail: "job mới nhất xong"}
	case "failed":
		return pipeStage{Key: "producer", Label: label, Icon: icon, Status: tmFail, Detail: "job mới nhất lỗi"}
	default:
		return pipeStage{Key: "producer", Label: label, Icon: icon, Status: tmWaiting, Detail: "chờ"}
	}
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
		stages = append(stages, pipeStage{Key: "hunter", Label: "Hunter", Icon: "🔎", Status: tmIdle, Detail: "chưa cấu hình Accesstrade"})
	} else {
		n, _ := s.Products.Count()
		camps, _ := s.AT.CachedCampaigns()
		st := tmWaiting
		if n > 0 {
			st = tmOK
		}
		stages = append(stages, pipeStage{Key: "hunter", Label: "Hunter", Icon: "🔎", Status: st,
			Detail: formatCount("sản phẩm", n) + " · " + formatCount("chiến dịch", int64(len(camps)))})
	}
	// Producer: render video affiliate.
	stages = append(stages, jobStage("Producer", "🎬", s.latestJobByKind("affiliate")))
	// Publisher: video đã xong, sẵn sàng đăng.
	done := s.countJobsByKind("affiliate", "done")
	st := tmWaiting
	if done > 0 {
		st = tmOK
	}
	stages = append(stages, pipeStage{Key: "publisher", Label: "Publisher", Icon: "📤", Status: st, Detail: formatCount("video xong", done)})
	// Analyst: đối soát hoa hồng AT.
	if s.AT == nil {
		stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: tmIdle, Detail: "chưa cấu hình Accesstrade"})
	} else {
		os_, _ := s.AT.GetOrderStats()
		st := tmWaiting
		if os_.ApprovedCount+os_.PendingCount > 0 {
			st = tmOK
		}
		stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: st,
			Detail: formatCount("đơn duyệt", os_.ApprovedCount) + " · " + formatCount("đơn chờ", os_.PendingCount)})
	}
	return pipeTeam{Key: "affiliate", Name: "Affiliate", Icon: "🛒", Stages: stages}
}

func (s *Server) reupTeam() pipeTeam {
	stages := []pipeStage{}
	if s.Reup == nil {
		for _, st := range []pipeStage{
			{Key: "hunter", Label: "Hunter", Icon: "🔎"},
			{Key: "director", Label: "Director", Icon: "📋"},
			{Key: "producer", Label: "Producer", Icon: "🎬"},
			{Key: "qc", Label: "QC", Icon: "✅"},
			{Key: "publisher", Label: "Publisher", Icon: "📤"},
			{Key: "analyst", Label: "Analyst", Icon: "📊"},
		} {
			st.Status = tmIdle
			st.Detail = "chưa cấu hình Reup"
			stages = append(stages, st)
		}
		return pipeTeam{Key: "reup", Name: "Reup", Icon: "🔁", Stages: stages}
	}
	vst, _ := s.Reup.Stats()
	srcs, _ := s.Reup.ListSources()
	// Hunter: nguồn + video chờ tải.
	st := tmWaiting
	if vst.Queued+vst.Downloading > 0 {
		st = tmWorking
	} else if vst.Downloaded > 0 {
		st = tmOK
	}
	stages = append(stages, pipeStage{Key: "hunter", Label: "Hunter", Icon: "🔎", Status: st,
		Detail: formatCount("nguồn", int64(len(srcs))) + " · " + formatCount("chờ tải", vst.Queued)})
	// Director: bài chờ transform (kế hoạch).
	pending, _ := s.Reup.ListPostsByStatus("pending", 1000)
	st = tmWaiting
	if len(pending) > 0 {
		st = tmOK
	}
	stages = append(stages, pipeStage{Key: "director", Label: "Director", Icon: "📋", Status: st, Detail: formatCount("bài chờ transform", int64(len(pending)))})
	// Producer: đang transform.
	transforming, _ := s.Reup.ListPostsByStatus("transforming", 1000)
	st = tmWaiting
	if len(transforming) > 0 {
		st = tmWorking
	}
	stages = append(stages, pipeStage{Key: "producer", Label: "Producer", Icon: "🎬", Status: st, Detail: formatCount("đang transform", int64(len(transforming)))})
	// QC: tải xong (QC đạt) vs lỗi.
	st = tmOK
	if vst.Failed > 0 && vst.Downloaded == 0 {
		st = tmFail
	} else if vst.Failed > 0 {
		st = tmWorking
	}
	stages = append(stages, pipeStage{Key: "qc", Label: "QC", Icon: "✅", Status: st,
		Detail: formatCount("đạt", vst.Downloaded) + " · " + formatCount("lỗi", vst.Failed)})
	// Publisher: bài đã đăng.
	posted, _ := s.Reup.ListPostsByStatus("posted", 1000)
	st = tmWaiting
	if len(posted) > 0 {
		st = tmOK
	}
	stages = append(stages, pipeStage{Key: "publisher", Label: "Publisher", Icon: "📤", Status: st, Detail: formatCount("bài đã đăng", int64(len(posted)))})
	// Analyst: bài lỗi + kill rule 0-view.
	failed, _ := s.Reup.ListPostsByStatus("failed", 1000)
	st = tmOK
	if len(failed) > 0 {
		st = tmFail
	}
	stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: st,
		Detail: formatCount("bài lỗi", int64(len(failed))) + " · kill rule 0-view: 5 bài liên tiếp → dừng"})
	return pipeTeam{Key: "reup", Name: "Reup", Icon: "🔁", Stages: stages}
}

func (s *Server) storyTeam() pipeTeam {
	stages := []pipeStage{}
	j := s.latestJobByKind("story")
	if j == nil {
		for _, st := range []pipeStage{
			{Key: "writer", Label: "Writer", Icon: "✍️"},
			{Key: "illustrator", Label: "Illustrator", Icon: "🎨"},
			{Key: "voice", Label: "Voice", Icon: "🎙️"},
			{Key: "producer", Label: "Producer", Icon: "🎬"},
			{Key: "publisher", Label: "Publisher", Icon: "📤"},
		} {
			st.Status = tmWaiting
			st.Detail = "chưa có truyện"
			stages = append(stages, st)
		}
		return pipeTeam{Key: "story", Name: "Kể chuyện", Icon: "📖", Stages: stages}
	}
	// Ánh xạ tiến độ job → chặng đang làm (ước lượng, ghi rõ trong UI).
	cur := storyStageFor(j)
	keys := []pipeStage{
		{Key: "writer", Label: "Writer", Icon: "✍️"},
		{Key: "illustrator", Label: "Illustrator", Icon: "🎨"},
		{Key: "voice", Label: "Voice", Icon: "🎙️"},
		{Key: "producer", Label: "Producer", Icon: "🎬"},
		{Key: "publisher", Label: "Publisher", Icon: "📤"},
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
			keys[i].Detail = "đang làm"
		default:
			keys[i].Status = tmWaiting
			keys[i].Detail = "chờ"
		}
		stages = append(stages, keys[i])
	}
	return pipeTeam{Key: "story", Name: "Kể chuyện", Icon: "📖", Stages: stages}
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
		s.reupTeam(),
		s.storyTeam(),
	}
	s.render(w, "team", s.ctx("Pipelines", pipelines))
}
