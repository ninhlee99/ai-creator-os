package web

// "Sẵn sàng kiếm tiền" trên Trang chủ: mỗi nguồn tiền đang ở đâu, còn
// thiếu đúng bước gì. Chỉ đọc cấu hình/token thật — không đoán doanh thu.

import "github.com/ninhlee99/ai-creator-os/internal/publishers"

type moneyRow struct {
	Name  string
	Level string // "ok" | "warn" | "no"
	State string // đang ở đâu
	Next  string // bước kế tiếp ("" khi không còn gì phía app)
	Link  string
}

func (s *Server) moneyReadiness() []moneyRow {
	var user string
	if s.Mgr != nil {
		if accts, err := s.Mgr.List(); err == nil && len(accts) > 0 {
			user = accts[0].Username
		}
	}
	row := func(name string, ok, partial bool, okState, partState, noState, next, link string) moneyRow {
		switch {
		case ok:
			return moneyRow{Name: name, Level: "ok", State: okState}
		case partial:
			return moneyRow{Name: name, Level: "warn", State: partState, Next: next, Link: link}
		}
		return moneyRow{Name: name, Level: "no", State: noState, Next: next, Link: link}
	}
	ytClient := publishers.NewYouTubePublisher(user, nil, "").HasClient()
	ytToken := user != "" && fileExists(publishers.YouTubeTokenPath(user))
	ttClient := publishers.NewTikTokPublisher(user).HasClient()
	ttToken := user != "" && fileExists(publishers.TikTokTokenPath(user))
	return []moneyRow{
		row("Hoa hồng Accesstrade", s.atKeySet(), false,
			"Đã nối mã — app tự tạo link theo dõi và đối soát đơn", "",
			"Chưa nối mã Accesstrade — video chưa có link ra tiền",
			"Nhập mã truy cập Accesstrade", "/settings/accesstrade"),
		row("YouTube (Shorts + video dài)", ytToken, ytClient,
			"Đã nối kênh — đăng private + link affiliate trong mô tả; quảng cáo cần đủ YPP (theo dõi ở Phát triển kênh)",
			"Đã có Client ID — còn bấm “Nối YouTube”",
			"Chưa nối YouTube", "Nối YouTube ở Đa nền tảng", "/publishers"),
		row("TikTok", false, ttToken || ttClient,
			"", "Chỉ đăng bản nháp — bạn bấm Đăng trong app TikTok (đăng thẳng cần TikTok duyệt app)",
			"Chưa nối TikTok", "Nối TikTok ở Đa nền tảng", "/publishers"),
		row("Facebook Page", publishers.NewFacebookPublisher(user).IsConfigured(), false,
			"Đã nối — video tải lên chưa công khai để bạn duyệt", "",
			"Chưa nối Trang Facebook", "Nhập ID Trang + mã truy cập ở Cài đặt", "/settings/he-thong"),
		row("Instagram Reels", false, publishers.NewInstagramPublisher(user).IsConfigured(),
			"", "Đã nối — chỉ tải lên, chưa đăng (Instagram không có nháp; bật “Instagram: tự đăng công khai” nếu muốn)",
			"Chưa nối Instagram", "Nhập ID Instagram + mã truy cập ở Cài đặt", "/settings/he-thong"),
		row("Threads", publishers.NewThreadsPublisher(user).IsConfigured(), false,
			"Đã nối — đăng bài kèm link sản phẩm/video", "",
			"Chưa nối Threads", "Nhập ID + mã truy cập Threads ở Cài đặt", "/settings/he-thong"),
	}
}
