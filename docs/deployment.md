# 公開唯讀 Demo 部署

目標：`https://wallet.tedlin.fyi`，所有 IP 可瀏覽與查詢；只有管理者可操作錢包。
Cloudflare Free + 公開 Tunnel + Oracle Ubuntu VM + GitHub Actions/GHCR。
不啟用 Zero Trust 訂閱、Access、Service Token 或按量付費附加產品。
域名註冊與續約另計。免費方案仍可能限流、中斷或回收；不是可用性保證。

## 權限與資源邊界

- 設定 `PUBLIC_ORIGIN` 後，未登入首頁為獨立公開查詢頁。公開 GET/HEAD 僅允許靜態檔案、EVM 網路、地址餘額、代幣公開餘額與交易查詢。
- EVM 支援 Ethereum/Arbitrum/Base/OP Sepolia 與 Polygon Amoy。Solana/TRON 共用錢包狀態不對訪客公開。
- 帳戶清單、CSRF token、錢包狀態、交易日誌、scanner、faucet，以及全部 POST/PUT/DELETE 均需管理者認證。新路由預設不公開。
- `/login` 使用 VM 上隨機產生的 `WALLET_ACCESS_TOKEN`，不可分享給訪客。建立、匯入、簽名、轉帳仍有原本的密碼及 CSRF 檢查。
- 固定 Host/Origin；不信任 forwarded headers。允許從履歷等外站點擊進入文件，但拒絕跨站 fetch 及提交。
- 管理者和公開查詢各有獨立的每分鐘 300 次 API 額度、8 read/1 write 併發上限；不是每人配額。惡意訪客仍可能耗盡公開額度，使查詢暫時回傳 429。
- API body 最多 16 KiB、請求 context 最多 45 秒；登入 body 最多 4 KiB，失敗認證/登入每分鐘共 10 次。
- Cookie Secure/HttpOnly/SameSite=Strict；程序重新啟動會撤銷舊 session。不要在共用電腦登入。
- 容器 non-root、唯讀 rootfs、無 capabilities、640 MiB RAM（禁止額外 swap）、480 MiB Go soft limit、128 PID、1 CPU、log 輪替 3×10 MiB。
- RAM 上限超過會停止程序，並非保證錢包操作永不 OOM。1 GiB VM 必須驗收實際尖峰及 OS/cloudflared 餘裕。
- HTTP 限流不是 RPC 次數或費用硬上限。使用無付費憑證的公共測試網 RPC；不要配置有自動超額計費的 RPC/API。背景維護也可能發出 RPC。
- 只使用可丟棄測試錢包，絕不匯入日常助記詞或真實資產。

## Cloudflare

網域已購買，wallet-demo Tunnel 已建立；VM connector、HTTPS/WAF 尚需完成設定及驗收。Zero Trust 結帳未啟用。

1. 網域保留 Free。由帳戶 **Networking → Tunnels** 建立 `wallet-demo`，不要進入 Zero Trust 結帳。
2. 在 VM 以官方 cloudflared 系統服務連接；憑證僅放 VM root-only 檔案，不放 Git/CI/log。
3. 公開 hostname `wallet.tedlin.fyi` → `http://127.0.0.1:8090`，保留 Host。
4. 驗證 Universal SSL 有效，HTTP 轉 HTTPS；免費 managed WAF ruleset 生效。
5. 唯一免費 rate limiting 規則優先保護此 hostname 的 `/login`，其餘 API 由程式限流。實際可選 period/action 以帳戶 UI 為準，不升級購買規則。
6. 不設定 Cache Everything；API、登入及管理者回應必須保留 `Cache-Control: no-store`。
7. 確認 Cloudflare hostname 可用後關閉 VM 公網 SSH 入站；永遠不公開 8090/Docker socket。

若帳戶的 Tunnel 入口仍強制超額扣款授權，停止並另議，不接受條款。
官方：[Tunnel](https://developers.cloudflare.com/tunnel/get-started/)、[WAF 方案](https://developers.cloudflare.com/waf/)。

## VM 首次安裝

本次機器為 Oracle AMD E2.1.Micro，Ubuntu 24.04 Minimal，1 GiB RAM。
起始管理 SSH 僅開放目前管理者 IP/32，不是網站訪客 IP 限制。
先有 OCI Console 復原方式，再移除公網 SSH。
Docker/Compose、curl、python3、flock、systemd 需先依官方來源安裝；啟用 OS 安全更新。

管理者將審閱後的部署檔案複製到 VM：

```sh
sudo bash scripts/deploy/install.sh
sudoedit /opt/testnet-wallet-lab/.env
```

設定 `PUBLIC_ORIGIN=https://wallet.tedlin.fyi`，保留腳本產生的存取碼。
Compose 只監聽主機 `127.0.0.1:8090`；wallet volume UID/GID 10001、0700。
`.env` 為 root-only 0600。不要把 token、備份或 wallet 複製到 Git。
安裝不建立 CI SSH 帳號、不授予 sudo/docker group、不安裝 self-hosted runner。

## CI/CD：VM 主動拉取

```text
push main → Go/race/vet + Browser + 雙架構 image smoke + govulncheck
          → build/test release image → GHCR sha-<commit>
VM 每兩分鐘 → 讀取 main SHA → 拉取對應 image → 固定 digest + revision 驗證
          → 部署鎖 → 再查 main → 停舊版 → 啟新版 → HTTP 權限驗收
          → 成功記錄 current-image，失敗回退原 image
```

Repository Variables：`DEMO_DEPLOY_ENABLED=true`（所有前置完成後）、`DEMO_RUNNER=ubuntu-latest`。
PR 不發布；publish 只用 workflow 內建 GITHUB_TOKEN packages:write。不要給 PR 部署 secrets。
GHCR package 必須 Public，VM 不保存 GitHub credentials。Repo 為公開不代表 package 自動公開。
設定 main ruleset 禁止 force push，要求必要 CI 檢查；變更 GitHub 權限需另行確認。

首次發布完成後，管理者執行：

```sh
sudo systemctl start wallet-demo-update.service
sudo journalctl -u wallet-demo-update.service -n 50 --no-pager
sudo systemctl enable --now wallet-demo-update.timer
```

timer 在前一次結束後約 120 秒再檢查，不是即時 webhook。CI 未完成/失敗、API/registry 失聯時不停止現有服務。
GitHub Actions 成功只證明 image 發布成功；遠端成功須查 journal、current-image 與 binary version。
無 CI 入站 SSH/Access 或 GitHub deploy secret。

部署腳本/Compose 為 root-owned，main push 只更新應用 image，不覆寫基礎權限設定。
失敗回退保留同一份 wallet/journal，**不回退資料**；不相容資料格式須先備份、驗證遷移。
容器 image 會累積磁碟用量：定期審查再清除不用的 image，保留現行與上一個可回退版本。
不要執行 `docker compose down -v` 或自動刪除 wallet。

## 驗證與剩餘工作

本機安全測試涵蓋匿名讀寫、跨鏈/帳戶繞路、錯誤 Host/Origin、外站連結、獨立流量額度及過大 body。
容器 smoke 使用專用 disposable volume 和 `--network none`，不接觸現有錢包或真實鏈。

```sh
go test -race ./...
go vet ./...
python3 -m unittest discover -s tests/deployment -p 'test_*.py'
docker build -f Dockerfile.deploy --build-arg APP_VERSION=local-test -t wallet-demo-test .
python3 tests/deployment/smoke.py wallet-demo-test local-test
```

上線前仍需：實際 VM 安裝、免費 Tunnel/WAF/HTTPS、匿名外部查詢與管理者拒絕測試、VM 直連拒絕、首次 main CI→遠端驗收。
備份應在持有 deploy.lock 且容器停止時封存 wallet，加密保存於 VM 外；隔離無網路還原測試不可省略。
目前尚未選定私人備份目的地，不把本機 smoke 當成災難復原完成。
所有安全控制降低風險，不保證沒有漏洞；沒有執行真實鏈交易。

## 2026-09-17 本機驗證

- `go test -race ./...`、`go vet ./...` 通過；新增公開權限/跨來源/流量/大小限制案例通過。
- `go fix -diff ./cmd/... ./internal/...` 無差異；actionlint v1.7.7、Compose config、shell syntax、diff whitespace 檢查通過。
- 13 個部署/輪詢替身測試通過，包含 registry 失敗、錯誤來源、stale main、回退與首次失敗；不是雲端實測。
- Playwright 全套通過，包含公開查詢頁、輸入驗證、375px 排版及原有多鏈介面；所有 API 使用 mock。
- govulncheck v1.8.0 無可達已知漏洞；另有 2 個 imported-package、20 個 required-module 不可達漏洞，不代表完整安全稽核。
- 上述為本機驗證紀錄；雲端上線狀態須以後續實測為準。
- ARM64 與 AMD64 實際容器均通過隔離 smoke：non-root/readonly、登入/CSRF、容器替換保留錢包與舊 session 拒絕。沒有真實 RPC 或鏈上交易。
