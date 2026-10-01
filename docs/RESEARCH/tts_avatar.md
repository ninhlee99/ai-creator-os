Research complete. Here is my full report.

---

# BÁO CÁO NGHIÊN CỨU: Vietnamese TTS + Realtime Talking Avatar (10/2026)

## PHẦN 1 — Vietnamese TTS (ưu tiên miễn phí / free tier)

### Bảng so sánh

| Lựa chọn | Loại | Free tier | Streaming realtime | Chất lượng tiếng Việt (đánh giá trung thực) |
|---|---|---|---|---|
| **Edge TTS** (endpoint của Microsoft Edge) | API, không cần key | Miễn phí, không giới hạn công khai (không chính thức) | Có (websocket, chunked) | Khá tốt (giọng HoaiMy/NamMinh Neural). Nhưng ngữ điệu phẳng, ít cảm xúc — đủ cho đọc tin, yếu cho live giải trí |
| **Azure AI Speech F0** | API chính thức | **500K ký tự/tháng miễn phí vĩnh viễn** (giọng Neural) | Có, streaming PCM thật + **viseme** (hình miệng theo âm) | Tốt, có SSML điều khiển prosody (pitch/rate/pause). Viseme là điểm cộng lớn cho avatar |
| **Google Cloud TTS** | API chính thức | ~1M ký tự/tháng free (Standard/WaveNet); Chirp 3 HD có 0–1M free rồi $30/1M | Không streaming input tốt | WaveNet vi-VN khá; Chirp 3 HD mới (2025) có emotion markup `[pause]`, style tags — hứa hẹn nhưng đắt |
| **Gemini TTS API** (AI Studio) | API chính thức | Free tier (giới hạn rate, không công bố cứng) | Có (Live API streaming) | **Rất hứa hẹn cho giải trí**: prompt được style ("cheerful", `<laughs>`), 30+ giọng, hỗ trợ tiếng Việt. Gemini 3.8 TTS được báo là free đến 31/12/2026 |
| **VieNeu-TTS v3** | Local, open-source (Apache-2.0) | Miễn phí hoàn toàn, offline | Có (`infer_stream` theo chunk; bản Nano nhanh hơn) | **Tốt nhất trong các local**: train riêng cho tiếng Việt (10.000+ giờ), 23 preset Bắc/Trung/Nam, emotion cues `[cười]` `[thở dài]`, voice cloning từ 3–5s audio, 48kHz. RTF ~0.37–0.62 trên CPU (int8) — gần realtime |
| **Piper vi_VN** | Local, open-source | Miễn phí | Theo câu (không native stream) | Công nghệ VITS đời cũ — nghe robotic, ngữ điệu đơn điệu. Chỉ nên làm fallback |
| **FPT.AI** | API VN | Có free tier "đánh giá" (quota **không công bố**) | Không rõ | Uy tín tốt ở thị trường VN, có giọng 3 miền. Phải đăng ký console mới biết quota |
| **Vbee** | API VN | Free trial; trả phí từ $5.99/tháng (125K ký tự) | Không rõ | Giọng Gen 2 được đánh giá tốt cho tiếng Việt, có voice cloning. Không có free tier dùng lâu dài |
| **Viettel AI** | API VN | 60 phút STT free (TTS không rõ) | Không rõ | Không đủ dữ liệu đánh giá TTS |

### Đánh giá trung thực về "giọng tự nhiên có nhịp điệu, trầm bổng"
- Không có option miễn phí nào cho ra giọng giải trí "có hồn" ngay từ hộp. Muốn trầm bổng phải dùng **style prompting** (Gemini), **emotion cues** (VieNeu `[cười]`/`[thở dài]`), hoặc **SSML prosody** (Azure/Google).
- Thứ hạng mình đề xuất cho live giải trí: **(1) Gemini TTS free tier** (thử trước, expressive nhất) → **(2) VieNeu-TTS local** (fallback offline, không tốn quota, giọng Việt native) → **(3) Azure F0** (ổn định, có viseme cho avatar) → **(4) Edge TTS** (dự phòng không key).
- Chiến lược khôn ngoan: **dùng local (VieNeu) làm mặc định** để không bao giờ cạn quota giữa live, **API (Gemini/Azure) cho các đoạn cần cảm xúc cao**.

## PHẦN 2 — Realtime talking avatar

### (a) Paid streaming-avatar APIs

| Dịch vụ | Giá realtime | Latency | Chất lượng | Ghi chú |
|---|---|---|---|---|
| **HeyGen LiveAvatar** | Free: 10 credits/tháng; Starter $19/tháng (200 credits); ~$0.10/phút (LITE 1 credit/phút) | <300ms time-to-first-frame (công bố) | Best-in-class (Avatar IV), 1080p | Có "Avatar Only mode" — mang TTS của mình vào, chỉ trả tiền render avatar |
| **Beyond Presence** | Free 20 phút/tháng; từ $49/tháng (~$0.09–0.35/phút) | Realtime | Hyper-realistic | Có avatar mặc định, không cần setup |
| **D-ID** (streaming agents) | Trial 3 phút free; Lite ~$4.7–5.9/tháng (10 phút); API Build từ $18/tháng | Realtime | Lip-sync tốt, realism trung bình | Rẻ nhất để thử; phàn nàn về minh bạch giá |
| **Synthesia** | Từ $64/tháng (Creator) | Không realtime streaming (chủ yếu batch) | Rất tốt | Không phù hợp live |
| **Azure AI Avatar** | $0.50/phút (interactive) | Realtime | Tốt, gắn với Azure TTS | Đắt cho live dài |
| **AvatarTalk / TruGen** | $0.05–0.10/phút; TruGen từ $28/tháng (có free plan) | Realtime | Khá | Ít tên tuổi hơn |

### (b) Free / open-source trên M1 Pro 32GB

| Phương án | Realtime? | Chất lượng | Chạy được trên M1 Pro? |
|---|---|---|---|
| **MuseTalk** (lip-sync SOTA) | Có — nhưng **chỉ trên NVIDIA GPU** (30 FPS @256px trên V100; trên CPU chậm hơn 30–50×) | Rất tốt | ❌ Không realtime trên M1 (MPS/CPU quá chậm). Không có số liệu M1 được xác minh |
| **SadTalker** | ❌ Không (render offline) | Tốt (head motion + lip-sync) | ⚠️ Chạy được trên CPU Mac nhưng **~50× chậm hơn realtime** (clip 3.4s mất ~3 phút render). Dùng được cho **pre-render** đoạn ngắn, không live |
| **Wav2Lip** | ❌ (batch) | Lip-sync tốt, không đổi expression | ⚠️ Nhanh hơn SadTalker 5–10×, vẫn không realtime trên CPU. Pre-render được |
| **Hallo / Hallo2 / EchoMimic** | ❌ (diffusion, vài phút/clip ngay cả trên NVIDIA) | Rất đẹp | ❌ Không khả thi |
| **LongCat-Video-Avatar 1.5** | ❌ (~90s compute cho 1s video trên card 16GB) | Rất đẹp | ❌ |
| **VTube Studio + Live2D** (mic-based lip sync) | ✅ **Realtime** | Stylized (anime), **không photorealistic**; miệng mở theo âm lượng/nguyên âm — gần đúng, không chính xác từng phoneme | ✅ Chạy tốt trên M1. Có virtual camera → FFmpeg → RTMP. Đây là đường VTuber chuẩn |
| **PNG-tuber** (2 ảnh miệng đóng/mở) | ✅ Realtime | Rất đơn giản | ✅ Nhẹ nhất, làm được trong vài ngày |
| **3D VRM avatar + audio-driven blendshapes** (52 ARKit blendshapes @60fps) | ✅ Realtime | Stylized 3D | ✅ Có project open-source (`react-ai-voice-avatar`) chứng minh 60fps trên GPU client |
| **Azure viseme + SVG overlay** | ✅ Realtime | Cartoon | ✅ Nhưng cần Azure TTS |

### VERDICT trung thực
**Photorealistic + realtime + frame-coherent lip-sync + miễn phí trên M1 Pro 32GB = KHÔNG khả thi ở thời điểm hiện tại.** Các model diffusion/lip-sync SOTA (MuseTalk, Hallo, EchoMimic, LongCat) đều đòi NVIDIA GPU để realtime; trên CPU/MPS của M1 chúng chậm hơn realtime hàng chục lần. Đây là giới hạn vật lý của phần cứng, không phải vấn đề tối ưu code.

**Hai con đường thực tế:**
1. **Trả phí (duy nhất cho photorealistic realtime):** HeyGen LiveAvatar Avatar-Only mode ~$0.10/phút + TTS của mình (VieNeu local = $0). Live 2 tiếng/ngày ≈ $12/ngày ≈ $360/tháng. Beyond Presence/D-ID rẻ hơn để thử.
2. **Miễn phí (khuyến nghị):** avatar **stylized** realtime — VTube Studio + Live2D model (hoặc VRM 3D) với mic-based lip sync, ăn audio từ TTS, xuất qua virtual camera → FFmpeg → RTMP lên TikTok. Kết hợp **pre-render SadTalker/Wav2Lip** cho các đoạn product showcase không cần tương tác. Đánh đổi: không phải người thật, nhưng cộng đồng VTuber đã chứng minh khán giả chấp nhận và yêu thích format này — phù hợp với định vị "live giải trí".

## Kiến trúc đề xuất (free-first, theo yêu cầu của user)
- **TTS:** VieNeu-TTS v3 Turbo local làm mặc định (offline, không quota, giọng Việt native, emotion cues) → Gemini TTS free tier cho đoạn cần cảm xúc → Azure F0 dự phòng (có viseme).
- **Avatar live:** VTube Studio + Live2D/VRM, lip-sync theo audio TTS, virtual camera → FFmpeg RTMP.
- **Avatar pre-render:** SadTalker/Wav2Lip trên CPU cho clip giới thiệu sản phẩm, phát xen kẽ trong live.
- **Nâng cấp khi có doanh thu:** HeyGen LiveAvatar Avatar-Only ($0.10/phút) để có photorealistic realtime.

## NHỮNG GÌ KHÔNG XÁC MINH ĐƯỢC (cần test thực tế)
1. **Quota free chính xác của FPT.AI TTS** — không công bố công khai, phải đăng ký console.fpt.ai mới biết.
2. **Giá/quota TTS của Viettel AI** — chỉ tìm được free tier STT (60 phút), không có số liệu TTS.
3. **Zalo TTS API công khai** — không tìm thấy; coi như không tồn tại.
4. **Hiệu năng MuseTalk/Wav2Lip cụ thể trên M1 Pro** — không có benchmark công khai; kết luận "không realtime" dựa trên số liệu V100→CPU (chậm 30–50×) và SadTalker trên Mac CPU (~50× realtime).
5. **Rate limit thực tế của Edge TTS và Gemini TTS free tier** — Edge là endpoint không chính thức (xám về ToS, có thể bị chặn khi spam); Gemini free tier có giới hạn request/phút không công bố — cả hai đều rủi ro cho live 24/7, vì vậy mới đề xuất local làm mặc định.
6. **Chất lượng giọng thực tế** — mọi đánh giá "tự nhiên" ở trên đều từ tài liệu và báo cáo cộng đồng; cần nghe thử trực tiếp (đặc biệt VieNeu vs Gemini cho tiếng Việt giải trí).
7. **Chính sách TikTok về AI streamer** — ngoài phạm vi task này, cần research riêng.

## Nguồn chính
- VieNeu-TTS: https://github.com/quanghuybest2k2/vieneu-tts ; VietTS fork: https://github.com/ngoc2012/vietts/blob/HEAD/README.md
- Edge TTS research: https://github.com/vanson0105/galaxyvoice/blob/HEAD/tools/galaxy_ai_voice_subtitle_studio/docs/voice-engine-research.md ; VTube mic lip-sync: https://github.com/DenchiSoft/VTubeStudio/wiki/Lipsync
- Google TTS pricing/free tier: https://github.com/adrian333dev/flow/blob/HEAD/lab/research/tts.md ; Chirp 3: https://github.com/qishuilalala/dsh-voice-mode/blob/HEAD/docs/competitive/sources/competitive-tts-platforms-2026-05.md
- Azure F0 free tier: https://github.com/xujialiu/zotero-tts/blob/HEAD/tutorials/azure-speech-free-tier.md ; Azure pricing: https://azure.microsoft.com/fr-ca/pricing/details/speech/
- Gemini TTS: https://github.com/hashimmalikdev/gemini-realtime-tts/blob/HEAD/README.md ; Gemini 3.8 TTS: https://mer.vin/news/gemini-3-8-tts-2000-voices-and-native-two-speaker-dialogue/
- FPT.AI API: https://github.com/budecosystem/waav/blob/HEAD/gateway/docs/providers/fpt_ai.md ; Viettel: https://github.com/budecosystem/waav/blob/HEAD/gateway/docs/providers/viettel_ai.md
- Vbee pricing: https://www.capterra.com/p/10034055/Vbee-AIVoice/
- Piper vi_VN: https://github.com/letuhao/local-tts-service ; https://github.com/agisota/piper-TTS
- HeyGen LiveAvatar pricing: https://www.liveavatar.com/ ; https://www.therundown.ai/tools/heygen-labs-interactive-avatar ; provider comparison: https://github.com/dlasley/sally-schoolwork/blob/HEAD/docs/AVATAR_PROVIDERS.md
- Avatar API market table: https://github.com/juspay/director/blob/HEAD/video-production/library/docs/AVATAR-ANIMATION-RESEARCH.md
- D-ID pricing: https://heyfish.ai/d-id-review ; https://www.fahimai.com/how-to-use-d-id
- MuseTalk realtime analysis: https://github.com/PunithVT/ai-avatar-system ; warm-server: https://github.com/herehere14/musetalk
- SadTalker on Mac CPU (~50× realtime): https://github.com/paul30041004/openmontage/blob/HEAD/.agents/skills/sadtalker-avatar/SKILL.md
- LongCat (90s/1s video): https://github.com/brndngln/talker/blob/HEAD/README.md
- react-ai-voice-avatar (60fps blendshapes): https://github.com/927tanmay/react-ai-voice-avatar/blob/HEAD/README.md