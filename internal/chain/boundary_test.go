package chain

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestBoundaryConversionsPreserveAPIJSON(t *testing.T) {
	checkedAt := time.Date(2026, 9, 16, 1, 2, 3, 0, time.UTC)
	blockTime := time.Date(2026, 9, 15, 4, 5, 6, 0, time.UTC)
	hash := common.HexToHash("0x1234")
	blockHash := common.HexToHash("0x5678")
	address := common.HexToAddress("0x1111111111111111111111111111111111111111")

	tests := []struct {
		name string
		got  any
		want string
	}{
		{
			name: "network",
			got: networkToAPI(networkSnapshot{
				chainID: SepoliaID, block: big.NewInt(12), blockTime: blockTime, checkedAt: checkedAt,
			}),
			want: `{"chainId":11155111,"block":"12","blockTime":"2026-09-15T04:05:06Z","checkedAt":"2026-09-16T01:02:03Z"}`,
		},
		{
			name: "balance",
			got: balanceToAPI(balanceSnapshot{
				address: address, wei: big.NewInt(1000000000000000001), block: big.NewInt(12), checkedAt: checkedAt,
			}),
			want: `{"address":"0x1111111111111111111111111111111111111111","wei":"1000000000000000001","eth":"1.000000000000000001","block":"12","checkedAt":"2026-09-16T01:02:03Z"}`,
		},
		{
			name: "transaction",
			got: transactionToAPI(transactionSnapshot{
				finalized: true, blockHash: blockHash, hash: hash, state: transactionSucceeded,
				block: big.NewInt(12), confirmations: big.NewInt(3), gasUsed: 21000,
				feeWei: big.NewInt(21000000000000), checkedAt: checkedAt,
			}),
			want: `{"finalized":true,"blockHash":"0x0000000000000000000000000000000000000000000000000000000000005678","hash":"0x0000000000000000000000000000000000000000000000000000000000001234","state":"succeeded","block":"12","confirmations":"3","gasUsed":"21000","feeEth":"0.000021","checkedAt":"2026-09-16T01:02:03Z"}`,
		},
		{
			name: "pending activity",
			got: activityToAPI(activityRecord{
				hash: hash, state: transactionPending, checkedAt: checkedAt, movements: []movementRecord{},
			}),
			want: `{"hash":"0x0000000000000000000000000000000000000000000000000000000000001234","state":"pending","checkedAt":"2026-09-16T01:02:03Z","movements":[]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := json.Marshal(test.got)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != test.want {
				t.Fatalf("JSON changed\ngot:  %s\nwant: %s", got, test.want)
			}
		})
	}
}

func TestRPCAndScannerStorageBoundaries(t *testing.T) {
	owner := common.HexToAddress("0x1111111111111111111111111111111111111111")
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	hash := common.HexToHash("0x1234")
	blockHash := common.HexToHash("0x5678")
	raw := `{"hash":"` + blockHash.Hex() + `","transactions":[{"hash":"` + hash.Hex() + `","from":"` + owner.Hex() + `","to":"` + to.Hex() + `"}]}`

	var block rpcBlock
	if err := json.Unmarshal([]byte(raw), &block); err != nil {
		t.Fatal(err)
	}
	domainBlock, err := rpcBlockToDomain(block)
	if err != nil {
		t.Fatal(err)
	}
	if domainBlock.hash != blockHash || len(domainBlock.transactions) != 1 || domainBlock.transactions[0].from != owner || *domainBlock.transactions[0].to != to {
		t.Fatalf("incorrect RPC boundary decode: %+v", block)
	}

	stored := scannedBlockToStorage(scannedBlockRecord{hash: blockHash, hashes: []common.Hash{hash}, tokens: []common.Address{to}})
	if stored.Hash != blockHash.Hex() || len(stored.Hashes) != 1 || stored.Hashes[0] != hash.Hex() || len(stored.Tokens) != 1 || stored.Tokens[0] != to.Hex() {
		t.Fatalf("incorrect scanner storage conversion: %+v", stored)
	}

	block.Transactions[0].From = "not-an-address"
	if _, err := rpcBlockToDomain(block); err != ErrUnavailable {
		t.Fatalf("malformed RPC address crossed boundary: %v", err)
	}
}

func TestOPReceiptRPCBoundary(t *testing.T) {
	hash := common.HexToHash("0x1234")
	blockHash := common.HexToHash("0x5678")
	valid := &opReceiptRPC{TransactionHash: hash.Hex(), BlockHash: blockHash.Hex(), L1Fee: new("0x64")}

	record, err := opReceiptToDomain(valid)
	if err != nil {
		t.Fatal(err)
	}
	if record.transactionHash != hash || record.blockHash != blockHash || record.l1Fee.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("incorrect OP receipt conversion: %+v", record)
	}

	tests := []struct {
		name  string
		value *opReceiptRPC
	}{
		{name: "missing receipt"},
		{name: "malformed transaction hash", value: &opReceiptRPC{TransactionHash: "0x12", BlockHash: blockHash.Hex(), L1Fee: new("0x64")}},
		{name: "malformed block hash", value: &opReceiptRPC{TransactionHash: hash.Hex(), BlockHash: "0x56", L1Fee: new("0x64")}},
		{name: "missing quantity", value: &opReceiptRPC{TransactionHash: hash.Hex(), BlockHash: blockHash.Hex()}},
		{name: "non-canonical quantity", value: &opReceiptRPC{TransactionHash: hash.Hex(), BlockHash: blockHash.Hex(), L1Fee: new("0x00")}},
		{name: "negative quantity", value: &opReceiptRPC{TransactionHash: hash.Hex(), BlockHash: blockHash.Hex(), L1Fee: new("-0x1")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := opReceiptToDomain(test.value); err != ErrUnavailable {
				t.Fatalf("malformed OP receipt crossed boundary: %v", err)
			}
		})
	}
}

func TestDiagnosticsJSONContract(t *testing.T) {
	got, err := json.Marshal((&Client{}).Diagnostics())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"chainId":11155111,"instrumented":false,"rpc":{"requests":0,"transportFailures":0,"failovers":0,"lastRequestMs":0,"activeEndpoint":0,"endpointCount":0}}`
	if string(got) != want {
		t.Fatalf("diagnostics JSON changed\ngot:  %s\nwant: %s", got, want)
	}

	c, err := NewNetwork(SepoliaID, []string{"http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	diagnostics := c.Diagnostics()
	if !diagnostics.Instrumented || diagnostics.RPC.ActiveEndpoint != 1 || diagnostics.RPC.EndpointCount != 1 {
		t.Fatalf("incorrect instrumented diagnostics: %+v", diagnostics)
	}
}
