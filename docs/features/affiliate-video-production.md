# Video affiliate TikTok

## Flow

1. Chỉ chọn offer đã duyệt; giữ link và dữ kiện nguồn.
2. Lập shot plan theo listing và ảnh nhân vật của kênh.
3. Chọn video nhảy đã xác nhận trong TikTok Radar, được thu thập lại trong 7 ngày gần nhất, làm nguồn âm thanh.
4. Tạo từng shot bằng clip chuyển động đã cung cấp hoặc provider được phép.
5. Trích audio từ đúng video Radar, ghép vào output và lưu provenance.
6. QC hình, sản phẩm, audio và file; sau đó gửi TikTok Draft.

## Hiện trạng

Shot plan, tải clip, dựng video và visual QC đã sẵn sàng. Flow tạo video affiliate hỗ trợ 2 cơ chế:
1. **1-Link Import & Quick Video**: Dán link sản phẩm (Shopee, TikTok Shop, Lazada...), hệ thống tự động bóc tách ảnh sản phẩm, giá bán, platform PID và lưu vào Kệ hàng. Từ Kệ hàng, người dùng chọn workflow canvas (như `preset_affiliate_daily` hoặc custom workflow), model kênh, tải ảnh nhân vật và bấm tạo video 1-click.
2. **Affiliate Studio Workflow**: Chạy qua graph workflow với các node Character, Product, Audio Radar, Ghép shot 9:16, kiểm định QC và xuất bản.

Hệ thống hoạt động on-demand, không chạy background crawler gây ngốn RAM hay CPU của máy host (tối ưu mượt mà cho macOS Intel 8-16GB RAM). Nguồn audio có thể lấy từ TikTok Radar hoặc audio tải lên.

TikTok product attachment: Draft cần người dùng gắn sản phẩm hoặc link affiliate khi đăng trên TikTok Creator. Job hiển thị tên/ID listing để người dùng dễ dàng gắn giỏ hàng TikTok Shop hoặc Shopee.

Sau khi tự đăng video công khai, dán URL vào job hoàn tất để Growth ghép counters TikTok theo account/video ID. TikTok Display API cung cấp số liệu tương tác video thực tế (views, likes, comments, shares).
