# Polygon Amoy / TRON Shasta — implementation and verification

> Historical implementation snapshot. Subsequent TRX and TRC-20 sends, expiry recovery, and current limits are recorded in [live acceptance](onchain-acceptance-2026-09-15.md). Statements below about an uncreated TRON wallet and no outgoing Shasta transaction applied before that run.

Date: 2026-09-15 (Asia/Taipei). Review the current working tree, not only HEAD `a1631c58c8e5c0a1518cd3920f1e489de02ab8ce`. This batch is not committed or pushed. Earlier uncommitted multi-network work remains present.

## Delivered

- Polygon Amoy (`80002`) wired into Docker, account/network routing, network selection, native POL and ERC-20 send/approve, existing EVM replacement/cancel, receipt activity and CSV. Gas, balance and receipt amounts render as POL. Mainnet `137` is rejected; no Amoy DEX is configured.
- TRON Shasta (`/tron/`) with independent BIP-44 coin-type 195 account, Keystore V3 backup/restore/password change, receive address and QR, faucet link, TRX balance and Bandwidth/Energy queries, custom TRC-20 metadata/balance, bound transfer quote, local protobuf construction and signing, durable pre-broadcast journal, identical-byte retry, solidified receipt status and actual fees.
- Sender password and seed material stay local. No private key, mnemonic or provider credential is logged or included in this report. Browser contract tests use only fixture passwords/mnemonics and a disposable browser profile.

## Relevant code

- `cmd/flowledger/main.go`, `compose.yaml`, `.env.example`: routes, endpoints, storage directories.
- `internal/chain/{chain,fallback,activity}.go`: Amoy allowlist, native currency and receipt-derived POL movements.
- `internal/wallet/{service,replacement,keystore}.go`: native quote labels and reused BIP-39/BIP-32/Keystore implementation. Ethereum derivation remains coin type 60; TRON explicitly selects 195.
- `internal/wallet/tron_rpc.go`: genesis check, Base58Check, constant calls, typed local wire construction based on the TRON protobuf schema. No arbitrary node-created bytes are signed.
- `internal/wallet/tron.go`: quote, local signing, durable broadcast/retry and solidified receipts. TRC-20 `false` return is not reported as a successful transfer.
- `internal/web/{tron.go,templates/tron.html,static/tron.js}`: wallet UI and local Host/Origin/CSRF boundary.
- `internal/wallet/{polygon_test,tron_test,tron_live_test}.go`, `internal/web/tron_test.go`, `internal/chain/{activity_test,network_live_test}.go`, `tests/browser/browser.cjs`.

## Executed verification

Go used a separate disposable Docker container, `--network none`, no runtime wallet volume. Source was mounted read/write for the preceding `gofmt`; source isolation should not be confused with a read-only mount. Test wallet data uses `t.TempDir`, and the build cache was reused.

```sh
docker run --rm --network none \
  -v "$PWD:/app" \
  -v flowledger_go_modules:/go/pkg/mod:ro \
  -v /private/tmp/flowledger-development-cache:/tmp/review-cache \
  -e GOCACHE=/tmp/review-cache flowledger-app sh -c \
  'gofmt -w internal && go test -race -count=1 ./... && go vet ./... && test -z "$(gofmt -l cmd internal)"'
```

Final output, exit 0:

```text
?   github.com/a861252012/flowledger/cmd/flowledger [no test files]
ok  github.com/a861252012/flowledger/internal/chain  1.104s
ok  github.com/a861252012/flowledger/internal/wallet 6.494s
ok  github.com/a861252012/flowledger/internal/web    1.073s
```

Tests cover an official Shasta unsigned transaction/raw-hash vector, TRON address checksum and derivation, persisted signatures, restart, same-quote deduplication, exact-byte retry, wrong network/password, expired quotes/raw transactions, disk failure before broadcast, backup restoration, TRC-20 simulation/receipt false-return handling, POL journal/quote currency, Amoy signed chain ID/value, and foreign Host/Origin/missing-CSRF rejection. Race Detector results apply to executed paths; they are not proof of absence of logical races.

Browser command (temporary Chromium; all off-origin requests aborted):

```sh
NODE_PATH=/Users/a861252012/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules \
  node tests/browser/browser.cjs
```

Exit 0:

```text
PASS: five EVM networks including POL, contacts, observation, RPC failures, pool selection, both exchange workflows, lost-response reload recovery, history search, diagnostics, Solana and TRON create/quote/send/finalized UI, TRC20 query, mobile layout (mock APIs only).
```

The bundled Playwright is 1.62.1; CI pins 1.58.2, so that exact CI browser build was not run here. Chromium required a sandbox escalation for macOS Mach-port access; the approved run used the fixture server and disposable profile. `node --check` passed for changed scripts; `git diff --check` passed. A subsequent visual check found and fixed an empty TRON error box with the existing CSS `:empty` convention.

## Real network reads (no signing or broadcast)

Commands used a separate container with source/modules read-only, no wallet volume, network enabled, and the same disposable build cache:

```sh
# Prefix each command with the Docker mount arguments above, changing /app to :ro,
# omitting --network none, and setting the indicated environment flag.
FLOWLEDGER_LIVE_NETWORKS=1 go test -run '^TestAdditionalNetworksReadOnly$/80002$' -v -count=1 ./internal/chain
FLOWLEDGER_LIVE_TRON=1 go test -run '^TestTronShastaReadOnly$' -v -count=1 ./internal/wallet
```

```text
chain=80002 block=47636038 finalized=47636039 balanceWei=0 rollupFeeEstimateWei=0; no signing or broadcast
PASS (3.370s)
Shasta genesis verified; public-address balance/resources=map[active:true address:TUEZSdKsoDHQMeZwihtdoBiN46zxhGWYdH bandwidth:600 energy:154 trx:0]; no signing or broadcast
PASS (1.505s)
```

The Amoy latest and finalized observations are separate requests; a new block arrived between them. They are not a single atomic snapshot. The Shasta address is a **public derivation test vector**, not a runtime wallet or an address to fund.

Shasta `/wallet/createtransaction` also produced a public unsigned transfer fixture (1 SUN). The test verifies raw protobuf bytes and SHA-256 ID `3a894f8f31a6db67d75cca83ee0aa1a31aba1044d0a786042797879d685a0963`. This is an **unsigned fixture ID, not an on-chain transfer receipt**.

## Runtime and limits for reviewers

- `docker compose up -d --no-deps app` activated the new routes and environment. No volume deletion, key replacement, database cleanup, commit or push was performed.
- Actual browser navigation confirmed Polygon Amoy/POL and TRON Shasta setup pages. Existing EVM account remained available. TRON runtime account has not been created; setup is left for the wallet holder.
- No real outgoing Amoy or Shasta transaction was completed. TRC-20 behavior was tested with mocks, not a deployed Shasta contract. Faucet funding and local wallet-holder signing are still required for transaction-hash/receipt evidence. Do not describe this as completed end-to-end chain acceptance.
- TRON history is local outgoing history, not a complete incoming-payment indexer. One in-flight transaction, 1,000 record limit; unknown expired broadcasts can remain blocked pending investigation. No automatic journal deletion.
- TRON quote resource reserves are estimates. Fee prices/resources can change; `fee_limit` only limits Energy. Provider availability and solidification are reported by a single configured RPC, without independent quorum.
- No TRON DEX/staking/multisig/fee delegation or Amoy swap is implemented. Existing Exchange is Ethereum Sepolia only. Solana still supports native SOL only.
- The broader portfolio's real outgoing Ethereum/Solana acceptance and three-minute demo recording remain outstanding; this network addition does not close those earlier gaps.

For an independent review, run `go test -race -count=1 ./...` and `go vet ./...` in a disposable `--network none` container with `/app:ro`, a separate `GOCACHE`, and **no runtime wallet volume**. Do not use `docker compose down -v`.
