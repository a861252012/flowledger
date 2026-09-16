package erc4337

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var (
	ErrClientNilEndpoint = errors.New("erc4337: RPC 端點 URL 不得為空")
	ErrReceiptNotFound   = errors.New("erc4337: 尚未查詢到收據 (仍處於 pending 狀態)")
)

// BundlerClient 定義與 EIP-4337 打包器 (Bundler) 節點互動之標準介面
type BundlerClient interface {
	SendUserOperation(ctx context.Context, op *UserOperation, entryPoint common.Address) (common.Hash, error)
	EstimateUserOperationGas(ctx context.Context, op *UserOperation, entryPoint common.Address) (*GasEstimate, error)
	GetUserOperationReceipt(ctx context.Context, hash common.Hash) (*UserOperationReceipt, error)
	WaitForUserOperationReceipt(ctx context.Context, hash common.Hash, pollInterval time.Duration) (*UserOperationReceipt, error)
}

// Client 實作 BundlerClient 介面
type Client struct {
	endpoint   string
	httpClient *http.Client
	nextID     atomic.Uint64
}

// ClientOption 提供客製化 Client 之選項
type ClientOption func(*Client)

// WithHTTPClient 自訂底層 http.Client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// NewClient 建立 Bundler JSON-RPC 客戶端
func NewClient(endpoint string, opts ...ClientOption) (*Client, error) {
	if endpoint == "" {
		return nil, ErrClientNilEndpoint
	}
	c := &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

func (c *Client) call(ctx context.Context, method string, params []any, result any) error {
	reqID := c.nextID.Add(1)
	rpcReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	}

	reqBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return fmt.Errorf("erc4337: 序列化 RPC 請求失敗: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return fmt.Errorf("erc4337: 建立 HTTP 請求失敗: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("erc4337: 執行 HTTP 請求失敗: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("erc4337: Bundler HTTP 錯誤代碼 %d: %s", httpResp.StatusCode, string(body))
	}

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("erc4337: 解析 RPC 回應失敗: %w", err)
	}

	if rpcResp.Error != nil {
		return rpcResp.Error
	}

	if result != nil && len(rpcResp.Result) > 0 {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("erc4337: 解析 RPC 結果失敗: %w", err)
		}
	}
	return nil
}

// SendUserOperation 呼叫 eth_sendUserOperation 發送 UserOperation 至 Bundler
func (c *Client) SendUserOperation(ctx context.Context, op *UserOperation, entryPoint common.Address) (common.Hash, error) {
	if op == nil {
		return common.Hash{}, ErrNilUserOp
	}
	if entryPoint == (common.Address{}) {
		return common.Hash{}, ErrInvalidEntryPoint
	}

	var hashHex string
	params := []any{op, entryPoint.Hex()}
	if err := c.call(ctx, "eth_sendUserOperation", params, &hashHex); err != nil {
		return common.Hash{}, err
	}

	hash, err := hexutil.Decode(hashHex)
	if err != nil || len(hash) != 32 {
		return common.Hash{}, fmt.Errorf("erc4337: Bundler 回傳之 userOpHash 格式無效 (%s)", hashHex)
	}
	return common.BytesToHash(hash), nil
}

// EstimateUserOperationGas 呼叫 eth_estimateUserOperationGas 取得 Gas 估算
func (c *Client) EstimateUserOperationGas(ctx context.Context, op *UserOperation, entryPoint common.Address) (*GasEstimate, error) {
	if op == nil {
		return nil, ErrNilUserOp
	}
	if entryPoint == (common.Address{}) {
		return nil, ErrInvalidEntryPoint
	}

	var est GasEstimate
	params := []any{op, entryPoint.Hex()}
	if err := c.call(ctx, "eth_estimateUserOperationGas", params, &est); err != nil {
		return nil, err
	}
	return &est, nil
}

// GetUserOperationReceipt 呼叫 eth_getUserOperationReceipt 查詢收據
func (c *Client) GetUserOperationReceipt(ctx context.Context, hash common.Hash) (*UserOperationReceipt, error) {
	var raw json.RawMessage
	params := []any{hash.Hex()}
	if err := c.call(ctx, "eth_getUserOperationReceipt", params, &raw); err != nil {
		return nil, err
	}

	if len(raw) == 0 || string(raw) == "null" {
		return nil, ErrReceiptNotFound
	}

	var receipt UserOperationReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return nil, fmt.Errorf("erc4337: 解析 UserOperationReceipt 失敗: %w", err)
	}
	return &receipt, nil
}

// WaitForUserOperationReceipt 輪詢等待收據產生，支援 context 超時控制
func (c *Client) WaitForUserOperationReceipt(ctx context.Context, hash common.Hash, pollInterval time.Duration) (*UserOperationReceipt, error) {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		receipt, err := c.GetUserOperationReceipt(ctx, hash)
		if err == nil && receipt != nil {
			return receipt, nil
		}
		if err != nil && !errors.Is(err, ErrReceiptNotFound) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
