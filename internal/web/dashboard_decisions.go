package web

import (
	"encoding/json"
	"fmt"
	"strings"
)

type dashboardDecisionView struct {
	ID        int64
	Agent     string
	Action    string
	Target    string
	Reason    string
	CreatedAt string
	Evidence  string
	NextStep  string
	NextHref  string
}

func (s *Server) recentDashboardDecisions(limit int) ([]dashboardDecisionView, error) {
	rows, err := s.db.Query(`SELECT id, agent, action, COALESCE(target,''), COALESCE(reason,''),
		COALESCE(inputs_json,''), created_at FROM decisions
		WHERE agent NOT IN ('human','test') ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dashboardDecisionView
	for rows.Next() {
		var d dashboardDecisionView
		var rawInputs string
		if err := rows.Scan(&d.ID, &d.Agent, &d.Action, &d.Target, &d.Reason, &rawInputs, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Evidence = dashboardDecisionEvidence(d.Agent, d.Action, rawInputs)
		d.NextStep, d.NextHref = dashboardDecisionNextStep(d.Agent, d.Action, d.Target)
		out = append(out, d)
	}
	return out, rows.Err()
}

// dashboardDecisionEvidence exposes only reviewed scalar fields from known
// decision payloads. Never render raw inputs_json, which can contain URLs,
// provider errors, and unrelated private data.
func dashboardDecisionEvidence(agent, action, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var inputs map[string]any
	if decoder.Decode(&inputs) != nil {
		return ""
	}
	keys := []struct{ key, label string }{}
	switch {
	case agent == "analyst":
		keys = []struct{ key, label string }{
			{"orders", "đơn"}, {"revenue", "doanh thu"}, {"commission", "hoa hồng"},
			{"views", "views"}, {"sessions_featured", "phiên được giới thiệu"},
		}
	case agent == "growth_director" && action == "generate_plan":
		keys = []struct{ key, label string }{{"stage", "giai đoạn"}, {"items", "mục kế hoạch"}}
	case agent == "growth_analyst":
		keys = []struct{ key, label string }{
			{"format", "format"}, {"video", "video"}, {"videos", "số video"},
			{"days", "số ngày"}, {"views_prev", "views trước"}, {"views_now", "views hiện tại"},
		}
	case agent == "growth_publisher":
		keys = []struct{ key, label string }{
			{"item", "mục Growth"}, {"job_id", "job"}, {"quota_day", "ngày quota"}, {"used", "quota đã dùng"},
		}
	case agent == "system" && action == "commission_reconcile":
		keys = []struct{ key, label string }{{"source_state", "trạng thái nguồn"}}
	default:
		return ""
	}
	var parts []string
	for _, field := range keys {
		value, ok := inputs[field.key]
		if !ok {
			continue
		}
		text, ok := dashboardEvidenceScalar(value)
		if ok {
			parts = append(parts, field.label+": "+text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	source := ""
	switch {
	case agent == "analyst":
		source = "Ledger product stats"
	case agent == "growth_director":
		source = "Growth profile/plan"
	case agent == "growth_analyst":
		source = "Growth snapshots"
	case agent == "growth_publisher":
		source = "Growth + TikTok"
	case agent == "system" && action == "commission_reconcile":
		source = "AccessTrade reconciliation"
	}
	if source != "" {
		parts = append([]string{"nguồn: " + source}, parts...)
	}
	return truncateAttention(strings.Join(parts, " · "))
}

func dashboardEvidenceScalar(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		v = strings.TrimSpace(v)
		return v, v != ""
	case json.Number:
		return v.String(), true
	case bool:
		return fmt.Sprint(v), true
	default:
		return "", false
	}
}

func dashboardDecisionNextStep(agent, action, target string) (string, string) {
	switch {
	case agent == "system" && action == "commission_reconcile":
		return "Mở đối soát Accesstrade", "/products#at-orders"
	case agent == "at_hunter" || agent == "hunter" || agent == "analyst":
		return "Mở danh sách sản phẩm", "/products"
	case strings.HasPrefix(agent, "growth_") || agent == "growth":
		return "Mở Growth", "/growth"
	case agent == "autopilot":
		if target != "" {
			return "Mở job Studio", "/studio/jobs?id=" + target + "#job-" + target
		}
		return "Kiểm tra cài đặt tự động", "/settings/system"
	case agent == "ai_orchestrator":
		return "Mở job Studio", "/studio/jobs"
	default:
		return "", ""
	}
}
