package wallet

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestArchivePreservesHistoryAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	jm, err := NewJournalManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	defer wipePrivateKey(key)
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	for i := 0; i < 900; i += 1 {
		tx, err := types.SignTx(types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(11155111), Nonce: uint64(i), To: &to, Gas: 21000, GasFeeCap: big.NewInt(2), GasTipCap: big.NewInt(1), Value: big.NewInt(1)}), types.LatestSignerForChainID(big.NewInt(11155111)), key)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := tx.MarshalBinary()
		jm.records = append(jm.records, &JournalRecord{Hash: TransactionHash(tx.Hash().Hex()), QuoteID: QuoteID(fmt.Sprint(i)), Nonce: uint64(i), SignedRaw: hexutil.Encode(raw), State: "succeeded", Finalized: i < 899, Version: 1, CreatedAt: time.Now()})
	}
	if err := jm.atomicSave(jm.records); err != nil {
		t.Fatal(err)
	}
	saved := append([]*JournalRecord{}, jm.records...)
	count, err := jm.ArchiveFinalized()
	if err != nil || count != 800 || len(jm.records) != 100 {
		t.Fatalf("archive count %d err %v", count, err)
	}
	restarted, err := NewJournalManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.ListHistory()) != 900 || restarted.FindByQuoteID("0") == nil {
		t.Fatal("lost archived history/idempotency")
	}
	if restarted.HasInFlightTx() {
		t.Fatal("archived success blocks new sends")
	}
	if len(restarted.RefreshItems()) != 1 {
		t.Fatal("unfinalized record not tracked")
	}
	// Simulate a crash after archive persistence but before shortening the active journal.
	if err := restarted.atomicSave(saved); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewJournalManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered.ListHistory()) != 900 {
		t.Fatal("duplicated history after interrupted compaction")
	}
}
