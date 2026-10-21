# Create vertical videos (`/viral`)

Trang `/viral` có YouTube Viral Radar và form tạo video 9:16. Radar dùng YouTube Data API tìm video 7 ngày gần nhất, xếp theo lượt xem, lọc kênh từ 100,000 subscribers. Tự động quét tối đa một lần mỗi 24 giờ; nút `Refresh radar` chạy ngay và có thể xếp một job video gốc cục bộ nếu auto production đang bật.

Radar cần `YOUTUBE_API_KEY` tại `/settings/system`. Mỗi lần quét dùng một `search.list` call cùng tối đa một `videos.list` và một `channels.list`. Kết quả lưu trong ledger settings dưới `viral.radar.sources`.

Job tự động dùng ảnh mẫu do người dùng tải lên và dựng cục bộ; không gọi API tạo video. Link nguồn chỉ lưu để truy vết. Footage và audio nguồn không được tải hoặc dùng lại. Video đầu ra dài ít nhất bằng thời lượng video nguồn, với thời gian render tự tăng cho nguồn dài. Tác vụ lưu trong Studio, theo dõi tại `/studio/jobs`.

Luồng chính nằm trong `internal/viralradar/youtube.go`, `internal/automation/viral_radar.go`, `internal/web/viral.go` và `internal/studio/viral.go`. Không có YOUTUBE_API_KEY hoặc ảnh mẫu đã tải lên thì hệ thống ghi rõ điều kiện còn thiếu và không tạo video.
