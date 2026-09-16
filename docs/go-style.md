# Go 分層與風格

參考 [Ardan Labs service，固定版本 `4d02671018b8b6138ceaaa7c40bd66e6d912bcbb`](https://github.com/ardanlabs/service/tree/4d02671018b8b6138ceaaa7c40bd66e6d912bcbb) 的型別邊界、分支可讀性與 modern Go 原則。這是 FlowLedger 採用的專案規範，不是 Ardan Labs 官方認證，也不把範例專案的 Kubernetes、資料庫與部署目錄搬進本機錢包。

## 套用範圍

| 範圍 | 責任與可檢查依據 |
|---|---|
| `cmd/flowledger` | `loadConfig` 集中環境設定解析；main 組裝服務、管理生命週期，不放交易規則。設定測試涵蓋預設值、無效值與 faucet 檔案權限。 |
| `internal/web` | HTTP、驗證存取來源、嚴格解碼、狀態碼與 primitive DTO。EVM、Solana、TRON、faucet、observe、diagnostics 都明確映射回應；不直接編碼交易日誌。完整入口清單見 `architecture.md`。 |
| `internal/wallet` | 交易動作、金額、帳戶與地址驗證、簽名、持久化順序與重送政策。`QuoteCommand`、`QuoteID`、`TransactionHash`、交易狀態及鏈別識別型別用於業務運算與查找。 |
| 儲存邊界 | EVM journal、Solana/TRON journal、scan 各有獨立 disk DTO 與具名轉換。service 持有業務紀錄；讀檔先解析，開啟 journal 仍驗證原始簽名交易。activity 索引只保存已驗證公開 hash。 |
| `internal/chain` | RPC adapter，primitive wire 值先解析為地址、hash、整數及狀態，再供查詢與交易邏輯使用。不可依賴 wallet 或 web。 |
| `internal/wallet/erc4337` | ABI/hash/signing 保持強型別，既有 JSON codec 經私有 primitive RPC DTO 轉換；失敗不得部分覆寫 receiver，不新增未使用的第二套公開解析 API。 |
| 前端、scripts、測試、CI | 驗收工具與測試統一使用 Go；前端保留 JavaScript。以 browser fixtures 和 Go CI 驗證整合。 |

複合 EVM 請求在 web → wallet 轉成 command；既有單值 service 入口在服務邊界解析地址、金額、報價或交易 ID，避免每個純量參數都再包一層沒有用途的 command。`QuoteRequest` 及公開 read model 保留相容用途；HTTP 與 durable 檔案使用自己的 DTO。加密 keystore 與備份沿用既有格式，`json.RawMessage` 代表完整加密檔案，不拆成交易業務物件。

## 可讀性與相容性

- 使用早期返回、有限狀態的 `switch`；不為單次欄位拷貝或未出現的需求建立介面與框架。
- 採 Go 1.26 的 `min`、`slices.SortFunc`／`SortStableFunc`、`slices.Clone`、`maps.Copy`、`errors.AsType`、`strings.SplitSeq` 與 `WaitGroup.Go`，以語意相同為前提。保留必要的 timeout context 與原本穩定排序。
- API 的欄位名稱、`omitempty`、時間格式、nil 與空陣列是契約，不因風格替換成 `omitzero`。
- 舊 EVM journal 允許缺少 `To`／`Action`，舊 opaque quote ID 不強迫改成新報價格式。交易 raw bytes、hash、落盤後廣播、CAS 版本與鎖順序保持原用途。
- 跨鏈 journal 的業務型別沒有 JSON tag；disk DTO 不嵌入公開回應型別。TRON 比較業務紀錄是否改變直接使用 `slices.Equal`，不透過 JSON round trip。

## 驗證與限制

`go test ./...` 包含 DTO literal JSON 契約、nil/empty、儲存格式、舊 journal、簽名重載、原始 bytes 重送、SIGKILL 復原及並行狀態更新測試。`TestLayerBoundaries` 檢查依賴方向與 web DTO 外部欄位型別，**不是**完整資料流靜態分析；新增路由仍要檢查實際 handler 與輸出。

CI 執行 `go test -race -count=1`、`go vet`、`gofmt` 與 `go fix -diff`，另外執行瀏覽器 fixture；CLI 測試已包含在 Go 測試內。Go 測試使用離線容器和暫存錢包；通過代表本機/mock 契約與復原驗證，不代表真實鏈上廣播、主網安全或正式環境效能。

驗收工具統一使用 Go：`go run ./cmd/send-and-verify --test-quote` 與
`go run ./cmd/verify-onchain-evidence`，測試納入 `go test ./...`。
後者預設從 repo 根目錄讀取 `internal/web/static/onchain-evidence.json`；
在其他目錄執行編譯後的 CLI 時，使用 `--manifest /absolute/path/to/onchain-evidence.json`。
`scripts/send_and_verify.sh` 僅保留為 Go CLI 的相容入口，不依賴 Python。
