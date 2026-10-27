# Hướng dẫn vận hành

Hướng dẫn tập trung hai flow TikTok đang được hoàn thiện. Truyện audio để phase sau.

## Chuẩn bị

1. Chạy app trên máy có FFmpeg.
2. Tạo kênh TikTok trong trang Kênh.
3. Kết nối TikTok tại trang Publisher bằng OAuth.
4. Cấu hình AccessTrade trong Cài đặt nếu dùng video affiliate.
5. Nếu Studio cần Web AI, mở Cài đặt → Hệ thống → Tài khoản AI và bấm Đăng nhập trên desktop. Tài khoản AI đang kết nối không chứng minh có credits miễn phí.

## Video affiliate TikTok

1. Mở Sản phẩm & hoa hồng; tải/cập nhật offer từ nguồn đã kết nối.
2. Kiểm tra commission và thông tin offer. Không dựa vào số bán/rating nếu nguồn không cung cấp.
3. Mở TikTok Radar và tự xem nội dung video. UI hiện chưa có thao tác xác nhận video nhảy; nhãn `Đã xác minh` tự bật nên không chứng minh video đã được người dùng xem.
4. Tạo video affiliate trong Studio. Xác nhận sản phẩm, audio nguồn, preview và QC.
5. Xem lại preview, xác nhận tài khoản đích và đồng ý gửi TikTok Draft; mở TikTok để kiểm tra/gắn sản phẩm rồi hoàn tất đăng.

## Video viral TikTok

1. Mở TikTok Radar và cập nhật feed; crawler cần browser cục bộ.
2. Mở video xem nội dung thật. Cờ `Đã xác minh` hiện chưa đáng tin vì backend tự đặt cho mọi kết quả; không dựa vào cờ này để kết luận đó là video nhảy.
3. Tạo video từ nguồn đã chọn.
4. Kiểm tra audio, hình ảnh, preview và QC.
5. Xem lại preview, xác nhận tài khoản đích và đồng ý gửi TikTok Draft; hoàn tất đăng trong TikTok.

## Khi flow dừng

- Chưa có luồng xác nhận video nhảy đáng tin: không dùng nhãn `Đã xác minh` làm căn cứ chọn nguồn âm thanh.
- Không tải/tách được audio: không thay bằng sound bất kỳ; chọn nguồn khác.
- Offer thiếu dữ liệu: chỉ xem dữ liệu provider thực trả về, không coi là bán chạy.
- TikTok trả lỗi hoặc timeout sau submit: kiểm tra trạng thái Draft trước khi gửi lại.
