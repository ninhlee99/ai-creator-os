package web

// TikTok connection status per account and the API usage-cost table.

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

func (s *Server) publisherRowFor(username, status string) publisherRow {
	ttPub := publishers.NewTikTokPublisher(username)
	ttExists := fileExists(publishers.TikTokTokenPath(username))
	ttDead := false
	if s.Ledger != nil {
		if _, ok, _ := s.Ledger.GetSetting(tiktokTokenDeadKey(username)); ok {
			ttDead = true
		}
	}
	return publisherRow{
		Username:        username,
		Status:          status,
		TiktokToken:     ttExists,
		TiktokTokenDead: ttDead,
		TiktokClient:    ttPub.HasClient(),
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
		row := s.publisherRowFor(a.Username, a.Status)
		row.AccountID = a.ID
		rows = append(rows, row)
	}
	rawVideo := r.URL.Query().Get("video")
	pendingVideo := s.pendingVideoURL(rawVideo)
	errMsg := r.URL.Query().Get("err")
	errDetail := ""
	if strings.Contains(errMsg, "lookup open.tiktokapis.com") {
		errDetail = errMsg
		errMsg = "Không phân giải được máy chủ TikTok trong môi trường chạy ứng dụng. Khởi động lại bản mới; nếu lỗi còn, kiểm tra DNS của máy hoặc container."
	} else if strings.Contains(errMsg, "x509: OSStatus -26276") {
		errDetail = errMsg
		errMsg = "Go không xác minh được chứng chỉ qua macOS. Nếu ứng dụng đang chạy trong Codex/IDE sandbox, hãy mở ứng dụng từ Terminal thường rồi đăng nhập lại."
	}
	if rawVideo != "" && pendingVideo == "" && errMsg == "" {
		errMsg = "Video không hợp lệ hoặc không còn trên máy."
	}
	s.render(w, "publishers", s.ctx("Rows", rows,
		"Msg", r.URL.Query().Get("msg"), "Err", errMsg, "ErrDetail", errDetail,
		"PendingVideo", pendingVideo, "PendingTitle", r.URL.Query().Get("title"),
	))
}

// pendingVideoURL accepts only an existing local video served by /media/{name}.
// The query parameter must never turn the download button into an external link.
func (s *Server) pendingVideoURL(raw string) string {
	const prefix = "/media/"
	if strings.HasPrefix(raw, "media/") { // old in-app player links
		raw = "/" + raw
	}
	if !strings.HasPrefix(raw, prefix) {
		return ""
	}
	name := strings.TrimPrefix(raw, prefix)
	ext := strings.ToLower(filepath.Ext(name))
	if !mediaNameRe.MatchString(name) || (ext != ".mp4" && ext != ".mov" && ext != ".webm") || !fileExists(filepath.Join(s.OutDir, name)) {
		return ""
	}
	return prefix + name
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
