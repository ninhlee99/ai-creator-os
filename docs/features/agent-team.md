# 🅿️ PARKED — Agent Team (`/team`)

**Trạng thái: đã park từ Đợt A (commit `efa218b`, 2026-10-02).** Route
`/team` không còn trong binary → mở trang này trả **404**.

**Không xóa hẳn** — `docs/PIVOT_REDESIGN.md` §1 chốt **định nghĩa lại** Agent
Team quanh 3 pipeline mới (thay vì team phục vụ live):
- **Affiliate team**: Hunter (quét AT datafeed) → Producer (render video) → Publisher (đăng) → Analyst (đối soát hoa hồng)
- **Reup team**: Hunter (tìm video viral) → Director (kế hoạch transform + voiceover) → Producer (FFmpeg) → QC → Publisher → Analyst (view/flag)
- **Story team**: Writer (viết truyện) → Illustrator (vẽ ảnh) → Voice (giọng đọc) → Producer → Publisher

## Code còn lại ở đâu

- Handler: `internal/web/team.go` — có `//go:build parked`.
- Template: `internal/web/templates_parked/team.html` (ngoài `go:embed`).
- Package agent cũ: `internal/agents/{analyst,content,governance,hunter,streamer}/`
  — `//go:build parked`.
- Thiết kế kiến trúc: `docs/AGENT_TEAM.md` (bản live — sẽ viết lại khi định nghĩa lại team).

## Logic cũ (để tham khảo)

Trước khi park: trang vẽ cây trạng thái **suy ra từ tiến độ job Studio**
(`s.Studio.ListJobs(12)` → ánh xạ sang từng agent trong dây chuyền), có dòng
ghi rõ "không phải agent thật đang chạy" — trung thực, không bịa trạng thái.
