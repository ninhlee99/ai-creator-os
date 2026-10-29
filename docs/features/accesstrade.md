# AccessTrade (Tính năng đã dừng hiển thị trên UI chính)

> [!NOTE]
> Tính năng tìm kiếm tự động và màn hình Cài đặt Accesstrade đã được **loại bỏ khỏi giao diện người dùng chính (UI)** theo quyết định tối ưu hóa luồng sản phẩm.
> - Người dùng hiện nhập sản phẩm trực tiếp bằng link Affiliate (Shopee, TikTok Shop, Lazada...) tại trang **Sản phẩm** (`/products`).
> - Module mã nguồn backend `internal/accesstrade` và bảng dữ liệu chỉ được duy trì ở chế độ tương thích ngược (legacy contract compatibility) phục vụ đối soát đơn cũ hoặc kiểm thử tự động, không còn xuất hiện trên thanh điều hướng Cài đặt hệ thống.

## Trạng thái hiện tại

1. **Giao diện Cài đặt**: Tab `🛒 Accesstrade` đã được gỡ bỏ hoàn toàn khỏi thanh điều hướng `/settings`.
2. **Tìm kiếm sản phẩm**: Quy trình quét feed tự động qua AccessTrade Hunter đã ngừng làm cổng vào bắt buộc; thay vào đó là tính năng phân tích trực tiếp link affiliate từ người dùng.
3. **Đối soát đơn**: Lưu lượng đối soát đơn cũ chỉ chạy nền khi có cấu hình trước đó; không hiển thị yêu cầu bắt buộc nhập API key trong quy trình thêm sản phẩm.
