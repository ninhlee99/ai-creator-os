package web

// Home-page platform summary. These rows report local configuration only;
// they do not claim that a remote API, monetization program, or payout works.

import "github.com/ninhlee99/ai-creator-os/internal/network"

type moneyRow struct {
	Name  string
	Level string // "configured" still needs live verification; "partial" or "no" needs setup
	State string
	Next  string
	Link  string
}

func (s *Server) moneyReadiness() []moneyRow {
	var accounts []*network.Account
	if s.Mgr != nil {
		accounts, _ = s.Mgr.List()
	}
	return s.moneyReadinessForAccounts(accounts)
}

func (s *Server) moneyReadinessForAccounts(accounts []*network.Account) []moneyRow {
	rows := make([]publisherRow, 0)
	for _, a := range accounts {
		if a != nil {
			rows = append(rows, s.publisherRowFor(a.Username, a.YoutubeChannel, a.YoutubeContentTypes, a.Status))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, s.publisherRowFor("", "", nil, ""))
	}
	any := func(check func(publisherRow) bool) bool {
		for _, row := range rows {
			if check(row) {
				return true
			}
		}
		return false
	}
	row := func(name string, configured, partial bool, configuredState, partialState, missingState, next, link string) moneyRow {
		switch {
		case configured:
			return moneyRow{Name: name, Level: "configured", State: configuredState, Next: next, Link: link}
		case partial:
			return moneyRow{Name: name, Level: "partial", State: partialState, Next: next, Link: link}
		default:
			return moneyRow{Name: name, Level: "no", State: missingState, Next: next, Link: link}
		}
	}

	atKey := s.atKeySet()
	ytConfigured := any(func(p publisherRow) bool { return p.YoutubeClient && p.YoutubeToken && !p.YoutubeTokenDead })
	ytPartial := any(func(p publisherRow) bool { return p.YoutubeClient || p.YoutubeToken })
	ttConfigured := any(func(p publisherRow) bool { return p.TiktokClient && p.TiktokToken && !p.TiktokTokenDead })
	ttPartial := any(func(p publisherRow) bool { return p.TiktokClient || p.TiktokToken })
	fbConfigured := any(func(p publisherRow) bool { return p.FbPage })
	fbPartial := any(func(p publisherRow) bool { return p.FacebookClient })
	igConfigured := any(func(p publisherRow) bool { return p.Instagram })
	igPartial := any(func(p publisherRow) bool { return p.FbPage })
	threadsConfigured := any(func(p publisherRow) bool { return p.Threads })
	return []moneyRow{
		row("Hoa hồng Accesstrade", atKey && s.AT != nil, atKey,
			"Có API key và kho đơn cục bộ; publisher approval, API live và payout chưa xác minh.",
			"Có API key nhưng kho đơn chưa sẵn sàng.",
			"Chưa có API key Accesstrade.",
			"Kiểm tra publisher approval, hợp đồng, thông tin thuế và ngân hàng trong AccessTrade.", "/settings/accesstrade"),
		row("YouTube", ytConfigured, ytPartial,
			"Có OAuth client + token; quyền API chưa kiểm tra. Quảng cáo cần đủ điều kiện YPP.",
			"Thiếu OAuth client hoặc token; chưa xác nhận kênh có thể đăng.",
			"Chưa cấu hình YouTube.",
			"Kiểm tra kết nối kênh và điều kiện YPP.", "/publishers"),
		row("TikTok", ttConfigured, ttPartial,
			"Có client + token; mặc định đăng bản nháp. Direct Post và quyền duyệt app chưa xác minh.",
			"Thiếu client hoặc token; chưa xác nhận kênh có thể đăng.",
			"Chưa cấu hình TikTok.",
			"Duyệt bản nháp trong app TikTok; xác minh eligibility Direct Post trước khi bật đăng thẳng.", "/publishers"),
		row("Facebook Page", fbConfigured, fbPartial,
			"Có Page ID + token; quyền Page và đăng thật chưa kiểm tra.",
			"Có cấu hình Meta app nhưng chưa đủ Page ID + token.",
			"Chưa cấu hình Facebook Page.",
			"Kiểm tra quyền Page và thử luồng đăng an toàn.", "/settings/system"),
		row("Instagram Reels", igConfigured, igPartial,
			"Có ID + token; quyền publish chưa kiểm tra. Đăng công khai phụ thuộc IG_AUTO_PUBLISH.",
			"Có cấu hình Meta Page nhưng chưa đủ ID Instagram + token.",
			"Chưa cấu hình Instagram.",
			"Kiểm tra quyền Instagram và lựa chọn đăng công khai.", "/settings/system"),
		row("Threads", threadsConfigured, false,
			"Có user ID + token; hiện chỉ hỗ trợ bài chữ kèm link, chưa đăng video.",
			"", "Chưa cấu hình Threads.",
			"Video Threads cần Media Delivery Gateway; hiện chưa hỗ trợ publish video.", "/publishers"),
	}
}
