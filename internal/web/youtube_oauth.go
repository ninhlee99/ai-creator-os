package web

// YouTube OAuth connect flow trong app (Đợt AA, finding C1).
//
// Trước đây đường duy nhất lấy refresh token là script Python
// scripts/get-youtube-token.py chạy ở terminal — vi phạm rule "mọi khả
// năng đều có UI". Flow ở đây theo mẫu TikTok (authorize → callback),
// dùng OAuth client loại "Desktop app" của Google:
//
//   - redirect_uri là loopback (http://127.0.0.1:<cổng>/publishers/youtube/callback):
//     Google cho phép redirect loopback với Desktop-app client mà không
//     cần khai báo trước.
//   - scope: youtube.upload + yt-analytics.readonly (giống script).
//   - access_type=offline + prompt=consent để luôn nhận refresh_token.
//   - token ghi vào youtube_token_<username>.json dạng {"refresh_token": "..."}
//     (đúng định dạng script + publishers đọc), quyền 0600.
//   - state chống CSRF tái dùng oauthStates của TikTok (TTL 15 phút).

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/network"
	"github.com/ninhlee99/ai-creator-os/internal/publishers"
)

// googleOAuthClient — client riêng có timeout cho mọi gọi tới Google OAuth.
// http.DefaultClient không timeout: Google treo là handler treo theo.
var googleOAuthClient = &http.Client{Timeout: 30 * time.Second}

const (
	youtubeOAuthAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	youtubeOAuthTokenURL = "https://oauth2.googleapis.com/token"
	youtubeOAuthScope    = "https://www.googleapis.com/auth/youtube.upload " +
		"https://www.googleapis.com/auth/youtube.readonly " +
		"https://www.googleapis.com/auth/yt-analytics.readonly"
)

// youtubeRedirectURI dựng callback URL loopback từ request hiện tại.
// App chạy local nên bắt buộc mở qua 127.0.0.1/localhost — nếu mở qua
// hostname/IP khác, Google sẽ từ chối redirect nên báo rõ ngay.
func youtubeRedirectURI(r *http.Request) (string, error) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return "", fmt.Errorf("mở app qua http://127.0.0.1 (đúng cổng đang chạy; đang mở qua %s) rồi bấm lại “Nối YouTube” — Google chỉ cho phép quay về địa chỉ máy local", r.Host)
	}
	return "http://" + r.Host + "/publishers/youtube/callback", nil
}

// handleYouTubeAuthorize đưa trình duyệt tới trang consent của Google
// cho một tài khoản hoặc để tạo kênh mới trực tiếp từ OAuth.
func (s *Server) handleYouTubeAuthorize(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	fromNew := false
	if username == "" {
		username = "__new_channel__"
		fromNew = true
	} else if !s.accountExists(username) {
		publishersMsg(w, r, "err", "Không có tài khoản "+username)
		return
	}
	p := publishers.NewYouTubePublisher(username, nil, "")
	if !p.HasClient() {
		msg := "Thiếu YOUTUBE_CLIENT_ID / YOUTUBE_CLIENT_SECRET — nhập ở Cài đặt · Hệ thống, mục “Mã & biến cấu hình”."
		if fromNew {
			seeOther(w, r, "/accounts/new?err="+url.QueryEscape(msg))
			return
		}
		publishersMsg(w, r, "err", msg)
		return
	}
	redirect, err := youtubeRedirectURI(r)
	if err != nil {
		if fromNew {
			seeOther(w, r, "/accounts/new?err="+url.QueryEscape(err.Error()))
			return
		}
		publishersMsg(w, r, "err", err.Error())
		return
	}
	q := url.Values{
		"client_id":     {p.ClientID()},
		"redirect_uri":  {redirect},
		"response_type": {"code"},
		"scope":         {youtubeOAuthScope},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {s.oauth.issueWithRedirect(username, redirect)},
	}
	http.Redirect(w, r, youtubeOAuthAuthURL+"?"+q.Encode(), http.StatusFound)
}

// handleYouTubeCallback nhận code từ Google, đổi lấy refresh token và
// ghi file token. Xóa cờ token_dead nếu có (finding C2).
func (s *Server) handleYouTubeCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		publishersMsg(w, r, "err", "Google từ chối: "+e+" "+q.Get("error_description")+
			" — bấm lại “Nối YouTube” để thử lại (nhớ chọn đúng kênh rồi bấm Cho phép).")
		return
	}
	code := q.Get("code")
	st, ok := s.oauth.take(q.Get("state"))
	if !ok {
		publishersMsg(w, r, "err", "state không khớp hoặc đã hết hạn (15 phút) — bấm lại “Nối YouTube”.")
		return
	}
	if code == "" {
		publishersMsg(w, r, "err", "Không có ?code= — bấm lại “Nối YouTube”.")
		return
	}

	p := publishers.NewYouTubePublisher(st.username, nil, "")
	refresh, accessToken, err := exchangeGoogleCode(p.ClientID(), p.ClientSecret(), code, st.redirectURI)
	if err != nil {
		publishersMsg(w, r, "err", "Đổi code lấy token thất bại: "+err.Error())
		return
	}

	targetUsername := st.username
	var targetAccountID int64

	// Khi kết nối từ màn hình Thêm kênh mới (__new_channel__)
	if targetUsername == "__new_channel__" {
		chInfo, err := fetchGoogleChannelInfo(accessToken)
		if err != nil {
			seeOther(w, r, "/accounts/new?err="+url.QueryEscape("Không đọc được kênh YouTube thật: "+err.Error()))
			return
		}
		candidateName := sanitizeUsername(chInfo.Title)
		if candidateName == "" {
			candidateName = fmt.Sprintf("yt_%s", strings.ToLower(chInfo.ID))
		}
		baseName := candidateName
		counter := 1
		for s.accountExists(candidateName) {
			counter++
			candidateName = fmt.Sprintf("%s_%d", baseName, counter)
		}
		acct, err := s.Mgr.Add(candidateName, "kênh YouTube thật đã xác thực", "")
		if err != nil {
			seeOther(w, r, "/accounts/new?err="+url.QueryEscape("Không tạo được kênh từ YouTube: "+err.Error()))
			return
		}
		targetUsername = candidateName
		targetAccountID = acct.ID
		if s.Mgr != nil {
			_, _ = s.Mgr.SetYoutube(acct.ID, chInfo.ID, []string{"short_video", "short_film"})
		}
		if s.Ledger != nil {
			_ = s.Ledger.SetAccountStatus(acct.ID, acct.Status, map[string]any{"youtube_channel": chInfo.ID})
			if chInfo.Subs > 0 {
				_ = s.Ledger.SetFollowers(acct.ID, chInfo.Subs)
			}
		}
		s.advanceOnboarding(acct.ID)
		s.autoGeneratePlan(acct, "Tự sinh cho kênh YouTube thật: "+chInfo.Title)
	} else {
		accounts, err := s.Mgr.List()
		if err != nil {
			publishersMsg(w, r, "err", "Không lấy được danh sách kênh: "+err.Error())
			return
		}
		var found *network.Account
		for _, a := range accounts {
			if a.Username == targetUsername {
				found = a
				break
			}
		}
		if found == nil {
			publishersMsg(w, r, "err", "Kênh "+targetUsername+" không còn tồn tại — thêm lại kênh rồi nối.")
			return
		}
		targetAccountID = found.ID
		chInfo, _ := fetchGoogleChannelInfo(accessToken)
		channelID := targetUsername
		if chInfo != nil && chInfo.ID != "" {
			channelID = chInfo.ID
			if chInfo.Subs > 0 && s.Ledger != nil {
				_ = s.Ledger.SetFollowers(found.ID, chInfo.Subs)
			}
		}
		contentTypes := found.YoutubeContentTypes
		if len(contentTypes) == 0 {
			contentTypes = []string{"short_video", "short_film"}
		}
		if s.Mgr != nil {
			_, _ = s.Mgr.SetYoutube(found.ID, channelID, contentTypes)
		}
	}

	path := publishers.YouTubeTokenPath(targetUsername)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		publishersMsg(w, r, "err", "Không tạo được thư mục token: "+err.Error())
		return
	}
	body, _ := json.Marshal(map[string]string{"refresh_token": refresh})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		publishersMsg(w, r, "err", "Không ghi được file token: "+err.Error())
		return
	}
	if s.Ledger != nil {
		_ = s.Ledger.DeleteSetting(tokenDeadKey(targetUsername))
	}

	if targetAccountID > 0 {
		seeOther(w, r, fmt.Sprintf("/accounts/%d?tab=connections&ok=%s", targetAccountID, url.QueryEscape("Đã kết nối thành công kênh YouTube "+targetUsername+"!")))
		return
	}
	publishersMsg(w, r, "msg", "Đã nối YouTube cho "+targetUsername+". Token lưu ở "+path+".")
}

// exchangeGoogleCode đổi authorization code lấy refresh token và access token.
func exchangeGoogleCode(clientID, clientSecret, code, redirectURI string) (string, string, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	resp, err := googleOAuthClient.PostForm(youtubeOAuthTokenURL, form)
	if err != nil {
		return "", "", fmt.Errorf("gọi Google thất bại: %w", err)
	}
	defer resp.Body.Close()
	var tok struct {
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
		AccessToken  string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", "", fmt.Errorf("đọc response Google: %w", err)
	}
	if tok.Error != "" {
		return "", "", fmt.Errorf("Google báo lỗi %s: %s", tok.Error, tok.ErrorDesc)
	}
	if tok.RefreshToken == "" {
		return "", "", fmt.Errorf("Google không trả refresh_token (tài khoản này có thể đã cấp quyền trước đây — thử thu hồi quyền ở myaccount.google.com/permissions rồi nối lại)")
	}
	return tok.RefreshToken, tok.AccessToken, nil
}

type googleChannelData struct {
	ID    string
	Title string
	Subs  int64
	Views int64
}

func fetchGoogleChannelInfo(accessToken string) (*googleChannelData, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/youtube/v3/channels?part=snippet,statistics&mine=true", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := googleOAuthClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google YouTube API status %d", resp.StatusCode)
	}
	var data struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title     string `json:"title"`
				CustomURL string `json:"customUrl"`
			} `json:"snippet"`
			Statistics struct {
				SubscriberCount string `json:"subscriberCount"`
				ViewCount       string `json:"viewCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	if len(data.Items) == 0 {
		return nil, fmt.Errorf("tài khoản Google này chưa tạo kênh YouTube")
	}
	item := data.Items[0]
	title := item.Snippet.Title
	if item.Snippet.CustomURL != "" {
		title = strings.TrimPrefix(item.Snippet.CustomURL, "@")
	}
	var subs, views int64
	fmt.Sscanf(item.Statistics.SubscriberCount, "%d", &subs)
	fmt.Sscanf(item.Statistics.ViewCount, "%d", &views)
	return &googleChannelData{
		ID:    item.ID,
		Title: title,
		Subs:  subs,
		Views: views,
	}, nil
}

func sanitizeUsername(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var buf strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			buf.WriteRune(r)
		} else if r == ' ' || r == '-' {
			buf.WriteRune('_')
		}
	}
	res := strings.Trim(buf.String(), "_")
	if len(res) > 30 {
		res = res[:30]
	}
	return res
}

// tokenDeadKey là setting đánh dấu refresh token YouTube đã chết
// (invalid_grant) — trang Đa nền tảng hiện badge đỏ thay vì "Có" xanh.
func tokenDeadKey(username string) string { return "token_dead:" + username }
