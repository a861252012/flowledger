package wallet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/a861252012/flowledger/internal/chain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// Service orchestrates all wallet functions: keystore, quotes, transactions, and journal.
type Service struct {
	client       *chain.Client
	keystore     *KeystoreManager
	quotes       *QuoteStore
	journal      *JournalManager
	csrfToken    string
	sendMu       sync.Mutex
	walletDir    string
	lockFile     *os.File
	storageFault bool
}

func NewService(client *chain.Client, walletDir string, scryptParams ...int) (*Service, error) {
	if walletDir == "" {
		walletDir = "./data/wallet"
	}

	var n, p int
	if len(scryptParams) >= 2 {
		n = scryptParams[0]
		p = scryptParams[1]
	}

	if err := os.MkdirAll(walletDir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(walletDir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(walletDir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("錢包資料夾正由另一個程序使用")
	}
	success := false
	defer func() {
		if !success {
			lock.Close()
		}
	}()
	km := NewKeystoreManager(walletDir, n, p)
	jm, err := NewJournalManager(walletDir)
	if err != nil {
		return nil, err
	}

	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err != nil {
		return nil, err
	}

	success = true
	return &Service{
		client:    client,
		keystore:  km,
		quotes:    NewQuoteStore(),
		journal:   jm,
		csrfToken: hex.EncodeToString(csrfBytes),
		walletDir: walletDir,
		lockFile:  lock,
	}, nil
}

func (s *Service) Close() error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.lockFile == nil {
		return nil
	}
	err := s.lockFile.Close()
	s.lockFile = nil
	return err
}

func (s *Service) CSRFToken() string {
	return s.csrfToken
}

func (s *Service) Status() (*WalletInfo, error) {
	addr, err := s.keystore.Address()
	if err != nil && !errors.Is(err, ErrWalletNotFound) {
		return nil, err
	}
	return &WalletInfo{Exists: err == nil, Address: addr, Path: "m/44'/60'/0'/0/0", CSRFToken: s.csrfToken, Exchange: map[string]string{"weth": common.HexToAddress(WETHAddress).Hex(), "usdc": common.HexToAddress(USDCAddress).Hex(), "router": common.HexToAddress(RouterAddress).Hex()}}, nil
}

func (s *Service) Create(password string) (*CreateResponse, error) {
	return s.keystore.Create(password)
}

func (s *Service) Import(mnemonic, password string) (*ImportResponse, error) {
	return s.keystore.Import(mnemonic, password)
}

func (s *Service) Backup(password string) (json.RawMessage, error) {
	return s.keystore.Backup(password)
}

func (s *Service) Token(ctx context.Context, contract string) (*TokenInfo, error) {
	if !s.keystore.Exists() {
		return nil, ErrWalletNotFound
	}
	addrStr, err := s.keystore.Address()
	if err != nil {
		return nil, err
	}
	ownerAddr := common.HexToAddress(addrStr)

	contractAddr, err := ValidateAddress(contract)
	if err != nil {
		return nil, err
	}

	sym, dec, err := QueryERC20Metadata(ctx, s.client, contractAddr)
	if err != nil {
		return nil, err
	}

	bal, err := QueryERC20BalanceOf(ctx, s.client, contractAddr, ownerAddr)
	if err != nil {
		return nil, err
	}

	return &TokenInfo{
		Contract:   contractAddr.Hex(),
		Symbol:     sym,
		Decimals:   dec,
		Balance:    FormatUnits(bal, dec),
		BalanceRaw: bal.String(),
	}, nil
}

func (s *Service) Quote(ctx context.Context, req *QuoteRequest) (*QuoteResponse, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.storageFault {
		return nil, errors.New("交易儲存發生錯誤，請修復磁碟後重啟錢包")
	}
	if !s.keystore.Exists() {
		return nil, ErrWalletNotFound
	}

	// Single outstanding tx constraint: block new quotes while a transaction is in flight
	if s.journal.HasInFlightTx() {
		return nil, ErrTxInFlight
	}

	addrStr, err := s.keystore.Address()
	if err != nil {
		return nil, err
	}
	fromAddr := common.HexToAddress(addrStr)

	bound, err := CreateQuote(ctx, s.client, fromAddr, req)
	if err != nil {
		return nil, err
	}

	if err := s.quotes.Add(bound); err != nil {
		return nil, err
	}

	return bound.ToResponse(), nil
}

func (s *Service) Send(ctx context.Context, quoteID, password string) (*SendResponse, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	if s.storageFault {
		return nil, errors.New("交易儲存發生錯誤，請修復磁碟後重啟錢包")
	}
	// Double-click / retry of same quoteID: return existing record without re-signing
	if existing := s.journal.FindByQuoteID(quoteID); existing != nil {
		return &SendResponse{
			Hash:      existing.Hash,
			State:     existing.State,
			To:        existing.To,
			Amount:    existing.Amount,
			Symbol:    existing.Symbol,
			Action:    existing.Action,
			CreatedAt: existing.CreatedAt.Format(time.RFC3339),
		}, nil
	}

	quote, err := s.quotes.Get(quoteID)
	if err != nil {
		return nil, err
	}

	if s.journal.HasInFlightTx() {
		return nil, ErrTxInFlight
	}

	// Decrypt keystore
	key, err := s.keystore.DecryptKey(password)
	if err != nil {
		return nil, err
	}
	defer wipePrivateKey(key.PrivateKey)

	if key.Address != quote.From {
		return nil, errors.New("金鑰地址與報價不符")
	}
	if !time.Now().Before(quote.ExpiresAt) {
		return nil, ErrQuoteExpired
	}
	// Verify chain & nonce
	if err := s.client.CheckNetwork(ctx); err != nil {
		return nil, err
	}

	currentNonce, err := s.client.PendingNonceAt(ctx, quote.From)
	if err != nil {
		return nil, err
	}
	if currentNonce != quote.Nonce {
		return nil, ErrNonceMismatch
	}

	// Verify ETH balance
	currentBal, err := s.client.BalanceAt(ctx, quote.From, nil)
	if err != nil {
		return nil, err
	}
	if currentBal.Cmp(quote.TotalETHWei) < 0 {
		return nil, ErrInsufficientFunds
	}

	// If token transfer, verify token balance
	if quote.Action == "transfer" {
		tokBal, err := QueryERC20BalanceOf(ctx, s.client, quote.Contract, quote.From)
		if err != nil {
			return nil, err
		}
		if tokBal.Cmp(quote.AmountRaw) < 0 {
			return nil, errors.New("代幣餘額不足以支付轉帳金額")
		}
	}

	head, err := s.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	if head.BaseFee == nil || head.BaseFee.Cmp(quote.MaxFeePerGas) > 0 {
		return nil, errors.New("目前基本費用超過報價上限，請重新預估")
	}
	if quote.Action == "approve" {
		allowance, err := QueryERC20Allowance(ctx, s.client, quote.Contract, quote.From, quote.To)
		if err != nil {
			return nil, err
		}
		if allowance.Sign() > 0 && quote.AmountRaw.Sign() > 0 {
			return nil, ErrApprovalRace
		}
	}
	if quote.Action == "wrap" || quote.Action == "unwrap" || quote.Action == "swap" {
		if err := RecheckExchange(ctx, s.client, quote); err != nil {
			return nil, err
		}
	}
	if quote.Action == "transfer" || quote.Action == "approve" {
		symbol, decimals, err := QueryERC20Metadata(ctx, s.client, quote.Contract)
		if err != nil {
			return nil, err
		}
		if symbol != quote.Symbol || decimals != quote.Decimals {
			return nil, errors.New("代幣資料已變更，請重新預估")
		}
		if err := SimulateERC20Call(ctx, s.client, quote.From, quote.TxTo, quote.Data); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, chain.ErrTimeout
	}
	if !time.Now().Before(quote.ExpiresAt) {
		return nil, ErrQuoteExpired
	}
	// Build EIP-1559 DynamicFeeTx
	dynamicTx := &types.DynamicFeeTx{
		ChainID:   big.NewInt(chain.SepoliaID),
		Nonce:     quote.Nonce,
		GasTipCap: quote.MaxPriorityFeePerGas,
		GasFeeCap: quote.MaxFeePerGas,
		Gas:       quote.GasLimit,
		To:        &quote.TxTo,
		Value:     quote.TxValue,
		Data:      quote.Data,
	}
	tx := types.NewTx(dynamicTx)
	signer := types.LatestSignerForChainID(big.NewInt(chain.SepoliaID))
	signedTx, err := types.SignTx(tx, signer, key.PrivateKey)
	if err != nil {
		return nil, err
	}

	txHash := signedTx.Hash().Hex()
	rawBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	signedRawHex := hexutil.Encode(rawBytes)

	now := time.Now().UTC()
	record := &JournalRecord{
		Hash:      txHash,
		QuoteID:   quote.ID,
		State:     "pending",
		To:        quote.To.Hex(),
		Amount:    quote.Amount,
		AmountRaw: quote.AmountRaw.String(),
		Symbol:    quote.Symbol,
		Action:    quote.Action,
		Nonce:     quote.Nonce,
		SignedRaw: signedRawHex,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// PREPARE / SIGN / BROADCAST:
	// Must persist atomically before any RPC broadcast! No send if persistence fails!
	if err := s.journal.AppendAtomic(record); err != nil {
		s.storageFault = true
		return nil, err
	}

	// Broadcast to RPC
	broadcastErr := s.client.SendTransaction(ctx, signedTx)
	if broadcastErr == nil {
		if err := s.journal.UpdateStateAtomic(txHash, "submitted", "", "", ""); err != nil {
			s.storageFault = true
			return &SendResponse{Hash: txHash, State: "broadcast_unknown", To: record.To, Amount: record.Amount, Symbol: record.Symbol, Action: record.Action, CreatedAt: now.Format(time.RFC3339)}, nil
		}
		s.quotes.Remove(quote.ID)
		return &SendResponse{
			Hash:      txHash,
			State:     "submitted",
			To:        record.To,
			Amount:    record.Amount,
			Symbol:    record.Symbol,
			Action:    record.Action,
			CreatedAt: now.Format(time.RFC3339),
		}, nil
	}

	// Ambiguity or timeout: record broadcast_unknown, never pretend failure/no write
	if err := s.journal.UpdateStateAtomic(txHash, "broadcast_unknown", "", "", broadcastErr.Error()); err != nil {
		s.storageFault = true
	}
	s.quotes.Remove(quote.ID)
	return &SendResponse{
		Hash:      txHash,
		State:     "broadcast_unknown",
		To:        record.To,
		Amount:    record.Amount,
		Symbol:    record.Symbol,
		Action:    record.Action,
		CreatedAt: now.Format(time.RFC3339),
	}, nil
}

func (s *Service) Retry(ctx context.Context, hash string) (*SendResponse, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	record := s.journal.FindByHash(hash)
	if record == nil {
		return nil, chain.ErrNotFound
	}

	if record.State == "succeeded" || record.State == "reverted" {
		return &SendResponse{
			Hash:      record.Hash,
			State:     record.State,
			To:        record.To,
			Amount:    record.Amount,
			Symbol:    record.Symbol,
			Action:    record.Action,
			CreatedAt: record.CreatedAt.Format(time.RFC3339),
		}, nil
	}

	rawBytes, err := hexutil.Decode(record.SignedRaw)
	if err != nil {
		return nil, err
	}

	var tx types.Transaction
	if err := tx.UnmarshalBinary(rawBytes); err != nil {
		return nil, err
	}

	// Sepolia check
	if tx.Hash().Hex() != record.Hash {
		return nil, errors.New("儲存的交易雜湊不符")
	}
	if tx.ChainId().Cmp(big.NewInt(chain.SepoliaID)) != 0 {
		return nil, ErrWrongChain
	}

	broadcastErr := s.client.SendTransaction(ctx, &tx)
	newState := "submitted"
	var errStr string
	if broadcastErr != nil {
		newState = "broadcast_unknown"
		errStr = broadcastErr.Error()
	}

	if err := s.journal.UpdateStateAtomic(record.Hash, newState, "", "", errStr); err != nil {
		s.storageFault = true
		newState = "broadcast_unknown"
	}
	record.State = newState

	return &SendResponse{
		Hash:      record.Hash,
		State:     record.State,
		To:        record.To,
		Amount:    record.Amount,
		Symbol:    record.Symbol,
		Action:    record.Action,
		CreatedAt: record.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *Service) History(ctx context.Context) (*HistoryResponse, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	// Refresh recent and outstanding records without treating RPC errors as transaction failure.
	refreshError := ""
	for _, hash := range s.journal.RefreshHashes() {
		txInfo, err := s.client.Transaction(ctx, hash)
		if err == nil && txInfo != nil {
			if txInfo.State == "succeeded" || txInfo.State == "reverted" || txInfo.State == "reorg_detected" || txInfo.State == "pending" || txInfo.State == "receipt_unavailable" {
				if err := s.journal.UpdateStateAtomic(hash, txInfo.State, txInfo.Confirmations, txInfo.FeeETH, ""); err != nil {
					s.storageFault = true
					return nil, err
				}
			}
		}
		if errors.Is(err, chain.ErrNotFound) {
			record := s.journal.FindByHash(hash)
			if record != nil && (record.State == "succeeded" || record.State == "reverted") {
				if err := s.journal.UpdateStateAtomic(hash, "broadcast_unknown", "", "", ""); err != nil {
					s.storageFault = true
					return nil, err
				}
			}
		}
		if err != nil {
			refreshError = "部分交易未能完成鏈上查核；以下保留本機最後紀錄，請稍後更新。"
		}
	}

	return &HistoryResponse{
		Transactions: s.journal.ListHistory(),
		RefreshError: refreshError,
	}, nil
}
