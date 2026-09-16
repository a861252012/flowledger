package wallet

import (
	"context"
	"encoding/json"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"testing"
)

func TestPolygonQuoteSignsAmoyAndUsesPOL(t *testing.T) {
	var signed types.Transaction
	c := mockRPC(t, func(method string, params json.RawMessage) any {
		switch method {
		case "eth_chainId":
			return "0x13882"
		case "eth_getBlockByNumber":
			return &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, BaseFee: big.NewInt(1000000000)}
		case "eth_maxPriorityFeePerGas":
			return "0x3b9aca00"
		case "eth_getTransactionCount":
			return "0x0"
		case "eth_getBalance":
			return "0xde0b6b3a76400000"
		case "eth_estimateGas":
			return "0x5208"
		case "eth_sendRawTransaction":
			var p []string
			_ = json.Unmarshal(params, &p)
			raw, err := hexutil.Decode(p[0])
			if err != nil {
				t.Error(err)
			}
			if err := signed.UnmarshalBinary(raw); err != nil {
				t.Error(err)
			}
			return signed.Hash().Hex()
		default:
			return nil
		}
	}, 80002)
	s, err := NewService(c, t.TempDir(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Create("test-password-123"); err != nil {
		t.Fatal(err)
	}
	q, err := s.Quote(context.Background(), &QuoteRequest{Action: "eth", To: "0x2222222222222222222222222222222222222222", Amount: "0.01"})
	if err != nil {
		t.Fatal(err)
	}
	if q.Symbol != "POL" {
		t.Fatal("quote mislabeled", q.Symbol)
	}
	first, err := s.Send(context.Background(), q.ID, "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if signed.ChainId().Int64() != 80002 || signed.Value().Cmp(big.NewInt(10000000000000000)) != 0 {
		t.Fatal("wrong transaction chain/value")
	}
	qSpeed, err := s.Quote(context.Background(), &QuoteRequest{Action: "speedup", Hash: first.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if qSpeed.Symbol != "POL" {
		t.Fatalf("speedup quote mislabeled: expected POL, got %s", qSpeed.Symbol)
	}
	qCancel, err := s.Quote(context.Background(), &QuoteRequest{Action: "cancel", Hash: first.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if qCancel.Symbol != "POL" {
		t.Fatalf("cancel quote mislabeled: expected POL, got %s", qCancel.Symbol)
	}
}
