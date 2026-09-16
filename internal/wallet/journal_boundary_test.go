package wallet

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func signedJournalRecord(t *testing.T, quoteID string) *JournalRecord {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(11155111), Nonce: 7, GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2),
		Gas: 21000, To: new(common.HexToAddress("0x2222222222222222222222222222222222222222")), Value: big.NewInt(3),
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(big.NewInt(11155111)), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 16, 1, 2, 3, 0, time.UTC)
	return &JournalRecord{
		Hash: TransactionHash(signed.Hash().Hex()), QuoteID: QuoteID(quoteID), State: JournalPending,
		To: EVMAddress(tx.To().Hex()), Amount: "0.000000000000000003", AmountRaw: "3", Symbol: "ETH",
		Action: ActionETH, Nonce: 7, SignedRaw: hexutil.Encode(raw), CreatedAt: now, UpdatedAt: now,
	}
}

func TestJournalStorageBoundaryPreservesLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewJournalManager(dir, 11155111)
	if err != nil {
		t.Fatal(err)
	}
	record := signedJournalRecord(t, "legacy-quote-id")
	if _, err := manager.AppendAtomic(record); err != nil {
		t.Fatalf("legacy quote ID should remain readable in durable records: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored []map[string]json.RawMessage
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0]["quoteId"] == nil || stored[0]["signedRaw"] == nil || stored[0]["createdAt"] == nil {
		t.Fatalf("journal JSON schema changed: %s", data)
	}
	if stored[0]["QuoteID"] != nil || stored[0]["SignedRaw"] != nil {
		t.Fatalf("domain field names leaked into storage JSON: %s", data)
	}

	restarted, err := NewJournalManager(dir, 11155111)
	if err != nil {
		t.Fatal(err)
	}
	loaded := restarted.FindByHash(string(record.Hash))
	if loaded == nil || loaded.QuoteID != record.QuoteID || loaded.State != JournalPending || loaded.Action != ActionETH || loaded.SignedRaw != record.SignedRaw {
		t.Fatalf("journal round trip changed domain record: %+v", loaded)
	}
}

func TestJournalStorageLoadsLegacyRecordWithoutRecipient(t *testing.T) {
	dir := t.TempDir()
	record := signedJournalRecord(t, "legacy-without-to")
	stored := journalRecordToDisk(record)
	stored.To = ""
	data, err := json.Marshal([]*journalRecordDisk{stored})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "journal.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewJournalManager(dir, 11155111)
	if err != nil {
		t.Fatalf("legacy journal without recipient should load: %v", err)
	}
	loaded := manager.FindByHash(string(record.Hash))
	if loaded == nil || loaded.To != "" || loaded.SignedRaw != record.SignedRaw {
		t.Fatalf("legacy recipient omission was not preserved: %+v", loaded)
	}
	if err := manager.atomicSave(manager.records); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var after []map[string]json.RawMessage
	if err := json.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0]["to"] != nil {
		t.Fatalf("legacy omitted recipient was fabricated during save: %s", data)
	}
}

func TestJournalRejectsInvalidDomainStateBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewJournalManager(dir, 11155111)
	if err != nil {
		t.Fatal(err)
	}
	record := signedJournalRecord(t, "legacy-quote-id")
	record.State = JournalState("invented")
	if _, err := manager.AppendAtomic(record); err == nil {
		t.Fatal("invalid journal state was persisted")
	}
	if _, err := os.Stat(filepath.Join(dir, "journal.json")); !os.IsNotExist(err) {
		t.Fatalf("invalid record created durable journal: %v", err)
	}
}

func TestQuoteAndAccountIdentifiersAreParsedAtBoundary(t *testing.T) {
	request := &QuoteRequest{
		Action: "transfer", To: "0x2222222222222222222222222222222222222222",
		Contract: "0x4444444444444444444444444444444444444444", AmountRaw: "1",
	}
	command, err := ParseQuoteRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if command.Action != ActionTransfer || command.To != EVMAddress("0x2222222222222222222222222222222222222222") || command.Contract != EVMAddress("0x4444444444444444444444444444444444444444") {
		t.Fatalf("request was not converted to typed command: %+v", command)
	}
	if _, err := ParseQuoteRequest(&QuoteRequest{Action: "invented", To: request.To}); err == nil {
		t.Fatal("unknown transaction action reached business logic")
	}
	if _, err := ParseQuoteRequest(&QuoteRequest{Action: "speedup", Hash: "not-a-hash"}); err == nil {
		t.Fatal("invalid replacement hash reached business logic")
	}
	if _, err := ParseAccountID("../../wallet"); err == nil {
		t.Fatal("path-like account ID was accepted")
	}
	if id, err := ParseAccountID("0123456789abcdef0123456789abcdef"); err != nil || string(id) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("valid account ID rejected: %q %v", id, err)
	}
}
