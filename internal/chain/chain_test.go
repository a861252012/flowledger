package chain

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func rpcClient(t *testing.T, reply func(string, json.RawMessage) any) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": reply(request.Method, request.Params)})
	}))
	t.Cleanup(server.Close)
	c, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestFormatETH(t *testing.T) {
	for _, tc := range []struct{ wei, eth string }{
		{"0", "0"}, {"1", "0.000000000000000001"}, {"1000000000000000000", "1"},
		{"1234567890123456789", "1.234567890123456789"},
		{"123456789012345678901234567890", "123456789012.34567890123456789"},
	} {
		t.Run(tc.wei, func(t *testing.T) {
			n, _ := new(big.Int).SetString(tc.wei, 10)
			if got := FormatETH(n); got != tc.eth {
				t.Fatalf("got %s, want %s", got, tc.eth)
			}
		})
	}
}

func TestRejectInvalidInputBeforeRPC(t *testing.T) {
	c := rpcClient(t, func(method string, _ json.RawMessage) any { t.Errorf("unexpected RPC: %s", method); return nil })
	if _, err := c.Balance(context.Background(), "0x123"); !errors.Is(err, ErrAddress) {
		t.Fatal(err)
	}
	if _, err := c.Transaction(context.Background(), "0x123"); !errors.Is(err, ErrHash) {
		t.Fatal(err)
	}
}

func TestRejectWrongChain(t *testing.T) {
	c := rpcClient(t, func(method string, _ json.RawMessage) any {
		if method != "eth_chainId" {
			t.Errorf("read continued on wrong chain: %s", method)
		}
		return "0x1"
	})
	if _, err := c.Network(context.Background()); !errors.Is(err, ErrNetwork) {
		t.Fatal(err)
	}
}

func TestTimeoutAndRPCSecretRedaction(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		c := rpcClient(t, func(_ string, _ json.RawMessage) any { time.Sleep(50 * time.Millisecond); return "0xaa36a7" })
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		if _, err := c.Network(ctx); !errors.Is(err, ErrTimeout) {
			t.Fatal(err)
		}
	})
	t.Run("provider error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "private-api-key", 500) }))
		defer server.Close()
		c, err := New(server.URL + "/private-api-key")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		_, err = c.Network(context.Background())
		if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "private-api-key") {
			t.Fatalf("unsafe error: %v", err)
		}
	})
}

func TestBalanceUsesBlockHashAndExactWei(t *testing.T) {
	h := &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, Time: 1700000000}
	c := rpcClient(t, func(method string, params json.RawMessage) any {
		switch method {
		case "eth_chainId":
			return "0xaa36a7"
		case "eth_getBlockByNumber":
			return h
		case "eth_getBalance":
			if !strings.Contains(string(params), h.Hash().Hex()) {
				t.Errorf("balance not pinned to block: %s", params)
			}
			return "0xde0b6b3a7640001"
		default:
			t.Errorf("unexpected RPC %s", method)
			return nil
		}
	})
	b, err := c.Balance(context.Background(), "0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if b.ETH != "1.000000000000000001" || b.Wei != "1000000000000000001" || b.Block != "100" {
		t.Fatalf("bad balance: %+v", b)
	}
}

func TestReceiptOutcomes(t *testing.T) {
	h := &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, Time: 1700000000}
	hash := common.HexToHash("0x" + strings.Repeat("a", 64))
	for _, tc := range []struct {
		name, state string
		status      uint64
		changed     bool
	}{
		{"success", "succeeded", 1, false}, {"revert", "reverted", 0, false}, {"reorg", "reorg_detected", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blockHash := h.Hash()
			if tc.changed {
				blockHash = common.HexToHash("0x1234")
			}
			c := rpcClient(t, func(method string, _ json.RawMessage) any {
				switch method {
				case "eth_chainId":
					return "0xaa36a7"
				case "eth_getBlockByNumber":
					return h
				case "eth_getTransactionReceipt":
					return &types.Receipt{
						Status: tc.status, TxHash: hash, BlockNumber: big.NewInt(100), BlockHash: blockHash,
						GasUsed: 21000, CumulativeGasUsed: 21000, EffectiveGasPrice: big.NewInt(1000000000), Logs: []*types.Log{},
					}
				default:
					t.Errorf("unexpected RPC %s", method)
					return nil
				}
			})
			got, err := c.Transaction(context.Background(), hash.Hex())
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.state {
				t.Fatalf("got %+v", got)
			}
			if !tc.changed && (got.FeeETH != "0.000021" || got.Confirmations != "1") {
				t.Fatalf("incorrect fee/confirmations: %+v", got)
			}
		})
	}
}

func TestMissingReceiptIsNotSuccess(t *testing.T) {
	c := rpcClient(t, func(method string, _ json.RawMessage) any {
		if method == "eth_chainId" {
			return "0xaa36a7"
		}
		return nil
	})
	if _, err := c.Transaction(context.Background(), "0x"+strings.Repeat("a", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
