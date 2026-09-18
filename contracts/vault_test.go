package contracts

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"
	"math/big"
	"os"
	"strings"
	"testing"
)

func TestVaultIndependentAcceptance(t *testing.T) {
	ctx := context.Background()
	aliceKey, _ := crypto.GenerateKey()
	bobKey, _ := crypto.GenerateKey()
	alice, _ := bind.NewKeyedTransactorWithChainID(aliceKey, big.NewInt(1337))
	bob, _ := bind.NewKeyedTransactorWithChainID(bobKey, big.NewInt(1337))
	chain := simulated.NewBackend(types.GenesisAlloc{alice.From: {Balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(21), nil)}, bob.From: {Balance: new(big.Int).Exp(big.NewInt(10), big.NewInt(21), nil)}})
	defer chain.Close()
	client := chain.Client()
	deploy := func(name string, args ...any) (common.Address, *bind.BoundContract) {
		t.Helper()
		raw, err := os.ReadFile("artifacts/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var artifact struct {
			ABI        json.RawMessage `json:"abi"`
			Bytecode   string          `json:"bytecode"`
			SourceFile string          `json:"sourceFile"`
			SourceHash string          `json:"sourceHash"`
		}
		if err = json.Unmarshal(raw, &artifact); err != nil {
			t.Fatal(err)
		}
		source, err := os.ReadFile(artifact.SourceFile)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(source)) != artifact.SourceHash {
			t.Fatal("stale contract artifact; run npm run compile --prefix contracts")
		}
		parsed, err := abi.JSON(strings.NewReader(string(artifact.ABI)))
		if err != nil {
			t.Fatal(err)
		}
		addr, tx, c, err := bind.DeployContract(alice, parsed, common.FromHex(artifact.Bytecode), client, args...)
		if err != nil {
			t.Fatal(err)
		}
		chain.Commit()
		receipt, err := client.TransactionReceipt(ctx, tx.Hash())
		if err != nil || receipt.Status != 1 {
			t.Fatalf("deploy %s: %v", name, err)
		}
		return addr, c
	}
	send := func(c *bind.BoundContract, signer *bind.TransactOpts, value int64, want uint64, method string, args ...any) *types.Receipt {
		t.Helper()
		opts := *signer
		opts.Value = big.NewInt(value)
		opts.GasLimit = 700000
		tx, err := c.Transact(&opts, method, args...)
		if err != nil {
			t.Fatal(err)
		}
		chain.Commit()
		r, err := client.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != want {
			t.Fatalf("%s status=%d want=%d", method, r.Status, want)
		}
		return r
	}
	number := func(c *bind.BoundContract, method string, args ...any) *big.Int {
		t.Helper()
		var result []any
		if err := c.Call(&bind.CallOpts{Context: ctx}, &result, method, args...); err != nil {
			t.Fatal(err)
		}
		return result[0].(*big.Int)
	}
	addr, vault := deploy("ETHVault")
	r := send(vault, alice, 1000, 1, "deposit")
	if len(r.Logs) != 1 || r.Logs[0].Address != addr || r.Logs[0].Topics[0] != crypto.Keccak256Hash([]byte("Deposited(address,uint256)")) || r.Logs[0].Topics[1] != common.BytesToHash(alice.From.Bytes()) || new(big.Int).SetBytes(r.Logs[0].Data).Int64() != 1000 {
		t.Fatal("deposit event mismatched")
	}
	send(vault, bob, 600, 1, "deposit")
	send(vault, bob, 0, 0, "withdraw", big.NewInt(601))
	if number(vault, "balanceOf", alice.From).Int64() != 1000 || number(vault, "balanceOf", bob.From).Int64() != 600 {
		t.Fatal("account isolation/failed withdrawal")
	}
	withdrawal := send(vault, alice, 0, 1, "withdraw", big.NewInt(400))
	if len(withdrawal.Logs) != 1 || withdrawal.Logs[0].Topics[0] != crypto.Keccak256Hash([]byte("Withdrawn(address,uint256)")) || new(big.Int).SetBytes(withdrawal.Logs[0].Data).Int64() != 400 {
		t.Fatal("withdrawal event mismatch")
	}
	send(vault, alice, 0, 0, "deposit")
	send(vault, alice, 0, 0, "withdraw", big.NewInt(0))
	rejectAddr, reject := deploy("RejectingReceiver", addr)
	send(reject, alice, 300, 1, "deposit")
	send(reject, alice, 0, 0, "withdraw", big.NewInt(300))
	if number(vault, "balanceOf", rejectAddr).Int64() != 300 {
		t.Fatal("rejected transfer did not restore balance")
	}
	attackerAddr, attacker := deploy("ReentrancyAttacker", addr)
	send(attacker, alice, 500, 1, "deposit")
	send(attacker, alice, 0, 1, "attack", big.NewInt(250))
	var flags []any
	if err := attacker.Call(&bind.CallOpts{}, &flags, "attackAttempted"); err != nil || flags[0] != true {
		t.Fatal("attack not attempted", err)
	}
	flags = nil
	if err := attacker.Call(&bind.CallOpts{}, &flags, "attackSucceeded"); err != nil || flags[0] != false {
		t.Fatal("reentry succeeded", err)
	}
	if number(vault, "balanceOf", attackerAddr).Int64() != 250 || number(vault, "balanceOf", alice.From).Int64() != 600 || number(vault, "balanceOf", bob.From).Int64() != 600 {
		t.Fatal("post-attack balances")
	}
	total, err := client.BalanceAt(ctx, addr, nil)
	if err != nil || total.Int64() != 1750 {
		t.Fatalf("vault balance=%v err=%v", total, err)
	}
}
