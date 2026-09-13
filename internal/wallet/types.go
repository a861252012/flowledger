package wallet

import (
	"errors"
	"time"
)

var (
	ErrWalletExists                 = errors.New("錢包已存在，無法覆寫")
	ErrWalletNotFound               = errors.New("尚未建立或匯入錢包")
	ErrInvalidPassword              = errors.New("密碼長度必須介於 12 至 128 位元組")
	ErrPasswordMismatch             = errors.New("密碼錯誤，無法解密金鑰")
	ErrInvalidMnemonic              = errors.New("助記詞格式不正確或校驗失敗")
	ErrInvalidAddress               = errors.New("地址格式不正確，請輸入 0x 開頭的 40 位十六進位地址")
	ErrZeroAddress                  = errors.New("不可使用零地址")
	ErrMalformedChecksum            = errors.New("地址混合大小寫校驗和不正確")
	ErrWrongChain                   = errors.New("RPC 連到其他網路，已停止操作；FlowLedger 僅允許 Sepolia")
	ErrQuoteNotFound                = errors.New("找不到指定的報價或報價已過期")
	ErrQuoteExpired                 = errors.New("報價已過期，請重新建立報價")
	ErrQuoteStorageFull             = errors.New("報價數量已達上限 (256)，請稍後重試")
	ErrTxInFlight                   = errors.New("已有處理中或廣播結果未確認的交易，請等待該交易確認後再操作")
	ErrNonceMismatch                = errors.New("鏈上 Nonce 已變更，請重新建立報價")
	ErrInsufficientFunds            = errors.New("餘額不足以支付轉帳金額與最高 Gas 手續費")
	ErrNotContract                  = errors.New("指定合約地址在 Sepolia 上沒有 bytecode")
	ErrApprovalRace                 = errors.New("既有授權額度大於 0，為避免 ERC20 approve race pattern，必須先將授權額度歸零（approve 0）後才能設定新的非零額度")
	ErrUnlimitedAllowanceNotAllowed = errors.New("不支援無上限授權，請輸入明確的授權額度")
	ErrSimulationFailed             = errors.New("交易模擬執行失敗（eth_call 未通過）")
	ErrJournalFull                  = errors.New("交易日誌數量已達上限 (1000)，為保全歷史紀錄已拒絕新交易")
	ErrDecimalsTooLarge             = errors.New("代幣小數位數超出上限 36")
	ErrSymbolTooLong                = errors.New("代幣符號長度超出上限 32 字元")
	ErrTooManyScryptRequests        = errors.New("系統密碼運算繁忙，請稍後重試")
)

type WalletInfo struct {
	Exists    bool              `json:"exists"`
	Address   string            `json:"address"`
	Path      string            `json:"path"`
	CSRFToken string            `json:"csrfToken"`
	Exchange  map[string]string `json:"exchange"`
}

type CreateResponse struct {
	Address  string `json:"address"`
	Mnemonic string `json:"mnemonic"`
	Path     string `json:"path"`
}

type ImportResponse struct {
	Address string `json:"address"`
	Path    string `json:"path"`
}

type TokenInfo struct {
	Contract   string `json:"contract"`
	Symbol     string `json:"symbol"`
	Decimals   int    `json:"decimals"`
	Balance    string `json:"balance"`
	BalanceRaw string `json:"balanceRaw"`
}

type QuoteRequest struct {
	Action      string `json:"action"`
	To          string `json:"to"`
	Amount      string `json:"amount"`
	Contract    string `json:"contract,omitempty"`
	TokenOut    string `json:"tokenOut,omitempty"`
	SlippageBPS int    `json:"slippageBps,omitempty"`
	PoolFee     int    `json:"poolFee,omitempty"`
}

type QuoteResponse struct {
	ID                   string           `json:"id"`
	Action               string           `json:"action"`
	From                 string           `json:"from"`
	To                   string           `json:"to"`
	Contract             string           `json:"contract"`
	Symbol               string           `json:"symbol"`
	Amount               string           `json:"amount"`
	AmountRaw            string           `json:"amountRaw"`
	Nonce                string           `json:"nonce"`
	GasLimit             string           `json:"gasLimit"`
	MaxFeePerGas         string           `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string           `json:"maxPriorityFeePerGas"`
	MaxFeeETH            string           `json:"maxFeeEth"`
	TotalETH             string           `json:"totalEth"`
	Data                 string           `json:"data"`
	Method               string           `json:"method"`
	ExpiresAt            string           `json:"expiresAt"`
	Exchange             *ExchangePreview `json:"exchange,omitempty"`
}

type SendRequest struct {
	QuoteID  string `json:"quoteId"`
	Password string `json:"password"`
}

type SendResponse struct {
	Hash      string `json:"hash"`
	State     string `json:"state"`
	To        string `json:"to"`
	Amount    string `json:"amount"`
	Symbol    string `json:"symbol"`
	Action    string `json:"action"`
	CreatedAt string `json:"createdAt"`
}

type RetryRequest struct {
	Hash string `json:"hash"`
}

type HistoryItem struct {
	Hash          string `json:"hash"`
	State         string `json:"state"`
	To            string `json:"to"`
	Amount        string `json:"amount"`
	Symbol        string `json:"symbol"`
	Action        string `json:"action"`
	CreatedAt     string `json:"createdAt"`
	Confirmations string `json:"confirmations,omitempty"`
	FeeETH        string `json:"feeEth,omitempty"`
	Error         string `json:"error,omitempty"`
}

type HistoryResponse struct {
	Transactions []HistoryItem `json:"transactions"`
	RefreshError string        `json:"refreshError,omitempty"`
}

type JournalRecord struct {
	Hash          string    `json:"hash"`
	QuoteID       string    `json:"quoteId"`
	State         string    `json:"state"`
	To            string    `json:"to"`
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
}
