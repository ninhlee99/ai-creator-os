"""Content agent: short-video factory.

Scheduled hourly (daytime). Takes products from the shelf, generates
script -> TTS voiceover -> FFmpeg assembles video (visuals + captions +
product card) -> posts via TikTok Content Posting API (draft-first).
"""
from __future__ import annotations

import subprocess
import sys
from pathlib import Path
sys.path.insert(0, ".")

from apps.orchestrator.config import Config
from apps.orchestrator.governance import check_kill_switch, evaluate_all
from ledger.store import Ledger


def build_script(llm, product: dict) -> str:
    """15-30s Vietnamese hook-driven script. LLM proposes."""
    res = llm.run(
        system="Bạn viết kịch bản video ngắn TikTok bán hàng, tiếng Việt, "
               "giọng tự nhiên, có hook 3 giây đầu, CTA cuối.",
        prompt=f"Sản phẩm: {product['title']}, giá {product['price']:.0f}đ. "
               f"Viết kịch bản 20 giây, gồm lời thoại và gợi ý hình ảnh.",
    )
    if not res.ok:
        raise RuntimeError(f"LLM failed: {res.error}")
    return str(res.payload)


def assemble_video(script_path: Path, audio_path: Path, out_path: Path):
    """FFmpeg: slideshow visuals + captions + audio. v1 simple, v2 richer."""
    # TODO: caption burn-in + product card overlay (research-backed template)
    cmd = ["ffmpeg", "-y", "-loop", "1", "-i", str(script_path),
           "-i", str(audio_path), "-shortest",
           "-c:v", "libx264", "-pix_fmt", "yuv420p",
           "-c:a", "aac", str(out_path)]
    subprocess.run(cmd, check=True, capture_output=True)


def run(cfg: Config, ledger: Ledger, llm, tts, tiktok) -> dict:
    v = evaluate_all(cfg, ledger.daily_spend_usd())
    if not v.allowed:
        return {"ok": False, "reason": v.reason}

    products = ledger.shelf_products(limit=5)
    made = 0
    for p in products:
        pd = dict(p)
        try:
            script = build_script(llm, pd)
            audio = tts.run(script)
            if not audio.ok:
                ledger.decide("content", "skip", str(pd["id"]),
                               f"TTS failed: {audio.error}")
                continue
            # TODO: persist audio, assemble video, post via tiktok client
            cid = ledger._insert("content_items",
                                 {"product_id": pd["id"], "kind": "short_video",
                                  "script": script, "status": "draft"})
            ledger.decide("content", "draft_video", str(cid),
                           f"product={pd['title'][:40]}")
            made += 1
        except Exception as e:
            ledger.decide("content", "error", str(pd["id"]), str(e))
    return {"ok": True, "videos_drafted": made}
