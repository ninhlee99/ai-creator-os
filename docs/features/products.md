# Quản lý Sản phẩm Affiliate

Trang **Sản phẩm** (`/products`) phục vụ thu thập sản phẩm affiliate và khởi chạy trực tiếp các workflow tạo video quảng bá trên TikTok Shop, Shopee, Lazada...

## Luồng vận hành (Đã tinh gọn)

1. **Thêm sản phẩm nhanh bằng 1 Link Affiliate duy nhất**:
   - Người dùng dán link affiliate sản phẩm (Shopee, TikTok Shop, Lazada...).
   - Hệ thống tự động fetch và bóc tách thông tin: ảnh sản phẩm, tiêu đề, khoảng giá, hoa hồng ước tính.
   - Sản phẩm tự động lưu vào cơ sở dữ liệu và hiển thị trên **Kệ hàng** ngay lập tức.
   - Loại bỏ hoàn toàn các bước rườm rà trước đây (không còn các tab "Khám phá", "Chờ duyệt", "Đã loại bỏ"). Người dùng đã chủ đích nhập link là hệ thống chấp nhận lưu vào kho.

2. **Khởi chạy Tạo Video 1-Click ngay tại Sản phẩm**:
   - Mỗi sản phẩm trên kệ hàng có nút **`⚡ Tạo video`**.
   - Bấm nút sẽ mở modal tinh gọn cho phép:
     - Chọn kênh TikTok đích (@account).
     - Chọn Workflow thực thi (mặc định: `preset_affiliate_daily` - Kling 3.0 & Veo AI).
     - Chọn ảnh nhân vật đại diện kênh hoặc tải lên nhiều ảnh góc mặt mới (multi-upload).
   - Bấm **`🚀 Khởi chạy Workflow Tạo Video`**: Tác vụ được thêm ngay vào hàng đợi (Jobs) và tự động thực thi.

## Cấu trúc hiển thị

- **Kệ hàng**: Danh sách bảng hiển thị ảnh thumbnail, tên sản phẩm, danh mục, khoảng giá VNĐ, hoa hồng ước tính, trạng thái và nút hành động.
- **Link gốc**: Link mở trực tiếp trang sản phẩm để kiểm tra thông tin thực tế.
