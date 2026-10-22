# Phát triển kênh (`/growth`)

## 1. Mục đích
Máy tăng trưởng kênh: sinh kế hoạch nội dung 30 ngày, đánh giá số liệu theo
ngưỡng, và tự tạm dừng tài khoản khi chạm luật kill. Tự sản xuất affiliate
đang chờ clip chuyển động do người vận hành cung cấp theo chính sách free-only.

## 2. Kích hoạt
- `GET /growth` → `handleGrowth` (`internal/web/growth.go:238`): tổng quan
  (profile, plan items, alerts, thresholds hiện tại).
- Nguồn số liệu (`SyncOneAccount`): `youtube_api` (Data API — lifetime totals)
  + `youtube_analytics` (Đợt H3 — views/watch hours/subs 30 ngày, cần token
  OAuth scope `yt-analytics.readonly`, badge trạng thái ở trang Kênh) +
  `tiktok_api`. Thiếu kết nối → fail-closed, báo rõ trong notes.
- `POST /growth/sync` → `handleGrowthSync`: đồng bộ số liệu + vòng quyết định ngay
  cho mọi tài khoản (gọi `syncOneAccount` — xem `autonomous-daemon.md`).
- `Harness.RunCycle` đọc latest per-video view readings vừa đồng bộ trong 7 ngày từ
  `growth.video_metric_snapshots`: số video có reading, average views và giờ đo.
  Đây là số video có metrics mới, không phải số video đăng. Không có data/error
  được ghi trạng thái riêng; OODA không suy luận views khi thiếu reading.
- `POST /growth/accounts/{id}/plan` → `handleGrowthPlan`: sinh lại kế hoạch 30 ngày.
- `POST /growth/production` → `handleGrowthProductionToggle`: bật/tắt sản xuất tự động
  (key `growth.production_enabled`; unset = ON, `"0"` = tắt). Bật công tắc không
  bỏ qua yêu cầu clip chuyển động của Studio.
- `POST /growth/production/run` → `handleGrowthProductionRun`: chạy ngay
  `GrowthTick(ctx, force=true)` (vẫn bị kill switch + dry-run chặn).
- `POST /growth/thresholds` → `handleGrowthThresholds` (`growth_produce.go:217`):
  lưu 7 ngưỡng + nút reset về mặc định.
- Tự động: daemon tick **5 phút** gọi `GrowthTick(ctx, false)` (xem `autonomous-daemon.md`).

## 3. Luồng vận hành chi tiết

### Sinh kế hoạch 30 ngày
`generatePlanFor` (`internal/web/growth.go:282`):
1. `s.Growth.EnsureProfile(accountID, platform, hasYouTube)` → `growth_profiles`
   (giai đoạn: `cold_start` → `format_testing` → `scaling` → `monetization_push`
   → `monetized`; `stalled` khi chững).
2. `growth.GeneratePlan(PlanRequest{Stage, HasYouTube, Pillars, Start, Days: 30})`
   → danh sách plan items (mỗi item: topic, variant, format, planned_for).
   Pillars lấy từ persona (`network.PERSONAS`) hoặc niche.
3. `s.Growth.InsertPlan(accountID, "d30", rationale, drafts)` → bảng `plan_items`
   (status `planned`). Ghi decision `growth_director/generate_plan`.
4. Tự sinh khi tạo tài khoản / đổi theme: `autoGeneratePlan` (best-effort, lỗi chỉ log).

### Biến thể đa nền tảng (một ý tưởng → 3 bản)
`internal/growth/production.go`:
| Variant | Thời lượng | Khổ | Độ trễ lịch |
|---|---|---|---|
| `tiktok` | 60s | 9:16 | +0 ngày |
| `youtube_shorts` | 45s | 9:16 | +1 ngày (`VariantLagDays`) |
| `youtube_long` | 150s | **16:9** (`VariantAspect`) | +3 ngày |

Mỗi variant là một Studio job + file riêng. `ConceptHash` (sha256, gấp dấu tiếng Việt)
+ Jaccard ≥ 0.8 → ép đổi góc kể (`dedupCheck` trong tick) để không trùng nội dung
giữa các tài khoản trong 45 ngày (`DedupWindowDays`).

### Ngưỡng governance (UI chỉnh được)
`automation.LoadThresholds` đọc `growth.thresholds` (JSON) phủ lên
`growth.DefaultConfig()` — hỏng từng trường thì fallback về default:
mặc định kill khi ≥8 video + ≥14 ngày + completion <35% (hoặc share/save <1%);
double-down khi gấp 5× median; breakout khi 10×; penalty khi views rơi >70%;
stalled khi 14 ngày tăng trưởng <2%. Vượt luật → `Evaluate` đề xuất hành động;
`SyncOneAccount` tự `Transition(id, "paused")` khi chạm kill.

> **Chiến lược Zero-Reup**: Toàn bộ video sản xuất tuân thủ nguyên tắc 100% tài sản gốc, tránh thuật toán phạt 0-view shadowban.

### Sản xuất & đăng (trong tick)
Video affiliate tự động hiện fail-closed đến khi có luồng cung cấp clip cho tick;
không tạo job, không đăng và không đánh dấu mục đã sản xuất. Phần governance và
đồng bộ số liệu vẫn chạy. Xem `autonomous-daemon.md` — item quá hạn 14 ngày → `dropped`; tối đa
**3 job/tài khoản/tick**; TikTok chỉ tới `produced` (draft-only trước audit Direct Post);
YouTube đăng khi đủ OAuth + qua quota guard trong code (`1` unit/lượt, mặc
định `100` lượt/ngày — `internal/growth/quota.go`), dùng reservation nguyên tử
và ngày reset Pacific. Google Cloud Console vẫn là nguồn chính xác cho giới
hạn project đã cấp. Thiếu gì thì item ở `waiting_connect`/`waiting_quota`
với ghi chú tiếng Việt trung thực. Mọi lượt đăng gắn cờ AI-generated
(`containsSyntheticMedia`, `Synthetic=true` — bắt buộc).

> Sản xuất affiliate tự động hiện dừng fail-closed vì Growth chưa nhận clip
> chuyển động do người vận hành tải lên. Mục kế hoạch không được đánh dấu
> hoàn thành hoặc đăng khi thiếu video/QC.
>
> Tài khoản **persona** (không có theme): pipeline phim đã park nên
> `studioGrowthProducer.Enqueue` hiện **fail-closed** với lỗi trung thực
> "tài khoản persona: pipeline kể chuyện YouTube chưa có (đang phát triển ở
> đợt F) — item được bỏ qua" (`internal/web/growth_produce.go:55`).

## 4. Fail-closed & an toàn
- Không backend sản xuất → item giữ `planned` + alert "backend chưa được nối", tick sau thử lại.
- Không OAuth/quota → `waiting_connect`/`waiting_quota`, không giả đăng.
- Nút "chạy ngay" (`force=true`) vẫn bị kill switch + dry-run chặn.
- Số liệu nguồn lỗi → giữ snapshot cũ, decision ghi lý do, không bịa số.

## 5. API key rotation
- Sync metrics YouTube: `YOUTUBE_API_KEY` (Data API v3, key đơn — không xoay vòng).
- Các bước phân tích/shot plan qua Studio → `GEMINI_API_KEYS` keyring xoay vòng (xem `key-rotation.md`); video API trả phí bị tắt.
- Đăng YouTube: OAuth refresh token từng tài khoản (`<data>/tokens/youtube_token_<user>.json`),
  tự refresh qua `oauth2.googleapis.com/token` khi hết hạn.
