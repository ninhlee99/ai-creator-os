# Đội ngũ (`/team`) — 3 pipeline

**Trạng thái: đã định nghĩa lại ở Đợt H2 (2026-10-04, commit `45c1466`).**
Route `/team` hoạt động bình thường — không còn park.

Mỗi trụ có một team chuyên trách, hiển thị dạng cây trạng thái (icon-first,
ít chữ). Trạng thái **suy ra từ dữ liệu thật** (studio jobs, kho AT/reup,
tick) — trang ghi rõ đây không phải agent đang chạy:
- **Affiliate team**: Hunter (quét AT datafeed → kho) → Producer (render video) → Publisher (video xong) → Analyst (đối soát hoa hồng)
- **Reup team**: Hunter (nguồn + video chờ tải) → Director (bài chờ transform) → Producer (đang transform) → QC (đạt/lỗi) → Publisher (đã đăng) → Analyst (bài lỗi + kill rule 0-view)
- **Story team**: Writer → Illustrator → Voice → Producer → Publisher (ánh xạ từ tiến độ job, ghi rõ ước lượng)

## Code ở đâu

- Handler: `internal/web/team_pipeline.go` — `handleTeamPipelines` (không build tag, vào binary).
- Template: `internal/web/templates/team.html` (+ CSS `.team-*` trong `static/style.css`).
- Sidebar: mục "Đội ngũ" (nhóm Vận hành).
- Test: `internal/web/team_pipeline_test.go` (+ cập nhật `settings_render_test.go` bỏ khẳng định `/team` còn park).

## Code cũ (park, để tham khảo)

- Handler live-era: `internal/web/team.go` — có `//go:build parked`.
- Template: `internal/web/templates_parked/team.html` (ngoài `go:embed`).
- Package agent cũ: `internal/agents/{analyst,content,governance,hunter,streamer}/`
  — `//go:build parked`.
- Thiết kế kiến trúc: `docs/AGENT_TEAM.md` (bản live — đã thay bằng thiết kế 3 pipeline trong `docs/PIVOT_REDESIGN.md` §1).

## Logic cũ (để tham khảo)

Trước khi park: trang vẽ cây trạng thái **suy ra từ tiến độ job Studio**
(`s.Studio.ListJobs(12)` → ánh xạ sang từng agent trong dây chuyền), có dòng
ghi rõ "không phải agent thật đang chạy" — trung thực, không bịa trạng thái.
