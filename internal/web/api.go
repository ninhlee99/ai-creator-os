package web

// Legacy JSON API endpoints (/api/stats, /api/products, /api/decisions).
// Kept behind the api.enabled flag; no UI page consumes them.

import (
	"encoding/json"
	"log"
	"math"
	"net/http"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	spend, err := s.Ledger.DailySpendUSD()
	if err != nil {
		s.fail(w, err, "daily spend")
		return
	}
	writeJSON(w, map[string]any{
		"dry_run":          s.Cfg.DryRun(),
		"kill_switch":      s.Cfg.KillSwitch(),
		"total_commission": s.scalarFloat("SELECT COALESCE(SUM(commission),0) FROM orders"),
		"daily_spend_usd":  spend,
	})
}

type productJSON struct {
	ID              int64   `json:"id"`
	PlatformPID     string  `json:"platform_pid"`
	Title           string  `json:"title"`
	Category        string  `json:"category"`
	Price           float64 `json:"price"`
	CommissionRate  float64 `json:"commission_rate"`
	CommissionValue float64 `json:"commission_value"`
	Score           float64 `json:"score"`
	Status          string  `json:"status"`
}

// handleAPIProducts lists the shared product store (best score first).
// R2-W1: one store — it reads what the Kệ hàng tab and the autopilot see.
func (s *Server) handleAPIProducts(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		writeJSON(w, []productJSON{})
		return
	}
	items, err := s.Products.All(50)
	if err != nil {
		s.fail(w, err, "get products")
		return
	}
	out := make([]productJSON, 0, len(items))
	for _, p := range items {
		status := p.ShelfStatus
		if status == "" {
			status = "candidate"
		}
		out = append(out, productJSON{
			ID: p.ID, PlatformPID: p.SourceID, Title: p.Title,
			Category: p.Category, Price: p.Price,
			CommissionRate:  p.CommissionRate,
			CommissionValue: math.Round(p.Price*p.CommissionRate*100) / 100,
			Score:           products.Score(p), Status: status,
		})
	}
	writeJSON(w, out)
}

func (s *Server) handleAPIDecisions(w http.ResponseWriter, r *http.Request) {
	decisions, err := s.queryDecisions(
		"SELECT id, agent, action, target, reason, created_at FROM decisions ORDER BY id DESC LIMIT 100")
	if err != nil {
		s.fail(w, err, "api decisions")
		return
	}
	if decisions == nil {
		decisions = []decisionView{}
	}
	writeJSON(w, decisions)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("web: write json: %v", err)
	}
}
