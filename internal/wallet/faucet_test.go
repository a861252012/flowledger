package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestFaucetRealSignerBoundsAndRestartDeduplication(t *testing.T) {
	var broadcasts atomic.Int32
	var signed types.Transaction
	head := &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, BaseFee: big.NewInt(1000000000)}
	c := mockRPC(t, func(method string, params json.RawMessage) any {
		switch method {
		case "eth_chainId":
			return "0xaa36a7"
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
			broadcasts.Add(1)
			var args []string
			_ = json.Unmarshal(params, &args)
			raw, _ := hexutil.Decode(args[0])
			if err := signed.UnmarshalBinary(raw); err != nil {
				t.Error(err)
			}
			return errors.New("response lost after accepted broadcast")
		default:
			return nil
		}
	})
	root, err := NewService(c, t.TempDir(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	receiver, err := root.Create("test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source, err := NewService(c, dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Create("test-password-123"); err != nil {
		t.Fatal(err)
	}
	f := &TestFaucet{Root: root, Sources: map[int64]*Service{11155111: source}, Password: "test-password-123"}
	ctx := context.Background()
	for _, req := range []struct {
		id             int64
		asset, address string
	}{
		{1, "native", receiver.Address}, {11155111, "anything", receiver.Address}, {11155111, "native", "0x2222222222222222222222222222222222222222"},
	} {
		if _, err := f.Claim(ctx, req.id, req.asset, req.address); err == nil {
			t.Fatal("unbounded claim accepted")
		}
	}
	if broadcasts.Load() != 0 {
		t.Fatal("rejected claim broadcast")
	}
	// Overpriced fees are rejected before signing.
	head.BaseFee = big.NewInt(1000000000000)
	if _, err := f.Claim(ctx, 11155111, "native", receiver.Address); err == nil {
		t.Fatal("excessive fee accepted")
	}
	if broadcasts.Load() != 0 {
		t.Fatal("overpriced transaction broadcast")
	}
	head.BaseFee = big.NewInt(1000000000)
	result, err := f.Claim(ctx, 11155111, "native", receiver.Address)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "broadcast_unknown" || signed.ChainId().Int64() != 11155111 || signed.Value().Cmp(big.NewInt(1000000000000000)) != 0 || signed.To().Hex() != receiver.Address {
		t.Fatalf("unexpected signed claim: %+v", result)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	source, err = NewService(c, dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	f = &TestFaucet{Root: root, Sources: map[int64]*Service{11155111: source}, Password: "test-password-123"}
	again, err := f.Claim(ctx, 11155111, "native", receiver.Address)
	if err != nil || again.Hash != result.Hash || !again.Reused || broadcasts.Load() != 1 {
		t.Fatalf("claim not recovered after restart: %+v %v", again, err)
	}
}

func TestSOLFaucetRejectsWrongGenesisAndLimitsAmount(t *testing.T) {
	genesis := "wrong-network"
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any               `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "getGenesisHash":
			result = genesis
		case "requestAirdrop":
			calls += 1
			var amount uint64
			_ = json.Unmarshal(req.Params[1], &amount)
			if amount != 10000000 {
				t.Error("incorrect airdrop amount", amount)
			}
			result = "1111111111111111111111111111111111111111111111111111111111111111"
		default:
			t.Error("unexpected RPC", req.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	s, err := NewSolanaService(server.URL, t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Create("", "test-password-123"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestTestSOL(context.Background()); err == nil || calls != 0 {
		t.Fatal("wrong genesis requested airdrop")
	}
	genesis = SolanaDevnetGenesis
	if _, err = s.RequestTestSOL(context.Background()); err != nil || calls != 1 {
		t.Fatal("airdrop", err, calls)
	}
}

func TestTronFaucetUsesOwnedReceiverAndRetainsUnknownBroadcast(t *testing.T) {
	fixture := newTronFixture(t)
	receiver, err := NewTronService(fixture.service.endpoint, "", t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	created, err := receiver.Create("", "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	faucet := &TestFaucet{TronSource: fixture.service, TronRecipient: receiver, TronPassword: "fixture-password"}
	fixture.wrongNetwork = true
	if _, err := faucet.ClaimTRX(context.Background()); err == nil || fixture.broadcasts != 0 {
		t.Fatal("wrong network accepted")
	}
	fixture.wrongNetwork = false
	fixture.broadcastFail = true
	first, err := faucet.ClaimTRX(context.Background())
	if err != nil || first.State != "broadcast_unknown" || first.To != created.Address || first.Amount != "5" {
		t.Fatalf("unexpected claim %v %v", first, err)
	}
	second, err := faucet.ClaimTRX(context.Background())
	if err != nil || second.Signature != first.Signature || !second.Reused || fixture.broadcasts != 1 {
		t.Fatal("duplicate claim broadcast", err)
	}

	for _, failedState := range []string{"reverted", "execution_failed", "expired_unconfirmed"} {
		fixture.service.mu.Lock()
		for i := range fixture.service.records {
			fixture.service.records[i].State = crosschainState(failedState)
			if failedState != "expired_unconfirmed" {
				fixture.service.records[i].Finalized = true
			} else {
				fixture.service.records[i].Finalized = false
			}
		}
		fixture.service.mu.Unlock()
		reclaimed, err := faucet.ClaimTRX(context.Background())
		if err != nil {
			t.Fatalf("failed state %s blocked reclaim: %v", failedState, err)
		}
		if reclaimed.Reused {
			t.Fatalf("failed state %s was reused", failedState)
		}
	}
}

func TestFaucetAllowsReclaimOnFailedEVMTransactionsAndSepoliaUSDCFeeLimit(t *testing.T) {
	var broadcasts atomic.Int32
	var signed types.Transaction
	head := &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, BaseFee: big.NewInt(2000000000)} // 2 gwei
	reverted := false
	c := mockRPC(t, func(method string, params json.RawMessage) any {
		switch method {
		case "eth_chainId":
			return "0xaa36a7"
		case "eth_getBlockByNumber":
			return head
		case "eth_maxPriorityFeePerGas":
			return "0x3b9aca00" // 1 gwei
		case "eth_getTransactionCount":
			if reverted {
				return "0x1"
			}
			return "0x0"
		case "eth_getBalance":
			return "0xde0b6b3a76400000"
		case "eth_getCode":
			return "0x6000"
		case "eth_getTransactionByHash":
			if !reverted {
				return nil
			}
			return map[string]any{
				"blockNumber":      "0x64",
				"blockHash":        head.Hash().Hex(),
				"transactionIndex": "0x0",
				"from":             "0x1111111111111111111111111111111111111111",
				"to":               "0x2222222222222222222222222222222222222222",
				"value":            "0x0",
				"gas":              "0x5208",
				"gasPrice":         "0x3b9aca00",
				"input":            "0x",
			}
		case "eth_getTransactionReceipt":
			if !reverted {
				return nil
			}
			return map[string]any{
				"status":            "0x0",
				"blockNumber":       "0x64",
				"blockHash":         head.Hash().Hex(),
				"transactionIndex":  "0x0",
				"gasUsed":           "0x5208",
				"effectiveGasPrice": "0x3b9aca00",
			}
		case "eth_call":
			var args []json.RawMessage
			_ = json.Unmarshal(params, &args)
			var call map[string]string
			_ = json.Unmarshal(args[0], &call)
			input := call["input"]
			if input == "" {
				input = call["data"]
			}
			data, _ := hexutil.Decode(input)
			if len(data) >= 4 {
				if method, err := erc20ABI.MethodById(data[:4]); err == nil {
					switch method.Name {
					case "symbol":
						res, _ := method.Outputs.Pack("USDC")
						return hexutil.Encode(res)
					case "decimals":
						res, _ := method.Outputs.Pack(uint8(6))
						return hexutil.Encode(res)
					case "balanceOf":
						res, _ := method.Outputs.Pack(big.NewInt(1000000000))
						return hexutil.Encode(res)
					case "transfer":
						res, _ := method.Outputs.Pack(true)
						return hexutil.Encode(res)
					}
				}
			}
			return "0x"
		case "eth_estimateGas":
			return "0xfe50" // 65104 gas for ERC20 transfer (total fee ~ 0.00026 ETH)
		case "eth_sendRawTransaction":
			broadcasts.Add(1)
			var args []string
			_ = json.Unmarshal(params, &args)
			raw, _ := hexutil.Decode(args[0])
			_ = signed.UnmarshalBinary(raw)
			return errors.New("accepted broadcast")
		default:
			return nil
		}
	})
	root, err := NewService(c, t.TempDir(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	receiver, err := root.Create("test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewService(c, t.TempDir(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.Create("test-password-123"); err != nil {
		t.Fatal(err)
	}
	f := &TestFaucet{Root: root, Sources: map[int64]*Service{11155111: source}, Password: "test-password-123"}
	ctx := context.Background()

	// 1. Sepolia USDC claim with ~0.00026 ETH fee succeeds under 0.0005 ETH limit
	result, err := f.Claim(ctx, 11155111, "usdc", receiver.Address)
	if err != nil {
		t.Fatalf("Sepolia USDC claim failed: %v", err)
	}
	if result.Action != "transfer" || result.Symbol != "USDC" {
		t.Fatalf("unexpected claim result: %+v", result)
	}

	// 2. Exact same claim while broadcast_unknown is reused
	reused, err := f.Claim(ctx, 11155111, "usdc", receiver.Address)
	if err != nil || !reused.Reused {
		t.Fatalf("expected reused claim: %+v %v", reused, err)
	}

	// 3. Mark previous transaction as reverted and verify EVM allows re-claiming (not reused)
	reverted = true
	source.journal.mu.Lock()
	for i := range source.journal.records {
		source.journal.records[i].State = "reverted"
		source.journal.records[i].Finalized = true
	}
	source.journal.mu.Unlock()

	reclaimed, err := f.Claim(ctx, 11155111, "usdc", receiver.Address)
	if err != nil {
		t.Fatalf("reverted state blocked reclaim: %v", err)
	}
	if reclaimed.Reused {
		t.Fatal("reverted state was improperly reused")
	}
}
