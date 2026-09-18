# ETHVault 本機驗證紀錄 — 2026-09-18

本文件保留 9 月 18 日的歷史結果。Gemini review 後的 CI／斷言補強與新來源指紋，見 [2026-09-19 複核及驗證](verification-vault-2026-09-19.md)；下方 569 個 pass 與封存不能當成後續修改已重跑全套的證據。

本輪實際執行時間為 2026-09-18 **21:23–21:30（Asia/Taipei）**，即 **13:23–13:30 UTC**。範圍為待發布 working tree 的本機驗證，尚未 commit、push 或發布這份修改；公共 Sepolia ETHVault 部署與存提仍待驗收。

## 可追溯的版本與原始結果

基底 commit：`66ae38bb4721d15b1993cf6343670883061c305b`。本輪保留接手時的 19 個 staged 檔案，改善內容另留在 working tree；HEAD 本身不包含這些尚未提交的內容。

| 階段 | 程式／建置／測試檔案 SHA-256 | 結果 |
|---|---|---|
| 初次執行 | `a76791f28d4f523d445987d5e98dbe165f9009972d097badf0d4aef89e3f348a` | Go、合約、瀏覽器通過；部署 fixture 因 PATH 空白引用錯誤失敗 |
| 修正部署 fixture 後 | `3a75df0852e8f14454e909f47f39c9092c35ac7029be1e529483270974cf1195` | 部署 13 項、workflow lint、錯誤環境拒絕、本機映像 smoke 通過 |

兩次 manifest 的差異僅有 `tests/deployment/test_deploy.py`、`tests/deployment/test_poll.py`；Go、JavaScript、Solidity、UI 及 CI 檔案未變。因此沿用初次成功結果，只針對受影響的部署 fixture 及剩餘項目驗證。各次執行前後來源一致，映像驗證完成後亦重新核對最終指紋。

指紋涵蓋 176 個 tracked／non-ignored 程式、設定、建置與測試檔案：取 `git ls-files --cached --others --exclude-standard`，排除 `docs/`、`README*`、`test-results/` 及 `.md`；逐檔 SHA-256 後，以檔名排序的 JSON（`sort_keys=True`、`separators=(',', ':')`）計算整體 SHA-256。這個值不是 Git tree／commit SHA；文件與證據另以 diff 審閱。

[完整執行索引與命令](evidence/vault-local-2026-09-18/verification.json)保留三個階段的時間、exit code、skip 情況、工具版本、image ID 與原始檔案雜湊。[原始結果封存](evidence/vault-local-2026-09-18/raw-results.zip)共 26 個檔案，包含未修改的 log、各次結果 JSON 與來源 manifest；初次失敗紀錄也保留在內。封存 SHA-256：

```text
09f9dad2d4939b50047141ad2b13da72034eece43356a3f25eab5a1e278d8b28
```

原始本機目錄為 `test-results/release-review-2026-09-18/`，受 `.gitignore` 排除；上面的 `docs/evidence/` 封存則可隨本次文件一起提交，讓其他 agent 不依賴同一個暫存目錄。

## 實測環境

| 工具 | 本輪使用版本 |
|---|---|
| Go | `go1.26.1 darwin/arm64` |
| Node.js / npm | `v24.2.0` / `11.3.0`；CI browser job 使用 Node 22，本輪未在 Node 22 重跑 |
| Playwright | `1.58.2`，本機已安裝 Chromium |
| Solidity | `0.8.37+commit.f401782d.Emscripten.clang`，optimizer 200 runs，EVM Cancun |
| Docker | `29.4.0`，Linux ARM64 engine |

npm 的全域 `allow-scripts` 設定產生非致命警告，原始 log 保留此訊息。兩個 npm 專案均以 lockfile 執行 `npm ci --ignore-scripts`；合約驗收先執行 `check`，沒有先覆寫 artifact。

## 已完成的驗證

| 指令／項目 | 結果與範圍 |
|---|---|
| `npm run check --prefix contracts` | Exit 0，重新編譯並核對提交的 Solidity artifacts |
| `npm run test:e2e --prefix tests/browser` | Exit 0，Chromium → Go → simulated EVM；存入 0.05、取回 0.02、剩餘 0.03 ETH |
| `npm test --prefix tests/browser` | Exit 0，HTTP fixtures；三種語言、亮／暗色、375/768/1024/1440 px、原有多鏈與錯誤流程 |
| `RUN_BROWSER_E2E=1 go test -race -count=1 -json ./...` | Exit 0，**569 個測試／子測試 pass，5 個 opt-in 外部鏈案例 skip，0 個 fail**；browser 案例有執行 |
| `go vet ./...` | Exit 0 |
| `gofmt -l cmd internal contracts tests` | Exit 0、無輸出 |
| `go fix -diff ./...` | Exit 0、無差異 |
| `python3 -m unittest discover -s tests/deployment -p 'test_*.py'` | 初次失敗；引用修正後 **13 項通過**，Exit 0 |
| `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/verify.yml` | Exit 0；檢查 workflow 語法，並非 GitHub Actions 遠端執行 |
| E2E 錯誤環境四例 | 缺 URL、缺合約、非 loopback URL、缺 simulated 標記，均以預期原因 Exit 1，驗收結果為通過 |
| `git diff --check HEAD` | Exit 0 |
| 本機 `Dockerfile.deploy` build + `tests/deployment/smoke.py` | Exit 0，Linux ARM64、隔離網路、一次性測試 volume；驗證 non-root/read-only、登入／CSRF、密碼保護、容器替換保存錢包及舊 session 拒絕 |

HTTP 整合案例 `TestE2EVaultFullLifecycle` 使用 **0.5 / 0.2 / 0.3 ETH**，與瀏覽器案例的金額不同，兩者都在本機 simulated EVM 上執行。瀏覽器送出後，Go runner 另外依各自 hash 讀取 EVM 成功收據、驗證 `Deposited`／`Withdrawn` 的合約／帳戶／金額，並直接核對 `balanceOf=0.03 ETH`。

本機映像：`testnet-wallet-lab:review-20260918-3a75df08`，`APP_VERSION=local-review-3a75df0852e8`。Image ID：`sha256:a81bb355a143329478b1baf337579245daa435c7c4163b8a7bb065cee6d53588`。此為本機 image ID，並非已發布的 GHCR digest。

## 初次失敗與修正

部署／輪詢替身直接把本機 PATH 拼進 shell，遇到 `Chat On Steroids.app` 的空白後，shell 把後半段誤當 export 參數，導致 6 failures、5 errors。兩份 fixture 改用 `shlex.quote` 處理 base、PATH 與替身程式路徑；暫存目錄名稱刻意含空白，以持續覆蓋此情境。修正後同一組 13 項測試全部通過，正式部署腳本未修改。

原始失敗為封存根目錄的 `deployment-unit.log`，修正後結果為 `followup/deployment-unit.log`；不要只引用初次整體 recorder 的 Exit 1，也不要刪去它而宣稱從未失敗。

## 跳過、未執行與接續事項

以下五項依既有 opt-in 規則跳過，不算公共鏈驗收成功：

```text
internal/chain  TestSepoliaActivityReadOnly
internal/chain  TestAdditionalNetworksReadOnly
internal/wallet TestSepoliaExchangeReadOnly
internal/wallet TestSolanaDevnetSendAcceptance
internal/wallet TestTronShastaReadOnly
```

本輪尚未執行這份修改的遠端 GitHub Actions、Node 22 瀏覽器流程、Linux AMD64 smoke、`govulncheck`、新版公開站 `test:live`、公共 Sepolia ETHVault 部署／原始碼驗證／存入／取回。這些結果不得由本機通過推論。`EXPECTED_VAULT_ADDRESS` 已新增且完成靜態複核，但指定真實地址的 live 驗收仍須等合約啟用後執行。

獨立 reviewer 做了唯讀程式審查，指出成功紀錄可能互相替代、金額 substring 假陽性等問題，已納入本輪修改並執行上述測試；reviewer 的靜態判斷沒有當成額外測試通過次數。

下一步依[發布交接](release-handoff-2026-09-18.md)審閱全部 staged／unstaged／新增文件，建立確切發布 commit，再依既有 CI/CD 完成 UI 發布與分階段公共 Sepolia 驗收。README 的公共存提狀態目前維持待驗收。
