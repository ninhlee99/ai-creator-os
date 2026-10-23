# Affiliate (`/products`)

## Mục đích

Tìm offer từ provider đã cấu hình, lưu catalog, quản lý kệ sản phẩm Bio, tạo
tracking link theo chiến dịch và đối soát đơn Accesstrade. Việc lên kệ không
chứng minh offer đủ demand evidence hoặc đủ điều kiện tự động tạo video.

Nguồn discovery mặc định là Accesstrade datafeed. TikTok Shop không còn là
provider discovery hoạt động; dữ liệu legacy có thể còn trong store sau migration.

## Luồng đang có

- `GET /products`: tìm trong catalog theo từ khóa/chủ đề, xem chiến dịch,
  offer đã săn, tracking links và đơn đã đồng bộ.
- `POST /products/search`: tìm trên provider đã cấu hình, lưu kết quả rồi render
  kết quả. Form gửi `keyword`, khớp handler.
- `POST /products/add`: lưu offer nhập tay. Rating/sales do người dùng nhập
  không qua automated demand gate dù có gắn URL tham chiếu.
- `POST /products/shelf/add` và `/products/shelf/toggle`: quản lý sản phẩm trên
  kệ Bio. Autopilot hiện dừng trước bước chọn sản phẩm.
- `POST /at/campaigns/sync`, `/at/links/create`, `/at/hunt` và
  `/at/orders/sync`: tải campaign, tạo link theo campaign, quét datafeed và
  đồng bộ đơn.
- Commission dashboard chỉ hiện approved/pending sau lần order sync thành
  công; chưa sync không được hiển thị thành doanh thu 0.

## Giới hạn sản phẩm hiện tại

- Datafeed hiện không cung cấp sales count/rating được parser map vào demand
  evidence. Offer được lưu để xem, nhưng AT Hunter đưa offer thiếu evidence vào
  watchlist, không tạo video.
- AT Hunter dùng `aff_link` có trong feed. Tracking link tạo riêng theo campaign
  là thao tác riêng; không gắn attribution video tự động.
- Form tìm kiếm xếp theo tỷ lệ hoa hồng và lọc theo chủ đề/mức commission.
  Không có xác nhận sales, review sample, ảnh 4K hoặc payout trong kết quả.
- Autopilot affiliate scheduler hiện gọi producer đang bị chặn cho đến khi có
  cơ chế consent dùng Google Flow credits. Bật lịch chỉ lưu cấu hình, không làm
  video chạy.
- `/products/schedule` và `/products/music` có handler nhưng chưa có form tương
  ứng trên trang Products. Dashboard chỉ cho đổi công tắc autopilot chính.
- CTA “Chuẩn bị trong Studio” mở form Studio kèm dữ liệu offer; nó không tạo job
  tự động.

## Cấu hình và an toàn

- Công tắc AT Hunter mặc định bật; vẫn cần API key, điều kiện evidence, account
  đích, ảnh nhân vật và quyền dùng provider trước khi tạo video.
- Order sync và Hunter có lịch riêng. Nhãn UI nói “lịch bật/tắt”; đây không phải
  xác nhận worker vừa chạy thành công.
- Commission theo offer là giá trị từ nguồn. Số tiền ước tính trên một đơn chỉ
  là `price × commission_rate`, không phải commission đã ghi nhận hoặc payout.
- Lỗi provider được báo theo nguồn; kết quả từ provider khác vẫn có thể được
  lưu.

## API key rotation

- Discovery và order sync dùng AccessTrade key trong settings.
- Rendering video còn phụ thuộc provider/key/quota đã cấu hình; Flow credits
  không được xem là miễn phí nếu chưa có xác nhận phù hợp.
