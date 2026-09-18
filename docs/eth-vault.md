# ETH 存入與取回

在側欄開啟「智慧合約」，就能使用「ETH 存入與取回」：把測試 ETH 存入專案的 Solidity `ETHVault` 合約，再取回目前錢包。只支援 Ethereum Sepolia，沒有利息或鎖定期，也不提供代幣、管理員代提、多簽或升級功能。

## 操作方式

1. 選好要使用的錢包，確認網路為 Ethereum Sepolia，開啟側欄「智慧合約」。
2. 核對「目前錢包餘額」與「此錢包的合約餘額」。存入與取回都要支付手續費（Gas），請在錢包留一些測試 ETH；需要時可按「需要測試 ETH？前往領取」。
3. 選擇「存入合約」或「取回錢包」，確認畫面上的資金流向，再輸入 ETH 金額。取回只能使用目前錢包的合約餘額，款項會回到同一個錢包地址。
4. 按「預估費用並核對」，查看操作、錢包、合約地址、金額與費用上限。取回時，「最多扣除」只計算錢包支付的 Gas 上限，取回的 ETH 會另外回到錢包。這一步只做預覽，還不會送出交易。
5. 確認內容後，輸入錢包密碼並按「簽署並送出至 Ethereum Sepolia」。報價過期時須重新預估。
6. 查看交易紀錄與鏈上收據，再按「更新餘額與紀錄」核對兩邊餘額。取得交易 hash 代表已取得廣播追蹤資料；仍須確認收據 `status=1`，才能判定鏈上執行成功。

共用展示錢包的合約餘額可能隨其他人的操作變動。畫面會提醒這是共用錢包；需要獨立餘額時，請使用自己建立的測試錢包。公開唯讀展示需登入後，才能查看錢包的合約餘額與操作。

## 功能與限制

- `deposit()` 接收測試 ETH，記在 `msg.sender` 的存款下。
- `withdraw(uint256 amount)` 僅能提領自己的餘額，金額單位為 wei，退回呼叫者。採 Checks-Effects-Interactions 與重入鎖，轉帳失敗會回滾餘額與事件。
- `balanceOf(address)` 查詢存款。`Deposited`／`Withdrawn` 事件提供操作證據。
- 零額存提與超額提領拒絕；普通 ETH 轉帳不入帳，必須呼叫 `deposit()`。不要直接使用錢包的原生 ETH 轉帳功能存入。
- 強制轉入的 ETH 不會增加任何人的存款；合約沒有取回這類額外 ETH 的管理員功能。
- 合約帳面餘額按地址隔離。公開 demo 若共用同一個錢包地址，就共用該地址的存款；需要隔離時使用自己建立的測試錢包。
- 寫入仍需現有錢包密碼。報價與送出前皆做合約檢查及 `eth_call`；模擬成功不保證之後的交易成功。交易沿用報價綁定、落盤後廣播、相同 quote ID 重用簽名交易與恢復流程。
- 活動頁只有在核對成功、canonical 收據後，才將伺服器指定合約的 `Withdrawn` 事件列為 ETH 收入。存入用交易 value 計帳並標示 `Deposited`，不重複計算。此功能不提供全鏈內部轉帳索引。

## 本機編譯與測試

CI 的瀏覽器 job 使用 Node.js 22，Go 版本依 `go.mod`；容器版本固定於 Dockerfile。本機實際工具版本須隨驗收紀錄保存。編譯器固定 Solidity 0.8.37，optimizer 開啟、200 runs、EVM Cancun。npm override 將編譯工具的 `tmp` 固定到 0.2.7；不影響 Solidity 程式碼。

```sh
npm ci --ignore-scripts --prefix contracts
npm run check --prefix contracts
npm ci --ignore-scripts --prefix tests/browser
(cd tests/browser && npx playwright install chromium)
go test -race -count=1 ./...
go vet ./...
npm test --prefix tests/browser
npm run test:e2e --prefix tests/browser
```

`contracts/artifacts/` 保留可重現編譯結果。Go 測試核對來源 SHA-256；CI 重新編譯並比較產物，防止 Solidity 修改後仍測舊 bytecode。Go 的 simulated backend 在記憶體執行合約，不使用使用者 keystore 或公開 RPC。測試用 `ReentrancyAttacker` 與 `RejectingReceiver` 不需要部署到公共鏈。

驗收先執行 `check`；只有刻意修改 Solidity 來源後，才執行 `npm run compile --prefix contracts` 並審閱 artifact diff。不要在驗收前先覆寫產物而掩蓋來源與 bytecode 不一致。

| 測試入口 | 執行範圍 | 存入／取回／剩餘 ETH |
|---|---|---|
| `npm test --prefix tests/browser` | Chromium + 本機 HTTP fixtures，檢查 UI、語言、手機版及錯誤狀態 | fixture 數值，不是合約存提證據 |
| `TestE2EVaultFullLifecycle`（`tests/e2e/e2e_test.go`） | HTTP client + Go server + simulated EVM，核對收據、事件、餘額及兩筆交易各自的 history | **0.5 / 0.2 / 0.3** |
| `npm run test:e2e --prefix tests/browser` | Chromium + Go server + simulated EVM（`-race`）；送出回應即核對 action／金額，再逐筆核對 hash、history 與 UI；Go runner 另查收據、事件與 `balanceOf` | **0.05 / 0.02 / 0.03** |
| `npm run test:live --prefix tests/browser` | 公開站版本、UI 與設定的唯讀檢查 | 不送出存提交易 |

一般 `go test` 在缺 Node／Playwright 或 short mode 時，可能跳過瀏覽器案例；因此發布驗收必須另有 `npm run test:e2e` 成功紀錄。該入口固定使用 `go test -race -count=1` 並設定 `RUN_BROWSER_E2E=1`，缺少依賴或 short mode 會失敗；race detector 需啟用 CGO，非 Darwin 系統還需 C compiler。CI 的 Go job 保留離線容器測試，browser job 透過此入口執行帶 race detector 的瀏覽器 E2E。瀏覽器子程序的 `E2E_BASE_URL`、`E2E_VAULT`、`E2E_BACKEND=simulated` 由 Go harness 提供；不接受不成對設定或非 loopback 伺服器。公共展示站使用 `test:live` 檢查。

本機 simulated EVM 使用 Sepolia 的 chain ID；相同 chain ID、合約地址格式或交易 hash 都不構成公共 Sepolia 證據。以上本機測試也不代表新版 Demo 已發布。交接及分階段完成條件見[發布交接](release-handoff-2026-09-18.md)。

## 啟用方式（需要另行部署）

本次實作不自動部署或支出測試 ETH。`SEPOLIA_VAULT_ADDRESS` 預設空字串，面板顯示「此環境尚未開放合約操作」及「合約尚未設定，目前無法存入或取回 ETH。」，隱藏餘額、操作表單與紀錄。設定錯誤地址會在啟動時拒絕；不是合約或無法讀取時顯示錯誤，不把未知餘額當成零。

另行獲准上鏈後：

1. 用 Remix 或既有 Solidity 部署工具，編譯 `contracts/ETHVault.sol`，使用上面的固定編譯設定。合約無 constructor 參數。
2. 確認錢包與部署工具使用 **Ethereum Sepolia，chain ID 11155111**，部署並核對收據 `status=1` 與地址上的 bytecode。
3. 在 Sepolia Etherscan 驗證原始碼、compiler、optimizer、EVM 設定；不要部署 `ReentrancyAttacker` 或 `RejectingReceiver`。
4. 將實際地址設定為 `SEPOLIA_VAULT_ADDRESS` 並重啟服務。直接執行 Go 時由程序環境傳入；Compose 會傳入 `.env`／shell 設定的值。不要提交 `.env`、密碼或私鑰。
5. 開啟 Ethereum Sepolia 的「智慧合約」，依上面的操作流程讀取餘額、用少量測試 ETH 存入，再取回錢包。核對成功收據、事件、更新後的存款與錢包餘額（扣除 Gas），留下兩筆交易 hash。

公開 demo 發布／重啟及鏈上部署須分別處理；修改本機設定不代表遠端已啟用。這是測試網學習實作，尚未完成公共 Sepolia 存提驗收，也不是主網安全審計。
