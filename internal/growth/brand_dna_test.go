package growth

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBrandDNADefaultsAndStorage(t *testing.T) {
	tmp := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(tmp, "growth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}

	accountID := int64(101)

	// 1. Initial lookup returns nil
	dna, err := store.GetBrandDNA(accountID)
	if err != nil {
		t.Fatalf("unexpected error getting brand dna: %v", err)
	}
	if dna != nil {
		t.Fatalf("expected nil for non-existent brand dna, got %+v", dna)
	}

	// 2. Generate default for skincare
	defaultDNA := DefaultBrandDNA("mỹ phẩm chăm sóc da", accountID)
	if defaultDNA == nil {
		t.Fatal("expected non-nil default brand dna")
	}
	if len(defaultDNA.HookRules) == 0 {
		t.Error("expected hook rules to be populated")
	}
	if len(defaultDNA.ContentPillars) == 0 {
		t.Error("expected content pillars to be populated")
	}
	if len(defaultDNA.QCChecklist) < 5 {
		t.Errorf("expected at least 5 QC checklist items, got %d", len(defaultDNA.QCChecklist))
	}

	// 3. Save Brand DNA
	if err := store.SaveBrandDNA(defaultDNA); err != nil {
		t.Fatalf("failed to save brand dna: %v", err)
	}

	// 4. Retrieve Brand DNA and verify fields
	saved, err := store.GetBrandDNA(accountID)
	if err != nil {
		t.Fatalf("failed to load saved brand dna: %v", err)
	}
	if saved == nil {
		t.Fatal("expected saved brand dna to be non-nil")
	}
	if saved.Positioning != defaultDNA.Positioning {
		t.Errorf("positioning mismatch: got %q, want %q", saved.Positioning, defaultDNA.Positioning)
	}
	if len(saved.HookRules) != len(defaultDNA.HookRules) {
		t.Errorf("hook rules count mismatch: got %d, want %d", len(saved.HookRules), len(defaultDNA.HookRules))
	}

	// 5. Validation test: check prohibited policy words
	pass, violations := saved.ValidateContentAgainstDNA("Kem trộn trắng da cấp tốc sau 1 đêm", "Cam kết chữa dứt điểm 100%")
	if pass {
		t.Error("expected content with prohibited words to fail validation")
	}
	if len(violations) == 0 {
		t.Error("expected violations to be reported")
	}

	// 6. Validation test: clean content passes
	passClean, violationsClean := saved.ValidateContentAgainstDNA(
		"Tại sao càng dùng serum dưỡng ẩm da càng nổi mụn?",
		"Đây là 3 sai lầm phổ biến khi dùng Niacinamide mà 90% các bạn hay mắc phải.",
	)
	if !passClean {
		t.Errorf("expected clean content to pass, got violations: %v", violationsClean)
	}
}

func TestBrandDNANicheTemplates(t *testing.T) {
	niches := []string{"công nghệ", "gia dụng", "kể chuyện lịch sử", "thời trang tổng hợp"}
	for _, n := range niches {
		dna := DefaultBrandDNA(n, 1)
		if dna.Positioning == "" {
			t.Errorf("expected positioning for niche %q", n)
		}
		if dna.ValuePromise == "" {
			t.Errorf("expected value promise for niche %q", n)
		}
		if len(dna.HookRules) == 0 {
			t.Errorf("expected hook rules for niche %q", n)
		}
	}
}
