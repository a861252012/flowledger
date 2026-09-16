# FlowLedger

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

**使用 Go 构建的测试网钱包，探索交易可靠性与故障恢复。**

FlowLedger 涵盖费用预览、本地签名、持久化、广播、回执核对与最终确定性观测。项目关注后端常见的交易难题：RPC 响应丢失、并发重试、进程终止与链重组。

[三分钟演示](docs/demo-script.md) · [链上验收记录](docs/onchain-acceptance-2026-09-15.md) · [故障恢复验证](docs/process-recovery.md)

## 工程重点

- **先保存，再广播**：先写入签名后的交易字节与哈希；恢复时复用同一笔已签名交易。
- **重试去重**：同一账户、网络与 quote ID 复用已有交易日志。不同报价仍需由上游支付服务通过业务标识防止重复付款。
- **保留更新的观测**：交易日志更新时检查版本，避免慢请求覆盖更新的回执或链重组状态。
- **分别判断交易状态**：广播、上链、执行成功及最终确定性分别核对；RPC 超时不代表交易失败。
- **精确金额**：采用整数运算、明确的费用预览及有限额度代币授权，让签名内容可供确认。

## 支持范围

| 网络 | 钱包功能 | 验收边界 |
| --- | --- | --- |
| Ethereum Sepolia | ETH／ERC-20、有限额度授权、WETH 包装／解包、Uniswap V3 WETH／测试 USDC 兑换 | 纳入 9 月 15 日验收记录 |
| Arbitrum／Base／OP Sepolia | 原生币转账、ERC-20、授权与替换交易 | 记录包含原生币转账 |
| Polygon Amoy | POL 与 EVM 代币操作 | 该记录中的发送验收尚未完成 |
| Solana Devnet | 独立 SOL 钱包 | 该记录中的发送验收尚未完成 |
| TRON Shasta | 独立 TRX／TRC-20 钱包 | 记录包含 TRX 与测试代币转账 |

[2026-09-15 验收记录](docs/onchain-acceptance-2026-09-15.md)记载了**五个测试网共 13 笔成功发送的交易**。这是历史快照，不代表所有功能或当前网络状态均已验证；Mock 测试属于另一类证据。

[ERC-4337 组件](internal/wallet/erc4337)包含 UserOperation 编码、哈希、构建、签名、Bundler 客户端及 Mock 测试。目前**尚未集成到钱包界面**，也未验证已部署的智能账户或真实 Bundler 验收；详见[模块范围](PROJECT.md)。

## 本地启动

先安装 Docker 与 Docker Compose，再在项目根目录执行。仅在尚无 `.env` 时创建，已有配置请保留。

```sh
cp .env.example .env
docker compose build
docker compose run --rm --no-deps app go mod download
docker compose up -d
```

打开 [localhost:8090](http://localhost:8090)，作品演示页为 `/showcase`。默认本地演示无需登录；将 `WALLET_ACCESS_TOKEN` 设为至少 32 个字符可启用登录。端口请保持绑定 `127.0.0.1`，不要通过隧道或公共代理对外开放钱包。

1. 创建或恢复测试专用钱包，离线妥善备份助记词。
2. 选择正确的测试网并获取测试资产。可选的本地发币账户需要配置与余额；克隆项目不会附带资金或凭证。
3. 预览收款地址、资产、金额及费用，再输入钱包密码确认签名与广播。
4. 分别核对回执与最终确定性。广播结果不明确时，重新广播原始已签名交易。

修改源代码或内嵌界面后执行 `docker compose restart app`；修改 Compose 环境配置后执行 `docker compose up -d --force-recreate app`。

## 数据保存与安全边界

**仅供测试网原型使用，请勿导入持有真实资产的钱包。** Go 进程会在签名时处理解密后的密钥；本项目不是经过安全审计的托管服务或公开多用户钱包。

- 保存完整的 `wallet_data` volume，包括交易日志与归档文件。仅恢复助记词无法恢复交易记录或待处理交易的保护机制。
- `docker compose down` 保留 volume；**`docker compose down -v` 会删除钱包数据。**
- 回执及最终确定性核对依赖配置的 RPC 提供商，尚未实现独立 RPC 多方共识验证。
- 收支流水及 CSV 仅涵盖已索引并验证的交易，不是完整账本、所有内部转账或独立余额对账。
- 当前演示范围不包含主网、生产就绪保证、SLA 或独立安全审计。

## 验证

```sh
docker compose run --rm --no-deps app go test -race ./...
docker compose run --rm --no-deps app go vet ./...
npm ci --prefix tests/browser
cd tests/browser && npx playwright install chromium && npm test
```

以上命令执行 Go 与浏览器检查，不代表真实链验收。[CI 配置](.github/workflows/verify.yml)包含隔离的 Go race／vet／coverage 与浏览器任务；实际运行结果请查看 [Actions](https://github.com/a861252012/flowledger/actions)。

通过公共测试网 RPC 重新核对已发布的交易证据，不签名也不广播：

```sh
go run ./cmd/verify-onchain-evidence
```

## 项目结构与文档

| 路径 | 用途 |
| --- | --- |
| `cmd/flowledger` | 启动与配置 |
| `internal/wallet` | 密钥、报价、签名及交易日志 |
| `internal/chain` | RPC 访问与回执验证 |
| `internal/web` | HTTP 防护及内嵌 HTML／CSS／JavaScript |
| `internal/wallet/erc4337` | 独立账户抽象组件 |
| `tests/browser` | 使用 Mock API 的浏览器测试 |

[详细操作参考](docs/wallet-reference.md) · [架构](docs/architecture.md) · [测试环境](TEST_INFRA.md) · [界面验收](docs/ui-refresh-2026-09-15.md)

四种 README 版本涵盖相同范围，链接的技术文档保留原语言。运行钱包不需要前端框架或 Node 构建；浏览器测试才使用 Node。
