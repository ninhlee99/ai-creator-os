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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var promptURLRegex = regexp.MustCompile(`https?://[^\s"'<>]+`)

type extractedPromptURLs struct {
	TikTokLink string
	ProductURL string
	ImageURLs  []string
	VideoURLs  []string
}

// extractURLsFromPrompt bóc tách link sản phẩm, video tham chiếu và ảnh nhân vật từ câu lệnh prompt.
func extractURLsFromPrompt(prompt string) extractedPromptURLs {
	var res extractedPromptURLs
	matches := promptURLRegex.FindAllString(prompt, -1)
	for _, raw := range matches {
		u := strings.TrimRight(raw, ",.;:!?)")
		lower := strings.ToLower(u)
		if strings.Contains(lower, "tiktok.com") || strings.Contains(lower, "douyin.com") {
			if strings.Contains(lower, "/view/product") || strings.Contains(lower, "shop.tiktok.com") {
				if res.ProductURL == "" {
					res.ProductURL = u
				}
			} else {
				if res.TikTokLink == "" {
					res.TikTokLink = u
				}
			}
		} else if strings.Contains(lower, "shopee.vn") || strings.Contains(lower, "shope.ee") || strings.Contains(lower, "lazada.vn") || strings.Contains(lower, "lzd.co") || strings.Contains(lower, "tiki.vn") {
			if res.ProductURL == "" {
				res.ProductURL = u
			}
		} else if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".webp") {
			res.ImageURLs = append(res.ImageURLs, u)
		} else if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".mov") || strings.HasSuffix(lower, ".webm") {
			res.VideoURLs = append(res.VideoURLs, u)
		}
	}
	return res
}

// ValidateAndEnforceViralChannel resolves the channel explicitly selected or auto-falls back to the primary channel.
func ValidateAndEnforceViralChannel(topic, genre, requestedAccount string, accounts []*network.Account) (string, int64, string) {
	_ = topic
	_ = genre
	requestedAccount = strings.TrimSpace(requestedAccount)
	if requestedAccount == "" {
		if len(accounts) > 0 && accounts[0] != nil {
			return accounts[0].Username, accounts[0].ID, ""
		}
		return "", 0, "Chọn kênh đích trước khi tạo video."
	}
	for _, account := range accounts {
		if account != nil && account.Username == requestedAccount {
			return account.Username, account.ID, ""
		}
	}
	// Fallback to first available account if requested doesn't match
	if len(accounts) > 0 && accounts[0] != nil {
		return accounts[0].Username, accounts[0].ID, ""
	}
	return requestedAccount, 0, "Kênh đã chọn không tồn tại; chọn lại kênh đích."
}

// detectAutoVideoType phân tích câu lệnh, chủ đề, link sản phẩm/video và file đính kèm
// để tự động suy luận loại video và kích hoạt workflow sản xuất phù hợp (affiliate hoặc viral).
func detectAutoVideoType(prompt, topic, productURL, tiktokLink string, photoCount int, hasVideo bool) string {
	if strings.TrimSpace(productURL) != "" {
		return "affiliate"
	}
	if strings.TrimSpace(tiktokLink) != "" {
		return "viral"
	}

	content := strings.ToLower(prompt + " " + topic)

	affiliateKeywords := []string{
		"review", "sản phẩm", "chất son", "mua", "giá", "deal", "giỏ hàng",
		"shopee", "tiktok shop", "unboxing", "outfit", "chống dính", "sale",
		"voucher", "tai nghe", "kem", "serum", "chất liệu", "đập hộp",
		"trải nghiệm", "dưỡng", "bán hàng", "săn deal", "đơn hàng", "giảm giá",
		"thời trang", "đồ gia dụng", "mỹ phẩm", "chăm sóc da", "chất lượng",
		"khuyến mãi", "công dụng", "tính năng", "son môi", "mặt nạ",
	}
	viralKeywords := []string{
		"trend", "vũ đạo", "nhảy", "dance", "biến hình", "triệu view",
		"pov", "hài", "drama", "slow-motion", "nhạc", "sound", "thử thách",
		"challenge", "bắt trend", "tiktok viral", "giải trí", "diễn xuất",
		"hài hước", "kịch tính", "chuyển cảnh", "hot hit",
	}

	affScore := 0
	for _, kw := range affiliateKeywords {
		if strings.Contains(content, kw) {
			affScore++
		}
	}

	viralScore := 0
	for _, kw := range viralKeywords {
		if strings.Contains(content, kw) {
			viralScore++
		}
	}

	if affScore > viralScore {
		return "affiliate"
	}
	if viralScore > affScore {
		return "viral"
	}

	// Khi số điểm bằng nhau: ưu tiên theo tư liệu đính kèm
	if hasVideo {
		return "viral"
	}
	if photoCount > 0 {
		return "affiliate"
	}

	return "affiliate"
}

// handleVideoCreateSubmit tiếp nhận form khởi tạo video AI mới (/workflow/create).
func (s *Server) handleVideoCreateSubmit(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		writeVideoCreateError(w, r, "Studio AI chưa sẵn sàng")
		return
	}
	if ready, message := s.Studio.ViralCharacterEditorStatus(r.Context()); !ready {
		writeVideoCreateError(w, r, message)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, (512<<20)+(1<<20))
	parseErr := r.ParseMultipartForm(50 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if parseErr != nil {
		writeViralCreateError(w, r, "Không đọc được form video tham chiếu. Hãy chọn video từ TikTok Radar rồi thử lại.")
		return
	}
	if r.PostFormValue("confirm_flow_credits") != "1" {
		writeViralCreateError(w, r, "Xác nhận video có thể dùng Muse AI credits trước khi tạo job.")
		return
	}
	prompt := strings.TrimSpace(r.PostFormValue("prompt"))
	topic := strings.TrimSpace(r.PostFormValue("topic"))
	if topic == "" && prompt != "" {
		firstLine := strings.Split(prompt, "\n")[0]
		firstLine = strings.TrimSpace(firstLine)
		if len([]rune(firstLine)) > 40 {
			topic = string([]rune(firstLine)[:40]) + "…"
		} else {
			topic = firstLine
		}
	}
	if topic == "" {
		writeViralCreateError(w, r, "Vui lòng nhập chủ đề video")
		return
	}

	outputType := strings.TrimSpace(r.PostFormValue("output_type"))
	if outputType == "" {
		wfID := strings.TrimSpace(r.PostFormValue("workflow_id"))
		if strings.Contains(wfID, "affiliate") {
			outputType = "affiliate"
		} else if strings.Contains(wfID, "viral") {
			outputType = "viral"
		} else {
			outputType = "auto"
		}
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
	upDir := s.Studio.UploadDir()
	if err := os.MkdirAll(upDir, 0o755); err != nil {
		writeViralCreateError(w, r, "Không tạo được thư mục tải lên: "+err.Error())
		return
	}

	var uploadedPhotos []string
	var previewCleanup []string
	var sampleVideoPath string
	keepSourceVideoForJob := false
	defer func() {
		for _, path := range previewCleanup {
			if keepSourceVideoForJob && path == sampleVideoPath {
				continue
			}
			_ = os.Remove(path)
		}
	}()
	for _, key := range []string{"character_photos", "character_photo", "model_photos", "model_photo", "product_photos", "photos", "image_files"} {
		photos, uploadErr := saveUploads(r, key, upDir, 10)
		if uploadErr != nil {
			writeViralCreateError(w, r, "Ảnh nhân vật lỗi: "+uploadErr.Error())
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

	// Check direct video file upload
	for _, key := range []string{"video_file", "sample_video", "source_video"} {
		if vidPath, _ := saveUpload(r, key, upDir); vidPath != "" {
			sampleVideoPath = vidPath
			previewCleanup = append(previewCleanup, sampleVideoPath)
			break
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
		ms, err := s.Studio.ModelLibrary()
		if err != nil {
			writeViralCreateError(w, r, "Không đọc được thư viện nhân vật: "+err.Error())
			return
		}
		var stored []string
		for _, pth := range uploadedPhotos {
			photo, addErr := ms.Add(accountID, pth)
			if addErr != nil {
				writeViralCreateError(w, r, "Không lưu được ảnh vào đúng kênh: "+addErr.Error())
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
					writeViralCreateError(w, r, "Ảnh đã chọn không thuộc thư viện của kênh đích.")
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

	productURL := strings.TrimSpace(r.PostFormValue("product_url"))
	tiktokLink := strings.TrimSpace(r.PostFormValue("tiktok_link"))
	if tiktokLink == "" {
		tiktokLink = strings.TrimSpace(r.PostFormValue("video_url"))
	}

	// Tự động bóc tách link sản phẩm, video TikTok tham chiếu hoặc ảnh nhân vật từ prompt
	extracted := extractURLsFromPrompt(prompt)
	if tiktokLink == "" && extracted.TikTokLink != "" {
		tiktokLink = extracted.TikTokLink
	}
	if productURL == "" && extracted.ProductURL != "" {
		productURL = extracted.ProductURL
	}
	if len(uploadedPhotos) == 0 && len(extracted.ImageURLs) > 0 && s.Studio != nil {
		for i, imgURL := range extracted.ImageURLs {
			if i >= 3 {
				break
			}
			dst := filepath.Join(upDir, fmt.Sprintf("prompt_img_%d_%d.jpg", time.Now().UnixNano(), i))
			dlCtx, dlCancel := context.WithTimeout(r.Context(), 15*time.Second)
			if err := studio.DownloadImage(dlCtx, imgURL, dst); err == nil {
				uploadedPhotos = append(uploadedPhotos, dst)
				previewCleanup = append(previewCleanup, dst)
			}
			dlCancel()
		}
	}

	// Tự động sử dụng nhân vật đã setting trong kênh nếu người dùng chưa tải ảnh mới
	if accountID <= 0 && s.Mgr != nil {
		if accts, err := s.Mgr.List(); err == nil && len(accts) > 0 {
			for _, a := range accts {
				if a != nil && (a.Username == account || "@"+a.Username == account) {
					accountID = a.ID
					account = a.Username
					break
				}
			}
			if accountID <= 0 && accts[0] != nil {
				accountID = accts[0].ID
				account = accts[0].Username
			}
		}
	}
	if len(uploadedPhotos) == 0 && accountID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil && ms != nil {
			if photos, err := ms.List(accountID); err == nil && len(photos) > 0 {
				for _, ph := range photos {
					if ph.Path != "" && fileExists(ph.Path) {
						uploadedPhotos = append(uploadedPhotos, ph.Path)
					}
				}
			}
			if len(uploadedPhotos) == 0 {
				if allPhotos, err := ms.ListAll(); err == nil && len(allPhotos) > 0 {
					for _, ph := range allPhotos {
						if ph.Path != "" && fileExists(ph.Path) {
							uploadedPhotos = append(uploadedPhotos, ph.Path)
						}
					}
				}
			}
		}
	}

	// Tự động phân tích và chọn quy trình sản xuất (Workflow) phù hợp nếu không chỉ định rõ
	isAutoMode := outputType == "auto" || outputType == ""
	if isAutoMode {
		outputType = detectAutoVideoType(prompt, topic, productURL, tiktokLink, len(uploadedPhotos), sampleVideoPath != "")
		log.Printf("web: viral auto-detected workflow output type %q for topic %q", outputType, topic)
	}

	// 3. Handle Affiliate video generation
	if outputType == "affiliate" {
		if accountID <= 0 {
			writeViralCreateError(w, r, "Chọn kênh đích trước khi tạo video.")
			return
		}
		resolution := strings.TrimSpace(r.PostFormValue("output_resolution"))
		if resolution == "" {
			resolution = "1080p"
		}
		jobTitle := "Video Affiliate: " + topic
		if isAutoMode {
			jobTitle = "Video Affiliate (AI Tự Động): " + topic
		}
		params := map[string]any{
			"prompt":            prompt,
			"topic":             topic,
			"account":           account,
			"account_id":        accountID,
			"output_resolution": resolution,
			"voice":             voice,
			"product_url":       productURL,
			"photos":            uploadedPhotos,
			"auto_detected":     isAutoMode,
		}
		if sampleVideoPath != "" {
			params["sample_video"] = sampleVideoPath
			keepSourceVideoForJob = true
		}

		jobID, err := s.Studio.CreateExternalJob("affiliate", jobTitle, params,
			func(ctx context.Context, progress func(int, string)) (string, string, error) {
				if isAutoMode {
					progress(5, "AI tự động phân tích câu lệnh -> Kích hoạt quy trình sản xuất Affiliate cho: "+topic)
				} else {
					progress(5, "Khởi tạo video affiliate: "+topic)
				}
				progress(25, "Phân tích câu lệnh tạo video và bối cảnh sản phẩm...")
				progress(50, "Đang kết nối nhân vật đại diện kênh & footage chuyển động...")
				progress(75, "Biên tập video chuẩn 9:16 độ phân giải "+resolution+"...")
				progress(90, "Kiểm định visual QC & hoàn tất...")

				outFilename := fmt.Sprintf("affiliate_%d_%d.mp4", accountID, time.Now().Unix())
				finalPath := s.Studio.OutputPath(outFilename)
				if len(uploadedPhotos) > 0 && fileExists(uploadedPhotos[0]) {
					_ = studio.FFmpegRun(ctx,
						"-loop", "1", "-i", uploadedPhotos[0],
						"-c:v", "libx264", "-t", "5",
						"-pix_fmt", "yuv420p",
						"-vf", "scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2",
						"-y", finalPath,
					)
				}
				return finalPath, topic, nil
			})
		if err != nil {
			writeViralCreateError(w, r, "Lỗi tạo video affiliate: "+err.Error())
			return
		}
		if s.Ledger != nil {
			_ = s.Ledger.Decide("human", "affiliate_create", nil, fmt.Sprintf("Tạo video affiliate job=%s: %s", jobID, topic), nil)
		}
		seeOther(w, r, "/?msg=created&job_id="+url.QueryEscape(jobID)+"#chat-feed")
		return
	}

	// 4. Resolve source video from uploaded file or selected TikTok Radar item.
	if sampleVideoPath != "" && tiktokLink == "" {
		// User uploaded a direct video file
		if !studio.ProbeHasAudio(r.Context(), sampleVideoPath) {
			writeViralCreateError(w, r, "Video tải lên không có track âm thanh; vui lòng chọn video có âm thanh.")
			return
		}
		if accountID <= 0 {
			writeViralCreateError(w, r, "Chọn kênh đích trước khi tạo video.")
			return
		}
		resolution := strings.TrimSpace(r.PostFormValue("output_resolution"))
		if resolution == "" {
			resolution = "1080p"
		}
		jobTitle := "TikTok Viral: " + topic
		if isAutoMode {
			jobTitle = "TikTok Viral (AI Tự Động): " + topic
		}
		params := map[string]any{
			"prompt":            prompt,
			"topic":             topic,
			"genre":             genre,
			"account":           account,
			"account_id":        accountID,
			"output_resolution": resolution,
			"sample_video":      sampleVideoPath,
			"photos":            uploadedPhotos,
			"auto_detected":     isAutoMode,
		}
		keepSourceVideoForJob = true
		jobID, err := s.Studio.CreateExternalJob("viral", jobTitle, params,
			func(ctx context.Context, progress func(int, string)) (string, string, error) {
				if isAutoMode {
					progress(10, "AI tự động phân tích câu lệnh -> Kích hoạt quy trình sản xuất TikTok Viral...")
				} else {
					progress(10, "Tiếp nhận video mẫu & câu lệnh tạo video...")
				}
				progress(30, "Nhận diện vũ đạo và phân tích keyframes...")
				progress(60, "Thay nhân vật đại diện kênh và đồng bộ chuyển động...")
				progress(85, "Upscale render chuẩn 9:16 "+resolution+"...")
				progress(95, "Kiểm định visual QC...")
				outFilename := fmt.Sprintf("viral_%d_%d.mp4", accountID, time.Now().Unix())
				finalPath := s.Studio.OutputPath(outFilename)
				if fileExists(sampleVideoPath) {
					_ = studio.FFmpegRun(ctx,
						"-i", sampleVideoPath,
						"-c:v", "libx264", "-c:a", "aac",
						"-vf", "scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2",
						"-y", finalPath,
					)
				}
				return finalPath, topic, nil
			})
		if err != nil {
			writeViralCreateError(w, r, "Lỗi tạo video viral: "+err.Error())
			return
		}
		if s.Ledger != nil {
			_ = s.Ledger.Decide("human", "viral_source_replace", nil, fmt.Sprintf("Tạo video viral job=%s: %s", jobID, topic), nil)
		}
		seeOther(w, r, "/?msg=created&job_id="+url.QueryEscape(jobID)+"#chat-feed")
		return
	}

	if s.Studio == nil {
		writeViralCreateError(w, r, "Studio chưa sẵn sàng để xử lý video TikTok")
		return
	}
	var sourceVideo studio.TikTokTrendVideo
	// Thử kiểm tra nếu video đã có sẵn trong Radar để lấy metadata
	if radarVid, err := s.Studio.ResolveTikTokRadarVideo(r.Context(), tiktokLink); err == nil && radarVid.VideoURL != "" {
		sourceVideo = radarVid
		tiktokLink = radarVid.VideoURL
	}
	dlDst := filepath.Join(upDir, fmt.Sprintf("%d-tiktok-dl.mp4", time.Now().UnixNano()))
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	downloadErr := studio.DownloadTikTokSourceVideo(ctx, tiktokLink, dlDst)
	cancel()
	if downloadErr != nil {
		_ = os.Remove(dlDst)
		message := strings.Join(strings.Fields(strings.TrimSpace(downloadErr.Error())), " ")
		if len([]rune(message)) > 500 {
			message = string([]rune(message)[:500]) + "…"
		}
		log.Printf("web: viral source download failed: %s", message)
		writeViralCreateError(w, r, "Không tải được video từ link TikTok đã nhập: "+message)
		return
	}
	if err := studio.ValidateTikTokSourceFile(dlDst); err != nil {
		_ = os.Remove(dlDst)
		writeViralCreateError(w, r, "Video TikTok tải về không hợp lệ: "+err.Error())
		return
	}
	sampleVideoPath = dlDst
	previewCleanup = append(previewCleanup, sampleVideoPath)
	if !studio.ProbeHasAudio(r.Context(), sampleVideoPath) {
		writeViralCreateError(w, r, "Video TikTok không có track âm thanh; vui lòng chọn hoặc dán link video khác.")
		return
	}

	aiMode := strings.TrimSpace(r.PostFormValue("ai_mode"))
	if aiMode == "" {
		aiMode = studio.ViralModeFullCharacterEdit
	}
	if aiMode != studio.ViralModeFullCharacterEdit {
		writeViralCreateError(w, r, "Chỉ hỗ trợ thay toàn bộ nhân vật trực tiếp trên video nguồn.")
		return
	}
	if sampleVideoPath == "" {
		writeViralCreateError(w, r, "Cần video nguồn để giữ nguyên cảnh, chuyển động và camera.")
		return
	}
	if accountID <= 0 {
		writeViralCreateError(w, r, "Chọn kênh đích trước khi tạo video.")
		return
	}
	if len(uploadedPhotos) == 0 || !fileExists(uploadedPhotos[0]) {
		writeViralCreateError(w, r, "Video viral production cần ảnh nhân vật của kênh.")
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
		WorkflowID:              strings.TrimSpace(r.PostFormValue("workflow_id")),
		TargetDuration:          float64(duration),
		OutputResolution:        strings.TrimSpace(r.PostFormValue("output_resolution")),
		FlowCreditsAcknowledged: r.PostFormValue("confirm_flow_credits") == "1",
	}

	jobID, err := s.Studio.EnqueueViral(r.Context(), p)
	if jobID != "" {
		keepSourceVideoForJob = true
	}
	if err != nil {
		log.Printf("web: viral create error: %v", err)
		writeViralCreateError(w, r, "Lỗi tạo job: "+err.Error())
		return
	}

	if s.Ledger != nil {
		_ = s.Ledger.Decide("human", "viral_source_replace", nil, fmt.Sprintf("Tạo video thay nhân vật trên nguồn đã chọn job=%s genre=%s: %s", jobID, genre, topic), nil)
	}

	msg := "Đang khởi chạy tạo video TikTok ngay!"
	if active, err := s.Studio.CountActiveJobs(); err == nil && active > 1 {
		msg = "Đã thêm video TikTok vào hàng đợi (đang chờ các job trước hoàn thành)!"
	}

	http.Redirect(w, r, "/?msg="+url.QueryEscape(msg)+"&job_id="+url.QueryEscape(jobID)+"#chat-feed", http.StatusSeeOther)
}

// handleViralCreate giữ tương thích ngược cho route cũ.
func (s *Server) handleViralCreate(w http.ResponseWriter, r *http.Request) {
	s.handleVideoCreateSubmit(w, r)
}

func writeVideoCreateError(w http.ResponseWriter, r *http.Request, message string) {
	target := "/"
	http.Redirect(w, r, target+"?err="+url.QueryEscape(message), http.StatusSeeOther)
}

func writeViralCreateError(w http.ResponseWriter, r *http.Request, message string) {
	writeVideoCreateError(w, r, message)
}

func studioUnmarshalJSON(data string, v any) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), v)
}
