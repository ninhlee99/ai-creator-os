package web

import (
	"log"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) originGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !sameOrigin(r) {
				log.Printf("web: từ chối %s %s — origin không khớp (chống CSRF)", r.Method, r.URL.Path)
				http.Error(w, "origin không khớp — từ chối để chống CSRF", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	return strings.EqualFold(proto, "https")
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if ref := r.Header.Get("Referer"); ref != "" {
			origin = ref
		} else {
			return true // curl / gọi nội bộ: không có header để kiểm
		}
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, "null") {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	// Đợt AK: sau tunnel/proxy (cloudflared, Tailscale serve) Host có thể bị
	// ghi đè thành 127.0.0.1 — so thêm host gốc proxy chuyển tiếp. Form CSRF
	// từ site lạ không tự đặt được header này.
	fh := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	return fh != "" && strings.EqualFold(u.Host, fh)
}

// recoverer keeps a panicking handler from taking the server down.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("web: panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// redirectTo returns a handler that 303-redirects every request to loc.
// Used for the legacy page URLs replaced by tabs in the Đợt 2 merge.
