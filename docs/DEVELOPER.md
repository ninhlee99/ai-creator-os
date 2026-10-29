# Hướng dẫn phát triển

## Thứ tự đọc

1. PRODUCT_SPEC.md: acceptance của hai flow TikTok.
2. PRODUCT_DECISIONS.md: quyết định sản phẩm đang có hiệu lực.
3. IMPLEMENTATION_STATUS.md: giới hạn code đã biết.
4. POLICY_AND_SAFETY.md: quy tắc dữ liệu, quyền truy cập, audio và publish.
5. features/tiktok-web-crawler.md: nguồn Radar và giới hạn crawler.

## Bản đồ code

| Gói | Phạm vi |
|---|---|
| cmd/aicos | Wiring và khởi chạy |
| internal/web | HTTP, UI và static assets |
| internal/network, internal/tiktok | Kênh và TikTok OAuth |
| internal/studio | Radar, tạo video, audio, QC |
| internal/products | Sản phẩm kệ hàng, bóc tách link affiliate, lưu trữ ảnh & giá |
| internal/publishers | TikTok Draft |
| internal/growth | Metrics và tăng trưởng TikTok |
| internal/webai | Phiên provider dùng cho tạo video |
| internal/ledger, internal/backup | Lưu dữ liệu và backup |

## Ranh giới sửa đổi

- Sửa đúng layer và giữ nguồn dữ liệu gắn provenance.
- Không thêm UI, route hay tác vụ cho flow ngoài phạm vi hiện hành. Không tự ý khởi chạy tiến trình daemon nền lúc khởi động app.
- Không biến thiếu dữ liệu thành 0 hoặc thành claim.
- Không retry request sau khi provider có thể đã nhận job nếu chưa đối chiếu trạng thái.
- Không commit credentials, cookies, session hoặc media riêng.
- Dùng gofmt cho Go. Kiểm tra theo yêu cầu task; không chạy test nếu chưa được yêu cầu.

## UI assets

UI dùng Go templates, HTMX và Tailwind CSS. HTMX được vendored local; Tailwind scan `internal/web/templates` rồi compile vào `internal/web/static/tailwind.css`.

```sh
npm ci
npm run build:ui
go build ./...
```

Không đưa CDN hoặc runtime Tailwind compiler vào app.
