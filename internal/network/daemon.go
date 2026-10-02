package network

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/ledger"
)

// Network daemon: the ON/OFF machine.
//
// One Tick():
//  1. master switch OFF (or kill switch) -> stop everything, now.
//  2. advance onboarding for accounts not yet live_ready.
//  3. (re)build today's slot plan once per day via the scheduler.
//  4. start streamer for due slots; stop when a slot ends.
//  5. run hunter/content/analyst on their cadence, per account niche.
//
// All side effects go through injected callables (OnStartLive, OnStopLive,
// OnRunAgent) so rehearsal/tests run without touching TikTok. When DryRun is
// set, no external action is ever started.

// Gate is the shared, live view of the operator safety switches. When
// set, it is the SINGLE source of truth for kill/dry-run: Tick reads it
// on every pass, so a dashboard toggle stops (or releases) the daemon
// without a restart. The static KillSwitch/DryRun fields below are only
// the fallback for standalone use (unit tests, no dashboard wired).
type Gate interface {
	KillSwitch() bool
	DryRun() bool
}

// NetConfig is the daemon configuration.
type NetConfig struct {
	// MasterSwitch is the startup seed for the master on/off button.
	// Default OFF. The live decision comes from MasterGate when wired
	// (R2-W4, R2-08): the persisted Settings toggle, read fresh every
	// tick — the UI toggle takes effect without a restart.
	MasterSwitch       bool
	MasterGate         MasterGate // live master switch; nil = MasterSwitch field wins
	KillSwitch         bool       // emergency stop (fallback; Gate wins when set)
	DryRun             bool       // when true, due slots are never started (fallback; Gate wins when set)
	Timezone           string     // IANA name; falls back to fixed UTC+7
	MaxConcurrentLives int
	LivesPerDay        int
	LiveMinutes        int

	// Gate, when non-nil, supplies kill/dry-run live from the shared
	// operator state (the web *Config) on every Tick.
	Gate Gate
}

// MasterGate reports the persisted master switch. automation.MasterSwitchGate
// (backed by the settings facade) implements it.
type MasterGate interface {
	MasterEnabled() bool
}

// masterOn resolves the effective master switch: the live gate wins when
// wired, otherwise the startup-seeded field.
func (c NetConfig) masterOn() bool {
	if c.MasterGate != nil {
		return c.MasterGate.MasterEnabled()
	}
	return c.MasterSwitch
}

// DefaultNetConfig returns the safe defaults: everything OFF, dry-run on.
func DefaultNetConfig() NetConfig {
	return NetConfig{
		MasterSwitch:       false,
		KillSwitch:         false,
		DryRun:             true,
		Timezone:           "Asia/Ho_Chi_Minh",
		MaxConcurrentLives: 2,
		LivesPerDay:        2,
		LiveMinutes:        90,
	}
}

// RunningSlot is one live slot currently on air: the ledger row plus the
// opaque handle returned by OnStartLive.
type RunningSlot struct {
	Slot   ledger.Slot
	Handle any
}

// NetState is the daemon's runtime state.
type NetState struct {
	LastSlotDate string
	Running      map[int64]RunningSlot // slot id -> running info
}

// OnboardedResult records one onboarding advance in a tick.
type OnboardedResult struct {
	Username string
	Status   string
}

// TickSummary is what one Tick did.
type TickSummary struct {
	StoppedAll bool
	Onboarded  []OnboardedResult
	Started    []string
	Stopped    []int64
	Agents     []string
}

// Daemon orchestrates the network. Construct with NewDaemon.
type Daemon struct {
	ledger *ledger.Ledger
	mgr    *AccountManager
	llm    LLMClient
	cfg    NetConfig
	state  *NetState

	// Scores ranks accounts for the scheduler (account id -> score).
	Scores map[int64]float64
	// OnStartLive starts the streamer for a due slot; returns an opaque
	// handle later passed to OnStopLive. Nil = never start.
	OnStartLive func(acct *Account, slot ledger.Slot, rtmpKey string) (any, error)
	// OnStopLive stops a running live. Nil = nothing to stop.
	OnStopLive func(handle any)
	// OnRunAgent runs one background agent (hunter/content/analyst) and
	// returns a short result string. Nil = agents skipped.
	OnRunAgent func(name string) string
}

// NewDaemon builds a daemon over an open ledger and account manager.
func NewDaemon(l *ledger.Ledger, mgr *AccountManager, llm LLMClient, cfg NetConfig) *Daemon {
	return &Daemon{
		ledger: l,
		mgr:    mgr,
		llm:    llm,
		cfg:    cfg,
		state:  &NetState{Running: make(map[int64]RunningSlot)},
	}
}

// State exposes the daemon's runtime state (for inspection/tests).
func (d *Daemon) State() *NetState { return d.state }

// todayICT returns (date YYYY-MM-DD, weekday Mon=0..Sun=6, minutes since
// midnight) in the configured timezone. Naive but dependency-free: falls
// back to fixed UTC+7 when the zone cannot be loaded (ICT has no DST).
func todayICT(tz string) (string, int, int) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600)
	}
	t := time.Now().In(loc)
	return t.Format("2006-01-02"), (int(t.Weekday()) + 6) % 7, t.Hour()*60 + t.Minute()
}

// Tick runs one orchestration pass and returns a summary of what happened.
func (d *Daemon) Tick(ctx context.Context) (TickSummary, error) {
	var done TickSummary
	_ = ctx

	// Kill/dry-run come from the shared Gate when wired (one source of
	// truth, read fresh every tick); the static fields are the fallback.
	kill, dry := d.cfg.KillSwitch, d.cfg.DryRun
	if d.cfg.Gate != nil {
		kill, dry = d.cfg.Gate.KillSwitch(), d.cfg.Gate.DryRun()
	}

	if !d.cfg.masterOn() || kill {
		if len(d.state.Running) > 0 && d.OnStopLive != nil {
			for id, rs := range d.state.Running {
				d.OnStopLive(rs.Handle)
				delete(d.state.Running, id)
				done.Stopped = append(done.Stopped, id)
			}
			done.StoppedAll = true
		}
		return done, nil
	}

	dateStr, weekday, nowMin := todayICT(d.cfg.Timezone)

	// 1. onboarding pipeline
	accts, err := d.mgr.List()
	if err != nil {
		return done, err
	}
	for _, a := range accts {
		switch a.Status {
		case "onboarding", "researching", "persona_assigned", "growing":
			ns, err := onboardStep(d.mgr, a.ID, nil, d.llm)
			if err != nil {
				return done, fmt.Errorf("onboard %s: %w", a.Username, err)
			}
			done.Onboarded = append(done.Onboarded, OnboardedResult{Username: a.Username, Status: ns})
		}
	}

	// 2. daily slot plan (once per day)
	if d.state.LastSlotDate != dateStr {
		eligible, err := d.mgr.ledger.ListAccounts([]string{"live_ready", "live"})
		if err != nil {
			return done, err
		}
		slots := PlanSlots(eligible, d.Scores, weekday,
			d.cfg.MaxConcurrentLives, d.cfg.LivesPerDay, d.cfg.LiveMinutes)
		rows := make([]ledger.Slot, 0, len(slots))
		labels := make([]string, 0, len(slots))
		for _, s := range slots {
			rows = append(rows, s.LedgerSlot(dateStr))
			labels = append(labels, s.Label())
		}
		if err := d.ledger.SaveSlots(rows); err != nil {
			return done, err
		}
		target := dateStr
		if err := d.ledger.Decide("scheduler", "daily_plan", &target,
			fmt.Sprintf("%d slots for %d accounts", len(slots), len(eligible)),
			map[string]any{"slots": labels}); err != nil {
			return done, err
		}
		d.state.LastSlotDate = dateStr
	}

	// 3. start due slots — never in dry-run
	if d.OnStartLive != nil && !dry {
		due, err := d.ledger.DueSlots(dateStr, nowMin)
		if err != nil {
			return done, err
		}
		for _, row := range due {
			if _, ok := d.state.Running[row.ID]; ok {
				continue
			}
			acct, err := d.mgr.Get(row.AccountID)
			if err != nil {
				return done, err
			}
			key := acct.RtmpKey()
			if key == "" {
				target := acct.Username
				_ = d.ledger.Decide("scheduler", "slot_skipped", &target,
					"missing RTMP key", map[string]any{"slot_id": row.ID})
				_ = d.ledger.SetSlotStatus(row.ID, "skipped")
				continue
			}
			handle, err := d.OnStartLive(acct, row, key)
			if err != nil {
				return done, fmt.Errorf("start live for %s: %w", acct.Username, err)
			}
			d.state.Running[row.ID] = RunningSlot{Slot: row, Handle: handle}
			if err := d.ledger.SetSlotStatus(row.ID, "started"); err != nil {
				return done, err
			}
			if acct.Status == "live_ready" {
				if _, err := d.mgr.Transition(acct.ID, "live", nil); err != nil {
					return done, err
				}
			}
			done.Started = append(done.Started, acct.Username)
		}
	}

	// 4. stop finished slots
	for id, rs := range d.state.Running {
		if nowMin >= rs.Slot.StartMin+rs.Slot.DurationMin {
			if d.OnStopLive != nil {
				d.OnStopLive(rs.Handle)
			}
			if err := d.ledger.SetSlotStatus(id, "done"); err != nil {
				return done, err
			}
			acct, err := d.mgr.Get(rs.Slot.AccountID)
			if err != nil {
				return done, err
			}
			if acct.Status == "live" {
				if _, err := d.mgr.Transition(acct.ID, "live_ready", nil); err != nil {
					return done, err
				}
			}
			delete(d.state.Running, id)
			done.Stopped = append(done.Stopped, id)
		}
	}

	// 5. background agents (cadence handled by caller via OnRunAgent)
	if d.OnRunAgent != nil {
		for _, name := range []string{"hunter", "content", "analyst"} {
			done.Agents = append(done.Agents, d.OnRunAgent(name))
		}
	}

	return done, nil
}

// Run ticks every interval until ctx is cancelled. Tick errors are logged
// and the loop continues.
func (d *Daemon) Run(ctx context.Context, interval time.Duration) {
	if _, err := d.Tick(ctx); err != nil {
		log.Printf("network: tick: %v", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := d.Tick(ctx); err != nil {
				log.Printf("network: tick: %v", err)
			}
		}
	}
}
