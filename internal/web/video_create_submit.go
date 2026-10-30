package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/products"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
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
		} else if strings.Contains(lower, "shopee.vn") || strings.Contains(lower, "shope.ee") || strings.Contains(lower, "vn.shp.ee") || strings.Contains(lower, "s.shopee.vn") ||
			strings.Contains(lower, "lazada.vn") || strings.Contains(lower, "lzd.co") || strings.Contains(lower, "s.lazada.vn") ||
			strings.Contains(lower, "tiki.vn") || strings.Contains(lower, "ti.ki") || strings.Contains(lower, "sendo.vn") {
			if res.ProductURL == "" {
				res.ProductURL = u
			}
		} else if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".webp") ||
			strings.Contains(lower, "susercontent.com") || strings.Contains(lower, "byteoversea.com") || strings.Contains(lower, "image_process") {
			res.ImageURLs = append(res.ImageURLs, u)
		} else if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".mov") || strings.HasSuffix(lower, ".webm") {
			res.VideoURLs = append(res.VideoURLs, u)
		}
	}
	return res
}

type promptDirectives struct {
	Resolution string
	Voice      string
	CleanTopic string
	Genre      string
}

// parsePromptDirectives bóc tách các chỉ thị chất lượng, giọng đọc và thể loại trực tiếp từ câu lệnh prompt.
func parsePromptDirectives(prompt string) promptDirectives {
	var d promptDirectives
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "2k") || strings.Contains(lower, "1440p") || strings.Contains(lower, "qhd") || strings.Contains(lower, "sắc nét") || strings.Contains(lower, "chất lượng cao") {
		d.Resolution = "2K"
	} else if strings.Contains(lower, "1080p") || strings.Contains(lower, "full hd") {
		d.Resolution = "1080p"
	}
	if strings.Contains(lower, "giọng nam") {
		d.Voice = "male"
	} else if strings.Contains(lower, "giọng nữ") {
		d.Voice = "default"
	}
	if strings.Contains(lower, "vũ đạo") || strings.Contains(lower, "nhảy") || strings.Contains(lower, "dance") {
		d.Genre = "dance"
	} else if strings.Contains(lower, "thời trang") || strings.Contains(lower, "outfit") || strings.Contains(lower, "tryon") || strings.Contains(lower, "phối đồ") {
		d.Genre = "fashion"
	} else if strings.Contains(lower, "truyện") || strings.Contains(lower, "kể chuyện") || strings.Contains(lower, "podcast") {
		d.Genre = "story"
	} else if strings.Contains(lower, "review") || strings.Contains(lower, "sản phẩm") || strings.Contains(lower, "đập hộp") || strings.Contains(lower, "unboxing") {
		d.Genre = "affiliate"
	}

	clean := promptURLRegex.ReplaceAllString(prompt, "")
	clean = strings.TrimSpace(clean)
	clean = strings.Join(strings.Fields(clean), " ")
	d.CleanTopic = clean
	return d
}

// detectAutoWorkflow tự động xác định danh mục và preset workflow tối ưu nhất từ câu lệnh và tư liệu đính kèm.
func detectAutoWorkflow(prompt, topic, productURL, tiktokLink string, photoCount int, hasVideo bool, explicitWF string) (outputType, workflowID string) {
	if explicitWF != "" && explicitWF != "auto" {
		if strings.Contains(explicitWF, "affiliate") {
			return "affiliate", explicitWF
		} else if strings.Contains(explicitWF, "viral") {
			return "viral", explicitWF
		}
		return "workflow", explicitWF
	}

	if strings.TrimSpace(productURL) != "" {
		return "affiliate", "preset_affiliate_daily"
	}
	if strings.TrimSpace(tiktokLink) != "" {
		return "viral", "preset_viral_dance"
	}

	content := strings.ToLower(prompt + " " + topic)
	if strings.Contains(content, "thời trang") || strings.Contains(content, "outfit") || strings.Contains(content, "phối đồ") || strings.Contains(content, "tryon") {
		return "workflow", "preset_fashion_tryon"
	}

	affType := detectAutoVideoType(prompt, topic, productURL, tiktokLink, photoCount, hasVideo)
	if affType == "affiliate" {
		return "affiliate", "preset_affiliate_daily"
	}
	return "viral", "preset_viral_dance"
}

type compositeVideoConfig struct {
	FinalPath       string
	Resolution      string
	Photos          []string
	CharacterPhoto  string
	SampleVideoPath string
	Voice           string
	NarrationText   string
}

// renderCompositeVideo dựng video chất lượng cao chuẩn TikTok 9:16 (1080p hoặc 2K)
// kết hợp mượt mà tư liệu nhiều ảnh (multi-image slideshow), ảnh nhân vật đại diện (PIP badge),
// âm thanh từ video mẫu hoặc thuyết minh AI (TTS voiceover).
func (s *Server) renderCompositeVideo(ctx context.Context, cfg compositeVideoConfig) (string, error) {
	width, height := 1080, 1920
	res := strings.ToUpper(strings.TrimSpace(cfg.Resolution))
	if res == "2K" || res == "2K QHD" || res == "1440P" {
		width, height = 1440, 2560
	}

	var validPhotos []string
	for _, p := range cfg.Photos {
		if fileExists(p) {
			validPhotos = append(validPhotos, p)
		}
	}

	// 1. Tổng hợp giọng đọc TTS nếu có kịch bản/nội dung
	var ttsAudioPath string
	if s.TTS != nil && strings.TrimSpace(cfg.NarrationText) != "" {
		voice := cfg.Voice
		if voice == "" || voice == "vi-VN-Standard-A" {
			voice = "default"
		}
		ttsCtx, ttsCancel := context.WithTimeout(ctx, 20*time.Second)
		if audioBytes, err := s.TTS.Synthesize(ttsCtx, cfg.NarrationText, voice); err == nil && len(audioBytes) > 0 {
			tmp := filepath.Join(s.Studio.UploadDir(), fmt.Sprintf("tts_voice_%d.mp3", time.Now().UnixNano()))
			if err := os.WriteFile(tmp, audioBytes, 0o644); err == nil {
				ttsAudioPath = tmp
				defer os.Remove(tmp)
			}
		}
		ttsCancel()
	}

	// 2. Trường hợp A: Có video mẫu tham chiếu (Viral Motion hoặc Sample Video)
	if cfg.SampleVideoPath != "" && fileExists(cfg.SampleVideoPath) {
		hasAudio := studio.ProbeHasAudio(ctx, cfg.SampleVideoPath)
		if cfg.CharacterPhoto != "" && fileExists(cfg.CharacterPhoto) {
			badgeSize := width / 5
			vf := fmt.Sprintf(
				"[0:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1[bg];"+
					"[1:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1[pip];"+
					"[bg][pip]overlay=main_w-overlay_w-40:40[vout]",
				width, height, width, height,
				badgeSize, badgeSize, badgeSize, badgeSize,
			)
			var args []string
			args = append(args, "-i", cfg.SampleVideoPath, "-i", cfg.CharacterPhoto)
			if ttsAudioPath != "" {
				args = append(args, "-i", ttsAudioPath)
				vf += ";[0:a][2:a]amix=inputs=2:duration=first:dropout_transition=2[aout]"
				args = append(args, "-filter_complex", vf, "-map", "[vout]", "-map", "[aout]")
			} else {
				args = append(args, "-filter_complex", vf, "-map", "[vout]")
				if hasAudio {
					args = append(args, "-map", "0:a")
				}
			}
			args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-shortest", "-y", cfg.FinalPath)
			if err := studio.FFmpegRun(ctx, args...); err == nil && fileExists(cfg.FinalPath) {
				return cfg.FinalPath, nil
			}
		}

		vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1", width, height, width, height)
		var args []string
		args = append(args, "-i", cfg.SampleVideoPath)
		if ttsAudioPath != "" {
			args = append(args, "-i", ttsAudioPath, "-filter_complex", "[0:a][1:a]amix=inputs=2:duration=first:dropout_transition=2[aout]", "-vf", vf, "-map", "0:v", "-map", "[aout]")
		} else {
			args = append(args, "-vf", vf)
			if hasAudio {
				args = append(args, "-map", "0:v", "-map", "0:a")
			} else {
				args = append(args, "-map", "0:v")
			}
		}
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-y", cfg.FinalPath)
		return cfg.FinalPath, studio.FFmpegRun(ctx, args...)
	}

	// 3. Trường hợp B: Dựng video từ thư viện ảnh sản phẩm & nhân vật (Không có video mẫu)
	// Quy định thời lượng: Tối thiểu 30 giây, tối đa 60 giây khi không có video mẫu.
	targetDuration := 30
	if ttsAudioPath != "" {
		ttsDur := studio.ProbeDuration(ctx, ttsAudioPath)
		if ttsDur > 30 {
			targetDuration = int(math.Min(math.Ceil(ttsDur), 60))
		}
	}

	if len(validPhotos) == 0 {
		args := []string{
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=#0f172a:s=%dx%d:d=%d", width, height, targetDuration),
		}
		if ttsAudioPath != "" {
			args = append(args, "-i", ttsAudioPath, "-c:a", "aac", "-b:a", "192k", "-t", strconv.Itoa(targetDuration))
		} else {
			args = append(args, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100", "-c:a", "aac", "-t", strconv.Itoa(targetDuration))
		}
		args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", cfg.FinalPath)
		return cfg.FinalPath, studio.FFmpegRun(ctx, args...)
	}

	if len(validPhotos) == 1 {
		totalFrames := targetDuration * 30
		vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,zoompan=z='min(zoom+0.0008,1.15)':d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d:fps=30,setsar=1", width, height, width, height, totalFrames, width, height)
		args := []string{"-loop", "1", "-t", strconv.Itoa(targetDuration), "-i", validPhotos[0]}
		if ttsAudioPath != "" {
			args = append(args, "-i", ttsAudioPath, "-filter_complex", "[0:v]"+vf+"[vout];[1:a]apad=whole_dur="+strconv.Itoa(targetDuration)+"[aout]", "-map", "[vout]", "-map", "[aout]", "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k", "-t", strconv.Itoa(targetDuration), "-y", cfg.FinalPath)
		} else {
			args = append(args, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100", "-vf", vf, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-t", strconv.Itoa(targetDuration), "-y", cfg.FinalPath)
		}
		return cfg.FinalPath, studio.FFmpegRun(ctx, args...)
	}

	// Đa ảnh: Tính toán thời lượng hiển thị từng ảnh sao cho tổng thời lượng đạt tối thiểu 30s và tối đa 60s
	numPhotos := len(validPhotos)
	perImgSec := int(math.Ceil(float64(targetDuration) / float64(numPhotos)))
	if perImgSec < 4 {
		perImgSec = 4
	}
	actualTotalSec := perImgSec * numPhotos
	if actualTotalSec > 60 {
		actualTotalSec = 60
		perImgSec = actualTotalSec / numPhotos
		if perImgSec < 3 {
			perImgSec = 3
		}
	}

	var inputArgs []string
	var filterParts []string
	for i, p := range validPhotos {
		inputArgs = append(inputArgs, "-loop", "1", "-t", strconv.Itoa(perImgSec), "-i", p)
		filterParts = append(filterParts, fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1[v%d]", i, width, height, width, height, i))
	}
	var concatInputs string
	for i := range validPhotos {
		concatInputs += fmt.Sprintf("[v%d]", i)
	}
	filterParts = append(filterParts, fmt.Sprintf("%sconcat=n=%d:v=1:a=0[vcat]", concatInputs, len(validPhotos)))

	finalMap := "[vcat]"
	nextIdx := len(validPhotos)

	if cfg.CharacterPhoto != "" && fileExists(cfg.CharacterPhoto) && cfg.CharacterPhoto != validPhotos[0] {
		inputArgs = append(inputArgs, "-i", cfg.CharacterPhoto)
		badgeSize := width / 5
		filterParts = append(filterParts,
			fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1[pip]", nextIdx, badgeSize, badgeSize, badgeSize, badgeSize),
			fmt.Sprintf("[vcat][pip]overlay=main_w-overlay_w-40:40[vout]"),
		)
		finalMap = "[vout]"
		nextIdx++
	}

	var audioArgs []string
	if ttsAudioPath != "" {
		inputArgs = append(inputArgs, "-i", ttsAudioPath)
		filterParts = append(filterParts, fmt.Sprintf("[%d:a]apad=whole_dur=%d[aout]", nextIdx, actualTotalSec))
		audioArgs = []string{"-map", "[aout]", "-c:a", "aac", "-b:a", "192k"}
	} else {
		inputArgs = append(inputArgs, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100")
		audioArgs = []string{"-map", fmt.Sprintf("%d:a", nextIdx), "-c:a", "aac"}
	}

	var fullArgs []string
	fullArgs = append(fullArgs, inputArgs...)
	fullArgs = append(fullArgs, "-filter_complex", strings.Join(filterParts, ";"), "-map", finalMap)
	fullArgs = append(fullArgs, audioArgs...)
	fullArgs = append(fullArgs, "-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-t", strconv.Itoa(actualTotalSec), "-y", cfg.FinalPath)

	return cfg.FinalPath, studio.FFmpegRun(ctx, fullArgs...)
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

// handleVideoCreateSubmit tiếp nhận form khởi tạo video AI mới (/).
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
	var productPhotos []string
	var productID int64
	var previewCleanup []string
	retainedUploads := make(map[string]struct{})
	var sampleVideoPath string
	defer func() {
		for _, path := range previewCleanup {
			if _, retained := retainedUploads[path]; retained {
				continue
			}
			_ = os.Remove(path)
		}
	}()
	for _, key := range []string{"character_photos", "character_photo", "model_photos", "model_photo", "photos", "image_files"} {
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
	if photos, uploadErr := saveUploads(r, "product_photos", upDir, 10); uploadErr != nil {
		writeViralCreateError(w, r, "Ảnh sản phẩm lỗi: "+uploadErr.Error())
		return
	} else if len(photos) > 0 {
		productPhotos = append(productPhotos, photos...)
		previewCleanup = append(previewCleanup, photos...)
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
	if len(productPhotos) == 0 && len(extracted.ImageURLs) > 0 && s.Studio != nil {
		for i, imgURL := range extracted.ImageURLs {
			if i >= 3 {
				break
			}
			dst := filepath.Join(upDir, fmt.Sprintf("prompt_img_%d_%d.jpg", time.Now().UnixNano(), i))
			dlCtx, dlCancel := context.WithTimeout(r.Context(), 15*time.Second)
			if err := studio.DownloadImage(dlCtx, imgURL, dst); err == nil {
				productPhotos = append(productPhotos, dst)
				previewCleanup = append(previewCleanup, dst)
			}
			dlCancel()
		}
	}

	// Bóc tách chỉ thị chất lượng, giọng đọc và thể loại từ câu lệnh prompt
	directives := parsePromptDirectives(prompt)
	if directives.CleanTopic != "" && (topic == "" || topic == prompt || strings.HasPrefix(topic, "http")) {
		if len([]rune(directives.CleanTopic)) > 50 {
			topic = string([]rune(directives.CleanTopic)[:50]) + "…"
		} else {
			topic = directives.CleanTopic
		}
	}

	// Tự động fetch dữ liệu và ảnh sản phẩm nếu có link sản phẩm affiliate
	if productURL != "" && s.Studio != nil {
		extractCtx, extractCancel := context.WithTimeout(r.Context(), 25*time.Second)
		if extProd, err := products.ExtractFromAffiliateLink(extractCtx, productURL); err == nil && extProd != nil {
			if (topic == "" || strings.HasPrefix(topic, "http")) && extProd.Title != "" {
				topic = extProd.Title
				if len([]rune(topic)) > 50 {
					topic = string([]rune(topic)[:50]) + "…"
				}
			}
			if s.Products != nil {
				productID, _ = s.Products.SaveImportedProduct(*extProd)
			}
			if len(productPhotos) == 0 && len(extProd.ImageURLs) > 0 {
				for idx, imgURL := range extProd.ImageURLs {
					if idx >= 5 {
						break
					}
					dst := filepath.Join(upDir, fmt.Sprintf("prod_dl_%d_%d.jpg", time.Now().UnixNano(), idx))
					dlCtx, dlCancel := context.WithTimeout(r.Context(), 15*time.Second)
					if dlErr := studio.DownloadImage(dlCtx, imgURL, dst); dlErr == nil && fileExists(dst) {
						productPhotos = append(productPhotos, dst)
						previewCleanup = append(previewCleanup, dst)
					}
					dlCancel()
				}
			}
		}
		extractCancel()
	}

	// Tự động sử dụng nhân vật đã setting trong kênh nếu người dùng chưa tải ảnh mới
	var channelPhotos []string
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
	if accountID > 0 && s.Studio != nil {
		if ms, err := s.Studio.ModelLibrary(); err == nil && ms != nil {
			if photos, err := ms.List(accountID); err == nil && len(photos) > 0 {
				for _, ph := range photos {
					if ph.Path != "" && fileExists(ph.Path) {
						channelPhotos = append(channelPhotos, ph.Path)
					}
				}
			}

		}
	}
	channelPhotos = prioritizeSelectedCharacterPhotos(channelPhotos, uploadedPhotos)

	workflowID := strings.TrimSpace(r.PostFormValue("workflow_id"))
	// Tự động phân tích câu lệnh và chọn quy trình sản xuất (Workflow) phù hợp
	isAutoMode := outputType == "auto" || outputType == ""
	if isAutoMode {
		detType, detWF := detectAutoWorkflow(prompt, topic, productURL, tiktokLink, len(uploadedPhotos), sampleVideoPath != "", workflowID)
		outputType = detType
		if workflowID == "" || workflowID == "auto" {
			workflowID = detWF
		}
		log.Printf("web: auto-detected workflow output type %q workflow_id %q for topic %q", outputType, workflowID, topic)
	}
	if message := missingChannelCharacter(outputType, channelPhotos); message != "" {
		writeViralCreateError(w, r, message)
		return
	}

	resolution := strings.TrimSpace(r.PostFormValue("output_resolution"))
	if resolution == "" {
		if directives.Resolution != "" {
			resolution = directives.Resolution
		} else {
			resolution = "1080p"
		}
	}
	if voice == "" || voice == "vi-VN-Standard-A" {
		if directives.Voice != "" {
			voice = directives.Voice
		} else {
			voice = "default"
		}
	}

	// 3. Handle Affiliate video generation
	if outputType == "affiliate" {
		if accountID <= 0 {
			writeViralCreateError(w, r, "Chọn kênh đích trước khi tạo video.")
			return
		}
		if workflowID == "" || workflowID == "preset_affiliate_daily" {
			workflowID = "preset_affiliate_showcase"
		}
		jobTitle := "Video Affiliate: " + topic
		if isAutoMode {
			jobTitle = "Video Affiliate (AI Tự Động): " + topic
		}
		runID := fmt.Sprintf("wf-aff-%d-%d", accountID, time.Now().Unix())
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
			"workflow_id":       workflowID,
			"run_id":            runID,
		}
		if sampleVideoPath != "" {
			params["sample_video"] = sampleVideoPath
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

				graph, flowErr := loadAffiliateFlow(s.Studio, workflowID)
				if flowErr != nil {
					return "", "", flowErr
				}
				targetDur := 30.0
				if sampleVideoPath != "" {
					if sd := studio.ProbeDuration(ctx, sampleVideoPath); sd > 0 {
						targetDur = sd
					}
				}
				flowInputs := prepareAffiliateFlow(graph, affiliateFlowConfig{
					Topic: topic, ProductURL: productURL, Prompt: prompt,
					Resolution: resolution, Voice: voice, Duration: targetDur,
					ReferenceVideo: sampleVideoPath, ProductID: productID,
					ProductPhotos: productPhotos, CharacterPhotos: channelPhotos,
				})
				services := s.flowExecutionServices()
				services.MuseCreditsAcknowledged = true
				services.GoogleFlowCreditsAcknowledged = true
				flowRun, flowErr := s.Studio.ExecuteFlowGraphWithInputs(ctx, runID, graph, flowInputs, services)
				if flowErr != nil {
					return "", "", fmt.Errorf("workflow affiliate thất bại: %w", flowErr)
				}
				if flowRun == nil || flowRun.Status != "completed" {
					status := "không có kết quả"
					if flowRun != nil {
						status = flowRun.Status
					}
					return "", "", fmt.Errorf("workflow affiliate chưa hoàn tất (status=%s)", status)
				}
				vid, ok := flowRun.Outputs["video"].(string)
				if !ok || vid == "" || !fileExists(vid) {
					return "", "", fmt.Errorf("workflow affiliate không xuất video hợp lệ")
				}
				if err := copyFile(vid, finalPath); err != nil {
					return "", "", fmt.Errorf("không lưu được video affiliate: %w", err)
				}
				return finalPath, topic, nil
			})
		if err != nil {
			writeViralCreateError(w, r, "Lỗi tạo video affiliate: "+err.Error())
			return
		}
		for _, path := range productPhotos {
			retainedUploads[path] = struct{}{}
		}
		if sampleVideoPath != "" {
			retainedUploads[sampleVideoPath] = struct{}{}
		}
		if s.Ledger != nil {
			_ = s.Ledger.Decide("human", "affiliate_create", nil, fmt.Sprintf("Tạo video affiliate job=%s: %s", jobID, topic), nil)
		}
		seeOther(w, r, "/?msg=created&job_id="+url.QueryEscape(jobID)+"#chat-feed")
		return
	}

	// 4. Resolve source video from uploaded file or selected TikTok Radar item.
	if sampleVideoPath != "" && tiktokLink == "" {
		if !studio.ProbeHasAudio(r.Context(), sampleVideoPath) {
			writeViralCreateError(w, r, "Video tải lên không có track âm thanh; vui lòng chọn video có âm thanh.")
			return
		}
		if accountID <= 0 {
			writeViralCreateError(w, r, "Chọn kênh đích trước khi tạo video.")
			return
		}
		jobTitle := "TikTok Viral: " + topic
		if isAutoMode {
			jobTitle = "TikTok Viral (AI Tự Động): " + topic
		}
		runID := fmt.Sprintf("wf-viral-%d-%d", accountID, time.Now().Unix())
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
			"workflow_id":       workflowID,
			"run_id":            runID,
		}
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

				fs, err := s.Studio.GetFlowStore()
				if err != nil {
					return "", "", fmt.Errorf("không mở được workflow viral: %w", err)
				}
				if fs == nil {
					return "", "", fmt.Errorf("kho workflow viral chưa sẵn sàng")
				}
				graph, err := fs.GetGraph("preset_viral_dance")
				if err != nil {
					return "", "", fmt.Errorf("không tìm thấy preset_viral_dance: %w", err)
				}
				if graph == nil {
					return "", "", fmt.Errorf("preset_viral_dance không có dữ liệu")
				}
				flowInputs := prepareViralFlow(graph, viralFlowConfig{
					SourceVideo: sampleVideoPath, Resolution: resolution,
					CharacterPhotos: channelPhotos, RequireRadar: false,
				})
				services := s.flowExecutionServices()
				services.MuseCreditsAcknowledged = true
				services.GoogleFlowCreditsAcknowledged = true
				flowRun, flowErr := s.Studio.ExecuteFlowGraphWithInputs(ctx, runID, graph, flowInputs, services)
				if flowErr != nil {
					return "", "", fmt.Errorf("workflow viral thất bại: %w", flowErr)
				}
				if flowRun == nil || flowRun.Status != "completed" {
					status := "không có kết quả"
					if flowRun != nil {
						status = flowRun.Status
					}
					return "", "", fmt.Errorf("workflow viral chưa hoàn tất (status=%s)", status)
				}
				vid, ok := flowRun.Outputs["video"].(string)
				if !ok || vid == "" || !fileExists(vid) {
					return "", "", fmt.Errorf("workflow viral không xuất video hợp lệ")
				}
				if err := copyFile(vid, finalPath); err != nil {
					return "", "", fmt.Errorf("không lưu được video viral: %w", err)
				}
				return finalPath, topic, nil
			})
		if err != nil {
			writeViralCreateError(w, r, "Lỗi tạo video viral: "+err.Error())
			return
		}
		if sampleVideoPath != "" {
			retainedUploads[sampleVideoPath] = struct{}{}
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
	if len(channelPhotos) == 0 || !fileExists(channelPhotos[0]) {
		writeViralCreateError(w, r, "Video viral production cần ảnh nhân vật của kênh.")
		return
	}
	modelPhoto := channelPhotos[0]

	p := studio.ViralParams{
		Topic:             topic,
		Genre:             genre,
		VisualMood:        visualMood,
		Voice:             voice,
		Account:           account,
		AccountID:         accountID,
		ModelPhoto:        modelPhoto,
		ModelPhotos:       channelPhotos,
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
		FlowCreditsAcknowledged: true,
	}

	jobID, err := s.Studio.EnqueueViral(r.Context(), p)
	if jobID != "" {
		retainedUploads[sampleVideoPath] = struct{}{}
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
