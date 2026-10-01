package network

import (
	"database/sql"
	"testing"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

func schedAccount(id int64, username, status, persona string, followers, rest int64) ledger.Account {
	return ledger.Account{
		ID:          id,
		Username:    username,
		Status:      status,
		Persona:     sql.NullString{String: persona, Valid: persona != ""},
		Followers:   followers,
		RestWeekday: rest,
	}
}

func TestBuildScheduleSamePersonaNoOverlap(t *testing.T) {
	weekday := 2
	accounts := []ledger.Account{
		schedAccount(1, "t1", "live_ready", "teacher", 1500, 0),
		schedAccount(2, "t2", "live_ready", "teacher", 1500, 0),
		schedAccount(3, "t3", "live_ready", "teacher", 1500, 0),
		schedAccount(4, "g1", "live_ready", "gamer", 1500, 0),
	}
	slots := BuildSchedule(accounts, weekday)
	if len(slots) == 0 {
		t.Fatal("expected slots to be scheduled")
	}
	personaOf := map[int64]string{}
	for _, a := range accounts {
		personaOf[a.ID] = a.Persona.String
	}
	for i := 0; i < len(slots); i++ {
		for j := i + 1; j < len(slots); j++ {
			si, sj := slots[i], slots[j]
			// same persona must never overlap
			if personaOf[si.AccountID] == personaOf[sj.AccountID] &&
				si.StartMin < sj.StartMin+sj.DurationMin &&
				sj.StartMin < si.StartMin+si.DurationMin {
				t.Errorf("same-persona overlap: %+v vs %+v", si, sj)
			}
			// 15-minute stagger between any two starts
			d := si.StartMin - sj.StartMin
			if d < 0 {
				d = -d
			}
			if d < StaggerMin {
				t.Errorf("stagger violated: starts %d and %d", si.StartMin, sj.StartMin)
			}
		}
		// every slot fits inside a known window with the default 90-min duration
		if slots[i].DurationMin != 90 {
			t.Errorf("duration = %d, want 90", slots[i].DurationMin)
		}
		inWindow := false
		for _, w := range append(append([][2]int{}, GoldenWindows...), OffpeakWindows...) {
			if w[0] <= slots[i].StartMin && slots[i].StartMin+slots[i].DurationMin <= w[1] {
				inWindow = true
			}
		}
		if !inWindow {
			t.Errorf("slot %+v outside all windows", slots[i])
		}
	}
}

func TestBuildScheduleEligibility(t *testing.T) {
	weekday := 2
	accounts := []ledger.Account{
		schedAccount(1, "ok1", "live_ready", "teacher", 1500, 0),
		schedAccount(2, "poor", "live_ready", "gamer", 500, 0),     // < 1000 followers
		schedAccount(3, "rest", "live_ready", "coder", 1500, 2),    // rest day = weekday
		schedAccount(4, "grow", "growing", "storyteller", 9999, 0), // wrong status
	}
	slots := BuildSchedule(accounts, weekday)
	seen := map[int64]bool{}
	for _, s := range slots {
		seen[s.AccountID] = true
	}
	if !seen[1] {
		t.Error("eligible account got no slots")
	}
	for _, id := range []int64{2, 3, 4} {
		if seen[id] {
			t.Errorf("ineligible account %d was scheduled", id)
		}
	}
}

func TestPlanSlotsScoresOrderGoldenFirst(t *testing.T) {
	weekday := 1
	accounts := []ledger.Account{
		schedAccount(1, "low", "live_ready", "teacher", 5000, 0),
		schedAccount(2, "high", "live_ready", "gamer", 5000, 0),
	}
	scores := map[int64]float64{1: 1.0, 2: 99.0}
	slots := PlanSlots(accounts, scores, weekday, 2, 1, 60)
	if len(slots) != 2 {
		t.Fatalf("got %d slots, want 2", len(slots))
	}
	// Highest score goes first, so it claims the earliest golden start.
	if slots[0].AccountID != 2 {
		t.Errorf("first slot went to account %d, want highest-scored account 2", slots[0].AccountID)
	}
	if slots[0].StartMin != GoldenWindows[0][0] {
		t.Errorf("first slot starts at %d, want golden window open %d",
			slots[0].StartMin, GoldenWindows[0][0])
	}
}
