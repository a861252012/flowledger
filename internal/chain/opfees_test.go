package chain

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestOPFeesIncludeDataAndHistoricalOperatorCharge(t *testing.T) {
	for _, id := range []int64{84532, 11155420} {
		t.Run(big.NewInt(id).String(), func(t *testing.T) {
			receipt := &types.Receipt{TxHash: common.HexToHash("0x1234"), BlockHash: common.HexToHash("0x5678"), GasUsed: 21000, EffectiveGasPrice: big.NewInt(7)}
			badReceipt := false
			historicalCalls := 0
			c := rpcClient(t, func(method string, params json.RawMessage) any {
				switch method {
				case "eth_chainId":
					return hexutil.EncodeBig(big.NewInt(id))
				case "eth_getTransactionReceipt":
					hash := receipt.BlockHash
					if badReceipt {
						hash = common.Hash{}
					}
					return map[string]any{"transactionHash": receipt.TxHash, "blockHash": hash, "l1Fee": "0x64"}
				case "eth_call":
					var args []json.RawMessage
					json.Unmarshal(params, &args)
					if strings.Contains(string(args[1]), receipt.BlockHash.Hex()) {
						historicalCalls += 1
					}
					var call map[string]string
					json.Unmarshal(args[0], &call)
					input := call["input"]
					if input == "" {
						input = call["data"]
					}
					n := int64(3)
					if strings.HasPrefix(input, hexutil.Encode(crypto.Keccak256([]byte("getL1FeeUpperBound(uint256)"))[:4])) {
						n = 100
					}
					return hexutil.Encode(common.LeftPadBytes(big.NewInt(n).Bytes(), 32))
				}
				t.Errorf("unexpected method %s", method)
				return nil
			})
			c.chainID = id
			to := common.HexToAddress("0x1111111111111111111111111111111111111111")
			tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(id), To: &to, Gas: 21000, GasFeeCap: big.NewInt(10), GasTipCap: big.NewInt(1), Value: big.NewInt(1)})
			estimate, err := c.RollupFee(context.Background(), tx)
			if err != nil || estimate.Int64() != 103 {
				t.Fatalf("estimate %v %v", estimate, err)
			}
			actual, err := c.ReceiptFee(context.Background(), receipt)
			if err != nil || actual.Int64() != 147103 || historicalCalls != 1 {
				t.Fatalf("actual %v %v historical=%d", actual, err, historicalCalls)
			}
			badReceipt = true
			if _, err := c.ReceiptFee(context.Background(), receipt); err == nil {
				t.Fatal("accepted mismatched receipt block")
			}
		})
	}
}

func TestAdditionalNetworksAreExplicitlyAllowlisted(t *testing.T) {
	for _, id := range []int64{11155111, 421614, 84532, 11155420} {
		c, err := NewNetwork(id, []string{"http://127.0.0.1:1"})
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	}
	for _, id := range []int64{1, 8453, 10, 42161, 0} {
		if c, err := NewNetwork(id, []string{"http://127.0.0.1:1"}); err == nil {
			c.Close()
			t.Fatalf("accepted mainnet/unknown %d", id)
		}
	}
}

func TestReceiptFeeCanceledContext(t *testing.T) {
	c := rpcClient(t, func(method string, _ json.RawMessage) any {
		t.Errorf("canceled receipt query reached RPC: %s", method)
		return nil
	})
	c.chainID = 84532
	receipt := &types.Receipt{
		TxHash: common.HexToHash("0x1234"), BlockHash: common.HexToHash("0x5678"),
		GasUsed: 21000, EffectiveGasPrice: big.NewInt(7),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ReceiptFee(ctx, receipt); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("public cancellation error changed: %v", err)
	}
}
