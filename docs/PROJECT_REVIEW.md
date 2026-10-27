# Rà soát dự án

Cập nhật: 2026-10-27. Rà tĩnh source và tài liệu; chưa xác minh bằng tài khoản/provider thật.

## Đã xử lý trong source

- Radar dùng Web Crawler làm nguồn TikTok. Crawler không phân loại dance; video mới cần người vận hành xác nhận, URL chưa có trong Radar không được tự xác nhận.
- Chọn video dance và lấy audio bị chặn nếu nguồn chưa được xác nhận; track audio được kiểm tra trong file đầu ra.
- Màn hình Sản phẩm tách khám phá/duyệt khỏi vận hành Affiliate. Growth tách Tổng quan khỏi Vận hành.
- Workflow có input/output contract, gọi workflow con theo loại Affiliate, TikTok Viral, Thời trang hoặc Custom; contract sai, thiếu hoặc thay đổi sẽ báo lỗi khi chạy.
- Scaffold 3D/VFX giả đã gỡ. Loại workflow và kiểu cổng không được backend hỗ trợ cũng đã gỡ khỏi UI.

## Còn cần xác minh ngoài source

1. **Radar:** crawler phụ thuộc browser cục bộ và cấu trúc trang TikTok không ổn định; quyền crawl production chưa được xác nhận.
2. **Video/audio:** kiểm tra end-to-end với video thật để chứng minh audio đúng nguồn đã chọn và còn trong MP4 cuối.
3. **Affiliate evidence:** AccessTrade feed hiện tại không chứng minh doanh số theo kỳ, rating hoặc review từng sản phẩm. UI phải giữ các trường này là unknown khi thiếu nguồn.
4. **TikTok Draft:** xác minh OAuth scope, submit, trạng thái Draft và xử lý timeout với tài khoản/app thật. Kết quả không rõ cần đối chiếu trước retry.
5. **Growth:** xác minh Display API scopes và metrics bằng tài khoản thật; không suy ra retention/demographics khi API không trả.

## Feedback UI/UX

- **Radar:** trạng thái “chưa xác nhận” và hành động xác nhận cần dễ nhận ra trên từng video; metadata/caption không đủ để người dùng kết luận có dance.
- **Sản phẩm:** phân tách Khám phá và Affiliate giúp giảm lẫn thao tác; nên giữ vị trí tab khi quay lại sau sync hoặc duyệt.
- **Workflow:** để tái sử dụng, người dùng phải lưu workflow con và khai báo ít nhất một input cùng một output. Canvas cần giúp nhận biết contract chưa đủ trước khi họ tạo node gọi workflow.
- **Workflow:** khi contract workflow con đổi, node gọi cần tải lại để cập nhật cổng; lỗi hiện tại phải chỉ rõ workflow/key cần sửa.
- **TikTok Draft:** trạng thái gửi cần nêu rõ Draft đã được tạo hay còn cần người dùng hoàn tất trong TikTok; không gộp với public publish.

## Phase tiếp

Truyện audio chỉ mở sau khi hai flow TikTok đạt acceptance trong PRODUCT_SPEC.
