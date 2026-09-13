package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/a861252012/flowledger/internal/chain"
	"github.com/a861252012/flowledger/internal/wallet"
)

//go:embed templates/*.html static/*
var assets embed.FS

func New(client *chain.Client, walletService ...*wallet.Service) (http.Handler, error) {
	page, err := template.ParseFS(assets, "templates/index.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = page.Execute(w, nil)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]string{"status": "ok", "mode": "wallet"}, nil)
	})
	mux.HandleFunc("GET /api/network", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := client.Network(ctx)
		respond(w, result, err)
	})
	mux.HandleFunc("GET /api/balance", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := client.Balance(ctx, strings.TrimSpace(r.URL.Query().Get("address")))
		respond(w, result, err)
	})
	mux.HandleFunc("GET /api/transactions/{hash}", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		result, err := client.Transaction(ctx, r.PathValue("hash"))
		respond(w, result, err)
	})
	if len(walletService) > 0 && walletService[0] != nil {
		registerWalletRoutes(mux, walletService[0])
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	}), nil
}

func respond(w http.ResponseWriter, result any, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, chain.ErrAddress), errors.Is(err, chain.ErrHash):
			status = http.StatusBadRequest
		case errors.Is(err, chain.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(err, chain.ErrTimeout):
			status = http.StatusGatewayTimeout
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}
