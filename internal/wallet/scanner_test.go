package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"os"
	"path/filepath"
	"testing"
)

func TestScannerPersistsCursorOnlyAfterSuccessfulBlock(t *testing.T) {
	head := &types.Header{Number: big.NewInt(100), Difficulty: big.NewInt(0), GasLimit: 30000000, BaseFee: big.NewInt(1)}
	fail := true
	client := mockRPC(t, func(method string, params json.RawMessage) any {
		switch method {
		case "eth_chainId":
			return "0xaa36a7"
		case "eth_getBlockByNumber":
			var args []any
			json.Unmarshal(params, &args)
			if len(args) > 1 && args[1] == true {
				if fail {
					return errors.New("unavailable")
				}
				return map[string]any{"hash": head.Hash(), "transactions": []any{}}
			}
			return head
		case "eth_getBlockReceipts":
			return []any{}
		}
		return nil
	})
	dir := t.TempDir()
	svc, err := NewService(client, dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create("test-password-123"); err != nil {
		t.Fatal(err)
	}
	start := uint64(100)
	if _, err := svc.ConfigureScan(true, &start); err != nil {
		t.Fatal(err)
	}
	if err := svc.ScanOnce(context.Background()); err == nil {
		t.Fatal("failed RPC was ignored")
	}
	state, _ := svc.ScanProgress()
	if state.Next != 100 || state.Error == "" {
		t.Fatal("failure advanced cursor")
	}
	fail = false
	if err := svc.ScanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, _ = svc.ScanProgress()
	if state.Next != 101 || state.Error != "" {
		t.Fatal("success did not advance cursor")
	}
	svc.Close()
	restarted, err := NewService(client, dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	state, err = restarted.ScanProgress()
	if err != nil || state.Next != 101 || !state.Enabled {
		t.Fatal("lost persisted cursor")
	}
}

func TestScanStorageBoundaryPreservesNullTokens(t *testing.T) {
	dir := t.TempDir()
	client := mockRPC(t, func(method string, _ json.RawMessage) any {
		if method == "eth_chainId" {
			return "0xaa36a7"
		}
		return nil
	})
	svc, err := NewService(client, dir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	path := filepath.Join(dir, "scan.json")
	data := []byte(`{"enabled":false,"start":0,"next":0,"finalized":0,"tokens":null,"updatedAt":"0001-01-01T00:00:00Z"}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := svc.ScanProgress()
	if err != nil || state.Tokens != nil {
		t.Fatalf("null token semantics changed: %+v %v", state, err)
	}
	if _, err := svc.ConfigureScan(false, nil); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(saved, []byte(`"tokens":null`)) {
		t.Fatalf("null tokens changed on round trip: %s", saved)
	}
}
