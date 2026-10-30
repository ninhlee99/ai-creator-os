# Video affiliate TikTok

## Flow

1. Chỉ chọn offer đã duyệt; giữ link và dữ kiện nguồn.
2. Lập shot plan theo listing và ảnh nhân vật của kênh.
3. Chọn video nhảy đã xác nhận trong TikTok Radar, được thu thập lại trong 7 ngày gần nhất, làm nguồn âm thanh.
4. Tạo từng shot bằng clip chuyển động đã cung cấp hoặc provider được phép.
5. Trích audio từ đúng video Radar, ghép vào output và lưu provenance.
6. QC hình, sản phẩm, audio và file; sau đó gửi TikTok Draft.

## Node workflow

`preset_affiliate_showcase` dùng chuỗi node: **Nhân vật kênh → Sản phẩm listing → Bóc tách sản phẩm → Outfit/tham chiếu → Bối cảnh → Ánh sáng → Shot plan → Workflow Map/Muse AI → Ghép shot → Trộn voice/music → Ghép audio → Cân màu → Video QC → Upscale → Xuất video**.

- `Nhân vật kênh` chỉ nhận ảnh người dùng tải lên hoặc ảnh thuộc profile kênh đã chọn. Không tìm ảnh thay thế và không dùng ảnh mẫu mặc định. Thiếu ảnh nhân vật thì dừng workflow.
- `Sản phẩm listing` nhận sản phẩm đã duyệt, ảnh sản phẩm hoặc ảnh bóc tách từ link. Ảnh nhân vật không được dùng thay ảnh sản phẩm.
- `Video mẫu` là input tùy chọn. Khi có video, Workflow Map truyền video làm tham chiếu chuyển động cho từng shot; khi vắng video, shot plan mặc định tạo lookbook tự nhiên.
- Ảnh nhân vật tải trực tiếp được lưu vào profile của kênh đang chọn rồi dùng làm khóa nhận diện. Không có ảnh nào trong profile kênh thì dừng trước khi xếp job.

`preset_viral_dance` xử lý nhánh viral: **Video Radar/upload → Phân tích chuyển động → Chia đoạn → Nhân vật profile → Retarget chuyển động → Render theo đoạn → Ghép audio nguồn → Cân màu → Video QC → Upscale/xuất**. Nhánh này cũng dừng nếu kênh thiếu ảnh nhân vật.

## Hiện trạng

Workflow và nhánh input đã được thiết kế. Render thật cần phiên Muse hoạt động hoặc clip Google Flow đã export. Veo API chưa dùng vì chính sách hiện tại chỉ cho phép provider miễn phí. Nếu provider không tạo được footage, workflow báo lỗi và dừng; không biến ảnh tĩnh thành video giả. Browser Edge headless trên máy này hiện crash trước khi mở DevTools, nên video render thử chưa đạt và không được xem là thành phẩm.

Flow tạo video affiliate hỗ trợ 2 cơ chế:
1. **1-Link Import & Quick Video**: Dán link sản phẩm (Shopee, TikTok Shop, Lazada...), hệ thống tự động bóc tách ảnh sản phẩm, giá bán, platform PID và lưu vào Kệ hàng. Từ Kệ hàng, người dùng chọn workflow canvas (như `preset_affiliate_daily` hoặc custom workflow), model kênh, tải ảnh nhân vật và bấm tạo video 1-click.
2. **Affiliate Studio Workflow**: Chạy qua graph workflow với các node Character, Product, Audio Radar, Ghép shot 9:16, kiểm định QC và xuất bản.

Hệ thống hoạt động on-demand, không chạy background crawler gây ngốn RAM hay CPU của máy host (tối ưu mượt mà cho macOS Intel 8-16GB RAM). Nguồn audio có thể lấy từ TikTok Radar hoặc audio tải lên.

TikTok product attachment: Draft cần người dùng gắn sản phẩm hoặc link affiliate khi đăng trên TikTok Creator. Job hiển thị tên/ID listing để người dùng dễ dàng gắn giỏ hàng TikTok Shop hoặc Shopee.

Sau khi tự đăng video công khai, dán URL vào job hoàn tất để Growth ghép counters TikTok theo account/video ID. TikTok Display API cung cấp số liệu tương tác video thực tế (views, likes, comments, shares).
