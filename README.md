# MiuNova (AI Creator OS)

Hệ thống sản xuất nội dung video AI tự động hóa linh hoạt theo quy trình (Workflow):

1. **Sản phẩm Affiliate**: Thêm sản phẩm nhanh chỉ từ 1 link (Shopee, TikTok Shop, Lazada...) → Hệ thống tự động phân tích lấy ảnh, tên, giá → Khởi chạy workflow tạo video 1-click.
2. **Universal Video Studio (`/workflow/create`)**: Tạo đa dạng thể loại video (TikTok Viral Dance, TikTok Affiliate Review, Custom Workflow) điều khiển trực quan theo từng workflow AI.
3. **Workflow Canvas**: Kéo-thả trực quan, hỗ trợ Web AI Tools, Prompt Custom, TTS, và Upscale hình ảnh/video tối đa 2K.

## Cấu trúc Menu Điều hướng

- **Tổng quan**: Dashboard trực quan hóa số liệu (`/`).
- **TikTok**: Xu hướng video & âm thanh hot (`/studio/trends`).
- **Affiliate**: Quản lý sản phẩm & kệ hàng (`/products`).
- **Workflow**: Canvas kéo-thả (`/workflow`), Tạo video đa thể loại (`/workflow/create`), Lịch sử tác vụ (`/studio/jobs`).
- **Hệ thống**: Phân tích tăng trưởng (`/growth`), Kênh TikTok & nhân vật (`/accounts`), Cài đặt (`/settings`).

## Hiện trạng

Hai flow chưa đạt hoàn chỉnh end-to-end. Radar AI phân loại vũ đạo và chủ đề từ frame/caption nhưng cần xác minh trên nguồn thật; nguồn sản phẩm AccessTrade thiếu dữ liệu bán gần đây và đánh giá; TikTok metrics chưa có nguồn đồng bộ đầy đủ. Xem [Implementation status](docs/IMPLEMENTATION_STATUS.md).

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
