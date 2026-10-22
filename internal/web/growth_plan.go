package web

import (
	"fmt"
	"github.com/ninhlee99/ai-creator-os/internal/growth"
	"github.com/ninhlee99/ai-creator-os/internal/network"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func pillarsFor(a *network.Account) []string {
	if a.Persona != "" {
		if p, ok := network.PERSONAS[a.Persona]; ok && len(p.ContentPillars) > 0 {
			return p.ContentPillars
		}
	}
	if a.Niche != "" {
		return []string{a.Niche}
	}
	return nil
}

// killedFormats trả về danh sách format_id đã bị kill (VerdictKilled) của
// tài khoản — plan mới loại các format này (đợt AA).
func (s *Server) killedFormats(accountID int64) []string {
	if s.Growth == nil {
		return nil
	}
	stats, err := s.Growth.ListFormatStats(accountID)
	if err != nil {
		return nil
	}
	var out []string
	for _, st := range stats {
		if st.Verdict == growth.VerdictKilled {
			out = append(out, st.FormatID)
		}
	}
	return out
}

// generatePlanFor creates (or replaces) the account's active 30-day plan
// from its current growth stage. Returns the number of scheduled items.
func (s *Server) generatePlanFor(a *network.Account, rationale string) (int, error) {
	pillars := pillarsFor(a)
	if len(pillars) == 0 {
		return 0, fmt.Errorf("kênh chưa có ngách nội dung hoặc trụ cột do người dùng cấu hình")
	}
	prof, err := s.Growth.EnsureProfile(a.ID, primaryPlatform(a), hasYouTube(a))
	if err != nil {
		return 0, err
	}
	drafts := growth.GeneratePlan(growth.PlanRequest{
		Stage:         prof.Stage,
		HasYouTube:    hasYouTube(a),
		Pillars:       pillars,
		Start:         time.Now(),
		Days:          30,
		KilledFormats: s.killedFormats(a.ID),
	})
	if _, err := s.Growth.InsertPlan(a.ID, "d30", rationale, drafts); err != nil {
		return 0, err
	}
	target := a.Username
	if s.Ledger != nil {
		_ = s.Ledger.Decide("growth_director", "generate_plan", &target,
			fmt.Sprintf("Sinh kế hoạch 30 ngày (giai đoạn %s, %d vị trí)", prof.Stage, len(drafts)),
			map[string]any{"stage": prof.Stage, "items": len(drafts)})
	}
	return len(drafts), nil
}

// autoGeneratePlan is the zero-touch wrapper (Đợt 3): plans appear on
// their own when an account is created or its theme changes — no visit
// to /growth needed. Best-effort: a failure only logs, never blocks the
// operator action that triggered it.
func (s *Server) autoGeneratePlan(a *network.Account, why string) {
	if s.Growth == nil || a == nil {
		return
	}
	if _, err := s.generatePlanFor(a, why); err != nil {
		log.Printf("web: auto plan %s: %v", a.Username, err)
	}
}

func (s *Server) handleGrowthPlan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := s.Mgr.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	base := "/accounts/" + strconv.FormatInt(id, 10)
	if s.Growth == nil {
		seeOther(w, r, base+"?tab=autopilot&err="+url.QueryEscape("Module Phát triển kênh chưa sẵn sàng."))
		return
	}
	n, err := s.generatePlanFor(a, "Sinh từ giao diện")
	if err != nil {
		seeOther(w, r, base+"?tab=autopilot&err="+url.QueryEscape("Không sinh được kế hoạch: "+err.Error()))
		return
	}
	seeOther(w, r, base+"?tab=autopilot&ok="+url.QueryEscape(fmt.Sprintf("Đã sinh kế hoạch 30 ngày: %d vị trí nội dung.", n)))
}

// growthTargetView is one ladder milestone with its live progress.
