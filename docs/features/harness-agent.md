# Bộ Não Tự Hành Cấp Cao (Autonomous Agent Harness)

AI Creator OS triển khai kiến trúc **Autonomous Agent Harness** đạt chuẩn các hệ thống tự hành tiên tiến nhất trên thế giới.

---

## 1. Bản Chất của Agent Harness

Một hệ thống tự động hóa truyền thống (như Cron Job hay Simple Pipeline) chỉ thực thi các mệnh lệnh cố định theo kịch bản có sẵn. Nếu video bị flop hoặc thuật toán nền tảng thay đổi, hệ thống cũ sẽ mù quáng lặp lại kịch bản thất bại.

Ngược lại, **Agent Harness** hoạt động như một **Bộ Não Điều Hành Khép Kín (Closed-Loop Cognitive Architecture)**:
1. **Quan sát (Observe):** Thu thập liên tục số liệu sức khỏe của mọi kênh, độ giữ chân người xem (Hook 3s), tỷ lệ hoàn thành video, chuyển đổi affiliate và xu hướng thị trường.
2. **Định hướng (Orient):** Đối chiếu với kho ký ức dài hạn (Episodic Memory), nhận diện điểm nghẽn, phát hiện format nội dung đang suy thoái hoặc cơ hội bứt phá.
3. **Quyết định (Decide):** Tự động lập kế hoạch đa kênh, ra lệnh điều phối 9 khối chuyên môn AI (Nội dung, Đạo diễn, Săn sản phẩm, Xuất bản, v.v.).
4. **Hành động (Act):** Tự động sản xuất video gốc (AI MediaGen, TTS giọng đọc truyền cảm, ghép nhạc xu hướng) và xuất bản lên các nền tảng (TikTok, YouTube Shorts).
5. **Đúc kết (Reflect & Learn):** Đánh giá kết quả của chu kỳ, rút ra bài học kinh nghiệm (Insights) và nạp ngược lại vào kho ký ức để các chu kỳ sau thông minh hơn.

---

## 2. 3 Cấp Độ Tự Hành (Autonomy Policy)

Doanh chủ nắm quyền quyết định mức độ can thiệp thông qua bảng điều khiển `/harness`:

| Cấp độ | Tên gọi | Phạm vi hoạt động của Agent | Mức độ can thiệp của con người |
|---|---|---|---|
| **Cấp độ 1** | **Trợ lý tư vấn (Assisted)** | Giám sát, chẩn đoán, đề xuất kế hoạch sản xuất và kịch bản. | Con người bấm duyệt từng video trước khi render và xuất bản. |
| **Cấp độ 2** | **Bán tự động (Semi-Autonomous)** | Tự động nghiên cứu xu hướng, render video demo sẵn sàng. | Con người kiểm tra chất lượng lần cuối và bấm xuất bản lên kênh chính thức. |
| **Cấp độ 3** | **Tự hành toàn diện (Fully Autonomous)** | Chủ động 100%: Quản lý danh mục kênh, săn deal affiliate, dựng video, đăng bài, đo lường và tự sửa sai. | Không cần can thiệp. Doanh chủ chỉ theo dõi doanh thu và báo cáo nhận thức. |

---

## 3. Kho Ký Ức Thực Chiến (Episodic & Semantic Memory)

Được lưu trữ bền vững trong bảng `harness_memories` (SQLite):
- **Hook Performance (`hook_performance`):** Đúc kết cấu trúc mở đầu 3s giữ chân người xem tốt nhất theo từng ngách (câu hỏi tò mò, khẳng định gây sốc, cắt cảnh gián đoạn).
- **Conversion Insight (`conversion_insight`):** Học cách gắn link bio, CTA và sản phẩm có tỷ lệ hoa hồng / đơn hàng thực tế cao nhất.
- **Format Reflection (`format_reflection`):** Nhận diện các format đã bão hòa để khai tử (kill) và nhân đôi (double-down) các format chiến thắng.
- **Channel Learning (`channel_learning`):** Tối ưu phong cách và nhịp độ video riêng biệt cho từng kênh.

Mỗi khi lên kịch bản mới, Harness tự động kích hoạt `GeneratePromptContext()` để inject các bài học này vào LLM Prompt, đảm bảo video sau luôn hay hơn video trước.

---

## 4. Giao Diện Điều Hành `/harness`

- **Executive Cockpit:** Xem chẩn đoán nhận thức thời gian thực, thời lượng chu kỳ và số quyết định hành động.
- **Decision Stream Table:** Xem chi tiết từng quyết định do AI đưa ra (Kênh phụ trách, Loại hành động, Căn cứ chiến lược, Độ ưu tiên P1-P5, Trạng thái).
- **Memory Bank:** Quản lý danh mục kinh nghiệm và cho phép doanh chủ nạp thêm các chỉ đạo / triết lý kinh doanh riêng biệt.
- **Nút Kích Hoạt Nhanh:** `⚡ Kích hoạt Chu kỳ Nhận thức OODA ngay` cho phép chạy chu kỳ kiểm tra và sản xuất ngay lập tức mà không cần đợi ticker định kỳ.
