package erc4337

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// MockBundlerServer 提供執行緒安全之 Bundler JSON-RPC 測試模擬伺服器
type MockBundlerServer struct {
	mu       sync.RWMutex
	server   *httptest.Server
	requests []JSONRPCRequest
	receipts map[common.Hash]*UserOperationReceipt
	errors   map[string]*JSONRPCError
}

// NewMockBundlerServer 建立並啟動 Mock Bundler 伺服器
func NewMockBundlerServer() *MockBundlerServer {
	m := &MockBundlerServer{
		receipts: make(map[common.Hash]*UserOperationReceipt),
		errors:   make(map[string]*JSONRPCError),
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handleHTTP))
	return m
}

// URL 回傳伺服器端點 URL
func (m *MockBundlerServer) URL() string {
	return m.server.URL
}

// Close 關閉伺服器
func (m *MockBundlerServer) Close() {
	m.server.Close()
}

// SetReceipt 設定指定 hash 的查詢收據
func (m *MockBundlerServer) SetReceipt(hash common.Hash, receipt *UserOperationReceipt) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receipts[hash] = receipt
}

// SetError 設定指定方法回傳之 JSON-RPC 錯誤
func (m *MockBundlerServer) SetError(method string, code int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[method] = &JSONRPCError{
		Code:    code,
		Message: message,
	}
}

// RecordedRequests 回傳所有收到的 RPC 請求拷貝
func (m *MockBundlerServer) RecordedRequests() []JSONRPCRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]JSONRPCRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

func (m *MockBundlerServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.requests = append(m.requests, req)
	customErr := m.errors[req.Method]
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	if customErr != nil {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   customErr,
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	switch req.Method {
	case "eth_sendUserOperation":
		if len(req.Params) < 2 {
			m.writeError(w, req.ID, ErrCodeInvalidParams, "參數不足")
			return
		}
		opBytes, err := json.Marshal(req.Params[0])
		if err != nil {
			m.writeError(w, req.ID, ErrCodeInvalidParams, "無效的 userOp")
			return
		}
		var op UserOperation
		if err := json.Unmarshal(opBytes, &op); err != nil {
			m.writeError(w, req.ID, ErrCodeInvalidParams, fmt.Sprintf("反序列化 userOp 失敗: %v", err))
			return
		}
		entryPointHex, ok := req.Params[1].(string)
		if !ok {
			m.writeError(w, req.ID, ErrCodeInvalidParams, "entryPoint 必須為十六進位字串")
			return
		}
		entryPoint := common.HexToAddress(entryPointHex)

		hash, err := GetUserOpHash(&op, entryPoint, big.NewInt(11155111))
		if err != nil {
			m.writeError(w, req.ID, ErrCodeValidationFailed, err.Error())
			return
		}

		resBytes, _ := json.Marshal(hexutil.Encode(hash.Bytes()))
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resBytes,
		})

	case "eth_estimateUserOperationGas":
		if len(req.Params) < 2 {
			m.writeError(w, req.ID, ErrCodeInvalidParams, "參數不足")
			return
		}
		est := &GasEstimate{
			PreVerificationGas:   big.NewInt(50000),
			VerificationGasLimit: big.NewInt(100000),
			CallGasLimit:         big.NewInt(200000),
		}

		resBytes, _ := json.Marshal(est)
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resBytes,
		})

	case "eth_getUserOperationReceipt":
		if len(req.Params) < 1 {
			m.writeError(w, req.ID, ErrCodeInvalidParams, "缺少 hash 參數")
			return
		}
		hashHex, ok := req.Params[0].(string)
		if !ok {
			m.writeError(w, req.ID, ErrCodeInvalidParams, "hash 格式錯誤")
			return
		}
		targetHash := common.HexToHash(hashHex)

		m.mu.RLock()
		receipt := m.receipts[targetHash]
		m.mu.RUnlock()

		if receipt == nil {
			json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage("null"),
			})
			return
		}

		resBytes, _ := json.Marshal(receipt)
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resBytes,
		})

	default:
		m.writeError(w, req.ID, -32601, fmt.Sprintf("方法 %s 不存在", req.Method))
	}
}

func (m *MockBundlerServer) writeError(w http.ResponseWriter, id any, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	json.NewEncoder(w).Encode(resp)
}
