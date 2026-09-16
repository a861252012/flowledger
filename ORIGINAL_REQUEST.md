# Original User Request

## 2026-09-16T05:16:06Z

<USER_REQUEST>
Implement an ERC-4337 Account Abstraction (v0.6/v0.7 compatible) UserOperation construction, hashing, signing, and Bundler JSON-RPC client module for the FlowLedger Go codebase to demonstrate modern smart contract wallet integration capabilities.

Working directory: /Users/a861252012/Desktop/folder/code/flowledger
Integrity mode: development

## Requirements

### R1. UserOperation Data Structures and Encoding
Define the ERC-4337 UserOperation data structures with ABI encoding and decoding utilities. Support calculating the standardized UserOpHash bound to canonical EntryPoint addresses (e.g. EntryPoint v0.6 0x5FF137D4b0FDCD49DcA30c7CF57E578a026d2789) and Chain ID.

### R2. UserOperation Signer and Builder
Provide a builder to construct UserOperations from transactions or call data, calculate pre-verification gas and fee parameters, and sign the UserOpHash using FlowLedger's existing key management / private key derivation without exposing private keys.

### R3. Bundler JSON-RPC Client and Mocking
Implement a Bundler JSON-RPC client capable of calling `eth_sendUserOperation`, `eth_estimateUserOperationGas`, and `eth_getUserOperationReceipt`. Include mock RPC tests simulating bundler responses, verification failures, and execution receipts.

### R4. Test Coverage and Race Safety
All new packages and modules must pass `go test -race ./...` and `go vet ./...` with zero race conditions, zero lint errors, and explicit error handling adhering strictly to Go best practices and existing FlowLedger conventions (zero floating-point math, strict integer units).

## Acceptance Criteria

### Functional Completeness
- [ ] UserOperation struct supports all standard EIP-4337 fields (sender, nonce, initCode, callData, callGasLimit, verificationGasLimit, preVerificationGas, maxFeePerGas, maxPriorityFeePerGas, paymasterAndData, signature).
- [ ] UserOpHash computation matches the official EIP-4337 specification verified against official test vectors or known testnet hashes.
- [ ] Signing logic successfully produces valid 65-byte ECDSA secp256k1 signatures for UserOperations.
- [ ] Bundler client correctly marshals and unmarshals `eth_sendUserOperation` and `eth_getUserOperationReceipt` RPC calls.

### Code Quality and Verification
- [ ] `go test -race -v ./internal/wallet/...` passes with all existing and new unit tests.
- [ ] `go vet ./...` reports zero issues.
- [ ] Clean decoupling: does not break existing EOA signing, keystore, or transaction journal workflows.
</USER_REQUEST>
