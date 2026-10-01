"""Onboarding pipeline: new account -> live_ready, fully automatic.

  onboarding -> researching -> persona_assigned -> growing -> live_ready

The human only does step 0: add_account(username, rtmp_key_ref, niche_hint).
Everything after is the pipeline. The research step is injected
(research_fn) so tests and rehearsal can use a mock.
"""
from __future__ import annotations

from .account import AccountManager
from .persona import assign_persona, describe
from .topics import TopicPlan, resolve_topic

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


def make_topic_research(manager: AccountManager, account_id: int,
                        llm=None):
    """Build a research_fn backed by the topic engine.

    Honors the account's niche_hint when the user set one; otherwise the
    model auto-researches a niche via the free LLM chain, avoiding niches
    already taken by other accounts.
    """
    def _research(niche_hint: str) -> dict:
        acct = manager.get(account_id)
        taken = [a.niche for a in manager.list()
                 if a.niche and a.id != account_id]
        plan: TopicPlan = resolve_topic(acct, llm=llm,
                                        network_niches=taken)
        manager.set_topic_plan(account_id, plan.niche, plan.topics)
        return {
            "niche": plan.niche,
            "topics": plan.topics,
            "source": plan.source,   # user | auto | fallback
            "hint": acct.niche_hint,
            "trending_hashtags": [],
            "competitor_notes": "",
            "affiliate_categories": [],
        }
    return _research


def onboard_step(manager: AccountManager, account_id: int,
                 research_fn=None, llm=None) -> str:
    """Advance one account by exactly one pipeline stage.
    Returns the new status. Idempotent per stage.

    research_fn: injected for tests/rehearsal. When None, the real topic
    engine is used (user hint honored, else auto-research via llm).
    """
    acct = manager.get(account_id)

    if acct.status == "onboarding":
        if research_fn is None:
            research_fn = make_topic_research(manager, account_id, llm)
        brief = research_fn(acct.niche_hint or "")
        manager.ledger.decide(
            "onboarding", "niche_research", acct.username,
            f"topic resolved (source={brief.get('source')})",
            {"brief": brief, "hint": acct.niche_hint})
        return manager.transition(account_id, "researching").status

    if acct.status == "researching":
        # Persona is chosen to FIT the researched niche. The hint was only
        # a suggestion; the niche resolved in the research step is never
        # overwritten by the persona label.
        niche_for_match = acct.niche or acct.niche_hint or ""
        persona = assign_persona(niche_for_match, manager.active_personas())
        p = describe(persona)
        manager.ledger.decide(
            "onboarding", "persona_assigned", acct.username,
            f"assigned {p['label']} for researched niche '{acct.niche}'",
            {"persona": persona, "niche": acct.niche,
             "affiliate_niches": p["affiliate_niches"],
             "red_lines": p["red_lines"]})
        return manager.transition(
            account_id, "persona_assigned", persona=persona).status

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
