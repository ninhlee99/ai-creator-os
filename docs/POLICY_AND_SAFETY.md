# Policy & Safety — Nguyên tắc An toàn & Tuân thủ

Các quy tắc bất di bất dịch được lập trình trực tiếp vào hệ thống (code-level enforcement). Vi phạm bất kỳ điều nào dưới đây đều được xem là bug nghiêm trọng.

## 1. Bảo vệ tài khoản & uy tín kênh (Tài sản quý giá nhất)

- **Zero-Reup / 100% Original Assets**: Tuyệt đối nói KHÔNG với reup video của bên thứ ba. Toàn bộ kịch bản, hình ảnh MediaGen, lồng tiếng và âm nhạc đều được tổng hợp nguyên bản 100%, bảo vệ kênh không bị shadowban hoặc dính bản quyền.
- **Giới hạn tần suất đăng (Rate limiting)**: Giới hạn tối đa số video đăng mỗi ngày trên từng kênh (tự động nghỉ và dãn cách thời gian), tránh kích hoạt spam filter của TikTok / YouTube / Facebook.
- **Minh bạch AI (AI Disclosure)**: Tự động khai báo nội dung do AI tạo (Synthetic Media) khi đăng tải qua API chính thức.
- **Từ khóa đỏ & Cấm kỵ (Red-lines)**: Brand DNA tự động kiểm duyệt kịch bản, chặn đứng các từ ngữ bị cấm (hứa hẹn sai sự thật, y tế chưa kiểm chứng, xúc phạm, từ khóa nhạy cảm).

## 2. Minh bạch tiền tệ & Hoa hồng

- **Dữ liệu thật 100%**: Doanh thu, đơn hàng, hoa hồng CHỈ được ghi nhận từ API nhà cung cấp chính thức (Accesstrade Order Sync). Tuyệt đối không giả lập hay bịa số liệu.
- **Sổ cái bất biến (Append-only)**: Bảng đơn hàng và hoa hồng chỉ ghi thêm, không UPDATE/DELETE làm sai lệch lịch sử tài chính.
- **Khóa trùng lặp (Idempotency)**: Mọi hành động gửi yêu cầu ra ngoài đều có khóa chống lặp; việc thử lại không bao giờ tạo tác vụ trùng.
- **Ngân sách API**: Tuân thủ trần ngân sách hàng ngày (`DAILY_API_BUDGET_USD`).

## 3. An toàn nội dung & Sản phẩm

- **Lọc danh mục cấm**: Các ngành hàng rủi ro cao (thuốc, thực phẩm chức năng chưa kiểm duyệt, cờ bạc...) bị chặn ngay tại khâu nạp sản phẩm (`product_eligible`).
- **Gắn nhãn tiếp thị liên kết**: Đảm bảo tuân thủ chính sách affiliate của nền tảng phát hành.

## 4. Cơ chế an toàn (Fail-safes)

- **Công tắc khẩn cấp (Kill Switch)**: Dừng ngay lập tức mọi hoạt động daemon, pipeline render và autopilot chỉ sau một thao tác.
- **Chế độ diễn tập (Dry-run)**: Mặc định bật khi chưa cấp quyền; cho phép kiểm tra toàn bộ luồng mà không gửi tác vụ thật ra bên ngoài.
- **Công tắc chính (MASTER_SWITCH)**: Phải được người quản trị bật chủ động thì hệ thống mới xuất bản nội dung tự động.
- **Nhật ký quyết định (Decision Log)**: Mọi quyết định của AI và daemon đều được lưu vết trong SQLite để kiểm toán minh bạch.

## 5. Quyền riêng tư & Bảo mật

- **Không rò rỉ khóa**: API key, client secret, OAuth token chỉ lưu trong env, SQLite được phân quyền bảo mật, không bao giờ in ra log hay lộ trên UI.
- **Xác thực bảo vệ**: Bật mật khẩu bảo vệ dashboard khi chạy trên mạng LAN / môi trường ngoài.
