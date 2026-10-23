# Phân Tích Khoảng Trống Sản Phẩm & Nhu Cầu Người Dùng Thực Tế

> ⚠️ Báo cáo lịch sử ngày 2026-10-17. Mục tiêu và nhu cầu đã được chuẩn hóa lại
> trong [PRODUCT_SPEC](PRODUCT_SPEC.md); trạng thái code xem tại
> [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md). Không dùng báo cáo này làm
> nguồn sự thật hiện hành.
**Báo cáo Đánh giá Trải nghiệm Hệ thống Dưới Góc Nhìn Nhà Sáng Tạo & Người Vận Hành Tự Động**

> **Tác giả:** Đội ngũ Kỹ thuật & Trải nghiệm Sản phẩm (MiuNova)
> **Phiên bản hệ thống:** v0.7.0 (100% Go Pure Binary, SQLite WAL)  
> **Ngày hoàn thành:** 17/10/2026

---

## 1. Tổng Quan & Định Vị Sản Phẩm

**MiuNova** là hệ thống tự chủ, đóng gói trong một binary Go, phục vụ 3 trụ cột doanh thu bền vững:
1. **Tiếp thị liên kết (Affiliate qua Accesstrade)**: Săn deal, tạo tracking link, đối soát hoa hồng.
2. **Video Viral AI 100% Gốc (TikTok/Shorts/Reels)**: Xưởng sáng tạo 9:16 nguyên bản với hook 3 giây tâm lý học, hình ảnh MediaGen, âm nhạc Kworb VN trending, loại bỏ triệt để rủi ro bản quyền và thuật toán phạt 0-view do reup.
3. **YouTube Kể Chuyện Ngôi Thứ Nhất**: Kịch bản tự sinh bằng LLM, chia phân cảnh, vẽ ảnh 16:9, lồng tiếng Gemini/Edge TTS, dựng video và tự động đăng private.

### Điểm mạnh cốt lõi hiện có:
- **Zero-Dependency Runtime**: 1 file nhị phân tĩnh duy nhất (~22MB), không cần cài Python, Node.js, venv hay Docker.
- **Tiết kiệm chi phí triệt để**: Triết lý *Free API first, local sidecar second, paid never*.
- **Kho Tài Khoản Web AI**: Lưu session được khôi phục từ JSON backup, kiểm tra session và tự xoay vòng account hợp lệ khi provider cho phép. Interactive browser login đã gỡ vì không thể đọc cookie từ browser bất kỳ.
- **Bộ não tự hành cấp cao (`/harness`)**: Vòng lặp nhận thức OODA (Observe - Orient - Decide - Act) cùng ký ức kinh nghiệm (Episodic Memory).

---

## 2. Chân Dung Người Dùng & Bối Cảnh Thực Tế

Hệ thống được thiết kế phục vụ 3 nhóm đối tượng:
1. **Affiliate Marketer / Commerce Creator**: Cần tìm sản phẩm hoa hồng cao, tạo hàng loạt video giới thiệu sản phẩm ngắn (15-30s), chèn link bio hoặc giỏ hàng, tối ưu tỷ lệ chuyển đổi (CR) và theo dõi doanh thu thực tế.
2. **Faceless Channel Builder (Xây kênh giấu mặt)**: Cần tạo kênh YouTube Kể Chuyện hoặc TikTok Viral chủ đề đời sống/showbiz/triết lý, đăng đều đặn mỗi ngày theo khung giờ vàng mà không cần lộ mặt hay thu âm thủ công.
3. **Solopreneur / Đội Vận Hành Tự Động (Hands-off Agency)**: Cần hệ thống chạy 24/7 trên máy Mac/VPS, tự động thích ứng khi tài khoản AI bị giới hạn, tự động đối soát đơn hàng và gửi báo cáo kết quả về điện thoại qua Telegram/Discord.

---

## 3. Phân Tích Khoảng Trống Chi Tiết Theo Từng Phân Hệ

Dưới góc nhìn một người dùng thực chiến, sau khi sử dụng toàn bộ tính năng và luồng công việc của hệ thống, các khoảng trống (Gaps) và nhu cầu chưa được đáp ứng (User Needs) được chỉ rõ như sau:

### 3.1. Trang Chủ & Quan Sát Hệ Thống (`/`)
* **Hiện trạng**: Hiển thị các thẻ tổng quan (Doanh thu Accesstrade, Đơn hàng, Tình trạng Kill switch, Trạng thái 4 AI, Log quyết định gần nhất).
* **Khoảng trống (Gaps)**:
  - Chưa có **biểu đồ trực quan hóa dữ liệu theo thời gian (Time-series Charts)**: Người dùng chỉ thấy số tổng mà không thấy được đường xu hướng 7 ngày / 30 ngày (doanh thu tăng hay giảm, ngày nào nổ đơn nhiều nhất).
  - Chưa có nút **1-Click Chạy Vòng Lặp Toàn Diện (Run Full Autonomous Loop)**: Khi muốn hệ thống quét săn deal ngay, sinh video ngay và kiểm tra đơn hàng ngay trong 1 nút bấm, hiện người dùng phải đi vào từng phân hệ để bấm tay.
* **Nhu cầu người dùng (User Needs)**:
  - Cần biểu đồ đường cong doanh thu và số lượt nhấp (Clicks vs Commissions).
  - Cần feed cảnh báo thời gian thực dạng pop-up/toast khi có đơn hàng mới phát sinh từ Accesstrade.

---

### 3.2. Quản Lý Kênh & Xuất Bản Đa Nền Tảng (`/accounts`, `/publishers`)
* **Hiện trạng**: Quản lý hồ sơ Brand DNA cho từng kênh; kết nối OAuth TikTok, YouTube, Meta (Facebook Page, Instagram Reels). TikTok hiện chạy ở chế độ **Bản nháp (Draft mode)** để tuân thủ chính sách kiểm duyệt.
* **Khoảng trống (Gaps)**:
  - **Thiếu cơ chế Đăng Đồng Loạt Đa Kênh (1-Click Multi-Channel Broadcast)**: Khi 1 video hoàn tất trong Studio hoặc Viral, người dùng muốn chọn 1 nút "Đăng lên TikTok + Shorts + Reels" với caption và hashtag tối ưu riêng cho từng nền tảng.
  - **Thiếu thông báo Đẩy Di Động khi Bản Nháp Sẵn Sàng**: Do TikTok chỉ hỗ trợ Draft, người dùng cần vào app TikTok trên điện thoại để bấm "Đăng". Nhưng hệ thống chưa có bot Telegram gửi link kèm thông báo: *"Video #104 đã render xong và đồng bộ nháp lên TikTok, bấm link này để mở app và duyệt đăng vào khung giờ vàng 19:30"*.
  - **Chưa có trình Xem Trước Bài Đăng (Social Post Preview)**: Người dùng không thấy được giao diện mô phỏng bài đăng thực tế trên điện thoại (video + thumbnail + caption + tag giỏ hàng) trước khi xuất bản.
* **Nhu cầu người dùng (User Needs)**:
  - Tích hợp Webhook thông báo Telegram kèm nút bấm mở TikTok app.
  - Bộ xem trước giao diện TikTok/Shorts (UI mockup simulator).

---

### 3.3. Kho Tài Khoản Web AI (`/settings/system#tab-webai`)
* **Hiện trạng hiện hành (2026-10-23)**: Tài khoản chỉ khôi phục qua JSON backup. Luồng interactive browser login đã gỡ theo U-11 vì app không thể đọc profile của browser người dùng bất kỳ. CDP cô lập chỉ còn phục vụ job tạo nội dung Flow/Muse, không đọc browser profile. HTTP 401 có thể đánh dấu hết hạn; HTTP 403/lỗi mạng là chưa xác minh và giữ cookie.
* **Khoảng trống (Gaps)**:
  - Không có cơ chế tự lấy cookie từ browser người dùng; account phải được khôi phục thủ công từ backup.
  - **Chưa có cảnh báo thời hạn cookie (TTL Estimator)**: Một số nền tảng (như ChatGPT hoặc Google) có chu kỳ làm mới cookie sau 14-30 ngày. Cần ghi nhận ngày đăng nhập và cảnh báo khi phiên làm việc quá 20 ngày.
  - **Thiếu thống kê Quota & Số Yêu Cầu Đã Dùng**: Chưa có bộ đếm hiển thị hôm nay tài khoản này đã sinh bao nhiêu kịch bản, còn bao nhiêu lượt trước khi chạm ngưỡng rate limit.
* **Nhu cầu người dùng (User Needs)**:
  - Nút "Kiểm tra tất cả phiên AI" trên giao diện.
  - Cảnh báo trực quan màu vàng/đỏ khi phiên sắp hết hạn.

---

### 3.4. Studio AI & Video Viral 9:16 (`/studio`, `/viral`)
* **Hiện trạng**: Dàn dựng video bán hàng (ảnh listing + nhạc), video đu trend 9:16 (hook 3s + MediaGen + lồng tiếng), truyện audio 16:9. Quản lý tiến trình jobs, render bằng FFmpeg thuần.
* **Khoảng trống (Gaps)**:
  - **Thiếu Trình Phát Video Trực Tiếp Trên Trình Duyệt (In-Browser Video Player Modal)**: Sau khi render xong video trong `/studio/jobs` hoặc `/viral`, người dùng phải tải file `.mp4` về máy để xem thử thay vì có thể bấm vào xem ngay lập tức trên giao diện web.
  - **Thiếu Công Cụ Tạo Phụ Đề Tự Động (Auto-Captions & File `.srt`)**: Trên TikTok/Reels, hơn 70% người dùng xem video ở chế độ tắt tiếng (mute). Video viral hiện có voiceover nhưng chưa tự sinh file phụ đề `.srt` hoặc dập chữ động (kinetic typography) lên khung hình.
  - **Hiệu ứng chuyển cảnh còn đơn giản**: Hiện FFmpeg sử dụng pan/zoom (Ken Burns) cơ bản. Người dùng mong muốn có thêm hiệu ứng chuyển động mượt mà hơn (zoom-in kịch tính khi đọc hook, zoom-out khi chuyển cảnh).
* **Nhu cầu người dùng (User Needs)**:
  - Modal xem video HTML5 tích hợp ngay trên danh sách Jobs.
  - Trình sinh phụ đề tự động đính kèm file video xuất bản.

---

### 3.5. Tiếp Thị Liên Kết & Kệ Hàng Sản Phẩm (`/products`)
* **Hiện trạng**: Kết nối API Accesstrade, săn sản phẩm theo từ khóa datafeed, tạo link tracking tự động, lưu vào kệ hàng SQLite (`products.db`), đối soát hoa hồng thật.
* **Khoảng trống (Gaps) — Rất Quan Trọng**:
  - **THIẾU TRANG BIO LINK CÔNG KHAI (Public Storefront / Bio Link Landing Page)**: Kệ hàng (`shelf`) đã có trong cơ sở dữ liệu, nhưng hệ thống lại **chưa có trang web công khai** (ví dụ: `http://localhost:8080/bio/{channel_name}` hoặc export ra file HTML tĩnh) để người dùng dán vào phần tiểu sử (Bio) trên kênh TikTok / Instagram. Người xem video muốn mua hàng thì bấm vào đâu? Nếu không có trang Bio link này, chuỗi chuyển đổi Affiliate bị đứt gãy hoàn toàn!
  - **Thiếu bộ lọc sản phẩm nâng cao**: Chưa có bộ lọc theo % hoa hồng cao nhất (ví dụ: chỉ lấy sản phẩm có hoa hồng > 15%), hoặc lọc theo chiến dịch D2C (Direct-to-Consumer) duyệt nhanh.
* **Nhu cầu người dùng (User Needs)**:
  - **Tính năng cực kỳ cấp thiết**: Trình tạo trang Bio Link thông minh với giao diện tối ưu cho điện thoại di động, tự động hiển thị các sản phẩm đang có trên kệ kèm ảnh, giá gốc, giá khuyến mãi và link tracking Accesstrade.

---

### 3.6. Kể Chuyện YouTube Ngôi Thứ Nhất (`/stories`)
* **Hiện trạng**: Tự động sinh cốt truyện, phân đoạn cảnh, vẽ ảnh minh họa 16:9, lồng tiếng Gemini/Edge TTS từng cảnh, ghép video 16:9 và tự động đăng YouTube ở chế độ riêng tư (Private).
* **Khoảng trống (Gaps)**:
  - **Thiếu Âm Thanh Môi Trường & Hiệu Ứng Nền (Ambient SFX / Background Soundscape)**: Video kể chuyện hiện chỉ có giọng đọc TTS, thiếu lớp âm thanh nền phù hợp với thể loại truyện (tiếng mưa rơi, tiếng bước chân, tiếng gió rít, nhạc nền du dương huyền bí).
  - **Thiếu cơ chế gom tập dài (Long-Form Compilation)**: Người nghe truyện YouTube thường thích nghe chuỗi tập 30–60 phút khi làm việc hoặc đi ngủ. Hệ thống hiện chỉ sản xuất từng mẩu truyện ngắn 3–5 phút độc lập.
* **Nhu cầu người dùng (User Needs)**:
  - Bộ trộn âm thanh nền tự động (Layering Ambient Audio).
  - Công cụ gộp nhiều tập truyện thành 1 video dài (Full Playlist Merger).

---

### 3.7. Bộ Não Tự Hành Cấp Cao (`/harness`)
* **Hiện trạng**: Chạy vòng lặp nhận thức OODA (Quan sát tình trạng kênh → Định hướng chiến lược → Quyết định hành động → Thực thi), lưu vết ký ức theo tập (Episodic Memory) trong SQLite.
* **Khoảng trống (Gaps)**:
  - **Thiếu Báo Cáo Định Kỳ Tự Động (Daily Executive Summary Report)**: Người vận hành muốn cứ đúng 22:00 mỗi đêm, Harness Agent sẽ tự tổng kết một bản tin cô đọng gửi qua Telegram: *"Hôm nay hệ thống đã tạo 4 video, kênh TikTok A tăng +120 followers, Accesstrade phát sinh 3 đơn hàng (hoa hồng tạm tính: 185.000 VNĐ), không có tài khoản AI nào bị lỗi"*.
  - **Chưa có chế độ Tự Động Điều Chỉnh Tần Suất Đăng theo Hiệu Suất (Dynamic Scheduling)**: Nếu một video buổi sáng đạt view đột biến (>10.000 view), hệ thống nên tự động tăng tốc sản xuất thêm video cùng chủ đề trong ngày.
* **Nhu cầu người dùng (User Needs)**:
  - Bản tin báo cáo tự động mỗi tối qua Telegram/Discord bot.

---

## 4. Bảng Tổng Hợp Khoảng Trống & Mức Độ Ưu Tiên

| Phân hệ | Tính năng | Tác động kinh doanh | Trạng thái hiện tại |
|---|---|---|---|
| **Affiliate** | **Trang Bio Link Storefront (`/bio/{channel}`)** | 🔴 Cực cao (Khai thông dòng tiền mua hàng) | ✅ **Đã hoàn thành 100%** (Mobile-first, tìm kiếm tức thì, lọc danh mục, link affiliate) |
| **Studio / Viral** | **Trình phát video HTML5 xem trực tiếp trong modal** | 🟡 Cao (Tiết kiệm thời gian review video) | ✅ **Đã hoàn thành 100%** (Modal theater mode, streaming HTTP 206 byte-range) |
| **Web AI Hub** | **Nút kiểm tra phiên (Test Alive) & Cảnh báo hạn dùng** | 🟡 Cao (Chủ động tránh đứt gãy tác vụ) | Một phần: có probe phiên; 403/lỗi mạng là chưa xác minh, chưa có TTL đáng tin. |
| **Web AI Hub** | **Đăng nhập tự động Muse AI qua OTP/2FA** | 🟡 Cao (Tự động bắt cookie sau khi nhập mã) | Đã gỡ theo U-11; khôi phục account qua JSON backup. |
| **Publishers** | **Telegram Webhook nhắc duyệt đăng nháp TikTok** | 🟡 Cao (Không bỏ lỡ khung giờ vàng) | ✅ **Đã hoàn thành 100%** (Tự động gửi tin nhắn báo nháp sẵn sàng) |
| **Studio / Viral** | **Tự tạo phụ đề file `.srt` cho video ngắn** | 🟡 Cao (Tăng 40% tương tác người xem mute) | ✅ **Đã hoàn thành 100%** (Sinh .srt chuẩn SubRip, mux soft-subs, nút tải srt) |

---

## 5. Ảnh Chụp Lịch Sử Ngày 2026-10-17

Các mục bên dưới ghi lại trạng thái được báo cáo khi tài liệu lập. Chúng không
chứng minh code hiện tại hoặc hành vi live; xem [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md).

Toàn bộ các tính năng cốt lõi phục vụ nhà sáng tạo nội dung đã được hoàn thiện trực tiếp và đưa vào mã nguồn chính thức:

1. **Trang Bio Link Storefront (`/bio` và `/bio/{channel_name}`)**:
   - Giao diện mobile-first hiện đại chuẩn Linktree/Bento.
   - Thanh tìm kiếm sản phẩm tức thì và bộ lọc danh mục.
   - Nút "Mua ngay" chuyển hướng an toàn qua `/bio-go/{id}` với link tracking Accesstrade.
   - Tích hợp nút sao chép link Bio ngay trên Kệ hàng `/products`.

2. **Trình phát video trực tiếp (In-Browser Video Player Modal)**:
   - Hỗ trợ HTTP 206 Partial Content byte-range streaming, tua mượt mà không nghẽn.
   - Thay thế toàn bộ video mẫu lỗi bằng `/static/sample_916.mp4`.
   - Nút ▶️ Xem trước được tích hợp xuyên suốt `/studio`, `/studio/jobs` và `/viral`.

3. **Web AI accounts hiện hành**:
   - Khôi phục account qua JSON backup; app không mở login browser và không đọc profile browser người dùng.
   - CDP cô lập chỉ chạy trong job tạo nội dung Flow/Muse. Cookie mới được lưu theo account khi provider trả cookie thay đổi.
   - Probe HTTP 403/lỗi mạng hiện phiên là chưa xác minh, không kết luận cookie sai.

4. **Tự động sinh phụ đề `.srt` cho Video Ngắn**:
   - Tự động sinh file SubRip `.srt` theo từng phân cảnh và mốc thời gian TTS.
   - Tự động mux phụ đề mềm (soft subtitles) vào file MP4 qua FFmpeg.
   - Bổ sung nút "Tải .srt" trực tiếp trên giao diện `/viral` và `/studio/jobs`.

5. **Bộ thông báo Telegram tự động**:
   - Tự động gửi thông báo qua Telegram kèm link khi video render hoàn tất hoặc bản nháp TikTok sẵn sàng.

6. **Kiểm tra phiên Web AI**:
   - Có nút kiểm tra tất cả và kiểm tra từng account trong Cài đặt Hệ thống.
   - HTTP 401 đánh dấu hết hạn; HTTP 403/lỗi mạng báo chưa xác minh và giữ cookie. Trạng thái account không chứng minh plan hoặc credits.
