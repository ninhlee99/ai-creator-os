# Agent Harness — Autonomous Control Plane

AI Creator OS triển khai kiến trúc **Agent Harness (Autonomous Control Plane)** đạt chuẩn các hệ thống tự hành tiên tiến nhất trên thế giới.

---

## 1. Vị trí của Agent Harness trong Quy trình Tự động 24/7

Nhiều người dùng thắc mắc: *Harness Agent nằm ở khâu nào trong quy trình 24/7? Nó là một tính năng độc lập hay gắn liền với toàn hệ thống?*

**Câu trả lời chuẩn kiến trúc phần mềm:**
Hệ thống AI Creator OS được chia làm 2 tầng rõ rệt:

```
┌────────────────────────────────────────────────────────────────────────┐
│              COGNITIVE CONTROL PLANE (LỚP ĐIỀU KHIỂN NHẬN THỨC)        │
│                         ★ AGENT HARNESS (OODA) ★                       │
│  - Phân tích danh mục kênh (Portfolio State: views, followers, stages) │
│  - Kho Ký ức thực chiến (Episodic Memory Bank: Hook 3s, Conversion)    │
│  - Sinh Prompt Context tiêm vào các kịch bản của Studio                │
│  - Ra quyết định điều phối ưu tiên (Decision Stream: P1, P2...)        │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Directs & Enhances
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                 EXECUTION PLANE (LỚP THỰC THI 24/7 NỀN)                │
│                         ★ AUTOMATION TICKS ★                           │
│  - Hunter Tick (Accesstrade / Products Store)                          │
│  - Studio Engine (AI MediaGen 9:16 + Voiceover + FFMPEG Renderer)      │
│  - Autopilot Tick (Sản xuất video định kỳ 6h)                          │
│  - Publishers Gateway (Đăng tải lên TikTok, YouTube Shorts)            │
│  - Watchdog & Self-Healing (Tự phục hồi, dọn ổ đĩa, alert Telegram)   │
└────────────────────────────────────────────────────────────────────────┘
```

1. **Không phải tính năng rời rạc:** Harness Agent đóng vai trò **Bộ Não Chỉ Huy Cấp Cao (Strategic Commander)**, còn các Tick tự động (`AutopilotTick`, `GrowthTick`, `HunterTick`, `Studio`) là **Cánh Tay Thực Thi (Execution Plane)**.
2. **Khâu Tham Gia Cụ Thể:**
   - **Khâu Lập Kịch Bản (Script Generation):** Thông qua hook `srv.Studio.PromptEnhancer`, mỗi khi Studio tạo kịch bản video viral hoặc affiliate, nó tự động gọi `harness.Memory().GeneratePromptContext()` để tiêm các bài học giữ chân 3s và lỗi cần tránh vào LLM prompt.
   - **Khâu Kích Hoạt Đợt Sản Xuất (Job Dispatch):** Trong giai đoạn **Act** của chu kỳ OODA, Harness Agent trực tiếp gọi `studioInst.EnqueueViral(...)` để sinh các lượt dựng video mới cho kênh cần đẩy mạnh mà không cần người dùng tự bấm tạo từng video.
   - **Khâu Tối Ưu Hóa (Continuous Reflection):** Đọc lại dữ liệu chuyển đổi và hiệu quả video để lưu bài học mới vào `harness_memories`.

---

## 2. Kiểm Tra Dữ Liệu Lưu Trữ (Data Storage Inspection)

Harness Agent minh bạch 100% mọi dữ liệu được lưu trong SQLite:

1. **Bảng Chu Kỳ Nhận Thức (`harness_cycles`):**
   - Lưu trữ: `id`, `started_at`, `finished_at`, `diagnosis`, `observations_json`, `decisions_json`, `action_count`, `status`.
   - **Cycle Inspector Modal:** Nhấp vào bất kỳ chu kỳ nào để xem toàn bộ ảnh chụp môi trường (Perception Matrix: số lượng follower, stage kênh, tải hệ thống, âm thanh hot, sản phẩm hot) và danh sách quyết định hành động.
   - **Raw JSON Viewer:** Cho phép copy trực tiếp toàn bộ chuỗi JSON lưu trong cơ sở dữ liệu với nút `[ 📋 Copy Stored JSON ]`.
   - **API nội bộ:** `GET /api/harness/cycles/{id}` trả về JSON đầy đủ của chu kỳ.

2. **Bảng Ký Ức Thực Chiến (`harness_memories`):**
   - Lưu trữ: `id`, `memory_type`, `channel_id`, `niche`, `format_id`, `title`, `insight`, `confidence_score`, `impact_score`, `sample_size`, `source_ref`, `created_at`, `updated_at`.
   - Mỗi bản ghi có nút `[ { } JSON ]` để soi dữ liệu thô và nút `[ ✕ ]` để xóa nếu người dùng muốn loại bỏ bài học không phù hợp.

3. **Prompt Context Simulator (Trình Mô Phỏng Tiêm Ngữ Cảnh):**
   - Cho phép chọn ngách nội dung (Niche) và kênh (Channel).
   - Tức thì gọi API `GET /api/harness/prompt-context` để hiển thị chính xác đoạn prompt markdown mà LLM nhận được khi viết kịch bản.

---

## 3. 3 Cấp Độ Tự Trị (Autonomy Policy)

| Cấp độ | Tên gọi chuẩn | Mô tả hành vi |
|---|---|---|
| **Level 1** | **Assisted (Advisory)** | Agent chẩn đoán và đề xuất. Yêu cầu phê duyệt thủ công từng khâu sản xuất và xuất bản. |
| **Level 2** | **Semi-Autonomous (Draft-First)** | Tự động tạo kịch bản và render video nháp. Người dùng bấm duyệt trước khi xuất bản lên kênh thật. |
| **Level 3** | **Fully Autonomous (Zero-Touch)** | Tự vận hành 100%: Quản lý danh mục kênh, săn deal, sản xuất, đăng tải và tự sửa sai theo dữ liệu. |
