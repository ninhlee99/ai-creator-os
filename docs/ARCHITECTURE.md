# ai-creator-os — Architecture

> Mô hình kinh doanh đã chốt tại `docs/MODEL.md` (AI Creator Network:
> multi-account, mỗi account một persona). File này mô tả kỹ thuật thực thi.
> Khi mâu thuẫn, MODEL.md thắng.

Autonomous multi-account TikTok network: AI personas run entertainment
livestreams on staggered schedules, earn LIVE gifts, sell affiliate products
via short videos, and release AI-composed music — with the human only
flipping the master switch and watching the dashboard.

## 1. Design principles

1. **One money loop, per account.** Everything serves: discover → attract →
   convert → reconcile → reinvest. Tracked separately for each account so
   winners get prime slots and losers get cut.
2. **Lightweight stack — Go first.** Go for orchestration, scheduling,
   account management, streaming and API (one ~10MB binary, 5–20MB RAM per
   service, instant start). Python only where the ecosystem forces it
   (game-playing agent, local TTS/music glue) as isolated workers.
   SQLite for state. No Kubernetes, no Java, no Node, no heavy frontend
   build. Must run comfortably on a Mac M1 Pro 32GB.
3. **Free API first, local second, paid optional.** Every external capability
   (LLM, TTS, avatar) is a provider interface with a priority chain. Free
   realtime APIs are tried first; local models are the fallback; paid APIs are
   opt-in and capped.
4. **Deterministic governance over money.** LLMs propose; a small rules engine
   disposes. Budgets, kill thresholds and payouts are code, not prompts.
5. **Provider evidence only.** Revenue, orders and commissions are recorded
   only from TikTok's own data. The system never invents a number.
6. **Account safety is a feature.** Accounts are the scarcest asset.
   Multi-account guardrails (no cross-interaction, no duplicate content,
   no ban evasion) are enforced in code, not left to agent discretion.

## 2. Component map

```
┌─────────────────────────────────────────────────────────────┐
│ MASTER SWITCH + DASHBOARD                                   │
│  human: ON/OFF, add account, watch. One kill switch stops   │
│  every account within seconds.                              │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌───────────────────────▼─────────────────────────────────────┐
│ ORCHESTRATOR (Go - target; Python scaffold is reference)    │
│  AccountManager: registry, onboarding pipeline, RTMP keys  │
│  PersonaEngine:  persona per account (voice, avatar, niche) │
│  Scheduler:      golden-hour slots, max 2 concurrent lives  │
│  Governance:     deterministic rules, budget caps, audit    │
└──────┬──────────┬──────────────┬──────────────┬──────────────┘
       │          │              │              │
┌──────▼───┐ ┌────▼─────┐ ┌──────▼──────┐ ┌─────▼──────┐
│ HUNTER   │ │ CONTENT  │ │ STREAMER    │ │ ANALYST    │
│ per-acct │ │ per-acct │ │ per-account │ │ per-account│
│ niche    │ │ short    │ │ live show   │ │ gift/ROI,  │
│ products │ │ video    │ │ director    │ │ slot       │
│          │ │ factory  │ │ (persona)   │ │ optimizer  │
└──────┬───┘ └────┬─────┘ └──────┬──────┘ └─────┬──────┘
       │          │              │              │
┌──────▼───────────▼──────────────▼──────────────▼──────────────┐
│ ENGINES (free API -> local -> paid)                         │
│  llm/    gemini-free -> ollama                              │
│  tts/    free-vi-tts -> local                               │
│  avatar/ local-stylized -> streaming-api (paid, optional)    │
│  music/  licensed-ai-model -> human-edit -> similarity-check │
│  game/   python worker: vision + controller (per game)       │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ STREAM-ENGINE (Go) x N - one supervisor per live account    │
│  FFmpeg RTMP publish (per-account key), overlay hot-reload, │
│  watchdog + auto-reconnect, resource-capped                 │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ LEDGER (SQLite WAL) - per-account: products, sessions,      │
│ gifts, orders, commissions, decisions, api_usage            │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ API + DASHBOARD (minimal HTML)                              │
│  master switch, accounts, slots, review queue, per-acct P&L │
└─────────────────────────────────────────────────────────────┘
```

## 3. The four agents

### Hunter — product discovery (scheduled, daily)
- Pulls affiliate products from TikTok Shop affiliate marketplace API.
- Scores each product: `score = commission_value × conversion_rate / competition`
  (commission %, not commission value alone — a 30% cut of nothing is nothing).
- Writes ranked candidates to ledger. Top-N enter the "shelf" after passing
  governance (category allowlist, price band, seller rating floor).

### Content — short video factory (scheduled, hourly during day)
- Takes products from the shelf, generates script → TTS voiceover → FFmpeg
  assembles video (stock/AI visuals + captions + product card).
- Posts via TikTok Content Posting API (draft-first mode by default).
- Every video links the affiliate product. Metrics flow back to analyst.

### Streamer — live show director (during live windows)
- Show format: entertainment-first (games, trivia, stories, challenges),
  product moments woven in — never a pure selling stream.
- Loop: LLM director produces the next segment → TTS speaks → avatar renders →
  stream-engine publishes via RTMP. Chat/votes feed back where an authorized
  event source exists.
- Session policy enforced in code: max duration, pacing, break intervals,
  AI disclosure overlay.

### Analyst — money (after each live session + daily rollup)
- Pulls orders/commissions from TikTok Shop API (provider evidence only).
- Computes per-product and per-session ROI: revenue, commission, cost
  (API usage, compute), net.
- Kill/scale rules (examples, tuned in config):
  - kill product if 0 orders after N sessions or X views;
  - scale winners by increasing content cadence and live product moments.
- Decisions go through governance; analyst cannot spend, only propose.

## 4. Engines: the provider chains

Each engine is an interface. The orchestrator asks for a capability; the
engine walks its chain until one succeeds. Usage is metered so free-tier
caps are never silently exceeded.

| Engine | Chain (in order) | Notes |
|---|---|---|
| LLM | Gemini free tier → Ollama local | Director/reasoning. Free API first for speed; local for privacy/offline. |
| TTS | Free Vietnamese TTS API → local model | Must support natural Vietnamese prosody: rhythm, pitch, stress. Chain order confirmed by research. |
| Avatar | Local stylized realtime → paid streaming API | See §5 — honest limitation documented. |

## 5. Avatar: the hard truth

Photorealistic + realtime + frame-coherent lip-sync + free does not exist as
a production-ready option today. Anyone promising all four is selling
something. Therefore:

- **v1 ships a high-quality stylized realtime avatar** (2D/3D, coherent
  visemes, consistent identity). Entertainment value comes from persona,
  voice and show format — not photorealism.
- The `engines/avatar` interface is swappable: when a paid streaming-avatar
  API (or a future open model) meets the bar, it plugs in with zero changes
  to show logic.
- Lip-sync is driven by TTS phoneme/viseme timing, never by guessing.

## 6. Governance (deterministic, Python)

`apps/orchestrator/governance.py` — pure functions, no LLM inside:

- Budget caps: per-day API spend, per-session stream cost.
- Kill thresholds: products and content formats die by rule, not by debate.
- Approval gates: first-time actions (new product category, new stream
  format) require human approval via dashboard; afterwards the rule runs alone.
- Global kill switch: one call stops all agents and the stream within seconds.
- Every decision is written to the ledger with its inputs (audit trail).

## 7. Ledger (SQLite, WAL mode)

Single file, zero ops, SSD-friendly. Tables: products, content_items,
live_sessions, live_events, orders, commissions, decisions, api_usage.
Append-only for money tables. Nightly snapshot backup. Postgres migration
path documented for when (if) scale demands it.

## 8. Stream engine (Go)

A tiny supervisor, not a media framework:

- Owns the FFmpeg process: builds the filter graph (avatar scene + overlays +
  audio), pushes RTMP to TikTok.
- Hot-reloads overlays (product cards, AI responses, game scores) without
  restarting the stream.
- Watchdog: detects FFmpeg death or RTMP drop, reconnects with backoff,
  gives up and alerts after N tries.
- Resource caps: pinned CPU/memory so a runaway never takes the machine down.

## 9. Strict operational rules (encoded, not suggested)

1. **Dry-run default.** Nothing touches TikTok until `--live` is explicitly
   passed and the checklist in docs/OPERATIONS.md is signed off in config.
2. **Anti-ban pacing.** Max live duration per session, mandatory breaks,
   no duplicate content spam, AI disclosure on stream per TikTok policy.
3. **Idempotency.** Every external action carries an idempotency key; retries
   never double-post or double-count.
4. **Secrets.** Env vars / macOS Keychain only. Never in repo, logs or chat.
5. **Alerting.** Telegram alerts on: stream death, API quota near-limit,
   governance kill, any error in money path. The human is hands-off but
   never blind.
6. **Rehearsal.** Full pipeline runs end-to-end against mocks before any
   real session. `make rehearse` must pass.
7. **Backups.** Nightly SQLite snapshot + config backup, 30-day retention.
8. **Cost meters.** Every engine call logs usage; free-tier caps are hard
   stops, not warnings.

## 10. Language decision record (2026-10-01)

**Go is the primary language** for everything hot: orchestrator, scheduler,
account manager, stream supervisor, control-plane API. One static binary
(~10MB), 5–20MB RAM per service, millisecond startup, goroutines multiplex
N accounts on one machine. It is the lightest choice that stays easy to
write and debug for a one-person team.

**Python stays only where the ecosystem forces it**, as isolated workers
invoked by the Go orchestrator:
- game-playing agent (mss / pyautogui / OpenCV have no Go equivalent worth
  using),
- local Vietnamese TTS glue and music-pipeline glue.

The current Python scaffold remains the executable reference for business
logic (persona rules, schedule policy, governance); hot paths migrate to Go
during production wiring. No logic changes in migration — only the runtime.

**Not used:** Node.js (RAM-hungry), Rust (slow iteration for solo dev),
Kubernetes/Java/heavy frontend builds (ops cost with zero revenue link).

## 11. What this v1 does NOT do

- No real-time TikTok chat reading unless an authorized event source exists
  (no unofficial scraping — ban risk is unacceptable).
- No photorealistic avatar (see §5).
- No autonomous spending beyond configured caps.
- No cross-account interaction, ever (guardrail, not a missing feature).
