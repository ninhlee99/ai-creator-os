package web

// Publishers (multi-platform) page: connection status per account and
// the API usage-cost table.

import (
	"net/http"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

func (s *Server) publisherRowFor(username, ytChannel string, ytContentTypes []string, status string) publisherRow {
	if len(ytContentTypes) == 0 && fileExists(publishers.YouTubeTokenPath(username)) {
		ytContentTypes = []string{"short_video", "short_film"}
	}
	var plats []string
	for _, p := range publishers.BuildPublishers(username, ytChannel, ytContentTypes) {
		plats = append(plats, p.Name())
	}
	ytDead := false
	if s.Ledger != nil {
		if _, ok, _ := s.Ledger.GetSetting(tokenDeadKey(username)); ok {
			ytDead = true
		}
	}
	ttPub := publishers.NewTikTokPublisher(username)
	ttExists := fileExists(publishers.TikTokTokenPath(username))
	ttDead := false
	if s.Ledger != nil {
		if _, ok, _ := s.Ledger.GetSetting(tiktokTokenDeadKey(username)); ok {
			ttDead = true
		}
	}
	return publisherRow{
		Username:         username,
		Status:           status,
		TiktokToken:      ttExists,
		TiktokTokenDead:  ttDead,
		TiktokClient:     ttPub.HasClient(),
		YoutubeToken:     fileExists(publishers.YouTubeTokenPath(username)),
		YoutubeClient:    publishers.NewYouTubePublisher(username, nil, "").HasClient(),
		YoutubeTokenDead: ytDead,
		FbPage:           publishers.FacebookPageConfigured(username),
		FacebookClient:   s.facebookOAuthConfigured(),
		Instagram:        publishers.NewInstagramPublisher(username).IsConfigured(),
		Threads:          publishers.NewThreadsPublisher(username).IsConfigured(),
		Platforms:        plats,
	}
}

func (s *Server) handlePublishers(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.Mgr.List()
	if err != nil {
		s.fail(w, err, "list accounts")
		return
	}
	rows := make([]publisherRow, 0, len(accounts))
	for _, a := range accounts {
		rows = append(rows, s.publisherRowFor(a.Username, a.YoutubeChannel, a.YoutubeContentTypes, a.Status))
	}
	anyClient := false
	for _, row := range rows {
		anyClient = anyClient || row.TiktokClient || row.YoutubeClient || row.FacebookClient
	}
	s.render(w, "publishers", s.ctx("Rows", rows, "AnySocialClient", anyClient,
		"Msg", r.URL.Query().Get("msg"), "Err", r.URL.Query().Get("err"),
		"PendingVideo", r.URL.Query().Get("video"), "PendingTitle", r.URL.Query().Get("title"),
		"RedirectURI", publishers.TikTokRedirectURI(), "RedirectLocal", publishers.TikTokRedirectIsLocal(),
		"AppBase", "http://"+r.Host))
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
