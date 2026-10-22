# OPEN-SPEC: CHUẨN VIDEO CÓ HỒN & QUAY PHIM TỰ NHIÊN
## (Natural Cinematic & Soulful Video Specification — SoulVideo Spec v1.0)

> **Trạng thái**: Đặc tả mục tiêu — không phải bằng chứng tính năng đã triển khai
> **Phiên bản**: 1.0.0  
> **Phạm vi áp dụng**: Toàn bộ hệ sinh thái `ai-creator-os` (`internal/studio`, `internal/web`, `internal/engines`, `automation`)  
> **Nguyên tắc nền tảng**: *"Một video chỉ có giá trị khi chạm đến cảm xúc người xem, mang sinh khí tự nhiên của con người và chuyển động điện ảnh chân thực — Tuyệt đối loại bỏ video dán ảnh, giật lắc cơ học và nền đen vô cảm."*

> **Trạng thái runtime (2026-10-21):** Chính sách free-only hiện tắt tạo/chỉnh sửa video viral tự động; `/viral` từ chối enqueue vì adapter thay nhân vật cần billing. Affiliate chỉ nhận clip chuyển động do người vận hành cung cấp cho từng shot. QC kiểm thay đổi frame ở 2 fps và phân tích ba frame mỗi shot, nên chưa chứng minh chuyển động người thật liên tục hoặc sản phẩm được mặc/dùng ở mọi thời điểm. Các mô tả Veo/Muse, “1:1”, “100%” và các mục tiêu tương tự bên dưới là yêu cầu đích, không phải cam kết năng lực đang chạy.

> **Phạm vi tài liệu:** đây là bộ tiêu chuẩn video chuyên ngành kế thừa. Mục tiêu
> sản phẩm hiện hành nằm trong [PRODUCT_SPEC](PRODUCT_SPEC.md); năng lực code nằm
> trong [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md). Các mục tiêu cũ ở file
> này không mở rộng hoặc ghi đè phạm vi sản phẩm hiện hành.

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
Đích thiết kế **Cinematic Natural Engine** yêu cầu:
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
> *Ưu tiên footage ổn định như quay bằng tripod hoặc gimbal; chuyển cảnh phải êm và không gây giật mắt.*
- Affiliate mặc định dùng footage chuyển động thật, camera ổn định và tiêu cự/khoảng cách nhất quán.
- Cấm zoom in/out giả, dolly push/pull, flash, strobe, flicker, rung, lắc và whip-pan.
- Dựng từ ảnh tĩnh chỉ dùng dissolve nhẹ, không áp hiệu ứng camera giả.

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
- 100% video truyện audio phải có 3 tầng âm thanh: Giọng đọc Gemini/Edge TTS + âm thanh môi trường chân thực + nhạc nền cinematic du dương lót dưới âm lượng -18dB.

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
0. **Thời lượng chuẩn**: Tối thiểu **30s – 60s**. Nếu video ngắn hơn (dưới 5s), hệ thống tự động yêu cầu sinh liên tiếp nhiều video nối liền để giữ trọn vẹn chi tiết khung hình, nhân vật và trang phục (váy, áo, túi xách) nhất quán theo phong cách điện ảnh thực thụ.
1. **Chế độ Lookbook Video Tự Nhiên (Khuyên Dùng - Mặc Định)**:
   - Hệ thống tự động kích hoạt kho footage Lookbook thời trang HD 9:16 (`data/clips/fashion/*.mp4`):
     * `clip_01_fashion_dress.mp4`: Người mẫu nữ sải bước trong không gian boutique sang trọng, đầm thanh lịch, chuyển động tự nhiên 360 độ.
     * `clip_02_fashion_streetwear.mp4`: Người mẫu dạo phố phong cách năng động, phối đồ cá tính, cận cảnh phụ kiện và túi xách.
     * `clip_03_fashion_lookbook_60s.mp4`: Phim thời trang lookbook 60 giây hoàn chỉnh (chuẩn TikTok Creator Rewards 1 phút+).
   - **Kích hoạt Face Swap**: Nếu người dùng chọn ảnh người mẫu của kênh (`ModelPhoto`), hệ thống kích hoạt InsightFace hoán đổi khuôn mặt vào người mẫu lookbook trong video chuyển động.
2. **Chế độ Ảnh Sản Phẩm Listing (`AffiliateModePhoto`)**:
   - Khi người dùng chọn dựng từ ảnh: Giữ khung hình ổn định, chỉ dissolve nhẹ giữa ảnh; không Ken Burns, zoom, rung hoặc flash cut.
   - Chế độ mặc định tạo footage chuyển động bằng engine video. Nếu engine không tạo được clip thật, job dừng và báo lỗi thay vì giả chuyển động từ ảnh.
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

#### Video tham chiếu
Trang `/viral` có YouTube Viral Radar qua YouTube Data API. Radar chọn video mới trong 7 ngày, xếp theo lượt xem và chỉ giữ kênh từ 100,000 subscribers. Khi tạo bản gốc, hệ thống dùng tiêu đề, URL và duration làm metadata tham chiếu; không tải hoặc tái dùng footage/audio nguồn. Đầu ra phải dài ít nhất bằng nguồn.

#### Nguyên Tắc Phản Chiếu Video Viral Gốc (Original Viral Mirroring Law):
*Tùy vào video viral gốc như thế nào thì video được tạo bởi AI sẽ có hiệu ứng và phong cách tương tự 1:1:*
1. **Thời lượng chuẩn Quỹ Nhà Sáng Tạo (≥ 60s)**: Toàn bộ video viral xuất bản có thời lượng từ **60 giây trở lên** (chuẩn bật kiếm tiền TikTok Creativity Program).
2. **Khởi động tức thì tại 0.0s (Không màn hình đen)**: Video đầu ra bắt đầu bằng ảnh người mẫu đã tải lên; không chèn frame hay audio từ video nguồn.
3. **Hình tượng nhân vật "Thần tiên tỉ tỉ"**: Profile visual chân dung người con gái Việt Nam xinh đẹp thuần khiết 21 tuổi (làn da trắng sứ phát sáng, đôi mắt hạnh trong veo có hồn, suối tóc dài đen bồng bềnh nhẹ bay).
4. **Âm thanh độc lập**: Video đầu ra dùng nhạc nền cục bộ đã cấu hình. Không trích xuất hay lồng lại audio từ video nguồn.
5. **Phản chiếu chuyển động & visual mood**: Toàn bộ góc máy, chuyển động cơ thể, động tác nhảy và ánh sáng của clip triệu view được giữ nguyên vẹn qua công nghệ AI Face Swap, biến video thành tác phẩm độc quyền của Creator mà vẫn mang trọn vẹn năng lượng của video viral gốc.

---

### 4.3. Phân hệ Truyện Audio Điện Ảnh 30 – 60 Phút (YouTube Cinema)

```
             QUY TRÌNH DỰNG PHIM TRUYỆN AUDIO ĐIỆN ẢNH
  ┌────────────────────┐     ┌──────────────────────┐     ┌──────────────────────┐
  │ Kịch Bản Truyện Dài│ ──> │ Phân Đoạn 15s/Cảnh   │ ──> │ Sinh Visual 16:9 HD  │
  │ (30 - 60 Phút)     │     │ (LLM Scene Director) │     │ Flux AI / B-Roll Cine│
  └────────────────────┘     └──────────────────────┘     └──────────┬───────────┘
                                                                     │
  ┌────────────────────┐     ┌──────────────────────┐                │
  │ Master Video 16:9  │ <── │ Hòa Âm Không Gian    │ <──────────────┘
  │ Full HD + Phụ Đề   │     │ Gemini TTS + Foley   │
  └────────────────────┘     └──────────────────────┘
```

#### Quy chuẩn điện ảnh chống màn hình đen:
1. **Mật độ phân cảnh (Scene Density) & Thời lượng**:
   - Thời lượng mỗi tập truyện: **30 phút đến 60 phút** (chuẩn nghe radio đêm khuya, chữa lành và thư giãn).
   - Thời lượng mỗi cảnh: 8 đến 15 giây. Một tập truyện 30 phút chứa 120 - 180 phân cảnh thị giác liên tục; 60 phút chứa 240 - 360 phân cảnh, kết nối nhất quán với nhân vật và diễn biến truyện.
2. **Kịch bản chân thật, đa cảm xúc**:
   - Cốt truyện cuốn hút, lôi cuốn có đủ các cung bậc kịch tính: cao trào, trầm lặng, sôi nổi, hào hứng, tức giận, buồn bã, lặng lẽ.
   - Gắn tag cảm xúc ngữ cảnh: `[trầm lặng]`, `[cao trào]`, `[sôi nổi]`, `[tức giận]`, `[buồn bã]`, `[vui vẻ]`, `[thì thầm]` để chỉ đạo giọng đọc AI.
3. **Lồng tiếng Gemini TTS**:
   - Sử dụng độc quyền model **`gemini-3.8-flash-tts`** mới nhất của Google Gemini.
   - Trực tiếp chuyển hóa các thẻ cảm xúc thành ngữ điệu, nhịp thở và sắc thái biểu cảm chân thực.
4. **Cấu trúc âm thanh 3 lớp (Triple-Layer Spatial Soundscape)**:
   - **Lớp 1 (Narrative Lead)**: Giọng đọc truyền cảm tự nhiên của Gemini TTS.
   - **Lớp 2 (Ambient Foley)**: Tiếng môi trường thực tế (tiếng mưa dầm, tiếng gió đêm xào xạc, tiếng chuông xa) đặt ở mức âm lượng -14dB.
   - **Lớp 3 (Emotional Soundtrack)**: Nhạc đệm piano/lo-fi/orchestra cinematic nhẹ nhàng, tự động ducking -8dB khi có giọng đọc.

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

### 4.5. Động Cơ Tự Động Cập Nhật Model Gemini Mỗi Ngày (24H Daily Model Auto-Updater)

Nhằm đảm bảo hệ thống luôn tận dụng được những đột phá công nghệ mới nhất từ Google mà không cần sửa code hay can thiệp thủ công:

```
          TIẾN TRÌNH CẬP NHẬT MODEL GEMINI THEO YÊU CẦU (ON-DEMAND)
  ┌───────────────────────┐         ┌─────────────────────────┐
  │ Nút "Cập Nhật Model"  │ ──────> │ Quét Google API         │
  │ Web UI: POST /update  │         │ GET /v1beta/models      │
  └───────────────────────┘         └────────────┬────────────┘
                                                 │
  ┌───────────────────────┐         ┌────────────▼────────────┐
  │ Hot-Swap Tức Thì      │ <────── │ Thuật Toán So Sánh Bản  │
  │ Zero-Downtime         │         │ Mới Nhất (PickLatest)   │
  └──────────┬────────────┘         └─────────────────────────┘
             │
  ┌──────────▼────────────┐
  │ Lưu Cache Bền Vững    │
  │ discovered_models.json│
  └───────────────────────┘
```

1. **Cập Nhật Model Thủ Công Trên Toàn Bộ Adapter**:
   - Người dùng chủ động kích hoạt cập nhật bằng nút `⚡ Cập Nhật Tất Cả Model Gemini` trên giao diện web Cài đặt · Hệ thống (`POST /settings/models/update`). Không chạy tiến trình định kỳ 24h ngầm để tối ưu hóa tài nguyên.
2. **Thuật Toán Tuyển Chọn `PickLatest`**:
   - Phân giải phiên bản ngữ nghĩa (`major.minor` như 3.8, 3.9, 4.0).
   - Tuyển chọn theo độ ưu tiên: `Phiên bản mới hơn > Bản chính thức (GA) > Bản thử nghiệm (Preview/Exp) > Bản đầy đủ > Bản Lite`.
   - Phân loại tự động cho 4 nhóm tác vụ (adapter):
     * **Text**: `gemini-*-flash` (mặc định: `gemini-3.8-flash`).
     * **TTS**: `gemini-*-flash-tts` (mặc định: `gemini-3.8-flash-tts`).
     * **Image**: Dòng tạo ảnh chất lượng cao Nano Banana (mặc định: `gemini-nano-banana-2.1`).
     * **Video**: Dòng tạo video chuyển động Google Veo (mặc định: `veo-3.1-generate-preview`).
3. **Hot-Swap & Bộ Nhớ Cache Bền Vững**:
   - Áp dụng ngay cho các tiến trình render mà không cần restart máy chủ.
   - Tự động lưu vào `data/discovered_models.json` và nạp lại ngay khi máy chủ khởi động lại.

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
| **Truyện Audio 30-60p** | Cinematic Ambient Piano/Strings | Gemini / Edge | Mưa / Gió / Đêm phố cổ | Voice 0dB + Nhạc -18dB + Foley -14dB |

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
        "voiceover_engine": { "type": "string", "enum": ["gemini_tts", "edge_hoaimy_neural", "none"] },
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
| **4. Kỹ Thuật Dựng & Chuyển Cảnh** | 15% | Footage ổn định, dissolve hoặc hard cut tự nhiên; không flash, zoom giả, flicker hay rung lắc. | Đánh rớt nếu có flash, zoompan, flicker hoặc rung cơ học. |
| **5. Dòng Chảy & Giữ Chân Người Xem** | 10% | 3 giây đầu bắt mắt, 0 giây màn hình đen trong toàn bộ thời lượng video. | Đánh rớt nếu có khung hình đen chết. |

---

> *Bản quyền đặc tả kỹ thuật thuộc về ai-creator-os Project. Mọi thay đổi kiến trúc hoặc cập nhật mã nguồn phải bảo lưu đầy đủ các điều khoản trong Open-Spec này.*
