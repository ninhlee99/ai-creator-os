package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const bioDeviceCookie = "aicos_bio_device"

const (
	bioPageViewDedupeWindow = 30 * time.Minute
	bioActionDedupeWindow   = 30 * time.Second
)

func ensureBioDeviceCookie(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(bioDeviceCookie); err == nil && validBioDeviceToken(cookie.Value) {
		return cookie.Value
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return ""
	}
	value := base64.RawURLEncoding.EncodeToString(token[:])
	http.SetCookie(w, &http.Cookie{
		Name: bioDeviceCookie, Value: value, Path: "/bio", MaxAge: 365 * 24 * 60 * 60,
		HttpOnly: true, Secure: isHTTPSRequest(r), SameSite: http.SameSiteLaxMode,
	})
	return value
}

func validBioDeviceToken(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func (s *Server) bioIdentityHashes(r *http.Request, deviceToken string) (ipHash, deviceHash string, err error) {
	if !validBioDeviceToken(deviceToken) {
		return "", "", fmt.Errorf("invalid Bio device token")
	}
	clientIP, err := s.bioClientIP(r)
	if err != nil {
		return "", "", err
	}
	key, err := s.bioTryOnKey()
	if err != nil {
		return "", "", err
	}
	return bioHMAC(key, "ip", clientIP), bioHMAC(key, "device", deviceToken), nil
}

func (s *Server) bioImageHash(image []byte) (string, error) {
	key, err := s.bioTryOnKey()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("bio-tryon-image\x00"))
	_, _ = mac.Write(image)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func bioHMAC(key []byte, scope, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("bio-tryon-" + scope + "\x00"))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) bioTryOnKey() ([]byte, error) {
	s.bioTryOnKeyMu.Lock()
	defer s.bioTryOnKeyMu.Unlock()
	return loadOrCreateBioSecret(s.DataDir, "bio-tryon.key", "Bio try-on")
}

func (s *Server) bioAnalyticsDedupeKey(clientIP string, accountID, productID int64, eventType string) (string, error) {
	s.bioAnalyticsMu.Lock()
	if len(s.bioAnalyticsHMACKey) == 0 {
		key, err := loadOrCreateBioSecret(s.DataDir, "bio-analytics.key", "Bio analytics")
		if err != nil {
			s.bioAnalyticsMu.Unlock()
			return "", err
		}
		s.bioAnalyticsHMACKey = key
	}
	key := append([]byte(nil), s.bioAnalyticsHMACKey...)
	s.bioAnalyticsMu.Unlock()

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("bio-analytics\x00"))
	_, _ = fmt.Fprintf(mac, "%s\x00%d\x00%d\x00%s", clientIP, accountID, productID, eventType)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func loadOrCreateBioSecret(dataDir, filename, purpose string) ([]byte, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("%s data directory is not configured", purpose)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create Bio data directory: %w", err)
	}
	path := filepath.Join(dataDir, filename)
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, fmt.Errorf("create %s key: %w", purpose, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, err = file.Write(key[:]); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
			return nil, fmt.Errorf("write %s key: %w", purpose, err)
		}
		return append([]byte(nil), key[:]...), nil
	}
	if !os.IsExist(err) {
		return nil, fmt.Errorf("open %s key: %w", purpose, err)
	}

	file, err = os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s key: %w", purpose, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s key permissions must be 0600", purpose)
	}
	stored := make([]byte, 32)
	if _, err := io.ReadFull(file, stored); err != nil {
		return nil, fmt.Errorf("read %s key: %w", purpose, err)
	}
	var extra [1]byte
	if n, err := file.Read(extra[:]); err != io.EOF || n != 0 {
		return nil, fmt.Errorf("%s key has invalid length", purpose)
	}
	return stored, nil
}

// Cloudflare supplies CF-Connecting-IP to its origin traffic. Trust it only
// when explicitly configured; ignore generic forwarded chains that proxies
// may append to client-provided X-Forwarded-For values.
func (s *Server) bioClientIP(r *http.Request) (string, error) {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	peerIP := net.ParseIP(strings.TrimSpace(peer))
	if peerIP == nil {
		return "", fmt.Errorf("invalid Bio client address")
	}
	if peerIP.IsLoopback() && s.TrustCloudflareIP {
		if candidate := net.ParseIP(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); candidate != nil {
			return candidate.String(), nil
		}
	}
	return peerIP.String(), nil
}

// recordBioPageView limits repeated refreshes from one network to one view
// per page in each 30-minute window.
func (s *Server) recordBioPageView(r *http.Request, accountID, productID int64, eventType string, at time.Time) {
	if eventType != "profile_view" && eventType != "product_view" {
		return
	}
	s.recordBioMetric(r, accountID, productID, eventType, at, bioPageViewDedupeWindow)
}

func (s *Server) recordBioInteraction(r *http.Request, accountID, productID int64, eventType string, at time.Time) bool {
	if eventType != "outbound_click" && eventType != "voucher_copy" {
		return false
	}
	return s.recordBioMetric(r, accountID, productID, eventType, at, bioActionDedupeWindow)
}

func (s *Server) recordBioMetric(r *http.Request, accountID, productID int64, eventType string, at time.Time, dedupeWindow time.Duration) bool {
	clientIP, err := s.bioClientIP(r)
	if err != nil {
		return false
	}
	if at.IsZero() {
		at = time.Now()
	}
	if s.Products == nil {
		return false
	}
	dedupeKey, err := s.bioAnalyticsDedupeKey(clientIP, accountID, productID, eventType)
	if err != nil {
		return false
	}
	recorded, err := s.Products.RecordBioEventOnce(dedupeKey, accountID, productID, eventType, at, dedupeWindow)
	return err == nil && recorded
}
