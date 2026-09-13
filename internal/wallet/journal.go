package wallet

import (
	"encoding/json"
	"errors"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// JournalManager manages transaction persistence, in-flight state tracking, and atomic updates.
type JournalManager struct {
	mu        sync.Mutex
	walletDir string
	records   []*JournalRecord
}

func NewJournalManager(walletDir string) (*JournalManager, error) {
	jm := &JournalManager{
		walletDir: walletDir,
		records:   make([]*JournalRecord, 0),
	}
	if err := jm.load(); err != nil {
		return nil, err
	}
	return jm, nil
}

func (jm *JournalManager) journalPath() string {
	return filepath.Join(jm.walletDir, "journal.json")
}

func (jm *JournalManager) load() error {
	path := jm.journalPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var records []*JournalRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return errors.New("無法解析交易日誌檔")
	}
	if len(records) > 1000 {
		return ErrJournalFull
	}
	for _, record := range records {
		if record == nil {
			return errors.New("交易日誌包含空紀錄")
		}
		raw, err := hexutil.Decode(record.SignedRaw)
		if err != nil {
			return errors.New("交易日誌已損毀")
		}
		var tx types.Transaction
		if tx.UnmarshalBinary(raw) != nil || tx.Hash().Hex() != record.Hash || tx.ChainId().Cmp(big.NewInt(11155111)) != 0 {
			return errors.New("交易日誌的簽名資料不符")
		}
	}
	jm.records = records
	return nil
}

// HasInFlightTx returns true if there is an unconfirmed transaction in flight.
func (jm *JournalManager) HasInFlightTx() bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	for _, r := range jm.records {
		if r.State != "succeeded" && r.State != "reverted" {
			return true
		}
	}
	return false
}

// FindByQuoteID searches for an existing journal record for the given quote ID.
func (jm *JournalManager) FindByQuoteID(quoteID string) *JournalRecord {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	for _, r := range jm.records {
		if r.QuoteID == quoteID {
			copy := *r
			return &copy
		}
	}
	return nil
}

// FindByHash searches for an existing journal record with the given tx hash.
func (jm *JournalManager) FindByHash(hash string) *JournalRecord {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	for _, r := range jm.records {
		if r.Hash == hash {
			copy := *r
			return &copy
		}
	}
	return nil
}

// AppendAtomic persists a new record atomically. Refuses if journal exceeds 1000 entries.
func (jm *JournalManager) AppendAtomic(record *JournalRecord) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if len(jm.records) >= 1000 {
		return ErrJournalFull
	}

	newRecords := append(jm.records, record)
	if err := jm.atomicSave(newRecords); err != nil {
		return err
	}
	jm.records = newRecords
	return nil
}

// UpdateStateAtomic updates the state and metadata of a record atomically.
func (jm *JournalManager) UpdateStateAtomic(hash string, state string, confirmations string, feeEth string, txErr string) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	next := make([]*JournalRecord, len(jm.records))
	found := false
	for i, record := range jm.records {
		copy := *record
		if copy.Hash == hash {
			copy.State = state
			copy.UpdatedAt = time.Now().UTC()
			copy.Confirmations = confirmations
			copy.FeeETH = feeEth
			copy.Error = txErr
			found = true
		}
		next[i] = &copy
	}
	if !found {
		return errors.New("找不到欲更新的交易紀錄")
	}
	if err := jm.atomicSave(next); err != nil {
		return err
	}
	jm.records = next
	return nil
}

// ListHistory returns a copy of history items sorted newest first, omitting signedRaw.
func (jm *JournalManager) ListHistory() []HistoryItem {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	items := make([]HistoryItem, len(jm.records))
	for i, r := range jm.records {
		items[i] = HistoryItem{
			Hash:          r.Hash,
			State:         r.State,
			To:            r.To,
			Amount:        r.Amount,
			Symbol:        r.Symbol,
			Action:        r.Action,
			CreatedAt:     r.CreatedAt.Format(time.RFC3339),
			Confirmations: r.Confirmations,
			FeeETH:        r.FeeETH,
			Error:         r.Error,
		}
	}

	// Newest first
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})

	return items
}

// RefreshHashes includes outstanding transactions and the latest 20 mined records for reorg checks.
func (jm *JournalManager) RefreshHashes() []string {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	var hashes []string
	for i := len(jm.records) - 1; i >= 0; i-- {
		r := jm.records[i]
		if i >= len(jm.records)-20 || (r.State != "succeeded" && r.State != "reverted") {
			hashes = append(hashes, r.Hash)
		}
	}
	return hashes
}

func (jm *JournalManager) atomicSave(records []*JournalRecord) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	dir := jm.walletDir
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "journal-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, jm.journalPath()); err != nil {
		return err
	}

	// Sync directory
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
