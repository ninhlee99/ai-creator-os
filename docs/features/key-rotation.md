# Quy tắc dùng chung: key rotation & fail-closed

## 1. Mục đích
Ghi một chỗ, dùng chung mọi tính năng: app xoay vòng key cloud thế nào, và
nguyên tắc "không có dữ liệu thật thì không bịa" áp dụng ở đâu.

> **Luật cứng (Ninh chốt 2026-10-02): chỉ dùng key Gemini FREE, không key trả
> phí, không billing.** Veo không bao giờ khả dụng với key free (đòi Google
> Cloud project bật billing) — app không thử, không tốn tiền.

## 2. Key rotation — `GEMINI_API_KEYS`

Nguồn key: env `GEMINI_API_KEYS` (phân tách dấu phẩy) → `web.Config.GeminiAPIKeys`
(`internal/web/config.go`); keyring trong DB cho từng engine cũng được
(quản lý ở Cài đặt · Nhà cung cấp — key hiện `••••abcd`, không render raw).

Ba keyring độc lập (mỗi engine một vòng, không dùng chung vòng):

| Engine | File keyring | Chính sách |
|---|---|---|
| Media (ảnh) | `internal/studio/mediagen.go` (`GeminiMediaGen`) | round-robin; bỏ key invalid; cooldown khi 429/quota theo thang **60s → 5m → 15m** (`keyBackoffs`); key khoẻ lại → reset thang |
| TTS (giọng Gemini) | `internal/engines/tts/keyring.go` (`KeyRing`) | round-robin; `ReportRateLimit` → cooldown thang 60s→5m→15m; `ReportSuccess` → reset; `AllCoolingDown()` true → **failover sang tier tiếp theo**, không retry cào thêm |
| LLM | `internal/engines/llm` (qua `LLMChain`) | cùng chính sách keyring như TTS |

> 🅿️ Đường Veo trong media keyring đã park theo pipeline phim.

### Chuỗi provider (failover theo tier)
`internal/engines/chains.go`:
- **LLM**: Gemini (free API) → llama-server local (`127.0.0.1:8081`, model
  `qwen2.5-7b-instruct-q4_k_m.gguf`, `-ngl 99`) → paid (placeholder, tắt).
- **TTS**: Gemini → VieNeu-TTS v3 local (sidecar `127.0.0.1:8000`, Go quản lý
  tiến trình) → Edge (phương án cuối).
- ~~Avatar~~: chuỗi Avatar đã park theo live (Đợt A).
- Thứ tự chuỗi chỉnh được ở Cài đặt · Nhà cung cấp (`ChainConfig`).

### Ràng buộc free tier
- Quota free tính **riêng từng key** (per project per model) nên càng nhiều key trong
  `GEMINI_API_KEYS` càng đỡ nghẽn: keyring xoay round-robin, key nào 429/quota thì
  cooldown theo thang 60s → 5m → 15m, key khác gánh tiếp.
- Số liệu tham khảo (nguồn bên thứ ba, chưa kiểm chứng, Google thay đổi thường xuyên):
  image gen free khoảng ~10 request/phút và ~50 ảnh/ngày/key; text gen khoảng
  15 request/phút, ~1500 request/ngày/key. Hết quota ngày giữa chừng: job chờ
  cooldown, hôm sau bấm **Chạy tiếp** (resume không render lại phần đã xong).
- TTS/LLM có tier local dự phòng (VieNeu, llama-server) nên app vẫn chạy được
  khi key free hết quota — chỉ phần vẽ ảnh là bắt buộc chờ key.

## 3. Fail-closed toàn app
- **Tiền**: `ReconcileCommissions` — không nguồn → giữ 0 + decision `no_source`;
  đơn idempotent theo `external_id` (không ghi trùng).
- **Số liệu**: nguồn metrics lỗi → giữ snapshot cũ + note "chưa kết nối", không bịa.
- **Sản xuất**: `mg.Healthy()` false → job failed "media-gen chưa sẵn sàng";
  không backend → item giữ `planned` + alert.
- **Đăng**: thiếu OAuth/quota → `waiting_connect`/`waiting_quota` + ghi chú thiếu gì.
- **Gate chưa nối** (`nil`) → coi như dry-run.
- **Backup/restore**: zip hỏng → từ chối, dữ liệu thật không bị đụng.
- **API nội bộ**: mặc định 404 khi tắt.
