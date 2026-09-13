# FlowLedger

A local, single-account **Go wallet for Ethereum Sepolia**. Create or restore a wallet, receive test assets, preview fees, sign EIP-1559 transactions locally, and broadcast ETH / ERC-20 transfers and finite approvals. Wrap/unwrap ETH and WETH, swap WETH/test USDC through Uniswap V3, and reconstruct receipt-based activity with CSV export.

This is a testnet prototype. Use a dedicated test mnemonic. **Do not import a wallet holding real assets.** The Go process handles decrypted keys transiently for signing; this is not a browser extension, hardware wallet, audited custody system, or public multi-user service.

## Run with Docker

```sh
docker compose build
docker compose run --rm --no-deps app go mod download
docker compose up -d
```

Open <http://localhost:8090>. The host port binds only to `127.0.0.1`. Optional settings are in `.env.example`; never commit RPC credentials. `WALLET_DIR=/data/wallet` uses the dedicated `wallet_data` volume. The existing PostgreSQL service is provisioned but is **not used by the wallet**; wallet persistence uses an encrypted keystore and an atomic transaction journal.

```sh
docker compose ps
docker compose logs -f app
docker compose restart app
```

Source and embedded HTML/CSS/JS changes require `restart app`. Changed Compose environment/volumes require `docker compose up -d --force-recreate app`.

## Use the wallet

1. Create a wallet with a password, or restore an English BIP-39 test mnemonic. Passwords must be 12–128 UTF-8 bytes; spaces are preserved. The UI requires at least 12 characters.
2. Write down the generated 12 words **offline and in order**. They are displayed once and never saved in plaintext. Do not send them to an AI, logs, screenshots, or chat. Confirm backup to clear them from the screen.
3. Fund the displayed address with **Sepolia ETH** from a testnet faucet. The dashboard links to [Google's Sepolia faucet](https://cloud.google.com/application/web3/faucet/ethereum/sepolia), [Ethereum's faucet list](https://ethereum.org/developers/docs/networks/#sepolia), and [Circle's test USDC faucet](https://faucet.circle.com/). External services may require login, verification or impose limits. The app does not provide test assets. ETH is required for gas even when transferring tokens.
4. Enter a recipient and amount, request a quote, and verify the network, recipient, asset contract, exact amount and maximum gas fee. Enter the wallet password to sign and broadcast.
5. Refresh history to check the receipt. A broadcast result is not confirmation of execution. If the result is uncertain, use **重新廣播原交易**; this reuses the original signed bytes and hash.

The dashboard's first-transaction guide can prefill a **0.000001 ETH self-transfer** or **0.000001 ETH → WETH wrap**. These controls only fill the existing forms; quotes and password confirmation remain separate. A self-transfer returns the principal to the same address and consumes test ETH gas. A positive ETH balance does not prove it covers the selected transaction's fee. Funding checks distinguish zero balance from unavailable RPC data.

ERC-20: enter a Sepolia token contract to read `symbol`, `decimals`, and `balanceOf`. Select that token for `transfer` or `approve`. For approval, the recipient field is the **spender**, not a transfer recipient. Only an explicit finite allowance is permitted; `0` revokes. Existing nonzero allowance must first be set to zero before a new nonzero allowance is accepted. Token metadata is supplied by the contract and is not proof of legitimacy. Tokens with nonstandard metadata or special transfer semantics may be unsupported.

Refreshing the wallet queries WETH, USDC and every token added in the current browser session. Failed token queries retain the selected asset and last known balance, visibly marked as outdated; a successful retry clears that warning. Quotes independently recheck chain data. Token precision, raw amounts and event evidence are available in expandable details; recipients, asset contracts, readable amounts and fee limits remain visible for confirmation.

## Keys and backup

- BIP-39 English mnemonics; BIP-32/BIP-44 path `m/44'/60'/0'/0/0`, first Ethereum account only.
- The optional BIP-39 passphrase is fixed to empty. A keystore encryption password is a separate concept; importing a mnemonic from a passphrase-protected wallet will not restore that wallet.
- Ethereum V3 keystore encrypted with geth StandardScrypt settings. Files use `0600`; wallet directory uses `0700`. A process lock prevents two app instances from operating the same directory.
- The backup control downloads a password-protected V3 keystore after verifying the password. The UI restores from mnemonic; the exported keystore is for compatible Ethereum tools or offline recovery. Keep the backup password separately.
- Passwords are not stored, and no unlocked session is retained. Mutable key material is cleared where practical; Go garbage collection prevents a guarantee that every in-memory copy is erased.
- Preserve the **whole wallet volume**, including `journal.json`, when moving/restarting an active wallet. Restoring only the mnemonic does not restore this application's transaction history or pending-operation safeguards.

`docker compose down` keeps data. **`docker compose down -v` deletes the wallet, journal and other named volumes.** Do not run it as a routine stop command. If mnemonic display is lost during creation, the encrypted key may still exist: use the password-protected backup; the app never overwrites an existing wallet to retry creation.

## Transaction behavior and limits

- Sepolia chain ID `11155111` is checked before RPC operations. Amounts use integer arithmetic; no floating-point currency math.
- ETH and token transactions use RPC gas estimation; failed estimation stops the operation. A quote binds action, recipient/spender, contract, calldata, nonce and fee caps for 120 seconds. The server rechecks nonce, funds and relevant token conditions before signing; it never silently raises an approved cap.
- Signed raw bytes and hash are synced to disk **before** broadcast. Repeated sends of one quote return the same journal entry. Timeouts and ambiguous responses remain uncertain; they are not treated as proof that nothing was sent.
- One outstanding transaction at a time, 256 live quotes, 1,000 journal entries. A pending, dropped or underpriced transaction may block new transfers. Gas replacement/cancellation is not implemented; rebroadcast cannot raise the original fee. This is an explicit prototype limit.
- History refresh checks outstanding records and the latest 20 mined records for canonical receipt changes. Confirmations are observations, not finality. There is no continuous chain indexer or full reorg recovery. The activity index supports explicit block-range synchronization and hash import; it is separate from the outgoing transaction journal.
- A successful ERC-20 transaction receipt proves execution status; it does not by itself prove the recipient's economic balance change for fee-on-transfer, rebasing or malicious tokens.
- Wallet POST endpoints require a per-process CSRF token, exact-origin checks, allowed local Host, JSON media type and a bounded strict body. Restarting invalidates browser CSRF state; reload the page.
- No mainnet, account switching, token discovery, hardware signing, password changes or multi-user access. Do not expose port 8090 with a tunnel or reverse proxy.

## Sepolia exchange

- **ETH → WETH / WETH → ETH:** deposit/withdraw against Uniswap's published Sepolia WETH9 at `0xfff9976782d46cc05630d1f6ebab18b2324d6b14`. The conversion is 1:1; ETH gas is additional.
- **WETH ↔ test USDC:** exact-input, single-pool Uniswap V3 swaps through the published Sepolia SwapRouter02 (`0x3bFA4769FB09eefC5a80d6E87c3B9C650f7Ae48E`), using QuoterV2 and the factory's `getPool`. Circle's test USDC is `0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238`.
- Choose a pool fee, input amount and slippage. Approve only the required input amount first, wait for its receipt, then request the swap quote. Revoke an existing nonzero approval before changing it. Each step has its own preview and password confirmation.
- The signed router call binds the wallet recipient, exact input, minimum output, pool fee and an on-chain deadline. No arbitrary router, token pair, recipient or calldata is accepted. Failed simulation, unavailable pool, insufficient allowance/balance or zero output stops the operation.
- Default slippage is 0.5%; supported range is 0.01–5%. Quotes expire after at most 120 seconds. Testnet pool prices do not represent USD valuations. No bridge, aggregator, LP creation or arbitrary token swapping is implemented.

Read-only integration verification (no key or broadcast):

```sh
docker compose run --rm --no-deps \
  -e FLOWLEDGER_LIVE_RPC=https://ethereum-sepolia-rpc.publicnode.com \
  app go test ./internal/wallet -run TestSepoliaExchangeReadOnly -v -count=1
```

This checks deployed code, token metadata and live quotes. It does **not** claim a completed swap. References: [Uniswap Sepolia deployments](https://developers.uniswap.org/docs/protocols/v3/deployments/v3-ethereum-deployments), [Circle test USDC](https://developers.circle.com/stablecoins/usdc-contract-addresses).

## Receive and account for test assets

Copy the wallet's Sepolia address to receive ETH or ERC-20 tokens. In **收支流水**, sync the latest 20 blocks, enter a starting block to scan the next batch of 20, or import a known transaction hash. Event scans specify WETH/test USDC and tokens added in the current browser session (20 contracts maximum), because the default public RPC requires a contract address filter. Add another token before syncing its incoming events. Outgoing journal transactions are included automatically. Repeated imports/synchronization deduplicate by hash. `activity.json` holds public transaction hashes (up to 1,000); it never stores a pretend balance.

Each page re-fetches up to 20 receipts with bounded concurrency. The view checks transaction identity, signature, chain ID, canonical block hash, receipt status and event provenance before calculating movements. Failed transactions count only sender gas. Pending, unavailable and observed orphaned transactions contribute no amounts; the page flags incomplete results. Self-transfers show both legs, leaving only the fee as net ETH change. Approval is not a token expenditure.

Supported evidence: direct transaction ETH value, ERC-20 `Transfer` logs, and the allowlisted WETH9 `Deposit`/`Withdrawal` logs. General internal ETH calls, rebasing, fee-on-transfer semantics and historical ranges that have not been scanned may not be fully represented. Token events are evidence of emitted logs, not an independent balance reconciliation or financial audit.

Page totals cover **only that page's verified transactions**, and are grouped by contract address rather than token symbol. CSV exports that page's raw integer amounts, asset addresses, hashes and evidence, including unverified rows without amounts. Import raw amount columns as text in spreadsheets to preserve every digit. The activity view is a test-asset cash-flow record, not a full double-entry accounting system or the current wallet balance. Ordering follows insertion into the index; verified block timestamps are shown separately.

## Verification

```sh
docker compose run --rm --no-deps app go test -race ./...
docker compose run --rm --no-deps app go vet ./...
```

Tests use temporary wallets, published mnemonic fixtures and local mock RPCs. They cover derivation/restore, exact units, keystore encryption, input guards, ETH/token signed payloads, finite allowance/revocation, changed chain/nonce/funds, concurrent duplicate submissions, persistence failure, restart/rebroadcast identity and HTTP origin/CSRF restrictions. Test KDF parameters are deliberately reduced; runtime uses geth standard parameters.

**Mock tests are not Sepolia evidence.** A real end-to-end acceptance requires a user-created/funded test wallet and an actual transaction hash with a Sepolia receipt. No such outgoing transaction is claimed solely because tests pass. Existing read-only Sepolia lookups and UI fixture checks are separate evidence.

## Code and references

- `cmd/flowledger`: startup, configuration, shutdown.
- `internal/wallet`: mnemonic/keystore, exact amounts, ERC-20 ABI, quotes, signing, journal.
- `internal/chain`: Sepolia RPC and receipt checks.
- `internal/web`: HTTP guards and embedded vanilla HTML/CSS/JS.

Go module: `github.com/a861252012/flowledger`. No frontend framework or Node build is required.

Design guidance: [UI UX Pro Max](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill/tree/7f69fed6a2717900085f1bc3b263721f8ba025e2). Go and Web3 implementation guidance: [wshobson/agents](https://github.com/wshobson/agents/tree/a30778f8c4e6b0a87567941b7cca4f534bf642b6), selectively applied; these references are not security certifications.

Standards: [BIP-39](https://github.com/bitcoin/bips/blob/master/bip-0039.mediawiki), [BIP-44](https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki), [EIP-1559](https://eips.ethereum.org/EIPS/eip-1559), [ERC-20](https://eips.ethereum.org/EIPS/eip-20).
