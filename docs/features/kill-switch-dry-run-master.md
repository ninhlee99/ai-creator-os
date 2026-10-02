# Kill switch + Dry-run + MASTER_SWITCH

## 1. Mục đích
Ba lớp an toàn vận hành: kill switch (dừng khẩn cấp), dry-run (chế độ thử —
không thao tác thật), MASTER_SWITCH (công tắc chính của daemon mạng/live).

## 2. Kích hoạt
- Kill switch: nút đỏ trên Trang chủ / trang An toàn → `POST /kill`
  (`handleKill`, `settings.go:435`); gỡ → `POST /unkill`.
- Dry-run: `POST /settings/dryrun` ở trang An toàn (`handleSettingsDryRun`) —
  `value=on` thì bật; mặc định **BẬT** (`DRY_RUN=true` khi khởi động).
- MASTER_SWITCH: toggle ở Cài đặt · Hệ thống → `POST /settings/master`
  (`handleSettingsMaster`) — lưu key `master.switch` (`"1"`/`"0"`), **mặc định TẮT**;
  gieo từ env `MASTER_SWITCH` đúng một lần (`SeedMasterSwitch`).

## 3. Luồng vận hành chi tiết
1. **Một nguồn sự thật**: `web.Config` là `network.Gate` duy nhất
   (`netCfg.Gate = webCfg` trong `main.go`). Mọi tick đọc `Gate.KillSwitch()` /
   `Gate.DryRun()` **trực tiếp mỗi lần** — UI đổi là có hiệu lực ngay, không restart
   (bug cũ "UI và daemon dùng 2 bản config" đã sửa ở Wave 1).
2. **Daemon mạng** (`internal/network/daemon.go:158 Tick`, chạy mỗi 60s):
   - `!masterOn() || kill` → **dừng toàn bộ live đang chạy** (`OnStopLive` từng
     handle), tick đứng yên. `masterOn()` đọc `MasterGate` =
     `automation.MasterSwitchGate{Settings}` → key `master.switch` live
     (nil settings = TẮT — fail-closed).
   - `dry` → slot tới giờ **không** được mở (`if d.OnStartLive != nil && !dry`).
3. **Automation** (`internal/automation/service.go`): `s.killed()` / `s.dryRun()`
   chặn đầu `GrowthTick`, `AutopilotTick`, `AutoPublishAffiliate`;
   `Gate == nil` → dry-run (an toàn nhất).
4. **Studio phim**: `Studio.BudgetUSD` nối knob ngân sách — vượt trần Veo thì dừng
   job (lớp an toàn tiền, xem `studio-ai.md` §3.4).
5. Mọi lần bật/tắt đều ghi decision log (`human/kill_switch`, `human/dry_run`,
   `system/master_switch`) — Trang chủ hiện chip trạng thái trung thực.

## 4. Fail-closed & an toàn
- Thứ tự thắng: **kill switch > MASTER_SWITCH > dry-run** — kill thắng tất cả,
  kể cả khi master đang bật.
- Tắt master một cách vô hình là lỗi (R2-08): chip Trang chủ luôn hiện trạng thái.

## 5. API key rotation
Không dùng key cloud. (Kill/dry-run/master chỉ là cổng logic.)
