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

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"github.com/ninhlee99/ai-creator-os/internal/viralradar"
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
	StudioOK              bool
	Jobs                  []viralJobView
	AccountNames          []string
	AccountOptions        []*network.Account
	SelectedAccountID     int64
	CharacterEditorReady  bool
	CharacterEditorStatus string
	AutoOn                bool
	ActiveCount           int
	DoneCount             int
	TotalCount            int
	Error                 string
	Notice                string
	AvailableModels       []modelPhotoCardView
	SelectedModelID       int64
	SelectedModelPhoto    string
	RadarSources          []viralradar.Source
	RadarQuery            string
	RadarLastRun          string
	RadarKeyReady         bool
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
		Notice:          q.Get("notice"),
		AvailableModels: s.listModelPhotoCards(),
		RadarQuery:      s.defaultViralRadarQuery(),
	}
	if s.Studio != nil {
		v.CharacterEditorReady, v.CharacterEditorStatus = s.Studio.ViralCharacterEditorStatus(r.Context())
	}
	if key, ok := s.effectiveEnv("YOUTUBE_API_KEY"); ok && strings.TrimSpace(key) != "" {
		v.RadarKeyReady = true
	}
	settings := s.settings()
	if raw, ok := settings.Get(automation.KeyViralRadarSources); ok {
		_ = json.Unmarshal([]byte(raw), &v.RadarSources)
		if len(v.RadarSources) > 10 {
			v.RadarSources = v.RadarSources[:10]
		}
	}
	if lastRun, ok := settings.Get("viral.radar.last_run"); ok {
		v.RadarLastRun = lastRun
	}
	if query, ok := settings.Get(automation.KeyViralRadarQuery); ok && strings.TrimSpace(query) != "" {
		v.RadarQuery = query
	}

	if msg := q.Get("msg"); msg == "created" {
		v.Notice = "Tác vụ sản xuất video viral AI đã được đưa vào hàng đợi thành công!"
	}

	// Channel selection scopes every identity photo shown and accepted below.
	if s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			v.AccountOptions = accounts
			for _, a := range accounts {
				if a != nil && a.Username != "" {
					v.AccountNames = append(v.AccountNames, a.Username)
				}
			}
		}
	}
	if raw := q.Get("account_id"); raw != "" {
		v.SelectedAccountID, _ = strconv.ParseInt(raw, 10, 64)
	}
	if v.SelectedAccountID == 0 && strings.TrimSpace(q.Get("account")) != "" {
		for _, account := range v.AccountOptions {
			if account != nil && account.Username == strings.TrimSpace(q.Get("account")) {
				v.SelectedAccountID = account.ID
				break
			}
		}
	}
	// A deep link to a saved character photo also selects its owning channel.
	if modelIDStr := q.Get("model_id"); modelIDStr != "" {
		if id, err := strconv.ParseInt(modelIDStr, 10, 64); err == nil && id > 0 {
			for _, m := range v.AvailableModels {
				if m.ID == id {
					if v.SelectedAccountID == 0 {
						v.SelectedAccountID = m.AccountID
					}
					if m.AccountID == v.SelectedAccountID {
						v.SelectedModelID = id
						v.SelectedModelPhoto = m.URL
					}
					break
				}
			}
		}
	}
	if v.SelectedAccountID == 0 && len(v.AccountOptions) > 0 && v.AccountOptions[0] != nil {
		v.SelectedAccountID = v.AccountOptions[0].ID
	}
	if v.SelectedModelID == 0 {
		for _, m := range v.AvailableModels {
			if m.AccountID == v.SelectedAccountID {
				v.SelectedModelID = m.ID
				v.SelectedModelPhoto = m.URL
				break
			}
		}
	}

	// Video tham chiếu chỉ được chọn rõ ràng từ file đã nhập.

	// Đọc công tắc tự động hóa viral
	st := s.settings()
	v.AutoOn = true
	if val, ok := st.Get(automation.KeyViralAutoProduce); ok {
		v.AutoOn = val == "1" || val == "true"
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

func (s *Server) defaultViralRadarQuery() string {
	if s.Mgr != nil {
		if accounts, err := s.Mgr.List(); err == nil {
			for _, account := range accounts {
				if account != nil && strings.TrimSpace(account.Niche) != "" && account.Status != "paused" && account.Status != "retired" {
					return strings.TrimSpace(account.Niche)
				}
			}
		}
	}
	return "video viral Việt Nam"
}

func (s *Server) handleViralRadarRefresh(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape("Could not read radar form"), http.StatusSeeOther)
		return
	}
	query := strings.TrimSpace(r.PostFormValue("query"))
	if err := s.automation().RefreshViralRadarQuery(query); err != nil {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	notes := s.AutomationService().ViralRadarTick(r.Context(), true)
	message := "YouTube radar finished."
	if len(notes) > 0 {
		message = strings.Join(notes, " ")
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "missing") || strings.Contains(lower, "failed") || strings.Contains(lower, "could not") || strings.Contains(lower, "kill switch") {
		http.Redirect(w, r, "/viral?err="+url.QueryEscape(message), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/viral?notice="+url.QueryEscape(message), http.StatusSeeOther)
}

// ValidateAndEnforceViralChannel resolves only the channel explicitly selected by the operator.
func ValidateAndEnforceViralChannel(topic, genre, requestedAccount string, accounts []*network.Account) (string, int64, string) {
	_ = topic
	_ = genre
	requestedAccount = strings.TrimSpace(requestedAccount)
	if requestedAccount == "" {
		return "", 0, "Chọn kênh đích trước khi tạo video."
	}
	for _, account := range accounts {
		if account != nil && account.Username == requestedAccount {
			return account.Username, account.ID, ""
		}
	}
	return requestedAccount, 0, "Kênh đã chọn không tồn tại; chọn lại kênh đích."
}

// handleViralCreate tiếp nhận form khởi tạo video viral mới.
func (s *Server) handleViralCreate(w http.ResponseWriter, r *http.Request) {
	analysisOnly := r.URL.Query().Get("action") == "analyze"
	if s.Studio == nil {
		writeViralCreateError(w, r, analysisOnly, http.StatusServiceUnavailable, "Studio AI chưa sẵn sàng")
		return
	}
	if !analysisOnly {
		if ready, message := s.Studio.ViralCharacterEditorStatus(r.Context()); !ready {
			writeViralCreateError(w, r, false, http.StatusServiceUnavailable, message)
			return
		}
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Không đọc được form video tham chiếu. Hãy chọn file nhỏ hơn hoặc dán link nguồn.")
		return
	}
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	if topic == "" {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Vui lòng nhập chủ đề video")
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

	musicURL := r.PostFormValue("music_url")

	// Character / Model face lookup or direct upload (supports multiple photos)
	upDir := filepath.Join(s.DataDir, "uploads")
	_ = os.MkdirAll(upDir, 0o755)

	var uploadedPhotos []string
	var previewCleanup []string
	defer func() {
		if analysisOnly {
			for _, path := range previewCleanup {
				_ = os.Remove(path)
			}
		}
	}()
	for _, key := range []string{"character_photos", "character_photo", "model_photos", "model_photo"} {
		photos, uploadErr := saveUploads(r, key, upDir, 10)
		if uploadErr != nil {
			writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Ảnh nhân vật lỗi: "+uploadErr.Error())
			return
		}
		if len(photos) > 0 {
			uploadedPhotos = append(uploadedPhotos, photos...)
			previewCleanup = append(previewCleanup, photos...)
		}
	}
	if len(uploadedPhotos) == 0 {
		if p1, _ := saveUpload(r, "character_photo", upDir); p1 != "" {
			uploadedPhotos = append(uploadedPhotos, p1)
			previewCleanup = append(previewCleanup, p1)
		} else if p2, _ := saveUpload(r, "model_photo", upDir); p2 != "" {
			uploadedPhotos = append(uploadedPhotos, p2)
			previewCleanup = append(previewCleanup, p2)
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
	if len(uploadedPhotos) > 0 && accountID > 0 && s.Studio != nil && !analysisOnly {
		ms, err := s.Studio.ModelLibrary()
		if err != nil {
			writeViralCreateError(w, r, false, http.StatusInternalServerError, "Không đọc được thư viện nhân vật: "+err.Error())
			return
		}
		var stored []string
		for _, pth := range uploadedPhotos {
			photo, addErr := ms.Add(accountID, pth)
			if addErr != nil {
				writeViralCreateError(w, r, false, http.StatusInternalServerError, "Không lưu được ảnh vào đúng kênh: "+addErr.Error())
				return
			}
			stored = append(stored, photo.Path)
		}
		uploadedPhotos = stored
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
				photos := s.findModelPhotosByIDsForAccount(accountID, ids)
				if len(photos) != len(ids) {
					writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Ảnh đã chọn không thuộc thư viện của kênh đích.")
					return
				}
				uploadedPhotos = append(uploadedPhotos, photos...)
			}
		}
	}

	// 2. Resolve model photo from model_id if provided (fallback single ID)
	if len(uploadedPhotos) == 0 {
		if modelIDStr := r.PostFormValue("model_id"); modelIDStr != "" {
			if mid, err := strconv.ParseInt(modelIDStr, 10, 64); err == nil && mid > 0 {
				if photoPaths := s.findModelPhotosByIDsForAccount(accountID, []int64{mid}); len(photoPaths) > 0 {
					uploadedPhotos = append(uploadedPhotos, photoPaths[0])
				}
			}
		}
	}

	// 4. Resolve the required source video from upload or supported download.
	tiktokLink := strings.TrimSpace(r.PostFormValue("tiktok_link"))
	if tiktokLink == "" {
		tiktokLink = strings.TrimSpace(r.PostFormValue("video_url"))
	}
	sampleVideoPath, uploadErr := saveUpload(r, "sample_video", upDir)
	if uploadErr != nil {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Video nguồn lỗi: "+uploadErr.Error())
		return
	}
	if sampleVideoPath != "" {
		previewCleanup = append(previewCleanup, sampleVideoPath)
	}
	if sampleVideoPath == "" && tiktokLink != "" && (strings.HasPrefix(tiktokLink, "http://") || strings.HasPrefix(tiktokLink, "https://")) {
		dlDst := filepath.Join(upDir, fmt.Sprintf("%d-tiktok-dl.mp4", time.Now().UnixNano()))
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		cmd := exec.CommandContext(ctx, "yt-dlp", "-f", "b", "--no-warnings", "-o", dlDst, tiktokLink)
		if err := cmd.Run(); err == nil && fileExists(dlDst) {
			sampleVideoPath = dlDst
		} else {
			cancel()
			writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Không tải được video nguồn. Tải file video trực tiếp hoặc kiểm tra lại link.")
			return
		}
		cancel()
		previewCleanup = append(previewCleanup, sampleVideoPath)
	}

	aiMode := strings.TrimSpace(r.PostFormValue("ai_mode"))
	if aiMode == "" {
		aiMode = studio.ViralModeFullCharacterEdit
	}
	if aiMode != studio.ViralModeFullCharacterEdit {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Chỉ hỗ trợ thay toàn bộ nhân vật trực tiếp trên video nguồn.")
		return
	}
	if sampleVideoPath == "" {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Cần video nguồn để giữ nguyên cảnh, chuyển động và camera.")
		return
	}
	if accountID <= 0 {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Chọn kênh đích trước khi tạo video.")
		return
	}
	if len(uploadedPhotos) == 0 || !fileExists(uploadedPhotos[0]) {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Video viral production cần ảnh nhân vật của kênh.")
		return
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
		SourceVideoURL: tiktokLink,
		AIMode:         aiMode,
		TargetDuration: float64(duration),
		MusicURL:       musicURL,
	}
	if analysisOnly {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
		defer cancel()
		storyboard, err := s.Studio.GenerateViralScript(ctx, p)
		if err != nil {
			writeViralCreateError(w, r, true, http.StatusUnprocessableEntity, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(storyboard); err != nil {
			log.Printf("web: encode viral storyboard: %v", err)
		}
		return
	}

	jobID, err := s.Studio.EnqueueViral(r.Context(), p)
	if err != nil {
		log.Printf("web: viral create error: %v", err)
		writeViralCreateError(w, r, false, http.StatusInternalServerError, "Lỗi tạo job: "+err.Error())
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("human", "viral_source_replace", nil, fmt.Sprintf("Tạo video thay nhân vật trên nguồn đã chọn job=%s genre=%s: %s", jobID, genre, topic), nil)
	}

	http.Redirect(w, r, fmt.Sprintf("/studio/jobs?tab=running&msg=created&job_id=%s", jobID), http.StatusSeeOther)
}

func writeViralCreateError(w http.ResponseWriter, r *http.Request, analysisOnly bool, status int, message string) {
	if analysisOnly {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	http.Redirect(w, r, "/viral?err="+url.QueryEscape(message), http.StatusSeeOther)
}

func studioUnmarshalJSON(data string, v any) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), v)
}
