# Audit lõi AI cho MacBook Pro M1 32 GB — 2026-10-02

**Cách audit:** đọc code + cấu hình thật trong repo (không suy đoán từ mô tả).
**Giới hạn trung thực:** không có máy Mac của Ninh ở đây (Mac chưa pair với VM này)
→ **mọi con số hiệu năng dưới đây là ƯỚC TÍNH** (từ tài liệu upstream + ngoại suy),
không phải số đo. Muốn chốt phải benchmark thật trên máy Ninh (mục 10).
Ưu tiên theo yêu cầu của Ninh: **tốc độ → độ chính xác → realtime**.

## Bảng verdict nhanh

| Thành phần | Verdict | Một dòng |
|---|---|---|
| TTS Gemini (mặc định) | **Tốt** | Cloud, nhanh, giọng Việt tốt nhất trong các lựa chọn hiện có |
| TTS VieNeu-TTS v3 local | **Chạy được, khá nhanh cho CPU** | ONNX/CPU; chế độ int8 nhanh hơn ~1.6–2× chưa được bật |
| TTS Edge | **Chạy được** (dự phòng cuối) | Endpoint không chính thức, có thể bị chặn bất cứ lúc nào |
| LLM Gemini | **Tốt** | Cloud, realtime cho director/caption |
| LLM llama-server local | **Vừa sửa lỗi lớn** | Trước đây chạy CPU 100% dù máy có GPU (thiếu `-ngl`) |
| Avatar local (MuseTalk sidecar) | **Rủi ro hỏng / chưa dùng được** | Sidecar chưa tồn tại, contract Go ≠ API sidecar đã chọn, và **không realtime** |
| Avatar paid (HeyGen/D-ID) | **Chưa nối dây** | Khung rỗng, tắt mặc định; bật lên cũng chưa chạy |
| Media-gen ảnh (Gemini) | **Tốt** | Cloud, xoay key khi hết quota |
| Veo 3 | **Tốt khi key có billing** | Cloud; không có billing tự rơi về list ảnh |
| Upscale 4K (lanczos) | **Chạy được** | 1 frame/ảnh; là phóng to + làm nét, không phải chi tiết AI |
| FFmpeg dựng video | **Vừa sửa** | Ưu tiên encoder phần cứng M1 (VideoToolbox), fallback libx264 |
| Đường LIVE hiện tại | **Chưa phải live thật** | Engine v1 phát hình test + tiếng sine; avatar chưa nối vào |

## 1. TTS (chuỗi: Gemini → VieNeu → Edge, chỉnh trong Settings)

- **Gemini TTS** (`gemini-2.5-flash-preview-tts`): gửi trọn câu trong 1 request,
  timeout 60s, xoay nhiều key (cooldown 60s→5m→15m khi 429). Ước tính vài giây
  cho đoạn thoại 3–5 câu. Là tier mặc định — đúng lựa chọn cho chất lượng.
- **VieNeu-TTS v3** (sidecar Python, Go khởi chạy bằng
  `uv run python -m apps.openai_speech`, port 8000):
  - Trên Mac không có CUDA nên upstream tự chọn **ONNX/CPU**
    (`VIENEU_BACKEND=auto`); không có đường MPS/CoreML trong upstream.
    Go không truyền env nào → dùng mặc định upstream: **fp32, tối đa 1 stream
    đồng thời**.
  - Số upstream tự đo (desktop 6 nhân, *không phải M1*): RTF fp32 ~0.55–0.61,
    int8 ~0.35; audio đầu tiên sau ~140–400 ms khi dùng streaming. Trên M1
    ước tính tương đương hoặc nhỉnh hơn — chưa đo thật.
  - **Điểm trừ:** Go gọi POST thường và chờ trọn WAV, không dùng streaming của
    upstream. Câu thoại 5 giây → ước tính chờ ~2–3s (fp32) mới có audio.
    Dựng video sẵn: chấp nhận được. Live: cộng thêm độ trễ từng segment.
  - Model 334 MB, RAM ~300–500 MB. Nhẹ.
  - **Rủi ro:** repo upstream được clone từ nhánh `main`, không ghim phiên bản
    → upstream đổi kiến trúc (đã xảy ra vài lần trong 2026) là có thể vỡ.
- **Edge TTS:** Go thuần (WebSocket tự viết), không phụ thuộc Python/Node —
  chạy mọi nền tảng. Endpoint không chính thức (token cứng trong code),
  Microsoft chặn lúc nào không hay. Đúng vị trí dự phòng cuối.

## 2. LLM (chuỗi: Gemini → llama-server → paid tắt)

- **Gemini** (`gemini-2.0-flash`): mặc định, nhanh, realtime. Tốt.
- **llama-server local:** model mặc định Qwen2.5-7B-Instruct Q4_K_M
  (file 4.7 GB; RAM ước tính ~5.5–6.5 GB kèm KV cache ở ctx 4096). Cỡ model
  hợp lý cho 32 GB, còn dư nhiều cho các sidecar khác.
  - **Lỗi đã sửa:** lệnh khởi chạy thiếu `-ngl` → llama.cpp mặc định offload
    0 layer → **chạy CPU 100% dù là bản Metal**; comment cũ trong code còn ghi
    sai "Metal tự tăng tốc". Ước tính tốc độ sinh chữ trên M1: CPU ~8–15
    tok/s → Metal ~25–45 tok/s (chưa đo thật). Director viết shot list/caption
    qua tầng local chậm đi vài lần là vì lỗi này.
  - Điều kiện: phải cài llama.cpp bản có Metal (`brew install llama.cpp`).
    USER_GUIDE hiện chỉ nhắc `brew install ffmpeg` — chưa cài llama-server
    (và `uv` cho VieNeu) thì tầng local chết lặng, chỉ ghi log.
  - ctx 4096 đủ cho prompt hiện tại; nâng 8192 tốn thêm ~0.3–0.5 GB RAM.

## 3. Avatar — và câu trả lời REALTIME

Nói thẳng: **avatar người thật realtime trên M1 local: KHÔNG — cả về vật lý
lẫn tình trạng code hiện tại.**

**a) Vật lý.** MuseTalk trên M1 ước tính ~2.5–4 fps (theo
`docs/RESEARCH/avatar_pipeline.md` của chính dự án), trong khi live cần
25 fps. Clip 60s ≈ 6–10 phút render offline. Đây là giới hạn model/phần cứng,
không phải lỗi tối ưu code.

**b) Code.** Tier local hiện **chưa chạy được end-to-end**:

- Go kỳ vọng một binary tên `avatar-server` trong
  `data/third_party/avatar-sidecar`, nhận flags `--host/--port/--model-dir`
  và nói contract `/render`, `/stream/*`. Binary này **không tồn tại** trong
  repo — là phần phải tự cài trên Mac, và chưa có tích hợp nào được kiểm chứng.
- Sidecar đã chọn trong RESEARCH (musetalk-mac, FastAPI) có API khác hẳn
  (`POST /`, `/warmup`, `/lipsync_stream`, `/speak`) → **không nói chuyện
  được với Go client**. Cần một adapter mỏng phía Mac (hoặc sửa Go client),
  chỉ kiểm chứng được trên Mac thật.
- HeyGen/D-ID mới là khung rỗng (`paid.go` tự ghi "wiring pending"), mọi call
  trả lỗi. Tắt mặc định là đúng chủ trương, nhưng bật lên cũng chưa chạy.
- **Đã sửa một bug thật:** HTTP client dùng chung timeout 60s trong khi render
  offline mất nhiều phút và chain cho phép 30 phút → mọi render thật đều chết
  ở giây 60. Nay `/render` có client riêng 30 phút.

**c) Đường live hiện tại.** `internal/stream/engine.go` là v1: ffmpeg phát
**hình test (testsrc 1280×720) + tiếng sine 440 Hz** lên RTMP. Segment
audio/avatar render xong chỉ được xếp hàng trong bộ nhớ, không ai đọc (code
tự ghi chú trung thực điều này). Streamer agent (`RunLive`) chưa có nơi nào
gọi trong binary; interface `RenderSegment` chưa có bản cài đặt thật.
`SupportsRealtime() = true` chỉ nghĩa là "contract cho phép mở stream",
**không** phải "đạt tốc độ realtime trên M1" — tài liệu trong code ghi rõ,
nhưng nhãn hiển thị trên dashboard có thể gây hiểu nhầm.

**Muốn live người thật realtime thật sự** (theo RESEARCH của dự án): HeyGen
LiveAvatar (cloud, API chính thức, ~$0.10/phút) — cần nối wiring cho paid
tier. Hoặc chấp nhận avatar stylized. Không có đường local miễn phí nào đạt.

## 4. Media-gen (cloud — không phụ thuộc máy Mac)

- **Ảnh:** `gemini-2.0-flash-preview-image-generation`, timeout 130s, xoay
  key khi 429/invalid. Ước tính 5–20s/ảnh tùy tải Google. Identity/product
  khoá bằng ảnh reference — thiết kế đúng ý Ninh.
- **Veo 3:** `predictLongRunning`, poll mỗi 12s, hạn 15 phút, tải file tối đa
  10 phút. Cần key có bật billing Google Cloud; không có thì pipeline tự rơi
  về chế độ list ảnh (đúng thiết kế, không gãy).

## 5. FFmpeg + upscale

- **Trước:** mọi encode là libx264 (CPU) preset medium CRF 18 ở 1080×1920.
  Phần nặng nhất thực ra là filter zoompan ở supersample 2160×3840 (CPU),
  encoder cộng thêm.
- **Đã sửa:** probe một lần lúc chạy — nếu ffmpeg có `h264_videotoolbox`
  (bản Homebrew trên Mac có sẵn) thì dùng **encoder phần cứng của M1**
  (12 Mbps), không có thì libx264 như cũ. Ước tính phần encode nhanh hơn vài
  lần và nhả CPU cho filter; tổng thời gian dựng video 30s giảm rõ nhưng
  không thần kỳ vì filter vẫn CPU-bound. Máy không có VideoToolbox (như máy
  build Linux này) tự fallback — test vẫn xanh.
- **Upscale 4K:** lanczos + unsharp, 1 frame/ảnh — ước tính 1–5s/ảnh ở 4K.
  Ghi nhận trung thực (code đã tự ghi): đây là phóng to thuật toán + làm nét,
  không phải chi tiết AI thật.
- `engines/film.go` (đường phim dự phòng) vẫn libx264 veryfast — chấp nhận
  được, xem khuyến nghị 6.

## 6. Đồng thời & RAM (ước tính, máy 32 GB)

Đỉnh đồng thời xấu nhất: macOS ~4–5 GB + llama-server ~6 GB + VieNeu
~0.5 GB + avatar sidecar ~2–4 GB + 2 job ffmpeg ~1–2 GB + aicos ~0.2 GB
≈ **14–18 GB / 32 GB** → vừa, còn dư.

- **Đã sửa:** trước đây studio job **không giới hạn** số job chạy song song
  (autopilot bắn 1 job/tài khoản, mỗi job 1 goroutine) → nhiều ffmpeg cùng
  lúc tranh CPU + ăn RAM, chậm toàn cục. Nay tối đa **2 job render đồng
  thời**; job thừa giữ trạng thái "queued" đúng sự thật.
- VieNeu CPU chỉ phục vụ 1 stream fp32 cùng lúc (int8: 2) — live và studio
  cùng gọi TTS local sẽ phải xếp hàng; vì Gemini là tier 1 nên thực tế ít đụng.

## 7. Rủi ro nền tảng khác

- Không có giả định CUDA/x86 trong code Go; không đường dẫn Linux hardcoded
  trong phạm vi AI. Phần này sạch.
- Phụ thuộc phải cài trên Mac nhưng guide chưa liệt kê đủ: ffmpeg (đã có),
  llama-server (`brew install llama.cpp`), `uv` (cho VieNeu). Thiếu cái nào,
  tầng đó tắt lặng.
- VieNeu clone không ghim version (mục 1).
- Flags của avatar sidecar (`--host/--port/--model-dir`) là contract tự đặt;
  sidecar thật phải theo đúng — sai là không start được, và chỉ phát hiện
  được khi cài trên Mac.

## 8. Fix đã áp dụng trong đợt này (chưa commit — chờ duyệt)

1. `internal/engines/chains.go` (+ comment trong `internal/engines/doc.go`):
   thêm `-ngl 99` cho llama-server — bật GPU Metal trên M1.
2. `internal/engines/avatar/local.go`: `/render` dùng HTTP client riêng
   timeout 30 phút (trước: chết ở 60s).
3. `internal/studio/assemble.go`: ưu tiên `h264_videotoolbox`, fallback
   libx264 (`AssemblePhotoList`, `AssembleBeatBounce`, `ConcatClips`).
4. `internal/studio/studio.go`: semaphore tối đa 2 job render đồng thời
   (affiliate + film).

Kiểm chứng: `go build ./...` OK; `go test ./...` **20/20 package pass**
(baseline trước khi sửa cũng 20/20 — không fix nào làm vỡ test).

## 9. Khuyến nghị xếp theo tác động (cần Mac hoặc Ninh quyết)

1. **[Lớn nhất — realtime]** Chốt hướng live: nối HeyGen LiveAvatar cho paid
   tier (realtime thật, có phí, API chính thức), hoặc hạ kỳ vọng live local
   xuống "pre-render đoạn ngắn, phát xen kẽ". Đừng để nhãn "hỗ trợ realtime"
   trên dashboard đánh lừa quyết định này.
2. **[Nhanh]** Bật `VIENEU_PRECISION=int8` cho sidecar VieNeu (1 dòng env
   trong launcher): upstream đo nhanh ~1.6–2× trên CPU; đổi lại chất giọng
   giảm nhẹ → nghe A/B trên Mac rồi chốt. Cân nhắc thêm: dùng streaming của
   upstream để audio đầu tiên tới sau ~150–400 ms thay vì chờ trọn câu.
3. **[Nhanh]** Ghim version VieNeu (tag/commit) thay vì clone `main`.
4. **[Nhanh]** Bổ sung USER_GUIDE: `brew install llama.cpp` + cài `uv`.
5. **[Trung bình]** Viết adapter musetalk-mac ↔ contract Go (hoặc ngược lại)
   và đo fps MuseTalk thật trên Mac Ninh — con số đó quyết định avatar local
   đáng giữ hay không.
6. **[Nhỏ]** `film.go` + stream engine dùng chung lựa chọn encoder như
   `assemble.go`; cân nhắc `-threads` cho ffmpeg khi 2 job chạy song song.
7. **[Nhỏ]** Cân nhắc Qwen2.5-14B Q4_K_M (~9 GB) cho LLM local nếu muốn
   director viết tốt hơn — vẫn vừa 32 GB, đổi lại chậm hơn bản 7B.

## 10. Cần Ninh làm trên Mac (không làm thay được từ VM này)

1. `brew install llama.cpp` (bản Metal) → xem log llama-server có nhận GPU.
2. Đo thử: thời gian director viết 1 shot list qua llama local; RTF VieNeu
   với 1 câu mẫu; (sau khi có adapter) fps MuseTalk thật.
3. Nghe A/B VieNeu fp32 vs int8 để chốt khuyến nghị 2.
