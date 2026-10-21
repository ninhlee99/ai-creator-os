# Kết nối mạng xã hội (`/publishers`)

## Mục đích

Hiển thị trạng thái và nút kết nối YouTube, TikTok, Facebook Page theo từng kênh. Nút OAuth mở khi client credentials tương ứng đã có trong Cài đặt. Facebook cho phép chọn một Page do tài khoản đang đăng nhập quản lý.

## Kết nối

- YouTube: `GET /publishers/youtube/authorize` → Google consent → `GET /publishers/youtube/callback`. Client ID/Secret lấy từ Settings; refresh token lưu trong `<data>/tokens/youtube_token_<username>.json`.
- TikTok: `GET /publishers/tiktok/authorize` → TikTok consent → `GET /publishers/tiktok/callback`. Client Key/Secret lấy từ Settings; token lưu trong `<data>/tokens/tiktok_token_<username>.json`.
- Facebook: `GET /publishers/facebook/authorize` → Meta consent → `GET /publishers/facebook/callback`. Người dùng chọn Page trong danh sách trả về; Page ID và Page access token lưu dạng per-channel settings `FB_PAGE_ID_<USERNAME>` và `META_PAGE_TOKEN_<USERNAME>`.

OAuth state dùng kho state trong `internal/web/tiktok_oauth.go`, một lần và hết hạn sau 15 phút. Callback Facebook yêu cầu `META_APP_ID`, `META_APP_SECRET`; `META_REDIRECT_URI` tùy chọn khi callback khác loopback mặc định. Cần cấu hình callback trùng Meta App.

## Trạng thái hiển thị

- YouTube: token file tồn tại / hết hạn; nút kết nối dựa trên `YOUTUBE_CLIENT_ID` + `YOUTUBE_CLIENT_SECRET`.
- TikTok: token file tồn tại; nút kết nối dựa trên `TIKTOK_CLIENT_KEY` + `TIKTOK_CLIENT_SECRET`.
- Facebook: sẵn sàng khi cả Page ID và Page token tồn tại; nút OAuth dựa trên `META_APP_ID` + `META_APP_SECRET`.
- Instagram/Threads status dựa trên token và account ID đã cấu hình. RTMP là trạng thái legacy.

## Fail-closed & an toàn

- State sai, dùng lại, hoặc quá hạn bị từ chối trước khi đổi token.
- Facebook Page token không đi qua URL hay HTML; khi có nhiều Page, selection state giữ token trong RAM và form chỉ gửi Page ID.
- OAuth credentials và Page token không render ra HTML. Page token lưu theo đúng kênh để không dùng nhầm Page.
- Thiếu credentials thì UI hiện đường dẫn cấu hình, không hiện nút login OAuth.

## Key rotation

OAuth token từng nền tảng lưu riêng theo tài khoản; đây không phải pool API key.
