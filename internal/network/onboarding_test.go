package network

import "testing"

func TestOnboardStepResearchUsesHint(t *testing.T) {
	_, mgr := newTestManager(t)
	a, err := mgr.Add("ob1", "truyện ma đêm khuya", "")
	if err != nil {
		t.Fatal(err)
	}
	// nil LLM -> fallback research; weak hint keeps the hint, source="hint"
	status, err := OnboardStep(mgr, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status != "researching" {
		t.Fatalf("status = %s, want researching", status)
	}
	got, _ := mgr.Get(a.ID)
	if got.Niche != "truyện ma đêm khuya" {
		t.Fatalf("niche = %q, want the hint", got.Niche)
	}
}

func TestOnboardStepKeepsResearchedNiche(t *testing.T) {
	_, mgr := newTestManager(t)
	a, err := mgr.Add("ob2", "gợi ý ban đầu", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OnboardStep(mgr, a.ID); err != nil {
		t.Fatal(err)
	}
	// Simulate the research step having resolved a niche.
	if _, err := mgr.SetTopicPlan(a.ID, "truyện ma đô thị", []string{"t1", "t2"}); err != nil {
		t.Fatal(err)
	}
	status, err := OnboardStep(mgr, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status != "persona_assigned" {
		t.Fatalf("status = %s, want persona_assigned", status)
	}
	got, _ := mgr.Get(a.ID)
	if got.Niche != "truyện ma đô thị" {
		t.Fatalf("niche = %q overwritten; researched niche must never be replaced by the persona label", got.Niche)
	}
	if got.Persona == "" {
		t.Fatal("persona was not assigned")
	}
}

func TestOnboardStepGrowingGate(t *testing.T) {
	l, mgr := newTestManager(t)
	a, err := mgr.Add("ob3", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a = walkChain(t, mgr, a, "researching", "persona_assigned", "growing")

	// Below the follower threshold: stays growing.
	status, err := OnboardStep(mgr, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status != "growing" {
		t.Fatalf("status = %s, want growing", status)
	}
	// At/above threshold: unlocks live.
	if err := l.SetFollowers(a.ID, 1500); err != nil {
		t.Fatal(err)
	}
	status, err = OnboardStep(mgr, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status != "live_ready" {
		t.Fatalf("status = %s, want live_ready", status)
	}
}

func TestOnboardStepDoneIsIdempotent(t *testing.T) {
	_, mgr := newTestManager(t)
	a, err := mgr.Add("ob4", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a = walkChain(t, mgr, a, "researching", "persona_assigned", "growing")
	status, err := OnboardStep(mgr, a.ID) // still growing, no followers
	if err != nil || status != "growing" {
		t.Fatalf("status = %s, err = %v", status, err)
	}
}
