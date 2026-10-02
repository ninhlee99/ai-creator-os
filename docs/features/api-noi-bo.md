# API nội bộ (`/api/*`)

## 1. Mục đích
3 endpoint JSON legacy cho vận hành ngoài (`/api/stats`, `/api/products`,
`/api/decisions`) — **mặc định TẮT**, chỉ mở khi người vận hành bật công tắc.

## 2. Kích hoạt
- Công tắc ở Cài đặt · Hệ thống: `POST /settings/api` →
  `automation.SetAPIEnabled` (key `api.enabled` trong ledger settings).
- Khi tắt: mọi `GET /api/*` → **404** (không lộ endpoint trên LAN).
- Khi bật: endpoint trả JSON bình thường.

## 3. Luồng vận hành chi tiết
`requireAPI` (`internal/web/api.go:19`) bọc 3 handler:
- `handleAPIStats`: `dry_run`, `kill_switch`, `total_commission`
  (`SUM(commission)` từ `orders`), `daily_spend_usd` (`s.Ledger.DailySpendUSD()`).
- `handleAPIProducts`: đọc kho `products.Store` chuẩn (R2-W1).
- `handleAPIDecisions`: đọc bảng `decisions`.
Không có UI nào trong app gọi 3 endpoint này.

## 4. Fail-closed & an toàn
- Mặc định TẮT (R2-W7/R2-09): `automation.APIEnabled` false khi chưa từng set —
  fail-closed trên mạng LAN.

## 5. API key rotation
Không dùng key cloud.
