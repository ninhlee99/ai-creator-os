# Điều Hướng & Giám Sát Hệ Thống

Ứng dụng khởi động trực tiếp vào **Workflow Studio (`/workflow`)** — không dùng dashboard trung gian rườm rà.

- **Trang chủ (`/`)**: Tự động điều hướng trực tiếp vào **Workflow Canvas (`/workflow`)** để người dùng sáng tạo video AI ngay khi mở app.
- **Giám sát RAM & CPU Toàn diện (Chân Sidebar trên mọi trang)**:
  - Tự động cập nhật mỗi 4 giây qua HTMX (`GET /api/system/sidebar-metrics`).
  - Đo lường RAM thời gian thực (Đã dùng, Còn trống, Phần trăm, Mức cảnh báo OOM).
  - Đo lường CPU Load Average và số nhân CPU khả dụng.
  - Hiển thị trực quan node workflow đang thực thi thực tế (e.g. `video_upscale` 2K, `face_swap`).
  - Cảnh báo trực quan 3 cấp độ: 🟢 ỔN ĐỊNH, 🟡 TẢI CAO, 🔴 OOM NGUY CƠ để bảo vệ an toàn máy Mac.
- **Quản lý & Dừng Tiến Trình Nặng (Process Killer)**:
  - Hỗ trợ API an toàn (`POST /api/system/kill-process`) để dừng khẩn cấp các tiến trình nền nặng hoặc crawler zombie.
