# FlowLedger (Testnet Wallet Lab) 全方位專業審查與 Web3 轉職評估報告（定案版）

本報告針對具備 **PHP / Web2 後端架構背景**（相關年資與實績需由個人履歷與實際工作案例支撐，非本專案代碼直接推導），計劃以 Go 多鏈測試網錢包專案（Testnet Wallet Lab，代碼庫：`testnet-wallet-lab`）作為關鍵作品轉職 Web3 後端／基礎設施工程師的求職者，進行嚴格立足於程式碼現實的技術審查。

本報告依據 **Linus Torvalds 的工程哲學**（實用主義、好品味、對簡潔的執念、程式碼服務於現實）與多輪嚴謹代碼覆核，剔除所有未經證實的過度包裝，分別從 **HR（技術招聘）**、**CTO（技術決策者）** 與 **技術面試官（架構師／鏈上工程師）** 三個維度進行事實校準，並確立最高投報率（ROI）的務實落地路線。

---

## 一、背景定位：Web2 後端轉職 Web3 的務實優勢

- **優勢本質**：
  若求職者具備多年 Web2 後端經驗（需依個人實際履歷為準），其所建立的資料庫交易事務（ACID）、悲觀鎖與樂觀鎖、死信佇列、重試退避機制與狀態一致性思維，在處理 Web3 不可靠網路（RPC 節點不穩定、區塊重組、Gas 價格劇烈波動）時可作為轉職時的互補優勢。
- **專案定位**：
  **Testnet Wallet Lab：Go 多鏈交易編排與測試網錢包**。
  使用成熟的鏈上函式庫（如 `go-ethereum`、`solana-go`），自行實作完整的交易組裝、狀態追蹤、檔案快照持久化與故障復原機制。坦誠定位為「多鏈實驗與交易編排實作」，絕不虛構為「正式企業級託管系統（Custodial Gateway）」。

---

## 二、維度一：HR／技術招聘視角（求職通關與真實性審查）

### 1. 職缺對齊與作品命名
- **專案命名**：保留 **Testnet Wallet Lab**，副標使用 **Go 多鏈交易編排與測試網錢包**。
- **目標職缺級別**：
  - Web3 Backend Engineer
  - Blockchain Integration Engineer
  - Transaction Infrastructure Engineer
- **ATS 關鍵字（基於實際代碼）**：
  `Go (Golang)`, `Multi-Chain (EVM, Solana, TRON)`, `go-ethereum`, `solana-go`, `ERC-4337 (v0.6/v0.7)`, `EIP-1559 Replacement`, `OP Stack Predeploy Gas Oracle`, `Uniswap V3 QuoterV2 / Multicall`, `POSIX Atomic File Replacement`, `Cloudflare Tunnel CI/CD`。

### 2. 破除偏見的面試敘事
- **建議表達**：
  > 「以我的 Web2 後端工程歷練為基礎（具體經歷與架構實績見履歷），我選擇使用 Go 語言自研 Testnet Wallet Lab，基於成熟鏈上函式庫（`go-ethereum`、`solana-go`）自行實作跨 EVM、Solana 與 TRON 的交易流程、狀態管理、廣播前原子快照持久化，以及 ERC-4337 帳戶抽象的打包與 Gas 排查。」

### 3. 可驗證的作品集呈現
1. **線上展示站點**：https://wallet.tedlin.fyi/。
2. **多鏈實證收據矩陣**：各鏈僅標記實際具備之證據，不作全覆蓋過度宣告。
3. **架構說明**：清晰記錄「先保存後廣播（Save-before-broadcast）」之檔案快照流程與設計邊界。

---

## 三、維度二：CTO／技術決策者視角（系統架構、工程品味與取捨邊界）

### 1. 核心流程：Save-before-broadcast 與原子檔案快照
- **代碼查核（internal/wallet/journal.go:453-463）**：
  系統在向 RPC 廣播前，透過 `atomicSave` 將記憶體中的交易陣列完整序列化為 JSON，並利用臨時檔寫入、`tmpFile.Sync()`、`os.Rename` 與父目錄 `d.Sync()` 完成覆蓋替換。
- **架構定性**：
  這是一套具備崩潰一致性（Crash-Consistency）的**檔案原子快照機制**，而非資料庫式的流式 Append-Only WAL。它有效防止了寫入中途斷電導致檔案損毀；保留原始已簽署交易，廣播結果不明時可重送同一筆交易。檔案快照本身不能提供全面的雙花保證。
- **演進路徑**：
  在單機測試錢包情境下，檔案原子快照搭配 `syscall.Flock` 足夠精簡實用；若未來需要水平擴展至多實例，才具備引入 PostgreSQL（如 `SELECT ... FOR UPDATE SKIP LOCKED`）作為分散式 Outbox 的條件。

### 2. 程式碼組織：暫緩 service.go 的過早重構
- **現狀評價**：
  `internal/wallet/service.go` 約 668 行，內部已依職責委派給 `KeystoreManager`、`QuoteStore`、`JournalManager`。
- **決策**：
  目前沒有頻繁修改牽動無關業務的痛點，暫緩拆分 `TransactionManager` 與 `HistoryManager`，避免純粹增加轉呼叫層與鎖協調負擔。

### 3. 診斷與監控
- **現狀**：`internal/web/observe.go:15` 與 `internal/chain/diagnostics.go` 提供 `GET /api/diagnostics`，即時輸出 RPC 請求數、傳輸錯誤數與容錯切換次數。目前暫不需要額外搭建重量級 Prometheus / Grafana 監控平台。

---

## 四、維度三：技術面試官視角（協議細節與邊界防禦審查）

### 1. EVM 協定細節
- **整數運算**：`internal/wallet/decimal.go` 嚴格使用 `*big.Int` 純整數計算代幣數量，無浮點數精度丟失。
- **EIP-1559 加價替換（internal/wallet/replacement.go:70-81）**：
  加速與取消交易時，GasTipCap 與 GasFeeCap 採用無條件進位計算至少 20% 加價（此為專案本地替換策略，如 replacement.go:69 註解所述，並非協議強制標準或節點包含保證；節點交易池替換門檻為可設定參數）。
- **OP Stack 費用模型（internal/chain/opfees.go:14-39）**：
  向 OP Stack 的**預部署合約（Predeploy）** `0x420000000000000000000000000000000000000F`（`GasPriceOracle`）查詢 `getL1FeeUpperBound` 與 `getOperatorFee`，用於**報價階段的費用上界預估**；實際鏈上交易扣費結算則從交易收據（Receipt）解析。
- **Nonce 限制**：`internal/wallet/service.go:323` 維持單在途交易（Single In-Flight）限制，在單機錢包邊界下消除排隊與 Nonce 空洞複雜度。

### 2. 多鏈與協議實作
- **DeFi 兌換（internal/wallet/exchange.go）**：
  對接 Uniswap V3 QuoterV2 鏈上估算，組裝 SwapRouter02 `exactInputSingle` multicall，支援 BPS 滑點保護與廣播前 `RecheckExchange` 再次核對。
- **ERC-4337 帳戶抽象**：
  支援 v0.6 與 v0.7 格式，並在 `docs/erc4337-acceptance.md` 詳實記錄 Sepolia 鏈上對接 Bundler 時排查 `AA95 out of gas` 並調校 Gas 參數之驗收紀錄。
- **TRON 協定封裝（internal/wallet/tron_rpc.go:210-256）**：
  以 Go 標準庫二進位工具手工編碼 Protobuf Wire-Format，組裝 `TransferContract` 與 `TriggerSmartContract` 並綁定 TAPOS 區塊參照，具備完整單元測試驗證編碼一致性。
- **Solana 狀態機制（internal/wallet/solana.go）**：
  基於 `solana-go` 處理 Recent Blockhash 與區塊高度過期判定。

### 3. 智慧合約層（ETHVault）與測試現況
- **合約實作（contracts/ETHVault.sol）**：
  具備使用者餘額獨立帳本（`_balances` 記錄目前可提領餘額）、Check-Effects-Interactions（CEI）模式與重入防護；定義標準事件 `Deposited(address indexed account, uint256 amount)` 與 `Withdrawn(address indexed account, uint256 amount)`。
- **測試邊界（docs/eth-vault.md:43-59）**：
  目前 E2E 測試係串接 Geth 記憶體內 SimulatedBackend 進行存款、提款與超額提領防護驗證；公共 Sepolia 存提操作尚未正式上鏈驗收。
- **Foundry 測試定位（選配）**：
  若進行 Foundry 測試，應注意不變量邏輯：由於合約可能透過 `selfdestruct` 等方式被強制轉入 ETH，驗證餘額守恆時應檢查：
  `合約 ETH 餘額 >= 所有帳戶目前記帳餘額總和`
  而非嚴格等於，且無需在生產合約中額外引入不必要的狀態變數。

---

## 五、定案執行的落地推進路線

遵循務實主義，依序執行具備明確證據支撐的任務：

```
[第一步] 新版 UI 發布與 E2E 驗收
   │
   ▼
[第二步] Sepolia 部署及存提驗收（留存完整操作紀錄）
   │
   ▼
[第三步] README 更新（逐項標示功能、測試方式、鏈上證據、已知限制）
   │
   ▼
[選配] 小範圍 Foundry 存提與守恆測試
```

---

### 第一步：新版 UI 發布與 E2E 驗收
- 檢查並提交本機未提交之 UI 修改。
- 執行前端與瀏覽器端對端測試（`npm run test:e2e --prefix tests/browser`），確保介面契約與交易流程運作正常。
- 確保展示網站 `https://wallet.tedlin.fyi/` 正常載入，介面行為與最新代碼一致。

### 第二步：Sepolia 部署及存提驗收（留存完整結果）
依照 `docs/eth-vault.md` 指引，將 `contracts/ETHVault.sol` 部署至 Ethereum Sepolia 測試網並留存完整紀錄：
1. **部署紀錄**：部署地址、編譯器版本與設定、Sepolia Etherscan 原始碼驗證結果。
2. **存入操作（Deposit）**：交易 Hash、收據 `status=1`、觸發之 `Deposited` 事件內容、操作前後合約存款記帳餘額與錢包餘額（扣除 Gas 耗費）。
3. **提領操作（Withdraw）**：交易 Hash、收據 `status=1`、觸發之 `Withdrawn` 事件內容、操作前後合約存款記帳餘額與錢包餘額。
4. 驗收完成後，方可將文檔中的「待驗收」狀態更新為「已完成驗收」。

### 第三步：README 更新（依據事實逐項標示）
1. **專案定位**：`Testnet Wallet Lab`（Go 多鏈交易編排與測試網錢包）。
2. **架構與取捨說明**：
   - 檔案原子快照替換機制及其單機邊界。
   - EIP-1559 替代加價邏輯。
   - OP Stack 預部署合約（Predeploy）費用預估與鏈上結算差異。
   - 成熟函式庫依賴與 TRON 手工編碼的一致性測試。
3. **鏈上證據矩陣（嚴格對齊實際資料）**：
   - 依據 `internal/web/static/onchain-evidence.json`，逐項標列各網路（Sepolia, Arbitrum, Base, Optimism, TRON）的交易紀錄；缺漏之鏈（如 Solana）如實標註，不作虛構。
   - 補上 Sepolia ETHVault 的部署與存提操作紀錄。
4. **可復現測試指令**：
   - 單元測試、SimulatedBackend 測試與瀏覽器端測試指令。
5. **已知限制（Known Limitations）**：
   - 誠實說明單在途交易限制與單機檔案快照在多實例情境下的演進條件。

### 第四步（選配）：小範圍 Foundry 測試
- 針對 `ETHVault.sol` 撰寫輕量測試，驗證基本存提流程與 `合約 ETH 餘額 >= 所有帳戶目前記帳餘額總和` 餘額守恆條件，作為輔助驗證工具，不阻擋前三項之交付。
