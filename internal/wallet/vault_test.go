package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a861252012/testnet-wallet-lab/internal/chain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestVaultConfigAndInheritance(t *testing.T) {
	dir := t.TempDir()
	sepoliaClient := mockRPC(t, func(method string, params json.RawMessage) any {
		return "0x0"
	}, chain.SepoliaID)

	svc, err := NewService(sepoliaClient, filepath.Join(dir, "root"), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()

	const testVault = "0x1111111111111111111111111111111111111111"
	if err := svc.SetVaultAddress(testVault); err != nil {
		t.Fatalf("SetVaultAddress: %v", err)
	}
	if got := svc.VaultAddress(); got != testVault {
		t.Fatalf("VaultAddress: got %q, want %q", got, testVault)
	}

	// Test subaccount inheritance
	account, err := svc.AddAccount("子錢包", "test-pass-1234")
	if err != nil {
		t.Fatal(err)
	}
	subSvc, err := NewAccountService(sepoliaClient, svc, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer subSvc.Close()

	if got := subSvc.VaultAddress(); got != testVault {
		t.Fatalf("subaccount VaultAddress: got %q, want %q", got, testVault)
	}

	// Non-sepolia client should disable vault
	otherClient := mockRPC(t, func(method string, params json.RawMessage) any {
		return "0x0"
	}, 84532) // Base Sepolia
	otherSvc, err := NewService(otherClient, filepath.Join(dir, "base"), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer otherSvc.Close()

	if err := otherSvc.SetVaultAddress(testVault); err != nil {
		t.Fatal(err)
	}
	if got := otherSvc.VaultAddress(); got != "" {
		t.Fatalf("non-Sepolia VaultAddress: got %q, want empty", got)
	}
}

func TestVaultStatus(t *testing.T) {
	dir := t.TempDir()
	const testVault = "0x1111111111111111111111111111111111111111"

	t.Run("Disabled when vaultAddress is empty", func(t *testing.T) {
		c := mockRPC(t, func(method string, params json.RawMessage) any {
			return "0x0"
		}, chain.SepoliaID)
		svc, err := NewService(c, filepath.Join(dir, "w1"), 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		defer svc.Close()

		status, err := svc.VaultStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if status.Enabled || status.Contract != "" || status.Balance != "" || status.BalanceRaw != "" {
			t.Fatalf("expected disabled vault status, got %#v", status)
		}
	})

	t.Run("Disabled on non-Sepolia network", func(t *testing.T) {
		c := mockRPC(t, func(method string, params json.RawMessage) any {
			return "0x0"
		}, 84532)
		svc, err := NewService(c, filepath.Join(dir, "w2"), 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		defer svc.Close()
		_ = svc.SetVaultAddress(testVault)

		status, err := svc.VaultStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if status.Enabled {
			t.Fatalf("expected disabled on non-Sepolia, got %#v", status)
		}
	})

	t.Run("Enabled returns balanceOf", func(t *testing.T) {
		c := mockRPC(t, func(method string, params json.RawMessage) any {
			switch method {
			case "eth_chainId":
				return "0xaa36a7"
			case "eth_getCode":
				return "0x60806040" // non-empty bytecode
			case "eth_call":
				// return 1.5 ETH (1500000000000000000 = 0x14d1120d7b160000)
				val := new(big.Int)
				val.SetString("1500000000000000000", 10)
				return hexutil.Encode(common.LeftPadBytes(val.Bytes(), 32))
			default:
				return nil
			}
		}, chain.SepoliaID)
		svc, err := NewService(c, filepath.Join(dir, "w3"), 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		defer svc.Close()
		if err := svc.SetVaultAddress(testVault); err != nil {
			t.Fatal(err)
		}
		_, err = svc.keystore.Create("test-password-12345")
		if err != nil {
			t.Fatal(err)
		}

		status, err := svc.VaultStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !status.Enabled {
			t.Fatalf("expected enabled, got %#v", status)
		}
		if status.Contract != testVault {
			t.Fatalf("expected contract %q, got %q", testVault, status.Contract)
		}
		if status.Balance != "1.5" || status.BalanceRaw != "1500000000000000000" {
			t.Fatalf("unexpected balance: %q (%q)", status.Balance, status.BalanceRaw)
		}
	})
}

func TestVaultQuoteAndSend(t *testing.T) {
	dir := t.TempDir()
	const testVault = "0x1111111111111111111111111111111111111111"

	var sent []*types.Transaction
	c := mockRPC(t, func(method string, params json.RawMessage) any {
		switch method {
		case "eth_chainId":
			return "0xaa36a7" // 11155111
		case "eth_getCode":
			return "0x60806040"
		case "eth_call":
			// balanceOf or simulation: return 2 ETH for balanceOf
			val := new(big.Int)
			val.SetString("2000000000000000000", 10)
			return hexutil.Encode(common.LeftPadBytes(val.Bytes(), 32))
		case "eth_getBlockByNumber":
			return &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, BaseFee: big.NewInt(1000000000)}
		case "eth_maxPriorityFeePerGas":
			return "0x3b9aca00"
		case "eth_estimateGas":
			return "0x5208" // 21000
		case "eth_getTransactionCount":
			return "0x1"
		case "eth_getBalance":
			return "0x1bc16d674ec80000" // 2 ETH
		case "eth_sendRawTransaction":
			var raw []hexutil.Bytes
			if err := json.Unmarshal(params, &raw); err != nil || len(raw) != 1 {
				t.Errorf("invalid raw transaction params: %v", err)
				return nil
			}
			var tx types.Transaction
			if err := tx.UnmarshalBinary(raw[0]); err != nil {
				t.Errorf("decode signed transaction: %v", err)
				return nil
			}
			sent = append(sent, &tx)
			return tx.Hash().Hex()
		default:
			return nil
		}
	}, chain.SepoliaID)

	svc, err := NewService(c, filepath.Join(dir, "w"), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if err := svc.SetVaultAddress(testVault); err != nil {
		t.Fatal(err)
	}
	created, err := svc.keystore.Create("test-password-12345")
	if err != nil {
		t.Fatal(err)
	}
	walletAddr := created.Address

	ctx := context.Background()

	// Client cannot specify contract
	_, err = svc.Quote(ctx, &QuoteRequest{
		Action:   "vault_deposit",
		To:       walletAddr,
		Amount:   "0.5",
		Contract: "0x2222222222222222222222222222222222222222",
	})
	if err == nil || err.Error() != "存款箱操作不得由客戶端指定 contract 合約地址" {
		t.Fatalf("expected contract reject, got %v", err)
	}

	// Client cannot specify mismatching to
	_, err = svc.Quote(ctx, &QuoteRequest{
		Action: "vault_deposit",
		To:     "0x3333333333333333333333333333333333333333",
		Amount: "0.5",
	})
	if err == nil || err.Error() != "存款箱目標地址不符" {
		t.Fatalf("expected mismatching to reject, got %v", err)
	}

	// Normal deposit quote
	depQuote, err := svc.Quote(ctx, &QuoteRequest{
		Action: "vault_deposit",
		To:     walletAddr,
		Amount: "0.5",
	})
	if err != nil {
		t.Fatalf("deposit quote failed: %v", err)
	}
	if depQuote.Contract != testVault {
		t.Fatalf("quote Contract: got %q, want %q", depQuote.Contract, testVault)
	}
	if depQuote.Method != "deposit()" {
		t.Fatalf("quote Method: got %q, want deposit()", depQuote.Method)
	}
	if depQuote.Symbol != "ETH" {
		t.Fatalf("quote Symbol: got %q, want ETH", depQuote.Symbol)
	}

	// Send deposit
	depResp, err := svc.Send(ctx, depQuote.ID, "test-password-12345")
	if err != nil {
		t.Fatalf("send deposit failed: %v", err)
	}
	if depResp.Hash == "" {
		t.Fatal("expected tx hash")
	}
	if err := svc.journal.UpdateStateAtomic(depResp.Hash, "succeeded", "1", "0.0001", ""); err != nil {
		t.Fatal(err)
	}

	// Normal withdraw quote
	withQuote, err := svc.Quote(ctx, &QuoteRequest{
		Action: "vault_withdraw",
		To:     walletAddr,
		Amount: "0.2",
	})
	if err != nil {
		t.Fatalf("withdraw quote failed: %v", err)
	}
	if withQuote.Contract != testVault {
		t.Fatalf("quote Contract: got %q, want %q", withQuote.Contract, testVault)
	}
	if withQuote.Method != "withdraw(uint256)" {
		t.Fatalf("quote Method: got %q, want withdraw(uint256)", withQuote.Method)
	}

	// Send withdraw
	withResp, err := svc.Send(ctx, withQuote.ID, "test-password-12345")
	if err != nil {
		t.Fatalf("send withdraw failed: %v", err)
	}
	if withResp.Hash == "" {
		t.Fatal("expected tx hash")
	}
	if len(sent) != 2 {
		t.Fatalf("sent %d transactions, want 2", len(sent))
	}
	for i, tx := range sent {
		if tx.To() == nil || *tx.To() != common.HexToAddress(testVault) || tx.ChainId().Uint64() != chain.SepoliaID {
			t.Fatalf("transaction %d has wrong target or chain", i)
		}
	}
	if sent[0].Value().Cmp(big.NewInt(500000000000000000)) != 0 || sent[1].Value().Sign() != 0 {
		t.Fatal("deposit must transfer principal; withdrawal must send zero ETH")
	}
	if depResp.Hash != sent[0].Hash().Hex() || withResp.Hash != sent[1].Hash().Hex() {
		t.Fatal("response hash does not match signed transaction")
	}
}

func TestVaultAdversarialCases(t *testing.T) {
	dir := t.TempDir()
	const testVault = "0x1111111111111111111111111111111111111111"

	var callErr error
	var getCodeRes = "0x60806040"
	var vaultBal = big.NewInt(1000000000000000000) // 1 ETH

	c := mockRPC(t, func(method string, params json.RawMessage) any {
		switch method {
		case "eth_chainId":
			return "0xaa36a7"
		case "eth_getCode":
			return getCodeRes
		case "eth_call":
			var args []json.RawMessage
			_ = json.Unmarshal(params, &args)
			if len(args) > 0 {
				var msg struct {
					Input string `json:"input"`
					Data  string `json:"data"`
				}
				_ = json.Unmarshal(args[0], &msg)
				dataPayload := msg.Data
				if dataPayload == "" {
					dataPayload = msg.Input
				}
				if strings.HasPrefix(dataPayload, "0x2e1a7d4d") && callErr != nil {
					return callErr
				}
			}
			return hexutil.Encode(common.LeftPadBytes(vaultBal.Bytes(), 32))
		case "eth_getBlockByNumber":
			return &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, BaseFee: big.NewInt(1000000000)}
		case "eth_maxPriorityFeePerGas":
			return "0x3b9aca00"
		case "eth_estimateGas":
			return "0x5208"
		case "eth_getTransactionCount":
			return "0x1"
		case "eth_getBalance":
			return "0x1bc16d674ec80000" // 2 ETH
		case "eth_sendRawTransaction":
			return "0x9999999999999999999999999999999999999999999999999999999999999999"
		default:
			return nil
		}
	}, chain.SepoliaID)

	svc, err := NewService(c, filepath.Join(dir, "adv"), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	_ = svc.SetVaultAddress(testVault)
	created, _ := svc.keystore.Create("test-password-12345")
	walletAddr := created.Address
	ctx := context.Background()

	// 1. Zero amount deposit rejection
	_, err = svc.Quote(ctx, &QuoteRequest{
		Action: "vault_deposit",
		To:     walletAddr,
		Amount: "0",
	})
	if err == nil || err.Error() != "存款金額必須大於 0" {
		t.Fatalf("expected zero deposit reject, got %v", err)
	}

	// 2. Zero amount withdraw rejection
	_, err = svc.Quote(ctx, &QuoteRequest{
		Action: "vault_withdraw",
		To:     walletAddr,
		Amount: "0",
	})
	if err == nil || err.Error() != "提款金額必須大於 0" {
		t.Fatalf("expected zero withdraw reject, got %v", err)
	}

	// 3. Non-contract bytecode rejection on Send
	qDep, err := svc.Quote(ctx, &QuoteRequest{
		Action: "vault_deposit",
		To:     walletAddr,
		Amount: "0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	getCodeRes = "0x" // Empty bytecode
	_, err = svc.Send(ctx, qDep.ID, "test-password-12345")
	if !errors.Is(err, ErrNotContract) {
		t.Fatalf("expected ErrNotContract, got %v", err)
	}
	getCodeRes = "0x60806040" // Restore

	// 4. Send withdraw with insufficient vault balance
	qWith, err := svc.Quote(ctx, &QuoteRequest{
		Action: "vault_withdraw",
		To:     walletAddr,
		Amount: "0.5", // Balance will change after quoting
	})
	if err != nil {
		t.Fatal(err)
	}
	vaultBal = big.NewInt(1)
	_, err = svc.Send(ctx, qWith.ID, "test-password-12345")
	if err == nil || err.Error() != "存款箱餘額不足以提款" {
		t.Fatalf("expected insufficient vault balance, got %v", err)
	}

	vaultBal = big.NewInt(1000000000000000000)
	// 5. Simulation failure on Send
	qValidWith, err := svc.Quote(ctx, &QuoteRequest{
		Action: "vault_withdraw",
		To:     walletAddr,
		Amount: "0.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	callErr = errors.New("execution reverted: TransferFailed")
	_, err = svc.Send(ctx, qValidWith.ID, "test-password-12345")
	if err == nil || err.Error() != "存款箱合約模擬執行失敗，未送出交易" {
		t.Fatalf("expected simulation revert, got %v", err)
	}
	callErr = nil

	// 6. Wrong password
	_, err = svc.Send(ctx, qValidWith.ID, "wrong-password-1234")
	if !errors.Is(err, ErrPasswordMismatch) {
		t.Fatalf("expected ErrPasswordMismatch, got %v", err)
	}
}
