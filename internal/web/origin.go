package web

import (
	"context"
	"net/http"
	"strings"
)

type publicOriginKey struct{}

// WithPublicOrigin uses a startup-validated fixed origin, never forwarded headers.
// The deployment must prevent direct access to the origin port.
func WithPublicOrigin(next http.Handler, origin string) http.Handler {
	return secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" && r.URL.Path != "/healthz" {
			navigation := r.Method == http.MethodGet && r.Header.Get("Sec-Fetch-Mode") == "navigate" && r.Header.Get("Sec-Fetch-Dest") == "document"
			if r.Host != strings.TrimPrefix(origin, "https://") || (r.Header.Get("Sec-Fetch-Site") == "cross-site" && !navigation) {
				http.Error(w, "不允許的來源", http.StatusForbidden)
				return
			}
			if supplied := r.Header.Get("Origin"); supplied != "" && supplied != origin {
				http.Error(w, "不允許的 Origin", http.StatusForbidden)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), publicOriginKey{}, origin))
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	}))
}

func allowedRequestHost(r *http.Request) bool {
	if origin, ok := r.Context().Value(publicOriginKey{}).(string); ok {
		return r.Host == strings.TrimPrefix(origin, "https://")
	}
	return isValidHost(r.Host)
}

func requestOrigin(r *http.Request) string {
	if origin, ok := r.Context().Value(publicOriginKey{}).(string); ok {
		return origin
	}
	if r.TLS != nil {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}
