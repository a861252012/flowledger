# Testnet Wallet Lab

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

用 Go 编写的多链测试网钱包实验项目，用来实现转账、代币兑换、交易跟踪，以及 RPC 失败或进程重启后的恢复处理。

![钱包界面，使用本地测试数据](docs/images/wallet-overview.png)

*界面预览使用本地测试数据。*

## 主要功能

- 创建、恢复及管理本地钱包，支持加密备份。
- 发送原生币与代币、预览手续费，设置或撤销 ERC-20 授权额度。
- 在 Ethereum Sepolia 包装／解包 WETH，通过 Uniswap V3 兑换 WETH 与测试 USDC。
- 根据交易收据整理 EVM 收支记录，支持 CSV 导出。

## 交易处理

交易签名后先保存，再发送到 RPC。同一账户、网络与 quote ID 的重试会复用已保存的交易。广播结果不明时，会按各链的有效期规则重发原始签名数据。

EVM 收据是否成功、所在区块是否仍在主链，以及是否 finalized，分别核对。进程重启后，钱包会读取交易日志，继续跟踪已保存的交易。

## 支持网络

| 网络 | 功能 |
|---|---|
| Ethereum Sepolia | 原生币／ERC-20 转账与授权、WETH 包装、Uniswap V3 兑换 |
| Arbitrum、Base、OP Sepolia · Polygon Amoy | 原生币／ERC-20 转账与授权 |
| Solana Devnet | SOL 转账，使用独立钱包 |
| TRON Shasta | TRX／TRC-20 转账，使用独立钱包 |

## 智能合约钱包实验

已在 Ethereum Sepolia 完成账户部署与转账测试，目前仅支持命令行操作。
