package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LLMClient is the text-generation backend the network package talks to.
// Defined locally so this package never imports internal/engines;
// engines.LLMChain is expected to satisfy it (Complete matches; it must
// additionally expose Name()).
type LLMClient interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
	Name() string
}

// FALLBACK_NICHES maps persona -> default niche when the LLM chain is
// unavailable. Ported verbatim from topics.py.
var FALLBACK_NICHES = map[string]string{
	"creator": "video TikTok theo chủ đề của kênh",
}

// Topic sources. The human's hint is always a SOFT suggestion ("gợi ý mềm"),
// never a final decision: source is "hint", never "user".
const (
	SourceHint     = "hint"
	SourceAuto     = "auto"
	SourceFallback = "fallback"
)

// TopicPlan is the resolved working niche and TikTok video topics for an account.
type TopicPlan struct {
	Niche  string
	Topics []string
	Source string // "hint" | "auto" | "fallback"
	Reason string // short note on how the plan was chosen
}

// parsePlan extracts the JSON plan from an LLM reply, tolerating ``` fences.
func parsePlan(raw string) (niche string, topics []string, ok bool) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		parts := strings.SplitN(text, "```", 3)
		if len(parts) >= 2 {
			text = parts[1]
		} else {
			text = strings.TrimPrefix(text, "```")
		}
		if t := strings.TrimSpace(text); len(t) >= 4 && strings.EqualFold(t[:4], "json") {
			text = strings.TrimSpace(t[4:])
		}
	}
	var data struct {
		Niche  string   `json:"niche"`
		Topics []string `json:"topics"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &data); err != nil {
		return "", nil, false
	}
	niche = strings.TrimSpace(data.Niche)
	if niche == "" {
		return "", nil, false
	}
	for _, t := range data.Topics {
		if s := strings.TrimSpace(t); s != "" {
			topics = append(topics, s)
		}
	}
	if len(topics) > 8 {
		topics = topics[:8]
	}
	return niche, topics, true
}

// buildPrompt renders the (system, prompt) pair for niche resolution,
// honoring the "gợi ý mềm" policy: the hint is a soft suggestion the model
// may refine, narrow, or replace with an adjacent niche.
func buildPrompt(personaID, hint string, taken []string) (system, prompt string) {
	p, ok := PERSONAS[personaID]
	if !ok {
		p = PERSONAS["creator"]
	}
	system = "Bạn là chiến lược gia nội dung TikTok cho thị trường Việt Nam. " +
		"Chỉ trả lời bằng JSON thuần, không giải thích, không markdown ngoài JSON. " +
		"Định dạng: {\"niche\": \"...\", \"topics\": [\"...\", ...]} (5-8 ý tưởng tập)."
	if hint != "" {
		prompt = fmt.Sprintf(
			"Persona: %s — %s.\n"+
				"Người dùng đưa ra gợi ý mềm về hướng chủ đề cho kênh này: \"%s\". "+
				"Đây chỉ là gợi ý mềm, KHÔNG phải quyết định cuối — bạn có toàn quyền "+
				"điều chỉnh, cụ thể hoá, hoặc đề xuất niche lân cận tốt hơn nếu "+
				"gợi ý gốc khó sản xuất/khó kiếm tiền. Hãy chốt 1 niche rõ ràng và "+
				"5-8 ý tưởng tập/video có thể sản xuất hàng ngày bằng AI "+
				"(kịch bản + giọng đọc + hình ảnh). "+
				"Niche phải thân thiện với quảng cáo, phù hợp người Việt 18-35.",
			p.Label, p.Description, hint)
	} else {
		takenTxt := "(chưa có)"
		if len(taken) > 0 {
			takenTxt = strings.Join(taken, ", ")
		}
		prompt = fmt.Sprintf(
			"Nhân vật kênh: %s — %s.\n"+
				"Các niche đã có trong hệ thống (TRÁNH trùng lặp): %s. "+
				"Đề xuất 1 niche MỚI, khác biệt, có thể sản xuất hàng ngày bằng AI, "+
				"thân thiện quảng cáo, phù hợp người Việt 18-35, kèm 5-8 ý tưởng tập. "+
				p.Label, p.Description, takenTxt)
	}
	return system, prompt
}

// ResolveTopic resolves the working niche and TikTok video topics for an account.
//
//   - hint set   -> source="hint": the hint is a SOFT suggestion ("gợi ý mềm");
//     the model may refine, narrow, or replace it with an adjacent niche.
//   - else LLM   -> source="auto", avoiding niches already taken (taken).
//   - LLM nil or unusable -> source="fallback": the persona default niche,
//     or the raw hint when one was given.
func ResolveTopic(ctx context.Context, llm LLMClient, hint, persona string, taken []string) TopicPlan {
	if _, ok := PERSONAS[persona]; !ok {
		persona = "creator"
	}
	h := strings.TrimSpace(hint)
	cleanTaken := make([]string, 0, len(taken))
	for _, n := range taken {
		if s := strings.TrimSpace(n); s != "" {
			cleanTaken = append(cleanTaken, s)
		}
	}

	if llm != nil {
		system, prompt := buildPrompt(persona, h, cleanTaken)
		if text, err := llm.Complete(ctx, system, prompt); err == nil {
			if niche, topics, ok := parsePlan(text); ok && len(topics) > 0 {
				src := SourceAuto
				if h != "" {
					src = SourceHint
				}
				return TopicPlan{Niche: niche, Topics: topics, Source: src,
					Reason: "llm plan accepted"}
			}
		}
	}

	niche, src := h, SourceHint
	if niche == "" {
		niche = FALLBACK_NICHES[persona]
		if niche == "" {
			niche = "nội dung AI"
		}
		src = SourceFallback
	}
	return TopicPlan{Niche: niche, Topics: []string{}, Source: src,
		Reason: "llm unavailable; fell back to hint/persona default"}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
