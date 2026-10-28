# Web AI dùng trong Studio

Web AI cung cấp account Muse AI/Google Flow cho tác vụ tạo media thuộc hai flow TikTok.

## Login và trạng thái

- Đăng nhập mở Chromium với profile tạm cô lập; app không đọc profile browser cá nhân.
- Tác vụ headless dùng profile riêng theo account trên Windows/Linux để giữ `localStorage`/IndexedDB; macOS dùng profile tạm riêng mỗi lần chạy để tránh Edge/Chrome full dùng chung browser process/profile. Cache tăng trưởng được dọn sau mỗi lần chạy.
- Toàn app chỉ cho một Chromium automation process chạy tại một thời điểm; workflow, Muse, Web AI và Radar chờ cùng slot, hủy chờ theo context. Điều này giới hạn cả browser process tree, không chỉ số workflow.
- `AICOS_EPHEMERAL_BROWSER_PROFILE=1` tắt profile bền. `AICOS_BROWSER_MAX_HEAP_MB` đặt giới hạn V8 từ 256–4096 MiB (mặc định 512); chỉ tăng khi tác vụ thực tế chạm giới hạn. Chỉ đặt `AICOS_BROWSER_DISABLE_DEV_SHM=1` trong container thiếu `/dev/shm`; flag này chuyển shared memory sang đĩa tạm.
- CDP dùng cổng động loopback; headless render gửi output tới null device, login chỉ giữ tail stderr tối đa 16 KiB. Shutdown dừng cả process tree trước khi đợi process chính.
- Session connected không chứng minh quyền generation, gói miễn phí hoặc credits còn lại.
- Không retry qua account khác nếu request có thể đã được provider nhận.

## Cấu hình JSON

JSON export ở màn API Key không chứa credential hoặc Web AI session. Backup ZIP cũng bỏ `webai.db`; sau khi restore sang máy mới, đăng nhập lại Web AI. Import vẫn nhận cấu hình legacy có credential; chỉ nhập file tin cậy và xoay key nếu file từng bị lộ.

## Giới hạn

Provider web APIs có thể đổi mà không báo trước. Chỉ dùng tác vụ/provider được phép và chi phí bằng 0; nếu giá/credits không rõ, dừng trước khi submit.
