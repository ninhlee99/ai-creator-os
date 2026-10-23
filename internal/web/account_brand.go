package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) handleAccountBrandDNASave(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err, "parse form")
		return
	}
	if s.Growth == nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Hệ thống Growth chưa sẵn sàng")))
		return
	}

	splitLines := func(val string) []string {
		var out []string
		for _, line := range strings.Split(val, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				out = append(out, line)
			}
		}
		return out
	}

	dna := &growth.BrandDNA{
		AccountID:       id,
		Positioning:     strings.TrimSpace(r.FormValue("positioning")),
		AudiencePersona: strings.TrimSpace(r.FormValue("audience_persona")),
		ValuePromise:    strings.TrimSpace(r.FormValue("value_promise")),
		ToneOfVoice:     strings.TrimSpace(r.FormValue("tone_of_voice")),
		HookRules:       splitLines(r.FormValue("hook_rules")),
		ContentPillars:  splitLines(r.FormValue("content_pillars")),
		VisualStyle:     strings.TrimSpace(r.FormValue("visual_style")),
		AudioStyle:      strings.TrimSpace(r.FormValue("audio_style")),
		RedLines:        splitLines(r.FormValue("red_lines")),
		QCChecklist:     splitLines(r.FormValue("qc_checklist")),
	}

	if err := s.Growth.SaveBrandDNA(dna); err != nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Lỗi khi lưu Brand DNA: "+err.Error())))
		return
	}

	seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&ok=%s", id, url.QueryEscape("Đã lưu Bản sắc thương hiệu (Brand DNA) thành công!")))
}

func (s *Server) handleAccountBrandDNAReset(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	acct, err := s.Mgr.Get(id)
	if err != nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Không tìm thấy kênh")))
		return
	}
	if s.Growth == nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Hệ thống Growth chưa sẵn sàng")))
		return
	}

	defaultDNA := growth.DefaultBrandDNA(acct.Niche, id)
	if err := s.Growth.SaveBrandDNA(defaultDNA); err != nil {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&err=%s", id, url.QueryEscape("Không lưu được mẫu Brand DNA: "+err.Error())))
		return
	}

	seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=brand-dna&ok=%s", id, url.QueryEscape("Đã thay bằng mẫu Brand DNA gợi ý theo ngách.")))
}

// ---------------------------------------------------------------- comment reply simulation

func (s *Server) handleAccountCommentSimulate(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	_, err := s.Mgr.Get(id)
	if err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Không tìm thấy kênh"})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Dữ liệu không hợp lệ"})
		return
	}

	userComment := strings.TrimSpace(r.FormValue("comment"))
	if userComment == "" {
		s.writeJSON(w, map[string]any{"ok": false, "error": "Vui lòng nhập nội dung bình luận để thử nghiệm"})
		return
	}

	low := strings.ToLower(userComment)
	intent := "Tương tác cộng đồng"
	intentCode := "engagement"
	action := "Mô phỏng: gợi ý phản hồi tự nhiên"
	delaySec := 45 + (len(userComment)*7)%75

	var reply string
	// This endpoint uses local keyword rules and fixed examples. It does not
	// call an AI provider, read platform comments, or send a reply.

	isPurchase := strings.Contains(low, "giá") || strings.Contains(low, "bao nhiêu") ||
		strings.Contains(low, "mua") || strings.Contains(low, "link") ||
		strings.Contains(low, "shop") || strings.Contains(low, "ship") ||
		strings.Contains(low, "inbox") || strings.Contains(low, "tư vấn") ||
		strings.Contains(low, "đâu") || strings.Contains(low, "chỗ nào")

	isPraise := strings.Contains(low, "hay") || strings.Contains(low, "đẹp") ||
		strings.Contains(low, "đỉnh") || strings.Contains(low, "chuẩn") ||
		strings.Contains(low, "tuyệt") || strings.Contains(low, "thích")

	isToxic := strings.Contains(low, "lừa đảo") || strings.Contains(low, "dở") ||
		strings.Contains(low, "xấu") || strings.Contains(low, "nhảm")

	if isPurchase {
		intent = "Câu hỏi về sản phẩm"
		intentCode = "purchase"
		action = "Mô phỏng: hỏi người vận hành kiểm tra listing"
		reply = "Bạn xem giá và phí vận chuyển đang hiển thị trên trang sản phẩm nhé; mô phỏng này không có dữ liệu listing để xác nhận giá hoặc ưu đãi."
	} else if isPraise {
		intent = "Khen ngợi / Đồng tình"
		intentCode = "praise"
		action = "Mô phỏng: cảm ơn người xem"
		reply = "Cảm ơn bạn đã xem và góp ý nha! 💛"
	} else if isToxic {
		intent = "Tiêu cực / Cần kiểm duyệt (Bảo vệ thương hiệu)"
		intentCode = "toxic"
		action = "Mô phỏng: đề xuất kiểm duyệt"
		reply = "Bình luận cần được người vận hành xem xét."
	} else {
		intent = "Thắc mắc chung"
		intentCode = "inquiry"
		action = "Mô phỏng: xin thêm thông tin"
		reply = "Bạn muốn mình làm rõ điểm nào? Mô phỏng này chưa có ngữ cảnh video để trả lời cụ thể."
	}

	s.writeJSON(w, map[string]any{
		"ok":          true,
		"intent":      intent,
		"intent_code": intentCode,
		"action":      action,
		"reply":       reply,
		"delay_sec":   delaySec,
		"comment":     userComment,
	})
}
