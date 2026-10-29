# UI/UX Blueprint

## 1. Cấu trúc Điều hướng (Sidebar Navigation)

Thanh điều hướng chính được tổ chức mạch lạc thành 5 nhóm tính năng:

- **TỔNG QUAN**:
  - `Dashboard` (`/`): Tổng quan số liệu, trạng thái hệ thống, biểu đồ trực quan hóa.
- **TIKTOK**:
  - `Xu hướng` (`/studio/trends`): Khám phá video và sound thịnh hành tại Việt Nam.
- **AFFILIATE**:
  - `Sản phẩm` (`/products`): Thêm sản phẩm nhanh bằng link affiliate, quản lý kệ hàng và tạo video 1-click.
- **WORKFLOW**:
  - `Workflow` (`/workflow`): Canvas trực quan kéo-thả quy trình sản xuất AI.
  - `Tạo video` (`/workflow/create`): Studio tạo video đa thể loại điều khiển theo workflow.
  - `Lịch sử` (`/studio/jobs`): Giám sát tiến độ job render và video thành phẩm.
- **HỆ THỐNG**:
  - `Tăng trưởng` (`/growth`): Phân tích hiệu suất kênh và video.
  - `Kênh` (`/accounts`): Quản lý kênh TikTok, nhân vật đại diện, token kết nối.
  - `Cài đặt` (`/settings`): Cấu hình Web AI, chuỗi nhà cung cấp, an toàn và backup.

## 2. Tiêu chuẩn Thiết kế UI/UX

1. **Ngắn gọn & Tập trung (No Fluff)**:
   - Loại bỏ toàn bộ text thừa, câu giải thích dài dòng hoặc hướng dẫn thao tác cơ bản làm rối mắt người dùng.
   - Sử dụng microcopy ngắn gọn, chuẩn giao diện ứng dụng web hiện đại.
2. **Loại bỏ tính năng không còn sử dụng**:
   - Loại bỏ tab cấu hình Accesstrade khỏi menu Cài đặt.
   - Loại bỏ các tab "Khám phá", "Chờ duyệt", "Đã loại bỏ" ở trang Sản phẩm.
   - Loại bỏ nút "Dán", nút "Phân tích Storyboard" và nhãn thuật ngữ "Radar" ở màn hình Tạo video.
3. **Thao tác 1-Click & Linh hoạt**:
   - Thêm sản phẩm chỉ cần 1 link affiliate duy nhất, tự bóc tách dữ liệu và lưu vào kho.
   - Tạo video đa dạng thể loại trong một màn hình duy nhất thông qua việc chuyển đổi Workflow.
   - Hỗ trợ tải lên cùng lúc nhiều góc ảnh nhân vật / sản phẩm (Multi-upload) để AI nhận diện tối ưu.
   - Node Upscale hỗ trợ nâng cấp chất lượng hình ảnh / video tối đa lên độ phân giải 2K.
