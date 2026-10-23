# MiuNova — Architecture

> Tài liệu kiến trúc code kế thừa; giữ chi tiết implementation và lịch sử.
> Mục tiêu sản phẩm hiện hành: [PRODUCT_SPEC](PRODUCT_SPEC.md). Trạng thái code:
> [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md). Các đoạn dưới đây về mục
> tiêu kinh doanh cũ không ghi đè hai tài liệu chuẩn này.
> **Runtime hiện tại:** viral character replacement bị tắt theo free-only; `/viral` chỉ discovery và từ chối enqueue. Affiliate nhận clip do operator cung cấp hoặc Flow generation có xác nhận credits theo job; chi phí $0 chưa được xác minh. Các đoạn lịch sử bên dưới về Veo/Muse/face swap không phải capability đang bật.

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
   Grok, Muse AI) lưu session được khôi phục từ JSON backup và kiểm tra trạng
   thái qua provider probe.
4. **Deterministic governance over money.** LLM đề xuất; rules engine (Go)
   quyết định. Ngưỡng kill/double-down, quota, kill switch là code, không
   phải prompt.
5. **Provider evidence only.** Doanh thu, đơn hàng, hoa hồng chỉ ghi từ dữ
   liệu thật của provider. Hệ thống không bao giờ bịa số.
6. **Human-gated automation.** Harness đọc metrics/verdict đã lưu; action
   production chưa được hỗ trợ và memory do operator nhập. Autopilot affiliate
   dừng trước product discovery/job creation. Operator vẫn cấu hình nguồn,
   cung cấp media và xác nhận các luồng có thể dùng credits.

## 2. Component map (hiện tại)

```
┌─────────────────────────────────────────────────────────────┐
│ DASHBOARD + WORKFLOW GATES                                   │
│  Điều hành: Tổng quan · Harness Agent 🧠 · Kênh · Đội ngũ   │
│  Sản xuất: Studio AI 🎬 · Affiliate · Viral discovery · Stories │
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
│ read-only   │ │ video jobs  │ │ plan 30d,   │ │ JSON restore │
│ OODA;       │ │ affiliate   │ │ ngưỡng,     │ │ session check│
│ operator    │ │ manual/Flow,│ │ quota,      │ │ cookie update│
│ memory      │ │ viral radar,│ │ YT Analytics│ │ rotation     │
│             │ │ Story 9:16  │ │             │ │              │
└──────┬──────┘ └─────┬───────┘ └─────────────┘ └───┬──────────┘
       │              │                             │
┌──────▼──────────────▼─────────────────────────────▼─────────┐
│ engines: LLM Gemini                                          │
│ TTS chain: local macOS speech → Edge → Gemini                │
│ Web AI pool (ChatGPT/Claude/Grok/Muse/Flow)                   │
│ Keyring xoay vòng, cooldown 60s→5m→15m khi 429              │
│ On-demand Gemini model refresh (không tự quét theo lịch)    │
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
(`internal/accesstrade`), Viral Radar (full character replacement production
currently paused; `internal/studio/viral.go` rejects enqueue), pipeline kể chuyện (`internal/studio/story.go` —
tách từ studio, kind `story` trên `studio_jobs`; automation
`internal/automation/story_tick.go` 24h/lần, đăng private-first, mặc định
chờ Ninh duyệt).

**Đợt H (2026-10-04) & Đợt I (2026-10-19 — Chuẩn Video Có Hồn & Quay Phim Tự Nhiên):**
- **I1: Open-Spec Chuẩn Video Có Hồn (`docs/OPEN_SPEC.md`)**: Ban hành đặc tả kỹ thuật Open-Spec v1.0 định nghĩa 7 luật video có hồn, chống méo hình học, loại bỏ hoàn toàn video dán ảnh tĩnh và giật lắc cơ học.
- **I2: YouTube Viral Radar**: `/viral` quét video 7 ngày gần nhất từ kênh có từ 100,000 subscribers; dùng title và duration làm metadata. Job gốc không tải hoặc tái dùng footage/audio nguồn.
- **I3: Legacy InsightFace face-swap utility (`scripts/face_swapper.py`)**: chỉ đổi khuôn mặt; không phải full-character replacement và không được dùng để chứng nhận viral QC.
- **I4: Affiliate Lookbook (`internal/studio/affiliate.go`)**: Shot plan khóa sản phẩm, nhân vật, cảm xúc và bối cảnh; mỗi shot cần clip chuyển động do người vận hành cung cấp. Studio chuẩn hóa timeline và chặn xuất bản nếu visual QC không đạt. Không gọi video API tính phí; autopilot affiliate tạm dừng vì không thể tự cung cấp clip.
- **I5: Dựng affiliate ổn định (`internal/studio/assemble.go`)**: Mặc định nối footage chuyển động thật; ảnh tĩnh chỉ dùng dissolve. Không flash, zoom giả hoặc rung.
- **I6: Rạp Chiếu Truyện Audio Điện Ảnh 30 – 60 Phút (`internal/studio/story.go`)**: Xóa bỏ hoàn toàn hiện tượng màn hình đen; mỗi 8-15 giây là một visual canvas chuyển động giàu cảm xúc kết hợp hòa âm 3 lớp (Gemini TTS đa cảm xúc + Foley môi trường + Nhạc nền cinematic lót -18dB).

**Đợt K (2026-10-20 — Google Gemini On-Demand Model Updater):**
- **K1: On-Demand Model Updater (`internal/engines/gmodels/updater.go`)**: Cập nhật theo yêu cầu người dùng. `PickLatest` chọn model cho các vai trò được cấu hình. Việc có tên model video trong danh sách không bật video API; billing và Veo vẫn bị cấm theo free-only.
- **K2: Hot-Swap Không Downtime & Cache Bền Vững**: Model mới phát hiện được cập nhật trực tiếp vào bộ nhớ luồng an toàn mà không cần khởi động lại máy chủ. Lưu vào `data/discovered_models.json` và tự nạp lại khi boot.
- **K3: Giao Diện Quản Trị Tinh Gọn (`internal/web/templates/settings/system.html`)**: Thẻ điều khiển Gemini Model hiển thị trạng thái quét, danh sách model kích hoạt cho 4 tác vụ, nút `⚡ Cập Nhật Tất Cả Model Gemini` (`POST /settings/models/update`), loại bỏ hoàn toàn các hướng dẫn dài dòng gây rối mắt.
- **K4: Chuẩn Hóa Thời Lượng & Bỏ Màn Hình Đen 5s**:
  * Video Affiliate: Tối thiểu 30s – 60s, giữ nhất quán nhân vật và sản phẩm.
  * Video Viral: Tối thiểu 60s+ (chuẩn TikTok Creativity Program), 0.0s instant start không có ảnh đen 5s đầu, visual "Thần tiên tỉ tỉ".
  * Video Truyện Audio: 30 - 60 phút, kịch bản chân thật, đa cảm xúc với Gemini TTS.

**Đợt J (2026-10-19 — historical, not active; superseded by free-only policy):**
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
| `GET /` | `handleDashboard` | Trang điều hành với snapshot job, đơn đã sync, heartbeat và cấu hình |
| `/harness*` | `handleHarness*` | OODA đọc metrics/verdict đã lưu; action chưa thực thi, memory do người vận hành nhập |
| `/accounts*` | `handleAccounts*` | Kênh & Brand DNA (sidebar "Kênh") |
| `/growth*` | `handleGrowth*` | Phát triển kênh & ngưỡng tự động |
| `/studio*` | `handleStudio*` | Affiliate, Muse/Flow creation, jobs, media and trends |
| `/products*` | `handleProducts*` | Affiliate (Accesstrade: chiến dịch/link/đối soát, đợt B/C) |
| `/viral*` | `handleViral*` | Viral Radar; character-replacement enqueue bị chặn theo free-only |
| `/stories*` | `handleStory*` | Kể chuyện: tạo truyện, jobs, đăng YouTube (đợt F) |
| `GET /team` | `handleTeamPipelines` | Đội ngũ: sức khỏe 3 pipeline từ dữ liệu thật |
| `/publishers*` | `handlePublishers*` | Đa nền tảng (OAuth TikTok, YouTube, Facebook Page) |
| `/settings*` | `handleSettings*` | 5 trang con: Hệ thống (gồm **Kho Tài Khoản Web AI**) · Accesstrade · Nhà cung cấp · Model local · An toàn |
| `/web-ai*` | `handleWebAI*` | Quản lý account Web AI đã khôi phục, session check và pool |
| `/api/*` | `requireAPI(...)` | Mặc định 404 khi tắt |
| `/onboard` | `handleOnboard` | Wizard lần đầu |
| `/kill`, `/unkill` | `handleKill` | Kill switch |

`/schedule` → 404 (parked). `/team` đã định nghĩa lại ở đợt H2 (không còn park).

## 4. Engines: the provider chains

`internal/engines/chains.go` — provider mặc định:
- **LLM**: Gemini API.
- **TTS**: local macOS speech → Edge → Gemini.
- **Web AI Hub** (`internal/webai`): ChatGPT, Claude, Gemini, Grok, Muse Chat, Google Flow. Accounts are restored from JSON backup; app does not read the user's browser profile. Session validators check imported cookies and persist refreshed cookies by account ID. Muse/Flow use saved web sessions; Muse prefers direct Noise gateway and has hidden CDP fallback before prompt submission; Flow uses observed web-client calls.
- Chuỗi Avatar đã gỡ theo live (Đợt A).

Keyring mỗi engine độc lập: round-robin, bỏ key invalid, cooldown 429/quota
thang 60s→5m→15m, key khoẻ lại reset thang. Failover tier nào cũng ghi
decision log (`engines/tier_failover`).

## 5. Governance (deterministic, Go)

- `MASTER_SWITCH` chỉ gates `network.Daemon`. Automation ticks kiểm tra kill
  switch/dry-run theo từng workflow; dry-run không chặn mọi thao tác thủ công,
  sync hoặc discovery trên toàn app.
- Ngưỡng growth chỉnh được trên UI (`/growth/thresholds`), hỏng từng trường
  → fallback default (`internal/growth`).
- Quota YouTube: `videos.insert` bucket riêng, mặc định 100 lượt/ngày, 1 unit/lượt;
  đếm request nguyên tử theo ngày reset Pacific (`internal/growth/quota.go`).
- Đơn hàng idempotent theo `external_id` (`Ledger.RecordOrder`).
- Zero-Reup: Viral Radar chỉ lưu metadata nguồn; runtime không tạo video thay
  nhân vật và không tải lại footage/audio nguồn. Nếu pipeline sản xuất được bật
  trong tương lai, chỉ xuất bản nội dung đã qua QC và dùng tài sản có quyền sử dụng.
  Không bao giờ tự xoá video đã đăng.

## 5b. Background ticks and gates

| Tick | Chu kỳ | Công tắc (unset = BẬT) | Fail-closed khi thiếu |
|---|---|---|---|
| Network daemon | 60s | `master.switch`, kill switch, dry-run | Live đang park; không nối callback mở live |
| AutopilotTick | 1′ | kill/dry-run, `autopilot_enabled`, cadence | Producer hiện dừng trước product discovery/job creation |
| GrowthTick | 5′ | kill switch, `growth.production_enabled`; dry-run chặn production/publish | Hourly metrics/commission sync vẫn có thể chạy trong dry-run |
| ATTick (hunter/order/campaign) | 5′ | kill/dry-run; từng `at.*_enabled` và cadence | Hunter có thể lưu offer; video cần Flow credits và các gate riêng |
| ViralRadarTick | 5′/đến hạn 24h | kill switch, `viral.auto_produce_enabled` | Read-only discovery; dry-run không chặn bước này; không tạo job video |
| StoryTick | 5′/đến hạn 24h | kill/dry-run, `story.enabled` | Tạo Story mới còn gate theo provider ảnh; có thể xử lý auto-publish job đã xong |

Gate khác nhau theo workflow; không phải mọi tick đều dừng chỉ vì dry-run bật.
Master switch chỉ điều khiển network daemon. Các tick có watermark/cadence riêng;
trạng thái thật hiển thị ở `/viral`, `/settings/accesstrade`, `/stories`.

## 6. Ledger & Data Stores (SQLite, WAL mode)

- `ledger.db`: accounts, decisions, settings, orders, growth_*, api_usage, harness_cycles, harness_memories…
- `studio.db`: studio_jobs, studio_assets, capabilities.
- `products.db`: products, shelf.
- `accesstrade.db`: campaigns, links, orders (đợt B/C).
- `webai.db`: accounts (kho tài khoản Web AI, session tokens, cooldown state, metrics).
- Migration qua `PRAGMA user_version`, có test round-trip.

## 7. Strict operational rules (encoded, not suggested)

1. Không nguồn dữ liệu thật → giữ 0 + decision log lý do, không bịa số.
2. Trong `automation.Service`, `Gate == nil` → production ticks coi như dry-run.
3. Kill switch chặn các workflow có áp dụng gate này; không phải middleware toàn app.
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
- ❌ TikTok Direct Post (đang khóa đến khi eligibility và consent theo từng bài được giải quyết) → adapter chỉ tải draft.
- ❌ Gemini API billing / direct Veo key. Google Flow is a separate web-account integration; check account access and credits before submitting a job.

## 10. Affiliate video pipeline (đang chạy)

`POST /studio/affiliate/plan` chạy Product Analyst và Director trước khi operator
hoàn thiện clip. Plan lưu brief + shot list để form hiển thị prompt và số clip;
`POST /studio/affiliate` tiêu thụ plan, nhận một clip mỗi shot hoặc chọn Flow
với xác nhận credits theo job, rồi `runAffiliate` chuẩn hóa footage, nối shot,
thêm audio do operator cung cấp và chạy temporal + visual QC. Chi phí Flow $0
chưa được xác minh. `fireOnDone` chỉ chạy
hook publish đã cấu hình; hoàn tất render không chứng minh hiệu quả tăng trưởng.
