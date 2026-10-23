package web

import (
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (s *Server) handleBioTryOnAccounts(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		http.Error(w, "Kho Bio chưa sẵn sàng", http.StatusServiceUnavailable)
		return
	}
	location, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		location = time.FixedZone("ICT", 7*60*60)
	}
	day := time.Now().In(location).Format("2006-01-02")
	accounts, err := s.Products.ListBioGoogleAccountUsage(day, 200)
	data := map[string]any{
		"Accounts":     accounts,
		"Day":          day,
		"DailyLimit":   15,
		"ProductLimit": 3,
		"Error":        r.URL.Query().Get("err"),
		"Notice":       r.URL.Query().Get("ok"),
	}
	if err != nil {
		data["Error"] = "Không đọc được usage Google: " + err.Error()
	}
	s.render(w, "bio_tryon_accounts", data)
}

func (s *Server) handleBioTryOnAccountActive(w http.ResponseWriter, r *http.Request) {
	if s.Products == nil {
		http.Error(w, "Kho Bio chưa sẵn sàng", http.StatusServiceUnavailable)
		return
	}
	accountID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	activeValue := r.FormValue("active")
	if err != nil || accountID <= 0 || activeValue != "0" && activeValue != "1" {
		http.Error(w, "Yêu cầu không hợp lệ", http.StatusBadRequest)
		return
	}
	active := activeValue == "1"
	if err := s.Products.SetBioGoogleAccountActive(accountID, active); err != nil {
		http.Redirect(w, r, "/bio-admin/try-on-accounts?err="+url.QueryEscape("Không cập nhật được tài khoản Google"), http.StatusSeeOther)
		return
	}
	message := "Đã khóa tài khoản Google thử đồ"
	if active {
		message = "Đã mở lại tài khoản Google thử đồ"
	}
	http.Redirect(w, r, "/bio-admin/try-on-accounts?ok="+url.QueryEscape(message), http.StatusSeeOther)
}
