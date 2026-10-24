# Video affiliate TikTok

## Flow

1. Chỉ chọn offer đã duyệt; giữ link và dữ kiện nguồn.
2. Lập shot plan theo listing và ảnh nhân vật của kênh.
3. Chọn video nhảy đã xác nhận trong TikTok Radar, được thu thập lại trong 7 ngày gần nhất, làm nguồn âm thanh.
4. Tạo từng shot bằng clip chuyển động đã cung cấp hoặc provider được phép.
5. Trích audio từ đúng video Radar, ghép vào output và lưu provenance.
6. QC hình, sản phẩm, audio và file; sau đó gửi TikTok Draft.

## Hiện trạng

Shot plan, tải clip, dựng video và một số QC đã có. Manual flow còn cần clip chuyển động cho mỗi shot. Provider generation có thể dùng credits; consent trong UI không chứng minh dịch vụ miễn phí. Chưa chứng minh affiliate video miễn phí end-to-end.

AccessTrade TikTok Shop feed trả commission, giá và units sold khi publisher được chia sẻ số liệu; không trả rating/review. Chỉ số có nguồn mới được dùng; rating và fit thị trường giữ chưa biết nếu không có dữ liệu.

Radar lưu sản phẩm mới ở trạng thái chờ duyệt. Sản phẩm bị loại giữ cùng lý do trong danh sách riêng. Studio từ chối lập shot plan từ product ID chưa duyệt. Sales/rating thiếu nguồn hoặc hết hạn chặn sản xuất tự động. Nguồn audio phải là video nhảy đã xác nhận và Radar đã thu thập trong 7 ngày; không tự rơi về sound cũ.

TikTok product attachment chưa được nối qua API; draft cần người dùng gắn sản phẩm trong TikTok. Job hiển thị tên/ID listing, chỉ dẫn gắn product card, link AccessTrade riêng để sao chép và mã `sub2` đối chiếu đơn. Link tracking không thay thế product card.

Với offer TikTok Shop từ AccessTrade, mỗi job affiliate tạo một tracking link riêng. `sub1` giữ product ID, `sub2` trùng job ID; mã và link hiển thị trên job, còn `sub2` AccessTrade trả về mở đúng job khi job còn được lưu. Đơn chỉ quy được về video khi bài đó dùng đúng link và AccessTrade trả lại `sub2`. TikTok Shop product card vẫn phải gắn thủ công trong Creator.

Sau khi tự đăng video công khai, dán URL vào job hoàn tất để Growth ghép counters TikTok theo account/video ID. TikTok Display API không cung cấp orders hoặc retention; số thiếu giữ unknown. Doanh số chỉ đối chiếu qua báo cáo AccessTrade có `sub2` đúng job.
