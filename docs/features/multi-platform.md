# Kết nối mạng xã hội (`/publishers`)

## Mục đích

Hiển thị trạng thái và nút kết nối YouTube, TikTok, Facebook Page theo từng kênh. Nút OAuth mở khi client credentials tương ứng đã có trong Cài đặt. Facebook cho phép chọn một Page do tài khoản đang đăng nhập quản lý.

## Kết nối

- YouTube: `GET /publishers/youtube/authorize` → Google consent →
  `GET /publishers/youtube/callback`. Client ID/Secret lấy từ Settings;
  refresh token lưu trong `<data>/tokens/youtube_token_<username>.json`. Code
  dùng Google OAuth Desktop + loopback khi `YOUTUBE_REDIRECT_URI` trống; khi có
  URI HTTPS cấu hình, dùng Google Web OAuth client và callback VPS. URI phải
  kết thúc đúng `/publishers/youtube/callback` và đăng ký chính xác trong
  Google Cloud.
- TikTok: `GET /publishers/tiktok/authorize` → TikTok consent →
  `GET /publishers/tiktok/callback`. Client Key/Secret lấy từ Settings; token
  lưu trong `<data>/tokens/tiktok_token_<username>.json`. Mặc định callback
  loopback; đặt `TIKTOK_REDIRECT_URI` để dùng callback HTTPS trên VPS. Code
  chỉ thêm PKCE cho callback loopback/Desktop; Web callback dùng state và
  server-side client secret theo Login Kit.
- Facebook: `GET /publishers/facebook/authorize` → Meta consent →
  `GET /publishers/facebook/callback`. Người dùng chọn Page trong danh sách trả
  về; Page ID và Page access token lưu dạng per-channel settings
  `FB_PAGE_ID_<USERNAME>` và `META_PAGE_TOKEN_<USERNAME>`.

TikTok, Facebook và YouTube cần callback đã đăng ký trùng URL cấu hình khi chạy
từ VPS:

```text
TIKTOK_REDIRECT_URI=https://<host>/publishers/tiktok/callback
META_REDIRECT_URI=https://<host>/publishers/facebook/callback
YOUTUBE_REDIRECT_URI=https://<host>/publishers/youtube/callback
```

`META_REDIRECT_URI` chỉ nhận callback path `/publishers/facebook/callback`;
đăng ký đúng URL trong Meta App. Khi TikTok dùng static relay tại
`docs/tiktok-app-site/callback.html`, người vận hành phải chép toàn URL kết quả
vào form “TikTok Relay” ở `/publishers`; relay không tự chuyển về localhost.
Đây là fallback thủ công, không phải luồng VPS tiêu chuẩn. Hãy dán ngay; OAuth
state hết hạn sau 15 phút và authorization code có thể hết hạn sớm hơn.

Google Web OAuth của YouTube cần client loại Web application; local không đặt
`YOUTUBE_REDIRECT_URI` và tiếp tục dùng Desktop OAuth/loopback. OAuth state dùng
kho trong `internal/web/tiktok_oauth.go`, chỉ dùng một lần và hết hạn sau 15
phút. Mỗi callback vẫn cần xác minh trên deployment với provider credentials.

## Trạng thái hiển thị

- YouTube/TikTok: UI cho biết token file có hay bị đánh dấu hết hạn; nút OAuth dựa trên cặp client credentials.
- Facebook: UI cho biết Page ID + Page token có; nút OAuth dựa trên `META_APP_ID` + `META_APP_SECRET`.
- Instagram/Threads: UI cho biết ID + token đã cấu hình và có link mở trực tiếp
  nhóm cấu hình Meta. Threads adapter hiện chỉ hỗ trợ text + link.
- CTA video trong Dashboard/Studio chỉ mở trang tải file để đăng thủ công; trang
  `/publishers` không upload video qua API.
- `Platforms` là danh sách adapter đủ cấu hình cục bộ (`IsConfigured`), không phải kết quả gọi API.
- Trang không ping provider; có token/ID không chứng minh API permission hoặc publish live hoạt động. RTMP là trạng thái legacy.

Threads adapter hiện đăng text kèm link, chưa đăng video từ file local. API video
được Meta document nhưng cần URL public để Meta tải file; app chưa có Media
Delivery Gateway với URL HTTPS tạm theo artifact. Xem
[IMPLEMENTATION_STATUS](../IMPLEMENTATION_STATUS.md) và
[PRODUCT_SPEC §9.1](../PRODUCT_SPEC.md).

## Cổng TikTok trong mô hình MiuNova

Token OAuth tồn tại không đồng nghĩa app đủ điều kiện xuất bản. TikTok
eligibility cho app cá nhân tự host chưa xác minh. Direct Post bị khóa trong
adapter; `TIKTOK_DRAFT_ONLY=0` không xin scope `video.publish` và không thể gọi
đăng thẳng. Luồng Growth giữ MP4 cục bộ để người vận hành đăng trong ứng dụng
TikTok. Không dùng browser automation để né cổng API.

Nếu Direct Post được cấp, mỗi bài vẫn cần preview, lựa chọn bắt buộc và consent
riêng. Hai câu hỏi còn mở trong [PRODUCT_DECISIONS](../PRODUCT_DECISIONS.md):
O-04 về eligibility trước, O-07 về phạm vi “một chạm” sau.

## Fail-closed & an toàn

- State sai, dùng lại, hoặc quá hạn bị từ chối trước khi đổi token.
- Facebook Page token không đi qua URL hay HTML; khi có nhiều Page, selection state giữ token trong RAM và form chỉ gửi Page ID.
- OAuth credentials và Page token không render ra HTML. Page token lưu theo đúng kênh để không dùng nhầm Page.
- Thiếu credentials thì UI hiện đường dẫn cấu hình, không hiện nút login OAuth.
- Callback VPS hoạt động khi cấu hình provider URI, HTTPS proxy và client đúng
  loại; trạng thái kết nối end-to-end trên deployment chưa được xác minh.

## Key rotation

OAuth token từng nền tảng lưu riêng theo tài khoản; đây không phải pool API key.
