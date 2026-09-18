package wallet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

type BoundQuote struct {
	RollupFeeWei         *big.Int
	ReplacementERC20     bool
	ReplacementCount     int
	ReplacementHash      TransactionHash
	ID                   QuoteID
	Action               TransactionAction
	From                 common.Address
	To                   common.Address // Real recipient or spender
	TxTo                 common.Address // Transaction target (recipient for ETH, contract for token)
	Contract             common.Address
	Symbol               string
	Decimals             int
	Amount               string
	AmountRaw            *big.Int
	TxValue              *big.Int
	Nonce                uint64
	GasLimit             uint64
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	MaxFeeETH            string
	TotalETH             string
	TotalETHWei          *big.Int
	Data                 []byte
	Method               string
	CreatedAt            time.Time
	ExpiresAt            time.Time
	Exchange             *ExchangePreview
}

type QuoteStore struct {
	mu     sync.Mutex
	quotes map[QuoteID]*BoundQuote
}

func NewQuoteStore() *QuoteStore {
	return &QuoteStore{
		quotes: make(map[QuoteID]*BoundQuote),
	}
}

func (qs *QuoteStore) cleanupExpiredLocked(now time.Time) {
	for id, q := range qs.quotes {
		if now.After(q.ExpiresAt) {
			delete(qs.quotes, id)
		}
	}
}

func (qs *QuoteStore) Add(q *BoundQuote) error {
	if q == nil {
		return ErrQuoteNotFound
	}
	if _, err := ParseQuoteID(string(q.ID)); err != nil {
		return err
	}
	qs.mu.Lock()
	defer qs.mu.Unlock()

	now := time.Now().UTC()
	qs.cleanupExpiredLocked(now)

	if len(qs.quotes) >= 256 {
		return ErrQuoteStorageFull
	}
	qs.quotes[q.ID] = q
	return nil
}

func (qs *QuoteStore) Get(id string) (*BoundQuote, error) {
	quoteID, err := ParseQuoteID(id)
	if err != nil {
		return nil, ErrQuoteNotFound
	}
	qs.mu.Lock()
	defer qs.mu.Unlock()

	now := time.Now().UTC()
	qs.cleanupExpiredLocked(now)

	q, ok := qs.quotes[quoteID]
	if !ok {
		return nil, ErrQuoteNotFound
	}
	return q, nil
}

func (qs *QuoteStore) Remove(id QuoteID) {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	delete(qs.quotes, id)
}

// ChainQuoteProvider defines RPC operations needed to generate a bound quote.
type ChainQuoteProvider interface {
	ChainCaller
	ChainID() int64
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
}

// CreateQuote builds and validates a server-bound fee quote.
func CreateQuote(ctx context.Context, provider ChainQuoteProvider, from common.Address, command QuoteCommand) (*BoundQuote, error) {
	action := command.Action
	// The request has already been validated.
	targetAddr := common.HexToAddress(string(command.To))

	var (
		contractAddr common.Address
		symbol       = "ETH"
		decimals     = 18
		txTo         common.Address
		txValue      *big.Int
		calldata     []byte
		methodName   string
		exchange     *ExchangePreview
		amountRaw    *big.Int
		amount       = command.Amount
	)

	switch action {
	case ActionVaultDeposit, ActionVaultWithdraw:
		if provider.ChainID() != 11155111 {
			return nil, errors.New("此合約目前僅支援 Ethereum Sepolia 測試網")
		}
		if command.Contract == "" {
			return nil, errors.New("尚未設定合約地址，暫時無法操作")
		}
		if targetAddr != from {
			return nil, errors.New("合約操作的錢包地址不符，請重新預估")
		}
		parsedAmount, err := ParseUnits(command.Amount, 18)
		if err != nil {
			return nil, err
		}
		if parsedAmount.Sign() <= 0 {
			if action == ActionVaultWithdraw {
				return nil, errors.New("取回金額必須大於 0")
			}
			return nil, errors.New("存入金額必須大於 0")
		}
		contractAddr = common.HexToAddress(string(command.Contract))
		if err := VerifyContractBytecode(ctx, provider, contractAddr); err != nil {
			return nil, err
		}
		txTo, amountRaw = contractAddr, parsedAmount
		txValue = parsedAmount
		methodName = "deposit()"
		if action == ActionVaultDeposit {
			calldata, err = ethVaultABI.Pack("deposit")
		} else {
			balance, balanceErr := QueryVaultBalanceOf(ctx, provider, contractAddr, from)
			if balanceErr != nil {
				return nil, balanceErr
			}
			if balance.Cmp(parsedAmount) < 0 {
				return nil, errors.New("合約餘額不足，請減少取回金額")
			}
			txValue = big.NewInt(0)
			methodName = "withdraw(uint256)"
			calldata, err = ethVaultABI.Pack("withdraw", parsedAmount)
		}
		if err != nil {
			return nil, err
		}
		if err := SimulateVaultCall(ctx, provider, from, txTo, txValue, calldata); err != nil {
			return nil, err
		}

	case ActionWrap, ActionUnwrap, ActionSwap:
		if targetAddr != from {
			return nil, errors.New("兌換資產只能回到自己的錢包")
		}
		prepared, err := PrepareExchange(ctx, provider, from, command)
		if err != nil {
			return nil, err
		}
		txTo, txValue, calldata, methodName = prepared.TxTo, prepared.Value, prepared.Data, prepared.Method
		contractAddr, symbol, decimals, exchange = prepared.Contract, prepared.Symbol, prepared.Decimals, prepared.Preview

	case ActionETH:
		txTo = targetAddr
		parsedAmount, err := ParseUnits(command.Amount, 18)
		if err != nil {
			return nil, err
		}
		if parsedAmount.Sign() <= 0 {
			return nil, errors.New("轉帳金額必須大於 0")
		}
		txValue = parsedAmount
		amountRaw = parsedAmount
		calldata = []byte{}
		methodName = "ETH transfer"

	case ActionTransfer, ActionApprove:
		if command.Contract == "" {
			if action == ActionTransfer {
				return nil, errors.New("代幣轉帳必須指定 contract 合約地址")
			}
			return nil, errors.New("代幣授權必須指定 contract 合約地址")
		}
		contractAddr = common.HexToAddress(string(command.Contract))
		txTo = contractAddr
		txValue = big.NewInt(0)

		sym, dec, err := QueryERC20Metadata(ctx, provider, contractAddr)
		if err != nil {
			return nil, err
		}
		symbol, decimals = sym, dec
		trustedSymbol, trustedDecimals, trusted := trustedEVMToken(provider.ChainID(), contractAddr)
		var parsedAmount *big.Int
		if trusted {
			if sym != trustedSymbol || dec != trustedDecimals {
				return nil, errors.New("RPC 回傳的代幣資料與內建登錄不符")
			}
			parsedAmount, err = ParseUnits(command.Amount, trustedDecimals)
		} else {
			parsedAmount, err = ParseRawTokenAmount(command.AmountRaw)
			if err == nil {
				amount = FormatUnits(parsedAmount, decimals)
			}
		}
		if err != nil {
			return nil, err
		}
		amountRaw = parsedAmount
		if action == ActionTransfer {
			if parsedAmount.Sign() <= 0 {
				return nil, errors.New("轉帳代幣數量必須大於 0")
			}

			// Check sender token balance
			bal, err := QueryERC20BalanceOf(ctx, provider, contractAddr, from)
			if err != nil {
				return nil, err
			}
			if bal.Cmp(parsedAmount) < 0 {
				return nil, errors.New("代幣餘額不足")
			}
		} else {
			if parsedAmount.Cmp(maxUint256) == 0 {
				return nil, ErrUnlimitedAllowanceNotAllowed
			}
			if parsedAmount.Sign() < 0 {
				return nil, errors.New("授權數量不可為負數")
			}

			// Enforce revoke-to-zero before nonzero->nonzero approval to prevent race condition
			currentAllowance, err := QueryERC20Allowance(ctx, provider, contractAddr, from, targetAddr)
			if err != nil {
				return nil, err
			}
			if currentAllowance.Sign() > 0 && parsedAmount.Sign() > 0 {
				return nil, ErrApprovalRace
			}
		}

		methodName = string(action)
		calldata, err = erc20ABI.Pack(methodName, targetAddr, parsedAmount)
		if err != nil {
			return nil, err
		}

		// Independent decode verification
		decoded, err := DecodeERC20Calldata(calldata, decimals)
		if err != nil || decoded.Method != methodName || decoded.Target != targetAddr || decoded.RawAmount.Cmp(parsedAmount) != 0 {
			return nil, errors.New("calldata 驗證失敗")
		}

		// Simulation check
		if err := SimulateERC20Call(ctx, provider, from, contractAddr, calldata); err != nil {
			return nil, err
		}

	default:
		return nil, errors.New("不支援的 action 操作，僅允許 eth、transfer、approve、wrap、unwrap 或 swap")
	}

	// Head & Gas parameters
	head, err := provider.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	if head == nil || head.BaseFee == nil || head.BaseFee.Sign() < 0 {
		return nil, errors.New("無法取得 EIP-1559 基本費用，請稍後重試")
	}
	baseFee := head.BaseFee

	suggestedTip, err := provider.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, err
	}
	if suggestedTip == nil || suggestedTip.Sign() < 0 {
		return nil, errors.New("無法取得優先費用")
	}
	maxPriorityFeePerGas := new(big.Int).Set(suggestedTip)

	// maxFeePerGas = 2 * baseFee + maxPriorityFeePerGas
	maxFeePerGas := new(big.Int).Mul(baseFee, big.NewInt(2))
	maxFeePerGas.Add(maxFeePerGas, maxPriorityFeePerGas)

	// Stop early if the balance cannot cover the value plus 21,000 gas at the fee cap.
	// EstimateGas would fail for insufficient funds too.
	ethBalance, err := provider.BalanceAt(ctx, from, nil)
	if err != nil {
		return nil, err
	}
	minGasFee := new(big.Int).Mul(big.NewInt(21000), maxFeePerGas)
	minRequiredETH := new(big.Int).Add(txValue, minGasFee)
	if ethBalance.Cmp(minRequiredETH) < 0 {
		return nil, ErrInsufficientFunds
	}

	// Estimate every transfer, including ETH sent to smart-contract recipients.
	est, err := provider.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &txTo, GasFeeCap: maxFeePerGas, GasTipCap: maxPriorityFeePerGas, Value: txValue, Data: calldata})
	if err != nil {
		return nil, err
	}
	if est < 21000 || est > head.GasLimit || est > ^uint64(0)/6*5 {
		return nil, errors.New("Gas 預估值無效")
	}
	gasLimit := min(est+est/5, head.GasLimit)
	// Nonce
	nonce, err := provider.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}

	// Compute maxFeeETH = gasLimit * maxFeePerGas
	maxFeeETHWei := new(big.Int).Mul(new(big.Int).SetUint64(gasLimit), maxFeePerGas)
	maxFeeETHStr := FormatUnits(maxFeeETHWei, 18)

	// totalETH = txValue (ETH only) + maxFeeETH
	totalETHWei := new(big.Int).Add(txValue, maxFeeETHWei)
	totalETHStr := FormatUnits(totalETHWei, 18)

	// Final check with exact gasLimit
	if ethBalance.Cmp(totalETHWei) < 0 {
		return nil, ErrInsufficientFunds
	}

	// Generate random 16-byte quote ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, err
	}
	quoteID := QuoteID(hex.EncodeToString(idBytes))

	now := time.Now().UTC()
	expiresAt := now.Add(120 * time.Second)
	if exchange != nil && exchange.Deadline != "" {
		expiresAt, _ = time.Parse(time.RFC3339, exchange.Deadline)
	}

	if amountRaw == nil {
		amountRaw, _ = ParseUnits(command.Amount, decimals)
	}

	return &BoundQuote{
		ID:                   quoteID,
		Action:               action,
		From:                 from,
		To:                   targetAddr,
		TxTo:                 txTo,
		Contract:             contractAddr,
		Symbol:               symbol,
		Decimals:             decimals,
		Amount:               amount,
		AmountRaw:            amountRaw,
		TxValue:              txValue,
		Nonce:                nonce,
		GasLimit:             gasLimit,
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		MaxFeeETH:            maxFeeETHStr,
		TotalETH:             totalETHStr,
		TotalETHWei:          totalETHWei,
		Data:                 calldata,
		Method:               methodName,
		CreatedAt:            now,
		ExpiresAt:            expiresAt,
		Exchange:             exchange,
	}, nil
}

// ToResponse converts BoundQuote to the API QuoteResponse format.
func (q *BoundQuote) ToResponse() *QuoteResponse {
	contractStr := ""
	if q.Contract != (common.Address{}) {
		contractStr = q.Contract.Hex()
	}
	dataStr := "0x"
	if len(q.Data) > 0 {
		dataStr = hexutil.Encode(q.Data)
	}

	rollup := ""
	if q.RollupFeeWei != nil {
		rollup = FormatUnits(q.RollupFeeWei, 18)
	}
	return &QuoteResponse{
		RollupFeeETH:         rollup,
		ID:                   string(q.ID),
		Action:               string(q.Action),
		From:                 q.From.Hex(),
		To:                   q.To.Hex(),
		Contract:             contractStr,
		Symbol:               q.Symbol,
		Amount:               q.Amount,
		AmountRaw:            q.AmountRaw.String(),
		Nonce:                new(big.Int).SetUint64(q.Nonce).String(),
		GasLimit:             new(big.Int).SetUint64(q.GasLimit).String(),
		MaxFeePerGas:         q.MaxFeePerGas.String(),
		MaxPriorityFeePerGas: q.MaxPriorityFeePerGas.String(),
		MaxFeeETH:            q.MaxFeeETH,
		TotalETH:             q.TotalETH,
		Data:                 dataStr,
		Method:               q.Method,
		ExpiresAt:            q.ExpiresAt.Format(time.RFC3339),
		Exchange:             q.Exchange,
	}
}
