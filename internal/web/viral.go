package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
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
	Platform    string `json:"platform"` // "TikTok VN" hoặc "Douyin CN"
	Region      string `json:"region"`   // "VN" hoặc "CN"
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
	DramasVN           []studio.ShowbizDrama
	DramasCN           []studio.ShowbizDrama
	Error              string
	Notice             string
	AvailableModels    []modelPhotoCardView
	SelectedModelID    int64
	SelectedModelPhoto string
	SampleClips        []sampleClipView
	SelectedClip       string
	TopChannels        []viralCreatorChannel
	TopicGroups        []studio.TopicVideoGroup
	PresetTopicGroups  []studio.TopicVideoGroup
}

// defaultViralClips danh sách các video vũ đạo triệu view chuẩn 9:16 từ TikTok Việt Nam và Douyin Trung Quốc.
var defaultViralClips = []sampleClipView{
	{
		ID:          "clip_vn_phuonghoa",
		Title:       "🇻🇳 TikTok VN: Đào Lê Phương Hoa - Sexy Dance Bắt Trend",
		Filename:    "clip_vn_phuonghoa.mp4",
		URL:         "/clips/viral/clip_vn_phuonghoa.mp4",
		Duration:    "30s (1080x1920 HD)",
		Category:    "dance",
		Platform:    "TikTok VN",
		Region:      "VN",
		Description: "Vũ đạo nóng bỏng triệu view từ Nữ hoàng vũ đạo TikTok Việt Nam (@hoaa.hanassii), thần thái quyến rũ, chuyển động 9:16 sắc nét.",
	},
	{
		ID:          "clip_vn_cac_dance",
		Title:       "🇻🇳 TikTok VN: C.A.C Dance Crew - Vũ Đạo Bắt Trend",
		Filename:    "clip_vn_cac_dance.mp4",
		URL:         "/clips/viral/clip_vn_cac_dance.mp4",
		Duration:    "15s (1080x1920 HD)",
		Category:    "dance",
		Platform:    "TikTok VN",
		Region:      "VN",
		Description: "Vũ đạo nhóm sôi động bắt trend TikTok Việt Nam, nhịp điệu cuốn hút, góc quay hiện đại chuẩn 9:16.",
	},
	{
		ID:          "clip_cn_douyin_hot",
		Title:       "🇨🇳 Douyin Trung Quốc: 抖音卡点热舞 (Douyin Hot Dance)",
		Filename:    "clip_cn_douyin_hot.mp4",
		URL:         "/clips/viral/clip_cn_douyin_hot.mp4",
		Duration:    "20s (1080x1920 HD)",
		Category:    "dance",
		Platform:    "Douyin CN",
		Region:      "CN",
		Description: "Điệu nhảy bắt beat cực chuẩn sốt xình xịch Douyin Trung Quốc, visual đỉnh cao, chuyển động toàn thân cuốn hút.",
	},
	{
		ID:          "clip_cn_douyin_kpop",
		Title:       "🇨🇳 Douyin Trung Quốc: 抖音女团元气齐舞 (Douyin Idol Dance)",
		Filename:    "clip_cn_douyin_kpop.mp4",
		URL:         "/clips/viral/clip_cn_douyin_kpop.mp4",
		Duration:    "15s (1080x1920 HD)",
		Category:    "dance",
		Platform:    "Douyin CN",
		Region:      "CN",
		Description: "Vũ đạo phong cách thần tượng Douyin năng lượng bùng nổ, màu sắc rực rỡ, trang phục bắt mắt.",
	},
	{
		ID:          "clip_03_sexy_dance_60s",
		Title:       "🇻🇳 TikTok VN: Vũ Đạo Phòng Ngủ 30s Chân Thực",
		Filename:    "clip_03_sexy_dance_60s.mp4",
		URL:         "/clips/viral/clip_03_sexy_dance_60s.mp4",
		Duration:    "30s (1080x1920 HD)",
		Category:    "dance",
		Platform:    "TikTok VN",
		Region:      "VN",
		Description: "Video tự quay phòng ngủ chân thực 100%, không sân khấu, không logo, chuyển động toàn thân mượt mà.",
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
		SampleClips:     s.listViralDanceClips(),
		AvailableModels: s.listModelPhotoCards(),
		TopChannels:     defaultViralChannels,
	}

	dramasVN, dramasCN, _ := studio.FetchSplitShowbizDrama(r.Context())
	v.DramasVN = dramasVN
	v.DramasCN = dramasCN
	v.TopicGroups = studio.GetConfiguredTopicGroups([]string{"nhay", "su_kien_vn", "bien_hinh", "hai_huoc"})
	v.PresetTopicGroups = studio.GetAllPresetTopicGroups()

	if msg := q.Get("msg"); msg == "created" {
		v.Notice = "Tác vụ sản xuất video viral AI đã được đưa vào hàng đợi thành công!"
	} else if msg == "imported" {
		v.Notice = "✓ Đã tải và chuẩn hóa video vũ đạo 9:16 vào xưởng thành công! Bạn có thể xem trước và ghép mặt ngay bên dưới."
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

// ValidateAndEnforceViralChannel checks whether the requested channel matches the topic / region.
// If there is a mismatch (e.g., C-Biz drama assigned to Vietnamese kechuyen_ai, or V-Biz drama assigned to douyin_reup),
// it automatically enforces the correct channel to preserve strict channel isolation and brand integrity.
func ValidateAndEnforceViralChannel(topic, genre, requestedAccount string, accounts []*network.Account) (string, int64, string) {
	topicLower := strings.ToLower(topic)
	cbizKeywords := []string{
		"trung quốc", "c-biz", "cbiz", "hoa ngữ", "bắc kinh", "thượng hải",
		"từ nghệ dương", "hoàng tử thao", "tiêu chiến", "vương nhất bác",
		"dương tử", "bạch lộc", "triệu lộ tư", "triệu lệ dĩnh", "lưu diệc phi",
		"phạm băng băng", "huỳnh hiểu minh", "angelababy", "weibo", "douyin",
		"xiao zhan", "wang yibo", "zhao lusi", "dilraba", "nhiệt ba",
	}

	isCBiz := false
	for _, kw := range cbizKeywords {
		if strings.Contains(topicLower, kw) {
			isCBiz = true
			break
		}
	}
	if genre == "douyin" || strings.Contains(topicLower, "douyin") {
		isCBiz = true
	}

	vbizKeywords := []string{
		"v-biz", "vbiz", "việt nam", "sơn tùng", "trấn thành", "hồ ngọc hà",
		"mỹ tâm", "đàm vĩnh hưng", "hoài linh", "ninh dương lan ngọc",
		"phương hoa", "lê ngọc phương anh", "vtv", "tiktok việt nam", "showbiz việt",
		"kể chuyện", "tâm sự", "truyện ma", "vụ án",
	}
	isVBiz := false
	for _, kw := range vbizKeywords {
		if strings.Contains(topicLower, kw) {
			isVBiz = true
			break
		}
	}

	targetAccount := requestedAccount
	var targetID int64
	var note string

	if isCBiz {
		// Strictly C-Biz / Douyin -> target douyin_reup
		targetAccount = "douyin_reup"
		for _, a := range accounts {
			if a != nil && (a.Username == "douyin_reup" || strings.Contains(strings.ToLower(a.Niche), "douyin") || strings.Contains(strings.ToLower(a.Niche), "trung")) {
				targetAccount = a.Username
				targetID = a.ID
				break
			}
		}
		if requestedAccount != "" && requestedAccount != targetAccount {
			note = fmt.Sprintf("Nội dung C-Biz / Douyin tự động định tuyến chuẩn xác về kênh @%s (không để nhầm vào kênh @%s)", targetAccount, requestedAccount)
		}
	} else if isVBiz || requestedAccount == "" {
		// Strictly V-Biz / Kể chuyện Việt -> target kechuyen_ai
		targetAccount = "kechuyen_ai"
		for _, a := range accounts {
			if a != nil && (a.Username == "kechuyen_ai" || strings.Contains(strings.ToLower(a.Niche), "kể chuyện") || strings.Contains(strings.ToLower(a.Niche), "việt")) {
				targetAccount = a.Username
				targetID = a.ID
				break
			}
		}
		if requestedAccount != "" && requestedAccount != targetAccount && requestedAccount == "douyin_reup" {
			note = fmt.Sprintf("Nội dung Showbiz V-Biz tự động định tuyến chuẩn xác về kênh @%s (không để nhầm vào kênh @%s)", targetAccount, requestedAccount)
		}
	}

	// Fallback lookup if targetID is still 0
	if targetID == 0 {
		for _, a := range accounts {
			if a != nil && a.Username == targetAccount {
				targetID = a.ID
				break
			}
		}
	}

	return targetAccount, targetID, note
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

	var accounts []*network.Account
	if s.Mgr != nil {
		accounts, _ = s.Mgr.ListWithAutopilot()
	}
	enforcedAccount, targetID, channelNote := ValidateAndEnforceViralChannel(topic, genre, account, accounts)
	account = enforcedAccount
	var accountID int64 = targetID
	if channelNote != "" {
		log.Printf("web: viral channel enforcement: %s", channelNote)
	}

	// If user uploaded new photos and selected an account, persist into the account's model library
	if len(uploadedPhotos) > 0 && accountID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil {
			for _, pth := range uploadedPhotos {
				_, _ = ms.Add(accountID, pth)
			}
		}
	}

	// 1. Resolve model photos from model_photo_ids if provided (multi-angle support)
	if len(uploadedPhotos) == 0 {
		var modelIDStrs []string
		if r.MultipartForm != nil {
			modelIDStrs = r.MultipartForm.Value["model_photo_ids"]
		}
		if len(modelIDStrs) == 0 {
			modelIDStrs = r.PostForm["model_photo_ids"]
		}
		if len(modelIDStrs) > 0 {
			var ids []int64
			for _, ms := range modelIDStrs {
				if mid, err := strconv.ParseInt(strings.TrimSpace(ms), 10, 64); err == nil && mid > 0 {
					ids = append(ids, mid)
				}
			}
			if len(ids) > 0 {
				uploadedPhotos = append(uploadedPhotos, s.findModelPhotosByIDs(ids)...)
			}
		}
	}

	// 2. Resolve model photo from model_id if provided (fallback single ID)
	if len(uploadedPhotos) == 0 {
		if modelIDStr := r.PostFormValue("model_id"); modelIDStr != "" {
			if mid, err := strconv.ParseInt(modelIDStr, 10, 64); err == nil && mid > 0 {
				if photoPath, perr := s.findModelPhotoByID(mid); perr == nil && photoPath != "" {
					uploadedPhotos = append(uploadedPhotos, photoPath)
				}
			}
		}
	}

	// 3. Fallback to stored photos from the account's model library
	if len(uploadedPhotos) == 0 && accountID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil {
			if photos, err := ms.List(accountID); err == nil && len(photos) > 0 {
				for _, ph := range photos {
					uploadedPhotos = append(uploadedPhotos, ph.Path)
				}
			}
		}
	}
	if len(uploadedPhotos) == 0 {
		if cards := s.listModelPhotoCards(); len(cards) > 0 {
			for _, c := range cards {
				if photoPath, perr := s.findModelPhotoByID(c.ID); perr == nil && photoPath != "" {
					uploadedPhotos = append(uploadedPhotos, photoPath)
				}
			}
		}
	}

	// 4. Resolve sample motion video or TikTok link
	tiktokLink := strings.TrimSpace(r.PostFormValue("tiktok_link"))
	if tiktokLink == "" {
		tiktokLink = strings.TrimSpace(r.PostFormValue("video_url"))
	}
	var sampleVideoPath string
	if tiktokLink != "" && (strings.HasPrefix(tiktokLink, "http://") || strings.HasPrefix(tiktokLink, "https://")) {
		dlDst := filepath.Join(upDir, fmt.Sprintf("%d-tiktok-dl.mp4", time.Now().UnixNano()))
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		cmd := exec.CommandContext(ctx, "yt-dlp", "-f", "b", "--no-warnings", "-o", dlDst, tiktokLink)
		if err := cmd.Run(); err == nil && fileExists(dlDst) {
			sampleVideoPath = dlDst
		}
		cancel()
	}

	sampleClip := strings.TrimSpace(r.PostFormValue("sample_clip"))
	if sampleVideoPath == "" && sampleClip != "" {
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

	aiMode := strings.TrimSpace(r.PostFormValue("ai_mode"))
	if aiMode == "" {
		aiMode = "faceswap"
	}

	var modelPhoto string
	if len(uploadedPhotos) > 0 {
		modelPhoto = uploadedPhotos[0]
	}

	isDance := strings.Contains(strings.ToLower(topic), "dance") || strings.Contains(strings.ToLower(topic), "nhảy") || strings.Contains(strings.ToLower(topic), "sexy") || genre == "dance"
	if isDance {
		voice = ""
	}
	musicFile := studio.FindTrendingMusic("nhay")

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
		MusicFile:      musicFile,
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

	_ = r.ParseMultipartForm(32 << 20)
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

	var accounts []*network.Account
	if s.Mgr != nil {
		accounts, _ = s.Mgr.ListWithAutopilot()
	}
	enforcedAccount, targetID, channelNote := ValidateAndEnforceViralChannel(topic, genre, account, accounts)
	account = enforcedAccount
	var accountID int64 = targetID
	if channelNote != "" {
		log.Printf("web: viral quick channel enforcement: %s", channelNote)
	}

	var modelPhoto string
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

// listViralDanceClips trả về danh sách các clip vũ đạo mẫu từ TikTok VN và Douyin CN,
// bao gồm các video mặc định và các video do người dùng import vào data/clips/viral/.
func (s *Server) listViralDanceClips() []sampleClipView {
	clipsDir := filepath.Join(s.DataDir, "clips", "viral")
	_ = os.MkdirAll(clipsDir, 0o755)

	out := append([]sampleClipView(nil), defaultViralClips...)
	seen := make(map[string]bool)
	for _, c := range out {
		seen[c.Filename] = true
	}

	entries, err := os.ReadDir(clipsDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".mp4") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		stem := strings.TrimSuffix(name, filepath.Ext(name))

		platform := "TikTok VN"
		region := "VN"
		title := "🇻🇳 TikTok: " + stem
		if strings.Contains(strings.ToLower(stem), "douyin") || strings.Contains(strings.ToLower(stem), "cn") {
			platform = "Douyin CN"
			region = "CN"
			title = "🇨🇳 Douyin: " + stem
		}

		out = append(out, sampleClipView{
			ID:          stem,
			Title:       title,
			Filename:    name,
			URL:         "/clips/viral/" + name,
			Duration:    "Tùy chỉnh (1080x1920 HD)",
			Category:    "dance",
			Platform:    platform,
			Region:      region,
			Description: fmt.Sprintf("Video vũ đạo chuyển động %s chất lượng cao 9:16 được nhập vào hệ thống.", platform),
		})
	}
	return out
}

// handleViralClipImport tiếp nhận link TikTok hoặc Douyin, dùng yt-dlp tải video,
// chuẩn hóa về 1080x1920 (9:16 vertical) và lưu vào data/clips/viral/.
func (s *Server) handleViralClipImport(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Studio chưa sẵn sàng"), http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	rawURL := strings.TrimSpace(r.PostFormValue("url"))
	if rawURL == "" {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Vui lòng dán link video TikTok hoặc Douyin"), http.StatusSeeOther)
		return
	}
	customTitle := strings.TrimSpace(r.PostFormValue("title"))
	platform := strings.TrimSpace(r.PostFormValue("platform"))
	if platform == "" {
		if strings.Contains(rawURL, "douyin.com") {
			platform = "Douyin CN"
		} else {
			platform = "TikTok VN"
		}
	}
	region := "VN"
	if strings.Contains(platform, "Douyin") || strings.Contains(platform, "CN") {
		region = "CN"
	}

	clipsDir := filepath.Join(s.DataDir, "clips", "viral")
	_ = os.MkdirAll(clipsDir, 0o755)

	clipID := fmt.Sprintf("imported_%s_%d", strings.ToLower(region), time.Now().Unix())
	tempFile := filepath.Join(clipsDir, clipID+"_raw.mp4")
	finalFile := filepath.Join(clipsDir, clipID+".mp4")

	// 1. Tìm đường dẫn yt-dlp
	ytdlp := "yt-dlp"
	candidates := []string{
		"/Users/ninh.le/.local/bin/yt-dlp",
		"/usr/local/bin/yt-dlp",
		"yt-dlp",
	}
	for _, cand := range candidates {
		if fileExists(cand) {
			ytdlp = cand
			break
		}
	}

	// Chạy yt-dlp tải video với timeout 3 phút
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, ytdlp, "-o", tempFile, "--no-playlist", "--no-warnings", rawURL)
	outBytes, err := cmd.CombinedOutput()
	if err != nil || !fileExists(tempFile) {
		log.Printf("viral import: yt-dlp error (%v): %s", err, string(outBytes))
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Không thể tải video từ link này — vui lòng kiểm tra lại link TikTok/Douyin hoặc thử link khác."), http.StatusSeeOther)
		return
	}
	defer os.Remove(tempFile)

	// 2. Chuyển đổi và chuẩn hóa nghiêm ngặt về 1080x1920 (9:16 vertical HD) không méo hình
	ffCmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tempFile,
		"-vf", "scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,setsar=1",
		"-c:v", "libx264", "-preset", "fast", "-crf", "20",
		"-c:a", "aac", "-b:a", "192k", finalFile)
	if ffErr := ffCmd.Run(); ffErr != nil || !fileExists(finalFile) {
		log.Printf("viral import: ffmpeg error: %v", ffErr)
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Lỗi chuẩn hóa video sang chuẩn 9:16"), http.StatusSeeOther)
		return
	}

	if customTitle == "" {
		customTitle = clipID
	}
	log.Printf("viral: ✓ Đã nhập thành công video vũ đạo (%s): %s", platform, finalFile)

	http.Redirect(w, r, fmt.Sprintf("/viral?clip=%s&msg=imported", clipID), http.StatusSeeOther)
}

