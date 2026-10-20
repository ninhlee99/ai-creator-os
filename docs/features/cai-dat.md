# Cài đặt — 4 trang con tinh gọn

## 1. Mục đích
Mọi cấu hình vận hành của app, chia 4 trang tinh gọn — mỗi trang trả lời đúng một câu hỏi.
`/settings` redirect 303 về `/settings/he-thong`.
## 2. Kích hoạt
- `GET /settings/he-thong` → `handleSettingsHeThong`
- `GET /settings/accesstrade` → Accesstrade key UI (nhập key 1 lần, nút test kết nối, badge trạng thái)
- `GET /settings/nha-cung-cap` → `handleSettingsNhaCungCap`
- `GET /settings/an-toan` → `handleSettingsAnToan`
- (tất cả trong `internal/web/settings.go`; template trong
  `internal/web/templates/settings/`; thanh chuyển trang là dải **tab** —
  tab đang mở tô màu accent). Các truy cập vào `/settings/model-local` cũ được tự động redirect 303 về `/settings/he-thong`).

## 3. Luồng vận hành chi tiết

### Hệ thống (`he-thong.html`)
- **Trạng thái hệ thống**: phiên bản app (ldflags, mặc định `dev`), chi phí API
  theo engine/provider (bảng `api_usage`).
- **Ngân sách API/ngày**: `POST /settings/api-budget` → `automation.SetAPIBudgetUSD`
  (key `ops.api_budget_usd`; validate 0–10000, sai → `?err=`); giá trị đã lưu thắng
  env `DAILY_API_BUDGET_USD` nhằm bảo vệ chi phí các API cloud có phí.
- **Khóa & biến cấu hình**: `POST /settings/env` → `handleSettingsEnvSave` —
  chỉ cho sửa các tên trong `envNames` (tên lạ → 400); lưu 2 nơi: `os.Setenv`
  (hiệu lực ngay) + `ledger.settings` (key `env.<NAME>`, để khởi động sau nạp lại
  qua `applyPersistedEnv`); xong gọi `s.Cfg.RefreshEnv()` → runtime Config đổi
  theo **không cần restart**.
- **Công tắc chính** (MASTER_SWITCH): `POST /settings/master` →
  `automation.SetMasterOn` (key `master.switch`, mặc định TẮT); tắt thì modal xác
  nhận; mọi lần đổi ghi decision log. Daemon đọc live mỗi tick (xem
  `kill-switch-dry-run-master.md`).
- **API nội bộ**: `POST /settings/api` → `automation.SetAPIEnabled`
  (key `api.enabled`, mặc định TẮT) — xem `api-noi-bo.md`.
- **Sao lưu & khôi phục**: xem `sao-luu-khoi-phuc.md`.

### Nhà cung cấp (`nha-cung-cap.html`)
- **Chuỗi provider TTS / LLM**: `GET /settings/chain` → `handleChainGet`;
  `POST /settings/chain` → `handleChainSave`; `POST /settings/chain/move` đổi
  thứ tự; `POST /settings/chain/health` probe từng provider. Cấu trúc
  `ChainConfig{Order[]}` lưu trong settings.
  - TTS: Gemini → VieNeu local → Edge (`internal/engines/chains.go`).
  - LLM: Gemini → llama-server local → paid (placeholder tắt).
- **Keyring từng engine**: `POST /settings/keys/add|delete`, `GET /settings/keys/status`,
  `POST /settings/keys/test` — key hiện dạng `••••abcd`, không bao giờ render raw.
- **Tab Accesstrade** (Đợt B): nhập `access_key` 1 lần → `POST` lưu ledger,
  nút **Test kết nối** (gọi API thật, không log key), badge trạng thái +
  số campaign đã duyệt. Chưa có key → tick bỏ qua im lặng (không spam alert).

### An toàn (`an-toan.html`)
- **Dry-run**: `POST /settings/dryrun` → `s.Cfg.SetDryRun(value=="on")` + decision log.
- **Kill switch**: `POST /kill` → `SetKillSwitch(true)`; `POST /unkill` → false;
  mỗi lần ghi decision `human/kill_switch`.

## 4. Fail-closed & an toàn
- Tên env không trong allowlist → 400, không ghi.
- Ngân sách API sai định dạng → `?err=`, giữ giá trị cũ.
- Mọi POST lỗi khác → `?err=` + toast.

## 5. API key rotation
Trang này là nơi **quản lý** key: keyring UI cho engine TTS/LLM (round-robin,
cooldown 429 theo thang 60s→5m→15m, key invalid bị skip — xem `key-rotation.md`).
Lưu key qua UI có hiệu lực ngay cho các lần gọi tiếp theo (keyring đọc lại),
không cần restart.
