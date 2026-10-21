# Web AI Multi-Account Hub & Trình Duyệt CDP Tự Động Đăng Nhập

## 1. Mục đích
Cung cấp giải pháp kết nối tài khoản Web AI (ChatGPT, Claude, Gemini, Grok, Muse AI) hoàn toàn tự động thông qua giao thức Chrome DevTools Protocol (CDP). Cho phép nhà sáng tạo sử dụng sức mạnh của các mô hình AI hàng đầu mà không cần mua API trả phí đắt đỏ hoặc sao chép cookie thủ công. Hỗ trợ xoay vòng đa tài khoản (multi-account rotation), cách ly phiên đăng nhập bằng thư mục hồ sơ tạm thời (`os.MkdirTemp`), tự động nhận diện danh tính (email/tên tài khoản) và quản lý thời gian hồi phục (cooldown) khi chạm ngưỡng giới hạn (rate limit).

## 2. Kích hoạt & Giao diện
- **Giao diện quản lý**: `GET /settings/he-thong#tab-webai` (thẻ "🤖 Tài khoản AI" trên trang Cài đặt Hệ thống).
- **Trình duyệt tự động đăng nhập (CDP Browser Login)**:
  - Bấm nút **"🌐 Đăng Nhập Tự Động"** tại thẻ của từng nền tảng (ChatGPT, Claude, Gemini, Grok, Muse AI).
  - Modal hướng dẫn xuất hiện, hiển thị trạng thái chờ và nút mở Chrome/Brave/Edge riêng biệt.
  - Khi người dùng đăng nhập xong, hệ thống tự động phát hiện cookie phiên và lưu vào `data/webai.db`; email chỉ ghi khi dịch vụ hiển thị danh tính đáng tin cậy.
- **Bàn làm việc tạo Prompt & Kịch bản**:
  - Tích hợp tại Studio AI (`/studio`) với bộ prompt chuyên biệt cho từng nền tảng:
    - **ChatGPT**: Kịch bản bán hàng, Hook 3 giây, Lời thoại voiceover, Phân tích tâm lý khách hàng.
    - **Claude**: Phân cảnh điện ảnh, Tối ưu câu từ, Chỉ đạo nghệ thuật, Prompt Gemini & Muse AI.
    - **Gemini**: Mô tả hình ảnh sản phẩm 4K siêu thực, Đồng nhất nhân vật người mẫu.
    - **Grok**: Bắt trend realtime X/Twitter, Kịch bản tranh luận đa chiều, Phản biện góc nhìn độc lạ.
    - **Muse AI**: Prompt tạo chuyển động video ngắn, Vòng lặp sản phẩm mượt mà.

## 3. Luồng vận hành chi tiết trong Code

```
┌─────────────────────────────────────────────────────────────┐
│ UI: Cài đặt · Hệ thống (#tab-webai)                         │
│  Nút: "🌐 Đăng Nhập Tự Động" [ChatGPT|Claude|Gemini|Grok|Muse]│
└──────────────────────┬──────────────────────────────────────┘
                       │ POST /web-ai/browser-login/start
┌──────────────────────▼──────────────────────────────────────┐
│ internal/web/web_ai.go & internal/webai/browser_login.go    │
│  1. Tạo thư mục UserDataDir tạm thời: os.MkdirTemp          │
│  2. Mở Browser (Chrome/Brave/Edge) với flag:                │
│     --remote-debugging-port=<random_port>                   │
│     --user-data-dir=<temp_dir>                              │
│  3. Kết nối CDP WebSocket (/json/version -> webSocketDebuggerUrl)│
│  4. Điều hướng tới login_url của nền tảng                   │
└──────────────────────┬──────────────────────────────────────┘
                       │ Polling định kỳ mỗi 1.5s
┌──────────────────────▼──────────────────────────────────────┐
│ Vòng lặp kiểm tra phiên đăng nhập (CDP Extraction Loop)     │
│  - Network.getAllCookies: lọc domain cookies                │
│  - Runtime.evaluate: trích xuất danh tính qua DOM / Storage khi có:│
│    * Grok: regex email chặt chẽ ([a-zA-Z0-9._%+-]+@...)     │
│    * Muse AI: hatch_sess + cookie Muse cùng domain          │
│    * ChatGPT: __Secure-next-auth.session-token              │
│    * Claude: sessionKey                                     │
│  - Thỏa mãn điều kiện auth -> Tự động đóng browser          │
│  - Lưu Account vào data/webai.db                            │
│  - Xóa dọn sạch thư mục UserDataDir tạm thời                │
└─────────────────────────────────────────────────────────────┘
```

### 3.1. Các nền tảng được hỗ trợ (`internal/webai/types.go`)
1. **ChatGPT** (`chatgpt`): Bắt token `__Secure-next-auth.session-token` hoặc cookie `session-token`.
2. **Claude** (`claude`): Bắt cookie `sessionKey` từ domain `claude.ai`.
3. **Gemini** (`gemini`): Bắt cookie xác thực Google `__Secure-1PSID`, `__Secure-3PSID` hoặc `SID`.
4. **Grok** (`grok`): Bắt cookie `sso`, `sso-rw` hoặc `x-anon-token`; trích xuất danh tính bằng biểu thức chính quy (Regex) loại trừ chuỗi dính liền trong DOM.
5. **Muse AI** (`museai`): Bắt cookie phiên `hatch_sess` cùng các cookie Muse cùng domain; `ps_l` chỉ là cookie phụ, không dùng thay phiên đăng nhập. Trích xuất danh tính từ DOM/storage khi trang cung cấp.

### 3.2. Cơ chế Đa Tài Khoản & Xoay Vòng (Multi-Account & Rotation)
- Mỗi nền tảng có thể kết nối không giới hạn số lượng tài khoản (ví dụ: 5 tài khoản Grok, 3 tài khoản Claude).
- Thư viện `internal/webai/store.go` lưu thông tin vào bảng `accounts` trong SQLite `data/webai.db`:
  - `status`: `ok`, `cooldown`, `rate_limited`, `invalid`, `disconnected`.
  - `cooldown_until`: Thời điểm kết thúc cooldown nếu bị nhà cung cấp chặn tạm thời.
  - Khi gọi AI tạo nội dung, hệ thống chọn tài khoản có `status == "ok"` và thời gian sử dụng xa nhất (`ORDER BY last_used ASC`).
  - Khi gặp lỗi Rate Limit (429), tài khoản tự động chuyển sang `cooldown` trong 60 phút, và hệ thống tự động chuyển sang tài khoản kế tiếp trong pool.

### 3.3. Bảo mật & Cô Lập Phiên (Session Isolation)
- **Zero-Profile Leakage**: Mỗi phiên đăng nhập CDP sử dụng một thư mục tạm thời độc lập sinh bởi `os.MkdirTemp("", "aicos_webai_*")`. Khi phiên kết thúc, toàn bộ dữ liệu duyệt web được xóa sạch (`os.RemoveAll`), ngăn chặn việc kế thừa session cũ hoặc ghi đè tài khoản.
- **Fail-Closed**: Khóa bí mật và cookie không bao giờ được render dưới dạng thô trên giao diện (chỉ hiển thị dưới dạng masked `••••abcd`).

## 4. Các điểm kiểm tra & API liên quan
- `POST /web-ai/browser-login/start`: Bắt đầu tiến trình mở trình duyệt đăng nhập CDP.
- `GET /web-ai/browser-login/status`: Kiểm tra trạng thái tiến trình đăng nhập (đang chờ, thành công, lỗi).
- `POST /web-ai/browser-login/cancel`: Hủy bỏ tiến trình và đóng trình duyệt.
- `POST /web-ai/accounts/add`: Thêm tài khoản thủ công bằng session key hoặc cookie.
- `POST /web-ai/accounts/delete`: Xóa tài khoản khỏi kho lưu trữ.
- `POST /web-ai/accounts/test`: Kiểm tra tính hợp lệ của phiên đăng nhập.
- `POST /web-ai/accounts/reset-cooldown`: Mở khóa tài khoản khỏi trạng thái cooldown bằng tay.
