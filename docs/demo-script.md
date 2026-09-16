# Three-minute demo storyboard

Show only a dedicated test wallet. Do not record setup phrases, backup passwords or signing-password entry.

- **0:00–0:35:** Open `/showcase`. Explain: Go local wallet, five EVM testnets plus independent Solana Devnet and TRON Shasta. Distinguish implemented networks from networks with recorded outgoing acceptance.
- **0:35–1:05:** Open read-only observation; query a public address. Switch to Base/OP and explain why balances and fees differ. A zero balance and an RPC error must appear differently.
- **1:05–1:50:** Open Exchange. Compare pools, inspect minimum output and fee. Show the ETH→USDC guide. Use previously completed signed transactions for the full recording; blur/pause before any password entry.
- **1:50–2:25:** Show a receipt and transaction diagnostics. Explain journal-before-broadcast, unknown outcomes, exact retry and same-nonce EVM replacement.
- **2:25–3:00:** Show Solana Devnet and the distinct blockhash expiry rule. Finish with CI/local validation artifacts and the exact acceptance limits.

Read-only footage must be labelled as such. A pool quote is not a completed swap. The [September 15 acceptance record](onchain-acceptance-2026-09-15.md) records 13 outgoing transactions across five testnets; Solana Devnet and Polygon Amoy outgoing acceptance remains incomplete. That record is historical evidence, not a fresh receipt check or proof of live fault recovery. Do not rewrite earlier validation results as if they covered later code.

## Recovery evidence matrix

These are distinct failure boundaries. Do not interrupt the running personal wallet to stage footage.

| Scenario | Expected behavior and evidence | Existing verification / limit |
|---|---|---|
| Repeated sends using the same quote | Return the original hash; no second signing/broadcast for that quote | `TestConcurrentSendSignsExactQuoteOnce`: four concurrent callers, one mock broadcast |
| RPC receives bytes and returns an ambiguous error | Persist `broadcast_unknown`; reopen the service and retry the exact bytes/hash | `TestUnknownBroadcastRestartReusesRaw`: mock RPC and service reopen, not an OS process kill or an actual chain receipt |
| A newer journal update arrives while broadcast is delayed | CAS retains the newer record and returns its state, including a reorg state | `TestSendConcurrentStateUpdatePreservesLatest*` and `TestRetryConcurrentStateUpdatePreservesLatest*`: channel-controlled mock broadcast with direct journal updates, not full History/receipt integration |
| Process stops after durable append but before broadcast-state update | A `pending` record may remain; RPC acceptance is unknown from that state alone | `TestProcessKillRestartReusesRaw`: SIGKILL after the local mock receives bytes, before its response; a new Service opens the released process lock, preserves nonce protection and retries identical bytes. This does not cover a kill before the request reaches the mock or power loss. |
| Receipt no longer matches the canonical block | Preserve uncertainty/reorg state and nonce protection | `TestHistoryUpdatesReorgDetectedFromRPC`: mock receipt/header mismatch, not a public-chain reorg |

For a reproducible local recovery segment, run from the repository root:

```sh
docker run --rm --network none \
  -v "$PWD:/app:ro" \
  -v flowledger_go_modules:/go/pkg/mod:ro \
  -e GOCACHE=/tmp/review-cache \
  flowledger-app go test -race -count=1 -v ./internal/wallet \
  -run '^(TestProcessKillRestartReusesRaw|TestConcurrentSendSignsExactQuoteOnce|TestUnknownBroadcastRestartReusesRaw|TestSendConcurrentStateUpdatePreservesLatest.*|TestRetryConcurrentStateUpdatePreservesLatest.*|TestHistoryUpdatesReorgDetectedFromRPC)$'
```

This container has no external network and no live wallet volume. Tests use temporary wallet directories and mock RPCs. A passing result covers the assertions above, not power-loss durability or blockchain execution. Do not display fixture signed bytes or credentials in a recording.

Suggested three-minute recovery narration: introduce the payment intent and same-quote protection (30 seconds); show the ambiguous-broadcast/reopen/exact-byte assertions (60 seconds); explain CAS and legal reorg updates (45 seconds); show a separately labelled historical explorer receipt and the tested process-kill boundary and its power-loss limitation (45 seconds). A real lost-response demonstration would require a dedicated disposable service and a controlled proxy that forwards the transaction but drops the response. Simply cutting the network before forwarding does not prove the node accepted it. No completed recording or real lost-response run is claimed here.
