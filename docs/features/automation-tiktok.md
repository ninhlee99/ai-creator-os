# Tác vụ hỗ trợ flow TikTok

## Kiến trúc On-Demand & Hiệu năng

Để đảm bảo hệ thống vận hành ổn định trên các máy cá nhân (kể cả macOS Intel 8-16GB RAM), toàn bộ các tiến trình crawler/worker nền nặng nề lúc khởi động đều đã chuyển sang chế độ **On-Demand**:

- **Sản phẩm & Affiliate**: Người dùng chủ động thêm sản phẩm qua link hoặc tìm kiếm theo nhu cầu, không quét tự động ngầm gây nghẽn băng thông và RAM.
- **TikTok Trends / Radar**: Kích hoạt theo yêu cầu người dùng trên giao diện để thu thập âm thanh, hashtag và video thịnh hành mới nhất.
- **Studio Maintenance & Jobs**: Hàng đợi công việc được quản lý bởi Studio worker có giới hạn luồng đồng thời (tối đa 2 jobs render cùng lúc, có cơ chế giải phóng render slot tránh deadlock), kiểm soát tải bộ nhớ chặt chẽ.
- **Growth & Kênh**: Đồng bộ số liệu khi người dùng truy cập hoặc theo cấu hình tài khoản.

## Ranh giới

Trạng thái scheduler, heartbeat, job và API result được phân tách rõ ràng. Flow affiliate lấy dữ liệu thực tế từ link sản phẩm và mẫu nhân vật của kênh. Dữ liệu tương tác video được đồng bộ khi có quyền TikTok Display API.
