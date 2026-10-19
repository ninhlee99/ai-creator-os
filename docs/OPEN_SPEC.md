# OPEN-SPEC: CHUẨN VIDEO CÓ HỒN & QUAY PHIM TỰ NHIÊN
## (Natural Cinematic & Soulful Video Specification — SoulVideo Spec v1.0)

> **Trạng thái**: Official Specification (Chính Thức)  
> **Phiên bản**: 1.0.0  
> **Phạm vi áp dụng**: Toàn bộ hệ sinh thái `ai-creator-os` (`internal/studio`, `internal/web`, `internal/engines`, `automation`)  
> **Nguyên tắc nền tảng**: *"Một video chỉ có giá trị khi chạm đến cảm xúc người xem, mang sinh khí tự nhiên của con người và chuyển động điện ảnh chân thực — Tuyệt đối loại bỏ video dán ảnh, giật lắc cơ học và nền đen vô cảm."*

---

## MỤC LỤC
1. [Bối Cảnh & Đánh Giá Hiện Trạng Hệ Thống](#1-bối-cảnh--đánh-giá-hiện-trạng-hệ-thống)
2. [Triết Lý Cốt Lõi: Thế Nào Là "Video Có Hồn"?](#2-triết-lý-cốt-lõi-thế-nào-là-video-có-hồn)
3. [Thất Pháp Video Có Hồn (The 7 Laws of Soulful Video)](#3-thất-pháp-video-có-hồn-the-7-laws-of-soulful-video)
4. [Đặc Tả Kỹ Thuật 3 Phân Hệ Sản Xuất Chính](#4-đặc-tả-kỹ-thuật-3-phân-hệ-sản-xuất-chính)
   - [4.1. Phân hệ Affiliate Lookbook Chuyển Động Người Thật](#41-phân-hệ-affiliate-lookbook-chuyển-động-người-thật)
   - [4.2. Phân hệ Viral Dance & Trendjack TikTok VN / Douyin](#42-phân-hệ-viral-dance--trendjack-tiktok-vn--douyin)
   - [4.3. Phân hệ Truyện Audio Điện Ảnh 30 – 60 Phút (YouTube Cinema)](#43-phân-hệ-truyện-audio-điện-ảnh-30--60-phút-youtube-cinema)
   - [4.4. Động Cơ AI Sinh Video Chuyển Động: Google Veo 3 & Muse AI](#44-động-cơ-ai-sinh-video-chuyển-động-google-veo-3--muse-ai)
5. [Chuẩn Hình Học & Tỷ Lệ Khung Hình 9:16 (Anti-Distortion Geometry)](#5-chuẩn-hình-học--tỷ-lệ-khung-hình-916-anti-distortion-geometry)
6. [Hệ Thống Âm Thanh Đa Tầng & Hồn Nhạc (Multi-Layer Acoustic Soul)](#6-hệ-thống-âm-thanh-đa-tầng--hồn-nhạc-multi-layer-acoustic-soul)
7. [Mô Hình Dữ Liệu & Hợp Đồng Dịch Vụ Mở (Open-Spec Schemas)](#7-mô-hình-dữ-liệu--hợp-đồng-dịch-vụ-mở-open-spec-schemas)
8. [Bộ Chỉ Số Đánh Giá Chất Lượng: Soul Index (QC Matrix)](#8-bộ-chỉ-số-đánh-giá-chất-lượng-soul-index-qc-matrix)

---

## 1. BỐI CẢNH & ĐÁNH GIÁ HIỆN TRẠNG HỆ THỐNG

### 1.1. Những Khiếm Khuyết Nghiêm Trọng Của Hệ Thống Cũ (The Pain Points)
Trước khi chuẩn hóa Open-Spec, hệ thống AI sinh video tự động thường mắc phải 5 căn bệnh chí mạng khiến sản phẩm làm ra bị người dùng đánh giá là "vô hồn", "như slide PowerPoint rẻ tiền" và "không thể dùng để kinh doanh":

1. **Bệnh "Méo Xẹo" Hình Học (Aspect Ratio Distortion)**:
   - Các ảnh chụp gốc tỷ lệ 16:9 (`1280x720`) bị ép co dãn trực tiếp thành 9:16 (`1080x1920`) qua lệnh `scale=w:h` đơn thuần.
   - Hậu quả: Khuôn mặt người mẫu bị kéo dài ngoằng hoặc bẹp dúm, tỉ lệ cơ thể dị dạng, sản phẩm méo mó làm mất hoàn toàn tính thẩm mỹ.
2. **Bệnh "Dán Ảnh Tĩnh & Chèn Thẻ Chữ Thô Kệch" (Static PowerPoint Syndrome)**:
   - Thay vì quay người thật trải nghiệm, hệ thống lấy 1 ảnh sản phẩm tĩnh, chèn các khối chữ lớn (`AdCard` xanh đỏ tím vàng) đè lên hình rồi dùng lệnh `zoompan` phóng to thu nhỏ.
   - Hậu quả: Video giống như bản trình chiếu thuyết trình nghiệp dư; người xem trên TikTok/Reels lướt qua ngay lập tức trong 0.5 giây đầu vì cảm giác bị "quảng cáo rác".
3. **Bệnh "Giật Lắc Nhức Mắt Cơ Học" (Nauseating Shake & Pulsing)**:
   - Áp dụng các thuật toán rung lắc toán học thô bạo (`30*sin()` và `24*cos()` cùng `pow(sin(), 2)`) khiến toàn bộ khung hình giật liên hồi từng giây một cách vô lý.
   - Hậu quả: Gây chóng mặt, nhức mắt cho người xem, hoàn toàn đối lập với phong cách quay phim mượt mà của các nhà sáng tạo nội dung chuyên nghiệp.
4. **Bệnh "Nền Đen Câm Lặng" Trong Truyện Audio (Black Screen Void)**:
   - Trong phân hệ Kể Chuyện Audio dài tập, hệ thống để mặc video trôi qua trên nền đen thui hoặc 1 ảnh tĩnh duy nhất không chuyển biến suốt hàng chục phút.
   - Hậu quả: Người xem YouTube rời bỏ video vì tưởng màn hình bị lỗi; thuật toán YouTube đánh giá tỉ lệ giữ chân (Retention) thấp và bóp tương tác.
5. **Bệnh "Giọng Đọc Robot & Âm Nhạc Lạc Quẻ" (Monotone Acoustic Disconnect)**:
   - Lồng tiếng tự động bằng giọng máy vô cảm, không nhấn nhá nhịp điệu, âm lượng giọng át cả nhạc hoặc chọn các bản nhạc ambient tẻ nhạt thay vì nhạc đang thịnh hành trên TikTok/Douyin.

### 1.2. Mục Tiêu Tái Thiết Hệ Thống (The Redesign Mandate)
Hệ thống `ai-creator-os` được nâng cấp toàn diện lên kiến trúc **Cinematic Natural Engine**, đảm bảo:
- 100% video xuất xưởng đạt chuẩn **Full HD 9:16 (1080x1920)** với thuật toán Scale & Crop thông minh chống méo hình.
- 100% video thời trang & affiliate dùng **chuyển động người mẫu thật trong không gian thực tế** (Lookbook Video), ứng dụng hoán đổi khuôn mặt AI (Face Swap) để giữ thương hiệu cá nhân của creator.
- 100% video nhảy viral được khai thác từ **các kênh triệu view TikTok Việt Nam & Douyin Trung Quốc**, bắt đúng giai điệu hot trend.
- 100% video truyện audio được trang bị **visual canvas điện ảnh đa cảnh liên tục**, kết hợp âm thanh môi trường (Foley) và âm nhạc cảm xúc, tuyệt đối không có khung hình đen.

---

## 2. TRIẾT LÝ CỐT LÕI: THẾ NÀO LÀ "VIDEO CÓ HỒN"?

Một video được định nghĩa là **"Có Hồn" (Soulful Video)** khi và chỉ khi thỏa mãn 4 yếu tố sinh khí:

```
                  ┌─────────────────────────────────────┐
                  │          VIDEO CÓ HỒN               │
                  │   (Natural Cinematic Soul)          │
                  └──────────────────┬──────────────────┘
                                     │
      ┌──────────────────┬───────────┴───────────┬──────────────────┐
      │                  │                       │                  │
┌─────▼─────┐      ┌─────▼─────┐           ┌─────▼─────┐      ┌─────▼─────┐
│ SINH KHÍ  │      │ CHUYỂN ĐỘNG│          │ GIAI ĐIỆU │      │ DÒNG CHẢY │
│ CON NGƯỜI │      │ ĐIỆN ẢNH   │          │ CẢM XÚC   │      │ LIÊN TỤC  │
│ (Humanity)│      │(Cinematic) │          │(Acoustics)│      │(Continuity)│
└───────────┘      └───────────┘           └───────────┘      └───────────┘
 Ánh mắt, nụ cười,  Góc máy trôi êm ái,    Nhạc trend bắt tai,  Hình ảnh biến
 tương tác tay-sản  cắt nhịp CapCut,       âm thanh môi trường, chuyển theo cốt
 phẩm, vũ đạo sống  lướt nhẹ như quay     giọng kể có hồn,    truyện, 0 giây
 động chân thực.    tay của người thật.    nhấn nhá xúc cảm.   màn hình đen.
```

1. **Sinh khí con người (Human Vitality)**: Có nhân vật sống động — ánh mắt có tiêu cự, chuyển động bước đi, xoay người, thử đồ, nụ cười biểu cảm hoặc vũ đạo uyển chuyển. Khán giả kết nối với con người, không kết nối với khối chữ.
2. **Chuyển động điện ảnh tự nhiên (Organic Kinematics)**: Khung hình chuyển động như thể được ghi lại bởi một nhà quay phim (Cinematographer) cầm gimbal hoặc máy ảnh cơ: chuyển động trôi nhẹ (*handheld organic drift*), lia máy tinh tế (*gentle pan/tilt*), chuyển cảnh dứt khoát tại điểm rơi âm nhạc (*beat drop match cut*).
3. **Giai điệu & Âm thanh rung động (Acoustic Resonance)**: Nhạc nền là mạch máu của video ngắn. Phải là những điệu nhạc đang khuấy đảo bảng xếp hạng TikTok/Douyin, khớp nhịp từng giây với động tác của nhân vật.
4. **Dòng chảy hình ảnh không ngắt quãng (Unbroken Visual Flow)**: Mỗi giây trôi qua đều có thông tin thị giác mới mẻ để giữ chân mắt người xem, loại bỏ mọi điểm chết màn hình.

---

## 3. THẤT PHÁP VIDEO CÓ HỒN (THE 7 LAWS OF SOULFUL VIDEO)

Mọi pipeline render video trong `ai-creator-os` bắt buộc phải tuân thủ nghiêm ngặt 7 luật sau:

### Điều 1: Luật Bất Biến Về Tỷ Lệ Khung Hình (Zero-Distortion Geometry)
> *Cấm tuyệt đối mọi hình thức co dãn cưỡng bức (`stretch`) làm biến dạng khuôn mặt hoặc vật thể.*  
Mọi tài nguyên đầu vào (ảnh, clip ngang 16:9, ảnh vuông 1:1) khi đưa vào khung hình 9:16 (1080x1920) bắt buộc phải đi qua bộ lọc tăng kích thước theo tỷ lệ gốc rồi cắt tâm (`force_original_aspect_ratio=increase,crop=1080:1920,setsar=1`).

### Điều 2: Luật Chuyển Động Người Thật (Living Subject Law)
> *Trong video thương mại affiliate và viral, chủ thể chính phải là con người đang hoạt động thật.*  
- Đối với thời trang, mỹ phẩm, phụ kiện: Sử dụng video lookbook người mẫu đang mặc, sải bước, ngắm gương hoặc cầm nắm sản phẩm.
- Tuyệt đối không thay thế người mẫu bằng việc chụp một bức ảnh áo/quần treo trên móc rồi phóng to thu nhỏ.

### Điều 3: Luật Ngôn Ngữ Ống Kính CapCut (Cinematographic Editing Standard)
> *Nói không với hiệu ứng rung lắc méo hình rẻ tiền; áp dụng chuẩn chuyển cảnh CapCut Pro.*  
- Khi chuyển giữa các phân cảnh ảnh/clip: Sử dụng hiệu ứng chớp sáng trắng siêu ngắn (`fade=t=in:st=0:d=0.12:color=white`) đồng bộ theo nhịp trống (*beat drop*).
- Hiệu ứng ống kính chậm (Ken Burns): Độ phóng đại cực đại chỉ dao động trong khoảng $1.00 \to 1.08$ (tối đa 8%), giữ nguyên đỉnh đầu và bàn tay người mẫu trong khung hình an toàn (*safe area*).

### Điều 4: Luật Bản Sắc Kênh (Identity Lock via Deep Latent Embedding)
> *Nhân vật trong video phải đồng nhất với thương hiệu của chủ kênh.*  
Hệ thống sử dụng mô hình trí tuệ nhân tạo nhận diện đặc trưng khuôn mặt (InsightFace ResNet-50/Antelopev2) với vector tiềm ẩn 512 chiều, căn chỉnh 5 điểm mốc nhân trắc học (*5 facial landmarks: 2 mắt, chóp mũi, 2 khóe môi*) và hòa trộn trường da quang học (*Poisson Seamless Blending*) để ghép khuôn mặt người sáng tạo vào thân hình người mẫu lookbook hoặc vũ công một cách mượt mà, không để lại viền cắt.

### Điều 5: Luật Nhạc Nền Thịnh Hành (Trending Sound Priority)
> *Nhạc nền phải là chất xúc tác giữ chân người nghe, cập nhật realtime.*  
- Video nhảy và video ngắn thời trang ưu tiên 100% nhạc nền thịnh hành từ TikTok Việt Nam (Top Kworb/TikTok Hot Sounds) hoặc Douyin Trung Quốc.
- Trong video bán hàng: Nhạc nền thương mại bắt tai, không lồng tiếng AI máy móc khi không có kịch bản chuyên sâu để tránh phá vỡ cảm xúc thời trang tự nhiên.

### Điều 6: Luật Cảnh Quan Kể Chuyện Tuyệt Đối Không Nền Đen (Zero-Black-Frame Storytelling)
> *Truyện audio dài 30 - 60 phút là một tác phẩm điện ảnh thu nhỏ.*  
- Mỗi 8 - 15 giây kịch bản phải tương ứng với một phân cảnh visual sống động: phong cảnh thiên nhiên, phòng trà đêm mưa, góc phố cổ, nội thất điện ảnh hoặc hoạt cảnh minh họa tạo bởi AI tạo sinh phân giải cao (Flux / Pollinations / Gemini).
- 100% video truyện audio phải có 3 tầng âm thanh: Giọng đọc truyền cảm (VieNeu Neural TTS) + Âm thanh môi trường chân thực (tiếng mưa rơi, tiếng gió rít, tiếng bước chân) + Nhạc nền cinematic du dương lót dưới âm lượng -18dB.

### Điều 7: Luật Điểm Nhấn 3 Giây Đầu & Khung Hình Sạch 100% (The 3-Second Clean Hook Rule)
> *3 giây đầu quyết định 90% thành công của một video ngắn — Giữ khung hình sạch sẽ 100% như camera điện thoại quay tự nhiên.*  
- Khung hình đầu tiên lập tức thể hiện một chuyển động người thật bắt mắt: một động tác vũ đạo cuốn hút, một góc quay cận cảnh trang phục tôn dáng, hoặc thần thái sống động của nhân vật.
- **Loại bỏ hoàn toàn phụ đề và chữ dán đè (No In-Video Subtitles / No Text Burn-in)**: Khung hình sạch 100%, không bị che khuất bởi chữ hay phụ đề. Mọi thông điệp và lời kêu gọi được truyền tải bằng giọng nói, caption bài viết và trang Bio Link Storefront (`/bio`).
- **Loại bỏ hiệu ứng và animation giả tạo**: Video chỉ quay mộc như camera điện thoại thông thường, không flash giật, không rung lắc cơ học, không animation hoạt họa.

---

## 4. ĐẶC TẢ KỸ THUẬT 3 PHÂN HỆ SẢN XUẤT CHÍNH

### 4.1. Phân hệ Affiliate Lookbook Chuyển Động Người Thật

```
                    QUY TRÌNH LOOKBOOK AFFILIATE CHUYỂN ĐỘNG THẬT
  ┌─────────────────┐       ┌────────────────────┐       ┌─────────────────────┐
  │ Sản Phẩm Listing│ ────> │ Thư Viện Lookbook  │ ────> │  InsightFace AI     │
  │ (Túi, Váy, Áo)  │       │ 9:16 Người Thật    │       │  Face Swap Engine   │
  └─────────────────┘       └────────────────────┘       └──────────┬──────────┘
                                                                    │
  ┌─────────────────┐       ┌────────────────────┐                  │
  │ Render Hoàn Tất │ <──── │ CapCut Beat Cutter │ <────────────────┘
  │ 1080x1920 MP4   │       │ + Nhạc Trend TikTok│
  └─────────────────┘       └────────────────────┘
```

#### Quy chuẩn đầu vào & Chế độ dựng:
1. **Chế độ Lookbook Video Tự Nhiên (Khuyên Dùng - Mặc Định)**:
   - Hệ thống tự động kích hoạt kho footage Lookbook thời trang HD 9:16 (`data/clips/fashion/*.mp4`):
     * `clip_01_fashion_dress.mp4`: Người mẫu nữ sải bước trong không gian boutique sang trọng, đầm thanh lịch, chuyển động tự nhiên 360 độ.
     * `clip_02_fashion_streetwear.mp4`: Người mẫu dạo phố phong cách năng động, phối đồ cá tính, cận cảnh phụ kiện và túi xách.
     * `clip_03_fashion_lookbook_60s.mp4`: Phim thời trang lookbook 60 giây hoàn chỉnh (chuẩn TikTok Creator Rewards 1 phút+).
   - **Kích hoạt Face Swap**: Nếu người dùng chọn ảnh người mẫu của kênh (`ModelPhoto`), hệ thống kích hoạt InsightFace hoán đổi khuôn mặt vào người mẫu lookbook trong video chuyển động.
2. **Chế độ Ảnh Sản Phẩm Listing (`AffiliateModePhoto`)**:
   - Khi người dùng tải lên ảnh chụp sản phẩm thật: Hệ thống áp dụng hiệu ứng chuyển động Ken Burns cao cấp (tối đa 1.08x) và chuyển cảnh Flash Cut theo nhịp nhạc beat.
   - Tuyệt đối loại bỏ các khối viền đồ họa to dày che mất ảnh sản phẩm; chuyển toàn bộ thông tin khuyến mãi sang phần Text Caption chuẩn SEO và TikTok Bio Link.
3. **Âm nhạc**:
   - Tự động lấy nhạc nền thương mại TikTok Shop thịnh hành (`internal/studio/assemble.go:FindTrendingMusic`).

---

### 4.2. Phân hệ Viral Dance & Trendjack TikTok VN / Douyin

```
              QUY TRÌNH SẢN XUẤT VIDEO VŨ ĐẠO VIRAL 1-CHẠM
  ┌───────────────────────┐         ┌─────────────────────────┐
  │ Nguồn Vũ Đạo Hot Trend│ ──────> │ Bộ Lọc Khu Vực & Tải Về │
  │ TikTok VN / Douyin CN │         │ yt-dlp + FFmpeg Conform │
  └───────────────────────┘         └────────────┬────────────┘
                                                 │
  ┌───────────────────────┐         ┌────────────▼────────────┐
  │ Xuất Bản Đa Nền Tảng  │ <────── │ Ghép Mặt Nhân Vật Kênh  │
  │ TikTok, Shorts, Reels │         │ InsightFace 512-dim     │
  └───────────────────────┘         └─────────────────────────┘
```

#### Danh mục Vũ đạo Radar Triệu View tích hợp sẵn:
- **TikTok Việt Nam**:
  * Đào Lê Phương Hoa (`@hoadancer`): Vũ đạo sexy dance nhịp điệu cực mạnh, biểu cảm hút hồn.
  * C.A.C Dance Crew (`@cacdance`): Vũ đạo đường phố hiện đại, biên đạo đồng đều, năng động.
  * Hot Beat Bedroom Dance: Vũ đạo gợi cảm trong phòng, ánh sáng neon ấm áp.
- **Douyin Trung Quốc (抖音)**:
  * 抖音卡点热舞 (Douyin Beat Sync Hot Dance): Vũ đạo chuẩn từng nhịp trống, trang phục thời thượng, visual lung linh.
  * 抖音女团元气齐舞 (Douyin K-Pop Girlgroup Dance): Vũ đạo nhóm nữ năng lượng cao, tươi vui.
- **Công cụ Nhập Link Bất Kỳ (URL Importer)**:
  * Endpoint `POST /viral/clips/import` nhận bất kỳ link TikTok VN (`tiktok.com/@...`) hoặc Douyin Trung Quốc (`v.douyin.com/...`).
  * Tự động xóa logo/watermark, chuẩn hóa về 1080x1920 30fps và đưa ngay vào danh mục chọn lựa.

#### Nguyên Tắc Phản Chiếu Video Viral Gốc (Original Viral Mirroring Law):
*Tùy vào video viral gốc như thế nào thì video được tạo bởi AI sẽ có hiệu ứng và phong cách tương tự 1:1:*
1. **Kế thừa 100% âm thanh & bài nhạc gốc**: Tách và lồng lại bản nhạc gốc của video viral (nếu không chỉ định nhạc riêng). Từng nhịp beat drop, tiếng bass/snare và điểm rơi cao trào đều khớp hoàn hảo với động tác vũ đạo của nhân vật.
2. **Kế thừa 100% thời lượng tự nhiên**: Thời lượng video AI kết thúc chính xác cùng lúc với clip mẫu, không cắt cụt bài nhảy và không lặp loop gượng gạo.
3. **Phản chiếu chuyển động & visual mood**: Toàn bộ góc máy, chuyển động cơ thể, động tác nhảy và ánh sáng của clip triệu view được giữ nguyên vẹn qua công nghệ AI Face Swap, biến video thành tác phẩm độc quyền của Creator mà vẫn mang trọn vẹn năng lượng của video viral gốc.

---

### 4.3. Phân hệ Truyện Audio Điện Ảnh 30 – 60 Phút (YouTube Cinema)

```
             QUY TRÌNH DỰNG PHIM TRUYỆN AUDIO ĐIỆN ẢNH
  ┌────────────────────┐     ┌──────────────────────┐     ┌──────────────────────┐
  │ Kịch Bản Truyện Dài│ ──> │ Phân Đoạn 15s/Cảnh   │ ──> │ Sinh Visual 9:16 HD  │
  │ (30 - 60 Phút)     │     │ (LLM Scene Director) │     │ Flux AI / B-Roll Cine│
  └────────────────────┘     └──────────────────────┘     └──────────┬───────────┘
                                                                     │
  ┌────────────────────┐     ┌──────────────────────┐                │
  │ Master Video 9:16  │ <── │ Hòa Âm Không Gian    │ <──────────────┘
  │ Full HD + Phụ Đề   │     │ Voice + Foley + Pad  │
  └────────────────────┘     └──────────────────────┘
```

#### Quy chuẩn điện ảnh chống màn hình đen:
1. **Mật độ phân cảnh (Scene Density)**:
   - Thời lượng mỗi cảnh: 8 đến 15 giây.
   - Một tập truyện dài 30 phút chứa khoảng $120 \to 180$ phân cảnh thị giác liên tục; tập truyện 60 phút chứa $240 \to 360$ phân cảnh.
2. **Chất lượng hình ảnh thị giác (Cinematic Visual Canvas)**:
   - Từng phân cảnh được sinh bởi mô hình AI đồ họa cao cấp (Pollinations Flux / Gemini 2.0 Flash Image Generation) bám sát mô tả cảm xúc của cốt truyện (ví dụ: *"đêm mưa phố cổ, ánh đèn vàng hắt hiu trên mặt đường ướt, tông màu điện ảnh ấm áp"*).
   - Tích hợp video b-roll phong cảnh chuyển động thực tế (mưa rơi ngoài cửa sổ, khói cà phê nghi ngút, lá vàng rơi) để tạo chiều sâu không gian.
3. **Cấu trúc âm thanh 3 lớp (Triple-Layer Spatial Soundscape)**:
   - **Lớp 1 (Narrative Lead)**: Giọng đọc truyền cảm tự nhiên (VieNeu Neural TTS / Edge Neural `vi-VN-HoaiMyNeural` hoặc `vi-VN-NamMinhNeural`).
   - **Lớp 2 (Ambient Foley)**: Tiếng môi trường thực tế (tiếng mưa dầm, tiếng gió đêm xào xạc, tiếng chuông chùa xa xa) đặt ở mức âm lượng -14dB.
   - **Lớp 3 (Emotional Soundtrack)**: Nhạc đệm piano/orchestra cinematic nhẹ nhàng, tự động giảm âm lượng (*audio ducking*) -8dB mỗi khi người dẫn chuyện cất tiếng.

---

### 4.4. Động Cơ AI Sinh Video Chuyển Động: Google Veo 3 & Muse AI

Nhằm triệt tiêu dứt điểm tình trạng video dán ảnh tĩnh và phóng to thu nhỏ (Ken Burns), hệ thống tích hợp trực tiếp 2 động cơ sinh video AI tiên tiến nhất hiện nay:

```
           MÔ HÌNH CASCADE ĐIỀU PHỐI VIDEO CHUYỂN ĐỘNG AI THỐNG NHẤT
  ┌────────────────────────────────────────────────────────────────────────┐
  │                 Yêu Cầu Sinh Phân Cảnh Chuyển Động                     │
  │     (Prompt chi tiết, Keyframe gốc 9:16, Thời lượng 4-8s)              │
  └───────────────────────────────────┬────────────────────────────────────┘
                                      │
              ┌───────────────────────┴───────────────────────┐
              ▼                                               ▼
   ┌───────────────────────┐                       ┌───────────────────────┐
   │    GOOGLE VEO 3       │                       │       MUSE AI         │
   │  veo-3.1-preview      │                       │  API & Web AI Pool    │
   │  veo-3.0-preview      │                       │  Keyframe Upload      │
   │  veo-2.0-001          │                       │  MP4 Direct Fetch     │
   └──────────┬────────────┘                       └───────────┬───────────┘
              │                                                │
              └───────────────────────┬────────────────────────┘
                                      │
                                      ▼
                        ┌───────────────────────────┐
                        │   FFmpeg Lanczos Conform   │
                        │   1080x1920 (9:16) 30fps  │
                        │   Anti-Distortion & SAR 1:1│
                        └─────────────┬─────────────┘
                                      │
                                      ▼
                        ┌───────────────────────────┐
                        │    Master ConcatClips     │
                        │    Ghép nối video hoàn chỉnh│
                        └───────────────────────────┘
```

#### Quy chuẩn vận hành:
1. **Google Veo 3 Engine (`VeoClient`)**:
   - Sử dụng endpoint Gemini API `/v1beta/models/{model}:predictLongRunning` với cơ chế xoay vòng candidate models: `veo-3.1-generate-preview` $\to$ `veo-3.0-generate-preview` $\to$ `veo-2.0-generate-001`.
   - Chế độ sinh: Text-to-Video và Image-to-Video (`firstFrame` Base64) để duy trì sự nhất quán của nhân vật và bối cảnh.
   - Polling trạng thái qua REST LRO với cơ chế Exponential Backoff (3s $\to$ 5s $\to$ 8s) cho đến khi hoàn thành xuất file MP4.
2. **Muse AI Studio Engine (`MuseAIClient`)**:
   - Tự động nhận diện khóa bảo mật `MUSE_AI_API_KEY` hoặc session cookie `MUSE_AI_SESSION` từ Web AI Multi-Account Hub.
   - Tải keyframe lên Muse Cloud Storage qua multipart upload, kích hoạt render video motion mượt mà và tự động tải MP4 về local storage.
3. **Thác Nước Thông Minh (Intelligent Cascade & Fallback)**:
   - Khi chọn `video_engine = "auto"`, hệ thống thử nghiệm Veo 3 trước; nếu gặp quota limit (429) hoặc timeout, tự động chuyển tiếp sang Muse AI.
   - Nếu cả 2 AI engine chưa có cấu hình API key, hệ thống tự động fallback về kho footage người thật chuyển động (Lookbook / Viral Dance) đã chuẩn hóa trước khi áp dụng chuyển động Ken Burns vi sai ($< 1.08x$).

---

## 5. CHUẨN HÌNH HỌC & TỶ LỆ KHUNG HÌNH 9:16 (ANTI-DISTORTION GEOMETRY)

Mọi pipeline chuyển đổi hình ảnh và video trong hệ thống bắt buộc sử dụng chuẩn mã hóa FFmpeg chống méo hình học:

### 5.1. Công Thức Chuẩn Hóa Hình Học (The Scaling Law)

$$\text{Filter} = \texttt{scale=W:H:force\_original\_aspect\_ratio=increase:flags=lanczos,crop=W:H,setsar=1}$$

Trong đó:
- $W = 1080$, $H = 1920$ (đối với định dạng dọc 9:16 chuẩn TikTok/Reels/Shorts).
- $W = 1920$, $H = 1080$ (đối với định dạng ngang 16:9 chuẩn YouTube Cinema).
- `force_original_aspect_ratio=increase`: Giữ nguyên tỉ lệ góc của hình ảnh gốc, chỉ phóng to vừa đủ để che kín hoàn toàn khung hình đích mà **không bóp méo pixel nào**.
- `crop=W:H`: Cắt gọt phần rìa thừa cân đối hai bên hoặc trên dưới, giữ chủ thể chính luôn nằm ở trung tâm thị giác.
- `flags=lanczos`: Thuật toán nội suy Lanczos 3-lobe chất lượng điện ảnh, chống vỡ hạt và giữ độ sắc nét tối đa cho da người và sợi vải.
- `setsar=1`: Chuẩn hóa tỉ lệ pixel thành $1:1$ vuông vắn trên mọi thiết bị di động hiện đại.

---

## 6. HỆ THỐNG ÂM THANH ĐA TẦNG & HỒN NHẠC (MULTI-LAYER ACOUSTIC SOUL)

### 6.1. Ma trận xử lý âm thanh theo thể loại:
| Thể loại Video | Nguồn Âm Nhạc | Tầng Giọng Đọc | Tầng Foley Môi Trường | Tỉ lệ Hòa Âm |
|---|---|---|---|---|
| **Vũ Đạo Triệu View** | 100% Nhạc Viral TikTok/Douyin | Không (giữ nguyên tiếng hát gốc) | Không | Nhạc 100% (0 dB) |
| **Affiliate Lookbook** | Commercial Beat Trending TikTok | Tùy chọn (chỉ khi có kịch bản) | Không | Nhạc 85% + Voice (ducking -8dB) |
| **Truyện Audio 30-60p** | Cinematic Ambient Piano/Strings | VieNeu/Edge Neural truyền cảm | Mưa / Gió / Đêm phố cổ | Voice 0dB + Nhạc -18dB + Foley -14dB |

---

## 7. MÔ HÌNH DỮ LIỆU & HỢP ĐỒNG DỊCH VỤ MỞ (OPEN-SPEC SCHEMAS)

### 7.1. Cấu Trúc Yêu Cầu Render Video Chuẩn (SoulJobSpec JSON Schema)

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "SoulJobSpec",
  "type": "object",
  "required": ["version", "kind", "aspect_ratio", "quality_preset"],
  "properties": {
    "version": { "type": "string", "enum": ["1.0.0"] },
    "kind": { "type": "string", "enum": ["affiliate_lookbook", "viral_dance", "audio_story_cinema"] },
    "aspect_ratio": { "type": "string", "enum": ["9:16", "16:9"], "default": "9:16" },
    "quality_preset": { "type": "string", "enum": ["cinema_5star", "pro_hd", "fast_preview"], "default": "cinema_5star" },
    "identity": {
      "type": "object",
      "properties": {
        "model_photo_path": { "type": "string" },
        "enable_face_swap": { "type": "boolean", "default": true },
        "swap_engine": { "type": "string", "enum": ["insightface_latent", "none"], "default": "insightface_latent" }
      }
    },
    "visual_pipeline": {
      "type": "object",
      "properties": {
        "source_mode": { "type": "string", "enum": ["lookbook_motion", "viral_dance_clip", "generative_flux_storyboard"] },
        "sample_clip_id": { "type": "string" },
        "conforming_filter": { "type": "string", "default": "lanczos_increase_crop" },
        "transition_type": { "type": "string", "enum": ["capcut_flash_beat_cut", "kenburns_gentle", "cinematic_dissolve"] }
      }
    },
    "acoustic_pipeline": {
      "type": "object",
      "properties": {
        "music_source": { "type": "string", "enum": ["tiktok_vn_trending", "douyin_beat_hot", "cinematic_ambient", "custom"] },
        "voiceover_engine": { "type": "string", "enum": ["vieneu_neural_vn", "edge_hoaimy_neural", "none"] },
        "enable_foley_ambience": { "type": "boolean", "default": false },
        "ducking_db": { "type": "number", "default": -8.0 }
      }
    }
  }
}
```

---

## 8. BỘ CHỈ SỐ ĐÁNH GIÁ CHẤT LƯỢNG: SOUL INDEX (QC MATRIX)

Mọi video sau khi render bắt buộc phải vượt qua bài kiểm tra chất lượng tự động (*Soul Index QC*) với điểm số tối thiểu **4.2 / 5.0 sao**:

| Tiêu Chí Đánh Giá | Trọng Số | Yêu Cầu Đạt Chuẩn 5 Sao (Cinematic 5-Star) | Mức Phạt Nếu Vi Phạm |
|---|---|---|---|
| **1. Tính Chân Thực & Con Người** | 30% | Người thật chuyển động tự nhiên (Lookbook / Vũ đạo), khuôn mặt đồng nhất 100%, không bị dị dạng. | Đánh rớt nếu dùng ảnh tĩnh dán chữ `AdCard`. |
| **2. Hình Học & Khung Hình 9:16** | 25% | Đúng $1080 \times 1920$, tỷ lệ khung hình nguyên bản, không co kéo méo mặt, không viền đen lỗi. | Đánh rớt nếu pixel bị bóp méo. |
| **3. Âm Nhạc & Nhịp Điệu Hồn Video** | 20% | Nhạc thịnh hành TikTok/Douyin, khớp nhịp beat drop, không bị rè hoặc ngắt quãng cụt lủn. | Trừ 2.0 sao nếu nhạc tắt ngúm hoặc tẻ nhạt. |
| **4. Kỹ Thuật Dựng & Chuyển Cảnh** | 15% | Cắt nhịp CapCut mượt mà, chớp sáng trắng tinh tế, Ken Burns nhẹ nhàng $< 8\%$, không giật lắc nhức mắt. | Đánh rớt nếu dùng thuật toán rung lắc cơ học. |
| **5. Dòng Chảy & Giữ Chân Người Xem** | 10% | 3 giây đầu bắt mắt, 0 giây màn hình đen trong toàn bộ thời lượng video. | Đánh rớt nếu có khung hình đen chết. |

---

> *Bản quyền đặc tả kỹ thuật thuộc về ai-creator-os Project. Mọi thay đổi kiến trúc hoặc cập nhật mã nguồn phải bảo lưu đầy đủ các điều khoản trong Open-Spec này.*
