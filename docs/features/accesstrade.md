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
| `campaigns.go` | `ListCampaigns` → GET `/v1/campaigns`; parse defensive nhiều dạng envelope (shape thật chốt khi có key thật). `ApprovedOnly()` lọc chiến dịch đã duyệt; `CommissionLabel()` — không có số thì hiện "—", không bịa |
| `datafeeds.go` | `ListDatafeeds` → GET `/v1/datafeeds` (limit max 200); parse defensive; chuẩn hoá hoa hồng %→tỉ lệ 0..1; sort HIGH_COMMISSION_RATE phía client |
| `links.go` | `CreateProductLink` → POST `/v1/product_link/create` (url, campaign_id, utm_source/medium, sub1–4) |
| `orders.go` | `ListOrders` → GET `/v1/order-list` (since/until ISO, theo phân trang); **rate limiter nội bộ 10 req/phút** (cửa sổ trượt 60s, inject clock/sleep để test) |
| `hunter.go` | `Hunt` → datafeed → lọc (có aff_link, giá > 0) → map vào `products.Store` (Source="accesstrade", aff_link→ProductURL, ảnh→ImageURLs, dedupe theo SKU/aff_link, category→theme) |
| `store.go` | SQLite `accesstrade.db` (WAL, `PRAGMA user_version` v2): `at_campaigns`, `at_links`, `at_orders` (upsert theo order_id, **UPDATE pending tại chỗ**), `at_sync_state` (watermark) |

Key lưu ở ledger settings (`at.access_key`), đọc mỗi lần gọi API → đổi key
không cần restart.

## 3. UI

**Cài đặt → tab Accesstrade** (`GET /settings/accesstrade`,
`templates/settings/accesstrade.html`):
- Ô nhập key (password, autocomplete off), badge **Đã kết nối** (mask
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
| 🎯 Săn sản phẩm | Nút **Săn ngay** (AJAX → toast "Đã săn X sản phẩm mới, tạo Y video"); bảng sản phẩm AT mới nhất (chưa dùng làm video) | `POST /at/hunt` → `handleATHunt`: `Client.Hunt` → `products.Store` (Source="accesstrade") → top N mới → `automation.Service.MakeHunterVideo` → `studio.CreateAffiliateJob` (ảnh listing + nhạc, không chữ/voiceover) |
| ⚙️ Tự động Accesstrade | Checkbox hunter/ordersync/campaigncheck + số video mỗi lần săn (AJAX → toast) | `POST /at/settings` → `handleATSettings` (lưu ledger settings `at.hunter_enabled`, `at.hunter_videos`, `at.ordersync_enabled`, `at.campaigncheck_enabled`) |
| 💰 Đối soát hoa hồng | Badge **Đã duyệt** / **Chờ duyệt** / **Từ chối** (hiện RIÊNG — pending không cộng vào đã duyệt); nút **Đồng bộ đơn** (AJAX → toast); bảng đơn gần nhất (mã đơn mask `AT12••••9X7Q`) | `POST /at/orders/sync` → `handleATOrderSync`: `ListOrders` (cửa sổ 7 ngày) → `UpsertOrders` (update pending tại chỗ) |

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
| `ATHunterTick` | `at.hunter_interval_hours` (mặc định 24) | `Hunt` → top N (`at.hunter_videos`, mặc định 3) sản phẩm MỚI → `MakeHunterVideo` (tải ảnh listing qua `studio.DownloadImage`, `CreateAffiliateJob` mode photo) → `products.RecordUse(id, 0, jobID)` (account 0 = video trực tiếp từ hunter) |
| `ATOrderSyncTick` | `at.ordersync_interval_minutes` (mặc định 30) | `ListOrders` (rolling 7 ngày — bắt kịp đơn pending→approved) → `UpsertOrders` → watermark `orders.until` trong `at_sync_state` |
| `ATCampaignCheckTick` | daily | `ListCampaigns` → upsert cache → campaign chưa duyệt → `ledger.Decide("at_hunter","campaign_pending",…)` làm alert |

Key đọc từ `accesstrade.KeySetting` (`at.access_key`) — **một nguồn duy nhất**
cho cả web và automation (định nghĩa trong package accesstrade).

## 6. Giới hạn đã biết (trung thực)

- Chưa test với key thật của Ninh — mapping field response đang defensive;
  khi bấm "Tải chiến dịch" bằng key thật có thể cần chỉnh 1–2 field.
- Một số campaign cần **đăng ký và chờ duyệt tay** trên site Accesstrade
  (5 phút → vài ngày); app chỉ tạo link cho campaign `approval=successful`,
  không đoán/bỏ qua. Tick campaign check chỉ báo, không tự duyệt.
- Tạo video hunter đốt quota Gemini (image gen) như autopilot thường —
  DRY-RUN chặn; hết quota ngày → job fail-closed, tick hôm sau chạy tiếp.
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
