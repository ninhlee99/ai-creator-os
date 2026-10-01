"""Topic engine: per-account niche resolution + per-session LIVE planner.

Policy (user-confirmed 2026-10-01, refined):
- The user MAY set a topic hint per account (`niche_hint`).
- The hint is a SOFT suggestion, not a final decision: the model may
  refine, narrow, or replace it with an adjacent niche that produces
  and monetizes better. Recorded as source="hint".
- If no hint is set, the model auto-researches a niche via the free LLM
  chain, avoiding niches already taken by other accounts (source="auto").
- LIVE scheduling is fully hands-off: the user never intervenes.
- Each LIVE session gets its own topic from `plan_live_topic()`, chosen
  from the account's episode plan with evidence (recent session stats).

Personas follow docs/MODEL.md guardrails (e.g. teacher never poses as a
medical/financial/legal expert).
"""
from __future__ import annotations

import json
from dataclasses import dataclass

from .persona import PERSONAS

# Fallback niches when the LLM chain is unavailable. Persona -> niche.
FALLBACK_NICHES = {
    "storyteller": "kể chuyện đêm khuya",
    "teacher": "tiếng Anh qua truyện ngắn",
    "gamer": "game indie chill",
    "coder": "live code game và mini-app",
    "musician": "nhạc AI thư giãn",
    "dancer": "nhảy theo trend remix",
}


@dataclass
class TopicPlan:
    niche: str
    topics: list
    source: str  # "hint" | "auto" | "fallback"


def _parse_plan(raw: str) -> dict | None:
    """Extract JSON from an LLM reply (tolerates ``` fences)."""
    text = raw.strip()
    if text.startswith("```"):
        text = text.split("```")[1]
        if text.lstrip().startswith("json"):
            text = text.lstrip()[4:]
    try:
        data = json.loads(text.strip())
    except Exception:
        return None
    if not isinstance(data, dict) or not data.get("niche"):
        return None
    topics = data.get("topics") or []
    topics = [str(t).strip() for t in topics if str(t).strip()][:8]
    return {"niche": str(data["niche"]).strip(), "topics": topics}


def _prompt(persona_id: str, hint: str | None, taken: list[str]) -> tuple[str, str]:
    p = PERSONAS.get(persona_id, PERSONAS["storyteller"])
    label, desc = p["label"], p["live_style"]
    system = (
        "Bạn là chiến lược gia nội dung TikTok/YouTube cho thị trường Việt Nam. "
        "Chỉ trả lời bằng JSON thuần, không giải thích, không markdown ngoài JSON. "
        "Định dạng: {\"niche\": \"...\", \"topics\": [\"...\", ...]} (5-8 ý tưởng tập)."
    )
    if hint:
        prompt = (
            f"Persona: {label} — {desc}.\n"
            f"Người dùng GỢI Ý hướng chủ đề cho kênh này: \"{hint}\". "
            "Đây chỉ là gợi ý, KHÔNG phải quyết định cuối — bạn có toàn quyền "
            "điều chỉnh, cụ thể hoá, hoặc đề xuất niche lân cận tốt hơn nếu "
            "gợi ý gốc khó sản xuất/khó kiếm tiền. Hãy chốt 1 niche rõ ràng và "
            "5-8 ý tưởng tập/video có thể sản xuất hàng ngày bằng AI "
            "(kịch bản + giọng đọc + hình ảnh). "
            "Niche phải thân thiện với quảng cáo, phù hợp người Việt 18-35."
        )
    else:
        taken_txt = ", ".join(taken) if taken else "(chưa có)"
        prompt = (
            f"Persona: {label} — {desc}.\n"
            f"Các niche đã có trong hệ thống (TRÁNH trùng lặp): {taken_txt}. "
            "Đề xuất 1 niche MỚI, khác biệt, có thể sản xuất hàng ngày bằng AI, "
            "thân thiện quảng cáo, phù hợp người Việt 18-35, kèm 5-8 ý tưởng tập. "
            + ("KHÔNG đóng vai chuyên gia y tế/tài chính/luật. " if persona_id == "teacher" else "")
        )
    return system, prompt


def resolve_topic(account, llm=None, network_niches: list[str] | None = None) -> TopicPlan:
    """Resolve the working niche + episode topics for an account.

    - account.niche_hint set  -> source="hint": the hint is a SOFT suggestion.
      The model may refine, narrow, or replace it with an adjacent niche.
    - else LLM proposes        -> source="auto", avoiding network_niches.
    - LLM unavailable/fails    -> source="fallback", persona default niche.
    """
    persona_id = account.persona or "storyteller"
    if persona_id not in PERSONAS:
        persona_id = "storyteller"
    hint = (account.niche_hint or "").strip() or None
    taken = [n for n in (network_niches or []) if n]

    if llm is not None:
        system, prompt = _prompt(persona_id, hint, taken)
        try:
            res = llm.run(system=system, prompt=prompt)
            if res.ok:
                plan = _parse_plan(str(res.payload))
                if plan and plan["topics"]:
                    return TopicPlan(
                        niche=plan["niche"],
                        topics=plan["topics"],
                        source="hint" if hint else "auto",
                    )
        except Exception:
            pass  # fall through to fallback

    niche = hint if hint else FALLBACK_NICHES.get(persona_id, "nội dung AI")
    return TopicPlan(niche=niche, topics=[], source="hint" if hint else "fallback")


def apply_to_account(account, plan: TopicPlan) -> None:
    """Persist the resolved niche onto the account object."""
    account.niche = plan.niche


def plan_live_topic(manager, account_id: int) -> dict:
    """Plan the topic for the account's NEXT live session.

    Picks the least-recently-used episode topic (round-robin over the
    account's topic plan) and records the decision with its evidence:
    recent session stats (peak viewers, gift USD) when available.

    Returns {"topic": str, "basis": str}. The streamer calls this right
    before going live; the human never picks topics.
    """
    acct = manager.get(account_id)
    topics = list(acct.topics or [])
    if not topics:
        # No episode plan yet — fall back to the niche itself.
        topic = acct.niche or acct.niche_hint or "giao lưu với khán giả"
        basis = "no episode plan; using niche as session theme"
    else:
        planned = [
            d["target"] for d in
            (dict(r) for r in manager.ledger.db.execute(
                "SELECT target FROM decisions WHERE agent = 'live_planner' "
                "AND action = 'session_topic' AND target LIKE ? "
                "ORDER BY id DESC LIMIT 32",
                (f"{acct.username}:%",)).fetchall())
        ]
        used = [t.split(":", 1)[1] for t in planned if ":" in t]
        topic = next((t for t in topics if t not in used), topics[0])
        basis = (f"round-robin over {len(topics)} planned episodes; "
                 f"{len(used)} recent sessions avoided")
    evidence = _recent_session_evidence(manager, account_id)
    if evidence:
        basis += f"; recent evidence: {evidence}"
    manager.ledger.decide(
        "live_planner", "session_topic", f"{acct.username}:{topic}",
        f"next live session topic — {basis}",
        {"account_id": account_id, "topic": topic, "basis": basis})
    return {"topic": topic, "basis": basis}


def _recent_session_evidence(manager, account_id: int) -> str:
    """One-line summary of the last live sessions for planning context."""
    rows = manager.ledger.db.execute(
        "SELECT peak_viewers, duration_min, status FROM live_sessions "
        "WHERE account_id = ? ORDER BY id DESC LIMIT 3",
        (account_id,)).fetchall()
    if not rows:
        return ""
    parts = [f"{r['peak_viewers'] or 0} peak/{r['duration_min'] or 0}min"
             for r in rows]
    gifts = manager.ledger.db.execute(
        "SELECT COALESCE(SUM(usd), 0) u FROM gifts WHERE account_id = ? "
        "AND date(recorded_at) >= date('now', '-7 days')",
        (account_id,)).fetchone()["u"]
    return f"last sessions [{'; '.join(parts)}], 7d gifts ${gifts:.2f}"
