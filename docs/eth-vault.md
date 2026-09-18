# 測試 ETH 存款箱

專案內的 Solidity `ETHVault`，搭配既有 Go 錢包的簡易面板。只支援 Ethereum Sepolia。沒有利息、代幣、管理員代提、多簽或升級機制。

## 功能與限制

- `deposit()` 接收測試 ETH，記在 `msg.sender` 的存款下。
- `withdraw(uint256 amount)` 僅能提領自己的餘額，金額單位為 wei，退回呼叫者。採 Checks-Effects-Interactions 與重入鎖，轉帳失敗會回滾餘額與事件。
- `balanceOf(address)` 查詢存款。`Deposited`／`Withdrawn` 事件提供操作證據。
- 零額存提與超額提領拒絕；普通 ETH 轉帳不入帳，必須呼叫 `deposit()`。不要直接使用錢包的原生 ETH 轉帳功能存入。
- 強制轉入的 ETH 不會增加任何人的存款；合約沒有取回這類額外 ETH 的管理員功能。
- 合約帳面餘額按地址隔離。公開 demo 若共用同一個錢包地址，就共用該地址的存款；需要隔離時使用自己建立的測試錢包。
- 寫入仍需現有錢包密碼。報價與送出前皆做合約檢查及 `eth_call`；模擬成功不保證之後的交易成功。交易沿用報價綁定、落盤後廣播、相同 quote ID 重用簽名交易與恢復流程。
- 活動頁只有在核對成功、canonical 收據後，才將伺服器指定存款箱的 `Withdrawn` 事件列為 ETH 收入。存入用交易 value 計帳並標示 `Deposited`，不重複計算。此功能不提供全鏈內部轉帳索引。

## 本機編譯與測試

Node.js 22、Go 1.26.1。編譯器固定 Solidity 0.8.28，optimizer 開啟、200 runs、EVM Cancun。npm override 將編譯工具的 `tmp` 固定到 0.2.7；不影響 Solidity 程式碼。

```sh
npm ci --ignore-scripts --prefix contracts
npm run compile --prefix contracts
npm run check --prefix contracts
go test ./contracts ./internal/wallet ./internal/web ./internal/chain
npm ci --prefix tests/browser
npm test --prefix tests/browser
```

`contracts/artifacts/` 保留可重現編譯結果。Go 測試核對來源 SHA-256；CI 重新編譯並比較產物，防止 Solidity 修改後仍測舊 bytecode。Go 的 simulated backend 在記憶體執行合約，不使用使用者 keystore 或公開 RPC。測試用 `ReentrancyAttacker` 與 `RejectingReceiver` 不需要部署到公共鏈。

瀏覽器測試使用本機 HTTP fixtures 並攔截外部請求。畫面截圖與測試成功都不代表 Sepolia 已部署或真實存提成功。

## 啟用方式（需要另行部署）

本次實作不自動部署或支出測試 ETH。`SEPOLIA_VAULT_ADDRESS` 預設空字串，面板顯示「存款箱尚未啟用」並停用存提。設定錯誤地址會在啟動時拒絕；不是合約或無法讀取時顯示錯誤，不把未知餘額當成零。

另行獲准上鏈後：

1. 用 Remix 或既有 Solidity 部署工具，編譯 `contracts/ETHVault.sol`，使用上面的固定編譯設定。合約無 constructor 參數。
2. 確認錢包與部署工具使用 **Ethereum Sepolia，chain ID 11155111**，部署並核對收據 `status=1` 與地址上的 bytecode。
3. 在 Sepolia Etherscan 驗證原始碼、compiler、optimizer、EVM 設定；不要部署 `ReentrancyAttacker` 或 `RejectingReceiver`。
4. 將實際地址設定為 `SEPOLIA_VAULT_ADDRESS` 並重啟服務。直接執行 Go 時由程序環境傳入；Compose 會傳入 `.env`／shell 設定的值。不要提交 `.env`、密碼或私鑰。
5. 開啟 Ethereum Sepolia 的「存款箱」，讀取餘額、用少量測試 ETH 存入，再提領。核對成功收據、事件、更新後的存款與錢包餘額（扣除 Gas），留下兩筆交易 hash。

公開 demo 發布／重啟及鏈上部署須分別處理；修改本機設定不代表遠端已啟用。這是測試網學習實作，尚未完成公共 Sepolia 存提驗收，也不是主網安全審計。
