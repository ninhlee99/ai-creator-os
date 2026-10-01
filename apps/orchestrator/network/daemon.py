"""Network daemon: the ON/OFF machine.

One tick() per minute:
  1. master switch OFF (or kill switch) -> stop everything, now.
  2. advance onboarding for accounts not yet live_ready.
  3. (re)build today's slot plan once per day via scheduler.
  4. start streamer for due slots; stop when a slot ends.
  5. run hunter/content/analyst on their cadence, per account niche.

All side effects go through injected callables so rehearsal/tests run
without touching TikTok.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime

from .account import AccountManager
from .onboarding import onboard_step
from .scheduler import build_schedule

LIVE_ELIGIBLE = {"live_ready", "live"}


@dataclass
class NetConfig:
    master_switch: bool = False          # THE on/off button. Default OFF.
    kill_switch: bool = False
    dry_run: bool = True
    timezone: str = "Asia/Ho_Chi_Minh"
    max_concurrent_lives: int = 2
    lives_per_day: int = 2
    live_minutes: int = 90


@dataclass
class NetState:
    last_slot_date: str = ""
    running: dict[int, dict] = field(default_factory=dict)  # slot_id -> info


def _today_ict(now: datetime | None = None) -> tuple[str, int, int]:
    """(date_str, weekday, minutes_since_midnight) in Asia/Ho_Chi_Minh.

    Naive but dependency-free: ICT = UTC+7 always (no DST).
    """
    from datetime import timedelta, timezone
    ict = timezone(timedelta(hours=7))
    t = (now or datetime.now(timezone.utc)).astimezone(ict)
    return t.strftime("%Y-%m-%d"), t.weekday(), t.hour * 60 + t.minute


def tick(state: NetState, cfg: NetConfig, ledger, manager: AccountManager,
         scores: dict[int, float] | None = None,
         start_live=None, stop_live=None,
         run_agent=None, now: datetime | None = None) -> dict:
    """One orchestration tick. Returns a summary of what happened."""
    done: dict = {"stopped_all": False, "onboarded": [], "started": [],
                  "stopped": [], "agents": []}
    if not cfg.master_switch or cfg.kill_switch:
        if state.running and stop_live:
            for slot_id in list(state.running):
                stop_live(state.running.pop(slot_id))
                done["stopped"].append(slot_id)
            done["stopped_all"] = True
        return done

    date_str, weekday, now_min = _today_ict(now)

    # 1. onboarding pipeline
    for acct in manager.list():
        if acct.status in ("onboarding", "researching",
                           "persona_assigned", "growing"):
            new_status = onboard_step(manager, acct.id)
            done["onboarded"].append((acct.username, new_status))

    # 2. daily slot plan
    if state.last_slot_date != date_str:
        accounts = [a for a in manager.list() if a.status in LIVE_ELIGIBLE]
        slots = build_schedule(
            accounts, scores, weekday,
            max_concurrent=cfg.max_concurrent_lives,
            lives_per_day=cfg.lives_per_day,
            duration_min=cfg.live_minutes)
        ledger.save_slots([{
            "account_id": s.account_id, "slot_date": date_str,
            "start_min": s.start_min, "duration_min": s.duration_min,
        } for s in slots])
        ledger.decide("scheduler", "daily_plan", date_str,
                      f"{len(slots)} slots for {len(accounts)} accounts",
                      {"slots": [s.label() for s in slots]})
        state.last_slot_date = date_str

    # 3. start due slots
    if start_live and not cfg.dry_run:
        for row in ledger.due_slots(date_str, now_min):
            if row["id"] in state.running:
                continue
            acct = manager.get(row["account_id"])
            key = acct.rtmp_key()
            if not key:
                ledger.decide("scheduler", "slot_skipped",
                              acct.username, "missing RTMP key",
                              {"slot_id": row["id"]})
                ledger.set_slot_status(row["id"], "skipped")
                continue
            info = start_live(acct, row, key)
            state.running[row["id"]] = info
            ledger.set_slot_status(row["id"], "started")
            manager.transition(acct.id, "live")
            done["started"].append(acct.username)

    # 4. stop finished slots
    for slot_id, info in list(state.running.items()):
        row = ledger.db.execute(
            "SELECT * FROM live_slots WHERE id = ?", (slot_id,)).fetchone()
        if row and now_min >= row["start_min"] + row["duration_min"]:
            if stop_live:
                stop_live(info)
            ledger.set_slot_status(slot_id, "done")
            acct = manager.get(row["account_id"])
            if acct.status == "live":
                manager.transition(acct.id, "live_ready")
            state.running.pop(slot_id)
            done["stopped"].append(slot_id)

    # 5. background agents (hunter/content/analyst cadence handled by caller)
    if run_agent:
        for name in ("hunter", "content", "analyst"):
            done["agents"].append(run_agent(name))

    return done
