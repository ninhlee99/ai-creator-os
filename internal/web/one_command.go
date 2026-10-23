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

const oneCommandMaxRequestBytes = 1 << 20

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

	// 1. Kiểm tra ý định KỂ CHUYỆN AUDIO DRAMA 9:16
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

// handleOneCommandCreate starts supported story jobs and routes other intents
// to their dedicated preparation flow.
func (s *Server) handleOneCommandCreate(w http.ResponseWriter, r *http.Request) {
	if s.Studio == nil {
		s.respondOneCommand(w, r, OneCommandResult{
			OK:      false,
			Message: "Studio AI chưa sẵn sàng",
		}, http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, oneCommandMaxRequestBytes)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			s.respondOneCommand(w, r, OneCommandResult{Message: "Form quá lớn hoặc không hợp lệ. Luồng nhanh chỉ nhận mô tả bằng chữ; tải media trong Studio."}, http.StatusBadRequest)
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
	} else if err := r.ParseForm(); err != nil {
		s.respondOneCommand(w, r, OneCommandResult{Message: "Form quá lớn hoặc không hợp lệ. Hãy nhập mô tả ngắn bằng chữ."}, http.StatusBadRequest)
		return
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

	if hasOneCommandProductMedia(r) {
		redirectURL := "/stories"
		if mode == "affiliate" {
			redirectURL = "/studio?mode=affiliate&product_name=" + url.QueryEscape(topic) + "&niche=" + url.QueryEscape(nicheOrGenre)
		} else if mode == "viral" {
			redirectURL = "/viral?topic=" + url.QueryEscape(topic)
		}
		s.respondOneCommand(w, r, OneCommandResult{
			Type: mode, Title: topic, RedirectURL: redirectURL,
			Message: "Luồng nhanh chỉ nhận mô tả bằng chữ; media chưa được lưu. Mở luồng chuyên biệt để thêm đầu vào.",
		}, http.StatusUnprocessableEntity)
		return
	}

	var jobID string
	var err error
	var redirectURL string
	var msg string

	switch mode {
	case "story":
		// Queue a story job using the requested topic.
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
		studioURL := "/studio?mode=affiliate&product_name=" + url.QueryEscape(topic) + "&niche=" + url.QueryEscape(nicheOrGenre)
		s.respondOneCommand(w, r, OneCommandResult{
			OK: false, Type: mode, Title: topic, RedirectURL: studioURL,
			Message: "Luồng nhanh chưa tạo video affiliate tự động. Mở Studio để lập shot plan và hoàn tất bằng footage khả dụng; chưa tạo job.",
		}, http.StatusUnprocessableEntity)
		return

	default: // "viral"
		s.respondOneCommand(w, r, OneCommandResult{
			OK: false, Type: mode, Title: topic, RedirectURL: "/viral?topic=" + url.QueryEscape(topic),
			Message: "Video Viral cần video nguồn và ảnh nhân vật kênh để tái hiện cảnh/chuyển động. Mở Xưởng Viral để cung cấp hai đầu vào.",
		}, http.StatusUnprocessableEntity)
		return
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

func hasOneCommandProductMedia(r *http.Request) bool {
	if r.MultipartForm == nil {
		return false
	}
	return len(r.MultipartForm.File["product_media"]) > 0 || len(r.MultipartForm.File["product_photo"]) > 0
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
