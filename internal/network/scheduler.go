package network

import (
	"fmt"
	"sort"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// Live slot scheduler: who goes live, when.
//
// Deterministic greedy allocator (no randomness — same inputs, same plan,
// fully auditable). Hard constraints, from docs/MODEL.md §6:
//
//   - golden hours (ICT): 11:30-13:30, 19:00-23:00
//   - max concurrent lives (default 2)
//   - 1-2 lives/day/account, 60-120 min each, 1 rest day/week/account
//   - never two same-persona accounts live at the same time
//   - 15-min stagger between live starts (no mechanical patterns)
//   - golden hours go to the highest-scoring accounts first

// GoldenWindows and OffpeakWindows are minutes since midnight, Asia/Ho_Chi_Minh.
var GoldenWindows = [][2]int{{11*60 + 30, 13*60 + 30}, {19 * 60, 23 * 60}}
var OffpeakWindows = [][2]int{{9 * 60, 11*60 + 30}, {13*60 + 30, 17 * 60}}

// StaggerMin is the minimum gap between any two live starts.
const StaggerMin = 15

// GridMin is the candidate-start grid step.
const GridMin = 30

var schedEligible = map[string]bool{"live_ready": true, "live": true}

// Slot is one planned live appearance.
type Slot struct {
	AccountID   int64
	Username    string
	Persona     string
	StartMin    int // minutes since midnight ICT
	DurationMin int
}

// EndMin returns the slot end in minutes since midnight.
func (s Slot) EndMin() int { return s.StartMin + s.DurationMin }

// Label renders the slot like "user1 [teacher] 19:00-20:30".
func (s Slot) Label() string {
	return fmt.Sprintf("%s [%s] %02d:%02d-%02d:%02d",
		s.Username, s.Persona,
		s.StartMin/60, s.StartMin%60, s.EndMin()/60, s.EndMin()%60)
}

// LedgerSlot converts to a ledger row for the given date (YYYY-MM-DD).
func (s Slot) LedgerSlot(date string) ledger.Slot {
	return ledger.Slot{
		AccountID:   s.AccountID,
		SlotDate:    date,
		StartMin:    s.StartMin,
		DurationMin: s.DurationMin,
		Status:      "planned",
	}
}

func overlaps(aStart, aEnd, bStart, bEnd int) bool {
	return aStart < bEnd && bStart < aEnd
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func fits(start, duration int, window [2]int, placed []Slot, persona string, maxConcurrent int) bool {
	end := start + duration
	if !(window[0] <= start && end <= window[1]) {
		return false
	}
	concurrent := 0
	for _, p := range placed {
		if overlaps(start, end, p.StartMin, p.EndMin()) {
			if p.Persona == persona {
				return false // same persona overlap: never
			}
			concurrent++
		}
		// stagger: no two starts within StaggerMin
		if absInt(p.StartMin-start) < StaggerMin {
			return false
		}
	}
	return concurrent < maxConcurrent
}

// PlanSlots allocates one day's live slots. Pure function — test me hard.
//
// Eligible: status live_ready/live, >= 1000 followers, and weekday is not the
// account's rest day. Accounts are ordered by score (desc), then username;
// golden windows are filled first.
func PlanSlots(accounts []ledger.Account, scores map[int64]float64, weekday, maxConcurrent, livesPerDay, durationMin int) []Slot {
	var eligible []ledger.Account
	for _, a := range accounts {
		if schedEligible[a.Status] && a.Followers >= 1000 && a.RestWeekday != int64(weekday) {
			eligible = append(eligible, a)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		si, sj := scores[eligible[i].ID], scores[eligible[j].ID]
		if si != sj {
			return si > sj
		}
		return eligible[i].Username < eligible[j].Username
	})

	windows := make([][2]int, 0, len(GoldenWindows)+len(OffpeakWindows))
	windows = append(windows, GoldenWindows...)
	windows = append(windows, OffpeakWindows...)

	var placed []Slot
	for _, acct := range eligible {
		persona := acct.Persona.String
		made := 0
		for _, w := range windows {
			if made >= livesPerDay {
				break
			}
			start := w[0]
			for start+durationMin <= w[1] && made < livesPerDay {
				if fits(start, durationMin, w, placed, persona, maxConcurrent) {
					placed = append(placed, Slot{
						AccountID:   acct.ID,
						Username:    acct.Username,
						Persona:     persona,
						StartMin:    start,
						DurationMin: durationMin,
					})
					made++
					start += durationMin // same account: no back-to-back
				} else {
					start += GridMin
				}
			}
		}
	}
	sort.Slice(placed, func(i, j int) bool { return placed[i].StartMin < placed[j].StartMin })
	return placed
}

// BuildSchedule allocates today's live slots with default tuning
// (max 2 concurrent, 2 lives/day, 90 min each). Deterministic.
func BuildSchedule(accounts []ledger.Account, weekday int) []ledger.Slot {
	planned := PlanSlots(accounts, nil, weekday, 2, 2, 90)
	out := make([]ledger.Slot, 0, len(planned))
	for _, s := range planned {
		out = append(out, s.LedgerSlot(""))
	}
	return out
}

// SlotsForDashboard renders slots as human-readable labels.
func SlotsForDashboard(slots []Slot) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		out = append(out, s.Label())
	}
	return out
}
