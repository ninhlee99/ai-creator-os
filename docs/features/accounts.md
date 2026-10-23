# Kênh (`/accounts` — sidebar "Kênh")

## 1. Mục đích
Quản lý một kênh: xem trạng thái + quyết định liên quan, cấu hình autopilot affiliate,
nối nền tảng (YouTube/TikTok), chuyển trạng thái vòng đời.

> Sidebar sau Đợt A đổi label "Tài khoản" → **"Kênh"** (theo
> `docs/PIVOT_REDESIGN.md` §1). Nội dung trang `/accounts` vẫn dùng từ
> "Tài khoản" ở tiêu đề/bảng — sẽ đồng bộ ở đợt UI sau.

## 2. Kích hoạt
- `GET /accounts` → `handleAccounts` (`internal/web/accounts.go:20`): danh sách.
- `GET /accounts/new` → form tạo; `POST /accounts` → `handleAccountCreate`.
- `GET /accounts/{id}?tab=overview|autopilot|brand-dna|connections|comments|model-photos` → `handleAccountDetail`
  (`internal/web/accounts.go:102`); tab mặc định/tên tab do `s.accountTab` đọc query.
- Các nút POST trong từng tab (xem mục 3).

## 3. Luồng vận hành chi tiết

### Tab Tổng quan
- Badge **YouTube Analytics**: `Có token · API chưa kiểm` (có token file) /
  `Chưa có token` / `Chưa gắn kênh` — chỉ kiểm tra file; thiếu scope
  `yt-analytics.readonly` thì lúc sync API báo 403 với hướng dẫn chạy lại
  `scripts/get-youtube-token.py`.
- `handleAccountDetail` đọc: `s.Mgr.GetWithAutopilot(id)` (account + cấu hình autopilot),
  `network.TRANSITIONS[acct.Status]` → các nút chuyển trạng thái được phép
  (máy trạng thái, không cho nhảy trạng thái bừa),
  `network.PERSONAS[acct.Persona]` → persona + content pillars,
  20 decision mới nhất có `target = username`.
- `POST /accounts/{id}/transition` (`handleAccountTransition`): `s.Mgr.Transition(id, to, nil)`
  → 303 về tab `ket-noi`, lỗi thì `?err=` + toast (không redirect câm).

### Tab Autopilot (affiliate settings)
- `POST /accounts/{id}/theme` (`handleAccountTheme`): lưu theme để lọc sản phẩm
  affiliate. Không thay Growth plan 30 ngày. Commission floor lưu riêng trong
  form Autopilot.
- `POST /accounts/{id}/autopilot` (`handleAccountAutopilot`): lưu bật/tắt và
  commission floor trên hồ sơ kênh trong `network.db`.
- Producer affiliate hiện dừng trước discovery/job creation đến khi có xác nhận
  dùng Google Flow credits theo kênh. Nút chạy ngay đã gỡ khỏi UI; POST cũ trả
  lỗi gate thay vì tuyên bố bắt đầu.
- `POST /accounts/{id}/models/upload` (`handleAccountModelUpload`): upload ảnh mẫu
  người mẫu → `ModelStore` (ảnh tham chiếu cho video affiliate; không đảm bảo
  giữ mặt tuyệt đối ở mọi frame); xem được ở
  `GET /models/{account}/{file}`; xoá bằng `POST .../models/{photoID}/delete`.
- Tab Mô phỏng phản hồi (`POST /accounts/{id}/comments/simulate`) phân loại bằng
  luật từ khóa và câu mẫu cố định; không gọi AI, đọc comment nền tảng hoặc gửi reply.

### Tab Bản sắc (Brand DNA)
- Form lưu positioning, audience persona, value promise, tone, hook rules, content pillars, visual/audio style, red lines và checklist vào SQLite `growth.db` bảng `brand_dna`.
- Nếu chưa có bản ghi, UI điền mẫu gợi ý theo ngách nhưng chưa lưu; trạng thái này được phân biệt với hồ sơ đã lưu. `POST /accounts/{id}/brand-dna` ghi hồ sơ do người vận hành gửi; không xác thực chính sách.
- `POST /accounts/{id}/brand-dna/reset` ghi đè hồ sơ bằng mẫu tĩnh theo ngách. Mẫu này không được AI tạo hoặc tối ưu.
- Affiliate Director dùng positioning, audience, tone, visual style và red lines làm ngữ cảnh prompt. Trend heuristic dùng positioning, content pillars và hook rules. Không phải mọi job Studio đều nhận Brand DNA.
- Checklist QC chỉ được lưu tham khảo; chưa tự kiểm tra nội dung hoặc chặn publish. Red lines trong Affiliate Director là chỉ dẫn prompt, chưa phải gate deterministic.

### Tab Kết nối
- `POST /accounts/{id}/youtube` (`handleAccountYoutube`): lưu YouTube channel +
  content types (`s.Mgr.SetYoutube`).
- `POST /accounts/{id}/onboard` (`handleAccountOnboard`): `advanceOnboarding(id)` —
  đẩy tài khoản qua một bước onboarding (`network.onboardStep`).
- `POST /accounts/{id}/replan` (`handleAccountReplan`): chạy lại topic research
  (`network.MakeTopicResearch(s.Mgr, id, s.LLM)`).
- Hiển thị adapter có credentials cục bộ (`publishers.BuildPublishers`); chưa
  gọi API để xác minh token scope, quyền đăng hoặc publish thành công.

## 4. Fail-closed & an toàn
- Mọi POST lỗi → `s.accountBack(..., errMsg)` → 303 về đúng tab kèm `?err=` + toast;
  không có chỗ nào log-and-redirect-như-thành-công.
- Chuyển trạng thái chỉ cho phép các đích trong `network.TRANSITIONS`.

## 5. API key rotation
- Tab Kết nối không hiện key. Topic research dùng `s.LLM`
  (LLMChain: xem `key-rotation.md`).
- `handleAccountReplan` dùng LLM keyring engine `llm` (round-robin, cooldown 429).
