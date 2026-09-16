package wallet

import (
	"cmp"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

type journalRecordDisk struct {
	Finalized bool   `json:"finalized,omitempty"`
	Hash      string `json:"hash"`
	QuoteID   string `json:"quoteId"`
	State     string `json:"state"`
	// To was absent from early journal records. Keep omitempty so loading and
	// rewriting such a valid legacy record does not fabricate a new field.
	To            string    `json:"to,omitempty"`
	Amount        string    `json:"amount"`
	AmountRaw     string    `json:"amountRaw"`
	Symbol        string    `json:"symbol"`
	Action        string    `json:"action"`
	Nonce         uint64    `json:"nonce"`
	SignedRaw     string    `json:"signedRaw"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Confirmations string    `json:"confirmations,omitempty"`
	FeeETH        string    `json:"feeEth,omitempty"`
	Error         string    `json:"error,omitempty"`
	Version       uint64    `json:"version,omitempty"`
}

func journalRecordFromDisk(stored *journalRecordDisk, chainID int64) (*JournalRecord, error) {
	if stored == nil {
		return nil, errors.New("交易日誌包含空紀錄")
	}
	quoteID, err := ParseLegacyQuoteID(stored.QuoteID)
	if err != nil {
		return nil, err
	}
	hash, err := ParseTransactionHash(stored.Hash)
	if err != nil {
		return nil, err
	}
	var to EVMAddress
	if stored.To != "" {
		parsed, err := ParseEVMAddress(stored.To)
		if err != nil {
			return nil, err
		}
		to = parsed
	}
	state, err := ParseJournalState(stored.State)
	if err != nil {
		return nil, err
	}
	action := TransactionAction(stored.Action)
	if stored.Action != "" {
		action, err = ParseTransactionAction(stored.Action)
		if err != nil {
			return nil, err
		}
	}
	version := stored.Version
	if version == 0 {
		version = 1
	}
	record := &JournalRecord{
		Finalized: stored.Finalized, Hash: hash, QuoteID: quoteID, State: state,
		To: to, Amount: stored.Amount, AmountRaw: stored.AmountRaw, Symbol: stored.Symbol,
		Action: action, Nonce: stored.Nonce, SignedRaw: stored.SignedRaw, CreatedAt: stored.CreatedAt,
		UpdatedAt: stored.UpdatedAt, Confirmations: stored.Confirmations, FeeETH: stored.FeeETH,
		Error: stored.Error, Version: version,
	}
	if err := validateJournalRecord(record, chainID); err != nil {
		return nil, err
	}
	return record, nil
}

func journalRecordToDisk(record *JournalRecord) *journalRecordDisk {
	if record == nil {
		return nil
	}
	return &journalRecordDisk{
		Finalized: record.Finalized, Hash: string(record.Hash), QuoteID: string(record.QuoteID), State: string(record.State),
		To: string(record.To), Amount: record.Amount, AmountRaw: record.AmountRaw, Symbol: record.Symbol,
		Action: string(record.Action), Nonce: record.Nonce, SignedRaw: record.SignedRaw, CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt, Confirmations: record.Confirmations, FeeETH: record.FeeETH,
		Error: record.Error, Version: record.Version,
	}
}

func validateJournalRecord(record *JournalRecord, chainID int64) error {
	if record == nil {
		return errors.New("交易日誌包含空紀錄")
	}
	if _, err := ParseLegacyQuoteID(string(record.QuoteID)); err != nil {
		return err
	}
	if _, err := ParseTransactionHash(string(record.Hash)); err != nil {
		return err
	}
	if record.To != "" {
		if _, err := ParseEVMAddress(string(record.To)); err != nil {
			return err
		}
	}
	if _, err := ParseJournalState(string(record.State)); err != nil {
		return err
	}
	if record.Action != "" {
		if _, err := ParseTransactionAction(string(record.Action)); err != nil {
			return err
		}
	}
	raw, err := hexutil.Decode(record.SignedRaw)
	if err != nil {
		return errors.New("交易日誌已損毀")
	}
	var tx types.Transaction
	if tx.UnmarshalBinary(raw) != nil || tx.Hash().Hex() != string(record.Hash) || tx.ChainId().Cmp(big.NewInt(chainID)) != 0 {
		return errors.New("交易日誌的簽名資料不符")
	}
	return nil
}

func isMinedJournalState(state JournalState) bool {
	return state == JournalSucceeded || state == JournalReverted
}

// JournalManager manages transaction persistence, in-flight state tracking, and atomic updates.
type JournalManager struct {
	mu        sync.Mutex
	chainID   int64
	walletDir string
	records   []*JournalRecord
	archived  []*JournalRecord
}

func NewJournalManager(walletDir string, chainIDs ...int64) (*JournalManager, error) {
	jm := &JournalManager{
		walletDir: walletDir,
		records:   make([]*JournalRecord, 0),
	}
	jm.chainID = 11155111
	if len(chainIDs) > 0 {
		jm.chainID = chainIDs[0]
	}
	if err := jm.load(); err != nil {
		return nil, err
	}
	if err := jm.loadArchives(); err != nil {
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
	var stored []*journalRecordDisk
	if err := json.Unmarshal(data, &stored); err != nil {
		return errors.New("無法解析交易日誌檔")
	}
	if len(stored) > 1000 {
		return ErrJournalFull
	}
	records := make([]*JournalRecord, 0, len(stored))
	for _, item := range stored {
		record, err := journalRecordFromDisk(item, jm.chainID)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	jm.records = records
	return nil
}

func (jm *JournalManager) forEachRecordLocked(fn func(r *JournalRecord) bool) {
	for _, r := range jm.archived {
		if !fn(r) {
			return
		}
	}
	for _, r := range jm.records {
		if !fn(r) {
			return
		}
	}
}

func (jm *JournalManager) allRecordsLocked() []*JournalRecord {
	total := len(jm.archived) + len(jm.records)
	if total == 0 {
		return nil
	}
	all := make([]*JournalRecord, total)
	copy(all, jm.archived)
	copy(all[len(jm.archived):], jm.records)
	return all
}

// HasInFlightTx returns true if there is an unconfirmed transaction in flight.
func (jm *JournalManager) HasInFlightTx() bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	mined := map[uint64]bool{}
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if isMinedJournalState(r.State) {
			mined[r.Nonce] = true
		}
		return true
	})
	inFlight := false
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if !mined[r.Nonce] {
			inFlight = true
			return false
		}
		return true
	})
	return inFlight
}

// FindByQuoteID searches for an existing journal record for the given quote ID.
func (jm *JournalManager) FindByQuoteID(quoteID string) *JournalRecord {
	id, err := ParseLegacyQuoteID(quoteID)
	if err != nil {
		return nil
	}
	jm.mu.Lock()
	defer jm.mu.Unlock()
	var result *JournalRecord
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if r.QuoteID == id {
			copy := *r
			result = &copy
			return false
		}
		return true
	})
	return result
}

// FindByHash searches for an existing journal record with the given tx hash.
func (jm *JournalManager) FindByHash(hash string) *JournalRecord {
	id, err := ParseTransactionHash(hash)
	if err != nil {
		return nil
	}
	jm.mu.Lock()
	defer jm.mu.Unlock()
	var result *JournalRecord
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if r.Hash == id {
			copy := *r
			result = &copy
			return false
		}
		return true
	})
	return result
}

// AppendAtomic persists a new record atomically. Refuses if journal exceeds 1000 entries.
// Returns the assigned version number on success.
func (jm *JournalManager) AppendAtomic(record *JournalRecord) (uint64, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if len(jm.records) >= 1000 {
		return 0, ErrJournalFull
	}
	if err := validateJournalRecord(record, jm.chainID); err != nil {
		return 0, err
	}

	recordCopy := *record
	recordCopy.Version = 1
	record.Version = 1
	newRecords := append(jm.records, &recordCopy)
	if err := jm.atomicSave(newRecords); err != nil {
		return 0, err
	}
	jm.records = newRecords
	return recordCopy.Version, nil
}

// UpdateStateAtomic updates the state and metadata of a record atomically.
func (jm *JournalManager) UpdateStateAtomic(hash string, state string, confirmations string, feeEth string, txErr string) error {
	journalState, err := ParseJournalState(state)
	if err != nil {
		return err
	}
	jm.mu.Lock()
	defer jm.mu.Unlock()

	next := make([]*JournalRecord, len(jm.records))
	found := false
	for i, record := range jm.records {
		copy := *record
		if string(copy.Hash) == hash {
			copy.State = journalState
			copy.UpdatedAt = time.Now().UTC()
			copy.Confirmations = confirmations
			copy.FeeETH = feeEth
			copy.Error = txErr
			copy.Version += 1
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

	records := jm.allRecordsLocked()
	winners := map[uint64]*JournalRecord{}
	for _, r := range records {
		if isMinedJournalState(r.State) {
			winners[r.Nonce] = r
		}
	}
	items := make([]HistoryItem, len(records))
	for i, r := range records {
		items[i] = HistoryItem{
			Hash: string(r.Hash), Finalized: r.Finalized, QuoteID: string(r.QuoteID),
			State:         string(r.State),
			To:            string(r.To),
			Amount:        r.Amount,
			Symbol:        r.Symbol,
			Action:        string(r.Action),
			CreatedAt:     r.CreatedAt.Format(time.RFC3339),
			Confirmations: r.Confirmations,
			FeeETH:        r.FeeETH,
			Error:         r.Error,
		}
		if winner := winners[r.Nonce]; winner != nil && winner.Hash != r.Hash {
			items[i].State = string(JournalReplaced)
			items[i].ReplacedBy = string(winner.Hash)
			items[i].Finalized = winner.Finalized
		}
	}

	// Newest first
	slices.SortStableFunc(items, func(a, b HistoryItem) int { return cmp.Compare(b.CreatedAt, a.CreatedAt) })

	return items
}

type RefreshItem struct {
	Hash    string
	Version uint64
	State   JournalState
}

// RefreshItems includes outstanding transactions and the latest 20 mined records for reorg checks.
func (jm *JournalManager) RefreshItems() []RefreshItem {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	var items []RefreshItem
	finalizedNonces := map[uint64]bool{}
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if r.Finalized && isMinedJournalState(r.State) {
			finalizedNonces[r.Nonce] = true
		}
		return true
	})
	for i := len(jm.records) - 1; i >= 0; i -= 1 {
		r := jm.records[i]
		if !r.Finalized && !finalizedNonces[r.Nonce] {
			items = append(items, RefreshItem{
				Hash:    string(r.Hash),
				Version: r.Version,
				State:   r.State,
			})
		}
	}
	return items
}

// UpdateStateAtomicIfVersion updates record state atomically only if the current version matches expectedVersion
// while allowing fresh observations of chain reorganizations.
func (jm *JournalManager) UpdateStateAtomicIfVersion(hash string, expectedVersion uint64, state string, confirmations string, feeEth string, txErr string, finalized ...bool) (bool, error) {
	journalState, err := ParseJournalState(state)
	if err != nil {
		return false, err
	}
	jm.mu.Lock()
	defer jm.mu.Unlock()

	next := make([]*JournalRecord, len(jm.records))
	found := false
	var target *JournalRecord
	for i, record := range jm.records {
		copy := *record
		if string(copy.Hash) == hash {
			target = &copy
			found = true
		}
		next[i] = &copy
	}
	if !found {
		return false, errors.New("找不到欲更新的交易紀錄")
	}

	// Stale check: if record was modified concurrently, do not overwrite with stale result
	if target.Version != expectedVersion {
		return false, nil
	}

	isFinal := len(finalized) > 0 && finalized[0]
	// No-op check: if record state and metadata are unchanged, avoid redundant disk writes
	if target.Finalized == isFinal && target.State == journalState && target.Confirmations == confirmations && target.FeeETH == feeEth && target.Error == txErr {
		return true, nil
	}

	target.Finalized = isFinal
	target.State = journalState
	target.UpdatedAt = time.Now().UTC()
	target.Confirmations = confirmations
	target.FeeETH = feeEth
	target.Error = txErr
	target.Version += 1

	if err := jm.atomicSave(next); err != nil {
		return false, err
	}
	jm.records = next
	return true, nil
}

func (jm *JournalManager) atomicSave(records []*JournalRecord) error {
	stored := make([]*journalRecordDisk, len(records))
	for i, record := range records {
		stored[i] = journalRecordToDisk(record)
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(jm.journalPath(), data, 0600)
}

func (jm *JournalManager) NonceRecords(nonce uint64) []JournalRecord {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	var records []JournalRecord
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if r.Nonce == nonce {
			records = append(records, *r)
		}
		return true
	})
	return records
}

func (jm *JournalManager) NonceMined(nonce uint64) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	mined := false
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if r.Nonce == nonce && isMinedJournalState(r.State) {
			mined = true
			return false
		}
		return true
	})
	return mined
}

// Archives are written before the active journal is shortened. A crash can leave duplicates, never a gap.
func (jm *JournalManager) loadArchives() error {
	paths, err := filepath.Glob(filepath.Join(jm.walletDir, "archive-*.json"))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range jm.records {
		seen[string(r.Hash)] = true
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var stored []*journalRecordDisk
		if json.Unmarshal(data, &stored) != nil {
			return errors.New("封存日誌格式錯誤")
		}
		for _, item := range stored {
			r, err := journalRecordFromDisk(item, jm.chainID)
			if err != nil {
				return err
			}
			if !r.Finalized {
				return errors.New("封存含未確定紀錄")
			}
			if !seen[string(r.Hash)] {
				jm.archived = append(jm.archived, r)
				seen[string(r.Hash)] = true
			}
		}
	}
	return nil
}

func (jm *JournalManager) ArchiveFinalized() (int, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	if len(jm.records) < 900 {
		return 0, nil
	}
	var archive, keep []*JournalRecord
	finalizedNonces := map[uint64]bool{}
	jm.forEachRecordLocked(func(r *JournalRecord) bool {
		if r.Finalized && isMinedJournalState(r.State) {
			finalizedNonces[r.Nonce] = true
		}
		return true
	})
	for i, r := range jm.records {
		if i < len(jm.records)-100 && (r.Finalized || finalizedNonces[r.Nonce]) {
			copy := *r
			if !copy.Finalized {
				copy.Finalized = true
				copy.State = JournalReplaced
			}
			archive = append(archive, &copy)
		} else {
			keep = append(keep, r)
		}
	}
	if len(archive) == 0 {
		return 0, nil
	}
	stored := make([]*journalRecordDisk, len(archive))
	for i, record := range archive {
		stored[i] = journalRecordToDisk(record)
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return 0, err
	}
	path := filepath.Join(jm.walletDir, "archive-"+string(archive[0].Hash)+"-"+string(archive[len(archive)-1].Hash)+".json")
	if err := atomicWriteFile(path, data, 0600); err != nil {
		return 0, err
	}
	if err := jm.atomicSave(keep); err != nil {
		return 0, err
	}
	jm.records = keep
	jm.archived = append(jm.archived, archive...)
	return len(archive), nil
}
