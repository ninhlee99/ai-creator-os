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
//   - Tải video không watermark: TikWM fallback ưu tiên lấy link sạch
//     không watermark (hdplay -> play) để phục vụ biên tập và remix video.
//     Khi tải qua TikWM, trạng thái video được đánh dấu "không watermark".
//     Trong pipeline transform, thông tin chủ sở hữu/tác giả gốc được
//     loại bỏ hoàn toàn khỏi kịch bản voiceover và mô tả bài đăng.
//
// Không tải thật trong test — mock yt-dlp bằng script giả, mock TikWM
// bằng httptest (BaseURL inject được).
package reup
