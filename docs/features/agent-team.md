# Agent Team (`/team`)

## 1. Mục đích
Nhìn một đội ngũ "agent" đang làm việc trên từng tác vụ — Orchestrator trên đỉnh,
Governance/Scheduler gác cổng, dây chuyền Hunter → Director → Producer → QC →
Publisher → Analyst, Streamer nhánh riêng — với hiệu ứng trạng thái trực quan.

## 2. Kích hoạt
- `GET /team` → `handleTeam` (`internal/web/team.go:129`); chọn tác vụ bên trái
  (`?job=<id>`) để xem cây của tác vụ đó.

## 3. Luồng vận hành chi tiết
1. `teamTasks(r)` (`internal/web/team.go:113`): lấy tối đa 12 job mới nhất từ
   `s.Studio.ListJobs(12)` → mỗi job thành một `teamTask` (ID, tiêu đề, loại,
   trạng thái, % tiến độ, icon).
2. `agentsFor(status, progress)` ánh xạ trạng thái job → trạng thái từng agent
   trong dây chuyền (đang làm / chờ / xong / lỗi). Governance + Scheduler luôn
   ở trạng thái "guard" (gác cổng).
3. Template `team.html` vẽ cây + hiệu ứng CSS: đang làm viền cam nhấp nháy,
   luồng nhận việc chuyển động, xong tick xanh, lỗi dấu đỏ.
4. **Trung thực**: cây là **suy ra từ tiến độ job Studio**, không phải agent thật
   đang chạy — trang có dòng ghi rõ điều này. Chưa có job nào → empty state
   "Chưa có tác vụ nào" + nút "Mở Studio AI"; **không bao giờ** bịa task demo
   (quy tắc cứng từ Wave 1).

## 4. Fail-closed & an toàn
- Studio nil hoặc 0 job → empty state, không lỗi.
- Chỉ đọc tiến độ job; không điều khiển gì nên không cần cổng an toàn.

## 5. API key rotation
Không dùng key cloud.
