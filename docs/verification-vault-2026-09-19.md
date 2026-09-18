# ETHVault：Gemini review 回應與驗證 — 2026-09-19

本輪在 **2026-09-19 00:10–00:11（Asia/Taipei）** 執行受影響的測試，對應 **2026-09-18 16:10–16:11 UTC**。基底仍為 `main`／`66ae38bb4721d15b1993cf6343670883061c305b`。本輪完成本機修改與驗證，未 stage、commit、push、修改遠端 repository variable、部署或送出公共鏈交易。

## Review 的處理結果

| 建議 | 處理與範圍 |
|---|---|
| browser E2E 加上 `-race` | 已採用。`npm run test:e2e` 的 Go 子程序固定使用 `go test -race -count=1`，保留 `RUN_BROWSER_E2E=1`。原 Go job 已有 race；缺口限於 browser 場景原本的 CI 入口，不能描述為整個 CI 都沒有 race。 |
| workflow 傳入 `EXPECTED_VAULT_ADDRESS` | 已採用，來源為 `${{ vars.EXPECTED_VAULT_ADDRESS }}`。未設定時為空字串；原本的 live script 已有 enabled／disabled 兩分支，新增的是預期地址約束的 CI 連接。 |
| `confirmSend` 驗證 action／amount | 已採用，存入與取回呼叫各自傳入 action 與精確 ETH 字串，send response 即驗證。history、UI 與獨立 EVM 收據仍各自核對，不把送出回應當成收據成功。 |
| HTTP lifecycle 的 event 數量 | 已採用，存入與取回的成功 receipt 各要求一筆 log，再保留合約、topics、帳戶、金額核對。這是目前 EOA → ETHVault 的單筆事件契約，不是所有合約交易通用規則。 |
| 改用 `process.exit(1)` | 未採用。`live.cjs` 已有 `finally` 關閉 browser，再設定 `process.exitCode=1`。本次三種錯誤情境均正常以 1 退出，未被 timeout 終止；沒有重現該情境的程序懸掛。強制退出可能截斷尚未寫完的 stdout／stderr。 |
| 補齊 staged／unstaged | 這是發布前必要的提交完整性作業。保留現有 19 個 staged 檔案，交接提供明確 `git add -- <paths>` 清單及剩餘差異檢查。本輪沒有將 Gemini 的建議視為直接 push main 的授權。 |

GitHub 的 [vars context](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#vars-context) 說明未設定的變數回傳空字串；因此此處不需要額外 `|| ''`。地址只控制 CI 驗收預期，VM 的 `SEPOLIA_VAULT_ADDRESS` 仍須另行設定與啟用。

Go 的 [race detector 文件](https://go.dev/doc/articles/race_detector) 說明 `-race` 只檢查實際執行的路徑，需啟用 CGO，非 Darwin 平台還需 C compiler。Node 的 [process 文件](https://nodejs.org/api/process.html) 說明 `exitCode` 的自然退出行為，以及 `process.exit()` 強制退出有截斷未完成輸出的風險。本輪沒有聲稱有限測試能證明永不懸掛或沒有任何競態。

## 版本與證據

| 階段 | 程式／建置／測試來源 SHA-256 |
|---|---|
| 本輪開始，對應前輪封存的最終版本 | `3a75df0852e8f14454e909f47f39c9092c35ac7029be1e529483270974cf1195` |
| 本輪修正後，測試前後及封存時相同 | `62073e35ea47dfd619eaec13779cb3bb1606e2a9c3ba2c319e649da67f75765f` |

指紋演算法沿用 [9 月 18 日紀錄](verification-vault-2026-09-18.md)：涵蓋 176 個程式、設定、建置及測試檔案，排除 docs、Markdown、README、ignored test-results。與前輪最終版本相比，程式來源差異僅有：

```text
.github/workflows/verify.yml
tests/browser/e2e.cjs
tests/e2e/e2e_test.go
```

文件另以 diff 審閱；`tests/browser/live.cjs` 本輪未修改。`git ls-files --stage -z` 的 SHA-256 在本輪前後同為 `caceea8c85328bc42c3680c71fb605d1987178651d136cbab10131acf9c27657`，確認既有暫存內容未變。

[執行索引](evidence/vault-review-2026-09-19/verification.json)保留原始命令、exit code、工具版本、逐檔指紋及六個 live 情境結果。[原始結果封存](evidence/vault-review-2026-09-19/raw-results.zip)含 18 份檔案，包含 log、manifest 與本次一次性 live fixture script。封存 SHA-256：

```text
55b5f9b3a13118c68420c350dc10e430601549454e2e7efb82963693918f3fa0
```

前輪原始封存的 SHA-256 仍為 `09f9dad2d4939b50047141ad2b13da72034eece43356a3f25eab5a1e278d8b28`；沒有覆寫舊結果。

## 本輪實跑

本機使用 Go `1.26.1 darwin/arm64`、Node `24.2.0`、npm `11.3.0`、Playwright `1.58.2` 及已安裝的 Chromium。

| 指令／驗證 | 結果 |
|---|---|
| `npm run test:e2e --prefix tests/browser` | Exit 0；透過新的 `-race` 入口執行 browser 案例，核對 0.05／0.02／0.03 ETH、兩筆 send response、history／UI、獨立收據／事件／合約餘額。 |
| `go test -race -count=1 -v -run '^TestE2EVault(FullLifecycle\|RevertDataPropagation)$' ./tests/e2e` | Exit 0，兩個 HTTP／revert 案例 pass，無 skip。包含新增的一筆事件數量要求。 |
| `gofmt -l cmd internal contracts tests` | Exit 0，無輸出。 |
| `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/verify.yml` | Exit 0；workflow 靜態檢查，不是遠端執行。 |
| `git diff --check HEAD` | Exit 0。 |
| `node test-results/gemini-review-2026-09-19/live-fixtures.cjs` | Exit 0；執行未修改的 `tests/browser/live.cjs`，使用真實 Chromium、專案 UI templates 與本機 fixture API，六個情境符合預期。 |

### live 的六個情境

| 預期地址設定 | API 狀態 | 預期／實際 exit | 觀察 |
|---|---|---|---|
| 空字串 | disabled | 0 / 0 | 停用 UI 與完整後續導覽通過。 |
| 空字串 | enabled | 0 / 0 | 啟用 UI 與完整後續導覽通過，反證「缺省永遠只能驗停用」的說法。 |
| 指定匹配地址 | enabled | 0 / 0 | 合約地址一致，完整後續導覽通過。 |
| 指定地址 | disabled | 1 / 1 | 以「預期合約必須啟用」原因失敗。 |
| 指定不同地址 | enabled | 1 / 1 | 以「服務必須使用預期地址」原因失敗。 |
| 非法地址字串 | enabled | 1 / 1 | 地址驗證失敗，HTTP 請求數為 0。 |

六個子程序均未被 timeout kill。上述兩個在 browser 啟動後的失敗情境，經現有清理流程正常退出；全部情境沒有提交狀態變更請求。此為本機 fixture 證據，不代表新版公開站部署或公共 Sepolia 存提成功。

## 尚待交付的範圍

本輪只重跑受影響測試，未重跑 569 個全套測試，也未重做前輪 ARM64 image smoke。這些舊紀錄仍屬 9 月 18 日；產品 Go／JS 與 Solidity 檔案本輪未改。

尚未執行這份修改的遠端 GitHub Actions、Node 22 瀏覽器流程、Linux AMD64 smoke、govulncheck、新版公開站 `test:live`、公共 Sepolia ETHVault 部署／原始碼驗證／存入／取回。真實合約地址的設定及鏈上證據仍待部署階段處理，不能由 fixture 成功推論。

下一步依[發布交接](release-handoff-2026-09-18.md#cicd-接手檢查)審閱所有 staged、unstaged 與新增證據，補齊同一份發布版本，再執行已授權的發布流程。README 的公共存提狀態保持待驗收。
