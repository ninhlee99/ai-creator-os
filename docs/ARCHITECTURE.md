# ai-creator-os — Architecture

> Mô hình kinh doanh đã chốt tại `docs/MODEL.md` (AI Creator Network:
> multi-account, mỗi account một persona). File này mô tả kỹ thuật thực thi.
> Khi mâu thuẫn, MODEL.md thắng.

Autonomous multi-account TikTok network: AI personas run entertainment
livestreams on staggered schedules, earn LIVE gifts, sell affiliate products
via short videos, and release AI-composed music — with the human only
flipping the master switch and watching the dashboard.

> **2026-10-02 — Kiến trúc Agent Team v2:** hệ thống được tổ chức lại thành
> một đội agent chuyên sâu (Orchestrator + Hunter/Director/Producer/QC/
> Publisher/Analyst/Streamer), mỗi nhiệm vụ mới nhân bản cả đội thành một
> TeamInstance song song, cô lập theo taskID. Thiết kế đầy đủ:
> [`docs/AGENT_TEAM.md`](AGENT_TEAM.md) · Sơ đồ:
> [`docs/assets/architecture-agent-team.svg`](assets/architecture-agent-team.svg) ·
> Mô phỏng trao đổi: [`docs/assets/agent-team-workflow.svg`](assets/agent-team-workflow.svg) ·
> UI/UX: [`docs/UI_UX_BLUEPRINT.md`](UI_UX_BLUEPRINT.md).

## 1. Design principles

1. **One money loop, per account.** Everything serves: discover → attract →
   convert → reconcile → reinvest. Tracked separately for each account so
   winners get prime slots and losers get cut.
2. **Lightweight stack — Go only.** The entire codebase is Go: orchestration,
   scheduling, account management, streaming, API/dashboard, provider chains
   and governance — one static binary (`aicos`), ~15MB, tens of MB RAM,
   instant start, goroutines multiplex N accounts on one machine.
   No Python runtime, no venv, no pip. SQLite for state. No Kubernetes,
   no Java, no Node, no heavy frontend build. Runs comfortably on a
   Mac M1 Pro 32GB.
   Third-party model servers (VieNeu-TTS v3, llama-server) run as isolated
   sidecar subprocesses managed by the Go binary — they are external tools
   like FFmpeg, not our code.
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
│ ORCHESTRATOR (Go — done since v0.5)                           │
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
│ ENGINES (user-configurable provider chains)                   │
│  llm/    gemini -> llama-server local (GGUF) -> paid (off)    │
│  tts/    gemini -> vieneu-v3 local -> edge (all swappable)    │
│  avatar/ local-stylized -> streaming-api (paid, optional)    │
│  music/  phase 3: licensed-ai-model -> human-edit            │
│  game/   phase 2: Go-native where allowed by game ToS        │
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

| Engine | Default chain (user-reorderable in Settings) | Notes |
|---|---|---|
| LLM | Gemini → llama-server local (GGUF 8B) → paid (opt-in, off) | Director/reasoning. Free API first for speed; local GGUF for offline/privacy. Chain order, keys and retry policy editable in dashboard Settings; failover is logged to decisions. |
| TTS | Gemini → VieNeu-TTS v3 local → Edge TTS | Must support natural Vietnamese prosody: rhythm, pitch, stress. VieNeu runs as a managed sidecar (OpenAI-compatible `POST /v1/audio/speech`); emotion cues (`[cười]`, `[thở dài]`) pass through untouched. |
| Avatar | Local sidecar (MuseTalk v1.5) → HeyGen → D-ID (paid, off by default) | Every frame is rendered per audio frame — static image + Ken Burns transitions are banned by design. See §5 — honest limitations documented. |

## 5. Avatar: the hard truth

Photorealistic + realtime + frame-coherent lip-sync + free does not exist as
a production-ready option today. Anyone promising all four is selling
something. Therefore (research: `docs/RESEARCH/avatar_pipeline.md`):

- **Default local = MuseTalk v1.5** (`dunso/musetalk-mac` port, MPS) run
  as a Go-managed sidecar. It lip-syncs from audio but does NOT generate
  head motion from audio, and it is NOT realtime on M1 Pro (~2.5–4 fps
  estimated → offline render ~6–10 min for a 60s clip). These numbers are
  extrapolated estimates — a real benchmark on the owner's Mac is required
  before the sidecar contract is final.
- **Quality bar (owner's requirement, enforced in code):** every frame must
  look like a real person filmed — lips, eyes, head, expression and gesture
  driven by the audio in every frame, consistent identity throughout.
  Static-image slideshows with transitions are banned: the pipeline refuses
  to render when the sidecar is down instead of returning a half-baked clip.
- **Identity lock:** `sha256(reference image + seed)`; a render is refused
  when the lock doesn't match the image on disk (no silent face drift).
- **Emotion cues:** 17 Vietnamese tags (`[cười]`, `[thở dài]`, `[hắng giọng]`,
  …) parsed from the TTS script and mapped to expressions per segment.
- The chain is `local → heygen → did` (paid tiers off by default, need API
  keys). Failover is logged to decisions, like every other engine.
- The `engines/avatar` interface stays swappable: when a future open model
  (or cloud GPU tier) meets the bar, it plugs in with zero changes to show
  logic.

## 6. Governance (deterministic, Go)

`internal/agents/governance` — pure functions, no LLM inside:

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
   real session. `go test ./...` must pass.
7. **Backups.** Nightly SQLite snapshot + config backup, 30-day retention.
8. **Cost meters.** Every engine call logs usage; free-tier caps are hard
   stops, not warnings.

## 10. Language decision record (2026-10-01, executed as v0.5-go)

**Go was chosen over Rust and Python** (owner delegated the choice):
- vs Python — one static binary, no interpreter/venv/pip; no GIL so N live
  accounts truly run concurrently; far lower RAM for 24/7 processes;
  compile-time type safety catches bugs (missing helpers, wrong signatures)
  that Python only reveals at runtime. Nothing in the design needs
  numpy/torch — AI goes through HTTP APIs, video through FFmpeg.
- vs Rust — this workload is I/O-bound (web server, API calls, scheduling,
  DB), not CPU-bound, so Rust's zero-cost abstractions buy little; Go ships
  features far faster for a one-person team (no borrow checker fights);
  builds take seconds not minutes; `net/http` + `html/template` are in the
  standard library.

**The codebase is now 100% Go.** Số liệu kiểm chứng lại ngày 2026-10-02:
119 file Go, 25.741 dòng, 43 file test / 250 test function, 20 package —
`go test ./...` xanh toàn bộ (xem §12 về những gì test chưa bao phủ).

**Not used:** Node.js (RAM-hungry), Kubernetes/Java/heavy frontend builds
(ops cost with zero revenue link). Local model sidecars (VieNeu-TTS,
llama-server) are third-party tools managed as subprocesses — like FFmpeg —
not our code.

## 11. What this v1 does NOT do

- No real-time TikTok chat reading unless an authorized event source exists
  (no unofficial scraping — ban risk is unacceptable).
- No photorealistic avatar (see §5).
- No autonomous spending beyond configured caps.
- No cross-account interaction, ever (guardrail, not a missing feature).

## 12. Known gaps & perfection criteria (audit 2026-10-02)

"Hoàn hảo" được định nghĩa bằng tiêu chí kiểm chứng được dưới đây — không
phải trạng thái vô hạn. Audit phân hai loại:

### 12.1 Sửa được trong VM (không cần Mac/tài khoản của Ninh)

| # | Khoảng trống | Trạng thái |
|---|---|---|
| 1 | Analyst mất luật kill theo phiên live (`sessionsFeatured` hard-code 0) | **Đã sửa 2026-10-02:** `Ledger.SessionsFeatured` (khớp `product_id` chính xác + fallback tựa đề 20 ký tự cho dữ liệu cũ), Streamer ghi event `product_moment` kèm `product_id`, có test |
| 2 | Dashboard bind mọi interface `:8080`, không auth | **Đã giảm rủi ro 2026-10-02:** mặc định `127.0.0.1:8080` (localhost); mở LAN phải chỉ định `-addr :8080` có chủ đích. Auth đầy đủ: việc tiếp theo |
| 3 | Daemon production chưa nối `OnStartLive`/`OnRunAgent` | Chưa nối — **cố ý chưa nối mù:** nối thẳng sẽ cho agent chạy mỗi 60s kể cả dry-run. Cần cổng cadence + dry-run gate trước (việc tiếp theo, kèm test) |
| 4 | Stream engine phát test pattern (`testsrc` + sine 440Hz), chưa phát avatar thật | Chưa sửa — cần thiết kế pipe frame/audio vào FFmpeg + benchmark M1 |
| 5 | `affiliatehunter` (ADB) chưa cắm vào products/UI | Chưa sửa — cần flow scan → store + nút UI; chạy thật cần Android của Ninh cắm USB vào Mac |
| 6 | Avatar paid tier (HeyGen/D-ID) còn khung "wiring pending" | Chưa sửa — cần API key thật để test hợp đồng REST |
| 7 | QC chưa thành agent độc lập chấm mù | Đã có thiết kế ở `AGENT_TEAM.md` §5; code là việc tiếp theo trên hạt nhân Studio job |
| 8 | Telegram alert + nightly backup được docs hứa nhưng chưa có code | Chưa sửa — phải hoặc làm, hoặc sửa docs; không để docs nói quá code |
| 9 | Đường content agent còn dùng ảnh tĩnh + zoompan (mâu thuẫn luật cấm slideshow cho phim) | Chưa sửa — cần chốt: chỉ dùng cho B-roll, phim đi đường Veo/Studio |
| 10 | UI/UX Settings trộn ~8 vấn đề; autopilot xé 3 nơi | Đã có bản thiết kế lại `UI_UX_BLUEPRINT.md`; đã làm các bước template thuần của đợt 1 (ô key `type=password`, vùng nguy hiểm viền đỏ, sidebar nhóm + active, mục lục Settings). Tách trang là các commit tiếp theo |

### 12.2 Cần Mac / tài khoản / quyết định của Ninh (không ai làm thay được)

1. E2E affiliate trên Mac thật: OAuth TikTok từng account, gắn giỏ hàng thủ
   công trong app (API không cho phép), đăng draft → public.
2. Benchmark avatar MuseTalk trên M1 Pro 32GB: fps/RAM thật (số 2.5–4fps
   hiện tại là ước tính), tải weights qua `AVATAR_MODEL_URL`.
3. Veo: Google Cloud project bật billing trên Gemini key; thiếu thì Studio
   tự rơi về photo-list.
4. RTMP key + quyền LIVE từng account, Gemini keys, FB Page ID (Facebook/
   YouTube Ninh đã chủ động hoãn).
5. Săn sản phẩm tự động: TikTok Shop API cần Partner Center + duyệt app
   (Ninh đã từ chối vì phức tạp) — đường thay thế ADB cần chiếc Android duy
   nhất của Ninh cắm vào Mac khi máy rảnh.
6. Nghiệm thu mắt người: bản dựng producer (mẫu chưa đủ đẹp, còn "AI", chưa
   nét) vẫn chờ verdict của Ninh từ 2026-10-01.

### 12.3 Tiêu chí được gọi là "xong" cho từng tầng

- **Logic:** `go test ./...` xanh + test mới cho mọi luật tiền/an toàn.
- **Nối dây:** daemon → agent → stream chạy end-to-end ở chế độ dry-run trên
  Mac, có event log để UI vẽ lại quá trình.
- **Thật:** một account live thật + một video affiliate đăng thật + một
  khoản hoa hồng đối soát vào Ledger từ bằng chứng nhà cung cấp.
- **Chất lượng:** Ninh duyệt mắt người trên sản phẩm thật, không qua trung gian.
