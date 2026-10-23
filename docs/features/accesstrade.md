# Accesstrade (affiliate)

## 1. Mục đích
Nối app với **Accesstrade Vietnam Publisher API** — nguồn affiliate duy nhất
sau pivot (thay TikTok Shop): lấy chiến dịch, tạo tracking link, (đợt C)
quét datafeed sản phẩm và đối soát hoa hồng.

Tài liệu API chính thức: `https://developers.accesstrade.vn/`
(base `https://api.accesstrade.vn`).

## 2. Client — `internal/accesstrade/`

| File | Vai trò |
|---|---|
| `client.go` | HTTP client: base URL, header `Authorization: Token <access_key>` + JSON, timeout 20s, retry tối đa 2 lần cho lỗi mạng/5xx (không retry 4xx). Không bao giờ log key — mask `••••abcd` |
| `settings.go` | `KeySetting = "at.access_key"` — một nguồn duy nhất cho web + automation |
| `campaigns.go` | `ListCampaigns` → GET `/v1/campaigns`; phân trang theo `page`/`total_page` hoặc `total`; đọc `description.commission_policy`. `ApprovedOnly()` lọc chiến dịch đã duyệt; `CommissionLabel()` — không có số thì hiện "—", không bịa |
| `datafeeds.go` | `ListDatafeeds` → GET `/v1/datafeeds` (page-based, tối đa 5 trang × 200); dùng đúng params `campaign`, `discount_from/to`; map `discount` thành giá bán và `price` thành giá gốc; chuẩn hoá hoa hồng nếu response có field |
| `links.go` | `CreateProductLink` → POST `/v1/product_link/create` (`campaign_id`, `urls[]`, UTM, sub1–4); đọc `data.success_link[].aff_link/short_link` |
| `orders.go` | `ListOrders` → GET `/v1/order-list` (since/until ISO, `page` số nguyên, limit 300, tối đa 10 trang); map `merchant`, `sales_time`, `is_confirmed`; **rate limiter nội bộ 10 req/phút** (cửa sổ trượt 60s) |
| `hunter.go` | `Hunt` → datafeed → lọc (có aff_link, giá > 0) → map vào `products.Store` (Source="accesstrade", aff_link→ProductURL, ảnh→ImageURLs, dedupe theo SKU/aff_link, category→theme) |
| `store.go` | SQLite `accesstrade.db` (WAL, `PRAGMA user_version` v2): `at_campaigns`, `at_links`, `at_orders` (upsert theo order_id, **UPDATE pending tại chỗ**), `at_sync_state` (watermark) |

Key lưu ở ledger settings (`at.access_key`), đọc mỗi lần gọi API → đổi key
không cần restart.

## 3. UI

**Cài đặt → tab Accesstrade** (`GET /settings/accesstrade`,
`templates/settings/accesstrade.html`):
- Ô nhập key (password, autocomplete off), badge **Có API key · chưa kiểm tra** (mask
  `••••abcd`) / **Chưa có key**.
- Nút **Kiểm tra kết nối** (`POST /settings/accesstrade/test`, AJAX → toast):
  gọi `ListCampaigns(limit=1)`; lỗi mạng/key sai trả JSON trung thực, không
  giả vờ thành công.

**Trang Affiliate** (`/products`, card "Chiến dịch Accesstrade"):
- Nút **Tải chiến dịch** (`POST /at/campaigns/sync`) → lưu cache vào
  `at_campaigns`; bảng campaign (tên, badge trạng thái duyệt, hoa hồng).
- Nút **Tạo link** (chỉ hiện với campaign đã duyệt) → modal nhập product URL
  + utm/sub → `POST /at/links/create` (JSON) → hiện link + nút Sao chép.
- Bảng **Link đã tạo** đọc từ `at_links`.

**Trang Affiliate — Đợt C** (`/products`, `internal/web/accesstrade.go`):

| Card | UI | Route |
|---|---|---|
| 🎯 Săn sản phẩm | Nút **Săn ngay** quét và lưu offer; trả riêng số offer mới, watchlist, video và lý do chưa tạo video | `POST /at/hunt` → `handleATHunt`: `Client.Hunt` → `products.Store`; chỉ thử tạo video khi có demand evidence, bật Flow credits, có ảnh tham chiếu theo kênh và producer sẵn sàng |
| ⚙️ Tự động Accesstrade | Cài đặt lịch hunter/order sync/campaign check, số video tối đa và cho phép Flow credits | `POST /settings/accesstrade/automation` → `handleATAutomationSave` lưu `at.hunter_enabled`, `at.hunter_videos`, `at.hunter_flow_credits`, `at.ordersync_enabled`, `at.campaigncheck_enabled` cùng cadence |
| ⏱️ Chu kỳ tự động (tab Cài đặt · Accesstrade) | Chu kỳ săn sản phẩm (giờ, 1–168) + chu kỳ đồng bộ đơn (phút, 5–1440); công tắc on/off ở trang Affiliate (form → 303 → toast) | `GET /settings/accesstrade`, `POST /settings/accesstrade/automation` → `handleATAutomationSave` (lưu `at.hunter_interval_hours`, `at.ordersync_interval_minutes`) |
| 💰 Đối soát hoa hồng | Badge **Đã duyệt** / **Chờ duyệt** / **Từ chối** (hiện RIÊNG — pending không cộng vào đã duyệt); nút **Đồng bộ đơn** (AJAX → toast); bảng đơn gần nhất (mã đơn mask `AT12••••9X7Q`) | `POST /at/orders/sync` → `handleATOrderSync`: `ListOrders` (cửa sổ 7 ngày) → `UpsertOrders` (update pending tại chỗ) |

Chỉ coi số tiền là dữ liệu hiện có sau khi `at_sync_state.orders.until` được ghi
bởi lần sync thành công. Dashboard đọc `at_orders` (đã duyệt/chờ duyệt), không
dùng tổng commission legacy trong `ledger.db`; lỗi đọc stats không được trình
bày thành 0.

## 4. Fail-closed

| Thiếu | Hành vi |
|---|---|
| Chưa nhập key | Mọi action AT chặn: redirect `?err=` về Cài đặt · Accesstrade, hoặc JSON 412 `{"ok":false,"error":"chưa có Accesstrade access_key…"}`; tick daemon bỏ qua im lặng + log (không spam alert) |
| Chưa tải chiến dịch | Tạo link trả 412 "chưa có dữ liệu chiến dịch — bấm “Tải chiến dịch” trước" |
| Mạng/API lỗi | JSON lỗi trung thực kèm mask key, không retry vô hạn |

## 5. Automation tick (Đợt C — `internal/automation/at_tick.go`)

`ATTick` (gọi mỗi 5 phút từ `cmd/aicos/main.go` §7c) → 3 tick con, cùng cổng
**kill switch + DRY-RUN** như `AutopilotTick`; công tắc + cadence chỉnh từ UI
(card ⚙️, unset = BẬT — zero-touch):

| Tick | Hạn | Việc |
|---|---|---|
| `ATHunterTick` | `at.hunter_interval_hours` (mặc định 24) | `Hunt` lưu offer; chỉ offer có sales/rating evidence mới vào production candidates. Tạo video còn cần `at.hunter_flow_credits=1`, kênh đích có ảnh tham chiếu và producer sẵn sàng; mặc định Flow credits tắt nên tick chỉ tìm/lưu offer. |
| `ATOrderSyncTick` | `at.ordersync_interval_minutes` (mặc định 30) | `ListOrders` (rolling 7 ngày — bắt kịp đơn pending→approved) → `UpsertOrders` → watermark `orders.until` trong `at_sync_state` |
| `ATCampaignCheckTick` | daily | `ListCampaigns` → upsert cache → campaign chưa duyệt → `ledger.Decide("at_hunter","campaign_pending",…)` làm alert |

Key đọc từ `accesstrade.KeySetting` (`at.access_key`) — **một nguồn duy nhất**
cho cả web và automation (định nghĩa trong package accesstrade).

## 5b. Tự đăng video affiliate (Đợt 3 + Đợt Q)

`AutoPublishAffiliate` (hook `SetOnDone` của Studio) đăng video affiliate
xong việc lên 2 kênh — mỗi kênh một công tắc riêng ở trang Affiliate
(card ⚙️, unset = BẬT):

| Kênh | Chế độ | Ghi chú |
|---|---|---|
| TikTok (`autopilot_auto_publish`) | **nháp** | Giới hạn nền tảng: gắn giỏ hàng + nhạc tay trong app TikTok |
| YouTube Shorts (`autopilot_youtube_enabled`, đợt Q) | **private** | Kèm link affiliate trong mô tả + dòng khai báo AI; AI disclosure `Synthetic=true`; quota-guarded (hết quota → log trung thực, không đăng) |

Mô tả YouTube: tên sản phẩm + link Accesstrade (`AffiliateParams.AffLink`,
hunter điền từ `ProductURL`) + `growth.DisclosureLine`.

## 6. Giới hạn đã biết (trung thực)

- Schema datafeed chính thức có `price`, `discount`, `cate`, `campaign`,
  `merchant`, `aff_link`, `sku`, `product_id`, `update_time`; không định nghĩa
  sales/rating. Adapter phân trang bằng `page` và đọc giá bán từ `discount`.
  Vì vậy datafeed chuẩn vẫn nằm trong watchlist, chưa đủ evidence để tự tạo
  video affiliate.
- Gate sales/rating yêu cầu nguồn provider, cả hai số, URL HTTPS làm nguồn đối
  chiếu và thời điểm ghi nhận không quá 30 ngày. Số liệu nhập tay chỉ để tham
  khảo, không tự xác minh nội dung trang và không qua gate production.
  Migration thêm nguồn, thời điểm và snapshot hai chỉ số,
  giữ nguyên số liệu cũ; snapshot làm gate tự đóng nếu bản app cũ đổi số mà
  không cập nhật provenance. Bản ghi cũ thiếu nguồn vẫn nằm trong kho nhưng
  không đủ gate và storefront sẽ ẩn số.
- Chưa test với key thật của Ninh — mapping field response đang defensive;
  khi bấm "Tải chiến dịch" bằng key thật có thể cần chỉnh 1–2 field.
- Một số campaign cần **đăng ký và chờ duyệt tay** trên site Accesstrade
  (5 phút → vài ngày); app chỉ tạo link cho campaign `approval=successful`,
  không đoán/bỏ qua. Tick campaign check chỉ báo, không tự duyệt.
- Tạo video hunter gọi Google Flow sau khi được bật Flow credits; code không xác nhận
  chi phí bằng 0. Khi chưa bật, hunter chỉ tìm/lưu offer và ghi lý do không tạo video.
- Không sandbox công khai → mọi test chạm dữ liệu thật đều chỉ đọc.

## 7. Test
`internal/accesstrade/pipeline_test.go` (mock `httptest`): parse datafeeds
(sort hoa hồng client-side, chuẩn hoá %→tỉ lệ), parse order-list, rate
limiter 10 req/phút (clock/sleep giả), upsert + update-pending-tại-chỗ,
`GetOrderStats` (pending hiện riêng), migrate v1→v2, hunter map/dedupe/theme.
`internal/automation/at_tick_test.go`: cổng kill/dry-run/thiếu key im lặng,
cadence, hunter tạo đúng N video, order sync + watermark, campaign alert.
`internal/web/accesstrade_wavec_test.go`: handler fail-closed khi chưa key,
lưu settings, render `/products` có 3 card mới.
`internal/accesstrade/accesstrade_test.go` (cũ): header auth `Token <key>`,
parse campaigns, không retry 4xx / retry 5xx, store upsert.
`internal/web/accesstrade_test.go` (cũ): handler fail-closed khi chưa key, mask key.
