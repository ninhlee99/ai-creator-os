# Lịch live (`/schedule`)

## 1. Mục đích
Xem các khung giờ live hôm nay của mọi tài khoản đủ điều kiện; build lại thủ công khi cần.

## 2. Kích hoạt
- `GET /schedule` → `handleSchedule` (`internal/web/schedule.go:15`).
- Nút "Xây lại lịch" → `POST /schedule/build` → `handleScheduleBuild`.
- Tự động: `ensureTodaySchedule()` được gọi mỗi khi mở Trang chủ hoặc trang Lịch live.

## 3. Luồng vận hành chi tiết
1. `ensureTodaySchedule()` (`internal/web/schedule.go:70`): đọc `s.Ledger.GetSlots(today)`
   (bảng `live_slots`); nếu hôm nay **chưa có slot nào** và có ít nhất một tài khoản
   `live_ready`/`live` → gọi `buildTodaySchedule()` **tối đa 1 lần/ngày/process**
   (cờ `s.schedBuilt`, mutex `s.schedMu`). Không có tài khoản đủ điều kiện → không làm gì.
2. `buildTodaySchedule()`:
   - `s.Ledger.ListAccounts(nil)` → `network.BuildSchedule(accts, weekday)` —
     allocator chỉ xếp tài khoản đủ điều kiện; mỗi slot gán `SlotDate = today`,
     `Status = "planned"`.
   - `DELETE FROM live_slots WHERE slot_date=today` rồi `s.Ledger.SaveSlots(slots)`
     → build lại là thay thế toàn bộ lịch hôm nay (không cộng dồn).
   - Ghi decision `scheduler/build_schedule` ("N slot") vào bảng `decisions`.
3. Daemon mạng (`internal/network/daemon.go`, tick **60 giây**) mới là nơi quyết định
   slot nào thật sự lên live — dưới cổng master/kill/dry-run (xem
   `kill-switch-dry-run-master.md`). Trang lịch chỉ hiển thị kế hoạch.

## 4. Fail-closed & an toàn
- Lỗi build → log `web: auto build schedule`, trang vẫn mở (không crash).
- Nút build tay lỗi → `s.fail` 500 với tên bước.
- Lịch chỉ là "planned"; việc mở live thật do daemon tick quyết định, bị dry-run chặn.

## 5. API key rotation
Không dùng key cloud.
