# Studio TikTok

Studio quản lý tạo video affiliate và video viral TikTok.

## Radar và nguồn

TikTok Radar lấy feed video Việt Nam qua TikWM khi không nhập chủ đề; có chủ đề thì gọi endpoint tìm video theo keyword của TikWM. Metadata và lịch sử chỉ số lưu trong SQLite. Kết quả tìm kiếm topic vẫn cần xác nhận live trên provider; Radar không tự phân loại video nhảy. Người dùng xác nhận video nhảy làm nguồn. Mọi audio dùng trong hai flow phải lấy từ video nhảy đã xác nhận, được Radar thu thập trong 7 ngày; không dùng sound thay thế tự động.

## Affiliate

Lập shot plan, cung cấp/generate clip chuyển động, trích audio từ video Radar, dựng và QC video sản phẩm. Flow bị chặn nếu không tải được nguồn hoặc audio.

## Viral

Dùng video TikTok Radar đã chọn làm reference; lưu URL/ID nguồn trên job và giữ audio tương ứng.

## Provider jobs

Muse AI/Google Flow có thể hỗ trợ một số media generation. Session và credits phụ thuộc provider; không được mô tả là miễn phí nếu chưa xác minh. Lỗi sau submit cần đối chiếu trước retry.

## Hoàn tất

Job chỉ xong khi artifact qua validation/QC. TikTok Draft cần người dùng duyệt trong app TikTok; không hiện như public post khi chưa có xác nhận thật.
