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
| `campaigns.go` | `ListCampaigns` → GET `/v1/campaigns`; parse defensive nhiều dạng envelope (shape thật chốt khi có key thật). `ApprovedOnly()` lọc chiến dịch đã duyệt; `CommissionLabel()` — không có số thì hiện "—", không bịa |
| `links.go` | `CreateProductLink` → POST `/v1/product_link/create` (url, campaign_id, utm_source/medium, sub1–4) |
| `store.go` | SQLite `accesstrade.db` (WAL, `PRAGMA user_version`): `at_campaigns` (upsert theo campaign_id), `at_links` (upsert theo product_url + campaign_id + account_ref) |

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

## 4. Fail-closed

| Thiếu | Hành vi |
|---|---|
| Chưa nhập key | Mọi action AT chặn: redirect `?err=` về Cài đặt · Accesstrade, hoặc JSON 412 `{"ok":false,"error":"chưa có Accesstrade access_key…"}` |
| Chưa tải chiến dịch | Tạo link trả 412 "chưa có dữ liệu chiến dịch — bấm “Tải chiến dịch” trước" |
| Mạng/API lỗi | JSON lỗi trung thực kèm mask key, không retry vô hạn |

## 5. Giới hạn đã biết (trung thực)

- Chưa test với key thật của Ninh — mapping field response đang defensive;
  khi bấm "Tải chiến dịch" bằng key thật có thể cần chỉnh 1–2 field.
- Một số campaign cần **đăng ký và chờ duyệt tay** trên site Accesstrade
  (5 phút → vài ngày); app chỉ tạo link cho campaign `approval=successful`,
  không đoán/bỏ qua.
- Datafeed hunter + order sync + đối soát hoa hồng là **đợt C** (chưa có).
- Không sandbox công khai → mọi test chạm dữ liệu thật đều chỉ đọc.

## 6. Test
`internal/accesstrade/*_test.go` (mock `httptest`): header auth đúng format
`Token <key>`, parse campaigns, không retry 4xx / retry 5xx, store upsert.
`internal/web/accesstrade_test.go`: handler fail-closed khi chưa key, mask key.
