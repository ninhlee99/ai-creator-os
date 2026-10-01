# tiktok-affiliate-os

Autonomous TikTok affiliate commerce: AI agents discover high-commission
products, produce short videos, run entertainment livestreams, and reconcile
commissions — hands-off operation.

**Status:** v0.1 scaffold — architecture + core modules. Provider details
(TikTok APIs, TTS, avatar) are being confirmed by research in `docs/RESEARCH/`.

## Quick start (Mac)

```bash
cp .env.example .env        # fill in keys
pip install -r apps/orchestrator/requirements.txt
python -m apps.orchestrator.main --dry-run   # rehearsal, touches nothing
```

## Layout

- `apps/orchestrator/` — Python: scheduler, agent loop, governance rules
- `apps/api/` — Python FastAPI: control plane + dashboard
- `apps/stream-engine/` — Go: FFmpeg supervisor, RTMP publish
- `agents/` — hunter, content, streamer, analyst
- `engines/` — llm, tts, avatar (provider chains: free API → local → paid)
- `ledger/` — SQLite schema + store (append-only money tables)
- `infra/` — docker-compose, Mac launchd services
- `docs/` — ARCHITECTURE.md, OPERATIONS.md, POLICY_AND_SAFETY.md

## Principles

1. One money loop: discover → attract → convert → reconcile → reinvest.
2. Free API first, local second, paid optional — every engine is a chain.
3. LLMs propose; deterministic governance disposes (budgets, kill rules).
4. Provider evidence only — revenue numbers come from TikTok, never invented.
5. Account safety is a feature — anti-ban behavior enforced in code.

Read `docs/ARCHITECTURE.md` before touching anything.
