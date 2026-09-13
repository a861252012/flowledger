package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/a861252012/flowledger/internal/chain"
	"github.com/a861252012/flowledger/internal/wallet"
	"github.com/a861252012/flowledger/internal/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	rpcURL := os.Getenv("SEPOLIA_RPC_URL")
	if rpcURL == "" {
		rpcURL = "https://ethereum-sepolia-rpc.publicnode.com"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("PORT 必須是 1–65535 的整數")
	}
	host := os.Getenv("HTTP_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	if host != "127.0.0.1" && host != "0.0.0.0" {
		return errors.New("HTTP_HOST 僅允許 127.0.0.1 或 0.0.0.0")
	}
	client, err := chain.New(rpcURL)
	if err != nil {
		return err
	}
	defer client.Close()
	walletDir := os.Getenv("WALLET_DIR")
	if walletDir == "" {
		walletDir = "./data/wallet"
	}
	walletService, err := wallet.NewService(client, walletDir)
	if err != nil {
		return err
	}
	defer walletService.Close()
	handler, err := web.New(client, walletService)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: host + ":" + port, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverError := make(chan error, 1)
	go func() { serverError <- server.ListenAndServe() }()
	slog.Info("FlowLedger listening", "url", "http://"+server.Addr, "mode", "Sepolia wallet")
	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP 服務啟動失敗；請檢查 PORT 是否已被使用")
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
	return nil
}
