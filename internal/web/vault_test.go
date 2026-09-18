package web

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a861252012/testnet-wallet-lab/internal/chain"
	"github.com/a861252012/testnet-wallet-lab/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestVaultWebRouteDisabled(t *testing.T) {
	c, err := chain.New("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	dir := t.TempDir()
	ws, err := wallet.NewService(c, dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	h, err := New(c, ws)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "http://localhost:8090/api/wallet/vault", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var res struct {
		Enabled    bool   `json:"enabled"`
		Contract   string `json:"contract"`
		Balance    string `json:"balance"`
		BalanceRaw string `json:"balanceRaw"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Enabled || res.Contract != "" || res.Balance != "" || res.BalanceRaw != "" {
		t.Fatalf("expected disabled vault info, got %#v", res)
	}
}

func TestVaultWebRouteEnabled(t *testing.T) {
	const testVault = "0x1111111111111111111111111111111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var res any
		switch req.Method {
		case "eth_chainId":
			res = "0xaa36a7"
		case "eth_getCode":
			res = "0x60806040"
		case "eth_call":
			val := new(big.Int)
			val.SetString("2500000000000000000", 10)
			res = hexutil.Encode(common.LeftPadBytes(val.Bytes(), 32))
		default:
			res = "0x0"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  res,
		})
	}))
	defer server.Close()

	c, err := chain.NewNetwork(chain.SepoliaID, []string{server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	dir := t.TempDir()
	ws, err := wallet.NewService(c, dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.SetVaultAddress(testVault); err != nil {
		t.Fatal(err)
	}
	_, err = ws.Create("password-12345")
	if err != nil {
		t.Fatal(err)
	}

	h, err := New(c, ws)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "http://localhost:8090/api/wallet/vault", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Enabled    bool   `json:"enabled"`
		Contract   string `json:"contract"`
		Balance    string `json:"balance"`
		BalanceRaw string `json:"balanceRaw"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Enabled || res.Contract != testVault || res.Balance != "2.5" || res.BalanceRaw != "2500000000000000000" {
		t.Fatalf("unexpected vault response: %#v", res)
	}
}
