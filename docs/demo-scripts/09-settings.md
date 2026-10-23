# Video 9 — Cài đặt (`/settings`, 5 trang)

**Mục tiêu:** đi nhanh 6 tab, nhấn mạnh: key rotation, model local 1 nút, công tắc an toàn.

## Chuẩn bị

- Không cần key thật.

## Kịch bản

**Tab 1 — Hệ thống.**
[Thao tác: Cài đặt → tab Hệ thống.]
"Tab Hệ thống: phiên bản app, chi phí API theo từng engine — tốn bao nhiêu key nào đều ghi lại. Có nút sao lưu toàn bộ dữ liệu ra file zip và khôi phục."

**Tab 2 — Accesstrade.**
[Thao tác: chuyển tab Accesstrade.]
"Tab này đã quay ở video Affiliate: nhập key, kiểm tra kết nối, chỉnh chu kỳ săn sản phẩm và đồng bộ đơn."

**Tab 3 — Model local.**
[Thao tác: mở trang Model local.]
"Trang này cấu hình model local. Các lựa chọn hiển thị là cấu hình; trang không chứng minh model đã chạy hay tạo được nội dung."

**Tab 4 — Nhà cung cấp.**
[Thao tác: chuyển tab Nhà cung cấp, chỉ vào chuỗi TTS và keyring.]
"Tab Nhà cung cấp: LLM dùng Gemini; TTS ưu tiên Gemini rồi chuyển sang Edge nếu cần. Key Gemini được xoay vòng, key lỗi tự bỏ qua và key chạm quota sẽ nghỉ theo thang 1 phút → 5 phút → 15 phút."

**Tab 5 — An toàn.**
[Thao tác: chuyển tab An toàn.]
"Trang An toàn có dry-run và kill switch. Mỗi gate chỉ chặn các workflow áp dụng gate đó; một số sync, discovery hoặc thao tác thủ công vẫn có thể chạy. Master switch riêng chỉ điều khiển network daemon. Các lần đổi được ghi vào nhật ký."

**Kết.**
[Thao tác: về Trang chủ.]
"Các trang cài đặt lưu cấu hình và credential cục bộ. Scheduler chỉ chạy qua gate riêng; affiliate Autopilot hiện dừng trước product discovery, còn video thủ công cần đầu vào và xác nhận dùng Flow credits nếu chọn Flow."

## Lưu ý trung thực

- Nói rõ `MASTER_SWITCH` mặc định TẮT lúc mới cài và chỉ điều khiển network daemon.
- Dry-run mặc định BẬT nếu chưa có giá trị lưu; thiết lập đã lưu trong UI được ưu tiên khi khởi động.
- API nội bộ `/api/*` mặc định TẮT.
- Không hứa key free "không bao giờ hết quota" — hết thì job chờ, tick hôm sau chạy tiếp.
