# FlowLedger

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

用 Go 写的测试网钱包，支持转账、代币授权、兑换及交易记录 CSV 导出。交易签名后先保存再广播；同一账户、网络与 quote ID 重试时复用原交易，并分别核对回执与最终确定性。

## 支持网络

- Ethereum、Arbitrum、Base、OP Sepolia 及 Polygon Amoy：原生币与 ERC-20。
- Ethereum Sepolia：WETH 包装／解包、Uniswap V3 WETH／测试 USDC 兑换。
- Solana Devnet：SOL。TRON Shasta：TRX 与 TRC-20，两者各用独立钱包。

## 启动

需要 Docker 和 Docker Compose。在项目根目录执行；已有 `.env` 就跳过第一行。

```sh
cp .env.example .env
docker compose build
docker compose run --rm --no-deps app go mod download
docker compose up -d
```
