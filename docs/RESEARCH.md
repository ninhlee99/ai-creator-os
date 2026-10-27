# Nghiên cứu cho hai flow TikTok

Rà theo source và tài liệu provider ngày 2026-10-26. Link provider có thể đổi; kiểm tra lại trước khi bật production.

## TikTok Content Posting API

TikTok yêu cầu quyền OAuth phù hợp và phân biệt upload Draft với Direct Post. Direct Post còn phụ thuộc app eligibility, creator controls và consent theo từng post. MiuNova hiện dùng draft-first; không suy ra app đã được duyệt Direct Post.

Nguồn chính thức:
- [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines)
- [Direct Post API](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post)

## TikTok Radar

Radar hiện dùng [TikTok Web Crawler](features/tiktok-web-crawler.md), không dùng TikTok Business Discovery. Business Discovery và Research API là các lựa chọn khác, không phải nguồn code hiện tại. Không có hồ sơ phê duyệt quyền crawl; nội dung tài liệu này không xác nhận crawler được TikTok cho phép. Trước production cần rà soát policy/quyền truy cập và chuyển sang nguồn được phép nếu chưa có approval. Crawler không phân loại video nhảy; operator phải xác nhận từng video trước khi dùng làm nguồn audio.

## AccessTrade

Datafeed API mô tả giá, giảm giá, sản phẩm, campaign, affiliate link và thời điểm cập nhật; schema đã rà chưa chứng minh sales gần đây hoặc rating/review per-product. Không gọi offer là bán chạy/đánh giá tốt dựa vào các field không có.

Nguồn:
- [Datafeeds API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-thong-tin-datafeeds)
- [Campaigns API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-danh-sach-campaigns)
- [Top products API](https://developers.accesstrade.vn/api-publisher-vietnamese/top-cac-san-pham-ban-chay-nhat)

## Kết luận

Ưu tiên hoàn thiện đúng hai flow TikTok hiện tại: video affiliate và video viral. Không mở thêm luồng hoặc nguồn dữ liệu trước khi các bước hiện tại có provenance và acceptance rõ.
