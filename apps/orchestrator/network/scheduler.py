"""Live slot scheduler: who goes live, when.

Deterministic greedy allocator (no randomness — same inputs, same plan,
fully auditable). Rules are hard constraints, from docs/MODEL.md §6:

- golden hours (ICT): 11:30-13:30, 19:00-23:00
- max concurrent lives (default 2 on M1 Pro 32GB — measure, then tune)
- 1-2 lives/day/account, 60-120 min each, 1 rest day/week/account
- never two same-persona accounts live at the same time
- 15-min stagger between live starts (no mechanical patterns)
- golden hours go to the highest-scoring accounts first
"""
from __future__ import annotations

from dataclasses import dataclass

# minutes since midnight, Asia/Ho_Chi_Minh
GOLDEN_WINDOWS = [(11 * 60 + 30, 13 * 60 + 30), (19 * 60, 23 * 60)]
OFFPEAK_WINDOWS = [(9 * 60, 11 * 60 + 30), (13 * 60 + 30, 17 * 60)]
STAGGER_MIN = 15
GRID_MIN = 30  # candidate start times on a 30-min grid

ELIGIBLE = {"live_ready", "live"}


@dataclass(frozen=True)
class Slot:
    account_id: int
    username: str
    persona: str
    start_min: int
    duration_min: int

    @property
    def end_min(self) -> int:
        return self.start_min + self.duration_min

    def label(self) -> str:
        return (f"{self.username} [{self.persona}] "
                f"{self.start_min // 60:02d}:{self.start_min % 60:02d}"
                f"-{self.end_min // 60:02d}:{self.end_min % 60:02d}")


def _overlaps(a_start: int, a_end: int, b_start: int, b_end: int) -> bool:
    return a_start < b_end and b_start < a_end


def _fits(slot_start: int, duration: int, window: tuple[int, int],
           placed: list[Slot], persona: str,
           max_concurrent: int) -> bool:
    end = slot_start + duration
    if not (window[0] <= slot_start and end <= window[1]):
        return False
    concurrent = 0
    for p in placed:
        if _overlaps(slot_start, end, p.start_min, p.end_min):
            if p.persona == persona:
                return False  # same persona overlap: never
            concurrent += 1
        # stagger: no two starts within STAGGER_MIN
        if abs(p.start_min - slot_start) < STAGGER_MIN:
            return False
    return concurrent < max_concurrent


def build_schedule(accounts: list,
                   scores: dict[int, float] | None = None,
                   weekday: int = 0,
                   max_concurrent: int = 2,
                   lives_per_day: int = 2,
                   duration_min: int = 90) -> list[Slot]:
    """Allocate today's live slots. Pure function — test me hard."""
    scores = scores or {}
    eligible = [a for a in accounts
                if a.status in ELIGIBLE
                and (a.followers or 0) >= 1000
                and a.rest_weekday != weekday]
    # golden hours first to the best performers
    ordered = sorted(eligible,
                     key=lambda a: (-scores.get(a.id, 0.0), a.username))

    placed: list[Slot] = []
    for acct in ordered:
        made = 0
        for window in GOLDEN_WINDOWS + OFFPEAK_WINDOWS:
            if made >= lives_per_day:
                break
            start = window[0]
            while start + duration_min <= window[1] and made < lives_per_day:
                if _fits(start, duration_min, window, placed,
                         acct.persona or "", max_concurrent):
                    placed.append(Slot(acct.id, acct.username,
                                       acct.persona or "", start,
                                       duration_min))
                    made += 1
                    start += duration_min  # same account: no back-to-back
                else:
                    start += GRID_MIN
    return sorted(placed, key=lambda s: s.start_min)


def slots_for_dashboard(slots: list[Slot]) -> list[str]:
    return [s.label() for s in slots]
