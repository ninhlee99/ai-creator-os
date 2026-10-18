# ai-creator-os — Architecture

> **PIVOT 2026-10-02 (Ninh chốt):** bỏ live avatar + bỏ phim điện ảnh, tập trung
> 3 trụ — Affiliate qua Accesstrade · Video Viral AI 100% Gốc (TikTok/Shorts/Reels) · YouTube kể chuyện
> ngôi thứ nhất. Thiết kế đích: `docs/PIVOT_REDESIGN.md`.
> Tài liệu này mô tả **trạng thái kỹ thuật hiện tại**.
> Khi mâu thuẫn với PIVOT_REDESIGN.md, file đó là đích đến.

## 1. Design principles

1. **Three money pillars.** Mọi thứ phục vụ 3 trụ: affiliate (hoa hồng),
   video viral 100% gốc (tăng trưởng kênh & thương hiệu), kể chuyện YouTube (view dài).
   Live và phim điện ảnh đã park — không phục vụ trụ nào nữa.
2. **Lightweight stack — Go only.** Toàn bộ code là Go: orchestration,
   account management, API/dashboard, provider chains, governance — một
   binary tĩnh (`aicos`). Không Python runtime, không venv, không pip.
   SQLite WAL cho state. Không Kubernetes, không Java, không Node, không
   frontend build nặng. Chạy tốt trên Mac M1 Pro 32GB.
   Model server bên thứ ba (VieNeu-TTS v3, llama-server) chạy như sidecar
   subprocess do binary Go quản lý — công cụ ngoài như FFmpeg, không phải
   code của dự án.
3. **Free API first, local second, paid never.** Mọi khả năng ngoài (LLM, TTS,
   vẽ ảnh) là provider interface với chuỗi ưu tiên. Free thử trước, local
   fallback; **paid tắt** (luật cứng của Ninh: key free-only, không billing,
   không Veo). Hỗ trợ thêm Web AI Multi-Account Hub (ChatGPT, Claude, Gemini,
   Grok, Muse AI) tự động bắt session qua Chrome DevTools Protocol (CDP).
4. **Deterministic governance over money.** LLM đề xuất; rules engine (Go)
   quyết định. Ngưỡng kill/double-down, quota, kill switch là code, không
   phải prompt.
5. **Provider evidence only.** Doanh thu, đơn hàng, hoa hồng chỉ ghi từ dữ
   liệu thật của provider. Hệ thống không bao giờ bịa số.
6. **Zero-touch & Autonomous Intelligence.** Ninh không can thiệp vận hành:
   Master Harness Agent (`internal/harness`) chạy vòng lặp OODA và ký ức kinh
   nghiệm (Episodic Memory) tự điều phối danh mục; mọi fallback, lịch,
   kill/double-down do hệ thống tự quyết theo rule đã định.

## 2. Component map (hiện tại)

```
┌─────────────────────────────────────────────────────────────┐
│ DASHBOARD + MASTER SWITCH + KILL SWITCH                     │
│  Điều hành: Tổng quan · Harness Agent 🧠 · Kênh · Đội ngũ   │
│  Sản xuất: Studio AI 🎬 · Affiliate · Viral 9:16 · Stories  │
│  Quản trị: Cài đặt Hệ thống (Kho Web AI) · Đa nền tảng      │
└──────────────────────┬──────────────────────────────────────┘
                       │
┌───────────────────────▼─────────────────────────────────────┐
│ internal/web — HTTP layer (routes.go, handler theo miền)     │
│  Template: templates/<page>.html; static/app.js + style.css │
└──────┬──────────────┬──────────────┬──────────────┬──────────┘
       │              │              │              │
┌──────▼──────┐ ┌─────▼───────┐ ┌────▼────────┐ ┌───▼──────────┐
│ harness     │ │ studio      │ │ growth      │ │ webai        │
│ OODA loop,  │ │ video jobs  │ │ plan 30d,   │ │ CDP Browser  │
│ episodic    │ │ (affiliate, │ │ ngưỡng,     │ │ login, cookie│
│ memory,     │ │ viral 9:16, │ │ quota,      │ │ capture,     │
│ portfolio   │ │ story 16:9) │ │ YT Analytics│ │ rotation     │
└──────┬──────┘ └─────┬───────┘ └─────────────┘ └───┬──────────┘
       │              │                             │
┌──────▼──────────────▼─────────────────────────────▼─────────┐
│ engines: LLM chain (Gemini→llama→paid tắt) · TTS chain      │
│ (Gemini→VieNeu→Edge) · Web AI pool (ChatGPT/Claude/Grok...) │
│ keyring xoay vòng, cooldown 60s→5m→15m khi 429              │
└─────────────────────────────────────────────────────────────┘
```

**Parked (build tag `parked`, không vào binary):** `internal/stream`
(stream engine), `internal/engines/avatar` (avatar + MuseTalk),
`internal/agents/{analyst,content,governance,hunter,streamer}`,
`internal/studio/film*.go` + `cinematic.go` (phim điện ảnh),
`internal/web/{schedule,team,avatar,studio_film}.go`,
`internal/web/templates_parked/`, `internal/studio/prompts/parked/`.
Test vùng parked chạy bằng `go test -tags parked ./...`.

**Đã code xong (đợt B→F):** Accesstrade client + datafeed hunter + đối soát
(`internal/accesstrade`), xưởng Video Viral AI 100% gốc 9:16
(`internal/studio/viral.go`), pipeline kể chuyện (`internal/studio/story.go` —
tách từ studio, kind `story` trên `studio_jobs`; automation
`internal/automation/story_tick.go` 24h/lần, đăng private-first, mặc định
chờ Ninh duyệt).

**Đợt H (2026-10-04):**
- **H1**: loại bỏ TikTok Shop hoàn toàn (Accesstrade-only) — xóa
  `internal/tiktok/shop.go`, `TikTokShopProvider`, wiring trong main.go,
  nhánh đối soát legacy, config `TIKTOK_SHOP_*` (kể cả vùng parked).
- **H2**: Agent Team định nghĩa lại quanh 3 pipeline — `/team` mới
  (`internal/web/team_pipeline.go`, không còn park): Affiliate
  (Hunter→Producer→Publisher→Analyst), Video Viral AI
  (Hunter→Director→Producer→QC→Publisher→Analyst), Story
  (Writer→Illustrator→Voice→Producer→Publisher). Trạng thái suy ra từ dữ liệu thật.
- **H3**: YouTube Analytics API (`internal/growth/youtube_analytics.go`,
  free) điền Views30d + watch hours 30 ngày cho growth thresholds;
  badge trạng thái trên trang Kênh; scope `yt-analytics.readonly`
  (script `get-youtube-token.py`).

## 3. Routes hiện tại (internal/web/routes.go)

| Route | Handler | Ghi chú |
|---|---|---|
| `GET /` | `handleDashboard` | Trang chủ điều hành & số liệu thời gian thực |
| `/harness*` | `handleHarness*` | **Master Agent Harness**: Vòng lặp nhận thức OODA, ký ức kinh nghiệm (Episodic Memory) |
| `/accounts*` | `handleAccounts*` | Kênh & Brand DNA (sidebar "Kênh") |
| `/growth*` | `handleGrowth*` | Phát triển kênh & ngưỡng tự động |
| `/studio*` | `handleStudio*` | Studio AI (affiliate/viral 9:16/audio story 16:9/jobs/trends) |
| `/products*` | `handleProducts*` | Affiliate (Accesstrade: chiến dịch/link/đối soát, đợt B/C) |
| `/viral*` | `handleViral*` | Video Viral AI: kịch bản 3s hook, hình ảnh 9:16, lồng tiếng 100% gốc |
| `/stories*` | `handleStory*` | Kể chuyện: tạo truyện, jobs, đăng YouTube (đợt F) |
| `GET /team` | `handleTeamPipelines` | Đội ngũ: sức khỏe 3 pipeline từ dữ liệu thật |
| `/publishers*` | `handlePublishers*` | Đa nền tảng (OAuth TikTok, YouTube, Meta Facebook/Instagram) |
| `/settings*` | `handleSettings*` | 5 trang con: Hệ thống (gồm **Kho Tài Khoản Web AI**) · Accesstrade · Nhà cung cấp · Model local · An toàn |
| `/web-ai*` | `handleWebAI*` | Quản lý kho tài khoản Web AI & Trình duyệt tự động đăng nhập CDP |
| `/api/*` | `requireAPI(...)` | Mặc định 404 khi tắt |
| `/onboard` | `handleOnboard` | Wizard lần đầu |
| `/kill`, `/unkill` | `handleKill` | Kill switch |

`/schedule` → 404 (parked). `/team` đã định nghĩa lại ở đợt H2 (không còn park).

## 4. Engines: the provider chains

`internal/engines/chains.go` — chỉ còn 2 chuỗi:
- **LLM**: Gemini (free API) → llama-server local (`127.0.0.1:8081`) → paid (placeholder, tắt).
- **TTS**: Gemini → VieNeu-TTS v3 local (sidecar `127.0.0.1:8000`) → Edge.
- **Web AI Hub** (`internal/webai`): ChatGPT, Claude, Gemini, Grok, Muse AI tự động bắt session CDP.
- Chuỗi Avatar đã gỡ theo live (Đợt A).

Keyring mỗi engine độc lập: round-robin, bỏ key invalid, cooldown 429/quota
thang 60s→5m→15m, key khoẻ lại reset thang. Failover tier nào cũng ghi
decision log (`engines/tier_failover`).

## 5. Governance (deterministic, Go)

- Kill switch > MASTER_SWITCH > dry-run — thứ tự thắng cứng, kiểm tra đầu
  mọi tick (`internal/automation/service.go`).
- Ngưỡng growth chỉnh được trên UI (`/growth/thresholds`), hỏng từng trường
  → fallback default (`internal/growth`).
- Quota YouTube: 1600 units/lượt, trần 10000 units/ngày (`internal/growth/quota.go`).
- Đơn hàng idempotent theo `external_id` (`Ledger.RecordOrder`).
- Giới hạn tần suất và Zero-Reup: Toàn bộ nội dung viral được sản xuất từ
  tài sản gốc 100% (Gemini MediaGen + VieNeu TTS + Kworb VN music).
  Không bao giờ tự xoá video đã đăng.

## 5b. Ticks automation (zero-touch, Ninh không chạm)

| Tick | Chu kỳ | Công tắc (unset = BẬT) | Fail-closed khi thiếu |
|---|---|---|---|
| AutopilotTick | 1′ | master/dry-run/kill | — |
| GrowthTick | 5′ | `growth.production_enabled` | `Growth == nil` → im lặng |
| ATTick (hunter/order/campaign) | 5′ | `at.hunter_enabled` / `at.ordersync_enabled` / `at.campaigncheck_enabled` | thiếu key → log + im lặng |
| ViralAutoTick | 5′/đến hạn 24h | `viral.auto_produce_enabled` | `Studio == nil` → im lặng |
| StoryTick | 5′/đến hạn 24h | `story.enabled` | `Story == nil` → im lặng |

Mọi tick: kill switch + dry-run chặn đầu; watermark last-run trong settings;
interval ≤ 0 → default. Trạng thái thật hiển thị ở `/viral`,
`/settings/accesstrade`, `/stories` — chỗ nào "chờ số liệu"/"chờ key" phải
nói rõ, không giả vờ đang chạy.

## 6. Ledger & Data Stores (SQLite, WAL mode)

- `ledger.db`: accounts, decisions, settings, orders, growth_*, api_usage, harness_cycles, harness_memories…
- `studio.db`: studio_jobs, studio_assets, capabilities.
- `products.db`: products, shelf.
- `accesstrade.db`: campaigns, links, orders (đợt B/C).
- `webai.db`: accounts (kho tài khoản Web AI, session tokens, cooldown state, metrics).
- Migration qua `PRAGMA user_version`, có test round-trip.

## 7. Strict operational rules (encoded, not suggested)

1. Không nguồn dữ liệu thật → giữ 0 + decision log lý do, không bịa số.
2. `Gate == nil` → coi như dry-run.
3. Kill switch thắng mọi tick.
4. TikTok draft-only (chưa audit Direct Post); YouTube private-first/fail-closed.
5. Mọi lượt đăng YouTube gắn cờ AI-generated (`containsSyntheticMedia`) — bắt buộc.
6. Không hứa "đảm bảo 100%" điều không chắc (thuật toán phân phối viral...).

## 8. Language decision record (2026-10-01, executed as v0.5-go)

Go được chọn (Ninh giao cho Milo quyết): 1 binary, pure Go, cấm cgo mới,
cấm runtime Python/Node đi kèm. Sidecar chỉ theo mẫu uv-managed đã có.

## 9. What this version does NOT do

- ❌ Live / livestream / avatar (parked).
- ❌ Phim điện ảnh 30–60 phút, Veo, realism upgrades (parked).
- ❌ TikTok Shop API — đã loại bỏ hoàn toàn ở đợt H1 (Accesstrade-only).
- ❌ TikTok đăng trực tiếp (cần app audit — Ninh từ chối) → chỉ nháp.
- ❌ Key trả phí / billing / Veo (luật cứng free-only).

## 10. Affiliate video pipeline (đang chạy)

`internal/studio/studio.go` `runAffiliate`: director LLM viết shot list +
khóa 1 địa điểm → Gemini vẽ 3–5 ảnh (identity/product lock bằng ảnh tham
chiếu, upscale 4K) → `AssemblePhotoList`/`AssembleBeatBounce` (Ken Burns 9:16)
→ `MuxMusic` (nhạc bản quyền, loudnorm −14 LUFS) → caption tiếng Việt →
`fireOnDone` → tự đăng TikTok **nháp** nếu bật. Không chữ, không voiceover.
