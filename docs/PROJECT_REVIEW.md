# Rà soát dự án

Cập nhật: 2026-10-27. Rà tĩnh source và tài liệu; chưa xác minh bằng tài khoản/provider thật.

## Đã xử lý trong source

- Radar dùng Web Crawler làm nguồn TikTok. Crawler contract là nguồn chi tiết chuẩn; video cần người vận hành xác nhận trước khi dùng.
- Chọn video dance và lấy audio bị chặn nếu nguồn chưa được xác nhận; track audio được kiểm tra trong file đầu ra.
- Màn hình Sản phẩm tách khám phá/duyệt khỏi vận hành Affiliate. Growth tách Tổng quan khỏi Vận hành.
- Workflow có input/output contract, gọi workflow con theo loại Affiliate, TikTok Viral, Thời trang hoặc Custom; contract sai, thiếu hoặc thay đổi sẽ báo lỗi khi chạy.
- Scaffold 3D/VFX giả đã gỡ. Loại workflow và kiểu cổng không được backend hỗ trợ cũng đã gỡ khỏi UI.

## Còn cần xác minh ngoài source

1. **Radar:** quyền crawl production chưa được xác nhận. Hành vi và giới hạn hiện tại: [TikTok Web Crawler](features/tiktok-web-crawler.md).
2. **Video/audio:** kiểm tra end-to-end với video thật để chứng minh audio đúng nguồn đã chọn và còn trong MP4 cuối.
3. **Affiliate evidence:** AccessTrade feed hiện tại không chứng minh doanh số theo kỳ, rating hoặc review từng sản phẩm. UI phải giữ các trường này là unknown khi thiếu nguồn.
4. **TikTok Draft:** xác minh OAuth scope, submit, trạng thái Draft và xử lý timeout với tài khoản/app thật. Kết quả không rõ cần đối chiếu trước retry.
5. **Growth:** xác minh Display API scopes và metrics bằng tài khoản thật; không suy ra retention/demographics khi API không trả.

## Feedback UI/UX

- **Radar:** card nêu rõ chưa xác nhận, yêu cầu xem video; crawler không phân loại vũ đạo.
- **Sản phẩm:** Khám phá và Affiliate tách riêng; thao tác duyệt/sync giữ đúng tab.
- **Workflow:** prereq và lý do workflow chưa dùng được hiện trên node gọi; cập nhật contract hỏi trước khi xóa dây không tương thích.
- **Workflow inputs:** chọn catalog đang yêu cầu nhập `product_id`; video Radar cần dán URL. Nên có product/Radar picker để tránh nhầm ID và nguồn chưa xác thực.
- **Workflow settings:** các tham số dạng enum như provider, output type, operation, location vẫn là text field; người dùng chỉ thấy lỗi sau khi chạy. Nên dùng dropdown có nhãn và mô tả lựa chọn.
- **Custom nodes:** Universal AI, Custom Web AI và AI Prompt chồng công dụng; cần hướng dẫn chọn node theo text/image/video, kèm nhãn rõ giới hạn audio và provider.
- **Preset workflows:** ba scene render tạo canvas rộng, cần zoom/pan để theo dõi toàn luồng. Nhãn 8a/8b/8c và node concat giúp thứ tự rõ hơn.
- **TikTok Draft:** UI phân biệt Draft đã tải lên nhưng chưa công khai với bước kiểm tra/duyệt tiếp theo trong TikTok.

## Phase tiếp

Truyện audio chỉ mở sau khi hai flow TikTok đạt acceptance trong PRODUCT_SPEC.
