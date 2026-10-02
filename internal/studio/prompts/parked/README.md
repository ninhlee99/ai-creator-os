# Prompt đã park (PIVOT 2026-10-02)

Các prompt của pipeline phim ngắn/điện ảnh đã park cùng code
(`internal/studio/film*.go`, build tag `parked`):

- `screenplay.txt` — pha 1: chuyển thể truyện thành kịch bản
- `film_act1.txt`, `film_act2.txt`, `film_act3.txt` — kịch bản 3 hồi
- `breakdown.txt` — pha 2: trích character bible + shot list
- `film_keyframe.txt` — prompt vẽ keyframe điện ảnh

Đặt trong thư mục con `parked/` để tách khỏi prompt đang dùng.
`//go:embed` nhúng cả hai (`prompts/*.txt` + `prompts/parked/*.txt`) và
`directorPrompt` tự fallback sang `prompts/parked/` khi không tìm thấy ở
`prompts/` — nên build `-tags parked` vẫn chạy được pipeline phim nếu cần.

GIỮ ở `prompts/`:
- `story.txt` — viết truyện (trụ Kể chuyện, đợt F dùng lại)
- `affiliate_shotlist.txt` — shot list video affiliate
