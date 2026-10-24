# AccessTrade cho video affiliate

## Dữ liệu sử dụng

AccessTrade là nguồn offer affiliate hiện được nối. Client đọc campaign, datafeed, tracking link và order. Datafeed có thể trả title, ảnh, giá, campaign, link và commission.

Code hiện không có nguồn đáng tin cho sales theo kỳ hoặc rating/review của từng sản phẩm. UI không được gọi sản phẩm bán chạy/đánh giá tốt nếu provider không cung cấp bằng chứng đó.

## Luồng

1. Cấu hình AccessTrade API key.
2. Tải campaign/datafeed hoặc chạy product hunt.
3. Sắp xếp theo commission và các field thật có trong offer.
4. Chọn offer, kiểm tra ảnh/URL/dữ kiện.
5. Tạo link tracking và gắn vào flow affiliate.
6. Đồng bộ order để xem trạng thái commission provider trả về.

Với offer TikTok Shop đã duyệt, lúc tạo job affiliate hệ thống tạo tracking link mới cho video đó: `sub1=product-{id}`, `sub2={job ID}`. Bảng đơn hiển thị `sub1` và `sub2` AccessTrade trả về; `sub2` mở đúng job nếu job còn được lưu. Growth ghép các đơn đã sync theo đúng `sub2`, tách pending/approved/rejected và chỉ cộng commission approved. Cần dùng đúng link và xác minh trên đơn thật trước khi xem attribution là hoạt động. TikTok Shop product card vẫn gắn riêng trong TikTok.

## An toàn

- Không log hoặc render API key.
- Pending order không tính là commission đã duyệt.
- Commission rate trên offer không phải số tiền đã trả.
- Thiếu field không được thay bằng 0 hay dữ liệu ước tính.
