package web

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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
	if analysisOnly {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))
	}
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
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Không đọc được form video tham chiếu. Hãy chọn video từ TikTok Radar rồi thử lại.")
		return
	}
	if !analysisOnly && r.PostFormValue("confirm_flow_credits") != "1" {
		writeViralCreateError(w, r, false, http.StatusBadRequest, "Xác nhận video có thể dùng Muse AI credits trước khi tạo job.")
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

	// Character / Model face lookup or direct upload (supports multiple photos)
	upDir := filepath.Join(s.DataDir, "uploads")
	_ = os.MkdirAll(upDir, 0o755)

	var uploadedPhotos []string
	var previewCleanup []string
	var sampleVideoPath string
	keepSourceVideoForJob := false
	defer func() {
		if analysisOnly {
			for _, path := range previewCleanup {
				_ = os.Remove(path)
			}
		}
		if !analysisOnly && !keepSourceVideoForJob && sampleVideoPath != "" {
			_ = os.Remove(sampleVideoPath)
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

	// 4. Resolve source video from the selected TikTok Radar item.
	tiktokLink := strings.TrimSpace(r.PostFormValue("tiktok_link"))
	if tiktokLink == "" {
		tiktokLink = strings.TrimSpace(r.PostFormValue("video_url"))
	}
	if s.Studio == nil {
		writeViralCreateError(w, r, analysisOnly, http.StatusServiceUnavailable, "Studio chưa sẵn sàng để xác minh video Radar TikTok")
		return
	}
	sourceVideo, err := s.Studio.ResolveTikTokRadarVideo(r.Context(), tiktokLink)
	if err != nil {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Chọn video từ Radar TikTok trước khi tiếp tục: "+err.Error())
		return
	}
	tiktokLink = sourceVideo.VideoURL
	dlDst := filepath.Join(upDir, fmt.Sprintf("%d-tiktok-dl.mp4", time.Now().UnixNano()))
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	cmd := exec.CommandContext(ctx, "yt-dlp", "-f", "b", "--no-warnings", "--no-playlist", "-o", dlDst, tiktokLink)
	downloadOutput, downloadErr := cmd.CombinedOutput()
	cancel()
	if downloadErr != nil {
		_ = os.Remove(dlDst)
		message := strings.Join(strings.Fields(strings.TrimSpace(string(downloadOutput))), " ")
		if message == "" {
			message = downloadErr.Error()
		}
		if len([]rune(message)) > 500 {
			message = string([]rune(message)[:500]) + "…"
		}
		log.Printf("web: viral Radar download failed: %s", message)
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Không tải được video từ Radar: "+message)
		return
	}
	if !fileExists(dlDst) {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "yt-dlp kết thúc nhưng không tạo file video Radar.")
		return
	}
	sampleVideoPath = dlDst
	previewCleanup = append(previewCleanup, sampleVideoPath)
	if !studio.ProbeHasAudio(r.Context(), sampleVideoPath) {
		writeViralCreateError(w, r, analysisOnly, http.StatusBadRequest, "Video Radar không có track âm thanh; chọn video khác.")
		return
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
		Topic:             topic,
		Genre:             genre,
		VisualMood:        visualMood,
		Voice:             voice,
		Account:           account,
		AccountID:         accountID,
		ModelPhoto:        modelPhoto,
		ModelPhotos:       uploadedPhotos,
		SampleVideo:       sampleVideoPath,
		SourceVideoURL:    tiktokLink,
		SourceVideoID:     sourceVideo.VideoID,
		SourceVideoAuthor: sourceVideo.Author,
		SourceMusicTitle:  sourceVideo.SoundTitle(),
		SourceCollectedAt: sourceVideo.CollectedAt,
		SourcePerformance: studio.ViralSourcePerformance{
			Views: sourceVideo.PlayCount, ViewsKnown: sourceVideo.PlayCountKnown,
			Likes: sourceVideo.DiggCount, LikesKnown: sourceVideo.DiggCountKnown,
			Comments: sourceVideo.CommentCount, CommentsKnown: sourceVideo.CommentKnown,
			Shares: sourceVideo.ShareCount, SharesKnown: sourceVideo.ShareCountKnown,
			ViewsPerHour: sourceVideo.ViewsPerHour, VelocityKnown: sourceVideo.VelocityKnown,
			EngagementRate: sourceVideo.EngagementRate, EngagementKnown: sourceVideo.EngagementKnown,
		},
		AIMode:                  aiMode,
		TargetDuration:          float64(duration),
		OutputResolution:        strings.TrimSpace(r.PostFormValue("output_resolution")),
		FlowCreditsAcknowledged: r.PostFormValue("confirm_flow_credits") == "1",
	}
	if analysisOnly {
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
		defer cancel()
		storyboard, err := s.Studio.GenerateViralScript(ctx, p)
		if err != nil {
			log.Printf("web: viral analyze error: %v", err)
			writeViralCreateError(w, r, true, http.StatusUnprocessableEntity, err.Error())
			return
		}
		log.Printf("web: viral analyze success for topic %q (%d scenes)", p.Topic, len(storyboard.Scenes))
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
	keepSourceVideoForJob = true

	if s.Ledger != nil {
		_ = s.Ledger.Decide("human", "viral_source_replace", nil, fmt.Sprintf("Tạo video thay nhân vật trên nguồn đã chọn job=%s genre=%s: %s", jobID, genre, topic), nil)
	}

	http.Redirect(w, r, "/studio/jobs?msg=created&job_id="+url.QueryEscape(jobID)+"#job-"+url.QueryEscape(jobID), http.StatusSeeOther)
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
