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
