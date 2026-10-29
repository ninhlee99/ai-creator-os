# Quản lý Sản phẩm Affiliate

Trang **Sản phẩm** (`/products`) phục vụ thu thập sản phẩm affiliate và khởi chạy trực tiếp các workflow tạo video quảng bá trên TikTok Shop, Shopee, Lazada, Tiki, Sendo...

## Luồng vận hành (Đã tinh gọn)

1. **Thêm sản phẩm nhanh bằng 1 Link duy nhất**:
   - Người dùng dán link sản phẩm (Shopee, TikTok Shop, Lazada, Tiki, Sendo...).
   - Hệ thống tự động fetch và bóc tách thông tin qua HTTP & Headless Crawler: danh sách nhiều ảnh sản phẩm (multi-image), tiêu đề, khoảng giá, hoa hồng ước tính.
   - Sản phẩm tự động lưu vào cơ sở dữ liệu và hiển thị trên **Kệ hàng** ngay lập tức.
   - Loại bỏ hoàn toàn các bước rườm rà (không còn các tab "Khám phá", "Chờ duyệt", "Đã loại bỏ"). Người dùng nhập link là hệ thống lưu vào kho.

2. **Xem chi tiết sản phẩm (Modal Preview)**:
   - Nút **Xem chi tiết** mở modal xem toàn bộ danh sách ảnh đã bóc tách, tiêu đề, khoảng giá và link gốc.
   - Cho phép thêm nhanh ảnh mới vào sản phẩm bằng cách dán URL ảnh trực tiếp.

3. **Khởi chạy Tạo Video 1-Click ngay tại Sản phẩm**:
   - Mỗi sản phẩm trên kệ hàng có nút **`⚡ Tạo video`**.
   - Bấm nút sẽ mở modal tinh gọn cho phép:
     - Chọn kênh TikTok đích (@account).
     - Chọn Workflow thực thi (mặc định: `preset_affiliate_daily` - Kling 3.0 & Veo).
     - Chọn ảnh nhân vật đại diện kênh hoặc tải lên nhiều ảnh góc mặt mới (multi-upload).
     - Gửi toàn bộ ảnh sản phẩm vào workflow render.
   - Bấm **`Tạo video`**: Tác vụ được thêm ngay vào hàng đợi (Jobs) và tự động thực thi.

## Cấu trúc hiển thị

- **Kệ hàng tinh gọn**: Bảng hiển thị ảnh thumbnail, tên sản phẩm, danh mục, khoảng giá VNĐ, hoa hồng ước tính và các nút hành động (Xem chi tiết, Tạo video). Loại bỏ các cột điểm số/trạng thái không cần thiết.
- **Link gốc**: Mở trực tiếp trang sản phẩm để kiểm tra thông tin thực tế.
