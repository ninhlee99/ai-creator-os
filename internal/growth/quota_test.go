package growth

import "testing"

func TestQuotaMath(t *testing.T) {
	if !QuotaCanUpload(0) {
		t.Error("fresh day should allow uploads")
	}
	if !QuotaCanUpload(DailyQuotaUnits - UploadCostUnits) {
		t.Error("exactly one upload of headroom should be allowed")
	}
	if QuotaCanUpload(DailyQuotaUnits - UploadCostUnits + 1) {
		t.Error("one unit over the last upload must block")
	}
	if QuotaCanUpload(DailyQuotaUnits) {
		t.Error("exhausted day must block")
	}
	if got := QuotaUploadsLeft(0); got != 6 {
		t.Errorf("QuotaUploadsLeft(0) = %d, want 6 (10000/1600)", got)
	}
	if got := QuotaUploadsLeft(DailyQuotaUnits); got != 0 {
		t.Errorf("QuotaUploadsLeft(exhausted) = %d, want 0", got)
	}
}

func TestQuotaStore(t *testing.T) {
	st := testStore(t)
	day := "2026-10-02"
	if used, err := st.QuotaUsed(day); err != nil || used != 0 {
		t.Fatalf("fresh day: used=%d err=%v", used, err)
	}
	if err := st.AddQuota(day, UploadCostUnits); err != nil {
		t.Fatal(err)
	}
	if err := st.AddQuota(day, UploadCostUnits); err != nil {
		t.Fatal(err)
	}
	used, err := st.QuotaUsed(day)
	if err != nil || used != 2*UploadCostUnits {
		t.Errorf("used = %d, want %d (err %v)", used, 2*UploadCostUnits, err)
	}
	if used2, _ := st.QuotaUsed("2026-10-03"); used2 != 0 {
		t.Errorf("other day used = %d, want 0", used2)
	}
}
