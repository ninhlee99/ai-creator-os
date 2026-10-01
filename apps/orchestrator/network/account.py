"""Account registry + lifecycle.

The RTMP key itself is NEVER stored — only the env var name that holds it
(rtmp_key_ref), resolved at stream time. Keys live in env / macOS Keychain.
"""
from __future__ import annotations

import os
from dataclasses import dataclass

# Allowed status transitions. Anything else is rejected loudly.
TRANSITIONS: dict[str, set[str]] = {
    "onboarding": {"researching", "retired"},
    "researching": {"persona_assigned", "paused", "retired"},
    "persona_assigned": {"growing", "paused", "retired"},
    "growing": {"live_ready", "paused", "retired"},
    "live_ready": {"live", "paused", "retired"},
    "live": {"live_ready", "paused", "penalized", "retired"},
    "paused": {"live_ready", "growing", "retired"},
    "penalized": {"paused", "retired"},  # human decides; never auto-resume
    "retired": set(),
}

LIVE_ELIGIBLE_STATUSES = {"live_ready", "live"}
FOLLOWERS_TO_LIVE = 1000  # TikTok requirement


@dataclass
class Account:
    id: int
    username: str
    status: str
    persona: str | None
    niche: str | None
    niche_hint: str
    followers: int
    rtmp_key_ref: str | None
    rest_weekday: int = 0

    @property
    def live_eligible(self) -> bool:
        return (self.status in LIVE_ELIGIBLE_STATUSES
                and (self.followers or 0) >= FOLLOWERS_TO_LIVE)

    def rtmp_key(self) -> str | None:
        """Resolve the actual key from env at runtime. None if missing."""
        if not self.rtmp_key_ref:
            return None
        return os.environ.get(self.rtmp_key_ref)


class AccountManager:
    def __init__(self, ledger):
        self.ledger = ledger

    # ---- CRUD ----
    def add(self, username: str, niche_hint: str = "",
            rtmp_key_ref: str = "") -> Account:
        account_id = self.ledger.add_account(username, niche_hint,
                                             rtmp_key_ref)
        return self.get(account_id)

    def get(self, account_id: int) -> Account:
        row = self.ledger.get_account(account_id)
        return self._wrap(row)

    def list(self, statuses: tuple | None = None) -> list[Account]:
        return [self._wrap(r) for r in self.ledger.list_accounts(statuses)]

    def active_personas(self) -> list[str]:
        return [a.persona for a in
                self.list() if a.persona and a.status != "retired"]

    @staticmethod
    def _wrap(row) -> Account:
        return Account(
            id=row["id"], username=row["username"], status=row["status"],
            persona=row["persona"], niche=row["niche"],
            niche_hint=row["niche_hint"] or "",
            followers=row["followers"], rtmp_key_ref=row["rtmp_key_ref"],
            rest_weekday=row["rest_weekday"] or 0)

    # ---- lifecycle ----
    def transition(self, account_id: int, to: str, **fields) -> Account:
        acct = self.get(account_id)
        allowed = TRANSITIONS.get(acct.status, set())
        if to not in allowed:
            raise ValueError(
                f"illegal transition {acct.status} -> {to} "
                f"(account {acct.username})")
        self.ledger.set_account_status(account_id, to, **fields)
        self.ledger.decide("orchestrator", "account_transition",
                           acct.username,
                           f"{acct.status} -> {to}", {"to": to, **fields})
        return self.get(account_id)

    def penalize(self, account_id: int, reason: str) -> Account:
        """A penalized account NEVER auto-resumes. Human decides."""
        acct = self.transition(account_id, "penalized")
        self.ledger.decide("governance", "account_penalized",
                           acct.username, reason, {})
        return acct

    def network_pause_similar(self, persona: str, reason: str) -> list[Account]:
        """Guardrail: when one account is penalized, pause every other
        account running the same persona — never evade via other accounts."""
        paused = []
        for acct in self.list():
            if acct.persona == persona and acct.status in ("live", "live_ready"):
                paused.append(self.transition(acct.id, "paused"))
        self.ledger.decide("governance", "network_pause",
                           f"persona={persona}",
                           reason, {"paused": [a.username for a in paused]})
        return paused
