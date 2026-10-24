# TikTok performance feedback

Đây là vòng cải tiến dùng chung cho hai flow TikTok, không phải flow sản phẩm thứ ba.

## Affiliate feedback

Theo dõi funnel theo campaign/video khi provider hỗ trợ: views → product clicks → orders → approved commission. So sánh giá, commission thực nhận, audience/category fit và retention. Không gán đơn cho video nếu attribution source/window không hỗ trợ ghép chính xác.

Gợi ý phải nêu evidence và giới hạn:

- View thấp, retention thấp: thử hook hoặc pacing.
- View cao, click thấp: kiểm tra CTA, product fit và vị trí product link.
- Click cao, order thấp: kiểm tra giá, offer, shipping/stock và audience fit.
- Order có nhưng commission thấp: rà điều kiện campaign và commission thực nhận.

Không có click/order metric thì không kết luận lý do không mua.

## Viral feedback

So sánh view velocity, retention/completion, shares/saves, profile visits và follows với baseline cùng account/topic/format/khoảng thời gian. Hiển thị cỡ mẫu. Thiếu metric thì hiện unknown; không suy đoán lý do ít view hoặc tăng follower.

## Tự cải tiến có giới hạn

- Gom snapshot theo cửa sổ 1h, 24h, 72h, 7d khi nguồn cho phép.
- Recommendation có evidence IDs, sample size, confidence, setting diff và version.
- Tự chỉnh chỉ các ngưỡng/radar weights/format mix đã cấu hình bounds; thay đổi phải đảo ngược được.
- Thay offer, nội dung/claim, quyền TikTok và public publish cần người duyệt.
- Chạy lại sync phải idempotent. Mỗi adjustment lưu before/after và hỗ trợ rollback.

## Hiện trạng

TikTok Display API hiện được nối để đọc profile follower/video/like totals (`user.info.stats`) và counters của public videos (`video.list`). Sau khi đăng thủ công, người dùng dán URL video vào job Affiliate/TikTok Viral đã hoàn tất. Studio lưu liên kết job + TikTok account + public video ID; Growth ghép counters mới nhất theo account/video và hiển thị cạnh job. Account phải kết nối lại và cấp hai scope; TikTok app cũng phải được duyệt Display API/scopes. Snapshot ghi theo account/video và thời điểm đồng bộ. Chưa có snapshot thì UI hiện dấu gạch, không coi là 0.

TikTok Display API không trả retention/watch-time, demographics, profile visits, follows-per-video hoặc orders. Các field đó vẫn unknown; `views_30d` không được suy ra từ lifetime counters. Riêng Affiliate, Growth tổng hợp đơn AccessTrade theo `sub2` đúng job và tách pending/approved/rejected; chỉ commission của đơn approved được cộng. Số này phụ thuộc báo cáo AccessTrade đã đồng bộ và dùng đúng link tracking, không chứng minh TikTok Shop product card đã gắn hay tự tạo attribution từ product card.
