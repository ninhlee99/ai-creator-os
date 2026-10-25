# Sản phẩm & hoa hồng

Trang Products phục vụ video affiliate TikTok.

## Luồng

- Radar dùng TikTok Shop product feed qua AccessTrade, tìm theo từ khóa của category và sort hoa hồng cao.
- Offer mới được lưu vào danh sách chờ duyệt; giá nhiều SKU hiển thị theo khoảng.
- Xem ảnh, giá, commission, campaign và affiliate link có trong nguồn.
- Ưu tiên hiển thị `commission.amount` và tiền tệ do AccessTrade trả; nếu thiếu, mới ước tính theo rate × giá. Ước tính theo dải giá hiển thị khoảng thấp–cao.
- Duyệt đưa offer vào kệ; loại bỏ yêu cầu lý do và giữ offer trong tab riêng.
- Lọc/sắp xếp theo commission và field được provider trả về.
- Tạo hoặc sao chép tracking link.
- Xem order sync cùng trạng thái pending/approved/rejected.

## Giới hạn

AccessTrade feed trả giá, commission amount/currency/rate và có thể trả units sold; units sold có thể vắng nếu thiếu quyền chia sẻ dữ liệu. Feed không trả rating/review. Offer mới chờ duyệt; link affiliate TikTok Shop được tạo khi duyệt. Chỉ offer đã được duyệt, đủ giá/tiền tệ/hoa hồng và có link hợp lệ mới vào sản xuất. Thiếu sales hoặc rating vẫn hiện là khoảng trống bằng chứng để người duyệt cân nhắc; app không tự suy đoán hoặc tự loại offer.

Fit xu hướng TikTok, tệp giới trẻ và mức độ viral chưa được chấm tự động. UI phải để các mục này ở trạng thái chưa có dữ liệu thay vì tự suy đoán.

## Đánh giá offer

Danh sách sản phẩm hiển thị đề xuất dựa trên độ đầy đủ của offer, hoa hồng tham khảo theo category, sales và rating có nguồn. Khi feed trả `commission.amount`, UI giữ đúng amount/currency nguồn; số đó không khẳng định payout cuối cùng. Chỉ khi thiếu amount, UI mới ước tính theo rate × giá và hiện rõ là ước tính; dải giá hiển thị dải ước tính. Đây là thông tin hỗ trợ người duyệt; không tự duyệt hoặc loại offer. AccessTrade chưa cung cấp rating/review trong feed hiện tại. UI tách số video TikTok Radar trong 7 ngày khớp tên sản phẩm khỏi số video khớp từ khóa ngành hàng, kèm video nguồn dẫn đầu. Tín hiệu nội dung không xác minh lượt bán; không có kết quả khớp vẫn chưa đủ để kết luận sản phẩm không phù hợp. Tệp tuổi tiếp tục hiện là chưa có dữ liệu cho đến khi analytics cung cấp bằng chứng phù hợp.
