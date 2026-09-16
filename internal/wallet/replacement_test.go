package wallet

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestReplacementPreservesNonceAndRequiresFreshQuote(t *testing.T) {
	for _, chainID := range []int64{11155111, 421614, 84532, 11155420} {
		for _, action := range []string{"speedup", "cancel"} {
			t.Run(action, func(t *testing.T) {
				var sent []*types.Transaction
				head := &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, BaseFee: big.NewInt(1000000000)}
				c := mockRPC(t, func(method string, params json.RawMessage) any {
					switch method {
					case "eth_call":
						return "0x" + strings.Repeat("0", 63) + "1"
					case "eth_chainId":
						return hexutil.EncodeBig(big.NewInt(chainID))
					case "eth_getBlockByNumber":
						return head
					case "eth_maxPriorityFeePerGas":
						return "0x3b9aca00"
					case "eth_getTransactionCount":
						return "0x0"
					case "eth_getBalance":
						return "0xde0b6b3a76400000"
					case "eth_estimateGas":
						return "0x5208"
					case "eth_sendRawTransaction":
						var args []string
						json.Unmarshal(params, &args)
						raw, _ := hexutil.Decode(args[0])
						tx := new(types.Transaction)
						if err := tx.UnmarshalBinary(raw); err != nil {
							t.Error(err)
							return nil
						}
						sent = append(sent, tx)
						return tx.Hash().Hex()
					}
					return nil
				}, chainID)
				svc, err := NewService(c, t.TempDir(), 2, 1)
				if err != nil {
					t.Fatal(err)
				}
				defer svc.Close()
				created, err := svc.Create("test-password-123")
				if err != nil {
					t.Fatal(err)
				}
				quote, err := svc.Quote(context.Background(), &QuoteRequest{Action: "eth", To: "0x2222222222222222222222222222222222222222", Amount: "0.000001"})
				if err != nil {
					t.Fatal(err)
				}
				first, err := svc.Send(context.Background(), quote.ID, "test-password-123")
				if err != nil {
					t.Fatal(err)
				}
				q1, err := svc.Quote(context.Background(), &QuoteRequest{Action: action, Hash: first.Hash})
				if err != nil {
					t.Fatal(err)
				}
				q2, err := svc.Quote(context.Background(), &QuoteRequest{Action: action, Hash: first.Hash})
				if err != nil {
					t.Fatal(err)
				}
				second, err := svc.Send(context.Background(), q1.ID, "test-password-123")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := svc.Send(context.Background(), q2.ID, "test-password-123"); err == nil {
					t.Fatal("accepted stale replacement quote")
				}
				if len(sent) != 2 || sent[1].ChainId().Int64() != chainID || sent[0].Nonce() != sent[1].Nonce() {
					t.Fatal("replacement nonce/broadcast mismatch")
				}
				if sent[1].GasFeeCap().Cmp(sent[0].GasFeeCap()) <= 0 || sent[1].GasTipCap().Cmp(sent[0].GasTipCap()) <= 0 {
					t.Fatal("fees not increased")
				}
				if action == "speedup" && (*sent[1].To() != *sent[0].To() || sent[1].Value().Cmp(sent[0].Value()) != 0) {
					t.Fatal("speedup changed intent")
				}
				if action == "cancel" && (sent[1].To().Hex() != created.Address || sent[1].Value().Sign() != 0 || len(sent[1].Data()) != 0) {
					t.Fatal("cancel is not zero self transfer")
				}
				if !svc.journal.HasInFlightTx() {
					t.Fatal("broadcast treated as cancellation success")
				}
				if err := svc.journal.UpdateStateAtomic(second.Hash, "succeeded", "1", "0.0001", ""); err != nil {
					t.Fatal(err)
				}
				if svc.journal.HasInFlightTx() {
					t.Fatal("mined replacement did not resolve nonce")
				}
				if err := svc.journal.UpdateStateAtomic(second.Hash, "reorg_detected", "", "", ""); err != nil {
					t.Fatal(err)
				}
				if !svc.journal.HasInFlightTx() {
					t.Fatal("reorg did not restore unresolved nonce")
				}
			})
		}
	}

}
