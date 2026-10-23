# MiuNova — Hướng dẫn người vận hành

> Hướng dẫn này mô tả những phần có trong app hiện tại. Đích tự hành đầy đủ
> nằm trong [PRODUCT_SPEC](PRODUCT_SPEC.md); khoảng cách code xem tại
> [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md). Tên kỹ thuật hiện tại là
> `ai-creator-os` / `aicos`.

## 1. Khởi chạy

Từ thư mục repository:

```bash
go build -o aicos ./cmd/aicos
./aicos
```

Mở `http://127.0.0.1:8080`. Dựng video cần FFmpeg được cài trên máy chạy app.
Các lựa chọn chạy/build đầy đủ nằm trong [README](../README.md) và
[DEVELOPER](DEVELOPER.md).

### Chạy trên VPS, mở bằng điện thoại

- Đặt app sau HTTPS với hostname công khai; giữ callback đúng đường dẫn và
  đăng ký URL trùng khớp trong trang developer của từng nền tảng.
- TikTok VPS: đặt `TIKTOK_REDIRECT_URI=https://<host>/publishers/tiktok/callback`.
  Callback quay thẳng về VPS; luồng Web không gửi PKCE. `TIKTOK_CLIENT_KEY` và
  `TIKTOK_CLIENT_SECRET` phải thuộc app TikTok cấu hình Web Login Kit với URI
  HTTPS đăng ký chính xác. `docs/tiktok-app-site/callback.html`
  chỉ là relay thủ công: chép toàn bộ URL kết quả sang form TikTok Relay tại
  `/publishers` ngay sau khi quay về. State hết hạn sau 15 phút; authorization
  code có thể hết hạn sớm hơn.
- Facebook VPS: đặt
  `META_REDIRECT_URI=https://<host>/publishers/facebook/callback` và đăng ký
  đúng URL này trong Meta App.
- YouTube VPS: dùng Google OAuth client loại Web application, đặt
  `YOUTUBE_REDIRECT_URI=https://<host>/publishers/youtube/callback`, rồi đăng ký
  chính xác URI đó trong Google Cloud. Local giữ `YOUTUBE_REDIRECT_URI` trống và
  dùng Desktop client/loopback. Google OAuth consent screen cũng cần cho phép
  tài khoản đăng nhập.
- Bật bảo vệ đăng nhập dashboard trước khi mở app ra internet. Không public
  cổng CDP, VNC hoặc browser-debug; CDP chỉ dùng nội bộ cho hidden generation
  fallback.
- TikTok, Facebook và YouTube đều có callback cấu hình cho VPS; cần xác minh
  HTTPS, proxy, consent screen và credentials thật của deployment.

## 2. Kết nối tài khoản

1. Tạo/chọn kênh tại `/accounts`.
2. Kết nối TikTok, Facebook hoặc YouTube tại `/publishers` sau khi điền app
   credentials và callback hợp lệ ở Settings.
3. Instagram/Threads hiện phụ thuộc token và ID cấu hình; xem trạng thái từng
   adapter tại [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md).
4. Web AI là nguồn tạo nội dung tùy chọn. Trong **Cài đặt → Hệ thống → Web AI**,
   dùng **Khôi phục JSON backup** để thêm account. MiuNova không đọc cookie từ
   browser đang chạy. Cookie mới được lưu theo account khi provider trả cookie
   mới. Trạng thái phiên không chứng minh account dùng gói Free; Flow yêu cầu
   xác nhận credits trước khi tạo video.
5. Affiliate hiện cần AccessTrade key, campaign được phép và publisher payout
   profile. App yêu cầu key trong Settings; AccessTrade còn yêu cầu publisher
   hoàn tất định danh, MST, hợp đồng điện tử và ngân hàng để nhận hoa hồng. Với
   VPS self-hosted, việc này thường buộc chính người dùng làm thêm ngoài social
   login. Vì vậy mục tiêu “chỉ kết nối social” chưa đạt cho tới khi O-01 chốt
   mô hình provision/payout. Xem [PRODUCT_DECISIONS](PRODUCT_DECISIONS.md).

## 3. Các màn hình hiện có

| Màn hình | Route | Năng lực hiện có và giới hạn |
|---|---|---|
| Tổng quan | `/` | Trạng thái app, quyết định và hoạt động đã ghi. Không phải bằng chứng mọi agent mục tiêu đang chạy. |
| Kênh | `/accounts` | Hồ sơ kênh, trạng thái kết nối, brand DNA và một số điều khiển autopilot. |
| Affiliate | `/products`, `/settings/accesstrade` | Tìm sản phẩm/campaign và tạo tracking link qua AccessTrade khi credential/campaign cho phép. Feed hiện chưa đủ sales/rating để khẳng định sản phẩm bán chạy hoặc đánh giá tốt. |
| Studio | `/studio`, `/studio/jobs` | Lập shot plan, tạo một số loại job và xem artifact. Affiliate autopilot hiện vẫn cần clip chuyển động từ operator; chưa đạt tạo video affiliate end-to-end tự động. |
| Viral | `/viral` | Radar tìm tín hiệu; pipeline video viral gốc đầy đủ chưa đạt đích và hiện bị chặn khi cần provider trả phí. |
| Phát triển kênh | `/growth` | Growth planner và một phần YouTube metrics; chưa tối ưu đa nền tảng theo dữ liệu end-to-end. |
| Xuất bản | `/publishers` | TikTok/Facebook/Instagram/Threads/YouTube có mức hỗ trợ khác nhau. Threads hiện chưa xuất bản video local; TikTok mặc định draft-only. |
| Web AI | `/web-ai` | Lưu/kiểm tra account; Studio dùng phiên Muse/Flow đang bật. Khôi phục JSON backup cũng được hỗ trợ. HTTP 403 không tự đánh dấu phiên hết hạn. |
| Cài đặt | `/settings` | Provider, AccessTrade, dry-run, master switch, thông báo và vận hành. Chỉ dùng cấu hình miễn phí theo yêu cầu sản phẩm. |
| Sức khỏe | `/healthz` | Endpoint trạng thái tiến trình để kiểm tra app còn phản hồi. |

Chi tiết từng năng lực và cách kích hoạt nằm trong
[features/README](features/README.md). Mục tiêu hoàn chỉnh nằm trong
[SYSTEM_BLUEPRINT](SYSTEM_BLUEPRINT.md).

## 4. Tài nguyên miễn phí và trạng thái job

- Chỉ dùng provider/account có mức phí bằng 0 và được phép cho tác vụ đó.
- Web AI account không cần xác nhận Free trong app và provider không có API plan
  chung để MiuNova kiểm chứng. Session active chỉ xác nhận kết nối, không chứng
  minh plan hoặc giá. Flow vẫn yêu cầu consent credits cho từng lần tạo; bật
  `Google Flow credits` cho AT Hunter sẽ cho phép tự động dùng credits.
- Khi Flow hoặc Muse báo HTTP 429 ở bước trước submit, MiuNova đưa account vào
  cooldown và thử account khác đang hoạt động. Không tạo/xoay account để
  né giới hạn, không retry sau khi provider đã nhận job/prompt, và không
  fallback sang trả phí.
- Với Gemini API key, vào **Cài đặt → Nhà cung cấp** và xác nhận toàn bộ key
  hiện tại chỉ dùng quota miễn phí. Thêm, xóa hoặc đổi key sẽ tự khóa API tới
  khi xác nhận lại. Gemini Veo video bị chặn vì cần project bật billing.
- Nếu hết tất cả nguồn phù hợp, job phải dừng, ghi stage/lý do và báo. Hiện
  fallback miễn phí end-to-end cho toàn pipeline **chưa được chứng minh**.
- Cài đặt AT Hunter có tùy chọn `Google Flow credits`. Để giữ đúng yêu cầu chỉ
  miễn phí, hãy để tùy chọn này tắt: xác nhận credits không chứng minh lượt tạo
  video có giá bằng 0. App hiện không tự xác minh phí trước khi gọi Flow.
- Thiếu sales, rating, metrics hoặc trạng thái API phải hiện là thiếu/chờ xác
  minh; không diễn giải thành dữ liệu tốt.

## 5. TikTok và các thao tác người dùng

Yêu cầu sản phẩm là xác nhận từng bài TikTok. Direct Post còn buộc hiển thị
preview, metadata, privacy/interactions và commercial disclosure cho từng bài
trước consent; vì thế “một chạm tổng” chưa được chốt. Phương án khuyên dùng là
một nút xác nhận cuối sau lựa chọn bắt buộc. Publisher hiện mặc định draft-only.
Ngoài ra, điều khoản repo mô tả MiuNova là app cá nhân tự host; TikTok hiện
hướng dẫn API client không được chỉ phục vụ account riêng của developer/team.
Eligibility/audit vì vậy chưa xác minh. Không coi draft là bài đã đăng hoặc
Direct Post public là năng lực chắc chắn. Xem
[PRODUCT_DECISIONS O-04/O-07](PRODUCT_DECISIONS.md).

Trong vận hành mục tiêu, người dùng chủ yếu kết nối account, theo dõi dashboard,
xác nhận từng bài TikTok và xử lý cảnh báo không tự khắc phục được. Thực tế app
chưa đạt toàn bộ tự động hóa này; không chọn sản phẩm, upload clip hay duyệt nội
dung định kỳ là mục tiêu, không phải giới hạn đã giải quyết.

## 6. Dừng và hỗ trợ

- Dùng Kill Switch trong dashboard để ngăn job mới và hoạt động tự động.
- Khi OAuth hết hạn, quota cạn hoặc API thiếu quyền, giữ job ở trạng thái chờ/dừng
  có lý do; không lặp lại thao tác vô hạn. YouTube video upload có bucket riêng,
  mặc định 100 lượt/ngày theo giờ Pacific; giới hạn thực xem trong Google Cloud
  Console.
- Kiểm tra [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md) trước khi dựa vào
  một luồng xuất bản, analytics hoặc login từ VPS.
- Không chia sẻ password, OAuth token, API key hoặc cookie qua log/ticket.
