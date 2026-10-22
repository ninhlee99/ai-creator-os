# MiuNova

**MiuNova** là hệ thống AI định hướng tự tìm cơ hội affiliate, tạo video
affiliate và nội dung gốc, phát triển kênh trên TikTok, Facebook, Threads,
Instagram và YouTube. Đích sản phẩm là để người vận hành kết nối tài khoản xã
hội rồi theo dõi; TikTok cần một chạm xác nhận cho từng bài. Chỉ dùng dịch vụ
miễn phí: chuyển sang tài khoản miễn phí khác khi phù hợp; hết mọi nguồn thì
dừng job và báo.

> Đây là mục tiêu sản phẩm, chưa phải mô tả đầy đủ năng lực hiện tại. Luồng tự
> tìm hàng → tự tạo video → xuất bản → đo lường trên đủ năm nền tảng chưa hoàn
> chỉnh. Xem [trạng thái implementation](docs/IMPLEMENTATION_STATUS.md).

## Tài liệu chuẩn

- [PRODUCT_SPEC](docs/PRODUCT_SPEC.md): sản phẩm cần trở thành gì.
- [PRODUCT_DECISIONS](docs/PRODUCT_DECISIONS.md): quyết định đã chốt, giả định,
  và điểm còn mở.
- [SYSTEM_BLUEPRINT](docs/SYSTEM_BLUEPRINT.md): bản vẽ hệ thống hoàn chỉnh,
  vòng đời từng job, các giai đoạn và điểm dừng.
- Bộ hình MiuNova: [logo](docs/assets/miunova-logo.svg), [kiến trúc 2D](docs/assets/miunova-2d.svg),
  [phòng nhân sự 2.5D](docs/assets/miunova-25d-workroom.svg),
  [xưởng sáng tạo 3D](docs/assets/miunova-3d-office.png).
- [IMPLEMENTATION_STATUS](docs/IMPLEMENTATION_STATUS.md): khả năng hiện có và
  giới hạn đã thấy trong code.
- [Ghi chú kiến trúc code kế thừa](docs/ARCHITECTURE.md), [tài liệu tính năng](docs/features/README.md),
  [hướng dẫn phát triển](docs/DEVELOPER.md).
- [Chỉ mục tài liệu](docs/README.md).

## Hiện trạng tóm tắt

Code có AccessTrade product discovery, shot plan/QC affiliate, Viral Radar,
growth planning, agent harness và publisher adapters. Affiliate autopilot vẫn
cần clip chuyển động do người vận hành cung cấp; pipeline video viral đầy đủ,
analytics đa nền tảng và UX xác nhận TikTok chưa đạt đích. Những giới hạn khác
được ghi trong [IMPLEMENTATION_STATUS](docs/IMPLEMENTATION_STATUS.md).

## Chạy app

```bash
git clone https://github.com/ninhlee99/ai-creator-os
cd ai-creator-os
go build -o aicos ./cmd/aicos
./aicos
```

Mở `http://localhost:8080`. FFmpeg cần được cài riêng cho tác vụ dựng video.
Khi chạy trên VPS và truy cập bằng điện thoại, cần cấu hình HTTPS, hostname
công khai và OAuth callback tương ứng.

## Định danh kỹ thuật

Tên sản phẩm hiển thị: **MiuNova**. Tên repository/module kỹ thuật
`ai-creator-os`, binary `aicos`, các route, cấu hình và thư mục dữ liệu hiện có
được giữ nguyên để tương thích.
