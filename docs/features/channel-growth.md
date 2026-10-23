# Phát triển kênh (`/growth`)

## 1. Mục đích
Máy tăng trưởng kênh: sinh kế hoạch nội dung 30 ngày, đánh giá số liệu theo
ngưỡng, và tự tạm dừng tài khoản khi chạm luật kill. Growth production áp dụng
variant thời lượng/khổ cho Story job của kênh persona và QC đầu ra. Affiliate
Autopilot vẫn dừng trước discovery vì chưa có xác nhận Flow credits tự động.

## 2. Kích hoạt
- `GET /growth` → `handleGrowth` (`internal/web/growth.go:238`): tổng quan
  (profile, plan items, alerts, thresholds hiện tại).
- Nguồn số liệu (`SyncOneAccount`): `youtube_api` (Data API — lifetime totals)
  + `youtube_analytics` (views/watch hours/subs 30 ngày, cần token OAuth scope
  `yt-analytics.readonly`) + `tiktok_api` stub. Thiếu kết nối → fail-closed,
  báo rõ trong notes. Growth page phân biệt API key + Channel ID đã lưu với
  API đã được kiểm tra.
- `POST /growth/sync` → `handleGrowthSync`: đồng bộ số liệu + vòng quyết định ngay
  cho mọi tài khoản (gọi `syncOneAccount` — xem `autonomous-daemon.md`).
- `Harness.RunCycle` đọc latest per-video view readings vừa đồng bộ trong 7 ngày từ
  `growth.video_metric_snapshots`: số video có reading, average views và giờ đo.
  Đây là số video có metrics mới, không phải số video đăng. Không có data/error
  được ghi trạng thái riêng; OODA không suy luận views khi thiếu reading.
- `POST /growth/accounts/{id}/plan` → `handleGrowthPlan`: sinh lại kế hoạch 30 ngày.
- `POST /growth/production` → `handleGrowthProductionToggle`: lưu trạng thái lịch
  (key `growth.production_enabled`; unset = ON, `"0"` = tắt). Bật lịch không
  chứng minh daemon đang chạy hoặc producer có thể tạo job.
- `POST /growth/production/run` → `handleGrowthProductionRun`: chạy ngay
  `GrowthTick(ctx, force=true)`; có thể tạo job/upload nếu item đến hạn và mọi
  credential, quota, provider, kill switch, dry-run gate đều qua.
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
4. `autoGeneratePlan` tạo plan best-effort khi tạo account hoặc thêm social channel mới. Đổi affiliate theme chỉ đổi bộ lọc sản phẩm; không thay Growth plan. Plan generation failure chỉ được ghi log ở các luồng tự sinh.

### Biến thể đa nền tảng (một ý tưởng → 3 bản)
`internal/growth/production.go`:
| Variant (persona Story target) | Thời lượng mục tiêu | Khổ mục tiêu | Độ trễ lịch |
|---|---|---|---|
| `tiktok` | 60s | 9:16 | +0 ngày |
| `youtube_shorts` | 45s | 9:16 | +1 ngày (`VariantLagDays`) |
| `youtube_long` | 150s | **16:9** (`VariantAspect`) | +3 ngày |

`studioGrowthProducer.Enqueue` truyền `VariantSeconds` và `VariantAspect` vào
Story job. Renderer dựng ảnh/cảnh theo khổ đó, rồi QC kích thước và yêu cầu thời
lượng trong khoảng 80–125% mục tiêu; sai khổ hoặc lệch thời lượng thì job fail,
không được coi là output đạt. Các ngưỡng là kiểm tra render, không phải bảo đảm
đúng thời lượng tuyệt đối. Tài khoản có theme vẫn đi qua Affiliate Autopilot,
định dạng mục tiêu 30s 9:16 khi luồng đó hoạt động. `ConceptHash` (sha256, gấp dấu tiếng Việt)
Jaccard ≥ 0.8 → ép đổi góc kể (`dedupCheck` trong tick) để không trùng nội dung
giữa các tài khoản trong 45 ngày (`DedupWindowDays`).

### Ngưỡng governance (UI chỉnh được)
`automation.LoadThresholds` đọc `growth.thresholds` (JSON) phủ lên
`growth.DefaultConfig()` — hỏng từng trường thì fallback về default:
mặc định kill khi ≥8 video + ≥14 ngày + completion <35% (hoặc share/save <1%);
double-down khi gấp 5× median; breakout khi 10×; penalty khi views rơi >70%;
stalled khi 14 ngày tăng trưởng <2%. Vượt luật → `Evaluate` đề xuất hành động;
`SyncOneAccount` tự `Transition(id, "paused")` khi chạm kill.

> Mục tiêu Zero-Reup là tạo nội dung từ nguồn ảnh/video do người dùng hoặc app
> cung cấp. Code không chứng minh quyền sở hữu mọi asset, cũng không bảo đảm
> tránh giảm phân phối hay “0-view shadowban”.

### Sản xuất & đăng (trong tick)
Trong `produceAccount`, tài khoản có theme gọi `Autopilot.Run`, hiện dừng với
credit gate trước discovery/job creation. Kênh persona tạo Story job với thời
lượng và khổ từ `PlanItem.Variant`; số từ ước tính theo 150 từ/phút, số cảnh
được suy ra từ thời lượng. Story renderer hiện vẫn cần LLM, image-gen và TTS.
Phần governance và đồng bộ số liệu vẫn chạy. Xem `autonomous-daemon.md` — item
quá hạn 14 ngày → `dropped`; tối đa
**3 job/tài khoản/tick**; TikTok Growth item chỉ tới `produced` (chưa tự tải lên
draft; trang Growth cho tải MP4 để đăng thủ công);
YouTube thử đăng khi OAuth app + token cục bộ có mặt và qua quota guard trong code (`1` unit/lượt, mặc
định `100` lượt/ngày — `internal/growth/quota.go`), dùng reservation nguyên tử
và ngày reset Pacific. Google Cloud Console vẫn là nguồn chính xác cho giới
hạn project đã cấp. Thiếu gì thì item ở `waiting_connect`/`waiting_quota`
với ghi chú tiếng Việt trung thực. Mọi lượt đăng gắn cờ AI-generated
(`containsSyntheticMedia`, `Synthetic=true` — bắt buộc).

> `YTState.Ready()` chỉ kiểm tra OAuth app và token file cục bộ; không ping
> YouTube trước khi upload. TikTok Growth item hiện chỉ được đánh dấu render xong;
> Growth chưa tự tải lên draft. Trang Growth có link tải MP4 để operator đăng
> thủ công trong app TikTok.
>
> Tài khoản **persona** áp dụng variant vào Story job; output chỉ được đánh dấu
> đạt khi QC khổ và khoảng thời lượng qua. TikTok Growth item chưa tự tải lên
> nháp, nhưng trang Growth có link tải MP4 để operator đăng thủ công trong app.
> Kể chuyện dài 30–60 phút cũng chưa đạt; xem
> [storytelling.md](storytelling.md).

## 4. Fail-closed & an toàn
- Job fail/gate fail → item giữ lý do hoặc lỗi thật; Affiliate Autopilot gate trước
  khi claim sản phẩm, persona có thể tạo Story job qua pipeline hiện tại.
- Không OAuth/quota → `waiting_connect`/`waiting_quota`, không giả đăng.
- Nút "chạy ngay" (`force=true`) vẫn bị kill switch + dry-run chặn.
- Số liệu nguồn lỗi → giữ snapshot cũ, decision ghi lý do, không bịa số.

## 5. API key rotation
- Sync metrics YouTube: `YOUTUBE_API_KEY` (Data API v3, key đơn — không xoay vòng).
- Các bước phân tích/shot plan qua Studio → `GEMINI_API_KEYS` keyring xoay vòng (xem `key-rotation.md`); video API trả phí bị tắt.
- Đăng YouTube: OAuth refresh token từng tài khoản (`<data>/tokens/youtube_token_<user>.json`),
  tự refresh qua `oauth2.googleapis.com/token` khi hết hạn.
