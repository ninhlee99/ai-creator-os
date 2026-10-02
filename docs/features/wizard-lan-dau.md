# Wizard lần đầu (`/onboard`)

## 1. Mục đích
Người mở app lần đầu (thư mục dữ liệu chưa có `ledger.db`) được chào + chọn nhanh
cấu hình an toàn trước khi vào app — không bỡ ngỡ, không chạy nhầm chế độ.

## 2. Kích hoạt
- `cmd/aicos/main.go` phát hiện `ledger.db` chưa tồn tại → `srv.Onboarding = true`.
- `onboardGate` (`internal/web/onboard.go:16`) bọc toàn bộ router: khi đang onboarding,
  mọi path (trừ `/onboard`, `/static/*`, `/favicon.ico`) → **303 về `/onboard`**.

## 3. Luồng vận hành chi tiết
1. `GET /onboard` → `handleOnboard` render `onboard.html`: chào, hiện version
   (`s.Cfg.Version`), thư mục data đang dùng (đổi bằng cờ `-data=` khi chạy binary).
2. `POST /onboard`: đọc checkbox `dryrun` → `s.Cfg.SetDryRun(dry)`; ghi decision
   `human/onboard` ("Hoàn tất thiết lập lần đầu", kèm `dry_run` + `data_dir`);
   `s.Onboarding = false` → 303 về `/?ok=Chào mừng! App đã sẵn sàng.`
3. Khởi động lại sau đó: `ledger.db` đã tồn tại → không hiện wizard nữa.

## 4. Fail-closed & an toàn
- Lỗi parse form → `s.fail` 500, wizard vẫn ở đó.
- Gate chặn toàn app cho tới khi hoàn tất — không lọt vào dashboard khi chưa setup.

## 5. API key rotation
Không dùng key cloud.
