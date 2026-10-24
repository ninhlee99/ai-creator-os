package web

type moneyRow struct {
	Name  string
	Level string
	State string
	Next  string
	Link  string
}

func (s *Server) moneyReadiness() []moneyRow {
	accountRows, _ := s.Mgr.List()
	rows := make([]publisherRow, 0, len(accountRows))
	for _, account := range accountRows {
		rows = append(rows, s.publisherRowFor(account.Username, account.Status))
	}
	if len(rows) == 0 {
		rows = append(rows, s.publisherRowFor("", ""))
	}

	tiktokReady, tiktokPartial := false, false
	for _, row := range rows {
		tiktokReady = tiktokReady || (row.TiktokClient && row.TiktokToken && !row.TiktokTokenDead)
		tiktokPartial = tiktokPartial || row.TiktokClient || row.TiktokToken
	}

	atKey := s.atKeySet()
	return []moneyRow{
		readinessRow("TikTok", tiktokReady, tiktokPartial,
			"Có client và token; đăng Draft mặc định, quyền public chưa xác minh.",
			"Thiếu client hoặc token.", "Chưa cấu hình TikTok.",
			"Kết nối account và duyệt Draft trong TikTok.", "/publishers"),
		readinessRow("AccessTrade", atKey && s.AT != nil, atKey,
			"Có API key và store cục bộ; publisher approval và API live cần xác minh.",
			"Có API key nhưng store chưa sẵn sàng.", "Chưa có AccessTrade API key.",
			"Kết nối AccessTrade để tìm offer và đồng bộ đơn.", "/settings/accesstrade"),
	}
}

func readinessRow(name string, configured, partial bool, configuredState, partialState, missingState, next, link string) moneyRow {
	switch {
	case configured:
		return moneyRow{Name: name, Level: "configured", State: configuredState, Next: next, Link: link}
	case partial:
		return moneyRow{Name: name, Level: "partial", State: partialState, Next: next, Link: link}
	default:
		return moneyRow{Name: name, Level: "no", State: missingState, Next: next, Link: link}
	}
}
