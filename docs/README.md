# MiuNova — Tài liệu chuẩn

Tên hiển thị **MiuNova** đã được chốt; mascot là mèo. Repository/module kỹ thuật
vẫn mang tên `ai-creator-os` để giữ tương thích.

## Mục tiêu sản phẩm

MiuNova tự tìm cơ hội affiliate, tạo nội dung gốc, phát triển nhiều kênh xã
hội, xuất bản theo quyền từng nền tảng, đo kết quả và tự điều chỉnh. Đích là
người dùng kết nối social rồi chủ yếu theo dõi; TikTok cần xác nhận từng bài,
Web AI có thể cần đăng nhập provider một lần trên desktop; login Web AI từ điện
thoại đã bị tắt. **Code chưa đạt đích này:**
AccessTrade yêu cầu key và payout identity chưa có mô hình provisioning không
buộc thêm bước affiliate; nguồn đang nối cũng thiếu sales gần đây và review;
Threads video chưa có Media Delivery Gateway.
TikTok còn hai cửa chưa chốt: app cá nhân tự host có thể không đủ điều kiện API;
Direct Post cần lựa chọn và consent riêng theo bài. Xem
[PRODUCT_DECISIONS](PRODUCT_DECISIONS.md), O-01, O-02, O-04 và O-07.

## Thứ tự đọc và nguồn sự thật

| Câu hỏi | Tài liệu chuẩn | Dùng để |
|---|---|---|
| Hệ thống cần trở thành gì? | [PRODUCT_SPEC.md](PRODUCT_SPEC.md) | Mục tiêu, luồng tự hành, yêu cầu sản phẩm và tiêu chí chấp nhận |
| Hiện code làm được gì? | [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) | Ảnh chụp trạng thái code, giới hạn và khoảng cách tới mục tiêu |
| Quyết định nào đã chốt? | [PRODUCT_DECISIONS.md](PRODUCT_DECISIONS.md) | Quyết định người dùng, giả định, giới hạn ngoài hệ thống, việc còn mở |
| Hệ thống hoàn chỉnh và một job chạy ra sao? | [SYSTEM_BLUEPRINT.md](SYSTEM_BLUEPRINT.md) | Sơ đồ kiến trúc, vòng đời job, trạng thái và bằng chứng cần lưu |
| Logo và mô hình hệ thống? | [Logo](assets/miunova-logo.svg) · [2D](assets/miunova-2d.svg) · [2.5D](assets/miunova-25d-workroom.svg) · [3D có chú giải](assets/miunova-3d-office-labeled.svg) | Nhận diện, luồng vận hành và bảy vai trò nhân viên |
| Code hiện được tổ chức ra sao? | [ARCHITECTURE.md](ARCHITECTURE.md) | Ghi chú kỹ thuật từ các vòng trước; chỉ dùng làm manh mối, kiểm tra source và IMPLEMENTATION_STATUS |
| Một trang/tính năng chạy thế nào? | [features/README.md](features/README.md) | Mô tả hành vi hiện có trong code |
| Cách phát triển/chạy app? | [DEVELOPER.md](DEVELOPER.md) | Hướng dẫn cho nhà phát triển |
| Cách vận hành app hiện tại? | [USER_GUIDE.md](USER_GUIDE.md) | Hướng dẫn as-built, có nêu giới hạn so với mục tiêu |
| Quy tắc an toàn? | [POLICY_AND_SAFETY.md](POLICY_AND_SAFETY.md) | Chính sách nội dung, tài khoản, dữ liệu và vận hành |

## Quy tắc xử lý mâu thuẫn

1. PRODUCT_SPEC là đích sản phẩm.
2. PRODUCT_DECISIONS ghi quyết định đã được chủ sản phẩm xác nhận; mục “giả
   định” không được trình bày như quyết định đã chốt.
3. IMPLEMENTATION_STATUS và tài liệu tính năng mô tả code hiện tại. Chỉ trạng
   thái đã kiểm tra bằng nguồn phù hợp mới được gọi là đã triển khai.
4. POLICY_AND_SAFETY quy định hard gate mục tiêu; nếu code chưa thực thi được
   chốt đó, IMPLEMENTATION_STATUS phải nêu khoảng cách và integration giữ trạng
   thái tắt/chờ.
5. README chỉ làm mục lục ngắn. Nghiên cứu phải có ngày và nguồn; đề xuất cũ,
   review cũ và spec video cũ không ghi đè tài liệu chuẩn. Khi kiến trúc cũ
   khác PRODUCT_SPEC hoặc IMPLEMENTATION_STATUS, dùng hai tài liệu chuẩn đó.

## Nhóm tài liệu

- [Tài liệu tính năng](features/README.md): mô tả as-built.
- [Login Web AI desktop khi app chạy VPS](features/remote-webai-login.md): login
  điện thoại đã bị tắt; live provider/VPS chưa xác minh.
- [Nghiên cứu](RESEARCH.md): nguồn và kết luận có ngày kiểm tra; chính sách API
  cần kiểm tra lại trước khi bật tích hợp.
- [Kế hoạch phát triển kênh cũ](CHANNEL_GROWTH.md): thiết kế trước đây, giữ
  thông tin code hữu ích; PRODUCT_SPEC quyết định đích mới.
- [OPEN_SPEC video](OPEN_SPEC.md): đề xuất video cũ; chỉ dùng tiêu chí riêng
  sau khi đối chiếu free-only, nội dung gốc và PRODUCT_SPEC.
- PIVOT_REDESIGN.md, MODEL.md, AGENT_TEAM.md, GAP_ANALYSIS_AND_USER_NEEDS.md,
  PROJECT_REVIEW.md và REVIEW_ROUND2.md là tài liệu lịch sử. Dùng để tra cứu
  quyết định/code tại thời điểm viết, không coi là kế hoạch hiện hành.
