# 公開 Demo 部署

目標：`https://wallet.tedlin.fyi`，所有 IP 可開啟共用測試錢包；簽署新交易與匯出加密金鑰仍需錢包密碼。
Cloudflare Free + 公開 Tunnel + Oracle Ubuntu VM + GitHub Actions/GHCR。
不啟用 Zero Trust 訂閱、Access、Service Token 或按量付費附加產品。
域名註冊與續約另計。免費方案仍可能限流、中斷或回收；不是可用性保證。

## 共用錢包模式

設定 `SHARED_DEMO=true` 並保留正確的 `PUBLIC_ORIGIN`。首次須在原本受保護的模式建立專用測試錢包；共用模式遇到空錢包會拒絕啟動，避免被第一位陌生訪客占用。保留原有 token 可供切回受保護模式，但共用模式不使用它驗證訪客。

- 首頁使用完整本機介面，沒有擁有者登入。EVM 測試網共用同一把測試金鑰；錢包地址、交易紀錄及狀態對訪客公開。
- 報價、代幣查詢、簽署送出、重新廣播既有已簽交易、密碼驗證後匯出加密備份可用。
- 匯入／更名／封存帳戶、改密碼、背景掃描、流水匯入與內建 faucet 不對公眾開放。Solana Devnet 與 TRON Shasta 的查詢、報價、密碼簽署、既有交易重送與加密備份可用。這些限制由伺服器強制執行。
- Solana/TRON 各自使用獨立測試錢包及備份；只有帶有效 `WALLET_ACCESS_TOKEN` 管理憑證及 CSRF 的 CLI 請求可呼叫 `/solana/api/create`、`/tron/api/create` 初始化空錢包，不能覆寫已有錢包。訪客不需登入。
- EVM「管理錢包」允許訪客以名稱與 12–128 字元密碼一次建立獨立錢包，再切換使用；不先公開空帳戶。全站最多 20 個帳戶（含主要錢包），名稱與地址公開，請下載加密備份保存。
- 建立帳戶與三種錢包的簽署、備份合計全站每分鐘最多 10 次；API 原有總額度、併發、CSRF、Host/Origin、容器限制持續生效。CSRF 不是使用者身分驗證。
- 變更模式前同步 VM 的 `compose.demo.yaml` 與 `verify-demo.py`；舊版驗收預期匿名 401，不能直接用來驗收共用模式。
- 設定 `SHARED_DEMO=false` 可恢復下述訪客唯讀模式。

## 受保護模式的權限與資源邊界

- 設定 `PUBLIC_ORIGIN` 後，未登入首頁與本機錢包共用 `index.html`、側欄、總覽與語言／主題控制。訪客只載入唯讀控制器，不請求私人錢包資料；查詢公開地址後可在總覽查看餘額。公開 GET/HEAD 僅允許工作區頁面、作品頁、靜態檔案、EVM 網路、地址餘額、代幣公開餘額與交易查詢。
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

wallet-demo Tunnel 已連線，公開路由與 HTTPS 已實測。使用 Cloudflare Free；Zero Trust 結帳未啟用。

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
私人備份目的地為管理者 Mac；不要把本機 smoke 當成已有錢包資料的災難復原證據。
所有安全控制降低風險，不保證沒有漏洞；沒有執行真實鏈交易。

## 2026-09-17 本機驗證

- `go test -race ./...`、`go vet ./...` 通過；新增公開權限/跨來源/流量/大小限制案例通過。
- `go fix -diff ./cmd/... ./internal/...` 無差異；actionlint v1.7.7、Compose config、shell syntax、diff whitespace 檢查通過。
- 13 個部署/輪詢替身測試通過，包含 registry 失敗、錯誤來源、stale main、回退與首次失敗；不是雲端實測。
- Playwright 全套通過，包含公開查詢頁、輸入驗證、375px 排版及原有多鏈介面；所有 API 使用 mock。
- govulncheck v1.8.0 無可達已知漏洞；另有 2 個 imported-package、20 個 required-module 不可達漏洞，不代表完整安全稽核。
- 上述為本機驗證紀錄；雲端上線狀態須以後續實測為準。
- ARM64 與 AMD64 實際容器均通過隔離 smoke：non-root/readonly、登入/CSRF、容器替換保留錢包與舊 session 拒絕。沒有真實 RPC 或鏈上交易。

## 2026-09-17 雲端驗收

- 首次應用版本 `cd7f97c29bb3abc2180e3c536f5724617964e2d2`；GitHub Actions [35127343438](https://github.com/a861252012/testnet-wallet-lab/actions/runs/35127343438) 全部通過。
- GHCR image 公開且 VM 無 registry credentials；部署日誌驗證 binary revision、認證、Origin、Cookie、CSRF 成功。
- `https://wallet.tedlin.fyi/` 回傳 200，HTTP 301 至 HTTPS；Cloudflare 通用憑證使用中，最低 TLS 1.2、TLS 1.3 啟用。
- 公開頁與五個 EVM 測試網 network endpoint 實際回傳 200。匿名錢包/帳戶/日誌/Solana/TRON/faucet 回傳 401。
- 外部 HTTPS 管理者登入成功，Cookie Secure/HttpOnly/SameSite=Strict；錯誤 Origin 與缺少 CSRF 的寫入要求均拒絕，未執行鏈上交易。
- Cloudflare Managed Free Ruleset 一律使用中；`Wallet demo login protection` 對此 hostname 的 POST `/login` 設定每 IP 5 次/10 秒，超出封鎖 10 秒。此為免費規則限制，不是費用硬上限。
- VM 使用 Ubuntu 套件庫 Docker/Compose 與 Cloudflare 官方 cloudflared；OS 更新已安裝，apt 安全更新 timer 啟用。停用 root SSH、密碼登入與不需要的 rpcbind。
- cloudflared 使用 DynamicUser、systemd credential 檔、唯讀系統與 160 MiB 上限。應用僅 bind `127.0.0.1:8090`。
- `wallet-demo-update.timer` 已啟用，每輪結束後 120 秒查 main；沒有 CI 入站 SSH 憑證。
- 管理者登入憑證只保留 VM root-only `.env` 與管理者 Mac 的受限檔案，不在 repository。
- 異地備份選用管理者 Mac，透過 age 加密；VM 目前尚未建立使用者錢包。

管理時可在 OCI 的 wallet-demo-security 暫時加入目前管理者 IP/32 的 TCP 22，使用既有 wallet-demo-admin SSH key，作業後移除。不要開放全網 SSH，也不要公開 8090。

## 管理者 Mac 加密備份

先暫時開放管理者 IP/32 的 SSH，再執行 `bash scripts/deploy/backup-to-mac.sh VM_IPV4`（Mac 需安裝 age）。
腳本持有部署鎖、暫停 app 後封存 wallet、環境設定、image digest 與 Compose；結束或失敗均嘗試啟動 app。
備份會造成短暫網站中斷，完成後須檢查 HTTPS 與健康狀態。
加密檔放在 `~/Documents/wallet-demo-backups/`，解密金鑰放在 `~/.ssh/wallet-demo-secrets/backup-age.key`，權限分別為 0700/0600。
金鑰遺失便無法還原；應自行另外離線保管，不能只依賴同一台 Mac。不得把金鑰提交 Git 或與備份一起分享。
這是手動備份，Mac 關機不會執行，也未設定自動排程；建立/匯入測試錢包後應再做備份。
還原只能先在隔離目錄解密，使用 `--network none` 的一次性容器驗證，不能直接覆蓋現行 VM wallet。
