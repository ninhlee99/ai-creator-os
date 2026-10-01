"""Orchestrator: scheduler + agent loop. The brain.

Schedules:
  hunter   daily    06:00
  content  hourly   08:00-22:00
  streamer live windows (config, default 2 sessions/day)
  analyst  after each session + daily rollup 23:30

Usage:
  python -m apps.orchestrator.main --dry-run     # rehearsal
  python -m apps.orchestrator.main --once hunter # single agent, once
  python -m apps.orchestrator.main               # daemon (production)
"""
from __future__ import annotations

import argparse
import sys
import time
from datetime import datetime

sys.path.insert(0, ".")

from apps.orchestrator.config import config
from ledger.store import Ledger

AGENTS = ("hunter", "content", "streamer", "analyst")


def run_agent(name: str, ledger: Ledger) -> dict:
    print(f"[{datetime.now():%H:%M:%S}] agent={name} starting (dry_run={config.dry_run})")
    if name == "hunter":
        from agents.hunter.agent import run
        return run(config, ledger)
    if name == "content":
        from agents.content.agent import run
        from engines.llm.provider import build_llm
        from engines.tts.provider import build_tts
        llm = build_llm(config.gemini_api_key, config.ollama_base_url,
                         config.ollama_model)
        tts = build_tts(config.tts_api_key)
        return run(config, ledger, llm, tts, tiktok=None)
    if name == "streamer":
        from agents.streamer.agent import run_live
        from engines.llm.provider import build_llm
        from engines.tts.provider import build_tts
        from engines.avatar.provider import build_avatar
        llm = build_llm(config.gemini_api_key, config.ollama_base_url,
                         config.ollama_model)
        tts = build_tts(config.tts_api_key)
        avatar = build_avatar(config.avatar_provider, config.avatar_api_key)
        from apps.stream_engine_client import StreamEngine
        return run_live(config, ledger, llm, tts, avatar, StreamEngine())
    if name == "analyst":
        from agents.analyst.agent import run
        return run(config, ledger)
    raise ValueError(name)


def rehearse(ledger: Ledger):
    """Full pipeline against mocks. Must pass before any live run."""
    print("== REHEARSAL ==")
    for name in AGENTS:
        try:
            result = run_agent(name, ledger)
            print(f"  {name}: {result}")
        except NotImplementedError as e:
            print(f"  {name}: PENDING ({e})")
        except Exception as e:  # noqa: BLE001
            print(f"  {name}: FAIL {e}")
    print("== END ==")


def daemon(ledger: Ledger):
    print("orchestrator daemon started. Ctrl-C to stop.")
    last = {a: 0.0 for a in AGENTS}
    intervals = {"hunter": 86400, "content": 3600, "analyst": 86400}
    # streamer runs on explicit live windows; simplified here
    while True:
        now = time.time()
        for name, every in intervals.items():
            if now - last[name] >= every:
                try:
                    print(run_agent(name, ledger))
                except Exception as e:  # noqa: BLE001
                    print(f"agent {name} failed: {e}")
                last[name] = now
        time.sleep(60)


def network_daemon(ledger: Ledger):
    """Multi-account network loop (docs/MODEL.md). One tick/minute.

    Master switch: MASTER_SWITCH=1 in env (default OFF). Kill: KILL_SWITCH=1.
    Nothing starts in dry-run; slot plans are still computed and audited.
    """
    import os
    from apps.orchestrator.network.account import AccountManager
    from apps.orchestrator.network.daemon import (NetConfig, NetState, tick)

    cfg = NetConfig(
        master_switch=os.environ.get("MASTER_SWITCH") == "1",
        kill_switch=config.kill_switch,
        dry_run=config.dry_run,
        max_concurrent_lives=int(os.environ.get("MAX_CONCURRENT_LIVES", "2")),
    )
    manager = AccountManager(ledger)
    state = NetState()
    print(f"network daemon started. master_switch={cfg.master_switch} "
          f"dry_run={cfg.dry_run}. Ctrl-C to stop.")
    while True:
        try:
            out = tick(state, cfg, ledger, manager,
                       run_agent=lambda name: run_agent(name, ledger))
            if any(out[k] for k in ("onboarded", "started", "stopped",
                                    "stopped_all")):
                print(f"[{datetime.now():%H:%M:%S}] {out}")
        except Exception as e:  # noqa: BLE001
            print(f"network tick failed: {e}")
        time.sleep(60)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--once", choices=AGENTS)
    ap.add_argument("--rehearse", action="store_true")
    ap.add_argument("--network", action="store_true",
                    help="multi-account network daemon (docs/MODEL.md)")
    args = ap.parse_args()

    if args.dry_run:
        object.__setattr__(config, "dry_run", True)

    ledger = Ledger(config.database_path)
    if args.network:
        network_daemon(ledger)
        return
    if args.rehearse or args.once:
        if args.once:
            print(run_agent(args.once, ledger))
        else:
            rehearse(ledger)
        return
    daemon(ledger)


if __name__ == "__main__":
    main()
