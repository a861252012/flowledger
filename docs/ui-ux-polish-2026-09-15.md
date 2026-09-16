# UI/UX 調整與驗證 — 2026-09-15

## 依據與取捨

使用使用者指定的 [UI UX Pro Max skill](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill)，本次讀取版本為 `15de38fb70bc80ae9276fa7703b48ae861a672e6`，入口為 `.claude/skills/ui-ux-pro-max/SKILL.md`，並閱讀 `references/quick-reference.md` 與 `references/pro-rules.md`。專案是 Go HTML templates、原生 JavaScript 與 CSS，沒有引入 React、Tailwind 或外部字型。

執行 skill 的設計查詢：

- `fintech wallet dashboard minimal --design-system`：配色建議可參考，但 Enterprise Gateway、Contact Sales 等網站行銷結構不適合錢包操作，未採用。
- 以 `banking dashboard --design-system` 收斂：採用 Minimalism & Swiss Style 的資訊層次與明暗主題方向。仍未採用行銷版型、客戶標誌或安全徽章。
- `progressive disclosure forms --domain ux` 與較窄的 `progressive-disclosure --domain ux` 未取得適合直接套用的精準結果；漸進揭露採用 skill 本身的表單規則，而非聲稱檢索結果提供了特定錢包設計。

實際設計以既有功能與使用者需求為準：深色餘額卡、青綠色主要操作、白色／深藍灰色工作區。使用系統字型與一致的 SVG 線條圖示。沒有新增虛構資產總值、價格走勢或交易證據。

## 本輪變更

- 桌面總覽將代幣與近期紀錄並排，縮減帳戶切換區的多餘容器，讓餘額與四個主要操作先被看到。
- 發送、收款、兌換、測試幣與地址簿使用較合適的內容寬度，避免表單橫跨整個大螢幕。
- EVM 收款 QR Code 預設展開；進階交易池與滑價設定改為可展開區塊。
- EVM 交易紀錄突出操作、時間、金額與狀態；地址、收據與診斷入口展開查看。待確認交易的重送與替換操作仍可操作。
- EVM、Solana、TRON 共用字級、留白、SVG 導覽與明暗色彩。新增操作說明均含英文、正體中文與簡體中文。
- 修正手機選單焦點時序：只在切回桌面寬度時收起選單，visibility 不參與進場動畫，避免初始影格尚不可聚焦。
- 保留既有表單、簽署確認、語言／主題偏好與深層連結，沒有改動簽署或廣播後端。

## 色彩檢查

依 sRGB 相對亮度公式計算以下固定色票的對比：

| 文字／背景 | 對比 |
| --- | ---: |
| 淺色主要文字／白色表面 | 14.65:1 |
| 淺色次要文字／頁面背景 | 5.49:1 |
| 淺色主要按鈕文字／背景 | 6.87:1 |
| 暗色主要文字／卡片背景 | 13.94:1 |
| 暗色次要文字／次級表面 | 6.99:1 |
| 暗色主要按鈕文字／背景 | 9.11:1 |
| 餘額卡次要文字／背景 | 9.19:1 |

這是列出色票的檢查，不等同整站 WCAG 認證。

## 驗證方式

`tests/browser/browser.cjs` 使用隨機 loopback 埠的 Mock 伺服器及可丟棄 Chromium context，封鎖其他 origin。涵蓋既有收付款與兌換流程、三語切換、輸入保留、手機選單、Esc、設定持久化及各鏈頁面。

新增 EVM / Solana / TRON × 三語 × 375/768/1024/1440 px × 明暗主題 × 總覽/發送/收款/測試幣，共 288 組頁面水平溢出檢查；另檢查桌面雙欄、進階設定鍵盤開啟及 reduced-motion。

```sh
npm ci --prefix tests/browser
cd tests/browser
npx playwright install chromium
npm test
```

已知獨立問題：目前 repo 尚缺 package-lock.json，因此上述乾淨安裝流程需先補齊 lockfile。本輪以已存在的 bundled Playwright 執行瀏覽器測試，不將其宣稱為專案依賴安裝成功：

```sh
NODE_PATH=/Users/a861252012/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules \
  node tests/browser/browser.cjs
```

設定 `FLOWLEDGER_UI_SCREENSHOTS` 可將 Mock 畫面的 12 張英文桌面／手機、明暗主題截圖輸出至指定目錄；未設定時不產生截圖檔。

Go HTTP/template 檢查使用無網路的隔離容器，原始碼與 module cache 為唯讀，不掛載 wallet_data：

```sh
docker run --rm --network none \
  -v "$PWD:/app:ro" \
  -v flowledger_go_modules:/go/pkg/mod:ro \
  -v /private/tmp/flowledger-development-cache:/tmp/review-cache \
  -e GOCACHE=/tmp/review-cache flowledger-app go test ./internal/web
```

本輪沒有以真實鏈上交易驗證 UI，也沒有修復前次報告中的 Solana 儲存／過期處理、兌換替換追蹤、approve 加速語意或 CI lockfile 問題。交易紀錄的資訊整理不等於上述核心邏輯已修復。

## 本輪結果

- 最後一輪瀏覽器命令 Exit 0；原有互動案例與新增的 288 組版面檢查通過。
- `go test ./internal/web` Exit 0；`node --check` 與 `git diff --check` 通過。
- 已檢視英文桌面明暗模式及手機截圖，並修正截圖中發現的交易列擁擠。
- 重啟 app 服務後，`/healthz`、`/`、`/solana/`、`/tron/` 均回傳 HTTP 200；HTTP 提供的 portal.css、portal.js、wallet.js、messages.js 與目前原始檔逐位元組一致。
- 未 commit 或 push。
