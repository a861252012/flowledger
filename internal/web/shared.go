package web

import (
	"context"
	"errors"
	"github.com/a861252012/testnet-wallet-lab/internal/wallet"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

type sharedDemoKey struct{}

var errSharedDemo = errors.New("共用 Demo 不提供錢包管理與背景掃描操作")
var errSharedAttempts = errors.New("密碼操作過於頻繁，請稍後再試")

// SharedDemo exposes the existing wallet UI but keeps wallet administration closed.
// Signing and encrypted exports still pass through the wallet's password checks.
func SharedDemo(next http.Handler, token string) http.Handler {
	initialize := RequireAccessToken(next, token)
	next = LimitTraffic(next)
	var mu sync.Mutex
	window := time.Now()
	attempts := 0
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/login" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		route := strings.TrimSuffix(r.URL.Path, "/")
		if route == "" {
			route = "/"
		}
		if path.Clean(r.URL.Path) != route {
			respondWallet(w, http.StatusForbidden, nil, errSharedDemo)
			return
		}
		if after, ok := strings.CutPrefix(route, "/accounts/"); ok {
			parts := strings.SplitN(after, "/", 2)
			if _, err := wallet.ParseAccountID(parts[0]); err != nil {
				respondWallet(w, 403, nil, errSharedDemo)
				return
			}
			route = "/"
			if len(parts) == 2 {
				route += parts[1]
			}
		}
		if after, ok := strings.CutPrefix(route, "/net/"); ok {
			parts := strings.SplitN(after, "/", 2)
			if len(parts) == 2 {
				switch parts[0] {
				case "arbitrum", "base", "optimism", "polygon":
					route = "/" + parts[1]
				}
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Method != http.MethodPost {
				respondWallet(w, http.StatusForbidden, nil, errSharedDemo)
				return
			}
			switch route {
			case "/solana/api/create", "/tron/api/create":
				// Only the operator may initialize an absent chain wallet.
				if len(token) < 32 {
					respondWallet(w, http.StatusForbidden, nil, errSharedDemo)
					return
				}
				initialize.ServeHTTP(w, r)
				return
			case "/solana/api/quote", "/solana/api/send", "/solana/api/retry", "/solana/api/backup",
				"/tron/api/quote", "/tron/api/send", "/tron/api/retry", "/tron/api/backup":
			case "/api/wallet/accounts", "/api/wallet/quote", "/api/wallet/send", "/api/wallet/retry", "/api/wallet/token", "/api/wallet/exchange/pools", "/api/wallet/backup":
			default:
				respondWallet(w, http.StatusForbidden, nil, errSharedDemo)
				return
			}
			if route == "/api/wallet/accounts" || strings.HasSuffix(route, "/send") || strings.HasSuffix(route, "/backup") {
				mu.Lock()
				if time.Since(window) >= time.Minute {
					window, attempts = time.Now(), 0
				}
				limited := attempts >= 10
				if !limited {
					attempts++
				}
				mu.Unlock()
				if limited {
					w.Header().Set("Retry-After", "60")
					respondWallet(w, http.StatusTooManyRequests, nil, errSharedAttempts)
					return
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sharedDemoKey{}, true)))
	})
}
