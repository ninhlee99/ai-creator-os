# UI/UX blueprint

## Phạm vi

Chỉ expose hai flow: TikTok affiliate và TikTok viral. Truyện audio để phase sau. Account, settings, jobs và insights là công cụ dùng chung cho hai flow.

## Công nghệ

- Go `html/template` render trang và partial phía server.
- HTMX cho tìm/lọc, refresh Radar, đánh giá sản phẩm, duyệt/loại, poll job, sync metrics và áp dụng/hoàn tác recommendation.
- Tailwind CSS version pin; compile thành local static CSS. Production không dùng CDN hoặc runtime compiler.
- Dùng full URL/form fallback cho page load và mutation quan trọng; HTMX enhancement không giấu state/error.
- Response partial cần có empty/loading/error/success states; action provider không được báo thành công trước provider evidence.

## Điều hướng

1. **Affiliate**: category radar, danh sách sản phẩm, candidate detail, approved/rejected, campaign jobs, publish review, order feedback.
2. **TikTok viral**: topic radar, phân tích video, tạo phiên bản mới, publish review, jobs và performance feedback.
3. **Kênh**: TikTok account, persona/brand, ảnh nhân vật, trạng thái OAuth.
4. **Insights**: phân tích riêng affiliate/viral với evidence, baseline và recommendations.
5. **Cài đặt**: TikTok OAuth, AccessTrade, media providers, automation bounds, safety, JSON config, backup.

## Candidate detail

- Header: sản phẩm, category, trạng thái và thời điểm cập nhật.
- Evidence: giá/currency, commission/link/campaign, sales/rating/review nếu có, trend signals, channel fit.
- Mỗi row có nguồn, timestamp và trạng thái `Có dữ liệu`, `Cũ`, `Chưa có dữ liệu`.
- Hiện verdict, confidence, reason, rule version và missing evidence.
- Action buttons: `Duyệt`, `Loại`, `Đánh giá lại`; loại sản phẩm chuyển sang danh sách `Bị loại` cùng evidence.

## Video and job states

- TikTok Radar dùng Web Crawler; hiển thị browser readiness, source, fetch error, metrics/audio metadata missing và topic chính xác. Xem [crawler contract](features/tiktok-web-crawler.md).
- Hiển thị rõ trạng thái chưa xác nhận/xác nhận thủ công. Chỉ video đã được người dùng xác nhận mới mở hành động dùng làm source audio; crawler không được gắn nhãn xác minh.
- Job detail cho source ID/URL, audio metadata, product/tracking link, QC, artifact và TikTok state.
- Draft, product attachment, public post, pending reconciliation và failed là trạng thái tách biệt.

## Publish review

- Preview video và hiện nickname/account TikTok đích; refresh creator capabilities trước khi render nút đăng.
- Caption và hashtags sửa được. Không đóng watermark, logo quảng bá, URL hay promotional text vào video.
- Người dùng tự chọn privacy từ các lựa chọn provider trả về; không preselect. Hiện interaction controls theo creator info và để off mặc định.
- Hiện commercial-content disclosure. Với video affiliate, cho chọn disclosure phù hợp và nêu nhãn TikTok sẽ gắn.
- Với video do AI tạo/chỉnh sửa đáng kể, hiện lựa chọn disclosure/AI-generated label; gửi đúng `is_ai_generated` cho TikTok Shop Shoppable Video API. Nếu adapter không hỗ trợ nhãn cần thiết, public submit bị khóa và UI đưa người dùng sang Draft/manual handoff.
- Với TikTok Shop, hiển thị TikTok Shop product ID, mapping tới AccessTrade offer đã duyệt, anchor title và trạng thái precheck/anchor. Mapping lệch thì chặn publish. AccessTrade tracking link là trường attribution riêng.
- Hiện consent declaration sát nút `Đăng lên TikTok`; chưa consent thì không upload. Với Direct Post app chưa audit, chặn public mode và hướng dẫn Draft/manual handoff.
- Sau submit, hiển thị provider state và poll/webhook result. Timeout dùng `Đang đối chiếu`, khóa retry tới khi reconcile; chỉ public status mới được gọi là đã đăng.

## Insights

- Affiliate funnel chỉ vẽ các bước có source thật; metric thiếu ghi `Chưa có dữ liệu`.
- Viral metrics so sánh cùng cửa sổ và cohort; hiện sample size và source.
- Recommendation hiển thị evidence, confidence, expected setting diff, current/new value, version, `Áp dụng` và `Hoàn tác`.
- Không tự đổi offer, claim, account permission hoặc public publish.

## Account card and API keys

- Account card: username, connection state, character image, niche/theme và thao tác account.
- Không dùng followers/view progress nếu không có metrics được xác minh.
- API key panel chỉ có `Xuất JSON` và `Nhập JSON`.

## HTMX behavior oracle

| Hành động | Request | Expected response |
|---|---|---|
| Lọc product radar | GET partial với category/filter | Candidate rows khớp filter; count khớp rows |
| Duyệt/loại candidate | POST action | Candidate state đổi, history reason/version hiện tại |
| Chọn nguồn dance | POST Radar video ID | Job source ID/URL trùng lựa chọn; không chọn ngầm |
| Refresh job | GET job partial | Hiện trạng thái DB/provider mới nhất, không giả lập thành công |
| Apply/rollback setting | POST versioned diff | Current version tăng/khôi phục đúng value; audit entry hiện rõ |
| Gửi video TikTok | Publish review → POST submit | Không gửi media trước consent; account/caption/privacy/disclosure gửi đúng lựa chọn; kết quả dùng provider state |
| TikTok Shop anchor thất bại | Publish review → job detail | `anchor_state=failed`; bài không hiện `Đã đăng kèm sản phẩm`; có manual handoff nếu Draft khả dụng |
| Offer không khớp Shop product | Publish review | Submit bị chặn; hiện hai ID và action sửa mapping |
| Public publish chưa được cấp quyền | Publish review | Public mode bị chặn; UI nêu `Draft` hoặc `Cần hoàn tất trong TikTok` |
