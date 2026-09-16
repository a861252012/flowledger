package erc4337

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// AA 標準 JSON-RPC 錯誤代碼 (EIP-4337 Bundler 規範)
const (
	ErrCodeValidationFailed       = -32500 // EntryPoint 驗證失敗 (AAxx 錯誤)
	ErrCodePaymasterDepositTooLow = -32501 // Paymaster 存款餘額不足
	ErrCodePaymasterRateLimited   = -32502 // Paymaster 頻率超限
	ErrCodeExecutionReverted      = -32503 // 執行被 Revert
	ErrCodeAlreadyKnown           = -32504 // UserOperation 已存在 mempool
	ErrCodeUnderpriced            = -32505 // Gas 費用過低，無法替換或納入
	ErrCodeRuleViolation          = -32506 // 違反操作碼或存取清單規則
	ErrCodeSignatureExpired       = -32507 // 簽章已過期或尚未生效
	ErrCodeInvalidParams          = -32602 // 參數格式錯誤
)

// JSONRPCRequest 定義標準 JSON-RPC 請求結構
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

// JSONRPCResponse 定義標準 JSON-RPC 回應結構
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError 定義標準 JSON-RPC 錯誤欄位
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("rpc 錯誤 (code: %d): %s, data: %s", e.Code, e.Message, string(e.Data))
	}
	return fmt.Sprintf("rpc 錯誤 (code: %d): %s", e.Code, e.Message)
}

// ParseAAErrorCode 嘗試解析 EntryPoint 核心錯誤碼（如 AA21, AA10, AA22）
func ParseAAErrorCode(errMsg string) string {
	parts := strings.FieldsSeq(errMsg)
	for p := range parts {
		if strings.HasPrefix(p, "AA") && len(p) >= 4 {
			return p[:4]
		}
	}
	return ""
}

// GasEstimate 封裝 eth_estimateUserOperationGas 估算結果
type GasEstimate struct {
	PreVerificationGas   *big.Int `json:"preVerificationGas"`
	VerificationGasLimit *big.Int `json:"verificationGasLimit"`
	CallGasLimit         *big.Int `json:"callGasLimit"`
}

type gasEstimateWire struct {
	PreVerificationGas   hexutil.Big `json:"preVerificationGas"`
	VerificationGasLimit hexutil.Big `json:"verificationGasLimit"`
	CallGasLimit         hexutil.Big `json:"callGasLimit"`
}

func (g *GasEstimate) MarshalJSON() ([]byte, error) {
	if g == nil {
		return nil, ErrNilField
	}
	wire := gasEstimateWire{
		PreVerificationGas:   hexutil.Big(*g.PreVerificationGas),
		VerificationGasLimit: hexutil.Big(*g.VerificationGasLimit),
		CallGasLimit:         hexutil.Big(*g.CallGasLimit),
	}
	return json.Marshal(wire)
}

func (g *GasEstimate) UnmarshalJSON(data []byte) error {
	var wire gasEstimateWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	g.PreVerificationGas = (*big.Int)(&wire.PreVerificationGas)
	g.VerificationGasLimit = (*big.Int)(&wire.VerificationGasLimit)
	g.CallGasLimit = (*big.Int)(&wire.CallGasLimit)
	return nil
}

// UserOperationReceipt 代表 eth_getUserOperationReceipt 回傳之收據
type UserOperationReceipt struct {
	UserOpHash    common.Hash    `json:"userOpHash"`
	EntryPoint    common.Address `json:"entryPoint"`
	Sender        common.Address `json:"sender"`
	Nonce         *big.Int       `json:"nonce"`
	Paymaster     common.Address `json:"paymaster,omitempty"`
	ActualGasCost *big.Int       `json:"actualGasCost"`
	ActualGasUsed *big.Int       `json:"actualGasUsed"`
	Success       bool           `json:"success"`
	Reason        string         `json:"reason,omitempty"`
	Receipt       *types.Receipt `json:"receipt,omitempty"`
	Logs          []*types.Log   `json:"logs"`
}

type receiptWire struct {
	UserOpHash    common.Hash    `json:"userOpHash"`
	EntryPoint    common.Address `json:"entryPoint"`
	Sender        common.Address `json:"sender"`
	Nonce         hexutil.Big    `json:"nonce"`
	Paymaster     common.Address `json:"paymaster,omitempty"`
	ActualGasCost hexutil.Big    `json:"actualGasCost"`
	ActualGasUsed hexutil.Big    `json:"actualGasUsed"`
	Success       bool           `json:"success"`
	Reason        string         `json:"reason,omitempty"`
	Receipt       *types.Receipt `json:"receipt,omitempty"`
	Logs          []*types.Log   `json:"logs"`
}

func (r *UserOperationReceipt) MarshalJSON() ([]byte, error) {
	if r == nil {
		return nil, ErrNilField
	}
	wire := receiptWire{
		UserOpHash:    r.UserOpHash,
		EntryPoint:    r.EntryPoint,
		Sender:        r.Sender,
		Nonce:         hexutil.Big(*r.Nonce),
		Paymaster:     r.Paymaster,
		ActualGasCost: hexutil.Big(*r.ActualGasCost),
		ActualGasUsed: hexutil.Big(*r.ActualGasUsed),
		Success:       r.Success,
		Reason:        r.Reason,
		Receipt:       r.Receipt,
		Logs:          r.Logs,
	}
	return json.Marshal(wire)
}

func (r *UserOperationReceipt) UnmarshalJSON(data []byte) error {
	var wire receiptWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	r.UserOpHash = wire.UserOpHash
	r.EntryPoint = wire.EntryPoint
	r.Sender = wire.Sender
	r.Nonce = (*big.Int)(&wire.Nonce)
	r.Paymaster = wire.Paymaster
	r.ActualGasCost = (*big.Int)(&wire.ActualGasCost)
	r.ActualGasUsed = (*big.Int)(&wire.ActualGasUsed)
	r.Success = wire.Success
	r.Reason = wire.Reason
	r.Receipt = wire.Receipt
	r.Logs = wire.Logs
	return nil
}
