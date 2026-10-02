//go:build parked

package web

// Live schedule page: rendering today's slots plus the auto-build that
// constructs the schedule on first view (zero-touch).

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
	"github.com/ninhlee99/ai-creator-os/internal/network"
)

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	s.ensureTodaySchedule()
	today := s.today()
	slots, err := s.Ledger.GetSlots(today)
	if err != nil {
		s.fail(w, err, "get slots")
		return
	}
	eligible := 0
	if accts, err := s.Mgr.List("live_ready", "live"); err == nil {
		eligible = len(accts)
	}
	// Go Sunday=0; Python weekday() Monday=0.
	wd := (int(time.Now().In(s.location()).Weekday()) + 6) % 7
	s.render(w, "schedule", s.ctx(
		"Slots", s.slotViews(slots),
		"Today", today,
		"EligibleCount", eligible,
		"DayName", weekdayNames[wd],
	))
}

// buildTodaySchedule (re)builds today's live slots from every account
// (the allocator itself filters to eligible ones). Shared by the manual
// rebuild button and the zero-touch auto-build.
func (s *Server) buildTodaySchedule() (int, error) {
	today := s.today()
	accts, err := s.Ledger.ListAccounts(nil)
	if err != nil {
		return 0, err
	}
	wd := (int(time.Now().In(s.location()).Weekday()) + 6) % 7
	slots := network.BuildSchedule(accts, wd)
	for i := range slots {
		slots[i].SlotDate = today
		slots[i].Status = "planned"
	}
	if _, err := s.db.Exec("DELETE FROM live_slots WHERE slot_date=?", today); err != nil {
		return 0, err
	}
	if err := s.Ledger.SaveSlots(slots); err != nil {
		return 0, err
	}
	if err := s.Ledger.Decide("scheduler", "build_schedule", &today,
		fmt.Sprintf("%d slots", len(slots)), map[string]any{}); err != nil {
		log.Printf("web: decide build_schedule: %v", err)
	}
	return len(slots), nil
}

// ensureTodaySchedule makes the live schedule build itself (Đợt 3):
// when today has no slots yet and at least one account could take one,
// build once — at most one auto-build per day per process. Opening the
// homepage or the schedule page is enough; no button to remember. The
// daemon still decides what actually goes live under master/kill/dry-run.
func (s *Server) ensureTodaySchedule() {
	today := s.today()
	if slots, err := s.Ledger.GetSlots(today); err != nil || len(slots) > 0 {
		return
	}
	accts, err := s.Ledger.ListAccounts(nil)
	if err != nil {
		return
	}
	eligible := false
	for _, a := range accts {
		if a.Status == "live_ready" || a.Status == "live" {
			eligible = true
			break
		}
	}
	if !eligible {
		return
	}
	s.schedMu.Lock()
	if s.schedBuilt == today {
		s.schedMu.Unlock()
		return
	}
	s.schedBuilt = today
	s.schedMu.Unlock()
	if _, err := s.buildTodaySchedule(); err != nil {
		log.Printf("web: auto build schedule: %v", err)
	}
}

func (s *Server) handleScheduleBuild(w http.ResponseWriter, r *http.Request) {
	if _, err := s.buildTodaySchedule(); err != nil {
		s.fail(w, err, "build schedule")
		return
	}
	seeOther(w, r, "/schedule")
}

// -------------------------------------------------- video chữ động (kinetic)

var weekdayNames = []string{"Thứ 2", "Thứ 3", "Thứ 4", "Thứ 5", "Thứ 6", "Thứ 7", "Chủ nhật"}

// slotView couples a slot with its account's username for display.
type slotView struct {
	ledger.Slot
	AccountName string
}

func (s *Server) slotViews(slots []ledger.Slot) []slotView {
	names := map[int64]string{}
	if accts, err := s.Mgr.List(); err == nil {
		for _, a := range accts {
			names[a.ID] = a.Username
		}
	}
	out := make([]slotView, 0, len(slots))
	for _, sl := range slots {
		name, ok := names[sl.AccountID]
		if !ok {
			name = strconv.FormatInt(sl.AccountID, 10)
		}
		out = append(out, slotView{Slot: sl, AccountName: name})
	}
	return out
}

func (s *Server) handleAccountLiveTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := s.accountID(w, r)
	if !ok {
		return
	}
	errMsg := ""
	if _, _, err := s.Mgr.PlanLiveTopic(s.LLM, id); err != nil {
		errMsg = err.Error()
		log.Printf("web: plan live topic: %v", err)
	}
	s.accountBack(w, r, id, "ket-noi", errMsg)
}
