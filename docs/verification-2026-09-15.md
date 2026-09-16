# FlowLedger verification — 2026-09-15

Baseline: `a1631c58c8e5c0a1518cd3920f1e489de02ab8ce`. These changes are local and have not been committed or pushed.

## Implemented

- Same-nonce speed-up/cancel with a new fee preview, password confirmation, retained original journal records, and nonce-group receipt resolution. A replacement broadcast is not a cancellation guarantee.
- V3 scrypt keystore import, bounded cryptographic input validation, and password change; original passwords remain necessary for old backups.
- Up to 20 independent accounts, duplicate signing-address rejection, Ethereum Sepolia / Arbitrum Sepolia selection, isolated journals/quotes, shared key within each account across networks.
- Configurable RPC fallback with per-endpoint chain checks, identical broadcast bytes, bounded retries, and retention of the last healthy endpoint.
- Background canonical receipt checks for unfinalized records, rotating refresh order, RPC-observed finality, and archive-before-compaction at 900 active records. Recent 100 records are retained. Insufficient finalized records can still reach the 1,000 active-record limit.
- Opt-in finalized-block scanning, saved cursor, receipt-based token discovery, explicit scan errors and pause/resume. No claim that unscanned history is complete.
- Replaced the unsafe acceptance script: no default password, explicit `--send`, hidden password input, and a strictly non-broadcasting `--test-quote` mode.

## Isolated Go verification

The source/module mounts are read-only. Only a disposable build cache is writable; the runtime wallet volume is not mounted. Network is disabled.

```sh
docker run --rm --network none \
  -v "$PWD:/app:ro" \
  -v flowledger_go_modules:/go/pkg/mod:ro \
  -v /private/tmp/flowledger-development-cache:/tmp/review-cache \
  -e GOCACHE=/tmp/review-cache \
  flowledger-app sh -c 'go test -race -count=1 ./... && go vet ./...'
```

Exit code: 0. Output:

```text
?   	github.com/a861252012/flowledger/cmd/flowledger	[no test files]
ok  	github.com/a861252012/flowledger/internal/chain	1.099s
ok  	github.com/a861252012/flowledger/internal/wallet	5.403s
ok  	github.com/a861252012/flowledger/internal/web	1.088s

```

Additional checks: `node --check` for both frontend scripts, `git diff --check`, and four isolated Python acceptance-script tests passed. Race detector success only describes executed memory-access paths, not the absence of every logical race.

New regression tests cover same-nonce intent/fee preservation on both networks, stale replacement quotes, reorg reopening, account/network isolation, duplicate addresses, backup/password round-trips, malformed KDF/IV data, archive crash ordering, scan cursor persistence, wrong-network fallback, and CSRF requirements.

## Live read-only evidence

- Ethereum Sepolia RPC: chain ID 11155111, block 11708057.
- Arbitrum Sepolia RPC: chain ID 421614, block 309056668.
- Original address preserved: `0x219465ECA5EB591b7587E4B1Aa97F34971206Ab9`.
- Browser showed 0.05 ETH on Ethereum Sepolia and 0 ETH on Arbitrum Sepolia.
- The automatic scanner processed blocks 11697064–11697066 and recovered the earlier faucet receipt as succeeded. It was paused after this bounded acceptance check; saved next block: 11697067.
- Incoming faucet transaction: https://sepolia.etherscan.io/tx/0xe2e6476a377737f2b4324e99a0c97ed97c1c97034eee9ec8d94a928d24b8dfde

No outgoing signed transaction, completed token transfer, or swap is claimed by this report. A wallet-password entry and actual testnet receipts are still needed for those end-to-end acceptance checks.

## Boundaries

Uniswap exchange contracts remain Ethereum Sepolia only. No mainnet, bridge, hardware signing, or generic internal-ETH tracing. Finality relies on correctly behaving RPC/consensus. Historical syncing starts at the displayed configured block, and is not an independent blockchain audit. Account workers are loaded when the account is visited after startup. Do not delete the wallet volume or archives.

## Exchange follow-up

Added optional spender/allowance fields to the existing token lookup and an Exchange preflight button. The UI compares integer token units and explains insufficient balance, allowance reset, initial approval, or readiness to quote. Signing still performs its own authoritative checks.

`go test -race -count=1 ./... && go vet ./...` passed again in the same network-disabled disposable test container (chain 1.104s, wallet 5.644s, web 1.102s). The allowance regression checks formatting, invalid spender rejection, unchanged ordinary lookup, and no broadcast. JavaScript syntax and diff whitespace checks passed.

Read-only live check (no wallet volume or credentials mounted):

```sh
docker run --rm -v "$PWD:/app:ro" \
 -v flowledger_go_modules:/go/pkg/mod:ro \
 -v /private/tmp/flowledger-development-cache:/tmp/review-cache \
 -e GOCACHE=/tmp/review-cache \
 -e FLOWLEDGER_LIVE_RPC=https://ethereum-sepolia-rpc.publicnode.com \
 flowledger-app go test -count=1 -run '^TestSepoliaExchangeReadOnly$' -v ./internal/wallet
```

PASS (7.299s). All four fee tiers returned nonzero quotes for 0.000001 WETH: 100 -> 0.028927 USDC, 500 -> 0.028665, 3000 -> 0.02883, 10000 -> 0.028447. These are historical testnet quotes, not executable guarantees or dollar valuations. Outgoing swap receipt acceptance remains pending local wallet signing.

## Final expansion verification

The later working-tree expansion adds Base/OP Sepolia, guided Exchange, observation/diagnostics, QR/address book, showcase, CI definition, and independent native SOL on Devnet. See `review-handoff-2026-09-15.md` for scope and explicit exclusions.

Final Go command (source/modules read-only, no runtime wallet, network disabled):

```sh
docker run --rm --network none \
 -v "$PWD:/app:ro" -v flowledger_go_modules:/go/pkg/mod:ro \
 -v /private/tmp/flowledger-development-cache:/tmp/review-cache \
 -e GOCACHE=/tmp/review-cache \
 flowledger-app sh -c 'go test -race -count=1 ./... && go vet ./... && test -z "$(gofmt -l cmd internal)"'
```

Exit 0:

```text
?   github.com/a861252012/flowledger/cmd/flowledger [no test files]
ok  github.com/a861252012/flowledger/internal/chain 1.098s
ok  github.com/a861252012/flowledger/internal/wallet 6.160s
ok  github.com/a861252012/flowledger/internal/web 1.079s
```

Local browser command used the desktop's bundled **Playwright 1.62.1** via `NODE_PATH` pointing to its installed `node_modules`, then `node tests/browser/browser.cjs`. The committed test package/CI definition pins 1.58.2; that exact version has not been run locally. All requests outside the disposable fixture server were aborted. No local wallet state or external RPC was accessed. Exit 0:

```text
PASS: four EVM networks, contacts, observation, RPC failures, pool selection, both exchange workflows, lost-response reload recovery, history search, diagnostics, Solana create/quote/send/finalized UI, mobile layout (mock APIs only).
```

The response-loss fixture returns HTTP 502 after recording a broadcast, then reloads and verifies recovery through the quote ID. An earlier attempt to simulate loss by destroying the socket was replaced because browser transport retries made the fixture nondeterministic. The test also exposed and fixed a guide-stop button that remained disabled after sending finished.

All frontend JavaScript passed `node --check`; four Python tests passed with `python3 -m unittest discover -s scripts -p 'test_*.py'`; `git diff --check` passed. CI is defined but no remote execution, coverage percentage, or absence of all logical races is claimed.

Live Base/OP read-only checks passed; snapshots are in the handoff. The isolated Solana Devnet outgoing test **SKIPPED** because the faucet did not provide funds. No outgoing Solana signature was obtained. Ethereum outgoing ETH/ERC-20/approval/swap acceptance and a complete on-chain demonstration video remain incomplete. The original wallet password must be entered locally for signing; it was not requested in chat, read, or guessed.

Migration note (2026-09-16): the Python test result above is a historical
observation. Those scripts have been replaced by `cmd/send-and-verify` and
`cmd/verify-onchain-evidence`; current checks run through `go test ./...`.
