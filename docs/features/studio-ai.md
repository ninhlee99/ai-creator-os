# Studio AI (`/studio`)

## 1. Mục đích
Xưởng sản xuất video bằng AI: **Video affiliate** (ảnh sản phẩm + nhạc, không
chữ không voiceover — format Ninh chốt), Video chữ động, quản lý Jobs, bảng
nhạc trending Việt Nam.

## 2. Kích hoạt
- `GET /studio` → `handleStudio` (`internal/web/studio.go:73`): Video affiliate, mặc định.
- `POST /studio/affiliate` → `handleStudioAffiliateCreate` — job affiliate.
- `POST /studio/kinetic` → `handleStudioKineticCreate` — video chữ động.
- `GET /studio/jobs`, `GET /studio/jobs/{id}` — danh sách + chi tiết job.
- `GET /studio/assets/{job}/{file}` — file asset của job.
- `GET /studio/trends` + `POST /studio/trends/refresh` — nhạc trending.
- `GET /studio/mediagen/health` → `handleStudioMediaGenHealth` — probe media-gen.

## 3. Luồng vận hành chi tiết

### 3.1. Nền tảng chung
- Mọi job lưu trong `studio.db`: bảng `studio_jobs` (id, kind, title, params JSON,
  status, progress, output, caption, log) và `studio_assets` (job_id, idx, kind,
  prompt, status, path).
- Chạy nền trong goroutine; tối đa **2 job render đồng thời**
  (`maxConcurrentStudioJobs = 2`, `internal/studio/studio.go:135`) — job thừa giữ
  status `queued` trung thực.
- `fireOnDone` gọi hook `SetOnDone` → autopilot tự đăng TikTok nháp
  (xem `autonomous-daemon.md`).
- Khởi động app: `MarkInterruptedJobs()` — job nào còn `running` → `failed` +
  log "Gián đoạn khi khởi động lại".

### 3.2. Video affiliate Lookbook Chuyển Động Người Thật — `runAffiliate` (`internal/studio/affiliate.go`)
Tuân thủ chuẩn **Open-Spec v1.0 (`docs/OPEN_SPEC.md`)**:
1. **Ưu tiên Lookbook Chuyển Động Thật (9:16 1080x1920)**:
   - Thay vì dựng ảnh tĩnh với thẻ chữ `AdCard` che khuất khung hình, hệ thống tự động kích hoạt kho video Lookbook thời trang người thật (`data/clips/fashion/*.mp4`).
   - Nếu có ảnh nhân vật kênh (`ModelPhoto`), kích hoạt InsightFace Deep Latent Face Swap hoán đổi khuôn mặt vào người mẫu lookbook đang bước đi/thử đồ.
2. **Dựng từ ảnh sản phẩm listing (`AffiliateModePhoto`)**:
   - Nếu người dùng chọn ảnh tĩnh, giữ khung ổn định và dùng dissolve nhẹ. Không zoom, rung, flash hoặc flicker.
   - Mặc định dùng engine video để tạo footage chuyển động thật. Nếu engine không tạo được clip, job báo lỗi thay vì dựng chuyển động giả từ ảnh.
   - Tự động chuẩn hóa tỷ lệ 9:16 với Lanczos interpolation chống méo hình (`scale=1080:1920:force_original_aspect_ratio=increase:flags=lanczos,crop=1080:1920,setsar=1`).
3. **Âm nhạc thương mại thịnh hành**:
   - Tự động chọn bài nhạc TikTok trending bắt tai từ kho nhạc thương mại (`FindTrendingMusic`), tạo năng lượng cho video ngắn.
4. **Caption & Hashtag**:
   - LLM viết caption/hashtag tiếng Việt chuẩn SEO TikTok Shop → cột `caption`, nút Copy Post Kit và Ghim vào Kệ Bio trên UI.
5. **Xong** → `fireOnDone` → tự đăng TikTok **nháp** nếu bật.

### 3.3. Video chữ động (legacy tab `?tab=text-animation`)
`handleStudioKineticCreate` → job lưu ở `s.Jobs` (file `content_jobs.json` của web
server, **không** phải studio.db) → `go s.runJob(jobID)` render nền → trang Jobs
poll tiến độ. Lỗi thiếu tiêu đề → `?err=` + toast.

### 3.4. Trends nhạc Việt
`internal/studio/trends.go`: `FetchVNTrending` cào bảng Kworb TikTok VN
(chart bên thứ ba — UI ghi rõ không phải Creative Center chính thức),
cache **6 giờ** (`trendsCacheTTL`); `TrendsStale()` → trang Studio tự
`RefreshVNTrendingAsync()` khi cũ (tối đa 1 refresh cùng lúc; lỗi cache lại,
không spam source). Nút "Làm mới" → `RefreshVNTrending` (xoá cache ép fetch).

## 4. Fail-closed & an toàn
- `mg.Healthy(ctx)` false → job failed "media-gen chưa sẵn sàng".
- Ít hơn 3 ảnh affiliate → failed "kiểm tra API key".
- Upscale lỗi → dùng bản gốc, không fail job.
- Backup loại trừ `output/` (video render nặng, tái tạo được).

## 5. API key rotation
- **Media (ảnh)**: `GeminiMediaGen` trên `GEMINI_API_KEYS` (keyring riêng trong
  `internal/studio/mediagen.go`): round-robin, bỏ qua key invalid, cooldown khi
  429/quota theo thang **60s → 5m → 15m** (`keyBackoffs`); key khoẻ lại thì reset thang.
- **Shot list/caption** (LLM): `s.llm` — LLMChain (xem `key-rotation.md`).
- **Nhạc trending**: không dùng key (cào Kworb, cache 6h).
- `GET /studio/mediagen/health` cho biết media-gen sẵn sàng hay không trước khi tạo job.
