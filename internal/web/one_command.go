package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/automation"
	"github.com/ninhlee99/ai-creator-os/internal/studio"
)

// OneCommandResult phản hồi kết quả tạo video bằng 1 câu lệnh.
type OneCommandResult struct {
	OK          bool   `json:"ok"`
	JobID       string `json:"job_id"`
	Type        string `json:"type"` // viral | affiliate | story
	Title       string `json:"title"`
	RedirectURL string `json:"redirect_url"`
	Message     string `json:"message"`
}

// detectCommandIntent phân tích câu lệnh tiếng Việt tự nhiên để xác định chế độ sản xuất video tối ưu.
func detectCommandIntent(cmd string) (mode, topic, niche string) {
	raw := strings.TrimSpace(cmd)
	lower := strings.ToLower(raw)

	// Loại bỏ tiền tố mệnh lệnh thừa nếu có (vd: "Tạo video về...", "Hãy làm video...", "Sản xuất video...")
	cleanTopic := raw
	prefixes := []string{
		"tạo video về ", "tạo video ", "làm video về ", "làm video ",
		"hãy tạo video ", "hãy làm video ", "sản xuất video ", "quay video ",
		"giúp tôi tạo video ", "viết kịch bản và làm video ",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(strings.ToLower(cleanTopic), p) {
			cleanTopic = strings.TrimSpace(cleanTopic[len(p):])
			break
		}
	}
	if cleanTopic == "" {
		cleanTopic = raw
	}

	// 1. Kiểm tra ý định KỂ CHUYỆN AUDIO DRAMA 16:9 (YouTube)
	storyKeywords := []string{
		"kể chuyện", "truyện", "radio", "đêm khuya", "tâm sự",
		"lịch sử", "kinh dị", "truyện ma", "bí ẩn", "podcast",
		"cổ tích", "thần thoại", "tiểu thuyết", "triết lý",
	}
	for _, kw := range storyKeywords {
		if strings.Contains(lower, kw) {
			return "story", cleanTopic, "mystery"
		}
	}

	// 2. Kiểm tra ý định BÁN HÀNG / AFFILIATE / REVIEW SẢN PHẨM 9:16
	affiliateKeywords := []string{
		"review", "sản phẩm", "bán hàng", "affiliate", "mua", "deal",
		"giảm giá", "serum", "kem", "nồi chiên", "tai nghe", "son",
		"nước hoa", "thời trang", "váy", "áo", "túi xách", "đồng hồ",
		"điện thoại", "phụ kiện", "mỹ phẩm", "gia dụng",
	}
	for _, kw := range affiliateKeywords {
		if strings.Contains(lower, kw) {
			niche := "thời trang"
			if strings.Contains(lower, "công nghệ") || strings.Contains(lower, "tai nghe") || strings.Contains(lower, "điện thoại") || strings.Contains(lower, "laptop") || strings.Contains(lower, "bluetooth") {
				niche = "công nghệ"
			} else if strings.Contains(lower, "gia dụng") || strings.Contains(lower, "nồi") || strings.Contains(lower, "bếp") || strings.Contains(lower, "máy hút") {
				niche = "gia dụng"
			} else if strings.Contains(lower, "mỹ phẩm") || strings.Contains(lower, "serum") || strings.Contains(lower, "kem") || strings.Contains(lower, "son môi") || hasWord(lower, "son") {
				niche = "mỹ phẩm"
			}
			return "affiliate", cleanTopic, niche
		}
	}

	// 3. Mặc định là VIDEO VIRAL 9:16 (TikTok, Shorts, Reels)
	genre := "trendjack"
	if strings.Contains(lower, "vũ đạo") || strings.Contains(lower, "nhảy") || strings.Contains(lower, "dance") {
		genre = "dance"
	} else if strings.Contains(lower, "mẹo") || strings.Contains(lower, "bí quyết") || strings.Contains(lower, "cách") {
		genre = "lifehack"
	} else if strings.Contains(lower, "sự thật") || strings.Contains(lower, "bí mật") {
		genre = "mystery"
	} else if strings.Contains(lower, "công nghệ") || strings.Contains(lower, "ai") {
		genre = "tech_fact"
	}

	return "viral", cleanTopic, genre
}

// handleOneCommandCreate xử lý tạo video toàn diện chỉ từ 1 câu lệnh duy nhất của người dùng.
func (s *Server) handleOneCommandCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.respondOneCommand(w, r, OneCommandResult{
			OK:      false,
			Message: "Studio AI chưa sẵn sàng",
		}, http.StatusServiceUnavailable)
		return
	}
	var uploadedProductPhoto string
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			s.respondOneCommand(w, r, OneCommandResult{Message: "Ảnh vượt giới hạn 20 MB hoặc form không hợp lệ"}, http.StatusBadRequest)
			return
		}
		var err error
		uploadedProductPhoto, err = saveUpload(r, "product_photo", s.Studio.UploadDir())
		if err != nil {
			s.respondOneCommand(w, r, OneCommandResult{Message: "Không đọc được ảnh sản phẩm: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}

	command := strings.TrimSpace(r.FormValue("command"))
	if command == "" {
		command = strings.TrimSpace(r.FormValue("goal"))
	}
	if command == "" {
		command = strings.TrimSpace(r.FormValue("prompt"))
	}

	if command == "" {
		s.respondOneCommand(w, r, OneCommandResult{
			OK:      false,
			Message: "Vui lòng nhập câu lệnh tạo video (Ví dụ: Tạo video review serum B5 hoặc Kể chuyện bí ẩn)",
		}, http.StatusBadRequest)
		return
	}

	mode, topic, nicheOrGenre := detectCommandIntent(command)

	var jobID string
	var err error
	var redirectURL string
	var msg string

	switch mode {
	case "story":
		// Khởi tạo Kể chuyện YouTube 16:9 chuẩn 5 sao
		p := studio.StoryParams{
			Topic:           topic,
			Genre:           nicheOrGenre,
			Words:           s.atSettingInt(automation.KeyStoryWords, 600),
			Scenes:          s.atSettingInt(automation.KeyStoryScenes, 6),
			MusicOn:         s.atSettingOn(automation.KeyStoryMusicOn, false),
			DurationMinutes: 0,
		}
		jobID, err = s.Studio.CreateStoryJob(p)
		if err == nil {
			redirectURL = "/stories?ok=" + url.QueryEscape(fmt.Sprintf("✓ Đang sản xuất truyện audio: %s", topic))
			msg = fmt.Sprintf("Đã bắt đầu sản xuất truyện audio 16:9: %s", topic)
		}

	case "affiliate":
		mediaReady := false
		if mg := s.Studio.MediaGen(); mg != nil {
			mediaReady = mg.Healthy(r.Context())
		}
		if uploadedProductPhoto == "" && !mediaReady {
			studioURL := "/studio?mode=affiliate&product_name=" + url.QueryEscape(topic) + "&niche=" + url.QueryEscape(nicheOrGenre)
			s.respondOneCommand(w, r, OneCommandResult{
				OK: false, Type: mode, Title: topic, RedirectURL: studioURL,
				Message: "Cần ảnh sản phẩm hoặc kết nối ImageGen trước khi dựng video Affiliate. Đã điền sẵn thông tin trong Studio.",
			}, http.StatusUnprocessableEntity)
			return
		}
		// Khởi tạo Video Affiliate Lookbook 9:16 chuẩn 5 sao
		p := studio.AffiliateParams{
			Mode:                     studio.AffiliateModePhoto,
			Niche:                    nicheOrGenre,
			ProductName:              topic,
			Seconds:                  30,
			BPM:                      120,
			ProductPhoto:             uploadedProductPhoto,
			AutoGenerateProductPhoto: uploadedProductPhoto == "",
		}
		jobID, err = s.Studio.CreateAffiliateJob(p)
		if err == nil {
			redirectURL = "/studio/jobs?ok=" + url.QueryEscape(fmt.Sprintf("✓ Đang sản xuất video affiliate: %s", topic))
			msg = fmt.Sprintf("Đã bắt đầu dựng video affiliate 9:16: %s", topic)
		}

	default: // "viral"
		// Khởi tạo Video Viral AI 9:16 chuẩn 5 sao
		p := studio.ViralParams{
			Topic:          topic,
			Genre:          nicheOrGenre,
			VisualMood:     "cinematic",
			TargetDuration: 60,
		}
		jobID, err = s.Studio.EnqueueViral(r.Context(), p)
		if err == nil {
			redirectURL = "/viral?ok=" + url.QueryEscape(fmt.Sprintf("✓ Đang sản xuất video viral 9:16: %s", topic))
			msg = fmt.Sprintf("Đã bắt đầu sản xuất video viral 9:16: %s", topic)
		}
	}

	if err != nil {
		s.respondOneCommand(w, r, OneCommandResult{
			OK:      false,
			Message: "Lỗi khởi tạo: " + err.Error(),
		}, http.StatusInternalServerError)
		return
	}

	s.respondOneCommand(w, r, OneCommandResult{
		OK:          true,
		JobID:       jobID,
		Type:        mode,
		Title:       topic,
		RedirectURL: redirectURL,
		Message:     msg,
	}, http.StatusOK)
}

func (s *Server) respondOneCommand(w http.ResponseWriter, r *http.Request, res OneCommandResult, statusCode int) {
	// Nếu gọi qua AJAX hoặc header JSON
	if strings.Contains(r.Header.Get("Accept"), "application/json") || r.FormValue("format") == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	// Nếu submit form thông thường
	if !res.OK && res.RedirectURL != "" {
		http.Redirect(w, r, res.RedirectURL+"&err="+url.QueryEscape(res.Message), http.StatusSeeOther)
		return
	}
	if !res.OK {
		http.Redirect(w, r, "/?err="+url.QueryEscape(res.Message), http.StatusSeeOther)
		return
	}

	target := res.RedirectURL
	if back := r.FormValue("back"); back != "" {
		sep := "?"
		if strings.Contains(back, "?") {
			sep = "&"
		}
		target = back + sep + "ok=" + url.QueryEscape(res.Message)
	}

	http.Redirect(w, r, target, http.StatusSeeOther)
}

func hasWord(s, word string) bool {
	fields := strings.Fields(s)
	for _, f := range fields {
		if strings.Trim(strings.ToLower(f), ",.!?\"'():;") == word {
			return true
		}
	}
	return false
}
