package web

import (
	"context"
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleStudioAdCreative queues a high quality ad creative video job leveraging Muse AI clips,
// Gemini Imagen 3 photos, and AI voiceover.
func (s *Server) handleStudioAdCreative(w http.ResponseWriter, r *http.Request) {
	studioErr := func(msg string) {
		seeOther(w, r, "/studio?err="+url.QueryEscape(msg))
	}
	if s.Studio == nil {
		studioErr("Studio chưa sẵn sàng — thử tải lại trang.")
		return
	}
	if err := r.ParseMultipartForm(120 << 20); err != nil {
		if perr := r.ParseForm(); perr != nil {
			studioErr("Không đọc được form — thử lại.")
			return
		}
	}
	if (r.MultipartForm != nil && len(r.MultipartForm.File["clip_files"]) > 0) ||
		strings.TrimSpace(r.FormValue("clip_urls")) != "" || strings.TrimSpace(r.FormValue("sample_clip")) != "" {
		studioErr("Clip render sẵn không được nối thành affiliate. Hãy dùng ảnh listing để tạo footage mới, nhân vật kênh phải mặc hoặc dùng đúng sản phẩm.")
		return
	}

	upDir := s.Studio.UploadDir()
	var accountID int64
	if raw := strings.TrimSpace(r.FormValue("account_id")); raw != "" {
		accountID, _ = strconv.ParseInt(raw, 10, 64)
	}
	if accountID == 0 && strings.TrimSpace(r.FormValue("account")) != "" && s.Mgr != nil {
		if accounts, err := s.Mgr.ListWithAutopilot(); err == nil {
			for _, account := range accounts {
				if account != nil && account.Username == strings.TrimSpace(r.FormValue("account")) {
					accountID = account.ID
					break
				}
			}
		}
	}
	if accountID <= 0 {
		studioErr("Chọn kênh đích để dùng đúng ảnh nhân vật.")
		return
	}
	productPhoto, err := saveUpload(r, "product_photo", upDir)
	if err != nil {
		studioErr("Ảnh sản phẩm lỗi: " + err.Error())
		return
	}
	var modelPhotos []string
	for _, key := range []string{"model_photos", "model_photo"} {
		photos, uploadErr := saveUploads(r, key, upDir, 10)
		if uploadErr != nil {
			studioErr("Ảnh người mẫu lỗi: " + uploadErr.Error())
			return
		}
		modelPhotos = append(modelPhotos, photos...)
	}
	if len(modelPhotos) == 0 {
		if photo, uploadErr := saveUpload(r, "model_photo", upDir); uploadErr != nil {
			studioErr("Ảnh người mẫu lỗi: " + uploadErr.Error())
			return
		} else if photo != "" {
			modelPhotos = append(modelPhotos, photo)
		}
	}
	if len(modelPhotos) > 0 {
		library, libErr := s.Studio.ModelLibrary()
		if libErr != nil {
			studioErr("Không đọc được thư viện nhân vật: " + libErr.Error())
			return
		}
		var stored []string
		for _, photoPath := range modelPhotos {
			photo, addErr := library.Add(accountID, photoPath)
			if addErr != nil {
				studioErr("Không lưu được ảnh nhân vật vào đúng kênh: " + addErr.Error())
				return
			}
			stored = append(stored, photo.Path)
		}
		modelPhotos = stored
	}
	var modelIDStrs []string
	if r.MultipartForm != nil {
		modelIDStrs = r.MultipartForm.Value["model_photo_ids"]
	}
	if len(modelIDStrs) == 0 {
		modelIDStrs = r.Form["model_photo_ids"]
	}
	if len(modelPhotos) == 0 && len(modelIDStrs) > 0 {
		var ids []int64
		for _, ms := range modelIDStrs {
			if id, err := strconv.ParseInt(strings.TrimSpace(ms), 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, ids)
			if len(modelPhotos) != len(ids) {
				studioErr("Ảnh đã chọn không thuộc thư viện của kênh đích.")
				return
			}
		}
	}
	if len(modelPhotos) == 0 && strings.TrimSpace(r.FormValue("model_id")) != "" {
		if mid, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("model_id")), 10, 64); err == nil && mid > 0 {
			modelPhotos = s.findModelPhotosByIDsForAccount(accountID, []int64{mid})
		}
	}
	if len(modelPhotos) == 0 {
		studioErr("Chọn hoặc tải ảnh nhân vật từ thư viện của kênh đã chọn.")
		return
	}
	modelPhoto := modelPhotos[0]
	extraPhotos, err := saveUploads(r, "extra_photos", upDir, 8)
	if err != nil {
		studioErr("Ảnh phụ lỗi: " + err.Error())
		return
	}

	// Download photos from URLs (e.g. Gemini Imagen 3, Web CDN)
	rawPhotoURLs := strings.TrimSpace(r.FormValue("photo_urls"))
	if rawPhotoURLs == "" {
		rawPhotoURLs = strings.TrimSpace(r.FormValue("photo_url"))
	}
	if rawPhotoURLs != "" {
		for i, line := range strings.Split(rawPhotoURLs, "\n") {
			u := strings.TrimSpace(line)
			if u == "" || !strings.HasPrefix(u, "http") {
				continue
			}
			dst := filepath.Join(upDir, fmt.Sprintf("%d-urlphoto-%d.png", time.Now().UnixNano(), i))
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
			if err := studio.DownloadImage(ctx, u, dst); err == nil {
				if productPhoto == "" {
					productPhoto = dst
				} else if len(extraPhotos) < 8 {
					extraPhotos = append(extraPhotos, dst)
				}
			}
			cancel()
		}
	}
	if productPhoto == "" {
		studioErr("Thiếu ảnh listing sản phẩm thật. Tải ảnh lên hoặc chọn sản phẩm có ảnh trong kệ hàng.")
		return
	}

	// Save Voiceover audio or synthesize from voiceover_text
	tiktokMusicOnly := r.FormValue("tiktok_music_only") == "1"
	voiceoverText := strings.TrimSpace(r.FormValue("voiceover_text"))
	voiceoverPath, _ := saveUpload(r, "voiceover_file", upDir)
	if !tiktokMusicOnly && voiceoverPath == "" && voiceoverText != "" && s.TTS != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		if audioBytes, err := s.TTS.Synthesize(ctx, voiceoverText, "default"); err == nil && len(audioBytes) > 0 {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-voiceover.mp3", time.Now().UnixNano()))
			if werr := os.WriteFile(dst, audioBytes, 0o644); werr == nil {
				voiceoverPath = dst
			}
		}
		cancel()
	}
	if tiktokMusicOnly {
		voiceoverPath = ""
		voiceoverText = ""
	}

	// Save Music
	musicPath, _ := saveUpload(r, "music_file", upDir)
	if musicPath == "" {
		if u := strings.TrimSpace(r.FormValue("music_url")); u != "" {
			dst := filepath.Join(upDir, fmt.Sprintf("%d-music.m4a", time.Now().UnixNano()))
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
			defer cancel()
			if derr := studio.DownloadAudio(ctx, u, dst); derr == nil {
				musicPath = dst
			}
		}
	}
	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	seconds = clampInt(seconds, 10, 120)
	bpm, _ := strconv.Atoi(r.FormValue("bpm"))

	bpm = clampInt(bpm, 60, 200)
	musicStart, _ := strconv.ParseFloat(r.FormValue("music_start"), 64)

	productName := strings.TrimSpace(r.FormValue("product_name"))
	if productName == "" {
		studioErr("Nhập tên sản phẩm theo listing trước khi dựng.")
		return
	}

	aspectRatio := strings.TrimSpace(r.FormValue("aspect_ratio"))
	if aspectRatio != "16:9" {
		aspectRatio = "9:16"
	}

	var assignedDepts []string
	if r.MultipartForm != nil {
		assignedDepts = r.MultipartForm.Value["assigned_depts"]
	}
	if len(assignedDepts) == 0 {
		assignedDepts = r.Form["assigned_depts"]
	}
	if len(assignedDepts) == 0 {
		assignedDepts = []string{"DEP-01", "DEP-02", "DEP-03", "DEP-04", "DEP-05"}
	}

	p := studio.AffiliateParams{
		Mode:          r.FormValue("mode"),
		AccountID:     accountID,
		Niche:         strings.TrimSpace(r.FormValue("niche")),
		ProductName:   productName,
		ProductPrice:  strings.TrimSpace(r.FormValue("product_price")),
		ProductPhoto:  productPhoto,
		ModelPhoto:    modelPhoto,
		ModelPhotos:   modelPhotos,
		ExtraPhotos:   extraPhotos,
		VoiceoverPath: voiceoverPath,
		VoiceoverText: voiceoverText,
		AspectRatio:   aspectRatio,
		Seconds:       seconds,
		BPM:           bpm,
		MusicPath:     musicPath,
		MusicStart:    musicStart,
		MusicTitle:    strings.TrimSpace(r.FormValue("music_title")),
		MusicArtist:   strings.TrimSpace(r.FormValue("music_artist")),
		FixedCaption:  strings.TrimSpace(r.FormValue("fixed_caption")),
		AffLink:       strings.TrimSpace(r.FormValue("aff_link")),
		AssignedDepts: assignedDepts,
	}

	if _, err := s.Studio.CreateAffiliateJob(p); err != nil {
		studioErr("Không tạo được video: " + err.Error())
		return
	}
	seeOther(w, r, "/studio/jobs?msg="+url.QueryEscape("Đang dựng video quảng cáo sản phẩm chất lượng cao…"))
}
