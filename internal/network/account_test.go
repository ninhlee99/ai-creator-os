package network

import (
	"database/sql"
	"errors"
	"testing"
)

func TestTransitionValidChain(t *testing.T) {
	_, mgr := newTestManager(t)
	a, err := mgr.Add("chain1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{"researching", "persona_assigned", "growing", "paused", "growing"} {
		a, err = mgr.Transition(a.ID, to, nil)
		if err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
		if a.Status != to {
			t.Fatalf("status = %s, want %s", a.Status, to)
		}
	}
}

func TestTransitionIllegal(t *testing.T) {
	_, mgr := newTestManager(t)
	a, err := mgr.Add("chain2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a = walkChain(t, mgr, a, "researching")

	// researching -> growing must pass persona assignment first.
	if _, err := mgr.Transition(a.ID, "growing", nil); err == nil {
		t.Fatal("expected error for illegal transition researching -> growing")
	}

	// penalized is terminal except paused/retired (human decides)
	a = walkChain(t, mgr, a, "persona_assigned", "growing")
	a, err = mgr.Penalize(a.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "penalized" {
		t.Fatalf("status = %s, want penalized", a.Status)
	}
	if _, err := mgr.Transition(a.ID, "growing", nil); err == nil {
		t.Fatal("expected error: penalized must never auto-resume")
	}
}

func TestTransitionMissingAccount(t *testing.T) {
	_, mgr := newTestManager(t)
	if _, err := mgr.Transition(9999, "growing", nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestPenalizeCountsAndPausesSiblings(t *testing.T) {
	_, mgr := newTestManager(t)

	mk := func(username string) *Account {
		a, err := mgr.Add(username, "", "")
		if err != nil {
			t.Fatal(err)
		}
		a = walkChain(t, mgr, a, "researching")
		var err2 error
		a, err2 = mgr.Transition(a.ID, "persona_assigned", map[string]any{"persona": "teacher"})
		if err2 != nil {
			t.Fatal(err2)
		}
		return walkChain(t, mgr, a, "growing")
	}
	bad := mk("bad1")
	sibling := mk("good1")

	if _, err := mgr.Penalize(bad.ID, "spam violation"); err != nil {
		t.Fatal(err)
	}

	got, _ := mgr.Get(bad.ID)
	if got.Status != "penalized" {
		t.Fatalf("bad account status = %s, want penalized", got.Status)
	}
	var count int64
	if err := mgr.db.QueryRow(
		"SELECT violation_count FROM accounts WHERE id = ?", bad.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("violation_count = %d, want 1", count)
	}

	// active sibling on same persona must be paused
	sib, _ := mgr.Get(sibling.ID)
	if sib.Status != "paused" {
		t.Fatalf("sibling status = %s, want paused", sib.Status)
	}
}

func TestAddDuplicateUsername(t *testing.T) {
	_, mgr := newTestManager(t)
	a, err := mgr.Add("dup1", "hint", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := mgr.Add("dup1", "other hint", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("duplicate username returned different ids %d vs %d", a.ID, b.ID)
	}
}
