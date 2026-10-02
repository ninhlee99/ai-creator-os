# Studio AI (`/studio`)

## 1. Mục đích
Xưởng sản xuất video bằng AI: **Video affiliate** (ảnh sản phẩm + nhạc, không
chữ không voiceover — format Ninh chốt), Video chữ động, quản lý Jobs, bảng
nhạc trending Việt Nam.

> 🅿️ **Phim điện ảnh đã park** (Đợt A, commit `efa218b`): form tạo job phim,
> `runFilm`, cinematic renderer, capability probe Veo — code còn trong
> `internal/studio/film*.go`, `cinematic.go` và `internal/web/studio_film.go`
> với `//go:build parked`; prompt phim ở `internal/studio/prompts/parked/`.

## 2. Kích hoạt
- `GET /studio` → `handleStudio` (`internal/web/studio.go:73`): 2 tab —
  `ai` (Video affiliate, mặc định) và `?tab=chu` (Video chữ động).
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
  (xem `daemon-tu-dong.md`).
- Khởi động app: `MarkInterruptedJobs()` — job nào còn `running` → `failed` +
  log "Gián đoạn khi khởi động lại".

### 3.2. Video affiliate — `runAffiliate` (`internal/studio/studio.go:473`)
Phong cách Ninh chốt: **không chữ, không voiceover, chỉ video + nhạc bản quyền**.
1. `mg.Healthy(ctx)` false (thiếu `GEMINI_API_KEYS` hoặc key lỗi) → job failed
   ngay "media-gen chưa sẵn sàng", không render bừa.
2. `directorPhotoPlan(ctx, p)` — LLM viết shot list + **khóa 1 địa điểm** cho cả video.
3. Chụp từng ảnh: `GenerateImage(pr, refs=[model_photo, product_photo], out)` —
   identity/product lock bằng ảnh tham chiếu; mỗi ảnh upscale 4K
   (`UpscalePhoto(..., PhotoWidth4K, ...)`; lỗi upscale → dùng bản gốc, không fail job).
   Ít hơn 3 ảnh → job failed ("kiểm tra API key").
4. Dựng: `AssemblePhotoList` (Ken Burns, khổ 9:16) hoặc `AssembleBeatBounce`
   (kiểu giật-giật theo nhịp) → `MuxMusic` (nhạc upload hoặc URL, `loudnorm` −14 LUFS).
5. `caption.go` — LLM viết caption/hashtag tiếng Việt → cột `caption`, nút Copy trên UI.
6. Xong → `fireOnDone` → tự đăng TikTok **nháp** nếu bật (xem `daemon-tu-dong.md`).

### 3.3. Video chữ động (tab `?tab=chu`)
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
