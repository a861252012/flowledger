# Project: FlowLedger ERC-4337 Account Abstraction 模組

## Current status and boundary

這個 package 已完成 UserOperation wire types、v0.6／v0.7 編碼、hash、builder、signer、Bundler JSON-RPC client 與 Mock Bundler 測試。它目前仍是與 EOA 錢包分離的 **library/component implementation**，不是已整合到 FlowLedger UI 的 smart-account 產品流程。

| 範圍 | 狀態 | 證據／限制 |
|---|---|---|
| UserOperation types、wire encoding 與 hash | Complete | Unit 與 adversarial tests |
| Builder、gas calculation 與 signer | Complete | Unit、boundary 與 signature recovery tests |
| Bundler JSON-RPC client 與 error classification | Complete | `httptest` Mock Bundler integration tests |
| FlowLedger service／UI integration | Not implemented | 現有 EOA 流程刻意不變 |
| Deployed smart account 與 live bundler acceptance | Complete (Sepolia v0.6) | SimpleAccount 部署、UserOperation 收據及 1 wei 轉帳已驗證；見 [驗收紀錄](docs/erc4337-acceptance.md) |
| Paymaster、session key 與 account recovery | Not implemented | 不在目前 package 範圍 |

文件中的「端到端」若指 Mock Bundler，僅代表 package/component 邊界的整合測試；不代表已完成 smart account 部署、public bundler 廣播或真實鏈上收據驗收。

## Architecture
- 本模組作為 FlowLedger 的現代智慧合約錢包與帳戶抽象化（Account Abstraction）核心擴充元件，位於 `internal/wallet/erc4337/`（子套件 `package erc4337`）。
- 完全解耦原則：不更動既有 `internal/wallet/service.go` 的 EOA 發送流程，不寫入 `internal/wallet/journal.go` 既有交易日誌，保持既有工作流程 100% 穩定相容。
- 密碼學與 ABI 編碼：直接利用專案既有之 `github.com/ethereum/go-ethereum`（`accounts/abi`, `crypto`, `common`, `common/hexutil`），不引入任何外部第三方依賴。
- 零浮點數規範：全模組所有 Gas、費用、Nonce、數值計算嚴格採用 `*big.Int` 與純整數運算。
- 金鑰處理：透過 `Signer` 介面封裝 `KeystoreManager`，簽署後對直接持有的私鑰資料執行 best-effort 記憶體清除（`wipePrivateKey`）；Go runtime 與密碼學 library 的其他副本不在此保證範圍。

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | UserOperation 核心資料結構 | 定義 EIP-4337 標準 11 欄位 UserOperation 及 JSON Hex 序列化/反序列化，支援 v0.6 與 v0.7 格式轉換 | M1 | Survey R1 |
| 2 | ABI 編碼與解碼工具 | 實作 UserOperation 之 ABI Pack 與 Unpack 工具，支援各欄位 Keccak-256 預雜湊處理 | M1 | Survey R1 |
| 3 | 標準 UserOpHash 計算 | 實作雙層 Keccak-256 哈希計算，支援 Canonical EntryPoint（v0.6 `0x5FF137D4b0FDCD49DcA30c7CF57E578a026d2789`、v0.7 `0x0000000071727De22E5E9d8BAf0edAc6f37da032`）與 Chain ID 綁定 | M1 | Survey R1 |
| 4 | UserOperation Builder | 提供流暢建構器，支援由一般交易或 callData 建構 UserOp，包含 execute(address,uint256,bytes) callData 打包 | M2 | Survey R2 |
| 5 | 純整數 PreVerificationGas 計算 | 依據未壓縮與序列化位元組統計零位元組 (4 gas) 與非零位元組 (16 gas)，加上固定開銷，純整數運算 | M2 | Survey R2 |
| 6 | 費用參數計算與預估 | 支援 EIP-1559 費用參數估算（maxFeePerGas, maxPriorityFeePerGas），無浮點數運算 | M2 | Survey R2 |
| 7 | 金鑰安全 Signer 介面與實作 | 定義 UserOpSigner 介面，實作 KeystoreSigner 與 PrivateKeySigner，簽署後立即抹除私鑰，產出 65 位元組 ECDSA 簽章 | M2 | Survey R2 |
| 8 | Bundler JSON-RPC Client | 實作 BundlerClient，支援 eth_sendUserOperation、eth_estimateUserOperationGas、eth_getUserOperationReceipt 呼叫 | M3 | Survey R3 |
| 9 | AA 錯誤碼解析與分類 | 支援標準 Bundler RPC 錯誤碼（-32500 至 -32508、-32602）與 EntryPoint 核心代碼（AA10 至 AA99）解析 | M3 | Survey R3 |
| 10 | 執行緒安全 Mock Bundler 伺服器 | 實作 MockBundlerServer（使用 net/http/httptest），模擬 Bundler 回應、驗證失敗注入與收據輪詢生命週期 | M3 | Survey R3 |
| 11 | Package 整合測試套件 (Tiers 1-4) | 涵蓋功能、極限邊界、跨功能組合與 Mock Bundler 黑箱場景；不代表 public bundler 鏈上 E2E | M4 (Integration Track) | Survey R4 |
| 12 | 對抗性覆蓋強化與 Race/Vet 檢查 | 執行對抗性邊界測試、`go test -race ./internal/wallet/...` 與 `go vet ./...` | M4 (Hardening) | Survey R4 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | UserOperation 核心結構與 ABI/Hash 計算 (R1) | Features 1, 2, 3: types.go, encoding.go, hash.go, 測試向量與單元測試 | None | COMPLETE |
| M2 | UserOperation Signer 與 Builder (R2) | Features 4, 5, 6, 7: builder.go, signer.go, PreVerificationGas, Keystore 整合 | M1 | COMPLETE |
| M3 | Bundler JSON-RPC Client 與 Mock 架構 (R3) | Features 8, 9, 10: client.go, rpc_types.go, mock_server.go, client_test.go | M1, M2 | COMPLETE (MOCK) |
| M4 | Package hardening | 對抗性、boundary、JSON concurrency、Race 與 Vet 檢查 | M1, M2, M3 | COMPLETE (LOCAL) |
| M5 | Live smart-account vertical slice | Sepolia、EntryPoint v0.6、SimpleAccount 部署與 1 wei 轉帳通過；見驗收紀錄 | M1–M4 | COMPLETE (SCOPED) |

## Interface Contracts

### M1 ↔ M2: UserOperation 結構與 Hash 計算
```go
// UserOperation 代表 EIP-4337 標準資料結構
type UserOperation struct {
    Sender               common.Address `json:"sender"`
    Nonce                *big.Int       `json:"nonce"`
    InitCode             []byte         `json:"initCode"`
    CallData             []byte         `json:"callData"`
    CallGasLimit         *big.Int       `json:"callGasLimit"`
    VerificationGasLimit *big.Int       `json:"verificationGasLimit"`
    PreVerificationGas   *big.Int       `json:"preVerificationGas"`
    MaxFeePerGas         *big.Int       `json:"maxFeePerGas"`
    MaxPriorityFeePerGas *big.Int       `json:"maxPriorityFeePerGas"`
    PaymasterAndData     []byte         `json:"paymasterAndData"`
    Signature            []byte         `json:"signature"`
}

// UserOpHash 計算函式
func GetUserOpHash(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) (common.Hash, error)
func PackUserOp(userOp *UserOperation) ([]byte, error)
```

### M2 ↔ M3: Builder, Signer ↔ Client
```go
// UserOpSigner 簽署介面
type UserOpSigner interface {
    Address() common.Address
    SignHash(hash common.Hash) ([]byte, error)
    SignUserOp(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error)
}

// Builder 建構介面
type Builder struct {
    // 包含鏈式呼叫方法，產出完整的 UserOperation 並計算 PreVerificationGas
}
func NewBuilder(entryPoint common.Address, chainID *big.Int) *Builder
func (b *Builder) Build() (*UserOperation, error)
```

### M3 ↔ External: BundlerClient
```go
type BundlerClient interface {
    SendUserOperation(ctx context.Context, op *UserOperation, entryPoint common.Address) (common.Hash, error)
    EstimateUserOperationGas(ctx context.Context, op *UserOperation, entryPoint common.Address) (*GasEstimate, error)
    GetUserOperationReceipt(ctx context.Context, hash common.Hash) (*UserOperationReceipt, error)
}
```

## Code Layout
- `internal/wallet/erc4337/types.go`：核心資料結構、常數（Canonical EntryPoints、預設參數）。
- `internal/wallet/erc4337/encoding.go`：ABI 打包、解包、十六進位 JSON 序列化工具。
- `internal/wallet/erc4337/hash.go`：EIP-4337 標準雙層 Keccak-256 UserOpHash 計算。
- `internal/wallet/erc4337/signer.go`：UserOpSigner 介面、KeystoreSigner、PrivateKeySigner、抹除私鑰安全邏輯。
- `internal/wallet/erc4337/builder.go`：UserOp 建構器、execute calldata 打包、純整數 PreVerificationGas 與費用計算。
- `internal/wallet/erc4337/client.go`：Bundler JSON-RPC Client、AA 錯誤代碼解析器。
- `internal/wallet/erc4337/rpc_types.go`：JSON-RPC 請求/回應與 Receipt 資料結構。
- `internal/wallet/erc4337/mock_server.go`：執行緒安全 Mock Bundler Server（供單元與端對端測試共用）。
- `internal/wallet/erc4337/erc4337_test.go`：單元測試、官方測試向量驗證、簽章還原測試。
- `internal/wallet/erc4337/client_test.go`：RPC 客戶端、Mock 伺服器、AA 錯誤模擬與收據輪詢測試。
- `test/e2e/erc4337/`：端對端整合測試套件（4-Tier 測試案例與 runner）。
