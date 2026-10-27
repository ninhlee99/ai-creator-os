# MiuNova

MiuNova hiện tập trung vào hai flow TikTok:

1. Video affiliate TikTok.
2. Video viral TikTok.

Truyện audio là roadmap, không nằm trong flow hiện tại. Cả hai flow TikTok chỉ lấy audio từ video Radar đã được người vận hành xác nhận có vũ đạo. Radar hiện dùng Web Crawler cục bộ; xem [TikTok Web Crawler](docs/features/tiktok-web-crawler.md) để biết nguồn, giới hạn và trạng thái hiện tại.

## Luồng đang ưu tiên

Kết nối kênh TikTok → Radar thu thập video viral/nhảy → chọn video nguồn → chọn sản phẩm hoặc tạo video viral → kiểm tra video và âm thanh → gửi TikTok Draft để người dùng duyệt.

## Hiện trạng

Hai flow chưa đạt hoàn chỉnh end-to-end. Radar chưa phân loại video nhảy đủ tin cậy; nguồn sản phẩm AccessTrade thiếu dữ liệu bán gần đây và đánh giá; TikTok metrics chưa có nguồn đồng bộ đầy đủ. Xem [Implementation status](docs/IMPLEMENTATION_STATUS.md).

## UI stack

Go HTML templates + HTMX partial updates + Tailwind CSS compiled to local static assets. Không tải HTMX/Tailwind từ CDN.

## Chạy ứng dụng

~~~bash
go build -o aicos ./cmd/aicos
./aicos
~~~

Mở http://localhost:8080. Dựng video cần FFmpeg trên máy chạy app.

Sau khi sửa UI, chạy `npm ci` rồi `npm run build:ui` để cập nhật HTMX/Tailwind assets trước khi build Go.

## Tài liệu

- [Product spec](docs/PRODUCT_SPEC.md)
- [Quyết định sản phẩm](docs/PRODUCT_DECISIONS.md)
- [Trạng thái triển khai](docs/IMPLEMENTATION_STATUS.md)
- [TikTok Web Crawler](docs/features/tiktok-web-crawler.md)
- [Luồng âm thanh Radar](docs/OPEN_SPEC.md)
- [Kiến trúc hệ thống](docs/SYSTEM_BLUEPRINT.md)
- [Hướng dẫn vận hành](docs/USER_GUIDE.md)
- [Tài liệu tính năng đang giữ](docs/features/README.md)

Tên repository/module kỹ thuật vẫn là ai-creator-os; tên sản phẩm là MiuNova.
