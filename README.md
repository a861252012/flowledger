# FlowLedger

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

A Go testnet wallet for transfers, token approvals, swaps and transaction history with CSV export. Signed transactions are saved before broadcast; retries with the same account, network and quote ID reuse the original transaction. Receipts and finality are checked separately.

## Networks

- Ethereum, Arbitrum, Base and OP Sepolia; Polygon Amoy: native coins and ERC-20 tokens.
- Ethereum Sepolia: WETH wrap/unwrap and Uniswap V3 WETH/test USDC swaps.
- Solana Devnet: SOL. TRON Shasta: TRX and TRC-20, each with a separate wallet.

## Run

Requires Docker and Docker Compose. Run from the repository root; skip the first command if `.env` already exists.

```sh
cp .env.example .env
docker compose build
docker compose run --rm --no-deps app go mod download
docker compose up -d
```

Open [localhost:8090](http://localhost:8090). Create or restore a test wallet, fund it with test assets, then preview and send a transaction.

## Limits

- Testnet prototype, not independently audited. Never import a wallet holding real assets or expose the service publicly.
- Back up the entire `wallet_data` volume. `docker compose down -v` deletes wallet data.
- Polygon and Solana outgoing acceptance remains incomplete in the 2026-09-15 record. ERC-4337 is a separate component, not integrated into the wallet UI.
