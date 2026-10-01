"""Tests for the AI Creator Network: persona, scheduler, account, onboarding."""
import sys
sys.path.insert(0, ".")

from datetime import datetime, timezone

from apps.orchestrator.network.account import Account, AccountManager
from apps.orchestrator.network.daemon import NetConfig, NetState, tick
from apps.orchestrator.network.onboarding import onboard_step
from apps.orchestrator.network.persona import (ASSIGNABLE, PERSONAS,
                                               assign_persona)
from apps.orchestrator.network.scheduler import (GOLDEN_WINDOWS,
                                                 build_schedule)
from ledger.store import Ledger


def make_ledger():
    return Ledger(":memory:")


def mk_account(i, persona=None, status="live_ready", followers=5000,
               rest_weekday=6):
    return Account(id=i, username=f"acct{i}", status=status,
                   persona=persona, niche=None, niche_hint="",
                   followers=followers, rtmp_key_ref=None,
                   rest_weekday=rest_weekday)


# ---------- persona ----------

def test_persona_keyword_match():
    assert assign_persona("kênh kể chuyện đêm khuya") == "storyteller"
    assert assign_persona("dạy tiếng Anh") == "teacher"
    assert assign_persona("game mobile") == "gamer"


def test_persona_diversity_no_duplicates():
    first = assign_persona("", [])
    second = assign_persona("", [first])
    assert second != first
    assert second in ASSIGNABLE


def test_persona_reuse_only_when_all_used():
    all_used = list(ASSIGNABLE)
    again = assign_persona("", all_used)
    assert again in ASSIGNABLE  # least-used reuse is allowed


def test_teacher_red_lines_present():
    red = PERSONAS["teacher"]["red_lines"]
    assert any("y tế" in r for r in red)


# ---------- scheduler ----------

def test_scheduler_golden_priority_by_score():
    accs = [mk_account(1, "storyteller"), mk_account(2, "teacher")]
    slots = build_schedule(accs, scores={1: 10.0, 2: 1.0}, weekday=0,
                           max_concurrent=1, lives_per_day=1)
    # best scorer gets the earliest golden window
    first = min(slots, key=lambda s: s.start_min)
    assert first.account_id == 1
    assert GOLDEN_WINDOWS[0][0] <= first.start_min < GOLDEN_WINDOWS[0][1]


def test_scheduler_max_concurrent():
    accs = [mk_account(i, p) for i, p in
            enumerate(["storyteller", "teacher", "gamer"], start=1)]
    slots = build_schedule(accs, weekday=0, max_concurrent=1,
                           lives_per_day=2, duration_min=60)
    for i, a in enumerate(slots):
        for b in slots[i + 1:]:
            overlap = a.start_min < b.end_min and b.start_min < a.end_min
            assert not overlap, f"{a.label()} overlaps {b.label()}"


def test_scheduler_no_same_persona_overlap():
    accs = [mk_account(1, "storyteller"), mk_account(2, "storyteller")]
    slots = build_schedule(accs, weekday=0, max_concurrent=2,
                           lives_per_day=2, duration_min=60)
    for i, a in enumerate(slots):
        for b in slots[i + 1:]:
            if a.persona == b.persona:
                overlap = (a.start_min < b.end_min
                           and b.start_min < a.end_min)
                assert not overlap


def test_scheduler_rest_day_and_followers():
    resting = mk_account(1, "storyteller", rest_weekday=0)
    small = mk_account(2, "teacher", followers=500)
    ok = mk_account(3, "gamer")
    slots = build_schedule([resting, small, ok], weekday=0,
                           max_concurrent=2, lives_per_day=1)
    ids = {s.account_id for s in slots}
    assert 1 not in ids and 2 not in ids and 3 in ids


def test_scheduler_stagger():
    accs = [mk_account(1, "storyteller"), mk_account(2, "teacher")]
    slots = build_schedule(accs, weekday=0, max_concurrent=2,
                           lives_per_day=2, duration_min=60)
    starts = sorted(s.start_min for s in slots)
    for a, b in zip(starts, starts[1:]):
        assert b - a >= 15


# ---------- account lifecycle ----------

def test_account_transition_legal_and_illegal():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("test1")
    assert acct.status == "onboarding"
    acct = mgr.transition(acct.id, "researching")
    assert acct.status == "researching"
    try:
        mgr.transition(acct.id, "live")
        raise AssertionError("should have raised")
    except ValueError:
        pass


def test_penalize_never_auto_resumes():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("test2")
    for s in ("researching", "persona_assigned", "growing", "live_ready",
              "live"):
        acct = mgr.transition(acct.id, s)
    mgr.penalize(acct.id, "spam strike")
    acct = mgr.get(acct.id)
    assert acct.status == "penalized"
    try:
        mgr.transition(acct.id, "live_ready")
        raise AssertionError("penalized must not auto-resume")
    except ValueError:
        pass


def test_network_pause_similar_persona():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    a1 = mgr.add("n1")
    a2 = mgr.add("n2")
    for a in (a1, a2):
        for s in ("researching", "persona_assigned", "growing",
                  "live_ready"):
            mgr.transition(a.id, s)
        ledger.set_account_status(a.id, "live", persona="storyteller")
    paused = mgr.network_pause_similar("storyteller", "strike on sibling")
    assert len(paused) == 2
    assert all(mgr.get(a.id).status == "paused" for a in (a1, a2))


# ---------- onboarding ----------

def test_onboarding_pipeline():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("story_chan", niche_hint="kể chuyện đêm khuya")
    assert onboard_step(mgr, acct.id,
                        research_fn=lambda h: {"hint": h}) == "researching"
    assert onboard_step(mgr, acct.id) == "persona_assigned"
    acct = mgr.get(acct.id)
    assert acct.persona == "storyteller"
    assert onboard_step(mgr, acct.id) == "growing"
    assert onboard_step(mgr, acct.id) == "growing"  # not enough followers
    ledger.set_followers(acct.id, 1500)
    assert onboard_step(mgr, acct.id) == "live_ready"


# ---------- daemon ----------

def _ict(year, month, day, hour, minute=0):
    from datetime import timedelta
    ict = timezone(timedelta(hours=7))
    return datetime(year, month, day, hour, minute, tzinfo=ict)


def test_daemon_master_off_stops_all():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    state = NetState(running={99: {"x": 1}})
    stopped = []
    out = tick(state, NetConfig(master_switch=False), ledger, mgr,
               stop_live=lambda info: stopped.append(info))
    assert out["stopped_all"] and stopped == [{"x": 1}]
    assert state.running == {}


def test_daemon_dry_run_never_starts_live():
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("live1", rtmp_key_ref="K1")
    for s in ("researching", "persona_assigned", "growing", "live_ready"):
        mgr.transition(acct.id, s, **(
            {"persona": "teacher"} if s == "persona_assigned" else {}))
    ledger.set_followers(acct.id, 2000)
    ledger.db.execute("UPDATE accounts SET rest_weekday = 6 WHERE id = ?",
                      (acct.id,))
    ledger.db.commit()
    state = NetState()
    started = []
    out = tick(state, NetConfig(master_switch=True, dry_run=True),
               ledger, mgr, start_live=lambda a, r, k: started.append(a),
               now=_ict(2026, 10, 5, 10, 0))  # Monday 10:00 ICT
    assert started == []  # dry run: plan saved, nothing started
    rows = ledger.db.execute("SELECT COUNT(*) c FROM live_slots").fetchone()
    assert rows["c"] > 0
    assert out["started"] == []


def test_daemon_starts_due_slot_when_live():
    import os
    os.environ["K2"] = "fake-key"
    ledger = make_ledger()
    mgr = AccountManager(ledger)
    acct = mgr.add("live2", rtmp_key_ref="K2")
    for s in ("researching", "persona_assigned", "growing", "live_ready"):
        mgr.transition(acct.id, s, **(
            {"persona": "gamer"} if s == "persona_assigned" else {}))
    ledger.set_followers(acct.id, 2000)
    ledger.db.execute("UPDATE accounts SET rest_weekday = 6 WHERE id = ?",
                      (acct.id,))
    ledger.db.commit()
    state = NetState()
    started = []
    # Monday 2026-10-05 12:00 ICT: inside golden window 11:30-13:30
    out = tick(state, NetConfig(master_switch=True, dry_run=False),
               ledger, mgr,
               start_live=lambda a, r, k: started.append((a.username, k)),
               stop_live=lambda info: None,
               now=_ict(2026, 10, 5, 12, 0))
    assert started and started[0][1] == "fake-key"
    assert mgr.get(acct.id).status == "live"
    del os.environ["K2"]
