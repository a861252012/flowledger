package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestTrafficBoundsBodyAndDeadline(t *testing.T) {
	h := LimitTraffic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 45*time.Second {
			t.Error("missing bounded deadline")
		}
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(413)
			return
		}
		w.WriteHeader(204)
	}))
	for _, length := range []int64{16385, -1} {
		r := httptest.NewRequest("POST", "/api/wallet/import", strings.NewReader(strings.Repeat("x", 16385)))
		r.ContentLength = length
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 413 {
			t.Fatal("unbounded body", length, w.Code)
		}
	}
}

func TestTrafficTokenAndPoolsUseReadSlotsWithoutBlockingWrites(t *testing.T) {
	// Verify that token and pools queries use the 8-slot read pool and do not block writes or get blocked by writes.
	startedRead := make(chan struct{}, 8)
	releaseRead := make(chan struct{})
	startedWrite := make(chan struct{}, 1)
	releaseWrite := make(chan struct{})
	var calls atomic.Int32

	h := LimitTraffic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cleanPath := strings.TrimSuffix(r.URL.Path, "/")
		if strings.HasSuffix(cleanPath, "/token") || strings.HasSuffix(cleanPath, "/pools") {
			startedRead <- struct{}{}
			<-releaseRead
		} else {
			startedWrite <- struct{}{}
			<-releaseWrite
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// 1. Occupy the single write slot with a slow write request.
	var wg sync.WaitGroup
	defer wg.Wait()
	var onceRead, onceWrite sync.Once
	defer onceRead.Do(func() { close(releaseRead) })
	defer onceWrite.Do(func() { close(releaseWrite) })

	wg.Go(func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/wallet/send", nil))
	})
	select {
	case <-startedWrite:
	case <-time.After(5 * time.Second):
		t.Fatal("write handler did not start")
	}

	// 2. While the write slot is fully occupied, 8 concurrent POST /api/wallet/token and /pools requests must still succeed.
	testPaths := []string{
		"/api/wallet/token",
		"/api/wallet/token/",
		"/net/polygon/api/wallet/token",
		"/api/wallet/exchange/pools",
		"/api/wallet/exchange/pools/",
		"/accounts/acc1/api/wallet/token",
		"/api/wallet/token",
		"/api/wallet/exchange/pools",
	}
	for _, p := range testPaths {
		wg.Go(func() {
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", p, nil))
		})
	}
	for i := 0; i < 8; i += 1 {
		select {
		case <-startedRead:
		case <-time.After(5 * time.Second):
			t.Fatalf("read handler %d did not start while write was occupied", i)
		}
	}

	// 3. The 9th read request must be rejected with 429 because all 8 read slots are occupied.
	rec9 := httptest.NewRecorder()
	h.ServeHTTP(rec9, httptest.NewRequest("POST", "/api/wallet/token", nil))
	if rec9.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when read slots full, got %d", rec9.Code)
	}

	// 4. A 2nd write request must be rejected with 429 because the 1 write slot is occupied.
	recWrite2 := httptest.NewRecorder()
	h.ServeHTTP(recWrite2, httptest.NewRequest("POST", "/api/wallet/quote", nil))
	if recWrite2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when write slot full, got %d", recWrite2.Code)
	}

	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/api/other/token"},
		{http.MethodPost, "/api/other/pools"},
		{http.MethodDelete, "/api/wallet/token"},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(request.method, request.path, nil))
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("%s %s bypassed occupied write slot: %d", request.method, request.path, rec.Code)
		}
	}

	// Clean up handlers
	onceRead.Do(func() { close(releaseRead) })
	onceWrite.Do(func() { close(releaseWrite) })
	wg.Wait()
}
