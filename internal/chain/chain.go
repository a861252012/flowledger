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
	ErrNetwork     = errors.New("RPC 連到其他網路，已停止操作；FlowLedger 僅允許 Sepolia")
	ErrUnavailable = errors.New("暫時無法取得 Sepolia 資料，請稍後重試")
	ErrTimeout     = errors.New("Sepolia 查詢逾時，結果未知，請稍後重試")
	ErrNotFound    = errors.New("此 RPC 尚未找到這筆 Sepolia 交易，請確認網路與雜湊，或稍後重試")
	hashPattern    = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
)

type Client struct{ rpc *ethclient.Client }

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
	if id.Cmp(big.NewInt(SepoliaID)) != 0 {
		return ErrNetwork
	}
	return nil
}

type Network struct {
	ChainID   int       `json:"chainId"`
	Block     string    `json:"block"`
	BlockTime time.Time `json:"blockTime"`
	CheckedAt time.Time `json:"checkedAt"`
}

func (c *Client) Network(ctx context.Context) (*Network, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	h, err := c.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, rpcError(err)
	}
	return &Network{SepoliaID, h.Number.String(), time.Unix(int64(h.Time), 0).UTC(), time.Now().UTC()}, nil
}

type Balance struct {
	Address   string    `json:"address"`
	Wei       string    `json:"wei"`
	ETH       string    `json:"eth"`
	Block     string    `json:"block"`
	CheckedAt time.Time `json:"checkedAt"`
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
	return &Balance{a.Hex(), amount.String(), FormatETH(amount), h.Number.String(), time.Now().UTC()}, nil
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

type Transaction struct {
	Hash          string    `json:"hash"`
	State         string    `json:"state"`
	Block         string    `json:"block,omitempty"`
	Confirmations string    `json:"confirmations,omitempty"`
	GasUsed       string    `json:"gasUsed,omitempty"`
	FeeETH        string    `json:"feeEth,omitempty"`
	CheckedAt     time.Time `json:"checkedAt"`
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
		state := "receipt_unavailable"
		if pending {
			state = "pending"
		}
		return &Transaction{Hash: id.Hex(), State: state, CheckedAt: time.Now().UTC()}, nil
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
		return &Transaction{Hash: id.Hex(), State: "reorg_detected", CheckedAt: time.Now().UTC()}, nil
	}
	head, err := c.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, rpcError(err)
	}
	if head.Number.Cmp(r.BlockNumber) < 0 {
		return nil, ErrUnavailable
	}
	state := "reverted"
	if r.Status == types.ReceiptStatusSuccessful {
		state = "succeeded"
	}
	confirmations := new(big.Int).Sub(head.Number, r.BlockNumber)
	confirmations.Add(confirmations, big.NewInt(1))
	fee := new(big.Int).Mul(new(big.Int).SetUint64(r.GasUsed), r.EffectiveGasPrice)
	return &Transaction{
		Hash: id.Hex(), State: state, Block: r.BlockNumber.String(),
		Confirmations: confirmations.String(), GasUsed: new(big.Int).SetUint64(r.GasUsed).String(),
		FeeETH: FormatETH(fee), CheckedAt: time.Now().UTC(),
	}, nil
}

func rpcError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	return ErrUnavailable
}
