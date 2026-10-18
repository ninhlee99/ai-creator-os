package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// viralTrend gợi ý góc tiếp cận và hook mở đầu cho video viral.
type viralTrend struct {
	Title          string
	Genre          string
	GenreVN        string
	Hook3s         string
	FormatTag      string
	SampleClip     string
	SampleVideoURL string
	SampleThumbURL string
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
	SRTURL     string
	LogTail    string
	CreatedAt  string
	FinishedAt string
}

// sampleClipView là mô hình hiển thị một video mẫu chuyển động (vũ đạo / lookbook).
type sampleClipView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Filename    string `json:"filename"`
	URL         string `json:"url"`
	Duration    string `json:"duration"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// viralCreatorChannel là dữ liệu của các kênh nữ hàng chục triệu follow đang tạo trend hot nhất.
type viralCreatorChannel struct {
	Handle     string `json:"handle"`
	Name       string `json:"name"`
	Followers  string `json:"followers"`
	Category   string `json:"category"`
	AvatarURL  string `json:"avatar_url"`
	HotTrend   string `json:"hot_trend"`
	Music      string `json:"music"`
	Views      string `json:"views"`
	SampleClip string `json:"sample_clip"`
	VideoURL   string `json:"video_url"`
	Hook       string `json:"hook"`
}

// viralView là mô hình dữ liệu chính cho trang /viral.
type viralView struct {
	StudioOK           bool
	Jobs               []viralJobView
	AccountNames       []string
	AutoOn             bool
	ActiveCount        int
	DoneCount          int
	TotalCount         int
	Trends             []viralTrend
	Error              string
	Notice             string
	AvailableModels    []modelPhotoCardView
	SelectedModelID    int64
	SelectedModelPhoto string
	SampleClips        []sampleClipView
	SelectedClip       string
	TopChannels        []viralCreatorChannel
}

// defaultViralClips danh sách các video vũ đạo mẫu chân thực có sẵn trong hệ thống.
var defaultViralClips = []sampleClipView{
	{
		ID:          "clip_03_sexy_dance_60s",
		Title:       "Vũ đạo phòng ngủ 60s (Chuẩn TikTok 1 phút+)",
		Filename:    "clip_03_sexy_dance_60s.mp4",
		URL:         "/clips/viral/clip_03_sexy_dance_60s.mp4",
		Duration:    "60 giây (1080x1920)",
		Category:    "viral",
		Description: "Video tự quay phòng ngủ chân thực 100%, không sân khấu, không logo, chuyển động toàn thân sắc nét.",
	},
	{
		ID:          "clip_01_dance_duo",
		Title:       "Vũ đạo đường phố đôi năng động 45s",
		Filename:    "clip_01_dance_duo.mp4",
		URL:         "/clips/viral/clip_01_dance_duo.mp4",
		Duration:    "45 giây (1080x1920)",
		Category:    "viral",
		Description: "Nhịp điệu đường phố bắt trend, góc quay chuyên nghiệp, chuyển động mượt mà.",
	},
	{
		ID:          "clip_02_dance_crew",
		Title:       "Nhảy vũ đoàn hiện đại K-Pop 60s",
		Filename:    "clip_02_dance_crew.mp4",
		URL:         "/clips/viral/clip_02_dance_crew.mp4",
		Duration:    "60 giây (1080x1920)",
		Category:    "viral",
		Description: "Vũ đạo nhóm đồng đều, sôi động, phong cách idol năng lượng cao.",
	},
}

// Các mẫu xu hướng và hook tâm lý học có tỷ lệ giữ chân cao (Curiosity & Shock hooks).
var defaultViralTrends = []viralTrend{
	{
		Title:          "Vũ Điệu Triệu View: Hot Dance Sexy Trend TikTok",
		Genre:          "dance",
		GenreVN:        "Vũ đạo & Bắt Trend",
		Hook3s:         "Điệu nhảy đang gây sốt toàn cõi mạng 24h qua bạn đã thử chưa?",
		FormatTag:      "Dance Trend Triệu View",
		SampleClip:     "clip_03_sexy_dance_60s",
		SampleVideoURL: "/clips/viral/clip_03_sexy_dance_60s.mp4",
		SampleThumbURL: "/clips/viral/test_thumb.jpg",
	},
	{
		Title:          "Vũ đạo đường phố đôi năng động 45s",
		Genre:          "dance",
		GenreVN:        "Vũ đạo đường phố",
		Hook3s:         "Nhịp điệu đường phố bắt trend, chuyển động toàn thân mượt mà...",
		FormatTag:      "Street Dance Duo",
		SampleClip:     "clip_01_dance_duo",
		SampleVideoURL: "/clips/viral/clip_01_dance_duo.mp4",
		SampleThumbURL: "/clips/viral/test_ny_thumb.jpg",
	},
	{
		Title:          "Nhảy vũ đoàn hiện đại K-Pop 60s",
		Genre:          "dance",
		GenreVN:        "Nhảy K-Pop thần tượng",
		Hook3s:         "Vũ đạo nhóm đồng đều, sôi động, phong cách idol năng lượng cao...",
		FormatTag:      "K-Pop Idol Dance",
		SampleClip:     "clip_02_dance_crew",
		SampleVideoURL: "/clips/viral/clip_02_dance_crew.mp4",
		SampleThumbURL: "/clips/viral/test_thumb.jpg",
	},
	{
		Title:          "Sự thật rúng động về AI mà 99% mọi người chưa từng nghe",
		Genre:          "mystery",
		GenreVN:        "Bí ẩn & Giật gân",
		Hook3s:         "Dừng lại 3 giây! Nếu bạn nghĩ AI chỉ là chatbot, bạn đang bỏ lỡ điều này...",
		FormatTag:      "Hook gây tò mò cao",
		SampleClip:     "clip_01_dance_duo",
		SampleVideoURL: "/clips/viral/clip_01_dance_duo.mp4",
		SampleThumbURL: "/clips/viral/test_ny_thumb.jpg",
	},
	{
		Title:     "Top 3 sai lầm khiến người mới cháy sạch tiền khi kinh doanh số",
		Genre:     "lifehack",
		GenreVN:   "Kinh nghiệm & Đời sống",
		Hook3s:    "Tại sao 90% người khởi nghiệp thất bại trong tháng đầu tiên?",
		FormatTag: "Hook cảnh báo sai lầm",
	},
	{
		Title:     "Bí quyết xây dựng thói quen siêu năng suất của các tỷ phú công nghệ",
		Genre:     "trendjack",
		GenreVN:   "Xu hướng nóng",
		Hook3s:    "Quy tắc 5 giây này đã thay đổi hoàn toàn sự nghiệp của Elon Musk...",
		FormatTag: "Hook thói quen tỷ phú",
	},
	{
		Title:     "Đột phá công nghệ 2026: Những công cụ thay đổi hoàn toàn tương lai",
		Genre:     "tech_fact",
		GenreVN:   "Công nghệ & Tri thức",
		Hook3s:    "Năm 2026, những ai không biết 3 công cụ này sẽ bị bỏ lại phía sau...",
		FormatTag: "Hook dự đoán tương lai",
	},
	{
		Title:     "Đánh giá chi tiết thiết bị sáng tạo nội dung đáng tiền nhất năm nay",
		Genre:     "koc_review",
		GenreVN:   "Đánh giá & Review",
		Hook3s:    "Đừng vội mua thiết bị này trước khi xem hết video 30 giây này!",
		FormatTag: "Hook review trải nghiệm",
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

var defaultViralChannels = []viralCreatorChannel{
	{
		Handle:     "@hoaa.hanassii",
		Name:       "Đào Lê Phương Hoa",
		Followers:  "21.2M Followers",
		Category:   "Nữ Hoàng Vũ Đạo TikTok VN",
		AvatarURL:  "https://images.unsplash.com/photo-1534528741775-53994a69daeb?w=200&q=80",
		HotTrend:   "Vũ đạo Sexy Dance Beat Remix giật giật",
		Music:      "Cắt Đôi Nỗi Sầu (Speed Up Remix)",
		Views:      "24.5M Views",
		SampleClip: "clip_03_sexy_dance_60s",
		VideoURL:   "/clips/viral/clip_03_sexy_dance_60s.mp4",
		Hook:       "Điệu nhảy đang chiếm lĩnh tab thịnh hành 24h qua!",
	},
	{
		Handle:     "@linhbarbie",
		Name:       "Linh Barbie",
		Followers:  "39.1M Followers",
		Category:   "Top 1 Creator Nữ TikTok VN",
		AvatarURL:  "https://images.unsplash.com/photo-1517841905240-472988babdf9?w=200&q=80",
		HotTrend:   "Biến hình Makeover từ mộc mạc sang thần thái Idol",
		Music:      "See Tình (Dance Remix Hot TikTok)",
		Views:      "38.2M Views",
		SampleClip: "clip_01_dance_duo",
		VideoURL:   "/clips/viral/clip_01_dance_duo.mp4",
		Hook:       "Biến hình 1 giây khiến cả mạng xã hội phải xem lại 3 lần!",
	},
	{
		Handle:     "@lengoc_phuonganh",
		Name:       "Phương Anh Lê",
		Followers:  "6.4M Followers",
		Category:   "Hot Girl Vũ Đạo & Thần Thái",
		AvatarURL:  "https://images.unsplash.com/photo-1524504388940-b1c1722653e1?w=200&q=80",
		HotTrend:   "Vũ đạo phòng ngủ ngọt ngào bắt trend Douyin",
		Music:      "Bật Tình Yêu Lên (Remix TikTok)",
		Views:      "15.8M Views",
		SampleClip: "clip_03_sexy_dance_60s",
		VideoURL:   "/clips/viral/clip_03_sexy_dance_60s.mp4",
		Hook:       "Trend vũ đạo ngọt ngào đang làm chao đảo mạng xã hội!",
	},
	{
		Handle:     "@douyin_girls_dance",
		Name:       "Douyin Dance Trends",
		Followers:  "18.5M Followers",
		Category:   "Vũ Đạo Triệu View Douyin / TikTok",
		AvatarURL:  "https://images.unsplash.com/photo-1494790108377-be9c29b29330?w=200&q=80",
		HotTrend:   "Nhảy K-Pop thần tượng năng lượng cao 60s",
		Music:      "Super Shy (Douyin Fast Remix)",
		Views:      "42.0M Views",
		SampleClip: "clip_02_dance_crew",
		VideoURL:   "/clips/viral/clip_02_dance_crew.mp4",
		Hook:       "Vũ đạo đồng đội cực cháy chuẩn idol âm nhạc quốc tế!",
	},
}

// handleViral hiển thị trang tổng quan Xưởng Sáng Tạo Video Viral AI 100% Gốc.
func (s *Server) handleViral(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v := viralView{
		StudioOK:        s.Studio != nil,
		Trends:          defaultViralTrends,
		Error:           q.Get("err"),
		SampleClips:     defaultViralClips,
		AvailableModels: s.listModelPhotoCards(),
		TopChannels:     defaultViralChannels,
	}

	if msg := q.Get("msg"); msg == "created" {
		v.Notice = "Tác vụ sản xuất video viral AI đã được đưa vào hàng đợi thành công!"
	}

	// Chọn nhân vật được chỉ định từ query param hoặc mặc định ảnh đầu tiên
	if modelIDStr := q.Get("model_id"); modelIDStr != "" {
		if id, err := strconv.ParseInt(modelIDStr, 10, 64); err == nil && id > 0 {
			v.SelectedModelID = id
			for _, m := range v.AvailableModels {
				if m.ID == id {
					v.SelectedModelPhoto = m.URL
					break
				}
			}
		}
	}
	if v.SelectedModelID == 0 && len(v.AvailableModels) > 0 {
		v.SelectedModelID = v.AvailableModels[0].ID
		v.SelectedModelPhoto = v.AvailableModels[0].URL
	}

	// Chọn video mẫu chuyển động
	if clipID := q.Get("clip"); clipID != "" {
		v.SelectedClip = clipID
	} else if len(v.SampleClips) > 0 {
		v.SelectedClip = v.SampleClips[0].ID
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
			var srtURL string
			if j.Status == studio.StatusDone && j.Output != "" {
				mediaURL = "/media/" + filepath.Base(j.Output)
				srtFile := strings.TrimSuffix(j.Output, filepath.Ext(j.Output)) + ".srt"
				if fileExists(srtFile) {
					srtURL = "/media/" + filepath.Base(srtFile)
				}
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
				SRTURL:     srtURL,
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

	_ = r.ParseMultipartForm(50 << 20)
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

	// Character / Model face lookup or direct upload (supports multiple photos)
	upDir := filepath.Join(s.DataDir, "uploads")
	_ = os.MkdirAll(upDir, 0o755)

	var uploadedPhotos []string
	for _, key := range []string{"character_photos", "character_photo", "model_photos", "model_photo"} {
		if photos, _ := saveUploads(r, key, upDir, 10); len(photos) > 0 {
			uploadedPhotos = append(uploadedPhotos, photos...)
		}
	}
	if len(uploadedPhotos) == 0 {
		if p1, _ := saveUpload(r, "character_photo", upDir); p1 != "" {
			uploadedPhotos = append(uploadedPhotos, p1)
		} else if p2, _ := saveUpload(r, "model_photo", upDir); p2 != "" {
			uploadedPhotos = append(uploadedPhotos, p2)
		}
	}

	var accountID int64
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

	// If user uploaded new photos and selected an account, persist into the account's model library
	if len(uploadedPhotos) > 0 && accountID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil {
			for _, pth := range uploadedPhotos {
				_, _ = ms.Add(accountID, pth)
			}
		}
	}

	// 1. Resolve model photo from model_id if provided (highest priority from user selection)
	if len(uploadedPhotos) == 0 {
		if modelIDStr := r.PostFormValue("model_id"); modelIDStr != "" {
			if mid, err := strconv.ParseInt(modelIDStr, 10, 64); err == nil && mid > 0 {
				if photoPath, perr := s.findModelPhotoByID(mid); perr == nil && photoPath != "" {
					uploadedPhotos = append(uploadedPhotos, photoPath)
				}
			}
		}
	}

	// 2. If no explicit model_id and no upload, fallback to stored photos from the account's model library
	if len(uploadedPhotos) == 0 && accountID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil {
			if photos, err := ms.List(accountID); err == nil && len(photos) > 0 {
				for _, ph := range photos {
					uploadedPhotos = append(uploadedPhotos, ph.Path)
				}
			}
		}
	}

	// 2. Resolve sample motion video (video mẫu chuyển động)
	sampleClip := strings.TrimSpace(r.PostFormValue("sample_clip"))
	var sampleVideoPath string
	if sampleClip != "" {
		candidates := []string{
			filepath.Join(s.DataDir, "clips", "viral", sampleClip),
			filepath.Join(s.DataDir, "clips", "viral", sampleClip+".mp4"),
			filepath.Join("data", "clips", "viral", sampleClip),
			filepath.Join("data", "clips", "viral", sampleClip+".mp4"),
		}
		for _, cand := range candidates {
			if fileExists(cand) {
				sampleVideoPath = cand
				break
			}
		}
	}
	if sampleVideoPath == "" {
		// Mặc định video vũ đạo phòng ngủ 60s
		defClip := filepath.Join(s.DataDir, "clips", "viral", "clip_03_sexy_dance_60s.mp4")
		if fileExists(defClip) {
			sampleVideoPath = defClip
		} else {
			defClip = filepath.Join("data", "clips", "viral", "clip_03_sexy_dance_60s.mp4")
			if fileExists(defClip) {
				sampleVideoPath = defClip
			}
		}
	}

	aiMode := strings.TrimSpace(r.PostFormValue("ai_mode"))
	if aiMode == "" {
		aiMode = "faceswap"
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
		SampleVideo:    sampleVideoPath,
		AIMode:         aiMode,
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

	http.Redirect(w, r, fmt.Sprintf("/studio/jobs?tab=running&msg=created&job_id=%s", jobID), http.StatusSeeOther)
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
	account := r.PostFormValue("account")
	if topic == "" {
		topic = "Bí quyết phát triển bản thân và tư duy tài chính thông minh"
	}
	if genre == "" {
		genre = "trendjack"
	}

	sampleClip := strings.TrimSpace(r.PostFormValue("sample_clip"))
	if sampleClip == "" && (genre == "dance" || strings.Contains(topic, "Dance") || strings.Contains(topic, "Vũ Điệu")) {
		sampleClip = "clip_03_sexy_dance_60s"
	}
	var sampleVideoPath string
	if sampleClip != "" {
		candidates := []string{
			filepath.Join(s.DataDir, "clips", "viral", sampleClip),
			filepath.Join(s.DataDir, "clips", "viral", sampleClip+".mp4"),
			filepath.Join("data", "clips", "viral", sampleClip),
			filepath.Join("data", "clips", "viral", sampleClip+".mp4"),
		}
		for _, cand := range candidates {
			if fileExists(cand) {
				sampleVideoPath = cand
				break
			}
		}
	}

	var accountID int64
	var modelPhoto string
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
				modelPhoto = photos[len(photos)-1].Path // use latest model face
			}
		}
	}

	p := studio.ViralParams{
		Topic:          topic,
		Genre:          genre,
		VisualMood:     "cinematic",
		Account:        account,
		AccountID:      accountID,
		ModelPhoto:     modelPhoto,
		SampleVideo:    sampleVideoPath,
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

	http.Redirect(w, r, fmt.Sprintf("/studio/jobs?tab=running&msg=created&job_id=%s", jobID), http.StatusSeeOther)
}

// studioUnmarshalJSON giải mã json chuỗi an toàn.
func studioUnmarshalJSON(data string, v any) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), v)
}
