"""Content factory: short videos + AI short films, multi-platform.

Per account, per persona:
  storyteller -> short_film (AI illustrated films) + short_video (affiliate)
  teacher      -> short_video (lessons)
  gamer/coder  -> short_video (highlights/tips)
  musician     -> ai_music / ai_remix (phase 3)

Every rendered video is published through all configured publishers for
the account (TikTok, Facebook Page, YouTube). YouTube additionally filters
by account.youtube_content_types — each channel only receives the kinds
its owner enabled. Draft-first wherever the platform supports it.
"""
from __future__ import annotations

import subprocess
import sys
from pathlib import Path
sys.path.insert(0, ".")

from apps.orchestrator.config import Config
from apps.orchestrator.governance import check_kill_switch, evaluate_all
from ledger.store import Ledger
from publishers import build_publishers
from publishers.base import CONTENT_KINDS

from .film import make_film, render_scene, synth_narration, wav_seconds


# persona -> content kinds it produces (in priority order)
KINDS_BY_PERSONA = {
    "storyteller": ("short_film", "short_video"),
    "teacher": ("short_video",),
    "gamer": ("short_video",),
    "coder": ("short_video",),
    "musician": ("ai_music", "ai_remix"),
    "dancer": ("short_video",),
}


def default_slide(prompt: str, out: Path, color: str = "0x1a1a2e") -> Path:
    """Fallback visual when no image model is wired: title-ish color slide."""
    subprocess.run(
        ["ffmpeg", "-y", "-f", "lavfi", "-i",
         f"color=c={color}:s=1080x1920:d=1",
         "-frames:v", "1", str(out)],
        check=True, capture_output=True)
    return out


def pick_kind(account, ledger: Ledger) -> str:
    """The model decides what this account produces. Persona gives the
    menu; recent performance (ledger) picks the winner — v1: persona
    priority order, YouTube-gated kinds only when the channel allows."""
    for kind in KINDS_BY_PERSONA.get(account.persona or "", ("short_video",)):
        if kind in ("ai_music", "ai_remix", "short_film"):
            # long-form/music kinds need an enabled YouTube channel
            if (account.youtube_content_types
                    and kind in account.youtube_content_types):
                return kind
            # storyteller films can also go to TikTok/FB without YouTube
            if kind == "short_film":
                return kind
            continue
        return kind
    return "short_video"


def make_short_video(topic: str, tts, image_fn, workdir: Path,
                     voice: str = "default") -> dict:
    """Single-scene affiliate/lesson video."""
    workdir.mkdir(parents=True, exist_ok=True)
    img = workdir / "cover.png"
    image_fn(topic, img)
    wav = workdir / "voice.wav"
    synth_narration(tts, topic, voice, wav)
    out = workdir / "short.mp4"
    render_scene(img, wav, wav_seconds(wav) + 0.5, out)
    return {"ok": True, "path": str(out),
            "seconds": round(wav_seconds(wav), 1)}


def distribute(account, kind: str, video_path: str, title: str,
               description: str, ledger: Ledger) -> list[dict]:
    """Publish one video to every configured platform for the account.
    Returns per-platform results; all are recorded in the ledger."""
    results = []
    for pub in build_publishers(account, kinds=(kind,)):
        if not pub.handles(kind):
            continue
        r = pub.publish(video_path, title, description, kind)
        results.append({"platform": pub.name, "ok": r.ok,
                        "remote_id": r.remote_id, "url": r.url,
                        "draft": r.draft, "error": r.error})
        ledger.decide(
            "content", "published" if r.ok else "publish_failed",
            f"{account.username}:{kind}",
            f"{pub.name} ok={r.ok} draft={r.draft} "
            f"{r.remote_id or r.error or ''}"[:200],
            {"platform": pub.name, "kind": kind,
             "remote_id": r.remote_id, "url": r.url, "draft": r.draft,
             "error": r.error})
    return results


def run_for_account(cfg: Config, ledger: Ledger, llm, tts, account,
                    image_fn=None) -> dict:
    """Produce + distribute one content item for one account.
    The model picks the kind; the account config picks the platforms."""
    v = evaluate_all(cfg, ledger.daily_spend_usd())
    if not v.allowed:
        return {"ok": False, "reason": v.reason}
    image_fn = image_fn or default_slide

    kind = pick_kind(account, ledger)
    topic = (account.topics or [account.niche or ""])[0] if (
        account.topics or account.niche) else "giới thiệu"
    workdir = Path(f"work/content/{account.username}") / kind
    voice = {"storyteller": "narrator",
             "teacher": "clear-teacher"}.get(account.persona or "",
                                             "default")

    if kind == "short_film":
        made = make_film(topic, llm, tts, image_fn, workdir, voice=voice)
    elif kind in ("ai_music", "ai_remix"):
        # phase 3: music pipeline lands here; v1 keeps the slot reserved
        ledger.decide("content", "skip", account.username,
                       f"{kind} reserved for phase 3")
        return {"ok": True, "skipped": kind}
    else:
        made = make_short_video(topic, tts, image_fn, workdir, voice=voice)

    title = made.get("title", topic)[:100]
    cid = ledger._insert("content_items",
                         {"product_id": None, "kind": kind,
                          "script": title, "status": "rendered"})
    pubs = distribute(account, kind, made["path"], title,
                      f"{title} #aivideo", ledger)
    ledger.decide("content", "distributed", str(cid),
                   f"{kind} -> {[p['platform'] for p in pubs if p['ok']]}",
                   {"kind": kind, "results": pubs})
    return {"ok": True, "kind": kind, "path": made["path"],
            "platforms": pubs}


def run(cfg: Config, ledger: Ledger, llm, tts, tiktok=None) -> dict:
    """Legacy entry: run for every onboarded account (network mode)."""
    from apps.orchestrator.network.account import AccountManager
    mgr = AccountManager(ledger)
    done = []
    for acct in mgr.list(statuses=("growing", "live_ready", "live")):
        try:
            done.append({acct.username: run_for_account(
                cfg, ledger, llm, tts, acct)})
        except Exception as e:
            ledger.decide("content", "error", acct.username, str(e)[:200])
    return {"ok": True, "accounts": done}
