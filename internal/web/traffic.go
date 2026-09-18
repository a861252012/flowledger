package web

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LimitTraffic bounds process-wide API work. It is not authentication or an edge DDoS filter.
func LimitTraffic(next http.Handler) http.Handler {
	reads := make(chan struct{}, 8)
	writes := make(chan struct{}, 1)
	var mu sync.Mutex
	window := time.Now()
	requests := 0
	return secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength > 16*1024 {
			http.Error(w, "請求內容過大", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		mu.Lock()
		if time.Since(window) >= time.Minute {
			window, requests = time.Now(), 0
		}
		limited := requests >= 300
		if !limited {
			requests += 1
		}
		mu.Unlock()
		if limited {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"請求過於頻繁，請稍後再試。"}`))
			return
		}
		slots := reads
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			cleanPath := strings.TrimSuffix(r.URL.Path, "/")
			if r.Method != http.MethodPost || (!strings.HasSuffix(cleanPath, "/api/wallet/token") && !strings.HasSuffix(cleanPath, "/api/wallet/exchange/pools")) {
				slots = writes
			}
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "2")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"服務忙碌，這次請求尚未執行，請稍後再試。"}`))
		}
	}))
}
