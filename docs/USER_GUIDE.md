# Hướng dẫn vận hành

Hướng dẫn vận hành quy trình TikTok đang được hoàn thiện và workflow canvas.

## Chuẩn bị

1. Chạy app trên máy có FFmpeg.
2. Tạo kênh TikTok trong trang Kênh.
3. Kết nối TikTok tại trang Publisher bằng OAuth.
4. Cấu hình AccessTrade trong Cài đặt nếu dùng video affiliate.
5. Nếu Studio cần Web AI, mở Cài đặt → Hệ thống → Tài khoản AI và bấm Đăng nhập trên desktop. Tài khoản AI đang kết nối không chứng minh có credits miễn phí.

## Video affiliate TikTok

1. Mở Sản phẩm & hoa hồng; tải/cập nhật offer từ nguồn đã kết nối.
2. Kiểm tra commission và thông tin offer. Không dựa vào số bán/rating nếu nguồn không cung cấp.
3. Mở từng video trong TikTok Radar. Chỉ tick xác nhận nếu đã xem và nội dung thực sự có vũ đạo; bỏ xác nhận nếu video không phù hợp.
4. Tạo video affiliate trong Studio. Xác nhận sản phẩm, audio nguồn, preview và QC.
5. Xem lại preview, xác nhận tài khoản đích và đồng ý gửi TikTok Draft; mở TikTok để kiểm tra/gắn sản phẩm rồi hoàn tất đăng.

## Video viral TikTok

1. Mở TikTok Radar và cập nhật feed. Cách chuẩn bị browser và giới hạn dữ liệu: [TikTok Web Crawler](features/tiktok-web-crawler.md).
2. Mở video xem nội dung thật rồi xác nhận vũ đạo trên card. Chỉ video đã xác nhận mới có hành động dùng làm nguồn.
3. Tạo video từ nguồn đã chọn.
4. Kiểm tra audio, hình ảnh, preview và QC.
5. Xem lại preview, xác nhận tài khoản đích và đồng ý gửi TikTok Draft; hoàn tất đăng trong TikTok.

## Workflow truyện audio

1. Mở Workflow và chọn preset **Truyện audio · Viết & kể**. Preset hệ thống chỉ đọc; nhân bản để sửa prompt hoặc cấu hình.
2. Bấm Chạy workflow. Nhập ý tưởng, chọn phong cách giọng đọc và tải nhạc nền tùy chọn cho lần chạy này.
3. Xem lại node **Viết truyện audio** nếu cần đổi cách viết; node **Tổng hợp giọng kể** dùng chuỗi TTS trong Cài đặt. Persona giọng phụ thuộc provider; macOS Say hiện dùng giọng Linh.
4. Node cân bằng âm thanh tự giảm nhạc khi có lời, lặp nhạc tới hết track chính và chuẩn hóa đầu ra. Không tải nhạc thì node truyền nguyên lời kể. Kết quả gồm kịch bản văn bản và file WAV trong thư mục output.

## Lặp workflow con qua danh sách

1. Tạo workflow con, thêm node **Đầu vào workflow** cho một phần tử và node **Đầu ra workflow** cho kết quả; nối các node xử lý giữa hai cổng rồi lưu.
2. Trong workflow cha, nối danh sách vào **Lặp qua danh sách (Workflow Map)**. Chọn workflow con, input nhận từng phần tử và output muốn thu về; có thể chọn thêm input kiểu số để nhận thứ tự phần tử.
3. Input bắt buộc khác trong workflow con phải có giá trị mặc định. Kết quả giữ nguyên thứ tự đầu vào; mặc định xử lý tối đa 20 phần tử, cấu hình được từ 1 đến 100.
4. Để ghép video, nối `results` vào cổng **Danh sách phân cảnh** của **Ghép nối phân cảnh**. Không nối đồng thời cổng danh sách và các cổng `scene_1`–`scene_3`.

## Khi flow dừng

- Chưa xác định video có vũ đạo: không dùng làm nguồn âm thanh; mở video để kiểm tra rồi xác nhận hoặc chọn video khác.
- Không tải/tách được audio: không thay bằng sound bất kỳ; chọn nguồn khác.
- Offer thiếu dữ liệu: chỉ xem dữ liệu provider thực trả về, không coi là bán chạy.
- TikTok trả lỗi hoặc timeout sau submit: kiểm tra trạng thái Draft trước khi gửi lại.
