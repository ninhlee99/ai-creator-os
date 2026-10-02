package studio

import "context"

// ShotQCChecker kiểm tra một shot đã render có giữ đúng identity/bối cảnh
// không (chống drift khuôn mặt qua nhiều shot — vấn đề Ninh quan tâm nhất).
// Trả về nil khi đạt, error mô tả lý do khi rớt (caller sẽ render lại shot,
// tối đa do caller quyết).
//
// TRUNG THỰC: bản mặc định là NoopQCChecker — KHÔNG kiểm tra thật. Vòng QC
// hiện tại là storyboard UI (duyệt từng shot bằng mắt) + nút "Quay lại shot
// này". Khi nào có thuật toán so khớp khuôn mặt khả thi (embedding local
// hoặc API) thì implement interface này, không bịa.
type ShotQCChecker interface {
	// CheckShot nhận keyframe (ảnh) và clip (mp4) của shot cùng ảnh chân
	// dung gốc của nhân vật chính để đối chiếu.
	CheckShot(ctx context.Context, portraitPath, keyframePath, clipPath string) error
}

// NoopQCChecker là QC giả — luôn đạt. Dùng cho tới khi có thuật toán thật.
// Mọi log/render phải ghi rõ "QC: chưa kiểm tra tự động" thay vì giả vờ đạt.
type NoopQCChecker struct{}

func (NoopQCChecker) CheckShot(_ context.Context, _, _, _ string) error { return nil }
