# Cài đặt Hệ thống

Trang **Cài đặt** (`/settings`) quản lý các cấu hình cốt lõi của AI Creator OS.

## Các tab chức năng trên giao diện

Thanh điều hướng Cài đặt gồm 3 tab chuẩn:

1. **⚙️ Hệ thống (`/settings/system`)**:
   - **Tài khoản Web AI**: Đăng nhập phiên tài khoản (Muse AI, Grok, ChatGPT, Claude...) qua Chromium desktop riêng biệt.
   - **Tài khoản TikTok Social**: Đăng nhập và bắt cookie phiên TikTok để cấp quyền cho Web Crawler & Radar.
   - **API Keys tùy biến**: Cấu hình API Key theo danh mục chức năng (LLM, Tạo ảnh, Tạo video, Tạo âm thanh/TTS, Upscale).
   - **Vận hành hệ thống**: Xem logs máy chủ thời gian thực, khởi động lại server, dọn dẹp (Vacuum) SQLite, bật/tắt Dry-Run và Kill Switch.
   - *(Lưu ý: Các thẻ thống kê rườm rà và tính năng Nhập/Xuất JSON cũ đã được loại bỏ hoàn toàn để giao diện tinh gọn, phản hồi nhanh).*

2. **🔀 Thứ tự AI (`/settings/providers`)**:
   - Cấu hình chuỗi ưu tiên nhà cung cấp AI (Provider chain order).
   - Thiết lập fallback khi một model hoặc provider gặp sự cố / hết quota.

3. **🛡️ An toàn (`/settings/safety`)**:
   - Trạng thái Kill Switch toàn hệ thống.
   - Chế độ Dry-Run thử nghiệm an toàn không tốn credits.
   - Quản lý sao lưu (Backup) và phục hồi cơ sở dữ liệu.
