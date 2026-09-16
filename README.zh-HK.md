# Testnet Wallet Lab

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

用 Go 編寫的多鏈測試網錢包實驗專案，用來實作轉賬、代幣兌換、交易追蹤，以及 RPC 失敗或程式重啟後的恢復處理。

![錢包介面，使用本機測試資料](docs/images/wallet-overview.png)

*介面預覽使用本機測試資料。*

## 主要功能

- 建立、還原及管理本機錢包，支援加密備份。
- 發送原生幣與代幣、預覽手續費，設定或撤銷 ERC-20 授權額度。
- 在 Ethereum Sepolia 包裝／解包 WETH，透過 Uniswap V3 兌換 WETH 與測試 USDC。
- 按交易收據整理 EVM 收支紀錄，支援 CSV 匯出。

## 交易處理

交易簽署後先儲存，再發送至 RPC。同一賬戶、網絡與 quote ID 的重試會沿用已儲存的交易。廣播結果不明時，會按各鏈的有效期規則重發原始簽署資料。

EVM 收據是否成功、所在區塊是否仍在主鏈，以及是否 finalized，分開核對。程式重啟後，錢包會讀取交易日誌，繼續追蹤已儲存的交易。

## 支援網絡

| 網絡 | 功能 |
|---|---|
| Ethereum Sepolia | 原生幣／ERC-20 轉賬與授權、WETH 包裝、Uniswap V3 兌換 |
| Arbitrum、Base、OP Sepolia · Polygon Amoy | 原生幣／ERC-20 轉賬與授權 |
| Solana Devnet | SOL 轉賬，使用獨立錢包 |
| TRON Shasta | TRX／TRC-20 轉賬，使用獨立錢包 |

## 智能合約錢包實驗

已在 Ethereum Sepolia 完成賬戶部署及轉賬測試，目前只支援命令列操作。
