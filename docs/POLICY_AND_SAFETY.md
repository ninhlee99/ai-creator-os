# POLICY_AND_SAFETY — MiuNova

> Chính sách mục tiêu cho sản phẩm. Không khẳng định toàn bộ chốt đã có trong
> code; xem [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md) trước khi vận
> hành. Nguồn yêu cầu sản phẩm: [PRODUCT_SPEC](PRODUCT_SPEC.md). Quyết định và
> giả định còn mở: [PRODUCT_DECISIONS](PRODUCT_DECISIONS.md).

## 1. Quyền tài khoản và nền tảng

- Chỉ dùng account do chủ sở hữu kết nối hoặc instance được ủy quyền; chỉ yêu
  cầu OAuth scope cần cho tác vụ.
- Đăng/đọc số liệu qua API chính thức và quyền đã cấp. Không giả creator, né
  app review, vượt rate limit, scrape dữ liệu riêng tư hoặc bypass endpoint.
- Token và secret chỉ được xử lý ở backend; không render vào HTML, URL, event,
  log hoặc thông báo. Bảo vệ khi lưu, backup và thu hồi.
- OAuth từ điện thoại về app VPS phải dùng HTTPS, hostname công khai và callback
  tương ứng. Khi token hết hạn/bị thu hồi, giữ job ở trạng thái chờ kết nối lại.
- Mỗi nền tảng có trạng thái và kết quả publish riêng. Không ghi “đã đăng” trước
  khi API xác nhận đúng trạng thái.

## 2. Miễn phí và hạn mức

- Chỉ gọi provider/account có mức phí bằng 0 và được phép dùng cho tác vụ đó.
  Provider cần billing, gói trả phí hoặc mức giá không xác định được xem là
  không đủ điều kiện; không gọi.
- Khi account hết quota, Quota Guard có thể thử account miễn phí khác đã được
  chủ sở hữu kết nối và provider cho phép. Không tự tạo account, dùng danh tính
  người khác hoặc xoay account để né giới hạn.
- Hết mọi account miễn phí phù hợp, lỗi billing hoặc không xác định được quota:
  dừng job phụ thuộc nguồn đó, lưu provider/account, stage, phản hồi và lý do;
  báo người vận hành. Không tự chuyển sang trả phí.
- Nếu một lane bị chặn, chỉ lane đó dừng. Lane organic độc lập có thể tiếp tục
  khi vẫn đạt điều kiện chất lượng và quyền; không đổi affiliate offer âm thầm.
- Không ghi phí dự kiến là 0 nếu chưa có bằng chứng. Mục tiêu vận hành là chi
  phí provider bằng 0; quota, retry và account đã dùng phải truy vết được.

## 3. Bằng chứng sản phẩm, nội dung và tiền

- Mỗi offer giữ nguồn, thời điểm cập nhật, campaign, publisher account, điều
  kiện commission và provenance của giá, lượt bán, đánh giá, tồn kho.
- Không bịa hoặc suy ra “bán chạy”, “đánh giá tốt”, trải nghiệm, claim hiệu quả,
  giá, view, đơn hoặc hoa hồng. Thiếu field thì ghi unknown; nguồn cũ thì đánh
  dấu stale.
- Chỉ quảng bá offer/campaign mà publisher được phép dùng; tracking link phải
  gắn đúng publisher và có thể đối soát theo kênh/nội dung.
- Tách commission dự kiến, pending, approved và rejected. Chỉ commission đã
  được nhà cung cấp duyệt mới tính là doanh thu đã chốt.
- Không tự đăng ký, chấp nhận điều khoản affiliate, nhận payout thay chủ tài
  khoản hoặc tự tạo danh tính để vượt bước duyệt. Credential/payout setup là
  phụ thuộc instance, được ghi ở [PRODUCT_DECISIONS](PRODUCT_DECISIONS.md).

## 4. Quyền nội dung và minh bạch thương mại

- Trend là tín hiệu để tạo ý tưởng gốc; không reup, sao chép kịch bản/footage,
  nhạc hoặc hình ảnh khi không có quyền sử dụng.
- Nội dung affiliate phải thể hiện đúng sản phẩm và thông tin có nguồn, có
  disclosure theo yêu cầu nền tảng. Không bịa review, trải nghiệm hoặc khan hiếm.
- QC kiểm claim, disclosure, quyền media, sản phẩm hiển thị, file, âm thanh,
  metadata và format trước khi Publisher nhận artifact.
- Claim rủi ro cao hoặc không đủ chứng cứ phải sửa/hold/skip; không để agent
  “ước lượng cho đủ nội dung”. QC fail sau số retry hữu hạn thì dừng và báo.

## 5. Consent và xuất bản

- TikTok cần consent riêng cho từng bài trước khi gửi publish; consent bài này
  không cấp quyền cho bài khác. Với Direct Post, phải hiển thị creator info mới
  nhất, preview, metadata có thể sửa, privacy/interactions đúng theo creator
  options, commercial disclosure và AI disclosure phù hợp; người dùng tự chọn
  các mục TikTok yêu cầu, không đặt lựa chọn mặc định, rồi xác nhận gửi.
- Một nút xác nhận cuối sau lựa chọn bắt buộc là phương án khuyên dùng. Không
  gọi flow này là “một chạm tổng”; cách hiểu yêu cầu TikTok của người dùng còn
  chờ O-07 trong [PRODUCT_DECISIONS](PRODUCT_DECISIONS.md).
- Chỉ dùng Content Posting API sau khi app eligibility/scope/audit được xác
  nhận. Intended-use guidance không chấp nhận công cụ chỉ phục vụ account riêng
  của developer/team. Nếu app MiuNova vẫn là private self-hosted, không dùng
  browser automation/API khác để né cổng này; chuyển sang handoff thủ công.
- Không chèn MiuNova logo, watermark, link hoặc promotional text lên video
  TikTok; disclose paid/affiliate content bằng UI/API theo hướng dẫn hiện hành.
- Facebook, Instagram, Threads và YouTube chỉ tự publish khi account type,
  endpoint, app review, scope và nội dung cho phép. Nếu không, lưu draft hoặc
  chờ với lý do; không mạo nhận trạng thái đã đăng.
- Request publish phải idempotent hoặc có reconciliation trước khi retry để
  tránh bài trùng khi response bị mất.
- Không batch consent thay cho từng bài TikTok. Tham chiếu yêu cầu hiện hành:
  [TikTok Direct Post](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post)
  và [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines).

## 6. Job, retry và dừng an toàn

- Job lưu stage, input/output, nguồn dữ liệu, account/provider, artifact, quyết
  định, retry và lỗi; dashboard chỉ hiển thị trạng thái có event/job thật.
- Mọi retry có giới hạn; lỗi quota, quyền, claim hoặc trạng thái publish không
  rõ phải fail-closed. Không nhân retry để vượt giới hạn.
- Quota cạn, quyền thiếu, campaign chưa duyệt, QC hết retry hoặc dữ liệu không
  đủ cho claim phải thành trạng thái chờ/dừng có lý do và thông báo.
- Có công tắc dừng khẩn cấp để ngăn job mới và request publish chưa gửi. Không
  tuyên bố đã thu hồi request mà nền tảng đã nhận.

## 7. Tương tác người vận hành

Người dùng chủ yếu đăng nhập/kết nối tài khoản mạng xã hội, xem/chọn thông tin
bắt buộc và consent riêng cho từng bài TikTok, theo dõi dashboard và xử lý cảnh
báo cần quyền/con người. Cách hiểu “một chạm” chờ O-07.
Không yêu cầu chọn sản phẩm, viết script, tải clip, xếp lịch hoặc duyệt video
định kỳ. Khi hệ thống tự phục hồi được, tiếp tục và ghi lại kết quả; khi không,
dừng phần việc bị chặn và báo cách tiếp tục.

## 8. Rà soát trước khi bật tích hợp

Quy định API, permission, quota miễn phí và quy tắc thương mại có thể đổi. Trước
khi bật provider/platform, đối chiếu tài liệu chính thức, account/app review và
thực tế credential. Ghi nguồn/ngày kiểm tra vào [RESEARCH](RESEARCH.md); nếu
không chứng minh được điều kiện miễn phí hoặc quyền hợp lệ, giữ integration ở
trạng thái tắt/chờ.
