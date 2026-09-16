# FlowLedger reviewer handoff — 2026-09-15 expansion

Review the **working tree**, not only remote main. Baseline commit: `a1631c58c8e5c0a1518cd3920f1e489de02ab8ce`. No new commit or push is claimed. Earlier verification notes describe an earlier subset of this working tree.

## Implemented scope

- Five EVM testnets: Ethereum/Arbitrum/Base/OP Sepolia and Polygon Amoy, same account key with independent per-chain quotes/journals. Mainnet IDs rejected.
- OP Stack fees: separate data/operator reserve, send-time recheck, full receipt fee reconstruction.
- Sepolia exchange: four-pool gross-output comparison, explicit partial failures, ETH↔USDC guided steps, finite allowance, receipt-derived unwrap amount, quote-ID recovery after a lost broadcast response.
- QR, browser-local address book, labels, history search, receipt details, public-address observation and RPC transport diagnostics.
- Independent Solana Devnet native SOL wallet: hardened Ed25519 derivation, AES-GCM/scrypt encrypted seed, mnemonic/encrypted restore, password changes, bound fee/blockhash quotes, signed-byte journal, exact rebroadcast, finalized status.
- Independent TRON Shasta TRX/TRC-20 wallet with locally constructed transaction bytes and Solidity-node receipt checks. See [TRON scope](polygon-tron-2026-09-15.md) and [later acceptance](onchain-acceptance-2026-09-15.md).
- Showcase page, architecture/acceptance documentation, isolated browser tests, CI definition.

## Reviewer focus

1. Trace each chain's fees from quote to balance check and confirmation text. OP L1 costs have no enforceable type-2 cap. Historical operator queries must use the receipt block hash.
2. Reproduce response loss after an Exchange step was actually journaled. Reloading must find the quote ID and preserve the operation. Ending a guide must not delete the journal.
3. Check that USDC→ETH unwraps actual verified WETH receipts, not expected output or a pre-existing balance.
4. Verify observation routes have no key/journal mutation and arbitrary address labels cannot inject HTML.
5. Verify Solana mainnet rejection, expired blockhash rejection, Ed25519 signature verification, key-restore identity, record-before-broadcast, and retry identity. Review its separate limitations rather than assuming EVM replacement/archive guarantees apply.
6. Preserve wallet_data. Do not run `docker compose down -v`. Do not read/log passwords, seed phrases or encrypted key files unnecessarily.
7. Distinguish same-quote deduplication from upstream business-payment identity. Repeated `quoteID` requests already return the existing record; do not report ordinary same-quote HTTP retries as generating new nonces.
8. Distinguish a returned broadcast error from a stopped process. Initial journal state is `pending`; `broadcast_unknown` requires a subsequent update. Use the [recovery evidence matrix](demo-script.md#recovery-evidence-matrix) to separate mock assertions, inferred crash behavior and missing real fault-injection acceptance.
9. Use actual symbols (`RollupFee`, `ReceiptFee`, `ArchiveFinalized`) and the [architecture boundaries and official references](architecture.md#precise-implementation-and-interview-boundaries). KMS/HSM is not implemented; do not describe a 65-byte signing-interface format as the universal EVM wire encoding.

## Reproducible checks

```sh
docker run --rm --network none \
 -v "$PWD:/app:ro" -v flowledger_go_modules:/go/pkg/mod:ro \
 -e GOCACHE=/tmp/review-cache \
 flowledger-app sh -c 'go test -race -count=1 ./... && go vet ./...'

npm ci --prefix tests/browser
(cd tests/browser && npx playwright install chromium && npm test)
go test ./cmd/send-and-verify ./cmd/verify-onchain-evidence
git diff --check
```

Race detector success concerns executed memory accesses; it is not proof of absence of logical races. Browser fixtures are not blockchain execution evidence. Network-enabled Go tests are opt-in and separate from normal CI. GitHub Actions is defined locally; no remote run is claimed.

## Current chain evidence and remaining acceptance

- The later [on-chain acceptance record](onchain-acceptance-2026-09-15.md) supersedes the earlier outgoing-transaction gap: it records 13 outgoing transactions across Ethereum/Base/OP/Arbitrum Sepolia and TRON Shasta. Product API execution and mock browser coverage are separately identified there.
- Solana Devnet and Polygon Amoy outgoing acceptance remains incomplete. Implemented signing paths are not equivalent to accepted public-chain transactions.
- Earlier read-only snapshots and the original account's incoming faucet transaction remain in `verification-2026-09-15.md`; they do not describe current balances. This documentation correction did not recheck live receipts or send new transactions.
- A three-minute storyboard and mock recovery command are available in `demo-script.md`. No recorded video, deterministic subprocess-kill test or real lost-response fault-injection run is claimed.

## Documentation correction verification — 2026-09-15

This correction changed README, architecture, demo and handoff documents only. The existing implementation was checked against the revised claims. Eight selected top-level tests passed, including both Base/OP fee subtests, with exit code 0:

```sh
docker run --rm --network none \
  -v "$PWD:/app:ro" -v flowledger_go_modules:/go/pkg/mod:ro \
  -v /private/tmp/flowledger-development-cache:/tmp/review-cache \
  -e GOCACHE=/tmp/review-cache flowledger-app \
  go test -race -count=1 -v ./internal/wallet ./internal/chain \
  -run '^(TestConcurrentSendSignsExactQuoteOnce|TestUnknownBroadcastRestartReusesRaw|TestSendConcurrentStateUpdatePreservesLatest.*|TestRetryConcurrentStateUpdatePreservesLatest.*|TestHistoryUpdatesReorgDetectedFromRPC|TestOPFeesIncludeDataAndHistoricalOperatorCharge)$'
```

```text
ok  github.com/a861252012/flowledger/internal/wallet  1.515s
ok  github.com/a861252012/flowledger/internal/chain   1.013s
```

The container used read-only source/modules, a reusable disposable build cache, temporary fixture wallets and no external network or live wallet-volume mount. `/private/tmp/flowledger-development-cache` is the local cache path used for this run; omit that mount to use an ephemeral container cache. The full test suite, browser tests, `go vet` and live receipts were not rerun for this documentation-only change. Local Markdown file links/code fences and `git diff --check` passed. No commit or push was performed.
