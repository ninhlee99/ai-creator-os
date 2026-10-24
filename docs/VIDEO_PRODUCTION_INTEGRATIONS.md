# Tạo video qua Web AI

Web AI là nguồn hỗ trợ hai flow TikTok trong Studio.

## Đăng nhập

Tài khoản được mở trong Chromium cô lập; app không đọc profile browser cá nhân. Chỉ login desktop. Trạng thái session không xác nhận plan miễn phí, quota hoặc credits.

## Sử dụng trong Studio

- Chỉ gọi provider đã được người vận hành bật.
- Không gửi yêu cầu tính phí.
- Lưu provider/account/job ID theo artifact để xử lý lỗi và chống submit trùng.
- Lỗi sau khi provider có thể đã nhận prompt phải dừng và đối chiếu; không thử account khác ngay.
- Kết quả tải xuống phải qua kiểm tra file trước khi vào pipeline.

## Giới hạn

Khả năng tạo, quyền dùng, chi phí và credits phụ thuộc tài khoản/provider. MiuNova không hứa provider luôn miễn phí hoặc luôn khả dụng. Xem [trạng thái triển khai](IMPLEMENTATION_STATUS.md).
