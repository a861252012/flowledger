# FlowLedger

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

用 Go 寫的測試網錢包，支援轉帳、代幣授權、兌換及交易紀錄 CSV 匯出。交易簽署後先存檔再廣播；同一帳戶、網路與 quote ID 重試時沿用原交易，並分別核對收據與最終確定性。

## 支援網路

- Ethereum、Arbitrum、Base、OP Sepolia 及 Polygon Amoy：原生幣與 ERC-20。
- Ethereum Sepolia：WETH 包裝／解包、Uniswap V3 WETH／測試 USDC 兌換。
- Solana Devnet：SOL。TRON Shasta：TRX 與 TRC-20，兩者各用獨立錢包。
