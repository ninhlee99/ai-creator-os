# Nghiên cứu cho hai flow TikTok

Rà theo source và tài liệu provider ngày 2026-10-24. Link provider có thể đổi; kiểm tra lại trước khi bật production.

## TikTok Content Posting API

TikTok yêu cầu quyền OAuth phù hợp và phân biệt upload Draft với Direct Post. Direct Post còn phụ thuộc app eligibility, creator controls và consent theo từng post. MiuNova hiện dùng draft-first; không suy ra app đã được duyệt Direct Post.

Nguồn chính thức:
- [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines)
- [Direct Post API](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post)

## TikTok Radar

Code lấy feed video Việt Nam và tìm kiếm keyword qua TikWM. Đây không phải TikTok API chính thức; freshness, độ phủ, giới hạn truy vấn và quy tắc thu thập cần được xác minh trước production. TikTok Research API hỗ trợ truy vấn keyword nhưng chỉ dùng được khi project được TikTok phê duyệt và cấp scope `research.data.basic`; không thay thế được bằng OAuth Login Kit thông thường. Metadata caption/sound không đủ để xác nhận video có nhảy. Dùng lựa chọn thủ công trong Radar khi detector chưa đáng tin.

## AccessTrade

Datafeed API mô tả giá, giảm giá, sản phẩm, campaign, affiliate link và thời điểm cập nhật; schema đã rà chưa chứng minh sales gần đây hoặc rating/review per-product. Không gọi offer là bán chạy/đánh giá tốt dựa vào các field không có.

Nguồn:
- [Datafeeds API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-thong-tin-datafeeds)
- [Campaigns API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-danh-sach-campaigns)
- [Top products API](https://developers.accesstrade.vn/api-publisher-vietnamese/top-cac-san-pham-ban-chay-nhat)

## Kết luận

Ưu tiên hoàn thiện đúng hai flow TikTok hiện tại: video affiliate và video viral. Không mở thêm luồng hoặc nguồn dữ liệu trước khi các bước hiện tại có provenance và acceptance rõ.
