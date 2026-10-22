# Daemon tự động (`internal/automation`)

## 1. Mục đích
Lớp vận hành nền: growth, đối soát hoa hồng và các tick lịch. Autopilot affiliate
chung dừng trước sản xuất vì đường clip thủ công cần operator. AT Hunter có nhánh
Google Flow riêng khi bật xác nhận credits; nhánh đó không kiểm chứng chi phí bằng
0, nên không đạt chính sách free-only và mặc định phải để tắt.

## 2. Kích hoạt (cadence thật trong `cmd/aicos/main.go`)
| Tick | Chu kỳ | Hàm |
|---|---|---|
| Autopilot affiliate | tick kiểm tra **mỗi phút**; production fail-closed theo free-only trước khi tìm hoặc claim sản phẩm | `Service.AutopilotTick` (`autopilot.go`) |
| Growth (sản xuất + đăng) | **mỗi 5 phút** | `Service.GrowthTick(ctx, false)` (`growth_tick.go`) |
| Đồng bộ số liệu + đối soát tiền | **mỗi giờ** (nằm trong GrowthTick khi `growthSyncDue()`) | `SyncOneAccount`, `ReconcileCommissions` (`sync_reconcile.go`) |
| Accesstrade (hunter/order/campaign) | **mỗi 5 phút** (hunter daily, order sync 30′, campaign check daily) | `Service.ATTick` (`at_tick.go`; đợt C) |
| Viral radar | **mỗi 5 phút**, đến hạn **24 giờ**; chỉ tìm và lưu ứng viên, không tạo video | `viral.auto_produce_enabled` (`Service.ViralRadarTick`) |
| Kể chuyện | **mỗi 5 phút**, đến hạn **24 giờ** (tự đăng private-first; tắt `story.auto_publish` để duyệt tay) | `Service.StoryTick` (`story_tick.go`; đợt F, K) |
| SEO + thumbnail khi đăng truyện | trong lúc đăng | LLM viết title/desc/tags tiếng Việt (thiếu LLM → template), ảnh cảnh đầu làm thumbnail (`story_tick.go`, `publishers.VideoMeta`; đợt M1) |
| Tự chữa lành (self-heal) | **mỗi giờ** (nằm trong story tick) | `Service.SelfHealTick` (`selfheal.go`; đợt L): quét job treo >12h → failed; đĩa >85% → dọn file tạm job lỗi + video đã đăng cũ |
| Tự sao lưu | **mỗi ngày** (nằm trong story tick) | `Service.BackupTick` (`backup_tick.go`; đợt M2): zip data dir vào `backups/`, giữ 7 bản mới nhất |
| Tự chuyển public | **mỗi 5 phút** (nằm trong story tick) | `Service.StoryPublicTick` (`story_tick.go`; đợt N): video private quá `story.public_after_hours` giờ → public qua `videos.update`. Mặc định 0 = tắt |
| Tự khởi động app | không cấu hình sẵn | App chỉ chạy khi người dùng mở thủ công; startup service do người dùng tự cấu hình |
| Thông báo Telegram | **mỗi 5 phút** (nằm trong story tick) | `Service.NotifyTick` (`notify_tick.go`; đợt O2): quét alert warn+ mới → Telegram (watermark nên mỗi alert gửi đúng 1 lần). Cấu hình ở Cài đặt · Hệ thống |
| Tự thử lại job lỗi | **mỗi 5 phút** (nằm trong story tick) | `Service.StoryRetryTick` (`story_retry.go`; đợt P): job story lỗi → job mới cùng params (cooldown 6h, tối đa 3 lần). Cấu hình ở trang Kể chuyện |
| Daemon mạng (onboarding, topic research) | **mỗi 60 giây** | `network.Daemon.Tick` |
| Nút "chạy ngay" trên UI | thủ công | `GrowthTick(ctx, force=true)` — vẫn bị kill + dry-run chặn |

> 🅿️ Live đã park: `network.Daemon` vẫn tick 60s nhưng **không nối
> `OnStartLive`** (chỉ test mới nối) → không mở live thật. Slot live
> (`live_slots`) không còn được build.

Mọi tick: **kill switch + dry-run chặn đầu**; công tắc riêng (unset = **BẬT**,
zero-touch); store/key nil → bỏ qua im lặng (AT log 1 dòng, không spam alert);
watermark last-run trong ledger settings; interval ≤ 0 → default. Trạng thái
thật hiển thị ở `/viral`, `/settings/accesstrade`, `/stories` — chỗ
nào "chờ số liệu"/"chờ key"/"chờ OAuth" phải nói rõ, không giả vờ đang chạy.
Chi tiết bảng công tắc: `docs/ARCHITECTURE.md` §5b.

`internal/automation` không import `web`: mọi phụ thuộc qua interface
(`Producer`, `Uploader`, `AccountManager`, `EnvProvider`, `Settings`).

## 3. Luồng vận hành chi tiết

### 3.1. `AutopilotTick` — chu kỳ affiliate (`autopilot.go`)
1. Chặn đầu: kill switch hoặc dry-run → return (không làm gì).
2. `AutopilotEnabled(settings)` — unset = ON (zero-touch), `"0"` = tắt.
3. Chưa đủ `autopilot_interval_hours` từ `autopilot_last_run` → return.
4. `Autopilot.RunAll(ctx)` dừng từng account đã bật autopilot với lỗi free-only
   vì tick không có đầu vào clip chuyển động.
5. Dừng trước khi tìm sản phẩm, claim usage hoặc tạo job; lỗi một account không
   chặn account khác. Dùng Studio thủ công để chọn sản phẩm và tải clip.

### 3.2. `GrowthTick` — sản xuất + đăng (`growth_tick.go`)
1. `s.killed()` → return ["Kill switch đang bật — vòng growth đứng yên."];
   `!force && !productionEnabled()` (key `growth.production_enabled`) → return.
2. **Sync giờ** (khi `growth.last_sync` quá 1 giờ):
   - `ReconcileCommissions` (mục 3.4) → tiền thật.
   - `SyncOneAccount` cho từng tài khoản (mục 3.3) → số liệu + quyết định.
   - Ghi `growth.last_sync`.
3. Với mỗi tài khoản (bỏ qua `paused`/`penalized`/`retired`):
   - `produceAccount`: `EnsureProfile` → `PlannedItemsDue(accountID, today, 60)`;
     với mỗi item tới hạn (đã trừ `VariantLagDays`):
     - Quá hạn **14 ngày** (`StaleItemDays`) → `dropped` + decision + alert.
     - Tối đa **3 job/tick** (`MaxProductionsPerTick`).
     - `dedupCheck`: concept trùng ≥80% (Jaccard) với nội dung 45 ngày qua
       (`DedupWindowDays`) → tự đổi góc kể (`ApplyAngle`), ghi decision + alert.
     - Dry-run → chỉ log "DRY-RUN — chưa sản xuất thật".
     - `Producer.Enqueue` → `studioGrowthProducer.Enqueue` (`growth_produce.go:40`):
       tài khoản có theme → `Autopilot.Run` (affiliate 30s);
       tài khoản persona → **fail-closed** "pipeline kể chuyện YouTube chưa có
       (đang phát triển ở đợt F)" — item được bỏ qua.
     - Lỗi enqueue → `NoteItemFailure` + alert warn (thử lại tick sau).
   - `advanceAccount`: item `producing` → hỏi `JobState(jobID)`:
     `done` → `produced` (TikTok: ghi chú "chờ đăng nháp"); `failed` → `failed`;
     job biến mất → `failed` ("không tìm thấy Studio job").
     Item `produced`/`waiting_connect`/`waiting_quota` (trừ TikTok — draft-only
     trước audit Direct Post) → `publishItem`:
     - `Uploader.State(a)` chưa đủ (thiếu `YOUTUBE_CLIENT_ID`/`SECRET` hoặc token)
       → `waiting_connect` + ghi chú thiếu gì.
     - Quota: reserve nguyên tử 1 slot trong bucket riêng `videos.insert`
       (mặc định `100` calls/ngày, reset Pacific). Hết → `waiting_quota` +
       alert + decision `defer_quota`; lỗi quota từ Google khóa phần còn lại
       của ngày reset.
     - Dry-run → giữ `produced` + note "DRY-RUN: đã render xong — chưa tải lên".
     - Upload thật: `publishers.YouTubePublisher.Publish` (resumable upload,
       `containsSyntheticMedia` bắt buộc) → reservation giữ lại khi request đã
       được gọi; lỗi trước request thì trả slot → `SetItemPublished(videoID)`
       + decision; Short ↔ bản dài nối hai chiều (`LinkRelatedItems`).

### 3.3. `SyncOneAccount` — đồng bộ + vòng quyết định (`sync_reconcile.go`)
1. `EnsureProfile`; nguồn metrics: `growth.NewYouTubeSource(YOUTUBE_API_KEY)` +
   `growth.TikTokSource{}` → `RecordSnapshot` → bảng `metric_snapshots`.
   Nguồn lỗi → note "chưa kết nối <nguồn>", không bịa số.
2. Followers mới → `Ledger.SetFollowers` + `MarkTargetsReached` → decision
   `growth_analyst/milestone_reached`.
3. `Growth.Evaluate(accountID, username, LoadThresholds(settings), now)` —
   ngưỡng UI (hỏng → fallback default, xem `channel-growth.md`) → các action
   (kill/double-down/breakout/penalty/stalled); chạm kill → tự
   `Accounts.Transition(id, "paused")` + note "ĐÃ TỰ TẠM DỪNG".

### 3.4. `ReconcileCommissions` — đối soát tiền thật (`sync_reconcile.go`)
> **Đợt H1 (2026-10-04):** TikTok Shop đã loại bỏ hoàn toàn. Đối soát hoa
> hồng production chạy qua **Accesstrade** (`ATOrderSyncTick` →
> `/at/orders/sync`, upsert vào `accesstrade.db`). `commissionSource()`
> legacy luôn fail-closed với lý do trung thực; `CommissionSource` giữ lại
> làm điểm tiêm cho test.
1. `commissionSource()`: thiếu key/token (đọc live qua `EnvProvider` mỗi lần gọi)
   **hoặc** endpoint `affiliate_orders_search` chưa xác thực → fail-closed: ghi decision
   `system/commission_reconcile` ("no_source"/"error") **một lần mỗi lần đổi trạng
   thái** (không spam tick giờ), số tiền giữ 0.
2. Có nguồn: `AffiliateOrders(24h qua, page 1, 50)` → với mỗi đơn:
   `Ledger.RecordOrder(external_id, ...)` — **idempotent** theo `external_id`
   (đơn cũ trả `rid=0`, không ghi trùng); đơn mới → cộng dồn commission.
3. Tổng commission mới > 0 → `Ledger.RecordCommission(tháng, tổng, "tiktok_shop_api")`
   (append-only). Trả note "+N đơn, X ₫ hoa hồng đã ghi vào sổ."

### 3.5. `AutoPublishAffiliate` — tự đăng TikTok (`autopilot.go`)
Hook `SetOnDone` của Studio gọi nền khi job affiliate xong (`AutoPublishHook`):
kill/dry-run → bỏ; job không phải affiliate / job tay (không `AccountID`) /
toggle `autopilot_auto_publish` tắt / thiếu TikTok OAuth → bỏ qua + ghi job log.
Đăng qua `publishers.BuildPublishers` → TikTok **dạng NHÁP** (draft) — Ninh gắn
giỏ hàng + sound trong app TikTok rồi mới public (giới hạn API TikTok, không tự
gắn giỏ được).

### 3.6. `ATHunterTick` — săn hàng và nhánh Google Flow tùy chọn (`at_hunter_tick.go`)
1. Hunter đọc AccessTrade datafeed, lọc campaign đã duyệt và link affiliate hợp
   lệ, rồi lưu sản phẩm. Datafeed parser/mapper hiện không có rating/sold count;
   không thể khẳng định “bán chạy/đánh giá tốt”.
2. `at.hunter_flow_credits` mặc định tắt. Khi tắt, tick săn/lưu sản phẩm nhưng
   đặt số video cần tạo bằng 0.
3. Khi bật và health checks qua, Hunter tạo shot plan rồi sinh clip từng shot
   bằng Google Flow. Code xác nhận credits, không kiểm tra chi phí/quota bằng 0.
4. Theo yêu cầu chỉ miễn phí, không bật nhánh này cho tới khi provider xác nhận
   rõ lượt dùng không phát sinh billing. Đây không phải fallback account miễn
   phí; code không tự đo hoặc đổi account khi hết quota.

## 4. Fail-closed & an toàn
- Kill switch thắng mọi tick, kiểm tra **đầu** mỗi hàm.
- `Gate == nil` → dry-run (an toàn nhất).
- Mọi lỗi nguồn/metric/upload → giữ trạng thái cũ + note/alert/decision tiếng Việt,
  không giả thành công.
- Tiền: không nguồn → giữ 0 + decision lý do; đơn idempotent theo external_id.

## 5. API key rotation
- Sản xuất (Studio): `GEMINI_API_KEYS` keyring xoay vòng (xem `key-rotation.md`).
- Metrics YouTube: `YOUTUBE_API_KEY` (key đơn).
- Đăng YouTube: refresh token từng tài khoản, tự refresh qua `oauth2.googleapis.com/token`.

## 6. Giám sát tick — chạy 24/7 không người trông (Đợt R+S)

### 6.1. Tick supervisor (`internal/ticksup`, wiring ở `cmd/aicos/main.go`)
Mỗi tick chạy trong goroutine riêng qua `Supervisor.Dispatch`:
- **recover**: panic trong tick → recover, ghi alert critical (`tick_panic`), tick chạy lại vòng sau. Process và dashboard không chết theo.
- **timeout**: story 30′, growth 20′, còn lại 5–15′ — treo quá hạn thì context bị hủy (tick fail, thử lại vòng sau).
- **bỏ qua khi kẹt**: lượt trước chưa xong → bỏ qua lượt mới, không dồn goroutine.
- **heartbeat**: mỗi tick ghi `Beat{LastStart, LastOK, LastErr, Runs}` — đọc ở `GET /healthz` và Cài đặt · Hệ thống ("Tự động nền").
- Trước đây 6 tick story chạy nối tiếp: StoryTick treo là self-heal + backup + Telegram alert đứng im theo. Giờ đã tách độc lập.

### 6.2. Health endpoint (`GET /healthz`, `internal/web/healthz.go`)
JSON: `ok`, `version`, `uptime_sec`, `db_ok` (ping SQLite), `ticks` (heartbeat), `quota_circuit_open/until`.
Luôn trả 200 (kể cả db_ok=false) để phân biệt "app treo" (không kết nối được) với "app sống nhưng DB lỗi".
Không qua cổng onboard. Endpoint chỉ hoạt động khi người dùng đã mở app.

### 6.3. Ngắt mạch quota ngày (`internal/engines/tts/quota_circuit.go`)
KeyRing backoff tối đa 15′ và chỉ trong bộ nhớ — hết quota ngày thì máy vẫn xoay key đốt API call vô vọng.
Circuit đếm 3 lần liên tiếp "mọi key cooldown" → mở mạch tới 0h đêm (giờ địa phương):
- các Gemini tier (LLM/TTS/image) fail-fast ngay, chain chuyển sang tier local như bình thường;
- 1 API call thành công (kể cả nút "test" ở Cài đặt) → đóng mạch;
- mở mạch → đúng 1 alert warn (`quota_circuit`) tới dashboard + Telegram.
Trạng thái hiện ở Cài đặt · Hệ thống ("Ngắt mạch quota Gemini").

### 6.4. Single-instance (`internal/lockfile`, `cmd/aicos/main.go`)
`flock` file `<dataDir>/aicos.lock` ngay đầu main, giữ suốt thời gian process sống.
OS tự giải phóng khi crash nên không kẹt khóa "ma". Instance thứ hai → thoát êm với log rõ ràng, không giành SQLite với instance đang chạy.

### 6.5. Xoay log trong app (`internal/logx`, `cmd/aicos/main.go`)
App tự ghi `<dataDir>/logs/aicos.log` và xoay theo dung lượng (20MB, giữ 7 bản).
App tự quản lý file log nên không cần cấu hình log riêng trong hệ điều hành.

### 6.6. Cứng rắn thêm (cùng đợt)
- **Backup** (`internal/backup/backup.go`): checkpoint WAL trước khi copy (ảnh chụp nhất quán); đủ 4 DB
  (`ledger/studio/products/accesstrade`).
- **Sweep** (`internal/studio/sweep.go`): job treo → `CancelRunning` kill ffmpeg/ffprobe thật trước khi đánh
  failed (trước đây process treo vẫn ngốn CPU/đĩa và giữ slot render).
- **Web DB** (`internal/web/server.go`): `SetMaxOpenConns(1)` — `busy_timeout` trước đây chỉ tác dụng trên 1
  connection trong pool → dashboard thỉnh thoảng `SQLITE_BUSY`.
- **Đăng truyện** (`internal/automation/story_tick.go`): marker `story.publishing.<jobID>` đặt trước upload,
  xóa khi xong/lỗi — crash giữa upload và marker để lại dấu vết thay vì im lặng đăng trùng.
- **HTTP timeouts**: `ReadTimeout 15s / WriteTimeout 60s / IdleTimeout 120s` trên máy 24/7.
- **Windows**: `DiskUsagePct` tách build tag (`diskusage_unix.go`/`diskusage_windows.go`) — trước đây
  `syscall.Statfs` làm gãy build Windows, đi ngược claim "1 binary / 1 nền tảng".
