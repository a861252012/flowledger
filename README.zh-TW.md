# Testnet Wallet Lab

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

**[線上 Demo（僅供測試鏈使用）](https://wallet.tedlin.fyi/)**

用 Go 寫的多鏈測試網錢包實驗專案，用來實作轉帳、代幣兌換、交易追蹤，以及 RPC 失敗或程序重啟後的恢復處理。

![錢包介面，使用本機測試資料](docs/images/wallet-overview.png)

*介面預覽使用本機測試資料。*

## 主要功能

- 建立、還原及管理本機錢包，支援加密備份。
- 發送原生幣與代幣、預覽手續費，設定或撤銷 ERC-20 授權額度。
- 在 Ethereum Sepolia 包裝／解包 WETH，透過 Uniswap V3 兌換 WETH 與測試 USDC。
- 依交易收據整理 EVM 收支紀錄，支援 CSV 匯出。

## 交易處理

交易簽署後先存檔，再送到 RPC。同一帳戶、網路與 quote ID 的重試會沿用已保存的交易。廣播結果不明時，會依各鏈的有效期限規則重送原始簽署資料。

EVM 收據是否成功、所在區塊是否仍在主鏈，以及是否 finalized，分開核對。程序重啟後，錢包會讀回交易日誌，繼續追蹤已保存的交易。

## 支援網路

| 網路 | 功能 |
|---|---|
| Ethereum Sepolia | 原生幣／ERC-20 轉帳與授權、WETH 包裝、Uniswap V3 兌換 |
| Arbitrum、Base、OP Sepolia · Polygon Amoy | 原生幣／ERC-20 轉帳與授權 |
| Solana Devnet | SOL 轉帳，使用獨立錢包 |
| TRON Shasta | TRX／TRC-20 轉帳，使用獨立錢包 |

## 智慧合約錢包實驗

已在 Ethereum Sepolia 完成帳戶部署與轉帳測試，目前僅支援命令列操作。

## 公開 Demo 部署

[部署與驗證說明](docs/deployment.md)包含 VM、免費 Cloudflare Tunnel、共用測試錢包模式與 main 自動部署設定。共用模式下，訪客免網站登入使用同一個測試錢包；簽署新交易及匯出加密金鑰仍需錢包密碼，訪客可建立並命名受密碼保護的 EVM 測試錢包（全站最多 20 個）；既有錢包的更名、封存仍受限制。VM 主動拉取驗證後映像，不需要 CI SSH 憑證。公開網址：https://wallet.tedlin.fyi/ 。每次 push main 通過 CI 後發布映像，VM 每兩分鐘檢查更新。

介面將交易狀態與依收據整理的收支明細集中在「活動」。診斷入口放在設定；公開共用模式隱藏無法使用的管理操作。EVM、Solana 與 TRON 的地址簿支援搜尋、改名、複製、直接發送及移除後復原，依測試網路儲存在目前瀏覽器；不會跨裝置同步。EVM 收款人選單也包含已建立的錢包。外部水龍頭仍有各自的資格與額度限制，不保證領取成功。
