package chain

import (
	"context"
	"errors"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const SepoliaID = 11155111

var (
	ErrAddress     = errors.New("地址格式不正確，請輸入 0x 開頭的 40 位十六進位地址")
	ErrHash        = errors.New("交易雜湊格式不正確，請輸入 0x 開頭的 64 位十六進位雜湊")
	ErrNetwork     = errors.New("RPC 連到其他網路，已停止操作；RPC 必須符合目前選擇的測試網")
	ErrUnavailable = errors.New("暫時無法取得目前測試網資料，請稍後重試")
	ErrTimeout     = errors.New("測試網查詢逾時，結果未知，請稍後重試")
	ErrNotFound    = errors.New("此 RPC 尚未找到這筆交易，請確認網路與雜湊，或稍後重試")
	hashPattern    = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
)

type Client struct {
	transport *fallbackTransport
	rpc       *ethclient.Client
	chainID   int64
}

func (c *Client) ChainID() int64 {
	if c.chainID == 0 {
		return SepoliaID
	}
	return c.chainID
}

func (c *Client) NativeSymbol() string {
	if c.ChainID() == 80002 {
		return "POL"
	}
	return "ETH"
}

func New(endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("SEPOLIA_RPC_URL 必須是 HTTP 或 HTTPS RPC 網址")
	}
	c, err := ethclient.Dial(endpoint)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Client{rpc: c}, nil
}

func (c *Client) Close() { c.rpc.Close() }

func (c *Client) checkNetwork(ctx context.Context) error {
	id, err := c.rpc.ChainID(ctx)
	if err != nil {
		return rpcError(err)
	}
	if !id.IsInt64() || id.Int64() != c.ChainID() {
		return ErrNetwork
	}
	return nil
}

func (c *Client) Network(ctx context.Context) (*Network, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	h, err := c.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, rpcError(err)
	}
	return networkToAPI(networkSnapshot{
		chainID:   c.ChainID(),
		block:     h.Number,
		blockTime: time.Unix(int64(h.Time), 0).UTC(),
		checkedAt: time.Now().UTC(),
	}), nil
}

func (c *Client) Balance(ctx context.Context, address string) (*Balance, error) {
	if !strings.HasPrefix(address, "0x") || !common.IsHexAddress(address) {
		return nil, ErrAddress
	}
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	h, err := c.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, rpcError(err)
	}
	a := common.HexToAddress(address)
	amount, err := c.rpc.BalanceAtHash(ctx, a, h.Hash())
	if err != nil {
		return nil, rpcError(err)
	}
	return balanceToAPI(balanceSnapshot{address: a, wei: amount, block: h.Number, checkedAt: time.Now().UTC()}), nil
}

// FormatETH preserves all 18 decimals without a float conversion.
func FormatETH(wei *big.Int) string {
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(wei, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil), fraction)
	if fraction.Sign() == 0 {
		return whole.String()
	}
	digits := fraction.String()
	return whole.String() + "." + strings.TrimRight(strings.Repeat("0", 18-len(digits))+digits, "0")
}

func (c *Client) Transaction(ctx context.Context, hash string) (*Transaction, error) {
	if !hashPattern.MatchString(hash) {
		return nil, ErrHash
	}
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	id := common.HexToHash(hash)
	r, err := c.rpc.TransactionReceipt(ctx, id)
	if errors.Is(err, ethereum.NotFound) {
		_, pending, lookupErr := c.rpc.TransactionByHash(ctx, id)
		if errors.Is(lookupErr, ethereum.NotFound) {
			return nil, ErrNotFound
		}
		if lookupErr != nil {
			return nil, rpcError(lookupErr)
		}
		state := transactionReceiptUnavailable
		if pending {
			state = transactionPending
		}
		return transactionToAPI(transactionSnapshot{hash: id, state: state, checkedAt: time.Now().UTC()}), nil
	}
	if err != nil {
		return nil, rpcError(err)
	}
	if r.BlockNumber == nil || r.EffectiveGasPrice == nil || r.TxHash != id {
		return nil, ErrUnavailable
	}
	canonical, err := c.rpc.HeaderByNumber(ctx, r.BlockNumber)
	if err != nil {
		return nil, rpcError(err)
	}
	if canonical.Hash() != r.BlockHash {
		return transactionToAPI(transactionSnapshot{hash: id, state: transactionReorgDetected, checkedAt: time.Now().UTC()}), nil
	}
	head, err := c.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, rpcError(err)
	}
	if head.Number.Cmp(r.BlockNumber) < 0 {
		return nil, ErrUnavailable
	}
	state := transactionReverted
	if r.Status == types.ReceiptStatusSuccessful {
		state = transactionSucceeded
	}
	confirmations := new(big.Int).Sub(head.Number, r.BlockNumber)
	confirmations.Add(confirmations, big.NewInt(1))
	finalized := false
	if finalHead, err := c.rpc.HeaderByNumber(ctx, big.NewInt(-3)); err == nil && finalHead != nil && finalHead.Number.Cmp(r.BlockNumber) >= 0 {
		// Recheck the receipt block after observing the finalized head.
		if verified, err := c.rpc.HeaderByNumber(ctx, r.BlockNumber); err == nil && verified.Hash() == r.BlockHash {
			finalized = true
		}
	}
	fee, err := c.ReceiptFee(ctx, r)
	if err != nil {
		return nil, err
	}
	return transactionToAPI(transactionSnapshot{
		finalized:     finalized,
		blockHash:     r.BlockHash,
		hash:          id,
		state:         state,
		block:         r.BlockNumber,
		confirmations: confirmations,
		gasUsed:       r.GasUsed,
		feeWei:        fee,
		checkedAt:     time.Now().UTC(),
	}), nil
}

func rpcError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	return ErrUnavailable
}
