package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSharedDemoBoundaries(t *testing.T) {
	h := WithPublicOrigin(SharedDemo(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })), "https://wallet.example")
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/wallet", 204}, {"GET", "/api/wallet/accounts", 204}, {"GET", "/api/wallet/history", 204},
		{"POST", "/api/wallet/send", 204}, {"POST", "/net/base/api/wallet/send", 204},
		{"POST", "/api/wallet/quote", 204}, {"POST", "/api/wallet/backup", 204},
		{"POST", "/api/wallet/accounts", 403}, {"POST", "/api/wallet/accounts/update", 403},
		{"POST", "/api/wallet/create", 403}, {"POST", "/api/wallet/import", 403},
		{"POST", "/api/wallet/password", 403}, {"POST", "/api/wallet/scan", 403},
		{"POST", "/net/base/api/wallet/scan", 403}, {"POST", "/api/faucet", 403},
		{"POST", "/solana/api/create", 403}, {"POST", "/tron/api/send", 403},
		{"GET", "/accounts/abc/api/wallet", 403}, {"POST", "/api/../api/wallet/send", 403},
		{"DELETE", "/api/wallet/send", 403}, {"GET", "/login", 303},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, "http://wallet.example"+tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s %s: %d want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
}

func TestSharedDemoPasswordAttemptLimit(t *testing.T) {
	h := SharedDemo(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for i := range 11 {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/wallet/send", nil))
		want := 204
		if i == 10 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d returned %d", i, w.Code)
		}
	}
}
