# Tài khoản (3 tab: Tổng quan / Autopilot / Kết nối)

## 1. Mục đích
Quản lý một kênh: xem trạng thái + quyết định liên quan, cấu hình autopilot affiliate,
nối nền tảng (YouTube/TikTok), chuyển trạng thái vòng đời.

## 2. Kích hoạt
- `GET /accounts` → `handleAccounts` (`internal/web/accounts.go:20`): danh sách.
- `GET /accounts/new` → form tạo; `POST /accounts` → `handleAccountCreate`.
- `GET /accounts/{id}?tab=tong-quan|autopilot|ket-noi` → `handleAccountDetail`
  (`internal/web/accounts.go:102`); tab mặc định/tên tab do `accountTab` đọc query.
- Các nút POST trong từng tab (xem mục 3).

## 3. Luồng vận hành chi tiết

### Tab Tổng quan
- `handleAccountDetail` đọc: `s.Mgr.GetWithAutopilot(id)` (account + cấu hình autopilot),
  `network.TRANSITIONS[acct.Status]` → các nút chuyển trạng thái được phép
  (máy trạng thái, không cho nhảy trạng thái bừa),
  `network.PERSONAS[acct.Persona]` → persona + content pillars,
  20 decision mới nhất có `target = username`,
  `s.Ledger.AccountGiftUSD(id)` → tổng gift (USD),
  decision `live_planner/session_topic` mới nhất → chủ đề live gần nhất.
- `POST /accounts/{id}/transition` (`handleAccountTransition`): `s.Mgr.Transition(id, to, nil)`
  → 303 về tab `ket-noi`, lỗi thì `?err=` + toast (không redirect câm).

### Tab Autopilot (affiliate hands-off)
- `POST /accounts/{id}/theme` (`handleAccountTheme`, `account_autopilot.go:40`):
  lưu theme + min commission → `s.autoGeneratePlan` sinh kế hoạch 30 ngày ngay
  (zero-touch, lỗi chỉ log).
- `POST /accounts/{id}/autopilot` (`handleAccountAutopilot`): bật/tắt autopilot
  (`autopilot_enabled` = `"1"`/`"0"` trong ledger settings; unset = ON).
- `POST /accounts/{id}/autopilot/run` (`handleAccountAutopilotRun`): chạy ngay một
  chu kỳ `Autopilot.Run(ctx, accountID)` (xem `daemon-tu-dong.md`).
- `POST /accounts/{id}/models/upload` (`handleAccountModelUpload`): upload ảnh mẫu
  người mẫu → `ModelStore` (identity lock cho video affiliate); xem được ở
  `GET /models/{account}/{file}`; xoá bằng `POST .../models/{photoID}/delete`.

### Tab Kết nối
- `POST /accounts/{id}/youtube` (`handleAccountYoutube`): lưu YouTube channel +
  content types (`s.Mgr.SetYoutube`).
- `POST /accounts/{id}/onboard` (`handleAccountOnboard`): `advanceOnboarding(id)` —
  đẩy tài khoản qua một bước onboarding (`network.onboardStep`).
- `POST /accounts/{id}/replan` (`handleAccountReplan`): chạy lại topic research
  (`network.MakeTopicResearch(s.Mgr, id, s.LLM)`).
- `POST /accounts/{id}/live-topic` (`handleAccountLiveTopic`): `s.Mgr.PlanLiveTopic`
  — LLM viết chủ đề phiên live, ghi decision `live_planner/session_topic`.
- Hiển thị trạng thái publisher từng nền tảng (`publishers.BuildPublishers`):
  TikTok token/client, YouTube token, FB Page, RTMP — badge trung thực "thiếu/chưa".

## 4. Fail-closed & an toàn
- Mọi POST lỗi → `accountBack(..., errMsg)` → 303 về đúng tab kèm `?err=` + toast;
  không có chỗ nào log-and-redirect-như-thành-công (R2-W4/R2-13).
- Chuyển trạng thái chỉ cho phép các đích trong `network.TRANSITIONS`.

## 5. API key rotation
- Tab Kết nối không hiện key. Topic research / live topic dùng `s.LLM`
  (LLMChain: xem `key-rotation.md`).
- `handleAccountReplan` dùng LLM keyring engine `llm` (round-robin, cooldown 429).
