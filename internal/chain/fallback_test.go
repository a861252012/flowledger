package chain

import (
	"context"
	"encoding/json"
	"github.com/ethereum/go-ethereum/common"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestFallbackRejectsWrongChain(t *testing.T) {
	var wrongCalls atomic.Int32
	var wrongProbes atomic.Int32
	wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "eth_chainId" {
			wrongProbes.Add(1)
		}
		if req.Method != "eth_chainId" {
			wrongCalls.Add(1)
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": "0x1"})
	}))
	defer wrong.Close()
	var rightCalls atomic.Int32
	right := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		result := "0xaa36a7"
		if req.Method == "eth_getTransactionCount" {
			rightCalls.Add(1)
			result = "0x3"
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer right.Close()
	client, err := NewFallback([]string{wrong.URL, right.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.CheckNetwork(context.Background()); err != nil {
		t.Fatal(err)
	}
	nonce, err := client.NonceAt(context.Background(), common.Address{})
	if err != nil || nonce != 3 || rightCalls.Load() != 1 {
		t.Fatalf("fallback did not execute on testnet: %v", err)
	}
	if wrongProbes.Load() != 1 {
		t.Fatal("did not retain healthy fallback")
	}
	if wrongCalls.Load() != 0 {
		t.Fatal("forwarded operation to mainnet")
	}
}
