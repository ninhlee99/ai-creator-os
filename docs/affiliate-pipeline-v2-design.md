# Affiliate Video Pipeline v2 — Thiết kế cho app AICOS

Ngày: 2026-10-01. Người duyệt: Ninh.

## 1. Nguyên tắc từ Ninh (bất di bất dịch)

- Input duy nhất từ người dùng: **ảnh nhân vật** (khuôn mặt/ngoại hình) + **ảnh sản phẩm**.
- Mọi thứ còn lại tự động: viết kịch bản → góc quay → quay → dựng → ghép nhạc → ra video.
- Thời trang/phụ kiện: **chỉ quay video + ghép nhạc chill/trending, không cần nói** (không voiceover mặc định).
- **Không tự ý chèn chữ** lên video.
- Mỗi video tạo ra phải **xem lại được**: từng phân đoạn (shot), từng khung hình (frame scrubber), storyboard (ảnh + clip từng shot, trạng thái QC).
- Mọi asset (ảnh, clip, audio, bản final) **tự động lưu lên Google Drive**.
- Chạy trên máy Ninh (Mac M1 Pro, 32GB): nhẹ, ưu tiên API realtime miễn phí trước, local sau. Mọi chức năng có UI, không CLI.

## 2. Cách làm affiliate TikTok thật (đã nghiên cứu 2026)

- **Hook 2–3 giây đầu** phải dừng được ngón tay lướt: cảnh đẹp nhất, sản phẩm rõ nhất lên trước.
- **Sản phẩm là nhân vật chính**: demo dùng thật, aesthetic close-up + nhạc trending. Không cần mặt trong nhiều format, nhưng có mặt thì tăng trust.
- **Shoppertainment**: giải trí trước, bán hàng sau — video không được "mùi quảng cáo".
- **Nhạc trending** là đòn bẩy reach lớn nhất cho video không lời.
- 15–60 giây, dọc 9:16, **gắn giỏ hàng** vào mọi video (video không tag = 0 hoa hồng).
- Đăng đều 2–3 video/ngày, khung giờ 11:00–13:00 và 19:00–22:00.

## 3. Pipeline v2 (10 module)

```
Input → Director → Identity/Product Lock → Shoot → QC → Re-shoot (nếu rớt)
      → Edit → Music → Review UI → Publish → Drive Backup
```

### 3.1. Input
- Ảnh nhân vật (1–3 ảnh, ưu tiên ảnh rõ mặt).
- Ảnh sản phẩm (nhiều góc càng tốt — đây là **ground truth**: màu, logo, chất liệu, form, khóa).
- Niche (thời trang, mỹ phẩm, gia dụng...) + thời lượng mục tiêu (mặc định 30s).

### 3.2. Director (LLM)
Viết kịch bản + shot list theo công thức Hook → Value → CTA, tùy niche:
- **Thời trang/phụ kiện**: aesthetic showcase — hook cận sản phẩm trên người mẫu → lifestyle (dạo phố, café) → macro chất liệu → use case (đeo/cầm/sử dụng thật) → closing đẹp. Không voiceover.
- **Gia dụng/công nghệ**: problem → solution demo, có thể kèm voiceover.
- **Mỹ phẩm**: before/after, texture macro.
Mỗi shot: mô tả cảnh, góc máy, chuyển động, mood nhạc.

### 3.3. Identity Lock & Product Lock
- Mọi shot sinh từ ảnh nhân vật (giữ khuôn mặt) và ảnh sản phẩm (giữ đúng sản phẩm).
- Lưu hash identity + mô tả sản phẩm để đối chiếu.

### 3.4. Shoot
- Image-to-video từng shot (API video gen; local khi có model phù hợp).
- Thời lượng/clip theo kịch bản (mặc định 6s/shot cho video 30s).

### 3.5. QC tự động (chụp frame giữa clip để kiểm tra)
- Khuôn mặt có giữ đúng người không (so với ảnh gốc).
- Tay/chân có biến dạng không.
- Sản phẩm có đúng màu/logo/form không.
- Rớt → tự động quay lại shot đó (tối đa N lần), log lý do.

### 3.6. Edit (FFmpeg)
- Nối shot, fade in/out, pacing theo nhạc.
- Mặc định: **không chữ, không voiceover** (thời trang). Voiceover là tùy chọn bật trong UI.
- Xuất 1080×1920, 30fps.

### 3.7. Music
- Thư viện nhạc **có license sạch** (Mixkit/Pixabay: free commercial, no attribution), phân loại theo mood: chill, lofi, upbeat, luxury...
- Director gợi ý mood theo niche; user có thể đổi track trong UI.
- Khi publish: tùy chọn đổi sang **trending sound của TikTok** qua API/library của nền tảng.

### 3.8. Review UI (trang "Video" trong dashboard)
- **Storyboard**: lưới từng shot — ảnh keyframe, clip preview, trạng thái QC (đạt/rớt/quay lại), nút xem/duyệt/quay lại từng shot.
- **Frame scrubber**: kéo xem từng khung hình của từng shot.
- **Bản final**: xem trước, duyệt hoặc yêu cầu dựng lại.
- Lịch sử: mọi video đã tạo, kèm asset.

### 3.9. Publish
- Upload TikTok/Facebook/YouTube, **gắn giỏ hàng / product tag**.
- Lên lịch đăng theo khung giờ vàng.

### 3.10. Drive Backup (tự động, sau mỗi video)
- Tạo folder `AICOS/videos/<ngày>_<tên-sản-phẩm>/`.
- Upload: `shots/` (từng clip + keyframe), `audio/` (nhạc dùng), `final/` (bản final), `storyboard.json` (kịch bản + shot list + log QC).
- Dùng Google Drive API đã kết nối; chạy nền, báo trạng thái trong UI.

## 4. Kiến trúc trong app Go hiện tại

Thêm package `affiliate/`:
- `director/` — gọi LLM viết kịch bản/shot list (dùng chain Gemini hiện có).
- `shooter/` — gọi video-gen API, lưu clip.
- `qc/` — trích frame, so identity/product (dùng LLM vision hoặc heuristic).
- `editor/` — FFmpeg concat/fade/mix (đã có FFmpeg trong app).
- `music/` — thư viện track + metadata mood, download/cache local.
- `drivebackup/` — upload Drive sau khi xong.
- `store/` — SQLite: bảng `affiliate_videos`, `affiliate_shots` (shot, clip path, keyframe, qc_status, drive_link).
- Dashboard thêm trang **Video Affiliate**: form input (ảnh nhân vật, ảnh sản phẩm, niche) → storyboard review → final → publish; trang **Thư viện** xem lại video cũ.

Nhẹ RAM/CPU: render qua API, máy local chỉ chạy FFmpeg + dashboard.

## 5. Roadmap

- **v2.1 (hiện tại)**: chạy thủ công theo pipeline trên — đã chứng minh với spot túi kem 30s.
- **v2.2**: gom thành 1 nút "Tạo video" trong dashboard + Review UI (storyboard/frame scrubber).
- **v2.3**: QC tự động + re-shoot, thư viện nhạc trong app.
- **v2.4**: Drive backup tự động + publish gắn giỏ hàng + lịch đăng.
