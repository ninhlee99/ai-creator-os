//go:build parked

package governance

import (
	"math"
	"strings"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// testConfig returns a dry-run-off config so governance tests exercise the
// rules themselves rather than env state.
func testConfig() Config {
	return Config{
		DryRun:                   false,
		DailyAPIBudgetUSD:        5.0,
		MaxLiveMinutesPerSession: 120,
		MinSellerRating:          4.0,
		MaxPrice:                 1_000_000,
		KillViewsNoOrder:         10_000,
		KillSessionsNoOrder:      3,
	}
}

func fptr(v float64) *float64 { return &v }

func TestCheckKillSwitch(t *testing.T) {
	c := testConfig()
	if v := CheckKillSwitch(c); !v.Allowed {
		t.Errorf("expected allowed, got %q", v.Reason)
	}
	c.KillSwitch = true
	if v := CheckKillSwitch(c); v.Allowed || !strings.Contains(v.Reason, "kill switch") {
		t.Errorf("expected kill-switch block, got %+v", v)
	}
}

func TestCheckBudget(t *testing.T) {
	c := testConfig()
	c.DailyAPIBudgetUSD = 5.0
	if v := CheckBudget(c, 4.99); !v.Allowed {
		t.Errorf("expected allowed, got %q", v.Reason)
	}
	if v := CheckBudget(c, 5.0); v.Allowed {
		t.Error("spent == budget must block")
	}
	if v := CheckBudget(c, 10); v.Allowed || !strings.Contains(v.Reason, "budget exhausted") {
		t.Errorf("expected budget block, got %+v", v)
	}
}

func TestCheckLiveSession(t *testing.T) {
	c := testConfig()
	c.MaxLiveMinutesPerSession = 120
	if v := CheckLiveSession(c, 119); !v.Allowed {
		t.Errorf("expected allowed, got %q", v.Reason)
	}
	if v := CheckLiveSession(c, 120); v.Allowed {
		t.Error("elapsed == max must block")
	}
	c.KillSwitch = true
	if v := CheckLiveSession(c, 0); v.Allowed || v.Reason != "global kill switch is ON" {
		t.Errorf("kill switch must propagate, got %+v", v)
	}
}

func TestProductEligible(t *testing.T) {
	c := testConfig()
	blocked := []string{"thuốc"}

	cases := []struct {
		name       string
		price      float64
		rate       float64
		rating     *float64
		category   string
		wantOK     bool
		wantReason string
	}{
		{"ok", 250000, 0.12, fptr(4.5), "gia dụng", true, ""},
		{"blocked category", 250000, 0.12, fptr(4.5), "thuốc", false, "category blocked"},
		{"price zero", 0, 0.12, fptr(4.5), "gia dụng", false, "price out of band"},
		{"price too high", 2_000_000, 0.12, fptr(4.5), "gia dụng", false, "price out of band"},
		{"bad rate", 250000, 1.5, fptr(4.5), "gia dụng", false, "invalid commission rate"},
		{"low rating", 250000, 0.12, fptr(3.0), "gia dụng", false, "seller rating too low"},
		{"nil rating ok", 250000, 0.12, nil, "gia dụng", true, ""},
	}
	for _, tc := range cases {
		v := ProductEligible(c, tc.price, tc.rate, tc.rating, tc.category, blocked)
		if v.Allowed != tc.wantOK {
			t.Errorf("%s: allowed=%v, want %v (%q)", tc.name, v.Allowed, tc.wantOK, v.Reason)
		}
		if !tc.wantOK && !strings.Contains(v.Reason, tc.wantReason) {
			t.Errorf("%s: reason %q missing %q", tc.name, v.Reason, tc.wantReason)
		}
	}
}

func TestHunterScore(t *testing.T) {
	// score = price*rate*conv / (1+competition)
	got := HunterScore(100_000, 0.10, 0.02, 1.0)
	want := 100_000 * 0.10 * 0.02 / 2.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("score = %v, want %v", got, want)
	}
	// negative competition is clamped to 0
	gotNeg := HunterScore(100_000, 0.10, 0.02, -5)
	if math.Abs(gotNeg-200) > 1e-9 {
		t.Errorf("negative competition: score = %v, want 200", gotNeg)
	}
	// expected earnings beat raw commission %: low-rate/high-price wins
	lowRate := HunterScore(500_000, 0.05, 0.02, 1.0)
	highRate := HunterScore(50_000, 0.30, 0.02, 1.0)
	if lowRate <= highRate {
		t.Errorf("expected earnings not prioritized: %v vs %v", lowRate, highRate)
	}
}

func TestShouldKillProduct(t *testing.T) {
	c := testConfig()
	c.KillViewsNoOrder = 10_000
	c.KillSessionsNoOrder = 3

	// keep: within tolerance
	v := ShouldKillProduct(c, ledger.ProductStats{Views: 9999}, 0)
	if v.Allowed {
		t.Errorf("expected keep, got %q", v.Reason)
	}
	// kill: views threshold, zero orders
	v = ShouldKillProduct(c, ledger.ProductStats{Views: 10_000}, 0)
	if !v.Allowed || !strings.Contains(v.Reason, "0 orders after") {
		t.Errorf("expected views kill, got %+v", v)
	}
	// kill: sessions trigger
	v = ShouldKillProduct(c, ledger.ProductStats{}, 3)
	if !v.Allowed || !strings.Contains(v.Reason, "live sessions") {
		t.Errorf("expected sessions kill, got %+v", v)
	}
	// keep: has orders despite views
	v = ShouldKillProduct(c, ledger.ProductStats{Orders: 1, Views: 50_000}, 5)
	if v.Allowed {
		t.Errorf("expected keep with orders, got %q", v.Reason)
	}
}

func TestShouldScaleProduct(t *testing.T) {
	// too few orders
	v := ShouldScaleProduct(ledger.ProductStats{Orders: 4, Commission: 100}, 5, 2.0)
	if v.Allowed {
		t.Errorf("expected no-scale, got %q", v.Reason)
	}
	// enough orders, no cost tracked -> infinite ROI -> scale
	v = ShouldScaleProduct(ledger.ProductStats{Orders: 5, Commission: 100}, 5, 2.0)
	if !v.Allowed || !strings.Contains(v.Reason, "scale:") {
		t.Errorf("expected scale, got %+v", v)
	}
}

func TestEvaluateAll(t *testing.T) {
	c := testConfig()
	c.DryRun = false
	if v := EvaluateAll(c, 0); !v.Allowed {
		t.Errorf("expected ok, got %q", v.Reason)
	}
	c.KillSwitch = true
	if v := EvaluateAll(c, 0); v.Allowed || !strings.Contains(v.Reason, "kill switch") {
		t.Errorf("expected kill-switch block, got %+v", v)
	}
	c = testConfig()
	c.DryRun = false
	c.DailyAPIBudgetUSD = 1
	if v := EvaluateAll(c, 1); v.Allowed {
		t.Error("expected budget block")
	}
	c = testConfig()
	c.DryRun = true
	if v := EvaluateAll(c, 0); v.Allowed || !strings.Contains(v.Reason, "dry-run") {
		t.Errorf("expected dry-run block, got %+v", v)
	}
}
