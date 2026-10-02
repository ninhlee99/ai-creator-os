> ⚠️ **Tài liệu lịch sử:** tính năng phim điện ảnh đã park (build tag `parked`, Đợt A 2026-10-02). Giữ để tham khảo, không còn là tính năng hiện tại.
>

# Nâng "điện ảnh từ ảnh" tiến gần video quay thật — Nghiên cứu & thiết kế

Trạng thái: **nghiên cứu, chưa code**. Mọi con số render là ước tính (chưa đo
trên M1 Pro của Ninh); con số quota/API bên thứ ba ghi rõ "chưa kiểm chứng".

Ràng buộc cứng: **miễn phí tuyệt đối** (không Veo/billing), 1 binary Go,
ưu tiên pure Go (không cgo mới), sidecar chỉ theo mẫu uv-managed đã có
(VieNeu-TTS, llama-server), chạy offline trên MacBook Pro M1 Pro 32GB,
1920×1080 30fps, mọi khả năng mới phải có UI/UX.

---

## 1. Gap analysis — output hiện tại vs video quay thật

Renderer hiện tại (`internal/studio/cinematic.go`): keyframe → zoompan
supersample 2x có easing → grade theo mood → `noise=alls=7:allf=t` → vignette
→ letterbox 2.35:1 ở bản cuối → xfade 0.7s giữa shot.

| # | Gap | Vì sao lộ khi xem kỹ |
|---|-----|----------------------|
| G1 | **Không parallax** | zoompan chỉ scale ảnh phẳng: lớp gần/xa phóng to cùng tỉ lệ. Máy quay thật tiến vào thì lớp gần trôi nhanh hơn lớp xa, và hé lộ vùng bị che khuất. |
| G2 | **Nhân vật đứng yên như tượng** | Không bước đi, không chớp mắt, không thở, tóc/áo không động. Đây là điểm lộ nặng nhất ở shot có người cỡ trung/cận. |
| G3 | **Không lip-sync** | Đã được luật 3c né bằng narration/off-camera, nhưng thoại trực diện vẫn là môi đứng yên + tiếng nói — lộ ngay. |
| G4 | **Không micro handheld shake** | Máy quay thật (kể cả trên tripod tốt) luôn có rung động vi mô + trôi nhẹ. Shot hiện tại chuyển động toán học hoàn hảo → cảm giác CGI. |
| G5 | **Không motion blur** | Vật/máy chuyển động trong quay thật nhòe nhẹ theo hướng chuyển động. zoompan 30fps cho biên sắc nét tuyệt đối → "sạch" một cách giả. |
| G6 | **Grain tổng hợp ≠ noise cảm biến** | `noise` của FFmpeg là nhiễu trắng đều; noise cảm biến thật phụ thuộc độ sáng (vùng tối nhiễu hơn), có thành phần cố định theo thời gian. Khác biệt tinh tế, chỉ lộ khi soi. |
| G7 | **Không hạt vật lý trong khung** | Mưa không rơi, bụi không bay, khói không cuộn. Quay thật ngoài trời/ trong quán cũ luôn có hạt chuyển động — thiếu nó, khung hình "chết". |
| G8 | **Không depth-of-field / focus pull** | Điện ảnh thật có xóa phông + chuyển nét (focus pull) dẫn mắt người xem. Ảnh AI thường nét đều toàn khung → phẳng, thiếu chiều sâu điện ảnh. |

Thứ tự lộ khi xem (nặng → nhẹ): G2 > G1 > G3 > G4 > G7 > G5 > G8 > G6.

---

## 2. Phương án khắc phục cho từng gap (chỉ free + local)

### G1 — Parallax

**P1a. Depth-based 2.5D warp (KHUYẾN NGHỊ)**
- Nguyên lý: sidecar ước lượng depth map **1 lần/shot** → pure Go tách 2–3
  lớp theo depth → mỗi frame dịch lớp gần nhiều hơn lớp xa theo đường máy
  quay; mép hở inpaint bằng edge-stretch (kỹ thuật 3D-photo chuẩn).
- Độ khó: trung bình. Sidecar Python mới (xem §4).
- Render M1 Pro (ước tính): depth ~3–5s/shot (MiDaS-small, MPS) + warp
  pure Go ~2s/shot (240 frame × bilinear sampling — Go đủ nhanh).
- Rủi ro: mép vật thể có thể rách >2px nếu depth sai; cần clamp + test.
- Binary: OK — sidecar uv-managed, Go vẫn pure.

**P1b. Fake parallax 2 lớp bằng mask người (dự phòng)**
- Nguyên lý: tách người ( GrabCut heuristic / depth threshold ) khỏi nền,
  dịch 2 lớp ngược pha nhẹ. Không cần depth net đầy đủ nếu chỉ có người.
- Độ khó: thấp hơn P1a nhưng méo mó khi có nhiều vật thể ở nhiều độ sâu.
- Khuyến nghị: chỉ dùng khi P1a thất bại.

**P1c. Giữ zoompan (không làm)**
- Chấp nhận phẳng. Rẻ nhất nhưng không cải thiện gì.

### G2 — Nhân vật đứng yên

**P2a. Không sửa được bằng free+local — né bằng đạo diễn (KHUYẾN NGHỊ, đã có)**
- Nguyên lý: luật 3c hiện tại (narration/off-camera/cutaway) chính là đáp án
  đúng của điện ảnh cho điểm yếu này. Tăng cường: prompt đạo diễn ưu tiên
  shot mà "đứng yên" là tự nhiên (nhìn xa xăm, trầm tư, ngủ, hồi tưởng).
- Độ khó: chỉ sửa prompt. Render +0s.
- Đây là phương án chính cho G2 trong mọi đợt.

**P2b. Face micro-motion (blink) qua sidecar**
- Nguyên lý: detect face → warp mí mắt theo chu kỳ chớp mắt ngẫu nhiên.
- Độ khó: trung bình; rủi ro uncanny cao nếu warp sai (mắt là vùng nhạy
  nhất). ROI thấp so với rủi ro → **không khuyến nghị**.

**P2c. Full-body animation (bước đi, cử chỉ)**
- Không tồn tại phương án free+local. Các model open (AnimateDiff, SVD)
  cần VRAM lớn, chất lượng identity kém, không chạy ổn định trên M1 32GB
  cho pipeline 30 shot. **Loại.**

### G3 — Lip-sync

**P3a. MuseTalk sidecar, chỉ shot thoại trực diện (KHUYẾN NGHỊ cho Đợt 3)**
- Nguyên lý: render shot cinematic như thường → MuseTalk (audio-driven
  lip-sync) animate vùng miệng theo file thoại TTS → composite lại.
- Trạng thái repo: client Go đã wired (`internal/engines/avatar/`,
  HTTP + health check), nhưng **sidecar chưa tồn tại** — cần: env uv +
  torch (MPS) + weights MuseTalk (~1–2GB tải 1 lần) + script server nhỏ
  exposing `/lip_sync`, quản lý bởi `local.Process` (mẫu VieNeu).
- Độ khó: cao (lần đầu dựng sidecar nặng; MPS compatibility của MuseTalk
  chưa kiểm chứng).
- Render M1 Pro (ước tính, từ research notes): ~3–6 fps → clip 8s
  (~200 frame) ≈ **1–2 phút/shot**. Chỉ áp dụng cho shot đạo diễn đánh dấu
  `talking_head` (2–5 shot/phim ngắn) → +5–10 phút/phim. Chấp nhận được.
- Rủi ro: torch/MPS lỗi → fail-closed về narration (luật 3c); chất lượng
  miệng vùng râu/tối có thể giả.
- Binary: OK — sidecar ngoài, Go pure.

**P3b. Procedural jaw animation (phương án rẻ)**
- Nguyên lý: detect vùng miệng → squash-stretch dọc theo biên độ envelope
  của audio. Pure Go/FFmpeg, ~0s render thêm.
- Chất lượng: trung bình-thấp, chỉ qua được khi mặt nhỏ/xa. Có thể làm
  "bản nháp" trước khi P3a xong. Khuyến nghị: làm nếu P3a tắc >1 đợt.

**P3c. Né bằng kịch bản (đã có, giữ nguyên)**
- Luật 3c: thoại trực diện tối thiểu + cutaway. Luôn là lớp phòng thủ.

### G4 — Micro handheld shake

**P4a. Sum-of-sines crop offsets, pure FFmpeg (KHUYẾN NGHỊ, Đợt 1)**
- Nguyên lý: scale ảnh lên 1.06 rồi crop với x/y = tổng 2–3 sóng sin tần số
  khác nhau (0.3Hz + 1.1Hz + 2.3Hz, biên độ 2–6px) → rung mượt, không giật
  (tránh `random()` của FFmpeg vì nó nhiễu trắng theo frame).
- Đạo diễn điều khiển cường độ: `tripod` (gần như đứng yên, biên 1px) /
  `handheld` (biên 4px) / `run` (biên 8px + roll nhẹ).
- Độ khó: thấp — thêm vào filter chain hiện tại, 0 dependency mới.
- Render: +~5% thời gian shot (crop/scale đã có sẵn trong chain).
- Rủi ro: gần như không.

### G5 — Motion blur

**P5a. `tmix` nhẹ (KHUYẾN NGHỊ, Đợt 1, có toggle)**
- Nguyên lý: `tmix=frames=3:weights='1 2 1'` trộn 3 frame liên tiếp → nhòe
  nhẹ theo hướng chuyển động, giống màn trập 180°.
- Độ khó: thấp. Rủi ro: quá đà → mờ; để toggle + mặc định BẬT ở mức nhẹ.
- Render: +~10% (giữ 3 frame trong RAM — 1920×1080×3, không đáng kể).

### G6 — Sensor-realistic grain

**P6a. Grain 2 lớp (KHUYẾN NGHỊ, Đợt 1)**
- Nguyên lý: giữ `noise=alls=7:allf=t` (temporal) + thêm lớp nhiễu tĩnh rất
  nhẹ (`allf=u` cường độ 2–3, mô phỏng fixed-pattern) — mắt thường khó phân
  biệt nhưng soi kỹ thấy "có thịt" hơn.
- Độ khó: thấp. Render ~+0%.

### G7 — Hạt vật lý (mưa/bụi/tuyết)

**P7a. Particle overlay generator pure Go (KHUYẾN NGHỊ, Đợt 1)**
- Nguyên lý: chương trình Go nhỏ sinh **1 lần/job** một vòng lặp overlay
  trong suốt: rain = streaks xiên có motion blur vẽ theo vector gió;
  dust = chấm trôi chậm nhiều lớp độ sâu; snow tương tự. Xuất PNG sequence
  → FFmpeg overlay loop lên shot.
- Đạo diễn gating: weather trong breakdown (`mưa` → rain, `quán cũ/bụi`
  → dust, `đêm` → có thể thêm đom đóm — không, giữ 2 loại).
- Độ khó: trung bình-thấp (thuật toán vẽ đơn giản, không vật lý phức tạp).
- Render: sinh overlay 1 lần ~5–10s/job; overlay khi render ~+5%/shot.
- Rủi ro: thấp. Binary: pure Go, 0 asset ship kèm (sinh runtime).

**P7b. Sidecar particle sim** — overkill, loại.

### G8 — DoF / focus pull

**P8a. Depth-based bokeh (Đợt 2, dùng chung depth map của P1a)**
- Nguyên lý: có depth map rồi → blur nền theo khoảng cách (pure Go,
  stacked box blur), giữ nét chủ thể; focus pull = animate bán kính blur
  theo thời gian theo chỉ đạo đạo diễn (`focus: foreground→background`).
- Độ khó: trung bình. Render +~3s/shot (blur trên band nền).
- Không depth thì chỉ làm được tilt-shift giả (gradient blur) — méo mó,
  không khuyến nghị.

---

## 3. Kế hoạch theo đợt

### Đợt 1 — "Máy quay thật" (chỉ FFmpeg + pure Go, KHÔNG sidecar mới)

**Mục tiêu:** xóa cảm giác CGI toán học; thêm sự sống cho khung hình.
**Việc làm:**
1. Handheld micro-shake (P4a) — 3 mức `tripod/handheld/run`, đạo diễn chọn
   trong breakdown; mặc định `handheld` nhẹ cho mọi shot (trừ khi kịch bản
   ghi tripod).
2. Motion blur nhẹ `tmix` (P5a) — toggle ở form phim, mặc định bật.
3. Grain 2 lớp (P6a).
4. Particle overlay generator (P7a) — rain/dust, đạo diễn gating theo weather.
5. UI: 2 toggle ở form phim ("Rung máy", "Hạt mưa/bụi tự động"); badge
   "🎥 rung tay" không cần — đây là mặc định pipeline, không phải method mới.

**Tiêu chí nghiệm thu (đo được):**
- Render 1 shot 8s: thời gian tăng <15% so với hiện tại (đo trên cùng máy).
- So sánh blind A/B (frame giữa shot, có/không shake+grain): người xem mô tả
  bản mới "giống quay tay" — kiểm bằng mắt, ghi nhận.
- Overlay mưa: loop 8s không thấy điểm nối (so sánh frame đầu/cuối).
- `go build`, `go vet`, `go test ./...` xanh; không dependency mới.

**KHÔNG làm:** parallax, depth, lip-sync, DoF, sidecar mới, thay đổi prompt
director ngoài việc thêm trường `camera_shake`/`weather_fx`.

### Đợt 2 — Parallax + DoF (1 sidecar depth mới, uv-managed)

**Mục tiêu:** chiều sâu 3D thật khi máy di chuyển; xóa phông điện ảnh.
**Việc làm:**
1. Sidecar `depth` mới: Python + MiDaS-small (hoặc Depth-Anything-V2-small
   nếu M1 MPS chạy được), quản lý bởi `local.Process` theo đúng mẫu
   VieNeu-TTS (`internal/engines/local/supervisor.go`): `uv venv` + tải
   weights 1 lần vào `<dataDir>/models/`, health check, fail-closed khi
   thiếu (rơi về zoompan cũ, log rõ).
2. Pure Go layered warp (P1a): 1 depth map/shot → 3 bands → shift vi phân
   theo camera path; edge-stretch inpaint; clamp rách mép ≤2px.
3. Depth-based bokeh + focus pull (P8a) — đạo diễn điều khiển qua breakdown.
4. UI: toggle "Parallax 3D" ở form phim (mặc định bật khi sidecar sẵn sàng);
   Cài đặt → Model local thêm hàng trạng thái sidecar depth (mẫu probe có sẵn).

**Tiêu chí nghiệm thu:**
- Video test dolly-in: đo pixel — vật foreground dịch chuyển nhiều hơn
  background ≥1.5x (chứng minh parallax, không phải scale đều).
- Không rách mép >2px ở 95% frame (so sánh frame kề).
- Overhead render ≤15s/shot trên M1 Pro (depth ~3–5s + warp ~2s + bokeh ~3s,
  đo thật khi làm).
- Sidecar thiếu/mất: job vẫn chạy bằng zoompan cũ, badge trung thực.

**KHÔNG làm:** lip-sync, subject animation, sidecar thứ hai, đụng vào
pipeline Veo.

### Đợt 3 — Lip-sync cho shot thoại trực diện (hoàn thiện MuseTalk sidecar)

**Mục tiêu:** môi mấp máy khớp thoại ở shot cận mặt nói chuyện.
**Việc làm:**
1. Hoàn thiện sidecar MuseTalk: env uv + torch MPS + weights (~1–2GB) +
   script server `/lip_sync`, supervised như VieNeu; UI Cài đặt → Model local
   hiện trạng thái (mẫu có sẵn).
2. Director đánh dấu shot `talking_head: true` (khi thoại trực diện không
   né được); pipeline: render cinematic → MuseTalk(face video + wav thoại)
   → mux lại; fail-closed về bản narration khi sidecar thiếu/lỗi.
3. UI: toggle "Lip-sync thử nghiệm" ở form phim (mặc định TẮT cho tới khi
   Ninh duyệt chất lượng); badge 🎙 trên storyboard cho shot đã lip-sync.
4. Phương án dự phòng P3b (procedural jaw) chỉ làm nếu P3a tắc.

**Tiêu chí nghiệm thu:**
- Đo tương quan: độ mở miệng (mouth openness từ frame) tương quan với
  envelope audio (Pearson r > 0.6 trên shot test) — đo bằng script, không
  bằng mắt.
- Chỉ shot `talking_head` bị xử lý; shot khác bit-identical với Đợt 2.
- Sidecar lỗi giữa chừng: job không fail, shot đó giữ bản cũ + log rõ.

**KHÔNG làm:** full-body motion, đi lại, cử chỉ tay, diễn xuất cơ thể —
vĩnh viễn ngoài phạm vi (xem §4).

---

## 4. Trần trung thực — nói thật với Ninh

**Với ràng buộc free + local, ĐẠT ĐƯỢC:**
- Cảm giác máy quay thật (rung tay, motion blur, grain có thịt).
- Chiều sâu 3D khi máy di chuyển (parallax 2.5D) + xóa phông/focus pull.
- Hạt vật lý: mưa rơi, bụi bay — đúng thời tiết kịch bản.
- Lip-sync cho shot cận mặt nói chuyện (sau Đợt 3).
- Nhân vật nhất quán, màu điện ảnh, dựng chuẩn — đã có.

**VĨNH VIỄN KHÔNG (free + local, công nghệ 2026):**
- Nhân vật tự bước đi, chạy, đánh nhau, cử chỉ tay phức tạp, diễn xuất cơ
  thể — cần video-gen thật (Veo = trả phí).
- Thay đổi góc máy thật: nhìn thấy mặt sau vật thể, hé lộ không gian bị che
  vượt quá khả năng inpaint — parallax 2.5D chỉ giả lập trong biên độ nhỏ.
- Tóc/áo bay theo vật lý, chất lỏng, lửa cháy lan — cần mô phỏng vật lý.
- Biểu cảm vi mô (mắt rưng rưng dần, môi mím) theo diễn biến cảm xúc.

**Kết luận trung thực:** sau 3 đợt, output đạt mức **"phim AI cao cấp"** —
xem trên điện thoại/TV thường khó phân biệt với phim quay thật ở shot tĩnh,
lộ ở shot hành động cơ thể. Kịch bản phải tiếp tục né điểm yếu (luật 3c) —
đó không phải tạm bợ mà là ngữ pháp làm phim đúng cho công cụ này, giống
cách phim kinh dị né lộ quái vật.

**Khuyến nghị thứ tự:** làm **Đợt 1 trước** — 100% pure Go/FFmpeg, rủi ro
~0, hiệu quả cảm nhận cao nhất (G4+G7 là 2 gap lộ nhất sau G2/G1 mà không
cần sidecar). Đợt 2 khi Đợt 1 xong. Đợt 3 cuối cùng (nặng nhất, ít shot dùng
đến nhất).
