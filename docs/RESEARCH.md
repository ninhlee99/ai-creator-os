# Nghiên cứu cho hai flow TikTok

Rà theo source và tài liệu provider ngày 2026-10-24. Link provider có thể đổi; kiểm tra lại trước khi bật production.

## TikTok Content Posting API

TikTok yêu cầu quyền OAuth phù hợp và phân biệt upload Draft với Direct Post. Direct Post còn phụ thuộc app eligibility, creator controls và consent theo từng post. MiuNova hiện dùng draft-first; không suy ra app đã được duyệt Direct Post.

Nguồn chính thức:
- [Content Sharing Guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines)
- [Direct Post API](https://developers.tiktok.com/docs/en/content-posting-api-reference-direct-post)

## TikTok Radar

Radar ưu tiên Apify TikTok Scraper khi có `APIFY_API_TOKEN`; nếu Apify lỗi hoặc trả video không hợp lệ, Radar tự chuyển sang TikTok Business Discovery khi access token + advertiser ID hợp lệ. Nếu chưa có Apify token, Discovery dùng làm nguồn chính. Apify tìm theo TikTok và tính phí theo kết quả; Business Discovery là API chính thức, trả video đã xếp hạng theo hashtag nhưng không có bộ đếm view/tym. Không có hai nguồn này thì Radar báo cần cấu hình, không gọi TikWM. TikTok Research API không phải lựa chọn thực tế cho app thương mại: TikTok giới hạn quyền cho nhà nghiên cứu đủ điều kiện, mục đích phi lợi nhuận và cần phê duyệt; xem [điều kiện Research API](https://developers.tiktok.com/products/research-api/). Metadata caption/sound không đủ để xác nhận video có nhảy. Dùng lựa chọn thủ công trong Radar khi detector chưa đáng tin.

## AccessTrade

Datafeed API mô tả giá, giảm giá, sản phẩm, campaign, affiliate link và thời điểm cập nhật; schema đã rà chưa chứng minh sales gần đây hoặc rating/review per-product. Không gọi offer là bán chạy/đánh giá tốt dựa vào các field không có.

Nguồn:
- [Datafeeds API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-thong-tin-datafeeds)
- [Campaigns API](https://developers.accesstrade.vn/api-publisher-vietnamese/lay-danh-sach-campaigns)
- [Top products API](https://developers.accesstrade.vn/api-publisher-vietnamese/top-cac-san-pham-ban-chay-nhat)

## Kết luận

Ưu tiên hoàn thiện đúng hai flow TikTok hiện tại: video affiliate và video viral. Không mở thêm luồng hoặc nguồn dữ liệu trước khi các bước hiện tại có provenance và acceptance rõ.
