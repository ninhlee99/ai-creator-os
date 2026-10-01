// Package governance is the Go port of apps/orchestrator/governance.py.
//
// Deterministic rules engine. Pure functions, NO LLM inside.
//
// LLMs propose; this package disposes. Every decision is recorded in the
// ledger by the calling agent.
//
// Semantic differences from the Python original:
//   - ProductEligible takes sellerRating as *float64 instead of float|None.
//   - ShouldKillProduct takes ledger.ProductStats and a sessionsFeatured
//     count. The Go ledger does not expose raw SQL, so callers cannot run
//     the original Python query that counted distinct live sessions whose
//     payload mentions the product title; pass 0 when that signal is
//     unavailable (disables the sessions-based kill trigger).
//   - ShouldScaleProduct takes ledger.ProductStats, which has no cost
//     field. Missing cost is treated as 0, exactly like Python's
//     stats.get("cost", 0.0) — a zero cost yields infinite ROI.
package governance

import (
	"fmt"
	"math"

	"github.com/ninhlee99/ai-creator-os/internal/agents/config"
	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// Verdict is the outcome of a governance check.
type Verdict struct {
	Allowed bool
	Reason  string
}

// CheckKillSwitch blocks everything when the global kill switch is on.
func CheckKillSwitch(cfg config.Config) Verdict {
	if cfg.KillSwitch {
		return Verdict{false, "global kill switch is ON"}
	}
	return Verdict{true, "ok"}
}

// CheckBudget blocks external actions once the daily API budget is spent.
func CheckBudget(cfg config.Config, spentTodayUSD float64) Verdict {
	if spentTodayUSD >= cfg.DailyAPIBudgetUSD {
		return Verdict{false, fmt.Sprintf(
			"daily API budget exhausted (%.2f/%.2f USD)",
			spentTodayUSD, cfg.DailyAPIBudgetUSD)}
	}
	return Verdict{true, "ok"}
}

// CheckLiveSession is called every segment during a live session.
func CheckLiveSession(cfg config.Config, elapsedMin int) Verdict {
	if v := CheckKillSwitch(cfg); !v.Allowed {
		return v
	}
	if elapsedMin >= cfg.MaxLiveMinutesPerSession {
		return Verdict{false, fmt.Sprintf(
			"session reached max duration (%d min) — anti-ban pacing",
			cfg.MaxLiveMinutesPerSession)}
	}
	return Verdict{true, "ok"}
}

// ProductEligible is the hunter gate: a product may enter the shelf only
// if eligible.
func ProductEligible(cfg config.Config, price, commissionRate float64,
	sellerRating *float64, category string,
	blockedCategories []string) Verdict {
	for _, b := range blockedCategories {
		if category == b {
			return Verdict{false, fmt.Sprintf("category blocked: %s", category)}
		}
	}
	if price <= 0 || price > cfg.MaxPrice {
		return Verdict{false, fmt.Sprintf("price out of band: %v", price)}
	}
	if !(commissionRate > 0 && commissionRate <= 1) {
		return Verdict{false, fmt.Sprintf("invalid commission rate: %v", commissionRate)}
	}
	if sellerRating != nil && *sellerRating < cfg.MinSellerRating {
		return Verdict{false, fmt.Sprintf("seller rating too low: %v", *sellerRating)}
	}
	return Verdict{true, "ok"}
}

// HunterScore scores by EXPECTED EARNINGS (not commission % alone):
//
//	score = commission value x conversion / (1 + competition)
func HunterScore(price, commissionRate, conversionRate, competition float64) float64 {
	commissionValue := price * commissionRate
	return commissionValue * conversionRate / (1.0 + math.Max(0, competition))
}

// ShouldKillProduct applies the analyst rule: cut losers fast.
func ShouldKillProduct(cfg config.Config, stats ledger.ProductStats, sessionsFeatured int) Verdict {
	if stats.Orders == 0 && stats.Views >= float64(cfg.KillViewsNoOrder) {
		return Verdict{true, fmt.Sprintf(
			"kill: 0 orders after %.0f views (threshold %d)",
			stats.Views, cfg.KillViewsNoOrder)}
	}
	if stats.Orders == 0 && sessionsFeatured >= cfg.KillSessionsNoOrder {
		return Verdict{true, fmt.Sprintf(
			"kill: 0 orders after %d live sessions", sessionsFeatured)}
	}
	return Verdict{false, "keep: within tolerance"}
}

// ShouldScaleProduct scales winners: enough orders and commission covers
// cost multiple times. Cost is not tracked in the Go ledger, so it is
// treated as 0 (infinite ROI) — same as Python's stats.get("cost", 0.0).
func ShouldScaleProduct(stats ledger.ProductStats, minOrders int, minROI float64) Verdict {
	if stats.Orders < int64(minOrders) {
		return Verdict{false, "not yet: too few orders"}
	}
	roi := math.Inf(1)
	// No cost field in ledger.ProductStats: cost stays 0 -> ROI infinite.
	if roi >= minROI {
		return Verdict{true, fmt.Sprintf(
			"scale: %d orders, ROI %.1fx", stats.Orders, roi)}
	}
	return Verdict{false, fmt.Sprintf("not yet: ROI %.1fx < %.1fx", roi, minROI)}
}

// EvaluateAll is the top-level gate before any external action.
func EvaluateAll(cfg config.Config, spentTodayUSD float64) Verdict {
	for _, v := range []Verdict{
		CheckKillSwitch(cfg),
		CheckBudget(cfg, spentTodayUSD),
	} {
		if !v.Allowed {
			return v
		}
	}
	if cfg.DryRun {
		return Verdict{false, "dry-run mode: external actions disabled"}
	}
	return Verdict{true, "ok"}
}
