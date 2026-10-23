package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ninhlee99/ai-creator-os/internal/products"
)

const (
	bioGoogleSessionCookie = "aicos_bio_google"
	bioGoogleStateCookie   = "aicos_bio_google_state"
	bioGoogleSessionTTL    = 30 * 24 * time.Hour
	bioGoogleStateTTL      = 10 * time.Minute
	bioGoogleAuthURL       = "https://accounts.google.com/o/oauth2/v2/auth"
	bioGoogleTokenURL      = "https://oauth2.googleapis.com/token"
	bioGoogleUserInfoURL   = "https://openidconnect.googleapis.com/v1/userinfo"
)

type bioGoogleOAuthCredentials struct {
	clientID     string
	clientSecret string
}

type bioGoogleUserInfo struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

func currentBioGoogleOAuthCredentials() bioGoogleOAuthCredentials {
	clientID := strings.TrimSpace(os.Getenv("BIO_GOOGLE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("BIO_GOOGLE_CLIENT_SECRET"))
	if clientID == "" && clientSecret == "" {
		clientID = strings.TrimSpace(os.Getenv("YOUTUBE_CLIENT_ID"))
		clientSecret = strings.TrimSpace(os.Getenv("YOUTUBE_CLIENT_SECRET"))
	}
	return bioGoogleOAuthCredentials{clientID: clientID, clientSecret: clientSecret}
}

func bioGoogleRedirectURI(r *http.Request) (string, error) {
	configured := strings.TrimSpace(os.Getenv("BIO_GOOGLE_REDIRECT_URI"))
	if configured == "" {
		host := r.Host
		hostname := host
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			hostname = parsedHost
		}
		hostname = strings.Trim(hostname, "[]")
		if !bioGoogleLoopbackHost(hostname) {
			return "", fmt.Errorf("thiếu BIO_GOOGLE_REDIRECT_URI; hãy cấu hình callback HTTPS đã đăng ký với Google")
		}
		return (&url.URL{Scheme: "http", Host: host, Path: "/bio/google/callback"}).String(), nil
	}
	u, err := url.Parse(configured)
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.Opaque != "" ||
		u.EscapedPath() != "/bio/google/callback" || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || strings.Contains(configured, "#") {
		return "", fmt.Errorf("BIO_GOOGLE_REDIRECT_URI phải kết thúc chính xác bằng /bio/google/callback")
	}
	hostname := u.Hostname()
	if u.Scheme == "https" {
		return u.String(), nil
	}
	if u.Scheme == "http" && bioGoogleLoopbackHost(hostname) {
		return u.String(), nil
	}
	return "", fmt.Errorf("callback Google từ xa cần HTTPS")
}

func bioGoogleLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func bioGoogleLoginReady(r *http.Request) bool {
	credentials := currentBioGoogleOAuthCredentials()
	if credentials.clientID == "" || credentials.clientSecret == "" {
		return false
	}
	_, err := bioGoogleRedirectURI(r)
	return err == nil
}

func bioTryOnReturnPath(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.RawQuery != "" ||
		u.ForceQuery || u.Fragment != "" || !strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "bio" || !validPublicBioChannel(parts[1]) ||
		parts[2] != "product" || !positivePathID(parts[3]) {
		return "", false
	}
	return u.Path, true
}

func (s *Server) handleBioGoogleLogin(w http.ResponseWriter, r *http.Request) {
	returnPath, ok := bioTryOnReturnPath(r.URL.Query().Get("return"))
	if !ok {
		http.Error(w, "Đường dẫn quay lại không hợp lệ", http.StatusBadRequest)
		return
	}
	credentials := currentBioGoogleOAuthCredentials()
	if credentials.clientID == "" || credentials.clientSecret == "" {
		http.Redirect(w, r, bioGoogleLoginResultPath(returnPath, "unavailable"), http.StatusSeeOther)
		return
	}
	redirectURI, err := bioGoogleRedirectURI(r)
	if err != nil {
		http.Redirect(w, r, bioGoogleLoginResultPath(returnPath, "unavailable"), http.StatusSeeOther)
		return
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		http.Error(w, "Không khởi tạo được đăng nhập Google", http.StatusInternalServerError)
		return
	}
	nonceValue := base64.RawURLEncoding.EncodeToString(nonce)
	nonceHash := sha256.Sum256([]byte(nonceValue))
	now := time.Now()
	state := s.oauth.issueBioGoogle(returnPath, redirectURI, hex.EncodeToString(nonceHash[:]))
	http.SetCookie(w, &http.Cookie{
		Name: bioGoogleStateCookie, Value: nonceValue, Path: "/bio/google/callback",
		MaxAge: int(bioGoogleStateTTL.Seconds()), Expires: now.Add(bioGoogleStateTTL),
		HttpOnly: true, Secure: isHTTPSRequest(r), SameSite: http.SameSiteLaxMode,
	})
	query := url.Values{
		"client_id":     {credentials.clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"prompt":        {"select_account"},
	}
	http.Redirect(w, r, bioGoogleAuthURL+"?"+query.Encode(), http.StatusFound)
}

func (s *Server) handleBioGoogleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, cookieErr := r.Cookie(bioGoogleStateCookie)
	clearBioGoogleStateCookie(w, r)
	state, ok := s.oauth.take(r.URL.Query().Get("state"))
	if !ok || cookieErr != nil || stateCookie.Value == "" {
		http.Error(w, "Đăng nhập Google hết hạn hoặc state không hợp lệ", http.StatusBadRequest)
		return
	}
	nonceHash := sha256.Sum256([]byte(stateCookie.Value))
	gotNonceHash := hex.EncodeToString(nonceHash[:])
	if subtle.ConstantTimeCompare([]byte(gotNonceHash), []byte(state.bioNonceHash)) != 1 {
		http.Error(w, "Phiên đăng nhập Google không khớp", http.StatusBadRequest)
		return
	}
	returnPath, validReturn := bioTryOnReturnPath(state.bioReturnPath)
	if !validReturn {
		http.Error(w, "Đường dẫn quay lại không hợp lệ", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, bioGoogleLoginResultPath(returnPath, "cancelled"), http.StatusSeeOther)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Redirect(w, r, bioGoogleLoginResultPath(returnPath, "failed"), http.StatusSeeOther)
		return
	}
	credentials := currentBioGoogleOAuthCredentials()
	if credentials.clientID == "" || credentials.clientSecret == "" {
		http.Redirect(w, r, bioGoogleLoginResultPath(returnPath, "unavailable"), http.StatusSeeOther)
		return
	}
	user, err := exchangeBioGoogleCode(credentials, code, state.redirectURI)
	if err != nil {
		http.Redirect(w, r, bioGoogleLoginResultPath(returnPath, "failed"), http.StatusSeeOther)
		return
	}
	if s.Products == nil {
		http.Error(w, "Kho Bio chưa sẵn sàng", http.StatusServiceUnavailable)
		return
	}
	subjectHash, err := s.bioGoogleSubjectHash(user.Subject)
	if err != nil {
		http.Error(w, "Không lưu được tài khoản Google", http.StatusServiceUnavailable)
		return
	}
	account, err := s.Products.UpsertBioGoogleAccount(subjectHash, user.Email, user.Name, time.Now())
	if err != nil {
		http.Error(w, "Không lưu được tài khoản Google", http.StatusServiceUnavailable)
		return
	}
	if !account.Active {
		http.Redirect(w, r, bioGoogleLoginResultPath(returnPath, "blocked"), http.StatusSeeOther)
		return
	}
	sessionBytes := make([]byte, 32)
	if _, err := rand.Read(sessionBytes); err != nil {
		http.Error(w, "Không tạo được phiên đăng nhập", http.StatusInternalServerError)
		return
	}
	sessionToken := base64.RawURLEncoding.EncodeToString(sessionBytes)
	sessionHash := bioGoogleSessionHash(sessionToken)
	now := time.Now()
	expiresAt := now.Add(bioGoogleSessionTTL)
	if err := s.Products.CreateBioGoogleSession(sessionHash, account.ID, now, expiresAt); err != nil {
		http.Error(w, "Không lưu được phiên đăng nhập", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: bioGoogleSessionCookie, Value: sessionToken, Path: "/bio",
		MaxAge: int(bioGoogleSessionTTL.Seconds()), Expires: expiresAt,
		HttpOnly: true, Secure: isHTTPSRequest(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, returnPath, http.StatusSeeOther)
}

func clearBioGoogleStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: bioGoogleStateCookie, Value: "", Path: "/bio/google/callback", MaxAge: -1,
		HttpOnly: true, Secure: isHTTPSRequest(r), SameSite: http.SameSiteLaxMode,
	})
}

func bioGoogleLoginResultPath(returnPath, result string) string {
	u := &url.URL{Path: returnPath}
	query := url.Values{}
	query.Set("google_login", result)
	u.RawQuery = query.Encode()
	return u.String()
}

func bioGoogleLoginMessage(result string) string {
	switch result {
	case "cancelled":
		return "Bạn đã hủy đăng nhập Google."
	case "unavailable":
		return "Đăng nhập Google chưa được cấu hình cho Bio."
	case "failed":
		return "Đăng nhập Google chưa hoàn tất. Hãy thử lại."
	case "blocked":
		return "Tài khoản Google này hiện không được phép tạo ảnh thử đồ."
	default:
		return ""
	}
}

func exchangeBioGoogleCode(credentials bioGoogleOAuthCredentials, code, redirectURI string) (bioGoogleUserInfo, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {credentials.clientID},
		"client_secret": {credentials.clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	response, err := googleOAuthClient.PostForm(bioGoogleTokenURL, form)
	if err != nil {
		return bioGoogleUserInfo{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil || response.StatusCode != http.StatusOK {
		return bioGoogleUserInfo{}, fmt.Errorf("Google token exchange failed")
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(data, &token); err != nil || token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return bioGoogleUserInfo{}, fmt.Errorf("Google token response is invalid")
	}
	request, err := http.NewRequest(http.MethodGet, bioGoogleUserInfoURL, nil)
	if err != nil {
		return bioGoogleUserInfo{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	infoResponse, err := googleOAuthClient.Do(request)
	if err != nil {
		return bioGoogleUserInfo{}, err
	}
	defer infoResponse.Body.Close()
	infoData, err := io.ReadAll(io.LimitReader(infoResponse.Body, 64<<10))
	if err != nil || infoResponse.StatusCode != http.StatusOK {
		return bioGoogleUserInfo{}, fmt.Errorf("Google userinfo request failed")
	}
	var user bioGoogleUserInfo
	if err := json.Unmarshal(infoData, &user); err != nil || user.Subject == "" || user.Email == "" || !user.EmailVerified {
		return bioGoogleUserInfo{}, fmt.Errorf("Google account is missing a verified email")
	}
	return user, nil
}

func (s *Server) bioGoogleSubjectHash(subject string) (string, error) {
	if strings.TrimSpace(subject) == "" || len(subject) > 255 {
		return "", fmt.Errorf("invalid Google subject")
	}
	s.bioGoogleSubjectKeyMu.Lock()
	if len(s.bioGoogleSubjectKey) == 0 {
		key, err := loadOrCreateBioSecret(s.DataDir, "bio-google-subject.key", "Bio Google identity")
		if err != nil {
			s.bioGoogleSubjectKeyMu.Unlock()
			return "", err
		}
		s.bioGoogleSubjectKey = key
	}
	key := append([]byte(nil), s.bioGoogleSubjectKey...)
	s.bioGoogleSubjectKeyMu.Unlock()
	return bioHMAC(key, "google-subject", subject), nil
}

func bioGoogleSessionHash(token string) string {
	sum := sha256.Sum256([]byte("bio-google-session\x00" + token))
	return hex.EncodeToString(sum[:])
}

func bioGoogleSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(bioGoogleSessionCookie)
	if err != nil {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(decoded) != 32 {
		return ""
	}
	return cookie.Value
}

func (s *Server) bioGoogleAccountForRequest(r *http.Request) (products.BioGoogleAccount, bool) {
	if s.Products == nil {
		return products.BioGoogleAccount{}, false
	}
	token := bioGoogleSessionToken(r)
	if token == "" {
		return products.BioGoogleAccount{}, false
	}
	account, err := s.Products.BioGoogleAccountForSession(bioGoogleSessionHash(token), time.Now())
	return account, err == nil && account.Active
}

func (s *Server) handleBioGoogleLogout(w http.ResponseWriter, r *http.Request) {
	returnPath, ok := bioTryOnReturnPath(r.URL.Query().Get("return"))
	if !ok {
		returnPath = "/bio"
	}
	if token := bioGoogleSessionToken(r); token != "" && s.Products != nil {
		_ = s.Products.RevokeBioGoogleSession(bioGoogleSessionHash(token))
	}
	http.SetCookie(w, &http.Cookie{
		Name: bioGoogleSessionCookie, Value: "", Path: "/bio", MaxAge: -1,
		HttpOnly: true, Secure: isHTTPSRequest(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, returnPath, http.StatusSeeOther)
}
