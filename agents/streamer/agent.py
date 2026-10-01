"""Streamer agent: live show director.

Entertainment-FIRST format: games, trivia, stories, challenges — product
moments woven in, never a pure selling stream. Persona and voice carry the
show; photorealism is explicitly NOT required (see ARCHITECTURE.md §5).

Loop per segment:
  director (LLM) -> script -> TTS -> avatar visemes -> stream-engine overlay
  -> published via RTMP. Chat/votes feed back where authorized.

Anti-ban pacing and session limits are enforced by governance, not by prompt.
"""
from __future__ import annotations

import sys
import time
sys.path.insert(0, ".")

from apps.orchestrator.config import Config
from apps.orchestrator.governance import check_live_session
from ledger.store import Ledger

# show rundown: entertainment blocks with product moments interleaved
SEGMENT_PLAN = [
    ("welcome", 3),        # chào, warmup, minigame mở màn
    ("game", 10),          # đố vui / thử thách
    ("product_moment", 4), # giới thiệu sản phẩm 1 (mềm)
    ("story", 8),          # kể chuyện / tương tác
    ("game", 10),
    ("product_moment", 4), # sản phẩm 2
    ("music_break", 5),
    ("product_moment", 4), # sản phẩm 3 / nhắc lại best-seller
    ("closing", 3),         # tổng kết, hẹn phiên sau
]

DIRECTOR_SYSTEM = (
    "Bạn là đạo diễn kiêm MC livestream bán hàng TikTok, tiếng Việt, "
    "giọng vui vẻ tự nhiên như người thật. Luân phiên giải trí và giới thiệu "
    "sản phẩm một cách mềm mại, không gượng ép. Mỗi segment chỉ vài câu thoại."
)


def direct_segment(llm, kind: str, context: dict) -> str:
    prompt = (f"Loại segment: {kind}. Bối cảnh: {context}. "
              f"Sản phẩm nổi bật: {context.get('featured_product', 'chưa có')}. "
              f"Viết lời thoại MC 3-5 câu.")
    res = llm.run(system=DIRECTOR_SYSTEM, prompt=prompt)
    if not res.ok:
        raise RuntimeError(f"director LLM failed: {res.error}")
    return str(res.payload)


def run_live(cfg: Config, ledger: Ledger, llm, tts, avatar,
             stream_engine, max_minutes: int | None = None) -> dict:
    """Run one full live session. Blocks until session ends."""
    limit = max_minutes or cfg.max_live_minutes_per_session
    session_id = ledger.start_session()
    ledger.log_event(session_id, "segment",
                     {"msg": "session started", "disclosure": cfg.ai_disclosure_text})
    try:
        stream_engine.start(cfg.tiktok_rtmp_url, cfg.tiktok_rtmp_key,
                            disclosure=cfg.ai_disclosure_text)
        elapsed = 0
        for kind, minutes in SEGMENT_PLAN:
            v = check_live_session(cfg, elapsed)
            if not v.allowed:
                ledger.log_event(session_id, "segment", {"msg": v.reason})
                break
            products = [dict(p) for p in ledger.shelf_products(limit=3)]
            featured = products[0]["title"] if products and kind == "product_moment" else None
            script = direct_segment(llm, kind, {"featured_product": featured})
            audio = tts.run(script)
            if not audio.ok:
                ledger.log_event(session_id, "error",
                                 {"msg": f"TTS failed: {audio.error}"})
                continue
            # viseme timeline derived from TTS; avatar renders coherently
            avatar.speak(audio.payload, visemes=[])  # TODO: viseme extraction
            stream_engine.push_segment(audio.payload, caption=script)
            ledger.log_event(session_id, "segment",
                             {"kind": kind, "script": script[:200]})
            elapsed += minutes
            # paced in real time; in rehearsal this is mocked
            time.sleep(1 if cfg.dry_run else minutes * 60)
    finally:
        stream_engine.stop()
        ledger.end_session(session_id, status="ended",
                           duration_min=elapsed)
    return {"ok": True, "session_id": session_id, "minutes": elapsed}
