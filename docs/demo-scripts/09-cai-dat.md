# Video 9 — Cài đặt (`/settings`, 6 tab)

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

**Tab 3 — Reup.**
[Thao tác: chuyển tab Reup.]
"Tab này đã quay ở video Reup: mức transform, voiceover, tự đăng, video mỗi ngày, kill rule, warm-up — và card Quét nguồn: bật/tắt, mấy giờ quét một lần, mấy video một nguồn."

**Tab 4 — Nhà cung cấp.**
[Thao tác: chuyển tab Nhà cung cấp, chỉ vào chuỗi TTS và keyring.]
"Tab Nhà cung cấp: thứ tự ưu tiên giọng đọc — Gemini trước, VieNeu local sau, Edge cuối. Key thì quản lý theo vòng: hiện 4 số cuối, key nào lỗi thì tự bỏ qua, key nào bị giới hạn thì nghỉ theo thang 1 phút → 5 phút → 15 phút rồi thử lại. Bạn có 10 key thì nó xoay cả 10."

**Tab 5 — Model local.**
[Thao tác: chuyển tab Model local.]
"Chạy AI trên máy bạn: VieNeu TTS và llama-server — mỗi cái một nút bấm để tải và bật, app tự kiểm tra. Dưới này là kiểm tra công cụ: FFmpeg có chưa, chưa thì hiện đúng một dòng lệnh cài."

**Tab 6 — An toàn.**
[Thao tác: chuyển tab An toàn.]
"Tab quan trọng nhất: dry-run — bật thì mọi thao tác thật đều thành mô phỏng. Và kill switch — nút đỏ dừng toàn bộ daemon ngay lập tức. Mọi lần bấm đều ghi vào nhật ký."

**Kết.**
[Thao tác: về Trang chủ.]
"Chín video là toàn bộ app. Nguyên tắc của nó: bạn chỉ mở máy, giữ mạng, start app — còn lại máy tự lo."

## Lưu ý trung thực

- Nói rõ công tắc chính mặc định TẮT lúc mới cài (an toàn).
- API nội bộ `/api/*` mặc định TẮT.
- Không hứa key free "không bao giờ hết quota" — hết thì job chờ, tick hôm sau chạy tiếp.
