// Package reup là pipeline tải video viral Douyin (trụ reup của app,
// Đợt D) — download trước, transform + đăng ở Đợt E.
//
// Kiến trúc: discover (tìm video viral theo nguồn) → download (yt-dlp
// chính, TikWM fallback) → QC → lưu kho. Dedupe sha256 chống tải trùng.
//
// Trung thực phải đọc trước:
//
//   - yt-dlp có thể GÃY ĐỊNH KỲ khi Douyin đổi chữ ký (vài tuần/lần).
//     Manager tự tải binary mới nhất từ GitHub release vào <dataDir>/bin
//     (mẫu sidecar như models của engines/local — app KHÔNG bundle yt-dlp
//     vào binary, không thêm runtime/cgo), health check bằng --version,
//     phát hiện lỗi extractor → tự update một lần rồi thử lại.
//   - TikWM (https://www.tikwm.com) là API BÊN THỨ BA, không SLA, có thể
//     die bất cứ lúc nào. Link CDN có chữ ký, hết hạn sau vài phút —
//     fallback tải ngay lập tức, không lưu link chờ.
//   - yt-dlp KHÔNG có flag --no-watermark (không tồn tại trong yt-dlp
//     gốc) — không truyền flag bịa, vì sẽ làm mọi lượt tải lỗi "no such
//     option".
//   - RANH GIỚI CỨNG (Đợt I): mọi video tải về đều GIỮ watermark gốc.
//     yt-dlp giữ nguyên watermark Douyin; TikWM fallback CHỈ dùng link
//     wmplay (có watermark) — play/hdplay (no-watermark) không bao giờ
//     được dùng; Lookup thiếu wmplay → fail-closed. Provenance được ghi
//     trung thực từng video: nguồn tikwm → "có watermark"; nguồn yt-dlp
//     → "có thể có watermark" (không chắc, best effort).
//   - Reup vi phạm bản quyền nguyên tắc dù có transform — nguy cơ lớn
//     nhất 2026 là video 0-view/de-boost chứ không chỉ strike. Không
//     chỗ nào trong package này hứa "an toàn bản quyền".
//
// Không tải thật trong test — mock yt-dlp bằng script giả, mock TikWM
// bằng httptest (BaseURL inject được).
package reup
