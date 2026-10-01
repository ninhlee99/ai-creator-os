# Avatar pipeline research — 2026-10-01

Kết luận và giới hạn trung thực cho pipeline avatar của AI Creator OS.
Máy tham chiếu: Mac M1 Pro, RAM 32 GB.

## Quyết định

**Default local = MuseTalk v1.5** qua community port `dunso/musetalk-mac`
(MPS + FastAPI sẵn: `POST /`, `/warmup`, `/lipsync_stream`, `/speak`,
`GET /health`). Chạy dưới dạng sidecar do binary Go quản lý (start/stop/
health check), giống VieNeu-TTS và llama-server.

## So sánh các lựa chọn đã đánh giá

| Model | Audio-driven? | Realtime trên M1 Pro? | Kết luận |
|---|---|---|---|
| **MuseTalk v1.5** (musetalk-mac port) | Có (lip-sync theo audio) | Không (~2.5–4 fps ước tính → render offline ~6–10 phút cho clip 60s) | **Default local.** Chỉ sync môi, không tự sinh head motion theo audio. |
| **LivePortrait** | Không — image/video-driven | Có thể realtime-ish | Chỉ hợp làm **idle-motion loop** (nhân vật "thở", chớp mắt khi không nói). Không thay thế lip-sync. |
| **EchoMimicV2** | Có | CUDA-only | **Loại** — không chạy trên Apple Silicon. |
| **SadTalker** | Có | Không (CPU rất chậm) | **Loại** cho pipeline chính. |
| **HeyGen / D-ID** (paid API) | Có | Realtime (cloud) | Tier trả phí, **tắt mặc định**. Bật khi cần chất lượng cao. |

## Giới hạn trung thực (không overclaim)

1. **Không có model nào realtime được trên M1 Pro.** Mọi số fps ở trên
   đều là ước tính ngoại suy — cần **benchmark thật trên máy Ninh**
   trước khi đóng contract sidecar cuối cùng.
2. MuseTalk chỉ sync môi; head motion / biểu cảm theo audio không có sẵn.
   Pipeline dùng emotion cues (`[cười]`, `[thở dài]`, …) từ TTS để điều
   khiển expression theo đoạn, không phải từng frame.
3. Muốn "không nhận ra là AI" (Ditto / EchoMimic chất lượng cao) cần
   NVIDIA 16GB+ VRAM — tức cloud GPU, không phải máy Ninh.
4. **Cam kết chất lượng (yêu cầu của Ninh):** mọi khung hình phải được
   render theo audio từng frame — **cấm** kiểu "ảnh tĩnh + Ken Burns +
   hiệu ứng chuyển cảnh". Pipeline từ chối render nếu sidecar không chạy
   thay vì trả về clip nửa vời.

## Kiến trúc đã triển khai

- `internal/engines/avatar`: provider chain `local → heygen → did`
  (paid tiers tắt mặc định, cần API key).
- Sidecar contract v1 (HTTP): `GET /health`, `POST /render`,
  `POST /stream/open|push|close`, `GET /stream/frame`.
- Identity lock: `sha256(ảnh tham chiếu + seed)` — render bị từ chối nếu
  lock không khớp ảnh trên đĩa (chống face drift âm thầm).
- Emotion cues: 17 tag tiếng Việt (`[cười]`, `[thở dài]`, `[hắng giọng]`,
  …) được parse từ script TTS và map sang biểu cảm.
- Dashboard: quản lý nhân vật (upload ảnh, preview, xóa), render thử
  (TTS → emotion → avatar → preview mp4), bật/tắt tier, sidecar panel
  (tải model, restart, trạng thái).
