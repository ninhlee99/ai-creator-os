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

// viralView là mô hình dữ liệu chính cho trang /viral.
type viralView struct {
	StudioOK           bool
	Jobs               []viralJobView
	AccountNames       []string
	AutoOn             bool
	ActiveCount        int
	DoneCount          int
	TotalCount         int
	Error              string
	Notice             string
	AvailableModels    []modelPhotoCardView
	SelectedModelID    int64
	SelectedModelPhoto string
}

// handleViral renders the video creation form and actual job history.
func viralGenreVN(genre string) string {
	switch genre {
	case "dance":
		return "Vũ đạo"
	case "lifehack":
		return "Mẹo đời sống"
	case "mystery":
		return "Bí ẩn"
	case "tech_fact":
		return "Công nghệ"
	case "trendjack":
		return "Chủ đề phổ thông"
	default:
		return genre
	}
}

func viralStatusVN(st string) string {
	switch st {
	case studio.StatusQueued:
		return "Đang chờ"
	case studio.StatusRunning:
		return "Đang dựng"
	case studio.StatusDone:
		return "Hoàn tất"
	case studio.StatusFailed:
		return "Cần xử lý"
	default:
		return st
	}
}

func (s *Server) handleViral(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v := viralView{
		StudioOK:        s.Studio != nil,
		Error:           q.Get("err"),
		AvailableModels: s.listModelPhotoCards(),
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

	// Video tham chiếu chỉ được chọn rõ ràng từ file đã nhập.

	// Đọc công tắc tự động hóa viral
	st := s.settings()
	if val, ok := st.Get("viral.auto_produce_enabled"); ok {
		v.AutoOn = val == "1" || val == "true"
	} else {
		v.AutoOn = false
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
	if duration < 60 {
		duration = 60 // Video viral tối thiểu từ 60s trở lên (chuẩn TikTok Creativity Program)
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

	aiMode := strings.TrimSpace(r.PostFormValue("ai_mode"))
	if aiMode == "" {
		aiMode = "auto"
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

func studioUnmarshalJSON(data string, v any) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), v)
}
