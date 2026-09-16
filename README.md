# FlowLedger

[English](README.md) · [简体中文](README.zh-CN.md) · [正體中文（台灣）](README.zh-TW.md) · [繁體中文（香港）](README.zh-HK.md)

**A Go testnet wallet exploring reliable transaction execution and recovery.**

FlowLedger follows a transaction from fee preview and local signing to durable storage, broadcast, receipt verification and finality observation. It focuses on backend failure cases: a lost RPC response, concurrent retries, process termination and chain reorganizations.

[Three-minute demo](docs/demo-script.md) · [On-chain evidence](docs/onchain-acceptance-2026-09-15.md) · [Recovery evidence](docs/process-recovery.md)

## Engineering highlights

- **Persist before broadcast.** Save signed bytes and the transaction hash before sending; recovery reuses the same signed transaction.
- **Deduplicate retries.** The same account/network and quote ID reuse a journal entry. Different quotes require a separate business idempotency key in an upstream payment service.
- **Preserve newer observations.** Version-checked journal updates prevent slow requests from overwriting newer receipt or reorg state.
- **Separate transaction states.** Broadcast, inclusion, successful execution and finality are checked separately. An RPC timeout does not prove failure.
- **Use exact amounts.** Integer arithmetic, explicit fee previews and finite token approvals keep signing intent inspectable.

## Supported scope

| Network | Wallet features | Evidence boundary |
| --- | --- | --- |
| Ethereum Sepolia | ETH / ERC-20, finite approvals, WETH wrap/unwrap, Uniswap V3 WETH/test USDC swaps | Included in the September 15 acceptance record |
| Arbitrum / Base / OP Sepolia | Native transfers, ERC-20, approvals and replacement transactions | Native transfers in that record |
| Polygon Amoy | POL and EVM token operations | Outgoing acceptance incomplete in that record |
| Solana Devnet | Independent SOL wallet | Outgoing acceptance incomplete in that record |
| TRON Shasta | Independent TRX / TRC-20 wallet | TRX and test-token transfers in that record |

The [2026-09-15 record](docs/onchain-acceptance-2026-09-15.md) documents **13 successful outgoing transactions across five testnets**. This is a historical snapshot, not verification of every feature or current network state. Mock tests are separate evidence.

The [ERC-4337 component](internal/wallet/erc4337) contains UserOperation encoding, hashing, builders, signing and a Bundler client with mock tests. It is **not integrated into the wallet UI**; deployed smart accounts and live Bundler acceptance are not verified. See [package scope](PROJECT.md).

## Run locally

Install Docker with Docker Compose, then run from the repository root. Create `.env` only if you do not already have one; preserve existing settings.

```sh
cp .env.example .env
docker compose build
docker compose run --rm --no-deps app go mod download
docker compose up -d
```

Open [localhost:8090](http://localhost:8090), or `/showcase` for the portfolio view. The default localhost demo needs no login. Setting `WALLET_ACCESS_TOKEN` to at least 32 characters enables login. Keep the port bound to `127.0.0.1`; do not expose the wallet through a tunnel or public proxy.

1. Create or restore a dedicated test wallet and securely back up its mnemonic offline.
2. Select the correct test network and obtain test assets. The optional local dispenser needs configuration and funded accounts; a clone includes neither funds nor credentials.
3. Preview the recipient, asset, amount and fees, then confirm with the wallet password to sign and broadcast.
4. Check the receipt and finality separately. If broadcast is uncertain, rebroadcast the original signed transaction.

Source or embedded UI changes require `docker compose restart app`. Compose environment changes require `docker compose up -d --force-recreate app`.

## Data and safety

**Testnet prototype only. Never import a wallet holding real assets.** The Go process handles decrypted keys during signing. This is not an audited custody service or a public multi-user wallet.

- Preserve the entire `wallet_data` volume, including journals and archives. A mnemonic alone does not restore transaction history or pending-operation safeguards.
- `docker compose down` retains volumes. **`docker compose down -v` deletes wallet data.**
- Receipt and finality checks rely on configured RPC providers; independent RPC quorum is not implemented.
- Activity and CSV exports cover verified indexed transactions, not a complete ledger, all internal transfers or an independent balance reconciliation.
- Mainnet, production readiness, SLA and independent security audit are outside the demonstrated scope.

## Verification

```sh
docker compose run --rm --no-deps app go test -race ./...
docker compose run --rm --no-deps app go vet ./...
npm ci --prefix tests/browser
cd tests/browser && npx playwright install chromium && npm test
```

These commands run Go and browser checks; they do not establish live-chain acceptance. [CI configuration](.github/workflows/verify.yml) includes isolated Go race/vet/coverage and browser jobs; inspect [Actions](https://github.com/a861252012/flowledger/actions) for actual run results.

To recheck the published transaction evidence through public testnet RPCs without signing or broadcasting:

```sh
go run ./cmd/verify-onchain-evidence
```

## Project map and documentation

| Path | Purpose |
| --- | --- |
| `cmd/flowledger` | Startup and configuration |
| `internal/wallet` | Keys, quotes, signing and transaction journals |
| `internal/chain` | RPC access and receipt verification |
| `internal/web` | HTTP guards and embedded HTML/CSS/JavaScript |
| `internal/wallet/erc4337` | Separate account-abstraction component |
| `tests/browser` | Browser tests with mock APIs |

[Detailed operating reference](docs/wallet-reference.md) · [Architecture](docs/architecture.md) · [Test infrastructure](TEST_INFRA.md) · [UI acceptance](docs/ui-refresh-2026-09-15.md)

All four README editions cover the same scope. Linked technical references retain their original language. No frontend framework or Node build is required to run the wallet; Node is used for browser tests.
