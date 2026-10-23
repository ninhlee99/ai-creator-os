# Video 2 — Kênh (`/accounts`)

**Mục tiêu:** tạo 1 kênh, xem 3 tab Tổng quan / Autopilot / Kết nối, hiểu kênh là "đơn vị" mọi automation bám vào.

## Chuẩn bị

- Dry-run BẬT.

## Kịch bản

**Bước 1 — Tạo kênh.**
[Thao tác: Kênh → "Thêm kênh", nhập tên kênh demo, nền tảng TikTok, lưu.]
"Muốn máy chạy cho kênh nào thì tạo kênh đó ở đây. Mỗi kênh có vòng đời riêng: mới, đang chạy, tạm dừng… máy chỉ chuyển trạng thái theo luật cho phép, không nhảy bừa."

**Bước 2 — Tab Tổng quan.**
[Thao tác: mở kênh vừa tạo, tab Tổng quan.]
"Tab Tổng quan hiện trạng thái kênh, persona — tức 'tính cách' nội dung của kênh — và 20 quyết định liên quan gần nhất."

**Bước 3 — Tab Autopilot.**
[Thao tác: chuyển tab Autopilot.]
"Theme và mức hoa hồng tối thiểu được lưu làm bộ lọc. Producer đang tạm dừng trước bước tìm sản phẩm vì chưa có xác nhận dùng Google Flow credits theo kênh. Lưu cấu hình không tìm sản phẩm hay tạo video."
[Thao tác: chọn theme nếu cần, bật autopilot và lưu; xem dòng trạng thái producer tạm dừng. Không có nút chạy ngay trong giao diện hiện tại.]
"Nếu bật, cấu hình rảnh tay được lưu nhưng luồng sản xuất chưa chạy. Đổi theme cũng không thay Growth plan 30 ngày."

**Bước 4 — Upload ảnh tham chiếu.**
[Thao tác: tab Autopilot, upload 1 ảnh người mẫu demo.]
"Ảnh được lưu theo kênh cho những luồng Studio tương thích. Nó không bảo đảm giữ nguyên diện mạo ở mọi khung hình và không phải job nào cũng dùng ảnh này."

**Bước 5 — Tab Kết nối.**
[Thao tác: chuyển tab Kết nối.]
"Tab Kết nối hiển thị cấu hình và credential cục bộ. Badge không kiểm tra scope, quyền đăng hay publish thành công; luồng có gate sẽ dừng hoặc bỏ qua khi thiếu dữ liệu cần thiết."

**Bước 6 — Kiểm tra trạng thái OAuth.**
[Thao tác: mở `/publishers`; xem các credential indicator, không nhập credential demo.]
"Các badge phản ánh cấu hình/token cục bộ; chưa chứng minh API, scope, quyền đăng hay publish thành công. Xác minh quyền thật cần OAuth và một lần gọi API phù hợp."

## Lưu ý trung thực

- Không gọi Autopilot là luồng rảnh tay đang hoạt động: producer hiện tạm dừng.
- Ảnh tham chiếu chỉ áp dụng cho luồng tương thích; không gọi là khóa mặt.
- Không demo đăng thật khi chưa xác minh OAuth/API — credential badge chưa đủ bằng chứng.
