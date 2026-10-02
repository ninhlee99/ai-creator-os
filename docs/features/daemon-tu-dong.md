# Daemon tự động (`internal/automation`)

## 1. Mục đích
Lớp vận hành hands-off: growth (kế hoạch → sản xuất → đăng), autopilot affiliate,
đối soát hoa hồng — chạy nền theo tick, không cần bấm gì.

## 2. Kích hoạt (cadence thật trong `cmd/aicos/main.go`)
| Tick | Chu kỳ | Hàm |
|---|---|---|
| Autopilot affiliate | kiểm tra **mỗi phút**, chạy khi đủ `autopilot_interval_hours` (mặc định **6h**) | `Service.AutopilotTick` (`autopilot.go`) |
| Growth (sản xuất + đăng) | **mỗi 5 phút** | `Service.GrowthTick(ctx, false)` (`growth_tick.go`) |
| Đồng bộ số liệu + đối soát tiền | **mỗi giờ** (nằm trong GrowthTick khi `growthSyncDue()`) | `SyncOneAccount`, `ReconcileCommissions` (`sync_reconcile.go`) |
| Live (daemon mạng) | **mỗi 60 giây** | `network.Daemon.Tick` (xem `lich-live.md`) |
| Nút "chạy ngay" trên UI | thủ công | `GrowthTick(ctx, force=true)` — vẫn bị kill + dry-run chặn |

`internal/automation` không import `web`: mọi phụ thuộc qua interface
(`Producer`, `Uploader`, `AccountManager`, `EnvProvider`, `Settings`).

## 3. Luồng vận hành chi tiết

### 3.1. `AutopilotTick` — chu kỳ affiliate (`autopilot.go`)
1. Chặn đầu: kill switch hoặc dry-run → return (không làm gì).
2. `AutopilotEnabled(settings)` — unset = ON (zero-touch Đợt 3), `"0"` = tắt.
3. Chưa đủ `autopilot_interval_hours` từ `autopilot_last_run` → return.
4. `Autopilot.RunAll(ctx)` → với mỗi tài khoản `AutopilotReady`:
   `studio.Autopilot.Run` (`internal/studio/autopilot.go:58`):
   - a. Sản phẩm: `store.TopByTheme(theme, minCommission, accountID, 5)` — có sẵn
     thì dùng; không có → `products.Aggregate` tìm live (tối đa 20), lưu lại.
     Lấy ứng viên đầu (`cands[0]`).
   - b. Tải ≤3 ảnh thật của sản phẩm (`downloadImage`) — ground truth.
   - c. Ảnh mẫu người mẫu của tài khoản (upload tay trong UI — identity lock).
   - d. `CreateAffiliateJob(AffiliateParams{Mode: photo, Seconds: 30, ...})`
     → job affiliate 30s (xem `studio-ai.md` §3.3).
   - e. `RecordUse(prod.ID, accountID, jobID)` — không dùng lại sản phẩm cho
     cùng tài khoản.
   - Thiếu theme/ảnh mẫu/sản phẩm → lỗi tiếng Việt rõ lý do, không tạo job rỗng.
5. Ghi `autopilot_last_run` (RFC3339). Lỗi một tài khoản không chặn tài khoản khác.

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
       tài khoản persona → `CreateFilmJob{Topic, Seconds: VariantSeconds, Aspect}`.
     - Lỗi enqueue → `NoteItemFailure` + alert warn (thử lại tick sau).
   - `advanceAccount`: item `producing` → hỏi `JobState(jobID)`:
     `done` → `produced` (TikTok: ghi chú "chờ đăng nháp"); `failed` → `failed`;
     job biến mất → `failed` ("không tìm thấy Studio job").
     Item `produced`/`waiting_connect`/`waiting_quota` (trừ TikTok — draft-only
     trước audit Direct Post) → `publishItem`:
     - `Uploader.State(a)` chưa đủ (thiếu `YOUTUBE_CLIENT_ID`/`SECRET` hoặc token)
       → `waiting_connect` + ghi chú thiếu gì.
     - Quota: đã dùng `used`/`10000` units, 1 lượt = `1600` → hết → `waiting_quota`
       + alert + decision `defer_quota`.
     - Dry-run → giữ `produced` + note "DRY-RUN: đã render xong — chưa tải lên".
     - Upload thật: `publishers.YouTubePublisher.Publish` (resumable upload,
       `containsSyntheticMedia` bắt buộc) → `AddQuota` + `SetItemPublished(videoID)`
       + decision; Short ↔ bản dài nối hai chiều (`LinkRelatedItems`).

### 3.3. `SyncOneAccount` — đồng bộ + vòng quyết định (`sync_reconcile.go`)
1. `EnsureProfile`; nguồn metrics: `growth.NewYouTubeSource(YOUTUBE_API_KEY)` +
   `growth.TikTokSource{}` → `RecordSnapshot` → bảng `metric_snapshots`.
   Nguồn lỗi → note "chưa kết nối <nguồn>", không bịa số.
2. Followers mới → `Ledger.SetFollowers` + `MarkTargetsReached` → decision
   `growth_analyst/milestone_reached`.
3. `Growth.Evaluate(accountID, username, LoadThresholds(settings), now)` —
   ngưỡng UI (hỏng → fallback default, xem `phat-trien-kenh.md`) → các action
   (kill/double-down/breakout/penalty/stalled); chạm kill → tự
   `Accounts.Transition(id, "paused")` + note "ĐÃ TỰ TẠM DỪNG".

### 3.4. `ReconcileCommissions` — đối soát tiền thật (`sync_reconcile.go`)
1. `commissionSource()`: thiếu `TIKTOK_SHOP_APP_KEY`/`APP_SECRET`/`ACCESS_TOKEN`
   (đọc live qua `EnvProvider` mỗi lần gọi — R2-W4/R2-05) **hoặc** endpoint
   `affiliate_orders_search` chưa xác thực → fail-closed: ghi decision
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

## 4. Fail-closed & an toàn
- Kill switch thắng mọi tick, kiểm tra **đầu** mỗi hàm.
- `Gate == nil` → dry-run (an toàn nhất).
- Mọi lỗi nguồn/metric/upload → giữ trạng thái cũ + note/alert/decision tiếng Việt,
  không giả thành công.
- Tiền: không nguồn → giữ 0 + decision lý do; đơn idempotent theo external_id.

## 5. API key rotation
- Sản xuất (Studio): `GEMINI_API_KEYS` keyring xoay vòng (xem `key-rotation.md`).
- Metrics YouTube: `YOUTUBE_API_KEY` (key đơn).
- TikTok Shop: `TIKTOK_SHOP_APP_KEY/APP_SECRET/ACCESS_TOKEN` đọc live qua EnvProvider.
- Đăng YouTube: refresh token từng tài khoản, tự refresh qua `oauth2.googleapis.com/token`.
