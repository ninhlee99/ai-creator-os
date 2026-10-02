# Đa nền tảng (`/publishers`)

## 1. Mục đích
Bảng trạng thái kết nối từng nền tảng của từng kênh: TikTok, YouTube,
Facebook Page, RTMP — biết cái nào đã nối, cái nào còn thiếu để đăng được.

## 2. Kích hoạt
- `GET /publishers` → `handlePublishers` (`internal/web/publishers.go:13`).
- TikTok OAuth: `GET /publishers/tiktok/authorize` → `POST /publishers/tiktok/connect`
  → `GET /publishers/tiktok/callback` (`internal/web/tiktok_oauth.go`).

## 3. Luồng vận hành chi tiết
1. Mỗi dòng = một tài khoản; `publishers.BuildPublishers(username, youtubeChannel,
   youtubeContentTypes)` dựng publisher cho từng nền tảng.
2. Badge trung thực từng ô (đọc thật, không đoán):
   - TikTok: `TiktokToken` = file token tồn tại (`publishers.TikTokTokenPath`),
     `TiktokClient` = đã nhập client key/secret (`HasClient()`).
   - YouTube: `YoutubeToken` = file `youtube_token_<user>.json` trong `<data>/tokens`;
     client = `YOUTUBE_CLIENT_ID`/`YOUTUBE_CLIENT_SECRET`.
   - Facebook: `FB_PAGE_ID_<USERNAME>` (hoặc `FB_PAGE_ID` chung) có giá trị — đăng
     qua `facebook-cli` bằng tài khoản đã liên kết, không cần Facebook App.
   - RTMP: `a.RtmpKey() != ""` (legacy từ thời live — giữ lại cho tương lai).
3. TikTok OAuth (`tiktok_oauth.go`): `authorize` phát `state` CSRF
   (`oauthStates.issue`, TTL **15 phút**) → redirect sang TikTok → `callback`
   đổi code lấy token → lưu file token. Hết TTL hoặc sai state → từ chối.
4. Trang cũng hiện `RedirectURI` (và có phải localhost không) để cấu hình đúng
   trong TikTok Developer Portal.

## 4. Fail-closed & an toàn
- Thiếu token/client → badge "thiếu", các luồng đăng bỏ qua nền tảng đó
  (xem `daemon-tu-dong.md`: `waiting_connect`).
- State CSRF hết hạn → callback bị từ chối, không đổi token bừa.
- Token nằm trong `<data>/tokens` (không commit), quyền file 0600.

## 5. API key rotation
- TikTok: OAuth token từng tài khoản (refresh khi hết hạn qua endpoint token).
- YouTube: OAuth client + refresh token từng tài khoản (`oauth2.googleapis.com/token`).
- Không dùng key xoay vòng chung ở đây.
