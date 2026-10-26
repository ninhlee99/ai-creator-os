# Nghiên cứu cho hai flow TikTok

Rà theo source và tài liệu provider ngày 2026-10-26. Link provider có thể đổi; kiểm tra lại trước khi bật production.

## TikTok Content Posting API

TikTok yêu cầu quyền OAuth phù hợp và phân biệt upload Draft với Direct Post. Direct Post còn phụ thuộc app eligibility, creator controls và consent theo từng post. MiuNova hiện dùng draft-first; không suy ra app đã được duyệt Direct Post.

Nguồn chính thức:
- [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines)
- [Direct Post API](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post)

## TikTok Radar

Radar chỉ dùng TikTok Business Discovery sau khi có Access Token và Advertiser ID. API chính thức tìm hashtag phổ biến và video theo hashtag, nhưng không trả counters; topic search chỉ khớp hashtag đang thịnh hành. Xem [Discovery API](https://business-api.tiktok.com/gateway/docs/index?doc_id=1825119005427714) và [API for Business overview](https://ads.tiktok.com/resources/help/article/marketing-api?lang=en). TikTok Research API không phải lựa chọn thực tế cho app thương mại: TikTok giới hạn quyền cho nhà nghiên cứu đủ điều kiện, mục đích phi lợi nhuận và cần phê duyệt; xem [điều kiện Research API](https://developers.tiktok.com/products/research-api/). Không dùng Playwright để thu thập tự động hoặc reverse-engineer endpoint TikTok: [TikTok Terms of Service (Vietnamese)](https://t.tiktok.com/legal/page/us/terms-of-service/vi) cấm thu thập tự động và reverse engineering nếu chưa có chấp thuận bằng văn bản. Metadata caption/sound không đủ để xác nhận video có nhảy. Dùng lựa chọn thủ công trong Radar khi detector chưa đáng tin.

## AccessTrade

Datafeed API mô tả giá, giảm giá, sản phẩm, campaign, affiliate link và thời điểm cập nhật; schema đã rà chưa chứng minh sales gần đây hoặc rating/review per-product. Không gọi offer là bán chạy/đánh giá tốt dựa vào các field không có.

Nguồn:
- [Datafeeds API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-thong-tin-datafeeds)
- [Campaigns API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-danh-sach-campaigns)
- [Top products API](https://developers.accesstrade.vn/api-publisher-vietnamese/top-cac-san-pham-ban-chay-nhat)

## Kết luận

Ưu tiên hoàn thiện đúng hai flow TikTok hiện tại: video affiliate và video viral. Không mở thêm luồng hoặc nguồn dữ liệu trước khi các bước hiện tại có provenance và acceptance rõ.
