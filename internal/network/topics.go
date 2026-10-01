package network

import (
	"context"
	"database/sql"
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
	"storyteller": "kể chuyện đêm khuya",
	"teacher":     "tiếng Anh qua truyện ngắn",
	"gamer":       "game indie chill",
	"coder":       "live code game và mini-app",
	"musician":    "nhạc AI thư giãn",
	"dancer":      "nhảy theo trend remix",
}

// Topic sources. The human's hint is always a SOFT suggestion ("gợi ý mềm"),
// never a final decision: source is "hint", never "user".
const (
	SourceHint     = "hint"
	SourceAuto     = "auto"
	SourceFallback = "fallback"
)

// TopicPlan is the resolved working niche + episode topics for an account.
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
		p = PERSONAS["storyteller"]
	}
	system = "Bạn là chiến lược gia nội dung TikTok/YouTube cho thị trường Việt Nam. " +
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
			p.Label, p.LiveStyle, hint)
	} else {
		takenTxt := "(chưa có)"
		if len(taken) > 0 {
			takenTxt = strings.Join(taken, ", ")
		}
		guard := ""
		if personaID == "teacher" {
			guard = "KHÔNG đóng vai chuyên gia y tế/tài chính/luật. "
		}
		prompt = fmt.Sprintf(
			"Persona: %s — %s.\n"+
				"Các niche đã có trong hệ thống (TRÁNH trùng lặp): %s. "+
				"Đề xuất 1 niche MỚI, khác biệt, có thể sản xuất hàng ngày bằng AI, "+
				"thân thiện quảng cáo, phù hợp người Việt 18-35, kèm 5-8 ý tưởng tập. "+
				"%s",
			p.Label, p.LiveStyle, takenTxt, guard)
	}
	return system, prompt
}

// ResolveTopic resolves the working niche + episode topics for an account.
//
//   - hint set   -> source="hint": the hint is a SOFT suggestion ("gợi ý mềm");
//     the model may refine, narrow, or replace it with an adjacent niche.
//   - else LLM   -> source="auto", avoiding niches already taken (taken).
//   - LLM nil or unusable -> source="fallback": the persona default niche,
//     or the raw hint when one was given.
func ResolveTopic(ctx context.Context, llm LLMClient, hint, persona string, taken []string) TopicPlan {
	if _, ok := PERSONAS[persona]; !ok {
		persona = "storyteller"
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

// recentSessionTopics returns the topics already chosen for the last n live
// sessions of username, newest first (the round-robin avoidance list).
func (m *AccountManager) recentSessionTopics(username string, n int) []string {
	rows, err := m.db.Query(
		`SELECT target FROM decisions WHERE agent = 'live_planner'
		 AND action = 'session_topic' AND target LIKE ? ORDER BY id DESC LIMIT ?`,
		username+":%", n)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var target sql.NullString
		if err := rows.Scan(&target); err != nil {
			continue
		}
		if target.Valid {
			if i := strings.Index(target.String, ":"); i >= 0 {
				out = append(out, target.String[i+1:])
			}
		}
	}
	return out
}

// recentSessionEvidence summarizes the last live sessions for planning
// context: "last sessions [120 peak/90min; ...], 7d gifts $12.34".
func (m *AccountManager) recentSessionEvidence(accountID int64) string {
	rows, err := m.db.Query(
		`SELECT peak_viewers, duration_min FROM live_sessions
		 WHERE account_id = ? ORDER BY id DESC LIMIT 3`, accountID)
	if err != nil {
		return ""
	}
	var parts []string
	for rows.Next() {
		var peak, dur sql.NullInt64
		if err := rows.Scan(&peak, &dur); err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d peak/%dmin", peak.Int64, dur.Int64))
	}
	rows.Close()
	if len(parts) == 0 {
		return ""
	}
	var gifts float64
	_ = m.db.QueryRow(
		`SELECT COALESCE(SUM(usd), 0) FROM gifts WHERE account_id = ?
		 AND date(recorded_at) >= date('now', '-7 days')`, accountID).Scan(&gifts)
	return fmt.Sprintf("last sessions [%s], 7d gifts $%.2f",
		strings.Join(parts, "; "), gifts)
}

// PlanLiveTopic plans the topic for the account's NEXT live session.
//
// It picks the least-recently-used episode topic (round-robin over the
// account's topic plan, avoiding the 32 most recent session topics) and
// records the decision with its evidence (recent session stats). The llm
// parameter is reserved for future evidence summarization and currently
// unused. Returns the topic and the basis string.
func (m *AccountManager) PlanLiveTopic(llm LLMClient, accountID int64) (topic, basis string, err error) {
	_ = llm // reserved for future evidence summarization
	acct, err := m.Get(accountID)
	if err != nil {
		return "", "", err
	}
	topics := acct.Topics
	if len(topics) == 0 {
		// No episode plan yet — fall back to the niche itself.
		topic = firstNonEmpty(acct.Niche, acct.NicheHint, "giao lưu với khán giả")
		basis = "no episode plan; using niche as session theme"
	} else {
		used := m.recentSessionTopics(acct.Username, 32)
		usedSet := make(map[string]bool, len(used))
		for _, u := range used {
			usedSet[u] = true
		}
		topic = topics[0]
		for _, t := range topics {
			if !usedSet[t] {
				topic = t
				break
			}
		}
		basis = fmt.Sprintf("round-robin over %d planned episodes; %d recent sessions avoided",
			len(topics), len(used))
	}
	if ev := m.recentSessionEvidence(accountID); ev != "" {
		basis += "; recent evidence: " + ev
	}
	target := acct.Username + ":" + topic
	if derr := m.ledger.Decide("live_planner", "session_topic", &target,
		"next live session topic — "+basis,
		map[string]any{"account_id": accountID, "topic": topic, "basis": basis}); derr != nil {
		return "", "", derr
	}
	return topic, basis, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
