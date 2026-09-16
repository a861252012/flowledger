# FlowLedger

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

用 Go 寫的測試網錢包，支援轉帳、代幣授權、兌換及交易紀錄 CSV 匯出。交易簽署後先存檔再廣播；同一帳戶、網路與 quote ID 重試時沿用原交易，並分別核對收據與最終確定性。

## 支援網路

- Ethereum、Arbitrum、Base、OP Sepolia 及 Polygon Amoy：原生幣與 ERC-20。
- Ethereum Sepolia：WETH 包裝／解包、Uniswap V3 WETH／測試 USDC 兌換。
- Solana Devnet：SOL。TRON Shasta：TRX 與 TRC-20，兩者各用獨立錢包。

## 啟動

需要 Docker 和 Docker Compose。在專案根目錄執行；已有 `.env` 就跳過第一行。

```sh
cp .env.example .env
docker compose build
docker compose run --rm --no-deps app go mod download
docker compose up -d
```

開啟 [localhost:8090](http://localhost:8090)，建立或還原測試錢包，取得測試幣後即可預覽並發送交易。

## 限制

- 測試網原型，尚未經獨立安全稽核。請勿匯入持有真實資產的錢包，也不要將服務公開到網際網路。
- 請備份完整 `wallet_data` volume；`docker compose down -v` 會刪除錢包資料。
- 2026-09-15 驗收紀錄中，Polygon、Solana 尚未完成發送驗收。ERC-4337 是獨立元件，尚未接入錢包介面。
