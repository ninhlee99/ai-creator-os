"""Onboarding pipeline: new account -> live_ready, fully automatic.

  onboarding -> researching -> persona_assigned -> growing -> live_ready

The human only does step 0: add_account(username, rtmp_key_ref, niche_hint).
Everything after is the pipeline. The research step is injected
(research_fn) so tests and rehearsal can use a mock.
"""
from __future__ import annotations

from .account import AccountManager
from .persona import assign_persona, describe

FOLLOWERS_TO_LIVE = 1000


def default_research(niche_hint: str) -> dict:
    """Placeholder: replaced by the real trend/hashtag researcher.
    Returns a brief the persona engine can use."""
    return {
        "niche_hint": niche_hint,
        "trending_hashtags": [],
        "competitor_notes": "",
        "affiliate_categories": [],
        "source": "placeholder",
    }


def onboard_step(manager: AccountManager, account_id: int,
                 research_fn=default_research) -> str:
    """Advance one account by exactly one pipeline stage.
    Returns the new status. Idempotent per stage."""
    acct = manager.get(account_id)

    if acct.status == "onboarding":
        brief = research_fn(acct.niche_hint or "")
        manager.ledger.decide(
            "onboarding", "niche_research", acct.username,
            "auto niche research complete",
            {"brief": brief, "hint": acct.niche_hint})
        return manager.transition(account_id, "researching").status

    if acct.status == "researching":
        persona = assign_persona(acct.niche_hint, manager.active_personas())
        p = describe(persona)
        manager.ledger.decide(
            "onboarding", "persona_assigned", acct.username,
            f"assigned {p['label']}",
            {"persona": persona,
             "affiliate_niches": p["affiliate_niches"],
             "red_lines": p["red_lines"]})
        return manager.transition(
            account_id, "persona_assigned",
            persona=persona, niche=p["label"]).status

    if acct.status == "persona_assigned":
        # content factory grows the account with short videos
        return manager.transition(account_id, "growing").status

    if acct.status == "growing":
        if (acct.followers or 0) >= FOLLOWERS_TO_LIVE:
            manager.ledger.decide(
                "onboarding", "live_unlocked", acct.username,
                f"{acct.followers} followers >= {FOLLOWERS_TO_LIVE}",
                {"followers": acct.followers})
            return manager.transition(account_id, "live_ready").status
        return "growing"  # keep posting short videos

    return acct.status  # live_ready/live/paused/...: pipeline done
