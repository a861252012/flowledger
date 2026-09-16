# FlowLedger

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

**以 Go 打造的測試網錢包，探索交易可靠性與故障復原。**

FlowLedger 涵蓋費用預覽、本機簽署、持久化、廣播、收據核對與最終確定性觀測。專案著重後端常見的交易難題：RPC 回應遺失、並行重試、程序中止與鏈重組。

[三分鐘展示](docs/demo-script.md) · [鏈上驗收紀錄](docs/onchain-acceptance-2026-09-15.md) · [故障復原驗證](docs/process-recovery.md)

## 工程重點

- **先保存，再廣播**：先寫入簽署後的交易位元組與雜湊；復原時沿用同一筆已簽署交易。
- **重試去重**：同一帳戶、網路與 quote ID 沿用既有交易日誌。不同報價仍須由上游付款服務以業務識別碼防止重複付款。
- **保留較新的觀測**：交易日誌更新會檢查版本，避免慢請求覆蓋較新的收據或鏈重組狀態。
- **分開判讀交易狀態**：廣播、上鏈、執行成功及最終確定性分別核對；RPC 逾時不代表交易失敗。
- **精確金額**：採整數運算、明確費用預覽及有限額代幣授權，讓簽署內容可供確認。

## 支援範圍

| 網路 | 錢包功能 | 驗收界線 |
| --- | --- | --- |
| Ethereum Sepolia | ETH／ERC-20、有限額授權、WETH 包裝／解包、Uniswap V3 WETH／測試 USDC 兌換 | 納入 9 月 15 日驗收紀錄 |
| Arbitrum／Base／OP Sepolia | 原生幣轉帳、ERC-20、授權與替換交易 | 紀錄包含原生幣轉帳 |
| Polygon Amoy | POL 與 EVM 代幣操作 | 該紀錄中的發送驗收尚未完成 |
| Solana Devnet | 獨立 SOL 錢包 | 該紀錄中的發送驗收尚未完成 |
| TRON Shasta | 獨立 TRX／TRC-20 錢包 | 紀錄包含 TRX 與測試代幣轉帳 |

[2026-09-15 驗收紀錄](docs/onchain-acceptance-2026-09-15.md)記載**五個測試網共 13 筆成功發送交易**。這是歷史快照，不代表所有功能或目前網路狀態均已驗證；Mock 測試另屬不同證據。

[ERC-4337 元件](internal/wallet/erc4337)包含 UserOperation 編碼、雜湊、建構、簽署、Bundler 用戶端及 Mock 測試。目前**尚未整合至錢包介面**，也未驗證已部署的智慧帳戶或真實 Bundler 驗收；詳見[模組範圍](PROJECT.md)。

## 本機啟動

先安裝 Docker 與 Docker Compose，再於專案根目錄執行。只有尚無 `.env` 時才建立，已有設定請保留。

```sh
cp .env.example .env
docker compose build
docker compose run --rm --no-deps app go mod download
docker compose up -d
```

開啟 [localhost:8090](http://localhost:8090)，作品展示頁為 `/showcase`。預設本機展示不需登入；將 `WALLET_ACCESS_TOKEN` 設為至少 32 個字元可啟用登入。連接埠請維持綁定 `127.0.0.1`，不要透過通道或公開代理對外開放錢包。

1. 建立或還原測試專用錢包，離線妥善備份助記詞。
2. 選擇正確測試網並取得測試資產。選用的本機發幣帳戶需要設定與餘額；複製專案不會附帶資金或憑證。
3. 預覽收款地址、資產、金額及費用，再輸入錢包密碼確認簽署與廣播。
4. 分別核對收據與最終確定性。廣播結果不明時，重新廣播原始已簽署交易。

修改原始碼或內嵌介面後執行 `docker compose restart app`；修改 Compose 環境設定後執行 `docker compose up -d --force-recreate app`。

## 資料保存與安全界線

**僅供測試網原型使用，請勿匯入持有真實資產的錢包。** Go 程序會在簽署時處理解密後的金鑰；本專案不是經安全稽核的託管服務或公開多使用者錢包。

- 保存完整 `wallet_data` volume，包含交易日誌與封存檔。只還原助記詞無法恢復交易紀錄或待處理交易的保護機制。
- `docker compose down` 保留 volume；**`docker compose down -v` 會刪除錢包資料。**
- 收據及最終確定性核對依賴設定的 RPC 供應商，尚未實作獨立 RPC 多方共識驗證。
- 收支流水及 CSV 僅涵蓋已索引並驗證的交易，不是完整帳本、所有內部轉帳或獨立餘額對帳。
- 目前展示範圍不包含主網、正式營運就緒保證、SLA 或獨立安全稽核。

## 驗證

```sh
docker compose run --rm --no-deps app go test -race ./...
docker compose run --rm --no-deps app go vet ./...
npm ci --prefix tests/browser
cd tests/browser && npx playwright install chromium && npm test
```

以上指令執行 Go 與瀏覽器檢查，不代表真實鏈驗收。[CI 設定](.github/workflows/verify.yml)包含隔離的 Go race／vet／coverage 與瀏覽器工作；實際執行結果請查看 [Actions](https://github.com/a861252012/flowledger/actions)。

透過公開測試網 RPC 重新核對已發布的交易證據，不簽署也不廣播：

```sh
go run ./cmd/verify-onchain-evidence
```

## 專案結構與文件

| 路徑 | 用途 |
| --- | --- |
| `cmd/flowledger` | 啟動與設定 |
| `internal/wallet` | 金鑰、報價、簽署及交易日誌 |
| `internal/chain` | RPC 存取與收據驗證 |
| `internal/web` | HTTP 防護及內嵌 HTML／CSS／JavaScript |
| `internal/wallet/erc4337` | 獨立帳戶抽象化元件 |
| `tests/browser` | 使用 Mock API 的瀏覽器測試 |

[詳細操作參考](docs/wallet-reference.md) · [架構](docs/architecture.md) · [測試環境](TEST_INFRA.md) · [介面驗收](docs/ui-refresh-2026-09-15.md)

四種 README 版本涵蓋相同範圍，連結的技術文件保留原語言。執行錢包不需要前端框架或 Node 建置；瀏覽器測試才使用 Node。
