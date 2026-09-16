package wallet

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	sol "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
)

const SolanaDevnetGenesis = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"

var errSolRPC = errors.New("Solana RPC 查詢失敗或逾時；目前結果未知")

type SolanaQuote struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    string    `json:"amount"`
	Lamports  uint64    `json:"-"`
	Fee       uint64    `json:"-"`
	FeeSOL    string    `json:"feeSol"`
	Blockhash sol.Hash  `json:"blockhash"`
	LastValid uint64    `json:"lastValidBlockHeight"`
	Expires   time.Time `json:"expiresAt"`
	tx        *sol.Transaction
}
type SolanaRecord struct {
	Signature string    `json:"signature"`
	QuoteID   string    `json:"quoteId"`
	To        string    `json:"to"`
	Amount    string    `json:"amount"`
	State     string    `json:"state"`
	Finalized bool      `json:"finalized"`
	LastValid uint64    `json:"lastValidBlockHeight"`
	CreatedAt time.Time `json:"createdAt"`
	Raw       string    `json:"-"`
}
type solanaDiskRecord struct {
	Signature string    `json:"signature"`
	QuoteID   string    `json:"quoteId"`
	To        string    `json:"to"`
	Amount    string    `json:"amount"`
	State     string    `json:"state"`
	Finalized bool      `json:"finalized"`
	LastValid uint64    `json:"lastValidBlockHeight"`
	CreatedAt time.Time `json:"createdAt"`
	SignedRaw string    `json:"signedRaw"`
}

type SolanaService struct {
	mu      sync.Mutex
	rpc     *rpc.Client
	dir     string
	n       int
	lock    *os.File
	quotes  map[QuoteID]*SolanaQuote
	records []solanaJournalRecord
	fault   bool
}

func NewSolanaService(endpoint, dir string, n int) (*SolanaService, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("Solana RPC URL 無效")
	}
	if n == 0 {
		n = 262144
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, err
	}
	s := &SolanaService{rpc: rpc.New(endpoint), dir: dir, n: n, lock: lock, quotes: map[QuoteID]*SolanaQuote{}}
	data, err := os.ReadFile(filepath.Join(dir, "transactions.json"))
	if err != nil && !os.IsNotExist(err) {
		lock.Close()
		return nil, err
	}
	var stored []solanaDiskRecord
	if len(data) > 0 {
		if json.Unmarshal(data, &stored) != nil || len(stored) > 1000 {
			lock.Close()
			return nil, errors.New("Solana 交易日誌損壞")
		}
	}
	if stored != nil {
		s.records = make([]solanaJournalRecord, 0, len(stored))
	}
	for _, disk := range stored {
		record, err := solanaRecordFromDisk(disk)
		if err != nil {
			lock.Close()
			return nil, err
		}
		s.records = append(s.records, record)
		tx, err := sol.TransactionFromBase64(record.SignedRaw)
		if err != nil || len(tx.Signatures) != 1 || tx.Signatures[0].String() != string(record.Signature) || tx.VerifySignatures() != nil {
			lock.Close()
			return nil, errors.New("Solana 日誌簽名無效")
		}
	}
	return s, nil
}
func (s *SolanaService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lock.Close()
}
func (s *SolanaService) check(ctx context.Context) error {
	genesis, err := s.rpc.GetGenesisHash(ctx)
	if err != nil {
		return errSolRPC
	}
	if genesis.String() != SolanaDevnetGenesis {
		return errors.New("僅允許 Solana Devnet，RPC 網路不符")
	}
	return nil
}
func (s *SolanaService) Status() (SolanaStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, err := s.keyFile()
	if os.IsNotExist(err) {
		return SolanaStatus{Network: "Solana Devnet"}, nil
	}
	if err != nil {
		return SolanaStatus{}, err
	}
	return SolanaStatus{Exists: true, Network: "Solana Devnet", Address: key.Address}, nil
}
func (s *SolanaService) Balance(ctx context.Context, address string) (*SolanaBalance, error) {
	key, err := sol.PublicKeyFromBase58(address)
	if err != nil {
		return nil, errors.New("Solana 地址無效")
	}
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	result, err := s.rpc.GetBalance(ctx, key, rpc.CommitmentConfirmed)
	if err != nil || result == nil {
		return nil, errSolRPC
	}
	return &SolanaBalance{Address: key.String(), SOL: FormatUnits(new(big.Int).SetUint64(result.Value), 9), Lamports: new(big.Int).SetUint64(result.Value).String(), Slot: result.Context.Slot}, nil
}
func (s *SolanaService) hasPending() bool {
	for _, r := range s.records {
		if !r.Finalized && r.State != "expired_unconfirmed" && r.State != "execution_failed" {
			return true
		}
	}
	return false
}
func (s *SolanaService) Quote(ctx context.Context, to, amount string) (*SolanaQuote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fault {
		return nil, errors.New("Solana 儲存故障，請檢查磁碟後重啟")
	}
	if s.hasPending() {
		return nil, errors.New("請先更新 Solana 交易紀錄，等待前筆 finalized")
	}
	recipient, err := sol.PublicKeyFromBase58(to)
	if err != nil || recipient.IsZero() || !recipient.IsOnCurve() {
		return nil, errors.New("目前僅支援有效的一般 Solana 錢包收款地址")
	}
	raw, err := ParseUnits(amount, 9)
	if err != nil || raw == nil || !raw.IsUint64() || raw.Sign() <= 0 {
		return nil, errors.New("SOL 數量無效，最多 9 位小數")
	}
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	key, err := s.keyFile()
	if err != nil {
		return nil, err
	}
	from, err := sol.PublicKeyFromBase58(key.Address)
	if err != nil {
		return nil, err
	}
	recent, err := s.rpc.GetLatestBlockhash(ctx, rpc.CommitmentConfirmed)
	if err != nil || recent == nil || recent.Value == nil {
		return nil, errSolRPC
	}
	tx, err := sol.NewTransaction([]sol.Instruction{system.NewTransferInstruction(raw.Uint64(), from, recipient).Build()}, recent.Value.Blockhash, sol.TransactionPayer(from))
	if err != nil {
		return nil, err
	}
	message, err := tx.Message.MarshalBinary()
	if err != nil {
		return nil, err
	}
	fee, err := s.rpc.GetFeeForMessage(ctx, base64.StdEncoding.EncodeToString(message), rpc.CommitmentConfirmed)
	if err != nil || fee == nil || fee.Value == nil {
		return nil, errSolRPC
	}
	balance, err := s.rpc.GetBalance(ctx, from, rpc.CommitmentConfirmed)
	if err != nil || balance == nil {
		return nil, errSolRPC
	}
	total := new(big.Int).Add(raw, new(big.Int).SetUint64(*fee.Value))
	if new(big.Int).SetUint64(balance.Value).Cmp(total) < 0 {
		return nil, ErrInsufficientFunds
	}
	// A zero signature permits pre-sign simulation without exposing a private key.
	tx.Signatures = []sol.Signature{{}}
	simulation, err := s.rpc.SimulateTransactionWithOpts(ctx, tx, &rpc.SimulateTransactionOpts{Commitment: rpc.CommitmentConfirmed})
	if err != nil || simulation == nil || simulation.Value == nil || simulation.Value.Err != nil {
		return nil, ErrSimulationFailed
	}
	for id, q := range s.quotes {
		if time.Now().After(q.Expires) {
			delete(s.quotes, id)
		}
	}
	if len(s.quotes) >= 64 {
		return nil, ErrQuoteStorageFull
	}
	q := &SolanaQuote{ID: uuid.NewString(), From: key.Address, To: to, Amount: amount, Lamports: raw.Uint64(), Fee: *fee.Value, FeeSOL: FormatUnits(new(big.Int).SetUint64(*fee.Value), 9), Blockhash: recent.Value.Blockhash, LastValid: recent.Value.LastValidBlockHeight, Expires: time.Now().UTC().Add(60 * time.Second), tx: tx}
	s.quotes[QuoteID(q.ID)] = q
	return q, nil
}
func (s *SolanaService) persist() error {
	var stored []solanaDiskRecord
	if s.records != nil {
		stored = make([]solanaDiskRecord, len(s.records))
	}
	for i, record := range s.records {
		stored[i] = solanaRecordToDisk(record)
	}
	data, err := json.Marshal(stored)
	if err == nil {
		err = atomicWriteFile(filepath.Join(s.dir, "transactions.json"), data, 0600)
	}
	if err != nil {
		s.fault = true
	}
	return err
}
func (s *SolanaService) Send(ctx context.Context, id, password string) (*SolanaRecord, error) {
	quoteID, err := ParseLegacyQuoteID(id)
	if err != nil {
		return nil, ErrQuoteExpired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.records {
		if r.QuoteID == quoteID {
			copy := solanaRecordResponse(r)
			return &copy, nil
		}
	}
	if s.fault {
		return nil, errors.New("Solana 儲存故障")
	}
	if s.hasPending() {
		return nil, ErrTxInFlight
	}
	if len(s.records) >= 1000 {
		return nil, ErrJournalFull
	}
	q := s.quotes[quoteID]
	if q == nil || time.Now().After(q.Expires) {
		return nil, ErrQuoteExpired
	}
	seed, err := s.decryptSeed(password)
	if err != nil {
		return nil, err
	}
	defer wipeBytes(seed)
	private := sol.PrivateKey(ed25519.NewKeyFromSeed(seed))
	defer wipeBytes(private)
	if private.PublicKey().String() != q.From {
		return nil, errors.New("簽名地址不符")
	}
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	height, err := s.rpc.GetBlockHeight(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, errSolRPC
	}
	if height > q.LastValid || time.Now().After(q.Expires) {
		return nil, ErrQuoteExpired
	}
	balance, err := s.rpc.GetBalance(ctx, private.PublicKey(), rpc.CommitmentConfirmed)
	if err != nil || balance == nil {
		return nil, errSolRPC
	}
	total := new(big.Int).Add(new(big.Int).SetUint64(q.Lamports), new(big.Int).SetUint64(q.Fee))
	if new(big.Int).SetUint64(balance.Value).Cmp(total) < 0 {
		return nil, ErrInsufficientFunds
	}
	message, err := q.tx.Message.MarshalBinary()
	if err != nil {
		return nil, err
	}
	fee, err := s.rpc.GetFeeForMessage(ctx, base64.StdEncoding.EncodeToString(message), rpc.CommitmentConfirmed)
	if err != nil || fee == nil || fee.Value == nil {
		return nil, errSolRPC
	}
	if *fee.Value > q.Fee {
		return nil, ErrQuoteExpired
	}
	_, err = q.tx.Sign(func(key sol.PublicKey) *sol.PrivateKey {
		if key == private.PublicKey() {
			return &private
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	raw, err := q.tx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	record := solanaJournalRecord{Signature: solanaSignature(q.tx.Signatures[0].String()), QuoteID: quoteID, To: solanaAddress(q.To), Amount: q.Amount, State: "broadcast_unknown", LastValid: q.LastValid, CreatedAt: time.Now().UTC(), SignedRaw: base64.StdEncoding.EncodeToString(raw)}
	s.records = append(s.records, record)
	if err := s.persist(); err != nil {
		return nil, err
	}
	signature, err := s.rpc.SendTransactionWithOpts(ctx, q.tx, rpc.TransactionOpts{SkipPreflight: false, PreflightCommitment: rpc.CommitmentConfirmed})
	result := solanaRecordResponse(record)
	if err == nil && signature.String() == string(record.Signature) {
		result.State = "submitted"
		s.records[len(s.records)-1].State = "submitted"
		if err := s.persist(); err != nil {
			return nil, err
		}
	}
	return &result, nil
}

func (s *SolanaService) History(ctx context.Context) ([]SolanaRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	height, heightErr := s.rpc.GetBlockHeight(ctx, rpc.CommitmentConfirmed)
	changed := false
	result := []SolanaRecord{}
	for i := range s.records {
		record := &s.records[i]
		if !record.Finalized {
			signature, err := sol.SignatureFromBase58(string(record.Signature))
			if err != nil {
				log.Printf("Solana 簽名格式錯誤 %s: %v", record.Signature, err)
				if i >= len(s.records)-20 {
					result = append(result, solanaRecordResponse(*record))
				}
				continue
			}
			response, err := s.rpc.GetSignatureStatuses(ctx, true, signature)
			if err != nil || response == nil || len(response.Value) != 1 {
				log.Printf("Solana 簽名查詢失敗 %s: %v", record.Signature, err)
				if i >= len(s.records)-20 {
					result = append(result, solanaRecordResponse(*record))
				}
				continue
			}
			state := record.State
			finalized := false
			if status := response.Value[0]; status != nil {
				state = crosschainState(status.ConfirmationStatus)
				finalized = status.ConfirmationStatus == rpc.ConfirmationStatusFinalized
				if status.Err != nil {
					state = "execution_failed"
					finalized = true
				}
			} else if heightErr == nil && height > record.LastValid {
				state = "expired_unconfirmed"
			}
			if record.State != state || record.Finalized != finalized {
				record.State = state
				record.Finalized = finalized
				changed = true
			}
		}
		if i >= len(s.records)-20 {
			result = append(result, solanaRecordResponse(*record))
		}
	}
	if changed {
		if err := s.persist(); err != nil {
			return nil, err
		}
	}
	return result, nil
}
func (s *SolanaService) Retry(ctx context.Context, signature string) (*SolanaRecord, error) {
	parsed, err := sol.SignatureFromBase58(signature)
	if err != nil {
		return nil, errors.New("找不到原交易")
	}
	id := solanaSignature(parsed.String())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fault {
		return nil, errors.New("Solana 儲存故障")
	}
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	for i := range s.records {
		r := &s.records[i]
		if r.Signature == id {
			result := solanaRecordResponse(*r)
			if r.Finalized {
				return &result, nil
			}
			if r.State == "expired_unconfirmed" {
				return nil, errors.New("原 blockhash 已到期；仍保留日誌，請先查核原簽名的鏈上結果")
			}
			height, err := s.rpc.GetBlockHeight(ctx, rpc.CommitmentFinalized)
			if err != nil {
				return nil, errSolRPC
			}
			if height > r.LastValid {
				return nil, errors.New("原 blockhash 已到期；仍保留日誌，請先查核原簽名的鏈上結果")
			}
			got, err := s.rpc.SendEncodedTransactionWithOpts(ctx, r.SignedRaw, rpc.TransactionOpts{SkipPreflight: false, PreflightCommitment: rpc.CommitmentConfirmed})
			if err != nil {
				return nil, errSolRPC
			}
			if got.String() == signature {
				result.State = "submitted"
				r.State = "submitted"
				if err := s.persist(); err != nil {
					return nil, err
				}
			}
			return &result, nil
		}
	}
	return nil, errors.New("找不到原交易")
}
