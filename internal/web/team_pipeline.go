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
			Link: "/products", RoleDesc: "Cào & săn deal sản phẩm hoa hồng cao từ sàn"})
	}
	// Producer: render video affiliate.
	stages = append(stages, jobStage("Producer", "🎬", s.latestJobByKind("affiliate"), "/studio#tab-ad-creative", "AI viết kịch bản, 4 Hook 3s & dựng video quảng cáo"))
	// Publisher: video đã xong, sẵn sàng đăng.
	done := s.countJobsByKind("affiliate", "done")
	st := tmWaiting
	if done > 0 {
		st = tmOK
	}
	stages = append(stages, pipeStage{Key: "publisher", Label: "Publisher", Icon: "📤", Status: st, Detail: formatCount("video xong", done), Link: "/accounts", RoleDesc: "Tự động đăng video lên TikTok Shop & Shorts"})
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
			Link: "/products?tab=orders", RoleDesc: "Đối soát đơn hàng, hoa hồng và doanh thu"})
	}
	return pipeTeam{
		Key:         "affiliate",
		Name:        "Affiliate",
		Icon:        "🛒",
		Description: "Tự động săn sản phẩm hot, tạo video bán hàng 9:16 và đăng kênh TikTok Shop / YouTube Shorts.",
		ActionLink:  "/studio",
		ActionLabel: "🎬 Dựng video mới",
		Stages:      stages,
	}
}

func (s *Server) reupTeam() pipeTeam {
	stages := []pipeStage{}
	if s.Reup == nil {
		for _, st := range []pipeStage{
			{Key: "hunter", Label: "Hunter", Icon: "🔎", Link: "/reup", RoleDesc: "Quét & tải video Douyin không watermark"},
			{Key: "director", Label: "Director", Icon: "📋", Link: "/reup", RoleDesc: "Lên kế hoạch đảo góc nội dung"},
			{Key: "producer", Label: "Producer", Icon: "🎬", Link: "/reup", RoleDesc: "Biên tập video & chèn hiệu ứng"},
			{Key: "qc", Label: "QC", Icon: "✅", Link: "/reup", RoleDesc: "Kiểm tra chất lượng video & bản quyền"},
			{Key: "publisher", Label: "Publisher", Icon: "📤", Link: "/accounts", RoleDesc: "Đăng video lên các kênh mạng xã hội"},
			{Key: "analyst", Label: "Analyst", Icon: "📊", Link: "/reup", RoleDesc: "Giám sát view & kill format lỗi"},
		} {
			st.Status = tmIdle
			st.Detail = "chưa cấu hình Reup"
			stages = append(stages, st)
		}
		return pipeTeam{
			Key:         "reup",
			Name:        "Reup",
			Icon:        "🔁",
			Description: "Tự động quét video viral Douyin, biến đổi lách bản quyền và đăng tải đa nền tảng.",
			ActionLink:  "/reup",
			ActionLabel: "🔁 Quản lý Reup",
			Stages:      stages,
		}
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
		Detail: formatCount("nguồn", int64(len(srcs))) + " · " + formatCount("chờ tải", vst.Queued),
		Link: "/reup", RoleDesc: "Quét tài khoản Douyin và tải video viral không watermark"})
	// Director: bài chờ transform (kế hoạch).
	pending, _ := s.Reup.ListPostsByStatus("pending", 1000)
	st = tmWaiting
	if len(pending) > 0 {
		st = tmOK
	}
	stages = append(stages, pipeStage{Key: "director", Label: "Director", Icon: "📋", Status: st, Detail: formatCount("bài chờ transform", int64(len(pending))), Link: "/reup", RoleDesc: "Lên kế hoạch đảo góc nội dung tránh quét trùng"})
	// Producer: đang transform.
	transforming, _ := s.Reup.ListPostsByStatus("transforming", 1000)
	st = tmWaiting
	if len(transforming) > 0 {
		st = tmWorking
	}
	stages = append(stages, pipeStage{Key: "producer", Label: "Producer", Icon: "🎬", Status: st, Detail: formatCount("đang transform", int64(len(transforming))), Link: "/reup", RoleDesc: "Biên tập video, tăng tốc, cắt cúp và chèn hiệu ứng"})
	// QC: tải xong (QC đạt) vs lỗi.
	st = tmOK
	if vst.Failed > 0 && vst.Downloaded == 0 {
		st = tmFail
	} else if vst.Failed > 0 {
		st = tmWorking
	}
	stages = append(stages, pipeStage{Key: "qc", Label: "QC", Icon: "✅", Status: st,
		Detail: formatCount("đạt", vst.Downloaded) + " · " + formatCount("lỗi", vst.Failed), Link: "/reup", RoleDesc: "Kiểm tra chất lượng video, bitrate và âm thanh"})
	// Publisher: bài đã đăng.
	posted, _ := s.Reup.ListPostsByStatus("posted", 1000)
	st = tmWaiting
	if len(posted) > 0 {
		st = tmOK
	}
	stages = append(stages, pipeStage{Key: "publisher", Label: "Publisher", Icon: "📤", Status: st, Detail: formatCount("bài đã đăng", int64(len(posted))), Link: "/accounts", RoleDesc: "Đăng video lên các kênh mạng xã hội theo lịch"})
	// Analyst: bài lỗi + kill rule 0-view.
	failed, _ := s.Reup.ListPostsByStatus("failed", 1000)
	st = tmOK
	if len(failed) > 0 {
		st = tmFail
	}
	stages = append(stages, pipeStage{Key: "analyst", Label: "Analyst", Icon: "📊", Status: st,
		Detail: formatCount("bài lỗi", int64(len(failed))) + " · kill rule 0-view: 5 bài liên tiếp → dừng", Link: "/reup", RoleDesc: "Giám sát lượt xem và tự động dừng format 0-view"})
	return pipeTeam{
		Key:         "reup",
		Name:        "Reup",
		Icon:        "🔁",
		Description: "Tự động quét video viral Douyin, biến đổi lách bản quyền và đăng tải đa nền tảng.",
		ActionLink:  "/reup",
		ActionLabel: "🔁 Quản lý Reup",
		Stages:      stages,
	}
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
		s.reupTeam(),
		s.storyTeam(),
	}
	s.render(w, "team", s.ctx("Pipelines", pipelines))
}
