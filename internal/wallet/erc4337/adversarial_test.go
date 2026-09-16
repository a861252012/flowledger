package erc4337

import (
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// buildValidTestOp 建構一組合法基礎之 UserOperation 測試物件
func buildValidTestOp() *UserOperation {
	return &UserOperation{
		Sender:               common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Nonce:                big.NewInt(42),
		InitCode:             []byte{},
		CallData:             []byte{0xde, 0xad, 0xbe, 0xef},
		CallGasLimit:         big.NewInt(50000),
		VerificationGasLimit: big.NewInt(100000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(2000000000),
		MaxPriorityFeePerGas: big.NewInt(1000000000),
		PaymasterAndData:     []byte{},
		Signature:            []byte{},
	}
}

// independentPackUint256Word 獨立 Oracle 工具：將 big.Int 寫入 32 位元組大端序陣列
func independentPackUint256Word(val *big.Int) [32]byte {
	var out [32]byte
	if val == nil || val.Sign() <= 0 {
		return out
	}
	bytes := val.Bytes()
	if len(bytes) > 32 {
		return out
	}
	copy(out[32-len(bytes):32], bytes)
	return out
}

// independentCalculateUserOpHashV06 獨立 Oracle：以原生純位元組方式獨立計算 v0.6 UserOpHash
func independentCalculateUserOpHashV06(op *UserOperation, entryPoint common.Address, chainID *big.Int) common.Hash {
	var innerBuf [320]byte

	// Word 0: sender (左補 12 位元組 0x00)
	copy(innerBuf[12:32], op.Sender.Bytes())

	// Word 1: nonce
	nonceWord := independentPackUint256Word(op.Nonce)
	copy(innerBuf[32:64], nonceWord[:])

	// Word 2: keccak256(initCode)
	hashInitCode := crypto.Keccak256(op.InitCode)
	copy(innerBuf[64:96], hashInitCode)

	// Word 3: keccak256(callData)
	hashCallData := crypto.Keccak256(op.CallData)
	copy(innerBuf[96:128], hashCallData)

	// Word 4: callGasLimit
	callGasWord := independentPackUint256Word(op.CallGasLimit)
	copy(innerBuf[128:160], callGasWord[:])

	// Word 5: verificationGasLimit
	verGasWord := independentPackUint256Word(op.VerificationGasLimit)
	copy(innerBuf[160:192], verGasWord[:])

	// Word 6: preVerificationGas
	preVerGasWord := independentPackUint256Word(op.PreVerificationGas)
	copy(innerBuf[192:224], preVerGasWord[:])

	// Word 7: maxFeePerGas
	maxFeeWord := independentPackUint256Word(op.MaxFeePerGas)
	copy(innerBuf[224:256], maxFeeWord[:])

	// Word 8: maxPriorityFeePerGas
	maxPrioWord := independentPackUint256Word(op.MaxPriorityFeePerGas)
	copy(innerBuf[256:288], maxPrioWord[:])

	// Word 9: keccak256(paymasterAndData)
	hashPaymaster := crypto.Keccak256(op.PaymasterAndData)
	copy(innerBuf[288:320], hashPaymaster)

	innerHash := crypto.Keccak256Hash(innerBuf[:])

	var outerBuf [96]byte
	copy(outerBuf[0:32], innerHash.Bytes())
	copy(outerBuf[44:64], entryPoint.Bytes())
	chainIDWord := independentPackUint256Word(chainID)
	copy(outerBuf[64:96], chainIDWord[:])

	return crypto.Keccak256Hash(outerBuf[:])
}

// independentCalculateUserOpHashV07 獨立 Oracle：以原生純位元組方式獨立計算 v0.7 PackedUserOpHash
func independentCalculateUserOpHashV07(op *PackedUserOperation, entryPoint common.Address, chainID *big.Int) common.Hash {
	var innerBuf [256]byte

	// Word 0: sender
	copy(innerBuf[12:32], op.Sender.Bytes())

	// Word 1: nonce
	nonceWord := independentPackUint256Word(op.Nonce)
	copy(innerBuf[32:64], nonceWord[:])

	// Word 2: keccak256(initCode)
	hashInitCode := crypto.Keccak256(op.InitCode)
	copy(innerBuf[64:96], hashInitCode)

	// Word 3: keccak256(callData)
	hashCallData := crypto.Keccak256(op.CallData)
	copy(innerBuf[96:128], hashCallData)

	// Word 4: accountGasLimits
	copy(innerBuf[128:160], op.AccountGasLimits[:])

	// Word 5: preVerificationGas
	preVerWord := independentPackUint256Word(op.PreVerificationGas)
	copy(innerBuf[160:192], preVerWord[:])

	// Word 6: gasFees
	copy(innerBuf[192:224], op.GasFees[:])

	// Word 7: keccak256(paymasterAndData)
	hashPaymaster := crypto.Keccak256(op.PaymasterAndData)
	copy(innerBuf[224:256], hashPaymaster)

	innerHash := crypto.Keccak256Hash(innerBuf[:])

	var outerBuf [96]byte
	copy(outerBuf[0:32], innerHash.Bytes())
	copy(outerBuf[44:64], entryPoint.Bytes())
	chainIDWord := independentPackUint256Word(chainID)
	copy(outerBuf[64:96], chainIDWord[:])

	return crypto.Keccak256Hash(outerBuf[:])
}

// TestAdversarial_IndependentOracleVerification 透過自建獨立 Oracle 驗證 UserOpHash 計算正確性
func TestAdversarial_IndependentOracleVerification(t *testing.T) {
	t.Parallel()

	op := buildValidTestOp()
	chainID := big.NewInt(11155111)

	// 1. 驗證 v0.6 透過獨立 Oracle 與實作完全一致
	oracleHashV06 := independentCalculateUserOpHashV06(op, CanonicalEntryPointV06, chainID)
	actualHashV06, err := GetUserOpHashV06(op, CanonicalEntryPointV06, chainID)
	if err != nil {
		t.Fatalf("GetUserOpHashV06 計算失敗: %v", err)
	}
	if actualHashV06 != oracleHashV06 {
		t.Fatalf("v0.6 雜湊與獨立 Oracle 計算不符: 預期 %s, 實際 %s", oracleHashV06.Hex(), actualHashV06.Hex())
	}

	// 2. 驗證智慧路由 GetUserOpHash 在 v0.6 下輸出與 Oracle 完全一致
	routedHashV06, err := GetUserOpHash(op, CanonicalEntryPointV06, chainID)
	if err != nil {
		t.Fatalf("GetUserOpHash 路由失敗: %v", err)
	}
	if routedHashV06 != oracleHashV06 {
		t.Fatalf("智慧路由 v0.6 輸出與 Oracle 不符")
	}

	// 3. 驗證 v0.7 透過獨立 Oracle 與實作完全一致
	packedOp, err := op.ToPacked()
	if err != nil {
		t.Fatalf("op.ToPacked 失敗: %v", err)
	}
	oracleHashV07 := independentCalculateUserOpHashV07(packedOp, CanonicalEntryPointV07, chainID)
	actualHashV07, err := GetUserOpHashV07(packedOp, CanonicalEntryPointV07, chainID)
	if err != nil {
		t.Fatalf("GetUserOpHashV07 計算失敗: %v", err)
	}
	if actualHashV07 != oracleHashV07 {
		t.Fatalf("v0.7 雜湊與獨立 Oracle 計算不符: 預期 %s, 實際 %s", oracleHashV07.Hex(), actualHashV07.Hex())
	}

	// 4. 驗證智慧路由 GetUserOpHash 在 v0.7 下輸出與 Oracle 完全一致
	routedHashV07, err := GetUserOpHash(op, CanonicalEntryPointV07, chainID)
	if err != nil {
		t.Fatalf("GetUserOpHash 路由 v0.7 失敗: %v", err)
	}
	if routedHashV07 != oracleHashV07 {
		t.Fatalf("智慧路由 v0.7 輸出與 Oracle 不符")
	}
}

// TestAdversarial_CrossChainReplayProtection 驗證跨鏈防重放隔離矩陣
func TestAdversarial_CrossChainReplayProtection(t *testing.T) {
	t.Parallel()

	op := buildValidTestOp()
	targetChains := []*big.Int{
		big.NewInt(1),        // Ethereum Mainnet
		big.NewInt(5),        // Goerli
		big.NewInt(11155111), // Sepolia
		big.NewInt(137),      // Polygon PoS
		big.NewInt(10),       // Optimism
		big.NewInt(42161),    // Arbitrum One
		big.NewInt(8453),     // Base
		big.NewInt(56),       // BNB Smart Chain
		big.NewInt(43114),    // Avalanche C-Chain
		new(big.Int).SetBytes([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}), // 巨大 Chain ID
	}

	// 檢驗 v0.6 與 v0.7 下所有鏈產生的 UserOpHash 必然完全互斥
	seenHashesV06 := make(map[common.Hash]int64)
	seenHashesV07 := make(map[common.Hash]int64)

	for _, chainID := range targetChains {
		// v0.6 測試
		hashV06, err := GetUserOpHash(op, CanonicalEntryPointV06, chainID)
		if err != nil {
			t.Fatalf("Chain ID %s 下計算 v0.6 UserOpHash 失敗: %v", chainID.String(), err)
		}
		if previousChain, exists := seenHashesV06[hashV06]; exists {
			t.Fatalf("嚴重漏洞: 跨鏈防重放失效! Chain ID %d 與 %s 產出相同之 v0.6 Hash: %s", previousChain, chainID.String(), hashV06.Hex())
		}
		seenHashesV06[hashV06] = chainID.Int64()

		// v0.7 測試
		hashV07, err := GetUserOpHash(op, CanonicalEntryPointV07, chainID)
		if err != nil {
			t.Fatalf("Chain ID %s 下計算 v0.7 UserOpHash 失敗: %v", chainID.String(), err)
		}
		if previousChain, exists := seenHashesV07[hashV07]; exists {
			t.Fatalf("嚴重漏洞: 跨鏈防重放失效! Chain ID %d 與 %s 產出相同之 v0.7 Hash: %s", previousChain, chainID.String(), hashV07.Hex())
		}
		seenHashesV07[hashV07] = chainID.Int64()

		// 驗證同鏈下 v0.6 與 v0.7 雜湊亦完全隔離
		if hashV06 == hashV07 {
			t.Fatalf("嚴重漏洞: 同鏈 ID %s 下 v0.6 與 v0.7 產出完全相同之 Hash: %s", chainID.String(), hashV06.Hex())
		}
	}

	if len(seenHashesV06) != len(targetChains) {
		t.Fatalf("v0.6 跨鏈雜湊唯一性集合大小不符: 預期 %d, 實際 %d", len(targetChains), len(seenHashesV06))
	}
	if len(seenHashesV07) != len(targetChains) {
		t.Fatalf("v0.7 跨鏈雜湊唯一性集合大小不符: 預期 %d, 實際 %d", len(targetChains), len(seenHashesV07))
	}
}

// TestAdversarial_EntryPointIsolation 驗證 EntryPoint 地址隔離性
func TestAdversarial_EntryPointIsolation(t *testing.T) {
	t.Parallel()

	op := buildValidTestOp()
	chainID := big.NewInt(1)

	customEntryPoint1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	customEntryPoint2 := common.HexToAddress("0x9999999999999999999999999999999999999999")

	entryPoints := []common.Address{
		CanonicalEntryPointV06,
		CanonicalEntryPointV07,
		customEntryPoint1,
		customEntryPoint2,
	}

	seenHashes := make(map[common.Hash]common.Address)
	for _, ep := range entryPoints {
		hash, err := GetUserOpHash(op, ep, chainID)
		if err != nil {
			t.Fatalf("計算 EntryPoint %s 雜湊失敗: %v", ep.Hex(), err)
		}
		if previousEP, exists := seenHashes[hash]; exists {
			t.Fatalf("嚴重漏洞: EntryPoint 隔離失效! EntryPoint %s 與 %s 產出相同之 Hash: %s", previousEP.Hex(), ep.Hex(), hash.Hex())
		}
		seenHashes[hash] = ep
	}

	// 額外驗證：若手動以 GetUserOpHashV06 傳入 CanonicalEntryPointV07，其外層 Hash 與 CanonicalEntryPointV06 亦必然互斥
	hashForcedV06WithV07Addr, err := GetUserOpHashV06(op, CanonicalEntryPointV07, chainID)
	if err != nil {
		t.Fatalf("強制 v0.6 計算失敗: %v", err)
	}
	hashCanonicalV06, _ := GetUserOpHashV06(op, CanonicalEntryPointV06, chainID)
	if hashForcedV06WithV07Addr == hashCanonicalV06 {
		t.Fatalf("手動傳入不同 EntryPoint 地址時外層 Hash 未能正確隔離")
	}
}

// TestAdversarial_InvalidChainIDDefenses 驗證負數、零、nil 及超長 Chain ID 防禦
func TestAdversarial_InvalidChainIDDefenses(t *testing.T) {
	t.Parallel()

	op := buildValidTestOp()
	entryPoint := CanonicalEntryPointV06

	negativeChainIDs := []*big.Int{
		big.NewInt(-1),
		big.NewInt(-100),
		big.NewInt(-11155111),
		new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 64)),
	}

	// 1. 測試負數 Chain ID
	for _, negChainID := range negativeChainIDs {
		_, err := GetUserOpHash(op, entryPoint, negChainID)
		if err != ErrInvalidChainID {
			t.Fatalf("負數 Chain ID (%s) 預期回傳 ErrInvalidChainID, 實際回傳: %v", negChainID.String(), err)
		}

		_, err = PackUserOpHashData(common.Hash{}, entryPoint, negChainID)
		if err != ErrInvalidChainID {
			t.Fatalf("PackUserOpHashData 負數 Chain ID 預期 ErrInvalidChainID, 實際回傳: %v", err)
		}

		_, err = PackOuterUserOpHash(common.Hash{}, entryPoint, negChainID)
		if err != ErrInvalidChainID {
			t.Fatalf("PackOuterUserOpHash 負數 Chain ID 預期 ErrInvalidChainID, 實際回傳: %v", err)
		}
	}

	// 2. 測試零 Chain ID
	zeroChainID := big.NewInt(0)
	_, err := GetUserOpHash(op, entryPoint, zeroChainID)
	if err != ErrInvalidChainID {
		t.Fatalf("零 Chain ID 預期回傳 ErrInvalidChainID, 實際回傳: %v", err)
	}

	// 3. 測試 nil Chain ID
	_, err = GetUserOpHash(op, entryPoint, nil)
	if err != ErrInvalidChainID {
		t.Fatalf("nil Chain ID 預期回傳 ErrInvalidChainID, 實際回傳: %v", err)
	}

	// 4. 測試超出 uint256 邊界 (2^256) 之 Chain ID
	overflowChainID := new(big.Int).Lsh(big.NewInt(1), 256)
	_, err = PackUserOpHashData(common.Hash{}, entryPoint, overflowChainID)
	if err != ErrUint256Overflow {
		t.Fatalf("超出 256 位元之 Chain ID 預期 ErrUint256Overflow, 實際回傳: %v", err)
	}

	_, err = PackOuterUserOpHash(common.Hash{}, entryPoint, overflowChainID)
	if err != ErrUint256Overflow {
		t.Fatalf("PackOuterUserOpHash 超出 256 位元之 Chain ID 預期 ErrUint256Overflow, 實際回傳: %v", err)
	}
}

// TestAdversarial_ZeroAddressDefenses 驗證零地址 EntryPoint 與 Sender 防禦
func TestAdversarial_ZeroAddressDefenses(t *testing.T) {
	t.Parallel()

	op := buildValidTestOp()
	chainID := big.NewInt(1)
	zeroAddr := common.Address{}

	// 1. 零地址 EntryPoint
	_, err := GetUserOpHash(op, zeroAddr, chainID)
	if err != ErrInvalidEntryPoint {
		t.Fatalf("零地址 EntryPoint 呼叫 GetUserOpHash 預期 ErrInvalidEntryPoint, 實際回傳: %v", err)
	}

	_, err = GetUserOpHashV06(op, zeroAddr, chainID)
	if err != ErrInvalidEntryPoint {
		t.Fatalf("零地址 EntryPoint 呼叫 GetUserOpHashV06 預期 ErrInvalidEntryPoint, 實際回傳: %v", err)
	}

	packedOp, _ := op.ToPacked()
	_, err = GetUserOpHashV07(packedOp, zeroAddr, chainID)
	if err != ErrInvalidEntryPoint {
		t.Fatalf("零地址 EntryPoint 呼叫 GetUserOpHashV07 預期 ErrInvalidEntryPoint, 實際回傳: %v", err)
	}

	_, err = PackUserOpHashData(common.Hash{}, zeroAddr, chainID)
	if err != ErrInvalidEntryPoint {
		t.Fatalf("零地址 EntryPoint 呼叫 PackUserOpHashData 預期 ErrInvalidEntryPoint, 實際回傳: %v", err)
	}

	_, err = PackOuterUserOpHash(common.Hash{}, zeroAddr, chainID)
	if err != ErrInvalidEntryPoint {
		t.Fatalf("零地址 EntryPoint 呼叫 PackOuterUserOpHash 預期 ErrInvalidEntryPoint, 實際回傳: %v", err)
	}

	// 2. 零地址 Sender
	badOp := op.Clone()
	badOp.Sender = zeroAddr
	if err := badOp.Validate(); err != ErrInvalidSender {
		t.Fatalf("零地址 Sender 呼叫 Validate 預期 ErrInvalidSender, 實際回傳: %v", err)
	}
	_, err = badOp.ToPacked()
	if err != ErrInvalidSender {
		t.Fatalf("零地址 Sender 呼叫 ToPacked 預期 ErrInvalidSender, 實際回傳: %v", err)
	}
	_, err = PackUserOp(badOp)
	if err != ErrInvalidSender {
		t.Fatalf("零地址 Sender 呼叫 PackUserOp 預期 ErrInvalidSender, 實際回傳: %v", err)
	}
	_, err = PackUserOpTuple(badOp)
	if err != ErrInvalidSender {
		t.Fatalf("零地址 Sender 呼叫 PackUserOpTuple 預期 ErrInvalidSender, 實際回傳: %v", err)
	}
}

// TestAdversarial_NilAndNegativeFieldDefenses 驗證數值欄位為 nil 或負數時之健全防禦
func TestAdversarial_NilAndNegativeFieldDefenses(t *testing.T) {
	t.Parallel()

	// 測試 nil 數值欄位
	nilCases := []struct {
		name   string
		mutate func(*UserOperation)
	}{
		{"nil Nonce", func(op *UserOperation) { op.Nonce = nil }},
		{"nil CallGasLimit", func(op *UserOperation) { op.CallGasLimit = nil }},
		{"nil VerificationGasLimit", func(op *UserOperation) { op.VerificationGasLimit = nil }},
		{"nil PreVerificationGas", func(op *UserOperation) { op.PreVerificationGas = nil }},
		{"nil MaxFeePerGas", func(op *UserOperation) { op.MaxFeePerGas = nil }},
		{"nil MaxPriorityFeePerGas", func(op *UserOperation) { op.MaxPriorityFeePerGas = nil }},
	}

	for _, tc := range nilCases {
		t.Run(tc.name, func(t *testing.T) {
			op := buildValidTestOp()
			tc.mutate(op)

			// 驗證 Validate 必須回傳 ErrNilField，絕不可 panic
			if err := op.Validate(); err != ErrNilField {
				t.Fatalf("%s 呼叫 Validate 預期 ErrNilField, 實際: %v", tc.name, err)
			}

			// 驗證 PackUserOp 攔截
			if _, err := PackUserOp(op); err != ErrNilField {
				t.Fatalf("%s 呼叫 PackUserOp 預期 ErrNilField, 實際: %v", tc.name, err)
			}

			// 驗證 GetUserOpHash 攔截
			if _, err := GetUserOpHash(op, CanonicalEntryPointV06, big.NewInt(1)); err != ErrNilField {
				t.Fatalf("%s 呼叫 GetUserOpHash 預期 ErrNilField, 實際: %v", tc.name, err)
			}
		})
	}

	// 測試負數數值欄位
	negCases := []struct {
		name   string
		mutate func(*UserOperation)
	}{
		{"negative Nonce", func(op *UserOperation) { op.Nonce = big.NewInt(-1) }},
		{"negative CallGasLimit", func(op *UserOperation) { op.CallGasLimit = big.NewInt(-1) }},
		{"negative VerificationGasLimit", func(op *UserOperation) { op.VerificationGasLimit = big.NewInt(-1) }},
		{"negative PreVerificationGas", func(op *UserOperation) { op.PreVerificationGas = big.NewInt(-1) }},
		{"negative MaxFeePerGas", func(op *UserOperation) { op.MaxFeePerGas = big.NewInt(-1) }},
		{"negative MaxPriorityFeePerGas", func(op *UserOperation) { op.MaxPriorityFeePerGas = big.NewInt(-1) }},
	}

	for _, tc := range negCases {
		t.Run(tc.name, func(t *testing.T) {
			op := buildValidTestOp()
			tc.mutate(op)

			// 驗證 Validate 必須回傳 ErrNegativeValue
			if err := op.Validate(); err != ErrNegativeValue {
				t.Fatalf("%s 呼叫 Validate 預期 ErrNegativeValue, 實際: %v", tc.name, err)
			}

			// 驗證 PackUserOp 攔截
			if _, err := PackUserOp(op); err != ErrNegativeValue {
				t.Fatalf("%s 呼叫 PackUserOp 預期 ErrNegativeValue, 實際: %v", tc.name, err)
			}
		})
	}
}

// TestAdversarial_V07PackingBoundaryStress 驗證 v0.7 邊界極限、uint128 溢位與 paymaster 長度校驗
func TestAdversarial_V07PackingBoundaryStress(t *testing.T) {
	t.Parallel()

	maxUint128Val := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	overflowUint128Val := new(big.Int).Lsh(big.NewInt(1), 128)

	// 1. uint128 臨界最大值打包測試 (2^128 - 1) 必須完全成功
	packedMax, err := PackUint128Pair(maxUint128Val, maxUint128Val)
	if err != nil {
		t.Fatalf("PackUint128Pair 最大值打包失敗: %v", err)
	}
	highUnpacked, lowUnpacked := UnpackUint128Pair(packedMax)
	if highUnpacked.Cmp(maxUint128Val) != 0 || lowUnpacked.Cmp(maxUint128Val) != 0 {
		t.Fatalf("UnpackUint128Pair 最大值解碼不符")
	}

	// 2. uint128 溢位測試 (2^128)
	_, err = PackUint128Pair(overflowUint128Val, big.NewInt(1))
	if err != ErrGasLimitOverflow {
		t.Fatalf("高位 uint128 溢位預期 ErrGasLimitOverflow, 實際: %v", err)
	}
	_, err = PackUint128Pair(big.NewInt(1), overflowUint128Val)
	if err != ErrGasLimitOverflow {
		t.Fatalf("低位 uint128 溢位預期 ErrGasLimitOverflow, 實際: %v", err)
	}

	// 3. ToPacked gasLimit 溢位
	opGasOverflow := buildValidTestOp()
	opGasOverflow.CallGasLimit = overflowUint128Val
	if _, err := opGasOverflow.ToPacked(); err != ErrGasLimitOverflow {
		t.Fatalf("CallGasLimit 溢位預期 ErrGasLimitOverflow, 實際: %v", err)
	}

	opVerGasOverflow := buildValidTestOp()
	opVerGasOverflow.VerificationGasLimit = overflowUint128Val
	if _, err := opVerGasOverflow.ToPacked(); err != ErrGasLimitOverflow {
		t.Fatalf("VerificationGasLimit 溢位預期 ErrGasLimitOverflow, 實際: %v", err)
	}

	// 4. ToPacked gasFee 溢位
	opMaxFeeOverflow := buildValidTestOp()
	opMaxFeeOverflow.MaxFeePerGas = overflowUint128Val
	opMaxFeeOverflow.MaxPriorityFeePerGas = big.NewInt(100)
	if _, err := opMaxFeeOverflow.ToPacked(); err != ErrGasFeeOverflow {
		t.Fatalf("MaxFeePerGas 溢位預期 ErrGasFeeOverflow, 實際: %v", err)
	}

	// 5. paymasterAndData 長度防禦 (v0.7 規範：若非空，長度不得小於 52 位元組)
	opShortPM := buildValidTestOp()
	opShortPM.PaymasterAndData = make([]byte, 51) // 51 位元組，不合法
	if _, err := opShortPM.ToPacked(); err != ErrInvalidPaymasterLength {
		t.Fatalf("PaymasterAndData 51 位元組預期 ErrInvalidPaymasterLength, 實際: %v", err)
	}

	opValidPM := buildValidTestOp()
	opValidPM.PaymasterAndData = make([]byte, 52) // 52 位元組，合法邊界值
	packedValidPM, err := opValidPM.ToPacked()
	if err != nil {
		t.Fatalf("PaymasterAndData 52 位元組合法轉換失敗: %v", err)
	}
	if len(packedValidPM.PaymasterAndData) != 52 {
		t.Fatalf("PaymasterAndData 打包後長度不符")
	}
}

// TestAdversarial_NilPointerSafety 驗證所有導出方法在 nil 接收者與空指標輸入下絕對安全（無 panic）
func TestAdversarial_NilPointerSafety(t *testing.T) {
	t.Parallel()

	var nilOp *UserOperation
	var nilPackedOp *PackedUserOperation

	// 1. nil UserOperation 方法安全
	if err := nilOp.Validate(); err != ErrNilUserOp {
		t.Fatalf("nilOp.Validate 預期 ErrNilUserOp, 實際: %v", err)
	}
	if cloneRes := nilOp.Clone(); cloneRes != nil {
		t.Fatalf("nilOp.Clone 預期 nil, 實際: %v", cloneRes)
	}
	if _, err := nilOp.ToPacked(); err != ErrNilUserOp {
		t.Fatalf("nilOp.ToPacked 預期 ErrNilUserOp, 實際: %v", err)
	}
	jsonBytes, err := nilOp.MarshalJSON()
	if err != nil || string(jsonBytes) != "null" {
		t.Fatalf("nilOp.MarshalJSON 預期 null, 實際: %s, err: %v", string(jsonBytes), err)
	}

	// 2. nil PackedUserOperation 方法安全
	if cloneRes := nilPackedOp.Clone(); cloneRes != nil {
		t.Fatalf("nilPackedOp.Clone 預期 nil, 實際: %v", cloneRes)
	}
	if _, err := nilPackedOp.Unpack(); err != ErrNilPackedUserOp {
		t.Fatalf("nilPackedOp.Unpack 預期 ErrNilPackedUserOp, 實際: %v", err)
	}
	jsonBytesPacked, err := nilPackedOp.MarshalJSON()
	if err != nil || string(jsonBytesPacked) != "null" {
		t.Fatalf("nilPackedOp.MarshalJSON 預期 null, 實際: %s, err: %v", string(jsonBytesPacked), err)
	}

	// 3. 函式傳入 nil 接收者安全
	if _, err := PackUserOp(nil); err != ErrNilUserOp {
		t.Fatalf("PackUserOp(nil) 預期 ErrNilUserOp, 實際: %v", err)
	}
	if _, err := PackPackedUserOp(nil); err != ErrNilPackedUserOp {
		t.Fatalf("PackPackedUserOp(nil) 預期 ErrNilPackedUserOp, 實際: %v", err)
	}
	if _, err := CalculateInnerHash(nil); err != ErrNilUserOp {
		t.Fatalf("CalculateInnerHash(nil) 預期 ErrNilUserOp, 實際: %v", err)
	}
	if _, err := CalculatePackedInnerHash(nil); err != ErrNilPackedUserOp {
		t.Fatalf("CalculatePackedInnerHash(nil) 預期 ErrNilPackedUserOp, 實際: %v", err)
	}
	if _, err := GetUserOpHash(nil, CanonicalEntryPointV06, big.NewInt(1)); err != ErrNilUserOp {
		t.Fatalf("GetUserOpHash(nil) 預期 ErrNilUserOp, 實際: %v", err)
	}
	if _, err := GetUserOpHashV06(nil, CanonicalEntryPointV06, big.NewInt(1)); err != ErrNilUserOp {
		t.Fatalf("GetUserOpHashV06(nil) 預期 ErrNilUserOp, 實際: %v", err)
	}
	if _, err := GetUserOpHashV07(nil, CanonicalEntryPointV07, big.NewInt(1)); err != ErrNilPackedUserOp {
		t.Fatalf("GetUserOpHashV07(nil) 預期 ErrNilPackedUserOp, 實際: %v", err)
	}
	if _, err := PackUserOpTuple(nil); err != ErrNilUserOp {
		t.Fatalf("PackUserOpTuple(nil) 預期 ErrNilUserOp, 實際: %v", err)
	}
}

// TestAdversarial_MutationResistance 驗證深拷貝抗突變隔離性
func TestAdversarial_MutationResistance(t *testing.T) {
	t.Parallel()

	original := buildValidTestOp()
	original.InitCode = []byte{0x01, 0x02, 0x03}
	original.CallData = []byte{0xaa, 0xbb, 0xcc}
	original.PaymasterAndData = []byte{0x99}
	original.Signature = []byte{0x55}

	cloned := original.Clone()

	// 突變 cloned 之所有指標與切片
	cloned.Sender = common.HexToAddress("0x3333333333333333333333333333333333333333")
	cloned.Nonce.Add(cloned.Nonce, big.NewInt(100))
	cloned.CallGasLimit.Add(cloned.CallGasLimit, big.NewInt(100))
	cloned.VerificationGasLimit.Add(cloned.VerificationGasLimit, big.NewInt(100))
	cloned.PreVerificationGas.Add(cloned.PreVerificationGas, big.NewInt(100))
	cloned.MaxFeePerGas.Add(cloned.MaxFeePerGas, big.NewInt(100))
	cloned.MaxPriorityFeePerGas.Add(cloned.MaxPriorityFeePerGas, big.NewInt(100))
	cloned.InitCode[0] = 0xff
	cloned.CallData[0] = 0xff
	cloned.PaymasterAndData[0] = 0xff
	cloned.Signature[0] = 0xff

	// 驗證 original 數值未受任何突變污染
	if original.Sender == cloned.Sender {
		t.Fatalf("Sender 產生共享記憶體污染")
	}
	if original.Nonce.Int64() != 42 {
		t.Fatalf("Nonce 產生共享指標污染: 實際 %s", original.Nonce.String())
	}
	if original.CallGasLimit.Int64() != 50000 {
		t.Fatalf("CallGasLimit 產生共享指標污染")
	}
	if original.InitCode[0] != 0x01 {
		t.Fatalf("InitCode 切片產生共享底層陣列污染")
	}
	if original.CallData[0] != 0xaa {
		t.Fatalf("CallData 切片產生共享底層陣列污染")
	}
	if original.PaymasterAndData[0] != 0x99 {
		t.Fatalf("PaymasterAndData 切片產生共享底層陣列污染")
	}
	if original.Signature[0] != 0x55 {
		t.Fatalf("Signature 切片產生共享底層陣列污染")
	}

	// 驗證原始 UserOp 之 Hash 依然完全正確穩定
	hashV06, err := GetUserOpHash(original, CanonicalEntryPointV06, big.NewInt(1))
	if err != nil {
		t.Fatalf("突變後計算原始 UserOp 雜湊失敗: %v", err)
	}
	oracleHash := independentCalculateUserOpHashV06(original, CanonicalEntryPointV06, big.NewInt(1))
	if hashV06 != oracleHash {
		t.Fatalf("原始 UserOp 之雜湊受到外部突變破壞")
	}
}

// TestAdversarial_HighConcurrencyStress 驗證高並發競爭環境下無資料競態且計算確定性 100%
func TestAdversarial_HighConcurrencyStress(t *testing.T) {
	t.Parallel()

	baseOp := buildValidTestOp()
	concurrencyLimit := 200
	var wg sync.WaitGroup
	wg.Add(concurrencyLimit)

	chainIDs := []*big.Int{
		big.NewInt(1),
		big.NewInt(5),
		big.NewInt(137),
		big.NewInt(11155111),
	}

	expectedHashes := make([]common.Hash, len(chainIDs))
	for i, c := range chainIDs {
		h, err := GetUserOpHash(baseOp, CanonicalEntryPointV06, c)
		if err != nil {
			t.Fatalf("初始化基準雜湊失敗: %v", err)
		}
		expectedHashes[i] = h
	}

	for i := 0; i < concurrencyLimit; i += 1 {
		chainIndex := i % len(chainIDs)
		targetChain := chainIDs[chainIndex]
		expectedHash := expectedHashes[chainIndex]

		go func(opCopy *UserOperation, targetChainID *big.Int, expected common.Hash) {
			defer wg.Done()
			for iteration := 0; iteration < 50; iteration += 1 {
				// 並行計算 Hash
				calculated, err := GetUserOpHash(opCopy, CanonicalEntryPointV06, targetChainID)
				if err != nil {
					t.Errorf("並行計算 Hash 失敗: %v", err)
					return
				}
				if calculated != expected {
					t.Errorf("高並行計算不一致: 預期 %s, 實際 %s", expected.Hex(), calculated.Hex())
					return
				}

				// 並行執行 ToPacked 與 Unpack
				packed, err := opCopy.ToPacked()
				if err != nil {
					t.Errorf("並行 ToPacked 失敗: %v", err)
					return
				}
				unpacked, err := packed.Unpack()
				if err != nil {
					t.Errorf("並行 Unpack 失敗: %v", err)
					return
				}
				if unpacked.Sender != opCopy.Sender {
					t.Errorf("並行 Unpack 欄位損壞")
					return
				}

				// 並行執行 JSON 序列化與反序列化
				jsonBytes, err := opCopy.MarshalJSON()
				if err != nil {
					t.Errorf("並行 MarshalJSON 失敗: %v", err)
					return
				}
				var roundTrip UserOperation
				if err := roundTrip.UnmarshalJSON(jsonBytes); err != nil {
					t.Errorf("並行 UnmarshalJSON 失敗: %v", err)
					return
				}
			}
		}(baseOp.Clone(), targetChain, expectedHash)
	}

	wg.Wait()
}
