package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// viralTrend gợi ý góc tiếp cận và hook mở đầu cho video viral.
type viralTrend struct {
	Title    string
	Genre    string
	GenreVN  string
	Hook3s   string
	EstViews string
}

// viralJobView là dữ liệu hiển thị của một tác vụ dựng video viral 9:16.
type viralJobView struct {
	ID         string
	Title      string
	Topic      string
	Genre      string
	GenreVN    string
	Status     string
	StatusVN   string
	Progress   int
	MediaURL   string
	LogTail    string
	CreatedAt  string
	FinishedAt string
}

// viralView là mô hình dữ liệu chính cho trang /viral.
type viralView struct {
	StudioOK     bool
	Jobs         []viralJobView
	AccountNames []string
	AutoOn       bool
	ActiveCount  int
	DoneCount    int
	TotalCount   int
	Trends       []viralTrend
	Error        string
	Notice       string
}

// Các mẫu xu hướng và hook tâm lý học có tỷ lệ giữ chân cao (Curiosity & Shock hooks).
var defaultViralTrends = []viralTrend{
	{
		Title:    "Sự thật rúng động về AI mà 99% mọi người chưa từng nghe",
		Genre:    "mystery",
		GenreVN:  "Bí ẩn & Giật gân",
		Hook3s:   "Dừng lại 3 giây! Nếu bạn nghĩ AI chỉ là chatbot, bạn đang bỏ lỡ điều này...",
		EstViews: "2.4M",
	},
	{
		Title:    "Top 3 sai lầm khiến người mới cháy sạch tiền khi kinh doanh số",
		Genre:    "lifehack",
		GenreVN:  "Kinh nghiệm & Đời sống",
		Hook3s:   "Tại sao 90% người khởi nghiệp thất bại trong tháng đầu tiên?",
		EstViews: "1.8M",
	},
	{
		Title:    "Bí quyết xây dựng thói quen siêu năng suất của các tỷ phú công nghệ",
		Genre:    "trendjack",
		GenreVN:  "Xu hướng nóng",
		Hook3s:   "Quy tắc 5 giây này đã thay đổi hoàn toàn sự nghiệp của Elon Musk...",
		EstViews: "3.1M",
	},
	{
		Title:    "Đột phá công nghệ 2026: Những công cụ thay đổi hoàn toàn tương lai",
		Genre:    "tech_fact",
		GenreVN:  "Công nghệ & Tri thức",
		Hook3s:   "Năm 2026, những ai không biết 3 công cụ này sẽ bị bỏ lại phía sau...",
		EstViews: "1.2M",
	},
	{
		Title:    "Đánh giá chi tiết thiết bị sáng tạo nội dung đáng tiền nhất năm nay",
		Genre:    "koc_review",
		GenreVN:  "Đánh giá & Review",
		Hook3s:   "Đừng vội mua thiết bị này trước khi xem hết video 30 giây này!",
		EstViews: "950K",
	},
}

func viralGenreVN(genre string) string {
	switch genre {
	case "mystery":
		return "Bí ẩn & Giật gân"
	case "lifehack":
		return "Kinh nghiệm & Đời sống"
	case "trendjack":
		return "Xu hướng nóng"
	case "tech_fact":
		return "Công nghệ & Tri thức"
	case "koc_review":
		return "Đánh giá & Review"
	default:
		return "Đa dạng"
	}
}

func viralStatusVN(st string) string {
	switch st {
	case studio.StatusQueued:
		return "Chờ render"
	case studio.StatusRunning:
		return "Đang sản xuất AI"
	case studio.StatusDone:
		return "Hoàn thành 100%"
	case studio.StatusFailed:
		return "Gặp lỗi"
	default:
		return st
	}
}

// handleViral hiển thị trang tổng quan Xưởng Sáng Tạo Video Viral AI 100% Gốc.
func (s *Server) handleViral(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v := viralView{
		StudioOK: s.Studio != nil,
		Trends:   defaultViralTrends,
		Error:    q.Get("err"),
	}

	if msg := q.Get("msg"); msg == "created" {
		v.Notice = "Tác vụ sản xuất video viral AI đã được đưa vào hàng đợi thành công!"
	}

	// Đọc công tắc tự động hóa viral
	st := s.settings()
	if val, ok := st.Get("viral.auto_produce_enabled"); ok {
		v.AutoOn = val == "1" || val == "true"
	} else {
		v.AutoOn = true
	}

	// Lấy danh sách kênh
	if s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, a := range accounts {
				if a != nil && a.Username != "" {
					v.AccountNames = append(v.AccountNames, a.Username)
				}
			}
		}
	}

	if s.Studio != nil {
		jobs := s.Studio.ListViralJobs(40)
		v.TotalCount = len(jobs)
		for _, j := range jobs {
			var p studio.ViralParams
			_ = studioUnmarshalJSON(j.Params, &p)

			var mediaURL string
			if j.Status == studio.StatusDone && j.Output != "" {
				mediaURL = "/media/" + filepath.Base(j.Output)
			}

			topic := p.Topic
			if topic == "" {
				topic = j.Title
			}

			lines := strings.Split(strings.TrimSpace(j.Log), "\n")
			tail := ""
			if len(lines) > 0 {
				tail = lines[len(lines)-1]
			}

			jv := viralJobView{
				ID:         j.ID,
				Title:      j.Title,
				Topic:      topic,
				Genre:      p.Genre,
				GenreVN:    viralGenreVN(p.Genre),
				Status:     j.Status,
				StatusVN:   viralStatusVN(j.Status),
				Progress:   j.Progress,
				MediaURL:   mediaURL,
				LogTail:    tail,
				CreatedAt:  j.CreatedAt,
				FinishedAt: j.FinishedAt,
			}
			v.Jobs = append(v.Jobs, jv)

			if j.Status == studio.StatusRunning || j.Status == studio.StatusQueued {
				v.ActiveCount++
			} else if j.Status == studio.StatusDone {
				v.DoneCount++
			}
		}
	}

	s.render(w, "viral", s.ctx("Viral", v))
}

// handleViralCreate tiếp nhận form khởi tạo video viral mới.
func (s *Server) handleViralCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Studio AI chưa sẵn sàng"), http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	if topic == "" {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Vui lòng nhập chủ đề video"), http.StatusSeeOther)
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

	p := studio.ViralParams{
		Topic:          topic,
		Genre:          genre,
		VisualMood:     visualMood,
		Voice:          voice,
		Account:        account,
		TargetDuration: duration,
		MusicURL:       musicURL,
	}

	jobID, err := s.Studio.EnqueueViral(r.Context(), p)
	if err != nil {
		log.Printf("web: viral create error: %v", err)
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Lỗi tạo job: "+err.Error()), http.StatusSeeOther)
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("human", "viral_produce", nil, fmt.Sprintf("Khởi tạo video viral 100%% gốc job=%s genre=%s: %s", jobID, genre, topic), nil)
	}

	http.Redirect(w, r, "/viral?msg=created", http.StatusSeeOther)
}

// handleViralQuick khởi tạo tức thì từ mẫu xu hướng 1-chạm.
func (s *Server) handleViralQuick(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Studio AI chưa sẵn sàng"), http.StatusSeeOther)
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
		http.Redirect(w, r, "/viral?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("human", "viral_produce", nil, fmt.Sprintf("1-Chạm tạo video viral job=%s: %s", jobID, topic), nil)
	}

	http.Redirect(w, r, "/viral?msg=created", http.StatusSeeOther)
}

// studioUnmarshalJSON giải mã json chuỗi an toàn.
func studioUnmarshalJSON(data string, v any) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), v)
}
