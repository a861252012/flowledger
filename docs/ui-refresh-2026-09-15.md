# FlowLedger UI 改版驗證（2026-09-15）

本輪依使用者指定的本機參考頁 `http://fracted-web3.localhost:18085/dashboard`，實際檢視側欄、摘要卡、快捷入口與深色配色後調整 FlowLedger。參考網站未被修改。

## 交付

- 首頁集中顯示目前帳戶、原生幣餘額、常用操作、代幣與最多三筆近期發送紀錄。
- 發送、收款、兌換、領幣、交易紀錄、收支流水、地址簿、設定與備份改為 hash 導覽工作區。保留原有書籤與表單 ID，瀏覽器上一頁／下一頁仍可導覽。
- 網路／交易診斷、唯讀查詢、操作指南移入進階工具。代幣合約地址移入詳情；手動收支掃描設定預設收合。
- English、简体中文、繁體中文（台灣）可即時切換。範圍包含 EVM、Solana、TRON、作品證據頁、確認視窗、既有前端訊息及已知後端錯誤。
- 深色／淺色按鈕，初次依系統偏好；語言與主題存於此網站的 localStorage，跨網路頁面沿用。儲存不可用時仍可在當頁切換。
- 手機改為抽屜選單，可點關閉、背景遮罩或按 Escape 關閉；開啟時限制背景操作及鍵盤焦點。

## 實作位置

- `internal/web/templates/{index,solana,tron,showcase}.html`：資訊層級、入口、偏好控制。
- `internal/web/static/portal.css`：深淺色變數、摘要卡、側欄及手機版。
- `internal/web/static/portal.js`：工作區導覽、手機選單、偏好按鈕。
- `internal/web/static/preferences.js`：樣式載入前讀取主題，避免先閃過另一種配色。
- `internal/web/static/messages.js`：明確的英文及簡體中文訊息表；正體中文為來源文案。
- `internal/web/static/i18n.js`：已知訊息及參數化文案的顯示層轉換，保留原文以便切回，不重載頁面。

語系不改變 API 欄位、交易狀態值、金額、地址、交易雜湊、合約識別或簽署內容。輸入欄位、助記詞、原始簽署資料不交給文案轉換處理；地址簿自訂名稱及自訂代幣名稱另標記 `translate="no"`。未列入訊息表的外部錯誤保留原文，不以不精確的通用翻譯遮蔽資訊。新增 UI 文案時應同步擴充訊息表。

## 驗證

### 隔離瀏覽器回歸

```sh
NODE_PATH=/Users/a861252012/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules \
  node tests/browser/browser.cjs
```

測試使用臨時 Chromium context 與隨機埠的 fixture HTTP server；所有跨來源請求封鎖，API 全部使用 mock。未存取 runtime wallet volume，未發送真實鏈上交易。

最終 Exit Code 0，輸出：

```text
PASS: task-based navigation, en/zh-CN/zh-TW live switching, draft/password preservation, theme persistence, mobile menu/escape, and cross-chain preferences.
PASS: five EVM networks including POL, contacts, observation, RPC failures, pool selection, both exchange workflows, lost-response reload recovery, history search, diagnostics, Solana and TRON create/quote/send/finalized UI, TRC20 query, mobile layout (mock APIs only).
```

新增斷言涵蓋：

- 頁面切換後，無關表單與診斷不再出現在首頁。
- 即時切換語言保留地址、數量與確認視窗內的測試密碼。
- 主題重新整理後保留；語言跨 Solana／TRON 頁面保留。
- 英文下逐一開啟 EVM 各工作區、Solana／TRON 功能頁與進階說明，檢查未翻譯中文；作品頁的鏈上證據與限制亦檢查。
- 390px 手機版無水平溢出，選單開啟時焦點進入側欄，Escape 與選擇頁面可關閉。
- 地址簿名稱即使與 UI 文案相同（例如「總覽」），切換語言後仍保持原文。
- 字典的物件原型名稱不會被當成訊息翻譯。

最初瀏覽器啟動受 macOS sandbox 的 MachPort 權限阻擋；獲准啟動獨立測試瀏覽器後執行。測試曾發現手機導航斷言未等待 hashchange，以及 Solana／TRON 終局確認文案漏翻；已修正並重跑通過。

### Go Web 模板與路由

```sh
docker run --rm --network none \
  -v "$PWD:/app:ro" \
  -v flowledger_go_modules:/go/pkg/mod:ro \
  -v /private/tmp/flowledger-development-cache:/tmp/review-cache \
  -e GOCACHE=/tmp/review-cache flowledger-app \
  sh -c 'go test -race -count=1 ./internal/web/... && go vet ./internal/web/...'
```

最終 Exit Code 0；`internal/web` 通過（1.071s），`go vet` 無輸出。原始碼與模組唯讀、無外網、不掛載錢包資料；驗證 Go 內嵌頁面與 Web 路由。另執行 JavaScript 語法檢查與 `git diff --check`。

### 本機實際畫面

已在 `http://localhost:8090/` 檢視改版後的實際餘額頁、English 淺色模式、正體中文深色模式與 390px 簡體中文手機版。這部分只切換 UI、偏好設定與讀取資料，沒有送出轉帳。

## 邊界

本輪為介面與顯示層改版，沒有增加新的鏈或改變交易簽署邏輯。既有功能的真實上鏈驗收請見原有 on-chain 與 faucet 報告；mock UI 測試不等於再次完成真實交易驗收。本輪未 commit 或 push。
