package web

// Publishers (multi-platform) page: connection status per account and
// the API usage-cost table.

import (
	"net/http"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

func (s *Server) handlePublishers(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	rows := make([]publisherRow, 0, len(accounts))
	for _, a := range accounts {
		var plats []string
		for _, p := range publishers.BuildPublishers(a.Username, a.YoutubeChannel, a.YoutubeContentTypes) {
			plats = append(plats, p.Name())
		}
		rows = append(rows, publisherRow{
			Username:     a.Username,
			Status:       a.Status,
			TiktokToken:  fileExists(publishers.TikTokTokenPath(a.Username)),
			TiktokClient: publishers.NewTikTokPublisher(a.Username).HasClient(),
			YoutubeToken: fileExists(publishers.YouTubeTokenPath(a.Username)),
			FbPage:       strings.TrimSpace(getenv("FB_PAGE_ID_"+strings.ToUpper(a.Username), "")) != "",
			Rtmp:         a.RtmpKey() != "",
			Platforms:    plats,
		})
	}
	anyClient := false
	for _, row := range rows {
		anyClient = anyClient || row.TiktokClient
	}
	s.render(w, "publishers", s.ctx("Rows", rows, "AnyTiktokClient", anyClient,
		"Msg", r.URL.Query().Get("msg"), "Err", r.URL.Query().Get("err"),
		"RedirectURI", publishers.TikTokRedirectURI(), "RedirectLocal", publishers.TikTokRedirectIsLocal()))
}

// --------------------------------------------------------------------- usage

// usageRow is one engine/provider API-cost aggregate (Settings ▸ Trạng
// thái hệ thống — trang /analytics cũ đã tan vào đây, Đợt 2).
type usageRow struct {
	Engine   string
	Provider string
	N        int64
	Cost     float64
}

func (s *Server) queryUsage() ([]usageRow, error) {
	rows, err := s.db.Query(
		`SELECT engine, provider, COUNT(*) n, COALESCE(SUM(cost_usd),0) cost
		 FROM api_usage GROUP BY engine, provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []usageRow
	for rows.Next() {
		var u usageRow
		if err := rows.Scan(&u.Engine, &u.Provider, &u.N, &u.Cost); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------- settings
