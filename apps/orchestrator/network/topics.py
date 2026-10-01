"""Topic engine: per-account niche resolution.

Policy (user-confirmed 2026-10-01):
- The user MAY set an affiliate/topic hint per account (`niche_hint`).
- If a hint is set, the model respects it and only expands it into
  concrete episode topics.
- If no hint is set, the model auto-researches a niche via the free LLM
  chain, avoiding niches already taken by other accounts in the network.
- LIVE scheduling is fully hands-off: the user never intervenes.

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
    "musician": "nhạc AI thư giãn",
    "dancer": "nhảy theo trend remix",
}


@dataclass
class TopicPlan:
    niche: str
    topics: list
    source: str  # "user" | "auto" | "fallback"


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
            f"Người dùng đã CHỌN chủ đề affiliate cho kênh này: \"{hint}\". "
            "Hãy cụ thể hoá thành một niche rõ ràng và 5-8 ý tưởng tập/video "
            "có thể sản xuất hàng ngày bằng AI (kịch bản + giọng đọc + hình ảnh). "
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

    - account.niche_hint set  -> source="user", LLM expands the hint.
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
                        source="user" if hint else "auto",
                    )
        except Exception:
            pass  # fall through to fallback

    niche = hint if hint else FALLBACK_NICHES.get(persona_id, "nội dung AI")
    return TopicPlan(niche=niche, topics=[], source="user" if hint else "fallback")


def apply_to_account(account, plan: TopicPlan) -> None:
    """Persist the resolved niche onto the account object."""
    account.niche = plan.niche
