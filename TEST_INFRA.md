# E2E Test Infra: FlowLedger ERC-4337 Account Abstraction 模組

## Test Philosophy
- 黑箱、需求導向（Opaque-box, requirement-driven）：測試案例完全依據 ORIGINAL_REQUEST.md 與 ERC-4337 規範設計，不依賴實作細節。
- 測試方法論：Category-Partition + 邊界值分析 (BVA) + 成對組合測試 (Pairwise) + 真實應用負載 (Workload Testing)。
- 嚴格安全保證：涵蓋正常成功流程、極端邊界、異常與 AA 錯誤注入、以及並發呼叫下的 Race 檢測。

## Feature Inventory
| # | Feature | Source (Requirement) | Tier 1 | Tier 2 | Tier 3 |
|---|---------|----------------------|:------:|:------:|:------:|
| 1 | UserOperation 結構與 ABI 編解碼 | ORIGINAL_REQUEST §R1 | 5 | 5 | ✓ |
| 2 | UserOpHash 與 Canonical EntryPoint 計算 | ORIGINAL_REQUEST §R1 | 5 | 5 | ✓ |
| 3 | UserOp Builder 與純整數 PreVerificationGas 計算 | ORIGINAL_REQUEST §R2 | 5 | 5 | ✓ |
| 4 | 金鑰管理整合與零洩漏 65-byte ECDSA 簽署 | ORIGINAL_REQUEST §R2 | 5 | 5 | ✓ |
| 5 | Bundler eth_sendUserOperation 呼叫與 Hash 檢驗 | ORIGINAL_REQUEST §R3 | 5 | 5 | ✓ |
| 6 | Bundler eth_estimateUserOperationGas 估算呼叫 | ORIGINAL_REQUEST §R3 | 5 | 5 | ✓ |
| 7 | Bundler eth_getUserOperationReceipt 輪詢與收據解析 | ORIGINAL_REQUEST §R3 | 5 | 5 | ✓ |
| 8 | Bundler AA10-AA99 驗證失敗與錯誤解析 | ORIGINAL_REQUEST §R3 | 5 | 5 | ✓ |

## Test Architecture
- 測試執行器（Test Runner）：透過標準 Go 測試工具鏈執行 `go test -race -v ./test/e2e/erc4337/...` 與 `go test -race -v ./internal/wallet/...`。
- 測試案例格式：完全自包含測試，搭配執行緒安全之 `MockBundlerServer` 提供可預測的 JSON-RPC 伺服器端模擬。
- 目錄配置：`test/e2e/erc4337/` 放置端對端驗收測試，`internal/wallet/erc4337/` 放置單元與整合測試。

## Real-World Application Scenarios (Tier 4)
| # | Scenario | Features Exercised | Complexity |
|---|----------|--------------------|------------|
| 1 | 完整轉帳流程：透過 Builder 建構轉帳 UserOp，Keystore 簽章，發送給 Bundler 並輪詢確認收據 | F1, F2, F3, F4, F5, F7 | High |
| 2 | 合約部署流程：包含 initCode 的 UserOp 建構、估算 Gas、簽章與發送，模擬初次帳戶建立 | F1, F2, F3, F4, F6, F7 | High |
| 3 | Paymaster 代付流程：包含 paymasterAndData 的 UserOp 建構、簽章與 Bundler 驗證 | F1, F2, F3, F4, F5, F7 | Medium |
| 4 | 驗證失敗防禦：模擬簽名錯誤 (AA24)、資金不足 (AA21) 與 Paymaster 存款不足 (AA31) 之異常處理 | F5, F8 | Medium |
| 5 | 高並發操作與收據輪詢：多個 Goroutine 同時建構、簽署並發送 UserOp，驗證零資料競態與執行緒安全性 | F1-F8 | High |

## Coverage Thresholds
- Tier 1 (功能覆蓋): ≥5 per feature（8 features × 5 = 40 案例）
- Tier 2 (邊界與極端): ≥5 per feature（8 features × 5 = 40 案例）
- Tier 3 (跨功能成對組合): ≥8 案例
- Tier 4 (真實應用場景): 5 完整場景
- 總測試案例門檻：≥ 93 個測試案例
