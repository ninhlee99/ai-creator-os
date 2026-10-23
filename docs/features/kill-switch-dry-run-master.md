# Kill switch + Dry-run + MASTER_SWITCH

## 1. Mục đích
Kill switch và dry-run là gate cho các workflow có áp dụng chúng; chúng không
chặn toàn bộ mọi thao tác trong app. `MASTER_SWITCH` chỉ điều khiển network daemon.

## 2. Kích hoạt
- Kill switch: nút đỏ trên Trang chủ / trang An toàn → `POST /kill`
  (`handleKill`, `settings.go:435`); gỡ → `POST /unkill`.
- Dry-run: `POST /settings/dryrun` ở trang An toàn (`handleSettingsDryRun`) —
  `value=on` thì bật; mặc định **BẬT** (`DRY_RUN=true` khi khởi động).
- MASTER_SWITCH: toggle ở Cài đặt · Hệ thống → `POST /settings/master`
  (`handleSettingsMaster`) — lưu key `master.switch` (`"1"`/`"0"`), **mặc định TẮT**;
  gieo từ env `MASTER_SWITCH` đúng một lần (`SeedMasterSwitch`).

## 3. Luồng vận hành chi tiết
1. `web.Config` cung cấp kill/dry-run state cho network daemon và automation
   service; UI đổi state có hiệu lực ngay, không restart.
2. **Network daemon** (`internal/network/daemon.go`, mỗi 60s): Master switch
   hoặc kill switch dừng các handle; dry-run chặn mở live mới. Live đang park,
   production không nối callback mở live.
3. **Automation service** kiểm tra gate theo workflow. Autopilot/Story/AT ticks
   có kill/dry-run checks; Growth có thể chạy sync trước khi dry-run chặn
   production/publish. ViralRadar read-only discovery kiểm tra kill nhưng không
   dừng theo dry-run. Một số thao tác thủ công từ UI cũng có thể gọi provider.
4. `Gate == nil` trong `automation.Service` coi production như dry-run.
   Master switch không gate các automation tick khác network daemon.
5. Bật/tắt controls ghi decision log; dashboard/settings hiển thị state đã lưu.

## 4. Fail-closed & an toàn
- Network daemon xử lý riêng master/kill/dry-run. Automation workflows áp dụng
  kill/dry-run theo logic từng tick; không coi UI badge là xác nhận mọi worker dừng.
- Tắt master chỉ dừng network daemon; scheduler khác vẫn theo gate riêng.

## 5. API key rotation
Không dùng key cloud. (Kill/dry-run/master chỉ là cổng logic.)
