# Studio AI (`/studio`)

## 1. Mục đích
Xưởng sản xuất video bằng AI: Video chữ động, Phim (điện ảnh 30–3600s), Affiliate
(thời trang/sản phẩm), quản lý Jobs, bảng nhạc trending Việt Nam.

## 2. Kích hoạt
- `GET /studio` → `handleStudio` (`internal/web/studio.go:75`): 5 tab
  (`?tab=chu` video chữ động, phim, affiliate, jobs, trends).
- `POST /studio/kinetic` → `handleStudioKineticCreate` — video chữ động.
- `POST /studio/affiliate` → `handleStudioAffiliateCreate` — job affiliate.
- `POST /studio/film` → `handleStudioFilmCreate` — job phim (30–3600s).
- `GET /studio/jobs`, `GET /studio/jobs/{id}` — danh sách + storyboard chi tiết.
- `POST /studio/jobs/{id}/rerun` — chạy tiếp job phim bị lỗi/gián đoạn.
- `POST /studio/jobs/{id}/shots/{seq}/rerender` — quay lại một shot rồi dựng lại.
- `GET /studio/trends` + `POST /studio/trends/refresh` — nhạc trending.
- `GET /studio/mediagen/health` → `handleStudioMediaGenHealth` — probe media-gen.

## 3. Luồng vận hành chi tiết

### 3.1. Nền tảng chung
- Mọi job lưu trong `studio.db`: bảng `studio_jobs` (id, kind, title, params JSON,
  status, progress, output, caption, log) và `studio_assets` (job_id, idx, kind,
  prompt, status, path, **method**: `veo` | `image` — badge 🎬/🖼️ trên storyboard).
- Chạy nền trong goroutine; tối đa **2 job render đồng thời**
  (`maxConcurrentStudioJobs = 2`, `internal/studio/studio.go:170`) — job thừa giữ
  status `queued` trung thực.
- **Badge phương pháp** trên storyboard (`filmMethodLabel`): 🎬 Veo (`method="veo"`),
  🎞 Điện ảnh (`method="cinematic"` — chế độ chính từ Wave 3), 🖼 Ảnh + giọng đọc
  (`method="anh-tts"` — legacy Wave 1/2), 📱 Trailer 9:16.
- `fireOnDone` gọi hook `SetOnDone` → autopilot tự đăng TikTok (xem `daemon-tu-dong.md`).
- Khởi động app: `MarkInterruptedJobs()` (`studio.go:756`) — job nào còn `running`
  → `failed` + log "Gián đoạn khi khởi động lại — bấm Chạy tiếp"; `runFilm` bỏ qua
  asset `done` mà file còn tồn tại (`resumeAssetPath`) → resume thật từng shot.

### 3.2. Video chữ động (tab `?tab=chu`, gộp từ `/content` cũ)
`handleStudioKineticCreate` → job lưu ở `s.Jobs` (file `content_jobs.json` của web
server, **không** phải studio.db) → `go s.runJob(jobID)` render nền → trang Jobs
poll tiến độ. Lỗi thiếu tiêu đề → `?err=` + toast.

### 3.3. Affiliate — `runAffiliate` (`internal/studio/studio.go:510`)
Phong cách Ninh chốt: **không chữ, không voiceover, chỉ video + nhạc bản quyền**.
1. `directorPhotoPlan(ctx, p)` — LLM viết shot list + **khóa 1 địa điểm** cho cả video.
2. Chụp 3–5 ảnh: `GenerateImage(pr, refs=[model_photo, product_photo], out)` —
   identity/product lock bằng ảnh tham chiếu; mỗi ảnh upscale 4K
   (`UpscalePhoto(..., PhotoWidth4K, ...)`; lỗi upscale → dùng bản gốc, không fail job).
   Ít hơn 3 ảnh → job failed ("kiểm tra API key").
3. Dựng: `AssemblePhotoList`/`AssembleBeatBounce` (Ken Burns theo khổ 9:16) →
   `MuxMusic` (nhạc upload hoặc URL, `loudnorm` −14 LUFS) → `ConcatClips`
   (chuẩn hoá khổ từng clip trước khi nối).
4. `caption.go` — LLM viết caption/hashtag tiếng Việt → cột `caption`, nút Copy trên UI.
5. Xong → `fireOnDone` → tự đăng TikTok **nháp** nếu bật (xem `daemon-tu-dong.md`).

### 3.4. Phim điện ảnh — `runFilm` (`internal/studio/studio.go:971`)

```
Form /studio (topic, seconds 30–3600, genre, render_mode, music_file?, upscale?)
  │ POST /studio/film → handleStudioFilmCreate
  │   Topic trống → ?err= · Aspect ép "16:9" (Ninh 2026-10-02: mọi phim 16:9)
  │   render_mode ∈ {auto (mặc định, khuyên dùng), cinematic (miễn phí),
  │     veo (tốn phí)} — ước tính chi phí/ETA hiển thị theo chế độ
  ▼
CreateFilmJob (studio.go:711) → job kind="film", status queued
  ▼  goroutine runFilm (tối đa 2 job song song)
┌──────────────────────────────────────────────────────────────┐
│ 0. Khả năng AI — hệ thống tự biết mình làm được gì (capabilities.go)│
│ Bảng capabilities trong studio.db, xem ở Cài đặt → Model     │
│ local:                                                       │
│ · Vẽ ảnh (image_gen): ProbeImageGen VẼ THỬ 1 ảnh đơn giản    │
│ lúc khởi động app (goroutine, timeout 3 phút) → ok/fail +    │
│ thời điểm + chi tiết lỗi (cắt 120 ký tự).                    │
│ · Quay video (video_gen): KHÔNG tự probe (1 clip thử tốn ~8s │
│ Veo tiền thật) — RefreshVideoStatusFromHistory suy từ 30     │
│ shot gần nhất: có method "veo" thành công → ok; toàn         │
│ "anh-tts" → fail; chưa quay lần nào → unknown. Probe tay     │
│ luôn thắng: lịch sử chỉ ghi khi status còn unknown.          │
│ · Nút "Kiểm tra quay video" (POST                            │
│ /settings/model-local/probe-video): modal cảnh báo "TỐN ~8s  │
│ Veo (tiền thật, ước tính chưa kiểm chứng)" → job nền         │
│ (timeout 20 phút), xong quay lại xem kết quả.                │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 1. Chế độ dựng — resolveRenderMode (cinematic.go)            │
│ "auto" (khuyên dùng): video_gen = ok → "veo" (thử Veo từng   │
│ shot); unknown/fail (điển hình key Ninh) → THẲNG             │
│ "cinematic", KHÔNG thử Veo, không tốn thời gian poll API     │
│ chết.                                                        │
│ "cinematic": luôn dựng điện ảnh từ ảnh (chỉ tốn image gen).  │
│ "veo": luôn thử Veo từng shot (tốn phí); lỗi → rơi về điện   │
│ ảnh, không fail job.                                         │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 2. Director 3 pha — truyện → kịch bản → breakdown (director.go)│
│ Quy tắc bất di bất dịch: "truyện → kịch bản → breakdown →    │
│ storyboard → quay → dựng".                                   │
│ Pha 0 — Truyện (prompts/story.txt, 1 call): LLM viết truyện  │
│ ngắn/tiểu thuyết mini HOÀN CHỈNH (~2 từ/giây phim, kẹp       │
│ 300–6000 từ) — "linh hồn", pha 1 chuyển thể nó.              │
│ Pha 1 — Kịch bản (action + thoại là CHUẨN, chưa có character │
│ sheet/shot list): ≤180s → 1 call (screenplay.txt, 2–6 cảnh,  │
│ 3 hồi thu nhỏ); >180s → 3 call (film_act1/2/3.txt —          │
│ 20%/55%/25% thời lượng, 3–10 cảnh/hồi; dàn nhân vật hồi sau  │
│ trích XÁC ĐỊNH từ tên trong thoại hồi trước, không phụ thuộc │
│ LLM).                                                        │
│ Pha 2 — Breakdown (prompts/breakdown.txt, 1 call): LLM đọc   │
│ toàn kịch bản, trích character bible (thiết kế PHẢI lý giải  │
│ từ nhu cầu kịch bản — vd design_note "váy vàng cảnh 4 = sự   │
│ hồi sinh") + khóa bối cảnh + continuity + shot list từng     │
│ cảnh. Thiếu cảnh hoặc cảnh 0 shot → lỗi fail-closed.         │
│ → script.json trong work dir là FilmScriptBundle {story,     │
│ screenplay, breakdown, script} — storyboard UI xem lại từng  │
│ pha; chạy tiếp dùng đúng bundle cũ (job Wave 1–2 chỉ có      │
│ FilmScriptPro vẫn đọc được). film_short.txt ĐÃ XOÁ ở Wave 3. │
│ Prompt đạo diễn trong internal/studio/prompts/*.txt          │
│ (go:embed).                                                  │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 3. Chân dung nhân vật (identity lock)                        │
│ Mỗi nhân vật: GenerateImage(portrait 9:16, nền trơn) → asset │
│ kind="portrait" (idx 1000+i). Lỗi → asset failed + cảnh báo  │
│ trên storyboard (không continue chìm). Portrait được truyền  │
│ làm ImageRef vào MỌI lần sinh keyframe sau này + khối text   │
│ CHARACTER LOCK trong prompt.                                 │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 4. Chia shot — expandSceneShots (studio.go:928)              │
│ Mỗi cảnh → các shot ≤8s (đơn vị thật của Veo 3 —             │
│ veoMaxSeconds, mediagen.go:333). Thoại/lời dẫn chia theo     │
│ shot để phụ đề khớp. Đạo diễn đánh dấu shot trailer_worthy + │
│ luật center-safe (giữ nhân vật giữa khung để crop dọc).      │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 5. Trần chi phí — chỉ áp cho Veo (đường cinematic ~miễn phí nên không check)│
│ budget.cap = automation.APIBudgetUSD (knob                   │
│ ops.api_budget_usd, R2-W4); giá VEO_USD_PER_SEC (mặc định    │
│ $0.05/s — "ước tính chưa kiểm chứng"). Vượt trần → job       │
│ failed sạch + log tiếng Việt. Không đặt trần → log cảnh báo, │
│ vẫn chạy.                                                    │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 6. Render từng shot — renderFilmShot                         │
│ Với mỗi shot (bỏ qua shot đã done + file còn tồn tại).       │
│ Keyframe: keyPrompt = shot.ImagePrompt + charLocks +         │
│ SceneLockBlock + CONTINUITY BIBLE (nguyên văn) + "Cinematic  │
│ photorealistic, horizontal 16:9".                            │
│ a. Chế độ "veo" — tryVeoShot: FirstFrame (shot 0 vẽ keyframe │
│ mới; shot sau cắt frame cuối shot trước qua                  │
│ extractLastFrame, hỏng → vẽ lại). Trần chi phí check TRƯỚC   │
│ mỗi lần gọi (billed = min(seconds, 8)); vượt → job failed    │
│ sạch. vprompt gồm PERFORMANCE (action cụ thể) + FACIAL       │
│ EXPRESSIONS/EMOTIONS từ dialogue emotion + continuity bible  │
│ + camera language. Thành công → method "veo" 🎬 + học         │
│ capability: video_gen ← ok (bằng chứng từ lần chạy thật).    │
│ Veo lỗi → log tiếng Việt, RƠI VỀ điện ảnh (bước b), không    │
│ fail job.                                                    │
│ b. "Điện ảnh từ ảnh" — renderCinematicShot (CHẾ ĐỘ CHÍNH):   │
│ keyframe (prompts/film_keyframe.txt: bố cục chừa biên cho    │
│ chuyển động) → RenderCinematicShot (cinematic.go) — zoompan  │
│ có easing theo đúng camera_move của đạo diễn, grain nhẹ,     │
│ vignette, grade màu theo mood cảnh, 1920×1080 @30fps. Giọng  │
│ đọc shot: thoại+lời dẫn → chunk ≤800 ký tự → TTS →           │
│ ConcatWavs; lỗi TTS → dựng bản câm (không fail). Thời lượng  │
│ shot = max(giây kịch bản, giây audio). method = "cinematic"  │
│ → badge 🎞 "Điện ảnh".                                        │
│ · Asset kind="shot" (idx = seq): prompt đầy đủ lưu DB →      │
│ storyboard hiện thumbnail + "Xem prompt" + "Quay lại shot    │
│ này" (render lại 1 shot rồi dựng lại).                       │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 7. Dựng cuối — assembleFilmFinal (studio.go:1346, rẽ nhánh theo chế độ)│
│ Chế độ cinematic: AssembleCinematic — chuẩn hoá              │
│ 1920×1080/30fps rồi nối bằng chuyển cảnh xfade 0.7s +        │
│ letterbox 2.35:1 (viền điện ảnh; trailer không dính) + phụ   │
│ đề theo timeline xfade (cinematicCues) → mux mov_text vie.   │
│ Chế độ veo: ConcatClips nối cứng (probe, scale/pad về        │
│ 1920×1080, không đọc được → lỗi to) + phụ đề BuildSRT → mux  │
│ mov_text vie.                                                │
│ Nhạc: MixMusicBed — sidechain duck ≈−8dB khi có voice +      │
│ loudnorm −14 LUFS (lỗi → giữ bản không nhạc). Upscale        │
│ (optional): UpscaleVideo lanczos ×2 — UI ghi rõ "không phải  │
│ 4K native, render rất lâu". → studio-<id>.mp4 trong outDir,  │
│ setOutput + status done.                                     │
└──────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────┐
│ 8. Trailer dọc — buildTrailers (studio.go:1322)              │
│ Shot trailer_worthy → selectTrailerGroups chia 2 nhóm (nửa   │
│ đầu/nửa sau), mỗi nhóm 45–60s → CropCenterVertical           │
│ (crop=ih*9/16:ih → scale 1080×1920) → ConcatClips 9:16 →     │
│ nhạc + loudnorm → asset kind="trailer" 📱. Không shot nào     │
│ đánh dấu → bỏ qua, không lỗi.                                │
└──────────────────────────────────────────────────────────────┘
  ▼ fireOnDone → hook autopilot (chỉ job affiliate mới tự đăng)
```
### 3.5. Trends nhạc Việt
`internal/studio/trends.go`: `FetchVNTrending` cào bảng Kworb TikTok VN
(chart bên thứ ba — UI ghi rõ không phải Creative Center chính thức),
cache **6 giờ** (`trendsCacheTTL`); `TrendsStale()` → trang Studio tự
`RefreshVNTrendingAsync()` khi cũ (tối đa 1 refresh cùng lúc; lỗi cache lại,
không spam source). Nút "Làm mới" → `RefreshVNTrending` (xoá cache ép fetch).

## 4. Fail-closed & an toàn
- `mg.Healthy(ctx)` false (thiếu `GEMINI_API_KEYS` hoặc key lỗi) → job failed ngay
  với log "media-gen chưa sẵn sàng", không render bừa.
- Chế độ `auto` + `video_gen` unknown/fail → đi thẳng điện ảnh từ ảnh, **không bao
  giờ thử Veo** (không tốn phút poll API chết); chỉ khi chọn tay `veo` hoặc
  capability ok mới gọi Veo.
- Veo lỗi (chế độ veo) → rơi về điện ảnh từ ảnh có log, không bịa clip; badge
  method trung thực (🎬 / 🎞 / 🖼 legacy).
- Ít hơn 3 ảnh affiliate → failed "kiểm tra API key".
- Vượt trần chi phí Veo → dừng sạch, log tiếng Việt, bấm Chạy tiếp sau khi nâng trần.
- Backup loại trừ `output/` (video render nặng, tái tạo được).

## 5. API key rotation
- **Media (ảnh + Veo)**: `GeminiMediaGen` trên `GEMINI_API_KEYS` (keyring riêng trong
  `internal/studio/mediagen.go`): round-robin, bỏ qua key invalid, cooldown khi
  429/quota theo thang **60s → 5m → 15m** (`keyBackoffs`, mediagen.go:54); key khoẻ
  lại thì reset thang.
- **Kịch bản/shot list/caption** (LLM): `s.llm` — LLMChain (xem `key-rotation.md`).
- **TTS giọng đọc**: TTSChain — Gemini → VieNeu local → Edge (xem `key-rotation.md`).
- **Lưu ý trung thực (Ninh báo 2026-10-02)**: key Gemini của Ninh hiện **không tạo
  được video** (Veo đòi Google Cloud project bật billing trên key). Vì vậy chế độ
  dựng chính là **điện ảnh từ ảnh** (`renderCinematicShot`, badge 🎞) — Veo chỉ là
  tuỳ chọn khi `video_gen = ok` hoặc người dùng chọn tay. Không bao giờ nói dối là
  "quay bằng Veo" khi không phải.
- `GET /studio/mediagen/health` cho biết media-gen sẵn sàng hay không trước khi tạo job;
  `GET /settings/model-local` cho biết khả năng vẽ ảnh / quay video (xem `cai-dat.md`).
