# MiuNova (AI Creator OS)

Hệ thống sản xuất nội dung video tự động hóa linh hoạt theo quy trình (Workflow):

1. **Tạo video bằng 1 câu lệnh (`/`)**: Giao diện khởi động trực tiếp, tự động nhận diện ý tưởng, bóc tách link sản phẩm/video để kích hoạt quy trình render.
2. **Sản phẩm Affiliate (`/products`)**: Thêm sản phẩm nhanh chỉ từ 1 link (Shopee, TikTok Shop, Lazada, Tiki, Sendo...) → Hệ thống tự động bóc tách nhiều ảnh, tên, giá → Khởi chạy workflow tạo video 1-click.
3. **Workflow Canvas (`/workflow`)**: Kéo-thả trực quan, hỗ trợ Web AI Tools, Custom Prompt, TTS, và Upscale hình ảnh/video tối đa 2K.

## Cấu trúc Menu Điều hướng

- **Studio & Video**: Tạo video (`/`), Workflow Canvas (`/workflow`), Lịch sử tác vụ (`/studio/jobs`).
- **Affiliate**: Feed xu hướng (`/studio/trends`), Sản phẩm & kệ hàng (`/products`), Kênh TikTok & nhân vật (`/accounts`).
- **Vận hành**: Phân tích tăng trưởng (`/growth`), Cài đặt hệ thống (`/settings`).
- **Hạ tầng**: Giám sát RAM, CPU & Ổ cứng thời gian thực tại chân sidebar.

## Hiện trạng

Radar phân tích vũ đạo và chủ đề từ frame/caption qua Web AI; sản phẩm affiliate bóc tách trực tiếp từ link sàn TMĐT; TikTok Display API đồng bộ chỉ số tương tác video thực tế. Xem [Implementation status](docs/IMPLEMENTATION_STATUS.md).

## UI stack

Go HTML templates + HTMX partial updates + Tailwind CSS compiled to local static assets. Không tải HTMX/Tailwind từ CDN.

## Chạy ứng dụng

```bash
go build -o aicos ./cmd/aicos
./aicos
```

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
