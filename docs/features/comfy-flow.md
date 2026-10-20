# Comfy Flow Studio — Kiến Trúc Đồ Thị Node-Graph Tự Động Hóa
## (ComfyUI-like Dynamic DAG Execution Engine)

> **Mục tiêu**: Cung cấp một động cơ đồ thị Node-Graph (DAG) linh hoạt lấy cảm hứng từ ComfyUI, cho phép tự động biên dịch yêu cầu tự nhiên (Text Prompt, Trend Link, Story Script) thành các luồng kết nối Node để thực thi qua hệ sinh thái Web Adapters và Local Engines.

---

## 1. Bản Đồ Nghiệp Vụ & Khả Năng Thực Thi (100% Miễn Phí — Zero Cost)

> **Cam kết 0đ**: Toàn bộ hệ thống được xây dựng để tận dụng tối đa tài nguyên miễn phí:
> - **Google Flow**: 1.000 credits Veo miễn phí mỗi tháng để sinh video chuyển động.
> - **Muse AI Video Studio**: Tạo video chuyển động mượt mà 60fps qua tài khoản web free.
> - **Gemini Cloud (Google AI Studio)**: Free tier API sinh prompt, kịch bản, và TTS Audio chất lượng cao (`Aoede`).
> - **ChatGPT & Claude Web**: Tận dụng các phiên đăng nhập web miễn phí để viết kịch bản, phân đoạn đạo diễn.
> - **FFmpeg & Internal Engine**: Ghép stream, cắt beat drop, hòa âm foley, chống méo 9:16 không tốn GPU hay chi phí phần mềm.
>
> **Tuyệt đối KHÔNG phụ thuộc**: Các dịch vụ tốn phí như Kling AI Pro, Midjourney, Fashn Try-On API, Wan 2.1 Server...

1. **Phim Ngắn Vi Kịch AI (Google Flow + Muse AI + Gemini)**:
   - `input.cinema_script` (Claude/ChatGPT Web) -> `input.actor_bank` -> `local.cinematic_camera` -> `adapter.gemini_tts` -> `adapter.web_kling_video` (Google Flow / Muse AI Keyframes 0đ) -> `adapter.web_lip_sync` (Muse AI Sync Free) -> `adapter.web_faceswap` -> `local.audio_ducking_foley` -> `local.color_grading_lut` -> `qc.soul_index` -> `output.cinema_master` (21:9 Cinemascope & 9:16 Douyin Drama).
2. **Trendjack Vũ Đạo Douyin / TikTok Triệu View**:
   - `input.tiktok_trend` -> `input.product_outfit` -> `adapter.web_virtual_tryon` (Gemini/Claude) -> `adapter.web_faceswap` (Khóa mặt 360°) -> `adapter.web_motion_transfer` (Muse AI 60fps Free) -> `local.capcut_beat_mux` -> `qc.soul_index` -> `output.export_target`.
3. **Lookbook Thời Trang Chuyển Động Người Thật**:
   - `input.product_outfit` -> `input.character_face` -> `adapter.web_virtual_tryon` (Gemini Phối Đồ) -> `adapter.web_video_gen` (Google Flow Veo 1.000 Credits Free) -> `local.video_remaster` -> `adapter.web_faceswap` (Khóa mặt 360°) -> `local.antidistortion_crop` -> `qc.soul_index`.
4. **Phim Truyện Audio Điện Ảnh Đa Cảnh (Anime Makoto Shinkai)**:
   - `input.story_script` (ChatGPT/Claude) -> `adapter.web_story_decomposer` (Gemini) -> `adapter.web_image_gen` (Google Labs / Gemini 4K) -> `adapter.gemini_tts` (Aoede Free Tier) -> `local.acoustic_foley_bed` (Mưa & Piano -18dB) -> `local.capcut_beat_mux` (Ken Burns) -> `qc.soul_index`.


---

## 2. Giao Diện & Điều Khiển (`/flow`)

- **Khuôn Mẫu Tối Ưu Sẵn (Hybrid Recipes)**: 4 thẻ card 1-chạm nạp ngay các flow chuẩn đã qua kiểm định QC (Phim AI Trung Quốc, Vũ Đạo Douyin, Lookbook, Audio Truyện).
- **Thanh Biên Dịch Tự Nhiên (Natural Language to DAG)**: Nhập câu lệnh tùy ý, LLM Compiler tự động phân tích ý định và cấu trúc đồ thị tương ứng.
- **Visual Graph Canvas**: Sơ đồ khối trực quan hiển thị trạng thái từng Node theo thời gian thực (Xám `Pending` ⚪ -> Vàng `Running` 🟡 -> Xanh `Completed` 🟢 -> Cam `Fallback` 🟠).
- **Intermediate Artifacts Inspector**: Hiển thị ảnh/video trung gian tại mỗi chặng (Ảnh mẫu, Ảnh đổi đồ, Clip nhảy thô, Video khóa mặt, Video hoàn chỉnh).
- **Soul Index QC & Thất Pháp Video Có Hồn**: Tự động chấm điểm chất lượng (0-100) đảm bảo chống méo hình học, hòa âm 3-4 lớp, zero màn hình đen.

---

## 3. Mã Nguồn & Kiến Trúc Kỹ Thuật

- `internal/flow/types.go`: Định nghĩa `DataType`, `Pin`, `Node`, `Wire`, `Graph`, `GraphStatus`.
- `internal/flow/registry.go`: Catalog 25 Node Types chuẩn (Cinema Script, Actor Bank 360°, Kling Video, LivePortrait LipSync, Audio Ducking, Color LUT, QC, Export).
- `internal/flow/recipes.go`: Thư viện 4 Recipe chuẩn (Chinese Cinema Drama, Dance Trendjack, Lookbook Try-On, Audio Story Cinema).
- `internal/flow/compiler.go`: Phân tích ý định tự nhiên, sinh đồ thị DAG và gán tham số.
- `internal/flow/executor.go`: Kiểm tra chu trình (Cycle Detection), sắp xếp thứ tự phụ thuộc (Topological Sort), điều phối thực thi song song, quản lý fallback.
- `internal/flow/adapters_bridge.go`: Kết nối với `internal/webai` (Google Flow, ChatGPT, Claude, Gemini, Muse AI, Fashn Try-On, Viggle Dance) và local engines (InsightFace, VieNeu-TTS, FFmpeg).
- `internal/web/flow.go`: Web handlers và JSON APIs (`/api/flow/recipes`, `/api/flow/compile`, `/api/flow/run`, `/api/flow/status`).
- `internal/web/templates/flow.html`: Giao diện Visual Flow Studio.
