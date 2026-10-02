# RESEARCH.md — Kiến thức nền đã kiểm chứng của dự án

> Gộp ngày 2026-10-02 từ 6 file nghiên cứu rời trước đây + 2 vòng research
> avatar realtime (đã xoá thư mục gốc sau khi trích kết luận).
> Mỗi mục chỉ giữ **kết luận còn dùng được** kèm ngày + mức tin cậy. Muốn xem
> toàn văn chi tiết, tra lịch sử git của repo.
>
> Mức tin cậy: **[chính thức]** = nền tảng/tài liệu gốc công bố ·
> **[đo thật]** = có benchmark/số đo trên phần cứng nêu rõ · **[nguồn thứ
> cấp]** = blog/vendor, dùng thận trọng.

---

## 1. Chính sách live TikTok & giọng AI (đọc ngày 2026-10-01)

1. TikTok Shop **cấm giọng AI/audio thu sẵn trong live bán hàng**
   (promotional livestream/live commerce) — hiệu lực 5/2026, siết qua Account
   Health Rating từ 7/2026. Live giải trí thuần túy (không bật tính năng Shop)
   **không thấy văn bản cấm**; giọng AI trong live giải trí chỉ cần gắn nhãn
   AI như nội dung realistic. **[nguồn thứ cấp nhất quán: pymnts, ppc.land,
   techtimes]**
2. Live bán hàng: hình tĩnh/loop che >50% màn hình và figure không-phải-người
   che >50% màn hình đều vi phạm; avatar digital được phép nhưng <50% màn
   hình. Hễ ghim giỏ hàng/bật Shop trong live là vào diện cấm trên.
   **[nguồn thứ cấp]**
3. Không có API chính thức nào đọc chat/gift realtime của LIVE TikTok.
   Chỉ có client reverse-engineer không chính thức (TikTokLive, EulerStream).
   → Kiến trúc của ta **không** phụ thuộc chúng (luật: chỉ API chính thức).
   **[chính thức về sự vắng mặt + nhiều repo độc lập]**
4. Không có API chính thức để start/stop LIVE hay lấy RTMP key tự động; key
   lấy tay qua LIVE Center/app, FFmpeg đẩy RTMP là đường tuân thủ.
   **[chính thức]**

Hệ quả kiến trúc (đã chốt trong MODEL.md): live giải trí = gift, không bán
hàng trong live; affiliate đi qua video ngắn + showcase.

## 2. Việc được phép cho từng vertical (TikTok × YouTube, 10/2026)

Kết luận: **không vertical nào bị cấm ở mức nền tảng** (với điều kiện gắn nhãn
AI đầy đủ, không dùng tính năng Shop trong live). Thứ tự khuyến nghị triển
khai: kể chuyện → game → hát → nhảy → PK (khó automate nhất, affiliate thấp).

| Vertical | TikTok | YouTube |
|---|---|---|
| Hát nhạc AI tự sáng tác | Cho phép + nhãn AI | Cho phép + YPP nếu có nguyên bản + disclosure |
| Nhảy avatar AI | Cho phép + nhãn AI | Cho phép |
| Kể chuyện AI đọc | Cho phép + nhãn AI | Cho phép + YPP được (truyện tự sáng tác) |
| PK với host người | Cho phép, rủi ro trung bình | N/A |
| AI tự chơi game | Cho phép | Cho phép — gameplay được miễn disclosure |

Rủi ro thật không nằm ở nền tảng mà ở **ToS của từng game** (bot/anti-cheat)
— danh sách game an toàn xem MODEL.md §8b. YouTube cấm monetize: nội dung
template hàng loạt na ná nhau; **AI avatar đóng vai chuyên gia y tế/tài
chính/pháp lý**. **[chính thức + nguồn thứ cấp, 10/2026]**

## 3. Cách kiếm tiền TikTok ngoài bán hàng (10/2026)

1. **Quà tặng LIVE là trụ cột #1 cho live giải trí ở VN:** mở ở VN, không phân
   biệt AI/người, miễn live tuân thủ guidelines. TikTok giữ ~50% giá trị gift.
   Live cần 1.000 follower. **[đo độc lập/nguồn thứ cấp nhất quán]**
2. **Creator Rewards KHÔNG xác minh được ở VN** (danh sách nước công bố không
   có VN) → không đặt cược; chỉ tin khi thấy trong TikTok Studio thật.
   **[chính thức — danh sách công bố]**
3. SoundOn **chấp nhận nhạc AI tự sáng tác**, giữ 100% royalty trên nền tảng
   ByteDance; nhạc phải qua quét đạo nhạc ACRCloud trước khi phát hành. Nhưng
   distributor khác (LANDR) không cho nhạc AI đủ điều kiện Content ID →
   phát hành được ≠ hưởng đủ quyền Content ID. **[chính thức SoundOn]**
4. TikTok Series dùng được cho nội dung gốc ($0.99–189.99/series, điều kiện
   10K follower hoặc chứng minh đã bán premium); LIVE Subscription cần 10K
   follower + 100K view/tháng. Tình trạng mở ở VN **chưa verify**.
   **[chính thức + chưa verify VN]**

Các mục này là căn cứ cho thứ tự ưu tiên trong MODEL.md §4.

## 4. API TikTok Shop (đường chính thức — Ninh đã từ chối dùng, 2026-10-01)

Kết luận còn dùng: **toàn bộ API là miễn phí**; Affiliate Creator API có sẵn
endpoint săn sản phẩm open-collaboration (lọc theo hoa hồng) và đối soát đơn
hoa hồng; Content Posting API có 2 mode — Direct Post (đăng thẳng) và Upload
to Inbox (draft). Nhưng Direct Post phải qua **app review (~1–2 tuần) + Direct
Post audit (~5–10 ngày làm việc)**; chưa audit chỉ đăng được `SELF_ONLY` tối
đa 5 user. Giới hạn: ~15 bài/ngày/tài khoản qua Direct Post, 6 request/phút
theo token. **[chính thức + nguồn thứ cấp, 10/2026]**

Vì sao Ninh loại: Partner Center + app review + setup doanh nghiệp "rất phức
tạp". Đường thay thế đã chọn trong app: ADB quét Product Marketplace trên máy
Android của Ninh; đăng TikTok dạng draft. Mục này giữ lại chỉ để sau này khỏi
nghiên cứu lại: nếu đổi ý, chi phí thật là thời gian duyệt, không phải tiền.

## 5. TTS tiếng Việt + lựa chọn giọng đọc (10/2026)

1. Thứ hạng đề xuất: **Gemini TTS** (cảm xúc tốt nhất, style prompt được) →
   **VieNeu-TTS v3 local** (tiếng Việt native, emotion cues `[cười]`,
   `[thở dài]`, offline, không tốn quota) → Edge TTS (dự phòng cuối, endpoint
   không chính thức có thể bị chặn). Đây chính là chain app đang dùng.
   **[đánh giá từ tài liệu + repo upstream]**
2. VieNeu-TTS v3: RTF fp32 ~0.55–0.61, int8 ~0.35 (đo trên desktop 6 nhân,
   **không phải M1 Pro**); streaming cho audio đầu sau ~140–400ms. Model
   334MB. Số đo trên M1 Pro của Ninh **chưa có** — việc benchmark đang nợ
   (xem DEVELOPER.md §9). **[upstream]**
3. Piper vi_VN: robotic, chỉ nên làm fallback. FPT.AI/Vbee: free tier/quota
   không công bố đủ để dựa vào vận hành. **[chưa verify quota]**

## 6. Avatar realtime — kết luận cuối cùng (2 vòng research 2026-10-02)

**Verdict: KHÔNG có cách nào đạt avatar photoreal ≥20fps realtime chạy local
trên MacBook Pro M1 Pro 32GB.** Mọi ông lớn (HeyGen, D-ID, Tavus) đều render
bằng GPU server và phát WebRTC về máy khách — chất lượng của họ không phải
bằng chứng model chạy được trên laptop. Đây là lý do Ninh hạ ưu tiên live
avatar; kết luận này chỉ xem lại khi phần cứng/model đổi hẳn.

Số đo chính (gắn nhãn chip thật đã đo — không thay chip):

| Đo trên | Model | Kết quả | Mức |
|---|---|---|---|
| M1 Pro 32GB | LivePortrait | ~1 fps | [đo thật] |
| M2 Pro | MuseTalk 1.5 (PyTorch MPS) | ~1,6 fps | [đo thật] |
| M5 Max | MuseTalk-MLX | 22,4 fps @256px; 29,8 fps @128px | [đo thật, không phải M1 Pro] |
| M1 Pro (suy ra từ M5 Max) | MuseTalk-MLX | ước ~5–9 fps @256px, ~11–15 fps @128px; cộng thêm độ trễ windowing ~5,1s | [ước tính — chưa đo] |

Sự thật khác đã kiểm chứng:

1. Hội thoại Gemini Live API là **realtime giọng nói có tiếng Việt**, nhưng
   API công khai chỉ xuất audio — không xuất video avatar. "Live Avatar" của
   Google là Enterprise/allowlist, chưa có API công khai. Veo là async (vài
   phút/clip). **[chính thức, đọc 2026-10-02]**
2. Cloud photoreal rẻ nhất nếu sau này cần: Simli ~$0,009–0,01/phút
   (third-party, chưa đối chứng trang giá) và LiveAvatar LITE ~$0,09/phút
   (ước từ gói cước). Free quota đều ở mức demo (10 credits–50 phút/tháng).
   **[nguồn thứ cấp, đọc 2026-10-02]**
3. Não + tai + giọng realtime có thể $0: Groq free (Whisper nghe tiếng Việt;
   TTS của nó chỉ Anh/Ả Rập), NVIDIA NIM free có TTS Magpie tiếng Việt —
   nhưng credits NIM hữu hạn và trạng thái hosted Audio2Face còn tranh cãi.
   **[đo thật bởi bên thứ ba, 2026-09-22]**
4. **Kiến trúc local khả thi duy nhất nếu bắt buộc làm live local:** kiểu
   NVIDIA ACE — audio → blendshape ARKit-52 (model ONNX ~76MB) → render
   mesh 3D (VRM/ReadyPlayerMe/WebGL) trên M1. ≥25fps là khả thi về mặt
   tính toán, nhưng avatar **stylized, không photoreal**. Nỗ lực ước 2–4
   tuần. Chưa có benchmark M1 Pro nào cho route này. **[phân tích kỹ thuật]**
5. Default local hiện tại của app là **MuseTalk v1.5** (port
   `dunso/musetalk-mac`) chạy sidecar, render **offline** — chỉ sync môi,
   không tự sinh chuyển động đầu theo audio. Quality bar của Ninh (mọi frame
   nhân vật cử động như người thật, cấm slideshow) là tiêu chuẩn chấm, xem
   ARCHITECTURE.md §5.

---

*Cập nhật file này khi có kết quả benchmark M1 Pro thật hoặc khi chính sách
nền tảng đổi. Không chép số không nguồn/chưa gắn nhãn mức tin cậy vào đây.*
