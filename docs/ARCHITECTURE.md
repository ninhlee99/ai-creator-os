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
   Media rendering uses FFmpeg as an external tool. No local LLM/TTS model
   server runs as part of the app.
3. **Cloud API first, paid never.** LLM/TTS and image generation use configured
   provider APIs; **paid billing is off** (luật cứng của Ninh: key free-only,
   không billing, không Veo). Hỗ trợ thêm Web AI Multi-Account Hub (ChatGPT, Claude, Gemini,
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
│ engines: LLM Gemini                                          │
│ TTS chain: Gemini TTS → Edge                                 │
│ Web AI pool (ChatGPT/Claude/Grok...)                         │
│ Keyring xoay vòng, cooldown 60s→5m→15m khi 429              │
│ Daily Model Auto-Updater (quét 24h & hot-swap model mới)    │
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

**Đợt H (2026-10-04) & Đợt I (2026-10-19 — Chuẩn Video Có Hồn & Quay Phim Tự Nhiên):**
- **I1: Open-Spec Chuẩn Video Có Hồn (`docs/OPEN_SPEC.md`)**: Ban hành đặc tả kỹ thuật Open-Spec v1.0 định nghĩa 7 luật video có hồn, chống méo hình học, loại bỏ hoàn toàn video dán ảnh tĩnh và giật lắc cơ học.
- **I2: Video tham chiếu tùy chọn**: `/viral/create` nhận link TikTok/Douyin trực tiếp làm tham chiếu chuyển động. Danh sách video preset và endpoint import clip riêng đã gỡ; trang không hiển thị radar video realtime.
- **I3: InsightFace Deep Latent Identity Engine (`scripts/face_swapper.py`)**: Hoán đổi khuôn mặt AI với vector đặc trưng 512 chiều, căn chỉnh 5 điểm mốc nhân trắc học và hòa trộn Poisson Seamless Blending để ghép khuôn mặt creator vào thân hình vũ công hoặc người mẫu lookbook trong video chuyển động thực tế.
- **I4: Phim Lookbook Thời Trang Chuyển Động Người Thật (`internal/studio/affiliate.go`)**: Video affiliate thời trang/phụ kiện sử dụng 100% video quay người mẫu thật trong không gian thực tế (boutique, phố, dạo chơi), loại bỏ slide ảnh tĩnh thô kệch và thẻ chữ `AdCard` che khuất hình ảnh.
- **I5: Dựng affiliate ổn định (`internal/studio/assemble.go`)**: Mặc định nối footage chuyển động thật; ảnh tĩnh chỉ dùng dissolve. Không flash, zoom giả hoặc rung.
- **I6: Rạp Chiếu Truyện Audio Điện Ảnh 30 – 60 Phút (`internal/studio/story.go`)**: Xóa bỏ hoàn toàn hiện tượng màn hình đen; mỗi 8-15 giây là một visual canvas chuyển động giàu cảm xúc kết hợp hòa âm 3 lớp (Gemini TTS đa cảm xúc + Foley môi trường + Nhạc nền cinematic lót -18dB).

**Đợt K (2026-10-20 — Google Gemini On-Demand Model Updater Cho Toàn Bộ Adapter & Full Cloud Optimization):**
- **K1: On-Demand Model Updater (`internal/engines/gmodels/updater.go`)**: Cập nhật theo yêu cầu người dùng bằng nút bấm trên giao diện web Cài đặt · Hệ thống (loại bỏ tiến trình định kỳ 24h ngầm để tiết kiệm tài nguyên). Thuật toán `PickLatest` phân tích ngữ nghĩa phiên bản (`major.minor`), ưu tiên bản chính thức (GA) so với Preview/Exp, nâng cấp đồng loạt 4 adapter: Text (`gemini-3.8-flash`), Image (`gemini-nano-banana-2.1`), TTS (`gemini-3.8-flash-tts`), Video (`veo-3.1-generate-preview`).
- **K2: Hot-Swap Không Downtime & Cache Bền Vững**: Model mới phát hiện được cập nhật trực tiếp vào bộ nhớ luồng an toàn mà không cần khởi động lại máy chủ. Lưu vào `data/discovered_models.json` và tự nạp lại khi boot.
- **K3: Giao Diện Quản Trị Tinh Gọn (`internal/web/templates/settings/he-thong.html`)**: Thẻ điều khiển Gemini Model hiển thị trạng thái quét, danh sách model kích hoạt cho 4 tác vụ, nút `⚡ Cập Nhật Tất Cả Model Gemini` (`POST /settings/models/update`), loại bỏ hoàn toàn các hướng dẫn dài dòng gây rối mắt.
- **K4: Chuẩn Hóa Thời Lượng & Bỏ Màn Hình Đen 5s**:
  * Video Affiliate: Tối thiểu 30s – 60s, giữ nhất quán nhân vật và sản phẩm.
  * Video Viral: Tối thiểu 60s+ (chuẩn TikTok Creativity Program), 0.0s instant start không có ảnh đen 5s đầu, visual "Thần tiên tỉ tỉ".
  * Video Truyện Audio: 30 - 60 phút, kịch bản chân thật, đa cảm xúc với Gemini TTS.

**Đợt J (2026-10-19 — Nâng Cấp Generative AI Video Engine: Google Veo 3 & Muse AI):**
- **J1: Google Veo 3 Engine (`internal/studio/veo.go`)**: Tích hợp trực tiếp Google Veo 3 qua REST API (`generativelanguage.googleapis.com/v1beta`) với cơ chế fallback tự động theo danh sách ưu tiên: `veo-3.1-generate-preview`, `veo-3.0-generate-preview`, `veo-2.0-generate-001`, `veo-3.0-generate-001`, `veo-2.0-generate-preview`. Hỗ trợ cả Text-to-Video và Image-to-Video (`firstFrame`), xử lý long-running operation polling bất đồng bộ và tải video MP4 trực tiếp.
- **J2: Muse AI Studio Engine (`internal/studio/museai.go`)**: Tích hợp Muse AI API (`https://muse.ai/api`), tự động nhận diện `MUSE_AI_API_KEY` hoặc session cookie `MUSE_AI_SESSION` qua Web AI Pool, tải ảnh keyframe lên storage và sinh video chuyển động độ nét cao.
- **J3: Unified AI Video Engine & Intelligent Cascade (`internal/studio/aivideo.go`)**: `GenerateMotionVideoClip` điều phối Veo/Muse và chuẩn hóa clip. Pipeline affiliate loại bỏ fallback Ken Burns; nếu không có footage chuyển động thật, job dừng có lỗi.
- **J4: Nâng Cấp Pipeline Sản Xuất Viral & Affiliate**:
  - `internal/studio/viral.go`: Xóa bỏ hoàn toàn mô hình slideshow 5 ảnh tĩnh; từng phân cảnh kịch bản được sinh thành video chuyển động thực tế qua Veo 3 / Muse AI và ghép nối mượt mà (`ConcatClips`).
  - `internal/studio/affiliate.go`: Tích hợp chế độ sinh cảnh người mẫu tương tác và trải nghiệm sản phẩm thật bằng Veo 3 / Muse AI thay vì chỉ zoom ảnh tĩnh.
  - Giao diện người dùng (`templates/viral.html`, `templates/studio.html`): Bổ sung lựa chọn AI Video Engine trực quan (Google Veo 3, Muse AI, Auto Cascade, Lookbook).
- **H1**: loại bỏ TikTok Shop hoàn toàn (Accesstrade-only) — xóa
  `internal/tiktok/shop.go`, `TikTokShopProvider`, wiring trong main.go.
- **H2**: Agent Team định nghĩa lại quanh 3 pipeline — `/team` mới
  (`internal/web/team_pipeline.go`, không còn park): Affiliate, Video Viral AI, Story.
- **H3**: YouTube Analytics API (`internal/growth/youtube_analytics.go`,
  free) điền Views30d + watch hours 30 ngày cho growth thresholds.

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

`internal/engines/chains.go` — provider mặc định:
- **LLM**: Gemini API.
- **TTS**: Gemini → Edge.
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
  tài sản gốc 100% (Gemini MediaGen + Gemini/Edge TTS + Kworb VN music).
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
cấm runtime Python/Node đi kèm.

## 9. What this version does NOT do

- ❌ Live / livestream / avatar (parked).
- ❌ Phim điện ảnh 30–60 phút, Veo, realism upgrades (parked).
- ❌ TikTok Shop API — đã loại bỏ hoàn toàn ở đợt H1 (Accesstrade-only).
- ❌ TikTok đăng trực tiếp (cần app audit — Ninh từ chối) → chỉ nháp.
- ❌ Key trả phí / billing / Veo (luật cứng free-only).

## 10. Affiliate video pipeline (đang chạy)

`internal/studio/studio.go` `runAffiliate`: mặc định director viết shot list +
keyframe → engine video tạo footage chuyển động thật (identity/product lock
bằng ảnh tham chiếu) → nối shot → `MuxMusic` → caption tiếng Việt →
`fireOnDone` → tự đăng TikTok **nháp** nếu bật. Prompt cấm zoom, flash,
flicker và rung; nếu engine không tạo được footage, job dừng có lỗi. Chế độ
ảnh tĩnh riêng dùng dissolve ổn định. Không chữ, không voiceover.
