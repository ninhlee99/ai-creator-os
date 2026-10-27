# Rà soát dự án

Cập nhật: 2026-10-24. Phạm vi rà soát: hai flow TikTok đang ưu tiên.

## Kết luận

Chưa đạt flow end-to-end. Các điểm rủi ro tập trung ở chọn đúng video nhảy, bảo toàn audio nguồn, độ tin cậy của offer affiliate và trạng thái gửi Draft.

## Thứ tự xử lý

1. **Radar dance:** Web Crawler hiện không phân loại dance nhưng backend tự đánh dấu mọi kết quả là đã xác minh; UI cũng không có control để review. Không dùng cờ này làm bằng chứng hoặc auto-select audio.
2. **Audio:** xác minh audio được trích từ đúng video Radar đã chọn và tồn tại trong MP4 cuối.
3. **Affiliate evidence:** AccessTrade cung cấp offer/commission nhưng chưa có sales/review evidence cần để gọi sản phẩm bán chạy hoặc đánh giá tốt.
4. **Video QC:** kiểm tra sản phẩm, chuyển động, tỉ lệ, audio và file trước khi cho gửi Draft.
5. **TikTok Draft:** xác minh OAuth scope, submit, lỗi sau submit và trạng thái draft trên account thật; không retry mù.
6. **Growth data:** kết nối metrics TikTok thật sau khi publish flow ổn định.

## Phase tiếp

Truyện audio chỉ mở sau khi hai flow TikTok đạt acceptance trong PRODUCT_SPEC. Không mở thêm flow sản phẩm trước đó.
