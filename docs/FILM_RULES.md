# Luật làm phim — AI Creator OS (bất di bất dịch)

Mọi thay đổi pipeline phim phải tôn trọng các luật dưới đây. Luật nào bị phá
vỡ phải được sửa ngay, không "để sau".

## 1. Chuẩn điện ảnh
- **Mọi phim xuất 16:9** (chuẩn điện ảnh). Không scale, không quay lại.
- **Trailer dọc 9:16** được cắt tự động từ shot `trailer_worthy` bằng
  center-crop (`CropCenterVertical`). Tradeoff trung thực: crop giữ lại ~32%
  chiều ngang (~68% điểm ảnh ngang bị bỏ), rồi upscale ~1.78x lên 1080×1920 —
  trailer mềm hơn phim gốc, chấp nhận được vì xem trên điện thoại.

## 2. Kịch bản 3 hồi (đạo diễn + prompt)
- **Hook trong 60s đầu** (phim ngắn: 15s đầu). Không hook = kịch bản hỏng.
- **Bước ngoặt giữa phim**: bí mật/twist đổi hướng câu chuyện.
- **Cao trào + kết**: đối đầu lớn nhất rồi hạ nhiệt về kết thúc trọn vẹn,
  nhân vật thay đổi rõ rệt, dư âm cảm xúc (không kết cụt).
- Mỗi shot: **hành động + cảm xúc CỤ THỂ** (tiếng Việt). Cấm chung chung kiểu
  "cô ấy buồn" — phải viết "cô ấy siết chặt quai túi, mắt ráo hoảnh nhìn ra cửa".
- Kịch bản theo đúng **thể loại** đã chọn (tình tiết, nhịp, cảm xúc).

## 3. Nhất quán (identity lock + continuity bible)
- Chân dung nhân vật vẽ **một lần**, làm reference cho mọi lần sinh ảnh.
- Mọi prompt ảnh/video gắn nguyên khối **CHARACTER LOCK** + **SCENE LOCK**
  (địa điểm/thời gian/ánh sáng khóa cứng theo cảnh).
- **Continuity bible**: mỗi scene có một khối bất di bất dịch — trang phục chi
  tiết từng nhân vật, kiểu tóc, đạo cụ, hướng ánh sáng, thời tiết. MỌI shot
  trong scene kế thừa **nguyên văn** vào prompt, không diễn đạt lại.
- Shot sau nối mạch bằng **frame cuối của shot trước** (firstFrame chaining).
- QC tự động hiện tại là **no-op trung thực** — vòng QC thật là storyboard UI
  (duyệt từng shot bằng mắt) + nút "Quay lại shot này".

## 3b. Diễn xuất từng shot (Veo không được đoán)
- Prompt Veo của mỗi shot bắt buộc có khối **PERFORMANCE**: biểu cảm khuôn mặt
  + hành động cụ thể (từ action của kịch bản) và **EMOTIONS** (từ emotion của
  từng câu thoại). Không để Veo tự đoán diễn xuất.
- **Đa dạng shot chống nhàm**: không lặp cùng shot_size + camera_move quá 2
  shot liên tiếp; mỗi cảnh có ít nhất 1 wide (WS/EWS), 1 close-up (CU/ECU/MCU),
  1 movement shot (camera_move khác static).

## 3c. Thoại trung thực — KHÔNG có lip-sync thật (giới hạn kỹ thuật hiện tại)
- Pipeline hiện tại phủ giọng TTS lên clip Veo — **không có lip-sync**.
  Không hứa, không ghi nhãn gây hiểu nhầm.
- Vì vậy director ưu tiên: **lời dẫn narration + thoại ngoài hình**
  (off-camera — nhân vật không lộ mặt khi nói); thoại trực diện tối thiểu.
- Khi có thoại trực diện: xen **cutaway/reaction shot** (bàn tay, ánh mắt
  người nghe, chi tiết bối cảnh) giữa các câu thoại — ngữ pháp điện ảnh chuẩn,
  vừa hay vừa che điểm yếu AI.

## 4. Center-safe cho trailer
- Shot nào `trailer_worthy=true` thì nhân vật/hành động chính **phải nằm
  trong vùng 9:16 ở giữa khung hình 16:9** — crop dọc sẽ không cắt mất nội dung.

## 5. Đơn vị render
- **1 shot = tối đa 8s = đúng 1 lần gọi Veo 3.** Không bao giờ gửi quá 8s.
- Phim dài (>180s) viết kịch bản theo **3 hồi, mỗi hồi một lần gọi LLM**
  (vừa ép cấu trúc, vừa không vượt giới hạn response).

## 6. Trung thực tuyệt đối
- Veo lỗi → fallback ảnh + giọng đọc, **không bịa clip**. Log ghi rõ từng
  shot render bằng gì (🎬 Veo / 🖼 Ảnh + giọng đọc).
- Mọi con số tiền đều ghi **"ước tính chưa kiểm chứng"** cho tới khi Ninh
  xác nhận giá thật. Vượt trần chi phí → dừng job sạch sẽ.
- Upscale là **lanczos, không phải 4K native** — UI ghi rõ.
- Không chữ trên hình, không watermark (trừ phụ đề mov_text của phim).

## 7. Phạm vi
- Affiliate giữ phong cách riêng: **không chữ, không voiceover, nhạc only**.
- Shorts/TikTok = **trailer cắt từ phim**, không sản xuất riêng.
- Phụ đề tiếng Việt mux sẵn trong file phim cuối.
- **Giữ 30fps** cho mọi render (không 60fps) — quyết định của Ninh 2026-10-02.
