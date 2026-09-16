package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTrafficLimitCannotBeBypassedWithNetworkOrForwardedIP(t *testing.T) {
	var calls int
	h := LimitTraffic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls += 1
		w.WriteHeader(http.StatusNoContent)
	}))
	paths := []string{"/api/wallet", "/net/base/api/network", "/accounts/test/solana/api/status", "/tron/api/status"}
	for i := 0; i < 301; i += 1 {
		r := httptest.NewRequest("GET", paths[i%len(paths)], nil)
		r.Header.Set("X-Forwarded-For", "192.0.2."+strconv.Itoa(i%255))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if i < 300 && w.Code != http.StatusNoContent {
			t.Fatalf("request %d: %d", i, w.Code)
		}
		if i == 300 && (w.Code != 429 || w.Header().Get("Retry-After") != "60" || w.Header().Get("Content-Security-Policy") == "") {
			t.Fatalf("unbounded API: %d %v", w.Code, w.Header())
		}
	}
	if calls != 300 {
		t.Fatalf("rejected request reached handler: %d", calls)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != http.StatusNoContent {
		t.Fatal("API budget blocks health check")
	}
}

func TestTrafficConcurrencyRejectsBeforeHandlerAndReleasesSlots(t *testing.T) {
	for _, tc := range []struct {
		method string
		limit  int
	}{{"GET", 8}, {"POST", 1}} {
		t.Run(tc.method, func(t *testing.T) {
			started, release := make(chan struct{}, tc.limit), make(chan struct{})
			var calls atomic.Int32
			h := LimitTraffic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				started <- struct{}{}
				<-release
				w.WriteHeader(http.StatusNoContent)
			}))
			var wg sync.WaitGroup
			defer wg.Wait()
			var once sync.Once
			defer once.Do(func() { close(release) })
			for i := 0; i < tc.limit; i += 1 {
				wg.Go(func() {
					h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tc.method, "/api/wallet", nil))
				})
			}
			for i := 0; i < tc.limit; i += 1 {
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("handler did not start")
				}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, "/api/wallet", nil))
			if w.Code != 429 || calls.Load() != int32(tc.limit) {
				t.Fatalf("excess handler started: code=%d calls=%d", w.Code, calls.Load())
			}
			once.Do(func() { close(release) })
			wg.Wait()
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, "/api/wallet", nil))
			if w.Code != http.StatusNoContent {
				t.Fatalf("slot leaked: %d", w.Code)
			}
		})
	}
}
