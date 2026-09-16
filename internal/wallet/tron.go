package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
)

type TronQuote struct {
	ID          string    `json:"id"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	Contract    string    `json:"contract,omitempty"`
	Symbol      string    `json:"symbol"`
	Amount      string    `json:"amount"`
	AmountRaw   string    `json:"amountRaw,omitempty"`
	Decimals    int       `json:"decimals,omitempty"`
	FeeTRX      string    `json:"feeTrx"`
	FeeLimitTRX string    `json:"feeLimitTrx"`
	Energy      int64     `json:"energy"`
	Bandwidth   int64     `json:"bandwidth"`
	Expires     time.Time `json:"expiresAt"`
	raw         []byte
	required    int64
	decimals    int
	amountRaw   *big.Int
}
type TronRecord struct {
	Reused             bool      `json:"reused,omitempty"`
	Signature          string    `json:"signature"`
	QuoteID            string    `json:"quoteId"`
	From               string    `json:"from"`
	To                 string    `json:"to"`
	Contract           string    `json:"contract,omitempty"`
	Symbol             string    `json:"symbol"`
	Amount             string    `json:"amount"`
	State              string    `json:"state"`
	Finalized          bool      `json:"finalized"`
	FeeTRX             string    `json:"feeTrx,omitempty"`
	ExpiryCheckedBlock string    `json:"expiryCheckedBlock,omitempty"`
	ExpiryCheckedAt    int64     `json:"expiryCheckedAt,omitempty"`
	Result             string    `json:"result,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	Expires            time.Time `json:"expiresAt"`
}
type tronDiskRecord struct {
	Reused             bool      `json:"reused,omitempty"`
	Signature          string    `json:"signature"`
	QuoteID            string    `json:"quoteId"`
	From               string    `json:"from"`
	To                 string    `json:"to"`
	Contract           string    `json:"contract,omitempty"`
	Symbol             string    `json:"symbol"`
	Amount             string    `json:"amount"`
	State              string    `json:"state"`
	Finalized          bool      `json:"finalized"`
	FeeTRX             string    `json:"feeTrx,omitempty"`
	ExpiryCheckedBlock string    `json:"expiryCheckedBlock,omitempty"`
	ExpiryCheckedAt    int64     `json:"expiryCheckedAt,omitempty"`
	Result             string    `json:"result,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	Expires            time.Time `json:"expiresAt"`
	Raw                string    `json:"raw"`
	Signed             string    `json:"signed"`
}
type TronService struct {
	mu                    sync.Mutex
	endpoint, apiKey, dir string
	http                  *http.Client
	keys                  *KeystoreManager
	lock                  *os.File
	quotes                map[QuoteID]*TronQuote
	records               []tronJournalRecord
	fault                 bool
}

func NewTronService(endpoint, apiKey, dir string, n int) (*TronService, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("TRON RPC URL 無效")
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
	s := &TronService{endpoint: strings.TrimRight(endpoint, "/"), apiKey: apiKey, dir: dir, lock: lock, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, keys: NewKeystoreManager(dir, n, 1), quotes: map[QuoteID]*TronQuote{}}
	s.keys.coinType = 195
	data, err := os.ReadFile(filepath.Join(dir, "transactions.json"))
	if err != nil && !os.IsNotExist(err) {
		lock.Close()
		return nil, err
	}
	var stored []tronDiskRecord
	if len(data) > 0 && (json.Unmarshal(data, &stored) != nil || len(stored) > 1000) {
		lock.Close()
		return nil, errors.New("TRON 日誌無法讀取")
	}
	if stored != nil {
		s.records = make([]tronJournalRecord, 0, len(stored))
	}
	for _, disk := range stored {
		record, err := tronRecordFromDisk(disk)
		if err != nil {
			lock.Close()
			return nil, err
		}
		s.records = append(s.records, record)
		raw, e := hex.DecodeString(record.Raw)
		signed, e2 := hex.DecodeString(record.Signed)
		hash := sha256.Sum256(raw)
		prefix := tronBytes(nil, 1, raw)
		if e != nil || e2 != nil || hex.EncodeToString(hash[:]) != string(record.Signature) || len(signed) != len(prefix)+67 {
			lock.Close()
			return nil, errors.New("TRON 日誌交易損壞")
		}
		signature := signed[len(signed)-65:]
		pub, e := crypto.SigToPub(hash[:], signature)
		if e != nil || TronAddress(crypto.PubkeyToAddress(*pub)) != string(record.From) || hex.EncodeToString(tronBytes(prefix, 2, signature)) != record.Signed {
			lock.Close()
			return nil, errors.New("TRON 日誌簽名不符")
		}
	}
	return s, nil
}
func (s *TronService) Close() error { s.mu.Lock(); defer s.mu.Unlock(); return s.lock.Close() }
func (s *TronService) Status() (TronStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	addr, err := s.keys.Address()
	if errors.Is(err, ErrWalletNotFound) {
		return TronStatus{Network: "TRON Shasta"}, nil
	}
	if err != nil {
		return TronStatus{}, err
	}
	return TronStatus{Exists: true, Network: "TRON Shasta", Address: TronAddress(common.HexToAddress(addr))}, nil
}
func (s *TronService) Create(mnemonic, password string) (*TronCreateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(mnemonic) == "" {
		r, err := s.keys.Create(password)
		if err != nil {
			return nil, err
		}
		return &TronCreateResponse{Address: TronAddress(common.HexToAddress(r.Address)), Mnemonic: r.Mnemonic}, nil
	}
	r, err := s.keys.Import(mnemonic, password)
	if err != nil {
		return nil, err
	}
	return &TronCreateResponse{Address: TronAddress(common.HexToAddress(r.Address))}, nil
}
func (s *TronService) Backup(password string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keys.Backup(password)
}
func (s *TronService) RestoreBackup(raw json.RawMessage, password, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.keys.ImportKeystore(raw, password, newPassword)
	return err
}
func (s *TronService) ChangePassword(password, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keys.ChangePassword(password, newPassword)
}
func (s *TronService) pending() bool {
	for _, r := range s.records {
		if !r.Finalized && r.State != "expired_unconfirmed" {
			return true
		}
	}
	return false
}
func (s *TronService) save(records []tronJournalRecord) error {
	var stored []tronDiskRecord
	if records != nil {
		stored = make([]tronDiskRecord, len(records))
	}
	for i, record := range records {
		stored[i] = tronRecordToDisk(record)
	}
	data, err := json.Marshal(stored)
	if err == nil {
		err = s.keys.atomicWriteFile(filepath.Join(s.dir, "transactions.json"), data, 0600)
	}
	if err != nil {
		s.fault = true
		return err
	}
	s.records = records
	return nil
}
func (s *TronService) Quote(ctx context.Context, to, amount, contract, amountRaw string) (*TronQuote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fault {
		return nil, errors.New("TRON 儲存故障，請檢查磁碟後重啟")
	}
	if s.pending() {
		return nil, ErrTxInFlight
	}
	if len(s.records) >= 1000 {
		return nil, ErrJournalFull
	}
	toBytes, err := ParseTronAddress(to)
	if err != nil {
		return nil, err
	}
	owner, err := s.keys.Address()
	if err != nil {
		return nil, err
	}
	from := TronAddress(common.HexToAddress(owner))
	if contract == "" && to == from {
		return nil, errors.New("TRON 不允許 TRX 轉給自己，請填入另一個 Shasta 收款地址")
	}
	ownerBytes, _ := ParseTronAddress(from)
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	account, err := s.account(ctx, from)
	if err != nil {
		return nil, err
	}
	if account.Address == "" {
		return nil, errors.New("請先從 Shasta 水龍頭領取 TRX，啟用帳戶並支付費用")
	}
	var params struct {
		Values []struct {
			Key   string
			Value int64
		} `json:"chainParameter"`
	}
	if err := s.call(ctx, "/wallet/getchainparameters", map[string]any{}, &params); err != nil {
		return nil, err
	}
	costs := map[string]int64{}
	for _, p := range params.Values {
		costs[p.Key] = p.Value
	}
	for _, name := range []string{"getTransactionFee", "getEnergyFee", "getCreateNewAccountFeeInSystemContract", "getCreateAccountFee"} {
		if costs[name] <= 0 || costs[name] > 100000000 {
			return nil, errors.New("TRON 費率參數缺漏或超出本原型支援範圍")
		}
	}
	q := &TronQuote{ID: uuid.NewString(), From: from, To: to, Amount: amount, Contract: contract, Symbol: "TRX", Expires: time.Now().UTC().Add(60 * time.Second)}
	decimals := 6
	var contractBytes []byte
	var token *TronToken
	if contract != "" {
		contractBytes, err = ParseTronAddress(contract)
		if err != nil {
			return nil, err
		}
		token, err = s.token(ctx, from, contract)
		if err != nil {
			return nil, err
		}
		decimals = token.Decimals
		q.Symbol = token.Symbol
	}
	q.decimals = decimals
	q.Decimals = decimals
	var units *big.Int
	if contract == "" {
		units, err = ParseUnits(amount, decimals)
	} else {
		units, err = ParseRawTokenAmount(amountRaw)
		if err == nil {
			q.Amount = FormatUnits(units, decimals)
			q.AmountRaw = units.String()
		}
	}
	if err != nil {
		return nil, err
	}
	if units.Sign() <= 0 {
		return nil, errors.New("轉帳數量必須大於零")
	}
	q.amountRaw = new(big.Int).Set(units)
	feeLimit := int64(0)
	value := int64(0)
	activation := int64(0)
	if contract == "" {
		if !units.IsInt64() {
			return nil, errors.New("TRX 金額超出範圍")
		}
		value = units.Int64()
		recipient, err := s.account(ctx, to)
		if err != nil {
			return nil, err
		}
		if recipient.Address == "" {
			activation = costs["getCreateNewAccountFeeInSystemContract"] + costs["getCreateAccountFee"]
		}
	} else {
		if token.Raw.Cmp(units) < 0 {
			return nil, ErrInsufficientFunds
		}
		raw, energy, err := s.constant(ctx, from, contract, "transfer", common.BytesToAddress(toBytes[1:]), units)
		if err != nil {
			return nil, err
		}
		var success bool
		if erc20ABI.UnpackIntoInterface(&success, "transfer", raw) != nil || !success || energy <= 0 || energy > 10000000 {
			return nil, ErrSimulationFailed
		}
		q.Energy = energy
		feeLimit = energy * costs["getEnergyFee"] * 2
		if feeLimit > 100000000 {
			return nil, errors.New("Energy 費用預留超過本原型每筆 100 TRX 限制")
		}
	}
	var block struct {
		ID     string `json:"blockID"`
		Header struct {
			Raw struct {
				Timestamp int64 `json:"timestamp"`
			} `json:"raw_data"`
		} `json:"block_header"`
	}
	if err := s.call(ctx, "/wallet/getnowblock", map[string]any{}, &block); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if block.Header.Raw.Timestamp < now-120000 || block.Header.Raw.Timestamp > now+30000 {
		return nil, errors.New("TRON 節點區塊時間不新鮮，請稍後重試")
	}
	q.raw, err = tronRaw(ownerBytes, toBytes, units, contractBytes, block.ID, now, q.Expires.UnixMilli(), feeLimit)
	if err != nil {
		return nil, err
	}
	// Reserve full bandwidth burn, signature bytes, receipt overhead, and account activation when needed.
	q.Bandwidth = int64(len(tronBytes(tronBytes(nil, 1, q.raw), 2, make([]byte, 65))) + 64)
	reserve := q.Bandwidth*costs["getTransactionFee"] + activation + feeLimit
	if value > account.Balance || reserve > account.Balance-value {
		return nil, ErrInsufficientFunds
	}
	q.required = value + reserve
	q.FeeTRX = FormatUnits(big.NewInt(reserve), 6)
	q.FeeLimitTRX = FormatUnits(big.NewInt(feeLimit), 6)
	for id, old := range s.quotes {
		if time.Now().After(old.Expires) {
			delete(s.quotes, id)
		}
	}
	if len(s.quotes) >= 256 {
		return nil, ErrQuoteStorageFull
	}
	s.quotes[QuoteID(q.ID)] = q
	return q, nil
}
func (s *TronService) Send(ctx context.Context, id, password string) (*TronRecord, error) {
	quoteID, err := ParseLegacyQuoteID(id)
	if err != nil {
		return nil, ErrQuoteNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.records {
		if record.QuoteID == quoteID {
			r := tronRecordResponse(record)
			return &r, nil
		}
	}
	if s.fault {
		return nil, errors.New("TRON 儲存故障，請檢查磁碟後重啟")
	}
	if s.pending() {
		return nil, ErrTxInFlight
	}
	if len(s.records) >= 1000 {
		return nil, ErrJournalFull
	}
	q := s.quotes[quoteID]
	if q == nil {
		return nil, ErrQuoteNotFound
	}
	if time.Now().After(q.Expires) {
		return nil, ErrQuoteExpired
	}
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	account, err := s.account(ctx, q.From)
	if err != nil {
		return nil, err
	}
	if account.Balance < q.required {
		return nil, ErrInsufficientFunds
	}
	if q.Contract != "" {
		token, err := s.token(ctx, q.From, q.Contract)
		if err != nil {
			return nil, err
		}
		units := q.amountRaw
		if units == nil || token.Decimals != q.decimals || token.Symbol != q.Symbol || token.Raw.Cmp(units) < 0 {
			return nil, ErrInsufficientFunds
		}
		to, _ := ParseTronAddress(q.To)
		raw, energy, err := s.constant(ctx, q.From, q.Contract, "transfer", common.BytesToAddress(to[1:]), units)
		var ok bool
		if err != nil || erc20ABI.UnpackIntoInterface(&ok, "transfer", raw) != nil || !ok || energy <= 0 || energy > q.Energy*2 {
			return nil, ErrSimulationFailed
		}
	}
	key, err := s.keys.DecryptKey(password)
	if err != nil {
		return nil, err
	}
	defer wipePrivateKey(key.PrivateKey)
	if TronAddress(crypto.PubkeyToAddress(key.PrivateKey.PublicKey)) != q.From {
		return nil, errors.New("簽名帳戶與報價不符")
	}
	if time.Now().After(q.Expires) || ctx.Err() != nil {
		return nil, ErrQuoteExpired
	}
	hash := sha256.Sum256(q.raw)
	sig, err := crypto.Sign(hash[:], key.PrivateKey)
	if err != nil {
		return nil, err
	}
	record := tronJournalRecord{Signature: tronTransactionID(hex.EncodeToString(hash[:])), QuoteID: quoteID, From: tronAddress(q.From), To: tronAddress(q.To), Contract: tronAddress(q.Contract), Symbol: q.Symbol, Amount: q.Amount, State: "broadcast_unknown", CreatedAt: time.Now().UTC(), Expires: q.Expires, Raw: hex.EncodeToString(q.raw), Signed: hex.EncodeToString(tronBytes(tronBytes(nil, 1, q.raw), 2, sig))}
	next := append(slices.Clone(s.records), record)
	if err := s.save(next); err != nil {
		return nil, err
	}
	delete(s.quotes, quoteID)
	return s.broadcast(ctx, len(s.records)-1)
}
func (s *TronService) broadcast(ctx context.Context, index int) (*TronRecord, error) {
	// Recheck network immediately before transmitting identical, already durable signed bytes.
	if err := s.check(ctx); err != nil {
		r := tronRecordResponse(s.records[index])
		return &r, err
	}
	if time.Now().After(s.records[index].Expires) {
		return nil, errors.New("原交易已過期；保留紀錄並查核收據，請勿盲目重送新交易")
	}
	var result struct {
		Result bool
		Code   string
		TxID   string `json:"txid"`
	}
	err := s.call(ctx, "/wallet/broadcasthex", map[string]string{"transaction": s.records[index].Signed}, &result)
	next := slices.Clone(s.records)
	record := &next[index]
	if err == nil && result.Result && (result.TxID == "" || result.TxID == string(record.Signature)) {
		record.State = "submitted"
	} else {
		record.State = "broadcast_unknown"
	}
	if err := s.save(next); err != nil {
		return nil, err
	}
	r := tronRecordResponse(*record)
	return &r, nil
}
func (s *TronService) Retry(ctx context.Context, hash string) (*TronRecord, error) {
	if !validTronHash(hash) {
		return nil, errors.New("TRON 交易 ID 格式錯誤")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fault {
		return nil, errors.New("TRON 儲存故障")
	}
	id := tronTransactionID(hash)
	for i, r := range s.records {
		if r.Signature == id {
			if r.Finalized || r.State == "expired_unconfirmed" {
				x := tronRecordResponse(r)
				return &x, nil
			}
			return s.broadcast(ctx, i)
		}
	}
	return nil, errors.New("找不到原交易")
}
func (s *TronService) History(ctx context.Context) ([]TronRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	next := slices.Clone(s.records)
	for i := range next {
		r := &next[i]
		if r.Finalized {
			continue
		}
		if ctx.Err() != nil {
			return nil, errTronRPC
		}
		var solid struct {
			ID     string `json:"blockID"`
			Header struct {
				Raw struct {
					Timestamp int64 `json:"timestamp"`
				} `json:"raw_data"`
			} `json:"block_header"`
		}
		if time.Now().After(r.Expires) && !r.Expires.IsZero() {
			if err := s.call(ctx, "/walletsolidity/getnowblock", map[string]any{}, &solid); err != nil {
				return nil, err
			}
			if !validTronHash(solid.ID) || solid.Header.Raw.Timestamp <= 0 || solid.Header.Raw.Timestamp > time.Now().Add(30*time.Second).UnixMilli() {
				return nil, errTronRPC
			}
		}
		var info struct {
			ID             string
			Fee            int64
			Block          int64 `json:"blockNumber"`
			Result         string
			ContractResult []string `json:"contractResult"`
			Receipt        struct{ Result string }
		}
		if err := s.call(ctx, "/walletsolidity/gettransactioninfobyid", map[string]string{"value": string(r.Signature)}, &info); err != nil {
			return nil, err
		}
		if info.ID == "" {
			if solid.Header.Raw.Timestamp > r.Expires.UnixMilli() && !r.Expires.IsZero() {
				// Expiration is enforced against chain time, not the local clock. Keep the journal;
				// this is an absent receipt after expiry, never a successful/failed execution claim.
				r.State = "expired_unconfirmed"
				r.ExpiryCheckedBlock = solid.ID
				r.ExpiryCheckedAt = solid.Header.Raw.Timestamp
			}
			continue
		}
		if info.ID != string(r.Signature) || info.Block <= 0 || info.Fee < 0 {
			return nil, errTronRPC
		}
		r.Finalized = true
		r.State = "finalized"
		r.Result = info.Receipt.Result
		if info.Result == "FAILED" || (info.Receipt.Result != "" && info.Receipt.Result != "SUCCESS") {
			r.State = "execution_failed"
			if r.Result == "" {
				r.Result = info.Result
			}
		}
		if r.Contract != "" && r.State == "finalized" {
			if len(info.ContractResult) != 1 {
				return nil, errTronRPC
			}
			raw, e := hex.DecodeString(info.ContractResult[0])
			var ok bool
			if e != nil || erc20ABI.UnpackIntoInterface(&ok, "transfer", raw) != nil {
				return nil, errTronRPC
			}
			if !ok {
				r.State = "execution_failed"
				r.Result = "TRC20_RETURNED_FALSE"
			}
		}
		r.FeeTRX = FormatUnits(big.NewInt(info.Fee), 6)
	}
	if !slices.Equal(s.records, next) {
		if err := s.save(next); err != nil {
			return nil, err
		}
	}
	list := []TronRecord{}
	for _, r := range s.records[max(0, len(s.records)-20):] {
		list = append(list, tronRecordResponse(r))
	}
	return list, nil
}
