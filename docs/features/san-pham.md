# Affiliate (`/products` — sidebar "Affiliate")

## 1. Mục đích
Kho sản phẩm duy nhất của app (`products.Store` trong `products.db`): tìm sản phẩm
hoa hồng cao theo theme, xếp lên "Kệ hàng" cho autopilot affiliate dùng, đặt lịch
chạy autopilot + nhạc nền + tự đăng.

> Sidebar sau Đợt A đổi label "Sản phẩm" → **"Affiliate"**.
> Nguồn sản phẩm duy nhất: **Accesstrade datafeed** (hunter tick hàng ngày →
> kho). TikTok Shop đã loại bỏ hoàn toàn ở Đợt H1 (2026-10-04) — không còn
> code, config hay document nào trỏ vào nó như nguồn đang hoạt động.

## 2. Kích hoạt
- `GET /products` → `handleProducts` (`internal/web/products.go:69`): 2 tab
  (`?tab=ke` Kệ hàng; tab tìm kiếm).
- `POST /products/search`: tìm theo theme + % hoa hồng tối thiểu.
- `POST /products/shelf/add`: đưa sản phẩm lên kệ (`shelf_status`, `shelf_score`).
- `POST /products/add`: thêm sản phẩm thủ công.
- `POST /products/schedule` → `handleProductsSchedule`: lịch autopilot
  (bật/tắt, số giờ mỗi chu kỳ, tự đăng TikTok, nhạc nền).
- `POST /products/music` → `handleAutopilotMusicUpload`: upload file nhạc.

## 3. Luồng vận hành chi tiết
1. **Tìm kiếm** (`handleProductsSearch`): `products.QueryForTheme(theme, minComm, 50)`
   → `products.Aggregate(ctx, s.ProductProviders, q)` với timeout **90 giây** —
   gom từ mọi provider đã cấu hình; provider chưa cấu hình (thiếu API key) bị bỏ qua
   kèm cảnh báo. Kết quả lưu vào `products` qua `s.Products.Save(p)`; hiển thị
   `TopByTheme(theme, minComm, 0, 30)`.
2. **Kệ hàng**: `shelfViews()` đọc sản phẩm có `shelf_status` đã set; sản phẩm trên kệ
   là nguồn hàng của autopilot affiliate. Khởi động app: `MigrateLedgerShelf()`
   di trú dữ liệu kệ cũ từ ledger **một lần, idempotent**.
3. **Lịch autopilot** (`scheduleView`/`handleProductsSchedule`): 4 knob lưu trong
   ledger settings — `autopilot_enabled` (unset = ON), `autopilot_interval_hours`
   (1–168, mặc định 6), `autopilot_auto_publish` (unset = ON), `autopilot_music_name`
   (file `autopilot-music.m4a` cạnh DB). Lưu xong → scheduler nền (tick 1 phút)
   áp dụng ở chu kỳ kế tiếp, không cần restart.
4. Chu kỳ chạy thật: `AutopilotTick` → `Autopilot.RunAll` (xem `daemon-tu-dong.md`).

## 4. Fail-closed & an toàn
- Không provider nào cấu hình → trang báo "Chưa cấu hình nguồn sản phẩm", không giả kết quả.
- Nguồn lỗi → warning liệt kê từng lỗi, các nguồn khác vẫn chạy.
- Autopilot cần: theme đã chọn + ảnh mẫu đã upload + có sản phẩm đủ hoa hồng —
  thiếu gì `Autopilot.Run` trả lỗi tiếng Việt rõ lý do, không tạo job rỗng.
- `RecordUse(prod.ID, accountID, jobID)` — không quảng bá cùng sản phẩm 2 lần
  cho một tài khoản.

## 5. API key rotation
- Tìm kiếm sản phẩm: key của từng product provider (cấu hình trong Cài đặt).
- Render video affiliate: `GEMINI_API_KEYS` keyring (xem `key-rotation.md`).
