package growth

import "testing"

func TestTokensOfFoldsDiacritics(t *testing.T) {
	toks := TokensOf("Truyện Ma Hay Nhất!")
	for _, want := range []string{"truyen", "ma", "hay", "nhat"} {
		if !toks[want] {
			t.Errorf("TokensOf missing %q in %v", want, toks)
		}
	}
	if toks["nhất"] {
		t.Errorf("diacritics not folded: %v", toks)
	}
}

func TestConceptOfStripsEpisodeSuffix(t *testing.T) {
	cases := []struct{ format, topic, want string }{
		{"truyen_ma", "truyện ma — tập 3", "truyen_ma truyện ma"},
		{"truyen_ma", "truyện ma — bản dài tập 2", "truyen_ma truyện ma"},
		{"truyen_ma", "truyện ma — bản dài", "truyen_ma truyện ma"},
		{"ke_chuyen", "kể chuyện đời thường", "ke_chuyen kể chuyện đời thường"},
	}
	for _, c := range cases {
		if got := ConceptOf(c.format, c.topic); got != c.want {
			t.Errorf("ConceptOf(%q, %q) = %q, want %q", c.format, c.topic, got, c.want)
		}
	}
	// Two episodes of one series share the concept.
	a := ConceptHash(ConceptOf("truyen_ma", "truyện ma — tập 1"))
	b := ConceptHash(ConceptOf("truyen_ma", "truyện ma — tập 9"))
	if a != b {
		t.Errorf("series episodes should share a concept hash: %s != %s", a, b)
	}
}

func TestConceptHashDeterministicAndSensitive(t *testing.T) {
	h1 := ConceptHash("truyen_ma truyện ma")
	h2 := ConceptHash("truyện ma truyen_ma") // same tokens, other order/case
	if h1 != h2 {
		t.Errorf("hash not token-set based: %s != %s", h1, h2)
	}
	if h3 := ConceptHash("meo_vat mẹo vặt nhà bếp"); h3 == h1 {
		t.Errorf("different concepts collided: %s", h3)
	}
}

func TestTokenSimilarity(t *testing.T) {
	a := TokensOf("truyen ma dem khuya")
	if s := TokenSimilarity(a, a); s != 1 {
		t.Errorf("self similarity = %v, want 1", s)
	}
	if s := TokenSimilarity(a, TokensOf("nau an sang")); s != 0 {
		t.Errorf("disjoint similarity = %v, want 0", s)
	}
	if s := TokenSimilarity(nil, a); s != 0 {
		t.Errorf("empty similarity = %v, want 0", s)
	}
	// 3 tokens shared, 5 in union -> 0.6.
	b := TokensOf("truyen ma dem nhac")
	if s := TokenSimilarity(a, b); s < 0.59 || s > 0.61 {
		t.Errorf("partial similarity = %v, want ~0.6", s)
	}
}

func TestDecideDedup(t *testing.T) {
	cand := TokensOf(ConceptOf("truyen_ma", "truyện ma"))
	hit, sim := DecideDedup(cand, []map[string]bool{
		TokensOf(ConceptOf("truyen_ma", "truyện ma — tập 4")),
	})
	if !hit || sim < 0.99 {
		t.Errorf("identical concept: hit=%v sim=%v, want collision ~1", hit, sim)
	}
	hit, _ = DecideDedup(cand, []map[string]bool{
		TokensOf(ConceptOf("nau_an", "mẹo nấu ăn sáng")),
	})
	if hit {
		t.Errorf("unrelated concept flagged as collision")
	}
	hit, _ = DecideDedup(cand, nil)
	if hit {
		t.Errorf("empty recents flagged as collision")
	}
}

func TestApplyAngleChangesConcept(t *testing.T) {
	base := ConceptOf("truyen_ma", "truyện ma — tập 2")
	angled := ApplyAngle("truyện ma — tập 2", AngleFor(5, 0))
	if angled == "truyện ma — tập 2" {
		t.Fatalf("ApplyAngle did not change the topic")
	}
	sim := TokenSimilarity(TokensOf(base), TokensOf(ConceptOf("truyen_ma", angled)))
	if sim >= DedupSimilarityThreshold {
		t.Errorf("angled concept still too similar: %.2f", sim)
	}
	// Deterministic per (item, attempt); attempts move through angles.
	if AngleFor(5, 0) != AngleFor(5, 0) {
		t.Error("AngleFor not deterministic")
	}
	if AngleFor(5, 0) == AngleFor(5, 1) {
		t.Error("AngleFor should advance with attempts")
	}
}

func TestSameLineExemptions(t *testing.T) {
	base := PlanItem{ID: 1, AccountID: 7, FormatID: "truyen_ma", VariantGroup: "2026-10-02|truyen_ma", Variant: VariantTikTok}
	sibling := PlanItem{ID: 2, AccountID: 7, FormatID: "truyen_ma", VariantGroup: "2026-10-02|truyen_ma", Variant: VariantShorts}
	if !SameLine(base, sibling) {
		t.Error("variant siblings should be same-line")
	}
	series := PlanItem{ID: 3, AccountID: 7, FormatID: "truyen_ma", VariantGroup: "2026-10-09|truyen_ma", Variant: VariantTikTok}
	if !SameLine(base, series) {
		t.Error("same account+format series should be same-line")
	}
	otherAccount := PlanItem{ID: 4, AccountID: 8, FormatID: "truyen_ma", VariantGroup: "2026-10-02|truyen_ma", Variant: VariantTikTok}
	if SameLine(base, otherAccount) {
		t.Error("another account's identical concept must NOT be exempt")
	}
	otherFormat := PlanItem{ID: 5, AccountID: 7, FormatID: "nau_an", VariantGroup: "2026-10-02|nau_an", Variant: VariantTikTok}
	if SameLine(base, otherFormat) {
		t.Error("different format family must NOT be exempt")
	}
}
