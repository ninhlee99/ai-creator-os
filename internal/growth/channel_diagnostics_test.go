package growth

import (
	"testing"
)

func TestDiagnoseAffiliateVideo(t *testing.T) {
	// 1. Weak Hook / Low Views (< 300)
	d1 := DiagnoseAffiliateVideo(1, "job1", AffiliateMetricsInput{
		Views: 120, Clicks: 2, Orders: 0,
	})
	if d1.FunnelStage != "hook" {
		t.Errorf("low views stage = %s, want hook", d1.FunnelStage)
	}

	// 2. High Views, Low Clicks (CTR < 1%)
	d2 := DiagnoseAffiliateVideo(1, "job2", AffiliateMetricsInput{
		Views: 2000, Clicks: 5, Orders: 1,
	})
	if d2.FunnelStage != "click" {
		t.Errorf("low CTR stage = %s, want click", d2.FunnelStage)
	}

	// 3. High Clicks, Low Orders, Expensive Price for youth (> 350k VND)
	d3 := DiagnoseAffiliateVideo(1, "job3", AffiliateMetricsInput{
		Views: 3000, Clicks: 150, Orders: 1, ProductPrice: 850000, Currency: "VND",
	})
	if d3.FunnelStage != "purchase" {
		t.Errorf("expensive price stage = %s, want purchase", d3.FunnelStage)
	}
	if !containsStr(d3.Diagnosis, "quá cao so với túi tiền") {
		t.Errorf("diagnosis should mention price mismatch with youth: %s", d3.Diagnosis)
	}

	// 4. High Cancellation Rate (> 25%)
	d4 := DiagnoseAffiliateVideo(1, "job4", AffiliateMetricsInput{
		Views: 4000, Clicks: 200, Orders: 10, CanceledOrders: 8, ProductPrice: 200000,
	})
	if d4.FunnelStage != "payout" {
		t.Errorf("high cancel rate stage = %s, want payout", d4.FunnelStage)
	}

	// 5. Healthy conversion
	d5 := DiagnoseAffiliateVideo(1, "job5", AffiliateMetricsInput{
		Views: 5000, Clicks: 150, Orders: 12, CanceledOrders: 1, ProductPrice: 220000,
	})
	if d5.FunnelStage != "healthy" {
		t.Errorf("healthy funnel stage = %s, want healthy", d5.FunnelStage)
	}
}

func TestDiagnoseViralVideo(t *testing.T) {
	// 1. Hook drop-off (< 60% 3s retention)
	d1 := DiagnoseViralVideo(2, "vid1", ViralMetricsInput{
		Views: 1000, ThreeSecRetentionRate: 0.45, CompletionRate: 0.20,
	})
	if d1.FunnelStage != "hook" {
		t.Errorf("weak hook stage = %s, want hook", d1.FunnelStage)
	}

	// 2. Mid-video drop-off (< 35% completion)
	d2 := DiagnoseViralVideo(2, "vid2", ViralMetricsInput{
		Views: 2000, ThreeSecRetentionRate: 0.75, CompletionRate: 0.25,
	})
	if d2.FunnelStage != "retention" {
		t.Errorf("mid-video drop stage = %s, want retention", d2.FunnelStage)
	}

	// 3. Views high but no followers gained
	d3 := DiagnoseViralVideo(2, "vid3", ViralMetricsInput{
		Views: 5000, ThreeSecRetentionRate: 0.80, CompletionRate: 0.50, FollowersGained: 1,
	})
	if d3.FunnelStage != "growth" {
		t.Errorf("low follow conversion stage = %s, want growth", d3.FunnelStage)
	}

	// 4. Healthy virality
	d4 := DiagnoseViralVideo(2, "vid4", ViralMetricsInput{
		Views: 6000, ThreeSecRetentionRate: 0.85, CompletionRate: 0.55, FollowersGained: 30,
	})
	if d4.FunnelStage != "healthy" {
		t.Errorf("healthy viral stage = %s, want healthy", d4.FunnelStage)
	}
}

func TestApplyBoundedAdjustmentAndRollback(t *testing.T) {
	st := testStore(t)
	accountID := int64(10)

	// Test 1: Bounded clamp (+-15%)
	// Requesting +50% should be clamped to +15%
	h1, err := st.ApplyBoundedAdjustment(accountID, "radar.affiliate.max_price", 300000, 450000, "Thử nghiệm tăng trần giá")
	if err != nil {
		t.Fatal(err)
	}
	if h1.ChangePct != 15.0 {
		t.Errorf("h1.ChangePct = %.1f, want 15.0 (clamped)", h1.ChangePct)
	}
	if h1.NewValue != "345000.00" { // 300000 * 1.15 = 345000
		t.Errorf("h1.NewValue = %s, want 345000.00", h1.NewValue)
	}

	// Test 2: List history
	hist, err := st.ListTuningHistory(accountID, 10)
	if err != nil || len(hist) != 1 {
		t.Fatalf("ListTuningHistory = %d, err %v", len(hist), err)
	}

	// Test 3: Rollback
	rb, err := st.RollbackSetting(accountID, h1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !rb.RolledBack {
		t.Error("rb.RolledBack should be true")
	}

	// Cannot rollback again
	_, err = st.RollbackSetting(accountID, h1.ID)
	if err == nil {
		t.Error("second rollback must return error")
	}

	// History now has initial + reverse entry
	hist2, err := st.ListTuningHistory(accountID, 10)
	if err != nil || len(hist2) != 2 {
		t.Fatalf("hist2 = %d (want 2), err %v", len(hist2), err)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && len(sub) > 0 && searchSub(s, sub)))
}

func searchSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
