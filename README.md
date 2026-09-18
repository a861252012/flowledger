# Testnet Wallet Lab

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

**[Live demo (testnets only)](https://wallet.tedlin.fyi/)**

A multi-chain testnet wallet experiment written in Go, for exploring transfers, token swaps, transaction tracking and recovery after RPC failures or restarts.

![Wallet interface with local test data](docs/images/wallet-overview.png)

*Interface preview using local test data.*

## Solidity ETH vault

A minimal Solidity deposit/withdraw contract with a Sepolia-only panel, per-address balances and reentrancy protection. Local contract and browser tests pass; Sepolia deployment and live deposit/withdraw acceptance are still pending. The panel remains disabled until a deployed address is configured.

[Contract, tests and setup](docs/eth-vault.md). The vault was developed with AI assistance.

## Run and verify

[Local setup](docs/wallet-reference.md#run-with-docker) · [Architecture](docs/architecture.md) · [Demo walkthrough](docs/demo-script.md) · [Go style and checks](docs/go-style.md)

## Features

- Create, restore and manage local wallets with encrypted backups.
- Send native coins and tokens, preview fees, and set or revoke ERC-20 allowances.
- Wrap and unwrap WETH, and swap WETH / test USDC through Uniswap V3 on Ethereum Sepolia.
- Build EVM activity records from transaction receipts and export them as CSV.

## Transaction handling

Signed transactions are saved before broadcast. Retries with the same account, network and quote ID reuse the saved transaction. When a broadcast result is uncertain, recovery uses the original signed bytes, subject to each network's expiry rules.

EVM receipt success, canonical block inclusion and finality are checked separately. After a restart, the wallet reloads its transaction journal to continue tracking saved transactions.

## Networks

| Network | Operations |
|---|---|
| Ethereum Sepolia | Native coin / ERC-20 transfers and approvals, WETH wrapping, Uniswap V3 swaps |
| Arbitrum, Base, OP Sepolia · Polygon Amoy | Native coin / ERC-20 transfers and approvals |
| Solana Devnet | SOL transfers, separate wallet |
| TRON Shasta | TRX / TRC-20 transfers, separate wallet |

## Smart contract wallet experiment

Account deployment and a transfer have been tested on Ethereum Sepolia. This experiment currently runs from the command line only.

## Public demo deployment

See [deployment setup and verification](docs/deployment.md) for the VM, free Cloudflare Tunnel, shared test wallet mode, and main-branch CI/CD. In shared mode, visitors use the same test wallet without a website login; signing new transactions and exporting encrypted keys require the wallet password. Visitors can create and name a password-protected EVM test wallet (20 accounts total); renaming and archiving existing wallets remain restricted. The VM pulls verified images without CI SSH credentials. Live demo: https://wallet.tedlin.fyi/. Main pushes publish verified images; the VM checks for updates every two minutes.

The wallet UI groups transaction status and receipt-based asset movements under Activity. Advanced diagnostics live in Settings; unavailable shared-demo administration is hidden. Contacts support search, rename, copy, send and undo removal on EVM, Solana and TRON, stored only in the current browser per test network. The EVM recipient picker also includes existing wallets. Test tokens are requested directly with a button. EVM/TRON require a separately funded faucet account; SOL uses Devnet airdrops subject to upstream limits. The demo deployment accepts TEST_FAUCET_CONFIG pointing to a private 0600 JSON file inside /data/wallet; no funding keys or passwords belong in Git.
