# ETHVault UI 發布交接 — 2026-09-18

這份文件保留在 repository，使用相對連結，不依賴特定 agent 的 `~/.codex/visualizations/` 目錄。原始交接文件不在目前授權工作區內；以下依實際原始碼、Git 狀態及本輪執行紀錄整理。

更新：Gemini review 後於 2026-09-19 補齊 browser E2E 的 `-race`、CI 預期合約地址、send 回應斷言與 HTTP 事件數量；見[最新複核及驗證](verification-vault-2026-09-19.md)。9 月 18 日的封存保留為歷史證據。

## 工作目錄與版本

- Chat On Steroids 工作區：`/flowledger`；repository：`a861252012/testnet-wallet-lab`，產品名稱 Testnet Wallet Lab。
- 接手時分支 `main`、HEAD `66ae38bb4721d15b1993cf6343670883061c305b`。
- 接手時 **19 個檔案已暫存**：原先 18 個修改及 `docs/flowledger-evaluation-report.md`。本輪修改追加在 working tree，保留原 index；不能只 commit 既有 index 就當成包含本輪驗收改善。
- 未提交狀態的 HEAD 只識別基底；本輪待驗收來源的指紋、命令與結果見[本機驗證紀錄](verification-vault-2026-09-18.md)。接手時重新核對，避免其他 agent 的後續變更混入。

```sh
git status --short --branch
git rev-parse HEAD
git diff --stat HEAD
git diff --cached
git diff
```

`git diff HEAD` 涵蓋目前已追蹤檔案的暫存與未暫存變更；新未追蹤文件另依 `git status` 核對。不要用 reset、clean 或重新格式化整個專案來整理別人的變更。

## 已有內容與本輪改善

現有待發布內容包括「智慧合約／ETH 存入與取回」UI、語言文案、手機版、錯誤處理及邊界測試；後端四個業務檔案的既有 diff 為錯誤訊息調整。

本輪補強驗收與交接：

1. 本機 E2E log 明示 simulated EVM。瀏覽器場景 **0.05 / 0.02 / 0.03 ETH**，HTTP 場景 **0.5 / 0.2 / 0.3 ETH**，分別記錄。
2. 捕捉存提各自的 send hash，send 回應即核對 action／金額，再核對 history 的 action、金額、`state=succeeded`、無 `refreshError` 及對應 UI 紀錄。餘額與金額精確比對；提款後存款紀錄也須仍在。
3. Go browser runner 另向 simulated EVM 讀取兩筆成功收據、合約／帳戶／金額一致的事件，以及 `balanceOf=0.03 ETH`，不只接受 Node exit code。
4. 瀏覽器 E2E 的 npm 入口固定使用 `go test -race -count=1`。URL／合約設定不完整時失敗；只接受 Go harness 指定的 loopback 測試環境。對外頁面使用唯讀 `test:live`。
5. `test:live` 可選擇指定 `EXPECTED_VAULT_ADDRESS`，CI 由同名 repository variable 傳入；指定時必須啟用且地址一致。未設定時仍依 API 回報驗證啟用或停用 UI。
6. CI 保存 browser fixtures、simulated EVM E2E 與 live UI log artifact，供同一 run／commit 複核。
7. 實測發現部署替身把含空白的 PATH 直接寫成 shell 程式，導致本機驗收失敗。兩份 Python fixture 改用 `shlex.quote`，並以含空白的暫存目錄持續覆蓋此情境；正式部署腳本不變。

主要檔案：[瀏覽器 E2E](../tests/browser/e2e.cjs)、[Go E2E](../tests/e2e/e2e_test.go)、[線上唯讀檢查](../tests/browser/live.cjs)、[CI](../.github/workflows/verify.yml)、[合約操作文件](eth-vault.md)、[部署文件](deployment.md)。

## 接續順序與完成條件

| 階段 | 完成條件 | 不可由此推論的結果 |
|---|---|---|
| 本機待發布版本驗證 | 記錄基底 commit、程式／建置／測試檔案指紋、命令、工具版本、exit code、skip 情況；測試前後來源一致，文件另查 diff | 新版已在公開站執行 |
| 新版 UI 發布 | 指定 commit 的 CI 必要 jobs 通過、GHCR image 發布、`/healthz` 的 `X-App-Version` 相符，live 桌面／手機檢查通過 | 合約已部署或可存提；停用狀態也可通過 |
| 公共 Sepolia 部署與啟用 | 核對部署收據、bytecode、原始碼驗證；VM 設定實際地址並重啟；以預期地址重跑唯讀 live | 存提交易已成功 |
| 公開 UI 存入與取回 | 獲准的測試錢包透過新版 UI 操作；兩筆交易各有成功收據、匹配事件、前後存款／錢包餘額 | 主網安全稽核或其他鏈全覆蓋 |
| README 更新 | 依各項已具備的證據標記功能、測試範圍與限制 | 沒有證據的功能已完成 |

Foundry 維持選配，不阻擋前述交付；本輪不擴大服務分層、資料庫或監控架構。

## CI/CD 接手檢查

實際流程為 main push → 必要檢查 → 發布 GHCR immutable image → VM 主動輪詢／拉取 → live 驗收。publish 條件包括 repository variable `DEMO_DEPLOY_ENABLED=true`；GHCR package 須可供 VM 拉取。

timer 為前一輪結束後約 120 秒再檢查，另有 CI/build、下載及啟動時間。以確切 commit 的線上版本與 live 結果作準，不因 push 成功或舊版健康而標記發布完成。

發布前審閱 staged 與 unstaged diff，將真正待發布的檔案納入 commit。記錄完整 commit SHA、Actions run、GHCR digest、部署前後 revision 與 `live` 結果。CI 的 browser/live artifact 目前保留 14 天。

目前 index 保留 19 個既有 staged 檔案，本機優化尚未改動暫存安排。發布者審閱兩份 diff 與新增證據後，使用下列明確路徑補齊本次交付，避免只提交舊 index 或用 `git add .` 一併收入後續無關檔案。這段是發布時的操作指引，本輪尚未執行：

```sh
git add -- \
  .github/workflows/verify.yml \
  docs/deployment.md docs/eth-vault.md \
  tests/browser/e2e.cjs tests/browser/live.cjs \
  tests/deployment/test_deploy.py tests/deployment/test_poll.py \
  tests/e2e/e2e_test.go \
  docs/release-handoff-2026-09-18.md \
  docs/verification-vault-2026-09-18.md \
  docs/verification-vault-2026-09-19.md \
  docs/evidence/vault-local-2026-09-18/verification.json \
  docs/evidence/vault-local-2026-09-18/raw-results.zip \
  docs/evidence/vault-review-2026-09-19/verification.json \
  docs/evidence/vault-review-2026-09-19/raw-results.zip
git diff --cached --check
git diff --cached --stat
git diff --exit-code
git ls-files --others --exclude-standard
```

最後兩項在這份交付範圍完整且沒有其他工作時應無剩餘內容；若仍有變更，逐檔釐清，不能直接清除。19 個既有 staged 檔案也須保留在同一份發布審閱中。

授權沿用使用者當前指示；本輪執行的是本機修改、測試及唯讀核對。現有合約文件要求另行獲准上鏈，故公共部署、花費測試 ETH 與遠端設定須依已確認的部署／錢包／金額範圍處理。不要把測試密碼、私鑰或 `.env` 寫入報告或 commit。

## 公共 Sepolia 證據欄位

留存部署 transaction hash、receipt block number/hash、`status=1`、contract address、compiler 完整版本、optimizer／EVM 設定及原始碼驗證結果。保存的地址及 bytecode 必須與啟用的服務相符。

存入與取回各自保存：UI 執行的 release SHA、錢包地址、合約地址、金額（wei 字串及 ETH）、交易 hash、成功收據、`Deposited`／`Withdrawn` 的合約地址／帳戶／金額，與操作前後 `balanceOf`、錢包 ETH 餘額及所用區塊。費用使用 receipt 的 `gasUsed × effectiveGasPrice`（整數 wei）；取回增加的 ETH 與錢包扣除的 Gas 分開核對。

使用專用測試錢包且期間無其他交易時，可核對存入後錢包餘額為原餘額減存入與 Gas，取回後為原餘額加取回再減 Gas。共用錢包若同時被其他人操作，應先釐清額外收支，不能直接把餘額差當作單筆交易證據。執行成功、canonical 收據及 finalized 狀態分別記錄。

公共測試網金額依實際授權及餘額選定，不必複製本機自動測試的金額。不得用本機 simulated EVM 的 hash 填公共 Sepolia 紀錄。

## 給 Gemini 的複核範圍

請針對同一份 working tree 進行唯讀審查，回報具體檔案／行號與可重現反例；不要同時修改或暫存相同檔案。

1. 存提兩筆 hash、action、精確金額、成功狀態及 UI 紀錄是否仍可能互相替代？Go 的獨立 EVM 收據／事件／餘額核對是否涵蓋 browser 的金額？
2. 測試環境選擇及 required browser 行為，是否可能出現改測另一個環境或 skip 後被誤記成功？
3. CI 失敗時 log 是否留存並保有非零 exit code？既有 index、working tree 與測試來源指紋是否一致？
4. 停用 UI、指定地址啟用檢查、公共 Sepolia 存提是否被文件清楚區分？README 有無提前宣稱未完成成果？

交接結論請使用「已讀程式」「實際執行通過」「跳過／阻塞」「尚未驗收」等可核對描述，附原始結果位置；不以 agent 名稱或口頭宣稱替代測試證據。
