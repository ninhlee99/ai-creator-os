# Cài đặt — 5 trang con

## 1. Mục đích
Mọi cấu hình vận hành của app, chia 5 trang — mỗi trang trả lời đúng một câu hỏi.
`/settings` redirect 303 về `/settings/he-thong`.

## 2. Kích hoạt
- `GET /settings/he-thong` → `handleSettingsHeThong`
- `GET /settings/nha-cung-cap` → `handleSettingsNhaCungCap`
- `GET /settings/model-local` → `handleSettingsModelLocal`
- `GET /settings/nhan-vat` → `handleSettingsNhanVat`
- `GET /settings/an-toan` → `handleSettingsAnToan`
- (tất cả trong `internal/web/settings.go`; thanh chuyển trang là dải **tab** —
  tab đang mở tô màu accent).

## 3. Luồng vận hành chi tiết

### Hệ thống (`he-thong.html`)
- **Trạng thái hệ thống**: phiên bản app (ldflags, mặc định `dev`), chi phí API
  theo engine/provider (bảng `api_usage` — từ `/analytics` cũ tan vào đây, Đợt 2).
- **Ngân sách API/ngày**: `POST /settings/api-budget` → `automation.SetAPIBudgetUSD`
  (key `ops.api_budget_usd`; validate 0–10000, sai → `?err=`); giá trị đã lưu thắng
  env `DAILY_API_BUDGET_USD`. **Trung thực**: hiện tại chỉ pipeline phim Veo đọc
  trần này (qua `Studio.BudgetUSD`); chưa có consumer nào khác áp dụng.
- **Khóa & biến cấu hình**: `POST /settings/env` → `handleSettingsEnvSave` —
  chỉ cho sửa các tên trong `envNames` (tên lạ → 400); lưu 2 nơi: `os.Setenv`
  (hiệu lực ngay) + `ledger.settings` (key `env.<NAME>`, để khởi động sau nạp lại
  qua `applyPersistedEnv`); xong gọi `s.Cfg.RefreshEnv()` → runtime Config đổi
  theo **không cần restart** (R2-W4/R2-05).
- **Công tắc chính** (MASTER_SWITCH): `POST /settings/master` →
  `automation.SetMasterOn` (key `master.switch`, mặc định TẮT); tắt thì modal xác
  nhận; mọi lần đổi ghi decision log. Daemon đọc live mỗi tick (xem
  `kill-switch-dry-run-master.md`).
- **API nội bộ**: `POST /settings/api` → `automation.SetAPIEnabled`
  (key `api.enabled`, mặc định TẮT) — xem `api-noi-bo.md`.
- **Sao lưu & khôi phục**: xem `sao-luu-khoi-phuc.md`.
- **RTMP theo tài khoản**: bảng hiển thị (không sửa ở đây).

### Nhà cung cấp (`nha-cung-cap.html`)
- **Chuỗi provider TTS / LLM / Avatar**: `GET /settings/chain` →
  `handleChainGet`; `POST /settings/chain` → `handleChainSave`;
  `POST /settings/chain/move` đổi thứ tự; `POST /settings/chain/health` probe
  từng provider. Cấu trúc `ChainConfig{Order[]}` lưu trong settings.
  - TTS: Gemini → VieNeu local → Edge (`internal/engines/chains.go:34`).
  - LLM: Gemini → llama-server local → paid (placeholder tắt).
  - Avatar: local (MuseTalk sidecar, **không realtime**) → HeyGen → D-ID
    (paid tắt mặc định); `avatarRealtime()` chỉ true khi tier cloud bật —
    local không bao giờ được tính là realtime.
- **Keyring từng engine**: `POST /settings/keys/add|delete`, `GET /settings/keys/status`,
  `POST /settings/keys/test` — key hiện dạng `••••abcd`, không bao giờ render raw.

### Model local (`model-local.html`)
- **VieNeu TTS**: `GET /settings/vieneu/status`, `POST /settings/vieneu/ensure|restart`,
  `GET /settings/vieneu/progress`, `POST /settings/vieneu/voice` — Go quản lý
  tiến trình sidecar (start/stop/health check) như FFmpeg.
- **Avatar sidecar**: các route `/settings/avatar-sidecar/*` tương tự.
- **Khả năng AI** (`internal/studio/capabilities.go`, bảng `capabilities` trong
  studio.db) — app tự biết mình làm được gì trước khi hứa (Wave 3):
  - **Vẽ ảnh (Gemini)**: app vẽ thử 1 ảnh đơn giản ngay lúc khởi động (goroutine,
    timeout 3 phút, tốn tối thiểu) → badge ok/fail + thời điểm + chi tiết lỗi.
  - **Quay video (Veo)**: không tự vẽ thử (tốn ~8s Veo tiền thật) — suy từ 30 lần
    quay gần nhất (shot method `veo` thành công → ok; toàn fallback → fail; chưa
    quay → "chưa kiểm tra").
  - **Nút "Kiểm tra quay video"** (`POST /settings/model-local/probe-video`):
    modal cảnh báo "TỐN ~8s Veo (tiền thật, ước tính chưa kiểm chứng)" → quay thử
    thật trong job nền (timeout 20 phút); kết quả tay luôn thắng suy luận lịch sử.
  - Ảnh hưởng trực tiếp: chế độ dựng `auto` của Studio → ok mới thử Veo, ngược lại
    đi thẳng điện ảnh từ ảnh (xem `studio-ai.md` §3.4).
- **Công cụ hệ thống** (`internal/web/runtime.go`): probe thật `exec.LookPath`
  FFmpeg, llama-server, uv, docker — badge "Đã tìm thấy + đường dẫn" hoặc
  "Chưa có + câu lệnh cài (≤1 dòng, vd `brew install ffmpeg`)". Không tuyên bố
  sẵn sàng khi chưa tìm thấy.

### Nhân vật (`nhan-vat.html`)
- CRUD nhân vật avatar: `GET/POST /settings/avatar/characters`,
  `POST /settings/avatar/characters/{id}/delete`,
  `POST /settings/avatar/render-test` — render thử khung hình.

### An toàn (`an-toan.html`)
- **Dry-run**: `POST /settings/dryrun` → `s.Cfg.SetDryRun(value=="on")` + decision log.
- **Kill switch**: `POST /kill` → `SetKillSwitch(true)`; `POST /unkill` → false;
  mỗi lần ghi decision `human/kill_switch`.

## 4. Fail-closed & an toàn
- Tên env không trong allowlist → 400, không ghi.
- Ngân sách API sai định dạng → `?err=`, giữ giá trị cũ.
- Mọi POST lỗi khác → `?err=` + toast (R2-W4/R2-13).

## 5. API key rotation
Trang này là nơi **quản lý** key: keyring UI cho engine TTS/LLM (round-robin,
cooldown 429 theo thang 60s→5m→15m, key invalid bị skip — xem `key-rotation.md`).
Lưu key qua UI có hiệu lực ngay cho các lần gọi tiếp theo (keyring đọc lại),
không cần restart.
