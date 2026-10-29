# UI/UX Blueprint

## 1. Cấu trúc Điều hướng (Sidebar Navigation)

Thanh điều hướng chính được tổ chức mạch lạc thành 3 nhóm tính năng theo nghiệp vụ thực tế, cùng khu vực giám sát tài nguyên ở chân sidebar:

- **STUDIO & VIDEO**:
  - `Tạo video` (`/`): Trung tâm sản xuất video bằng 1 câu lệnh, nhận diện tự động affiliate/viral.
  - `Workflow` (`/workflow`): Canvas trực quan kéo-thả quy trình sản xuất node-based.
  - `Lịch sử` (`/studio/jobs`): Giám sát tiến độ job render và video thành phẩm.
- **AFFILIATE**:
  - `Feed` (`/studio/trends`): Khám phá video và âm thanh thịnh hành tại Việt Nam.
  - `Sản phẩm` (`/products`): Thêm sản phẩm nhanh bằng 1 link, bóc tách nhiều ảnh, xem chi tiết và tạo video 1-click.
  - `Kênh` (`/accounts`): Quản lý kênh TikTok, nhân vật đại diện, token kết nối.
- **VẬN HÀNH**:
  - `Tăng trưởng` (`/growth`): Phân tích hiệu suất kênh và số liệu tương tác.
  - `Cài đặt` (`/settings`): Cấu hình tài khoản Web AI, TikTok social session, Custom API keys, an toàn và backup.
- **CHÂN SIDEBAR (Hạ tầng)**:
  - Giám sát RAM, CPU & Ổ cứng (Disk) thời gian thực, tự động cập nhật mỗi 4 giây kèm cảnh báo tải cao.

## 2. Tiêu chuẩn Thiết kế UI/UX

1. **Ngắn gọn & Tập trung (No Fluff)**:
   - Loại bỏ toàn bộ text thừa, tiêu đề lặp lại (redundant headers) trên các thanh tab.
   - Thống nhất thuật ngữ giao diện: Sử dụng **`Tạo video`** (loại bỏ cách gọi rườm rà như `Tạo video AI`).
   - Nút hành động chính đồng nhất: `Tạo video`, `Sử dụng mẫu`, `Sử dụng nhạc`.
2. **Loại bỏ tính năng không còn sử dụng**:
   - Loại bỏ các thẻ thống kê stat cards và chức năng Nhập/Xuất JSON tại trang Cài đặt.
   - Loại bỏ tab cấu hình Accesstrade; thay thế bằng cơ chế nhập trực tiếp link sản phẩm (Shopee, TikTok Shop, Lazada, Tiki, Sendo...).
   - Loại bỏ các tab "Khám phá", "Chờ duyệt", "Đã loại bỏ" và các cột score/status tại trang Sản phẩm.
   - Loại bỏ nút "Phân tích Storyboard", nút "Dán" và luồng `/viral` cũ.
   - Loại bỏ hoàn toàn các lựa chọn upscale 4K/8K không thực tế, giới hạn trần tối đa 2K để bảo vệ tài nguyên máy.
3. **Thao tác 1-Click & Linh hoạt**:
   - Thêm sản phẩm chỉ cần 1 link duy nhất, tự bóc tách nhiều ảnh (multi-image) và lưu vào kho.
   - Cho phép xem chi tiết sản phẩm và thêm link ảnh bổ sung qua modal trực quan.
   - Tạo video đa dạng thể loại trong một màn hình duy nhất thông qua việc tự động nhận diện câu lệnh hoặc chọn Workflow.
