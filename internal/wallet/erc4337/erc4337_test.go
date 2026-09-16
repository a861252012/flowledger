package erc4337

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	testPrivKeyHex = "4f3edf983ac636a65a842ce7c78d9aa706d3b113bce9c46f30d7d21715b23b1d"
)

var (
	expectedSignerAddr = common.HexToAddress("0x90F8bf6A479f320ead074411a4B0e7944Ea8c9C1")
)

func mustHex(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// getOfficialTestOpV06 產出符合 EIP-4337 v0.6 欄位規範之自建合成測試資料
func getOfficialTestOpV06() *UserOperation {
	return &UserOperation{
		Sender:               common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:                big.NewInt(0),
		InitCode:             []byte{},
		CallData:             mustHex("0xb61d27f60000000000000000000000000000000000000000000000000000000000000001"),
		CallGasLimit:         big.NewInt(100000),
		VerificationGasLimit: big.NewInt(150000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(1000000000),
		MaxPriorityFeePerGas: big.NewInt(1000000000),
		PaymasterAndData:     []byte{},
		Signature:            []byte{},
	}
}

func assertUserOpEqual(t *testing.T, expected, actual *UserOperation) {
	t.Helper()
	if actual.Sender != expected.Sender {
		t.Fatalf("Sender 不符: 預期 %s, 實際 %s", expected.Sender.Hex(), actual.Sender.Hex())
	}
	if expected.Nonce.Cmp(actual.Nonce) != 0 {
		t.Fatalf("Nonce 不符: 預期 %s, 實際 %s", expected.Nonce.String(), actual.Nonce.String())
	}
	if !bytes.Equal(expected.InitCode, actual.InitCode) {
		t.Fatalf("InitCode 不符")
	}
	if !bytes.Equal(expected.CallData, actual.CallData) {
		t.Fatalf("CallData 不符")
	}
	if expected.CallGasLimit.Cmp(actual.CallGasLimit) != 0 {
		t.Fatalf("CallGasLimit 不符: 預期 %s, 實際 %s", expected.CallGasLimit.String(), actual.CallGasLimit.String())
	}
	if expected.VerificationGasLimit.Cmp(actual.VerificationGasLimit) != 0 {
		t.Fatalf("VerificationGasLimit 不符: 預期 %s, 實際 %s", expected.VerificationGasLimit.String(), actual.VerificationGasLimit.String())
	}
	if expected.PreVerificationGas.Cmp(actual.PreVerificationGas) != 0 {
		t.Fatalf("PreVerificationGas 不符: 預期 %s, 實際 %s", expected.PreVerificationGas.String(), actual.PreVerificationGas.String())
	}
	if expected.MaxFeePerGas.Cmp(actual.MaxFeePerGas) != 0 {
		t.Fatalf("MaxFeePerGas 不符: 預期 %s, 實際 %s", expected.MaxFeePerGas.String(), actual.MaxFeePerGas.String())
	}
	if expected.MaxPriorityFeePerGas.Cmp(actual.MaxPriorityFeePerGas) != 0 {
		t.Fatalf("MaxPriorityFeePerGas 不符: 預期 %s, 實際 %s", expected.MaxPriorityFeePerGas.String(), actual.MaxPriorityFeePerGas.String())
	}
	if !bytes.Equal(expected.PaymasterAndData, actual.PaymasterAndData) {
		t.Fatalf("PaymasterAndData 不符")
	}
	if !bytes.Equal(expected.Signature, actual.Signature) {
		t.Fatalf("Signature 不符")
	}
}

func assertPackedUserOpEqual(t *testing.T, expected, actual *PackedUserOperation) {
	t.Helper()
	if actual.Sender != expected.Sender {
		t.Fatalf("Sender 不符: 預期 %s, 實際 %s", expected.Sender.Hex(), actual.Sender.Hex())
	}
	if expected.Nonce.Cmp(actual.Nonce) != 0 {
		t.Fatalf("Nonce 不符: 預期 %s, 實際 %s", expected.Nonce.String(), actual.Nonce.String())
	}
	if !bytes.Equal(expected.InitCode, actual.InitCode) {
		t.Fatalf("InitCode 不符")
	}
	if !bytes.Equal(expected.CallData, actual.CallData) {
		t.Fatalf("CallData 不符")
	}
	if expected.AccountGasLimits != actual.AccountGasLimits {
		t.Fatalf("AccountGasLimits 不符")
	}
	if expected.PreVerificationGas.Cmp(actual.PreVerificationGas) != 0 {
		t.Fatalf("PreVerificationGas 不符")
	}
	if expected.GasFees != actual.GasFees {
		t.Fatalf("GasFees 不符")
	}
	if !bytes.Equal(expected.PaymasterAndData, actual.PaymasterAndData) {
		t.Fatalf("PaymasterAndData 不符")
	}
	if !bytes.Equal(expected.Signature, actual.Signature) {
		t.Fatalf("Signature 不符")
	}
}

// TestSyntheticVector_V06 驗證 ERC-4337 v0.6 規範結構之自建合成測試案例
func TestSyntheticVector_V06(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()
	entryPoint := CanonicalEntryPointV06
	chainID := big.NewInt(1)

	// 1. 驗證動態欄位中繼雜湊
	expectedEmptyHash := crypto.Keccak256Hash([]byte{})
	if crypto.Keccak256Hash(op.InitCode) != expectedEmptyHash {
		t.Fatalf("InitCode 雜湊不符")
	}
	if crypto.Keccak256Hash(op.PaymasterAndData) != expectedEmptyHash {
		t.Fatalf("PaymasterAndData 雜湊不符")
	}
	expectedCallDataHash := common.HexToHash("0xf5b577edbb26cb5ee2376d418c808b3d7fec26a946b56d7943dda15e71d430db")
	if crypto.Keccak256Hash(op.CallData) != expectedCallDataHash {
		t.Fatalf("CallData 雜湊不符: 預期 %s, 實際 %s", expectedCallDataHash.Hex(), crypto.Keccak256Hash(op.CallData).Hex())
	}

	// 2. 驗證內層打包長度為 320 位元組
	packedV06, err := PackUserOp(op)
	if err != nil {
		t.Fatalf("PackUserOp 失敗: %v", err)
	}
	if len(packedV06) != InnerPackLengthV06 {
		t.Fatalf("PackUserOp 長度不符: 預期 %d, 實際 %d", InnerPackLengthV06, len(packedV06))
	}

	// 驗證二進位原生打包與 abi.Arguments 打包結果逐位元組一致
	abiPackedV06, err := PackUserOpForHashV06(op)
	if err != nil {
		t.Fatalf("PackUserOpForHashV06 失敗: %v", err)
	}
	if !bytes.Equal(packedV06, abiPackedV06) {
		t.Fatalf("PackUserOp 與 PackUserOpForHashV06 結果不一致")
	}

	// 3. 驗證內層雜湊值
	expectedInnerHash := common.HexToHash("0x3c30e3433cf255f1e620e3c9945d097b5947ef56d7f8396d2a22d2656185dc66")
	innerHash, err := CalculateInnerHash(op)
	if err != nil {
		t.Fatalf("CalculateInnerHash 失敗: %v", err)
	}
	if innerHash != expectedInnerHash {
		t.Fatalf("innerHash 不符: 預期 %s, 實際 %s", expectedInnerHash.Hex(), innerHash.Hex())
	}

	// 4. 驗證外層打包長度為 96 位元組
	outerBytes, err := PackUserOpHashData(innerHash, entryPoint, chainID)
	if err != nil {
		t.Fatalf("PackUserOpHashData 失敗: %v", err)
	}
	if len(outerBytes) != OuterPackLength {
		t.Fatalf("OuterBytes 長度不符: 預期 %d, 實際 %d", OuterPackLength, len(outerBytes))
	}

	abiOuterBytes, err := PackOuterUserOpHash(innerHash, entryPoint, chainID)
	if err != nil {
		t.Fatalf("PackOuterUserOpHash 失敗: %v", err)
	}
	if !bytes.Equal(outerBytes, abiOuterBytes) {
		t.Fatalf("外層 ABI 打包與二進位組裝不一致")
	}

	// 5. 驗證最終 UserOpHash
	expectedUserOpHash := common.HexToHash("0xf09f5c22caf3315e500bd48ed65191b98ae05ef7840b2e4fabe9b0753e60fc51")
	userOpHash, err := GetUserOpHash(op, entryPoint, chainID)
	if err != nil {
		t.Fatalf("GetUserOpHash 失敗: %v", err)
	}
	if userOpHash != expectedUserOpHash {
		t.Fatalf("userOpHash 不符: 預期 %s, 實際 %s", expectedUserOpHash.Hex(), userOpHash.Hex())
	}

	// 6. 驗證以太坊待簽訊息雜湊與 65 位元組 ECDSA 簽名生成及還原
	expectedEthSignedMsgHash := common.HexToHash("0x20a104e89e8778c92e225207a05423fe1f18009bb779235b68db0839a42d3907")
	msgPrefix := []byte("\x19Ethereum Signed Message:\n32")
	ethSignedMsgHash := crypto.Keccak256Hash(msgPrefix, userOpHash.Bytes())
	if ethSignedMsgHash != expectedEthSignedMsgHash {
		t.Fatalf("ethSignedMsgHash 不符: 預期 %s, 實際 %s", expectedEthSignedMsgHash.Hex(), ethSignedMsgHash.Hex())
	}

	privKey, err := crypto.HexToECDSA(testPrivKeyHex)
	if err != nil {
		t.Fatalf("解析測試私鑰失敗: %v", err)
	}

	sig, err := crypto.Sign(ethSignedMsgHash.Bytes(), privKey)
	if err != nil {
		t.Fatalf("ECDSA 簽名失敗: %v", err)
	}
	if len(sig) != 65 {
		t.Fatalf("簽章長度不為 65 位元組: 實際 %d", len(sig))
	}

	// 將 V 值轉為以太坊 27/28 格式
	sigEthereum := make([]byte, 65)
	copy(sigEthereum, sig)
	sigEthereum[64] += 27

	expectedSig := mustHex("0x8df0873ef40e3dccc92d4f723196a448af8cfcb83a53fbb59d041370fcb510093f05cff054024c8bc35b7b49a4647c6ec5c49865ede12847a635434454702e981c")
	if !bytes.Equal(sigEthereum, expectedSig) {
		t.Fatalf("生成的簽章不符合預期: 預期 %x, 實際 %x", expectedSig, sigEthereum)
	}

	// 還原公鑰地址驗證 (SigToPub 需傳入 V = 0 或 1)
	recoveredPub, err := crypto.SigToPub(ethSignedMsgHash.Bytes(), sig)
	if err != nil {
		t.Fatalf("SigToPub 還原公鑰失敗: %v", err)
	}
	recoveredAddr := crypto.PubkeyToAddress(*recoveredPub)
	if recoveredAddr != expectedSignerAddr {
		t.Fatalf("還原地址不符: 預期 %s, 實際 %s", expectedSignerAddr.Hex(), recoveredAddr.Hex())
	}
}

// TestSyntheticVector_V07 驗證 ERC-4337 v0.7 規範結構之自建合成測試案例
func TestSyntheticVector_V07(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()
	entryPoint := CanonicalEntryPointV07
	chainID := big.NewInt(1)

	// 1. 測試轉換為 PackedUserOperation
	packedOp, err := op.ToPacked()
	if err != nil {
		t.Fatalf("ToPacked 轉換失敗: %v", err)
	}

	expectedAccountGasLimits := mustHex("0x000000000000000000000000000249f0000000000000000000000000000186a0")
	expectedGasFees := mustHex("0x0000000000000000000000003b9aca000000000000000000000000003b9aca00")
	if !bytes.Equal(packedOp.AccountGasLimits[:], expectedAccountGasLimits) {
		t.Fatalf("AccountGasLimits 不符: 預期 %x, 實際 %x", expectedAccountGasLimits, packedOp.AccountGasLimits[:])
	}
	if !bytes.Equal(packedOp.GasFees[:], expectedGasFees) {
		t.Fatalf("GasFees 不符: 預期 %x, 實際 %x", expectedGasFees, packedOp.GasFees[:])
	}

	// 2. 驗證內層打包長度為 256 位元組
	packedV07, err := PackPackedUserOp(packedOp)
	if err != nil {
		t.Fatalf("PackPackedUserOp 失敗: %v", err)
	}
	if len(packedV07) != InnerPackLengthV07 {
		t.Fatalf("PackPackedUserOp 長度不符: 預期 %d, 實際 %d", InnerPackLengthV07, len(packedV07))
	}

	abiPackedV07, err := PackUserOpForHashV07(packedOp)
	if err != nil {
		t.Fatalf("PackUserOpForHashV07 失敗: %v", err)
	}
	if !bytes.Equal(packedV07, abiPackedV07) {
		t.Fatalf("PackPackedUserOp 與 PackUserOpForHashV07 結果不一致")
	}

	// 3. 驗證內層雜湊值
	expectedInnerHash := common.HexToHash("0x2e53ec487ef99b51a3b397bcd97bfed1f30c743f2f00023630dedd17fdb12294")
	innerHash, err := CalculatePackedInnerHash(packedOp)
	if err != nil {
		t.Fatalf("CalculatePackedInnerHash 失敗: %v", err)
	}
	if innerHash != expectedInnerHash {
		t.Fatalf("v0.7 innerHash 不符: 預期 %s, 實際 %s", expectedInnerHash.Hex(), innerHash.Hex())
	}

	// 4. 驗證最終 UserOpHash
	expectedUserOpHash := common.HexToHash("0xed3feb815570598eca9c065493d7dc58b8301a61018c8b4e9814ea6277ce3e91")
	userOpHashDirect, err := GetUserOpHashV07(packedOp, entryPoint, chainID)
	if err != nil {
		t.Fatalf("GetUserOpHashV07 失敗: %v", err)
	}
	if userOpHashDirect != expectedUserOpHash {
		t.Fatalf("GetUserOpHashV07 雜湊不符: 預期 %s, 實際 %s", expectedUserOpHash.Hex(), userOpHashDirect.Hex())
	}

	// 5. 驗證 GetUserOpHash 自動適配 CanonicalEntryPointV07
	autoHash, err := GetUserOpHash(op, entryPoint, chainID)
	if err != nil {
		t.Fatalf("GetUserOpHash 自動適配失敗: %v", err)
	}
	if autoHash != expectedUserOpHash {
		t.Fatalf("自動適配之 UserOpHash 不符: 預期 %s, 實際 %s", expectedUserOpHash.Hex(), autoHash.Hex())
	}

	// 6. 驗證待簽訊息雜湊與簽名
	expectedEthSignedMsgHash := common.HexToHash("0x86aa5b17d5164945bda347194689c5164537eab9ced28e27637d3d8992a4e43e")
	msgPrefix := []byte("\x19Ethereum Signed Message:\n32")
	ethSignedMsgHash := crypto.Keccak256Hash(msgPrefix, expectedUserOpHash.Bytes())
	if ethSignedMsgHash != expectedEthSignedMsgHash {
		t.Fatalf("v0.7 ethSignedMsgHash 不符: 預期 %s, 實際 %s", expectedEthSignedMsgHash.Hex(), ethSignedMsgHash.Hex())
	}

	privKey, err := crypto.HexToECDSA(testPrivKeyHex)
	if err != nil {
		t.Fatalf("解析私鑰失敗: %v", err)
	}
	sig, err := crypto.Sign(ethSignedMsgHash.Bytes(), privKey)
	if err != nil {
		t.Fatalf("ECDSA 簽署失敗: %v", err)
	}

	sigEthereum := make([]byte, 65)
	copy(sigEthereum, sig)
	sigEthereum[64] += 27

	expectedSig := mustHex("0xd044a172450c5718592d02611aa68ecee23459023499b0f9748912a9fffa5fa929c1c8b65a14b52a1eb5704483dfa86cc3e5e53db71ca53cea651887519644f31c")
	if !bytes.Equal(sigEthereum, expectedSig) {
		t.Fatalf("v0.7 簽章不符: 預期 %x, 實際 %x", expectedSig, sigEthereum)
	}

	recoveredPub, err := crypto.SigToPub(ethSignedMsgHash.Bytes(), sig)
	if err != nil {
		t.Fatalf("v0.7 SigToPub 失敗: %v", err)
	}
	recoveredAddr := crypto.PubkeyToAddress(*recoveredPub)
	if recoveredAddr != expectedSignerAddr {
		t.Fatalf("v0.7 還原地址不符: 預期 %s, 實際 %s", expectedSignerAddr.Hex(), recoveredAddr.Hex())
	}
}

// TestUserOpHash_ChainIDBinding 驗證防跨鏈重放之 Chain ID 綁定效應
func TestUserOpHash_ChainIDBinding(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()
	entryPoint := CanonicalEntryPointV06

	chainIDs := []*big.Int{
		big.NewInt(1),        // Mainnet
		big.NewInt(11155111), // Sepolia
		big.NewInt(137),      // Polygon
		big.NewInt(42161),    // Arbitrum
		big.NewInt(10),       // Optimism
		big.NewInt(8453),     // Base
		big.NewInt(31337),    // Anvil/Hardhat
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1)), // 2^64 - 1
	}

	seenHashes := make(map[common.Hash]*big.Int)
	for _, cid := range chainIDs {
		h, err := GetUserOpHash(op, entryPoint, cid)
		if err != nil {
			t.Fatalf("ChainID %s 計算雜湊失敗: %v", cid.String(), err)
		}
		if prevCid, exists := seenHashes[h]; exists {
			t.Fatalf("發現雜湊碰撞！ChainID %s 與 %s 產生相同之 UserOpHash: %s", cid.String(), prevCid.String(), h.Hex())
		}
		seenHashes[h] = cid
	}
}

// TestUserOpHash_EntryPointBinding 驗證防跨合約版本重放之 EntryPoint 綁定效應
func TestUserOpHash_EntryPointBinding(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()
	chainID := big.NewInt(1)

	entryPoints := []common.Address{
		CanonicalEntryPointV06,
		CanonicalEntryPointV07,
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
	}

	seenHashes := make(map[common.Hash]common.Address)
	for _, ep := range entryPoints {
		h, err := GetUserOpHash(op, ep, chainID)
		if err != nil {
			t.Fatalf("EntryPoint %s 計算失敗: %v", ep.Hex(), err)
		}
		if prevEp, exists := seenHashes[h]; exists {
			t.Fatalf("EntryPoint %s 與 %s 產生重複 UserOpHash: %s", ep.Hex(), prevEp.Hex(), h.Hex())
		}
		seenHashes[h] = ep
	}
}

// TestUserOpHash_CrossVersionIsolation 驗證 v0.6 與 v0.7 跨版本隔離
func TestUserOpHash_CrossVersionIsolation(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()
	chainID := big.NewInt(1)

	h06, err := GetUserOpHashV06(op, CanonicalEntryPointV06, chainID)
	if err != nil {
		t.Fatalf("計算 v0.6 失敗: %v", err)
	}

	packed, err := op.ToPacked()
	if err != nil {
		t.Fatalf("ToPacked 失敗: %v", err)
	}
	h07, err := GetUserOpHashV07(packed, CanonicalEntryPointV07, chainID)
	if err != nil {
		t.Fatalf("計算 v0.7 失敗: %v", err)
	}

	if h06 == h07 {
		t.Fatalf("v0.6 與 v0.7 雜湊不得相同")
	}
}

// TestUserOp_EmptyByteFields 驗證空位元組切片安全打包
func TestUserOp_EmptyByteFields(t *testing.T) {
	t.Parallel()
	op := &UserOperation{
		Sender:               common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:                big.NewInt(0),
		InitCode:             []byte{},
		CallData:             []byte{},
		CallGasLimit:         big.NewInt(0),
		VerificationGasLimit: big.NewInt(0),
		PreVerificationGas:   big.NewInt(0),
		MaxFeePerGas:         big.NewInt(0),
		MaxPriorityFeePerGas: big.NewInt(0),
		PaymasterAndData:     []byte{},
		Signature:            []byte{},
	}

	packed, err := PackUserOp(op)
	if err != nil {
		t.Fatalf("PackUserOp 空位元組切片失敗: %v", err)
	}
	if len(packed) != InnerPackLengthV06 {
		t.Fatalf("長度不為 320 位元組: 實際 %d", len(packed))
	}

	// 檢查 Word 2, 3, 9 的雜湊是否皆為 keccak256("")
	emptyHash := crypto.Keccak256(nil)
	if !bytes.Equal(packed[64:96], emptyHash) {
		t.Fatalf("Word 2 (initCode) 雜湊不符")
	}
	if !bytes.Equal(packed[96:128], emptyHash) {
		t.Fatalf("Word 3 (callData) 雜湊不符")
	}
	if !bytes.Equal(packed[288:320], emptyHash) {
		t.Fatalf("Word 9 (paymasterAndData) 雜湊不符")
	}
}

// TestUserOp_NilByteFields 驗證 nil 切片與空切片具備一致密碼學語意
func TestUserOp_NilByteFields(t *testing.T) {
	t.Parallel()
	opNil := &UserOperation{
		Sender:               common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:                big.NewInt(0),
		InitCode:             nil,
		CallData:             nil,
		CallGasLimit:         big.NewInt(0),
		VerificationGasLimit: big.NewInt(0),
		PreVerificationGas:   big.NewInt(0),
		MaxFeePerGas:         big.NewInt(0),
		MaxPriorityFeePerGas: big.NewInt(0),
		PaymasterAndData:     nil,
		Signature:            nil,
	}

	opEmpty := opNil.Clone()
	opEmpty.InitCode = []byte{}
	opEmpty.CallData = []byte{}
	opEmpty.PaymasterAndData = []byte{}
	opEmpty.Signature = []byte{}

	hashNil, err := CalculateInnerHash(opNil)
	if err != nil {
		t.Fatalf("計算 opNil 失敗: %v", err)
	}

	hashEmpty, err := CalculateInnerHash(opEmpty)
	if err != nil {
		t.Fatalf("計算 opEmpty 失敗: %v", err)
	}

	if hashNil != hashEmpty {
		t.Fatalf("nil 切片與空切片雜湊不一致: got %s, want %s", hashNil.Hex(), hashEmpty.Hex())
	}
}

// TestUserOp_MaxUint256 驗證最大 uint256 大數邊界安全打包
func TestUserOp_MaxUint256(t *testing.T) {
	t.Parallel()
	maxVal := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	op := &UserOperation{
		Sender:               common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:                new(big.Int).Set(maxVal),
		InitCode:             []byte{},
		CallData:             []byte{},
		CallGasLimit:         new(big.Int).Set(maxVal),
		VerificationGasLimit: new(big.Int).Set(maxVal),
		PreVerificationGas:   new(big.Int).Set(maxVal),
		MaxFeePerGas:         new(big.Int).Set(maxVal),
		MaxPriorityFeePerGas: new(big.Int).Set(maxVal),
		PaymasterAndData:     []byte{},
		Signature:            []byte{},
	}

	packed, err := PackUserOp(op)
	if err != nil {
		t.Fatalf("PackUserOp MaxUint256 失敗: %v", err)
	}
	if len(packed) != InnerPackLengthV06 {
		t.Fatalf("長度不為 320 位元組")
	}

	// 檢查 nonce (Word 1 [32:64]) 是否全部填滿 0xff
	for i := 32; i < 64; i += 1 {
		if packed[i] != 0xff {
			t.Fatalf("Word 1 未填滿 0xff，byte[%d] = %x", i, packed[i])
		}
	}
}

// TestUserOp_V07_Uint128PackingAndOverflow 驗證 v0.7 Uint128 邊界與溢位防禦
func TestUserOp_V07_Uint128PackingAndOverflow(t *testing.T) {
	t.Parallel()

	// 1. 臨界值 2^128 - 1 成功打包與無損還原
	critVal := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	packed, err := PackUint128Pair(critVal, critVal)
	if err != nil {
		t.Fatalf("臨界值打包失敗: %v", err)
	}
	for i := 0; i < 32; i += 1 {
		if packed[i] != 0xff {
			t.Fatalf("臨界值未填滿 0xff，byte[%d] = %x", i, packed[i])
		}
	}
	high, low := UnpackUint128Pair(packed)
	if high.Cmp(critVal) != 0 || low.Cmp(critVal) != 0 {
		t.Fatalf("臨界值還原不符")
	}

	// 2. 溢位值 2^128 必須拒絕
	overflowVal := new(big.Int).Lsh(big.NewInt(1), 128)
	_, err = PackUint128Pair(overflowVal, big.NewInt(0))
	if err == nil || err != ErrGasLimitOverflow {
		t.Fatalf("溢位值未回傳 ErrGasLimitOverflow，得到: %v", err)
	}

	// 3. 負數必須拒絕
	_, err = PackUint128Pair(big.NewInt(-1), big.NewInt(0))
	if err == nil || err != ErrNegativeValue {
		t.Fatalf("負數未回傳 ErrNegativeValue，得到: %v", err)
	}

	// 4. 在 ToPacked 中驗證 GasLimit 溢位報錯
	opLimitOverflow := getOfficialTestOpV06()
	opLimitOverflow.CallGasLimit = new(big.Int).Set(overflowVal)
	_, err = opLimitOverflow.ToPacked()
	if err == nil || err != ErrGasLimitOverflow {
		t.Fatalf("ToPacked GasLimit 溢位未報錯 ErrGasLimitOverflow: %v", err)
	}

	// 5. 在 ToPacked 中驗證 GasFees 溢位報錯
	opFeeOverflow := getOfficialTestOpV06()
	opFeeOverflow.MaxFeePerGas = new(big.Int).Set(overflowVal)
	_, err = opFeeOverflow.ToPacked()
	if err == nil || err != ErrGasFeeOverflow {
		t.Fatalf("ToPacked GasFees 溢位未報錯 ErrGasFeeOverflow: %v", err)
	}
}

// TestUserOp_Validation_Errors 驗證資料校驗與防禦異常捕獲
func TestUserOp_Validation_Errors(t *testing.T) {
	t.Parallel()

	// 1. nil 指標防禦
	if err := (*UserOperation)(nil).Validate(); err != ErrNilUserOp {
		t.Fatalf("nil Validate 應回傳 ErrNilUserOp: %v", err)
	}
	if _, err := PackUserOp(nil); err != ErrNilUserOp {
		t.Fatalf("PackUserOp(nil) 應回傳 ErrNilUserOp: %v", err)
	}
	if _, err := GetUserOpHash(nil, CanonicalEntryPointV06, big.NewInt(1)); err != ErrNilUserOp {
		t.Fatalf("GetUserOpHash(nil) 應回傳 ErrNilUserOp: %v", err)
	}

	// 2. 零地址 sender 防禦
	opZeroSender := getOfficialTestOpV06()
	opZeroSender.Sender = common.Address{}
	if err := opZeroSender.Validate(); err != ErrInvalidSender {
		t.Fatalf("零地址 sender 未報錯 ErrInvalidSender: %v", err)
	}

	// 3. nil 大數欄位防禦
	opNilField := getOfficialTestOpV06()
	opNilField.Nonce = nil
	if err := opNilField.Validate(); err != ErrNilField {
		t.Fatalf("nil 欄位未報錯 ErrNilField: %v", err)
	}

	// 4. 負數數值防禦
	opNegative := getOfficialTestOpV06()
	opNegative.Nonce = big.NewInt(-1)
	if err := opNegative.Validate(); err != ErrNegativeValue {
		t.Fatalf("負數未報錯 ErrNegativeValue: %v", err)
	}

	// 5. maxPriorityFeePerGas > maxFeePerGas 防禦
	opFeeInconsistent := getOfficialTestOpV06()
	opFeeInconsistent.MaxPriorityFeePerGas = big.NewInt(2000)
	opFeeInconsistent.MaxFeePerGas = big.NewInt(1000)
	if err := opFeeInconsistent.Validate(); err != ErrGasFeeInconsistent {
		t.Fatalf("費用不一致未報錯 ErrGasFeeInconsistent: %v", err)
	}

	// 6. EntryPoint 為零地址防禦
	opValid := getOfficialTestOpV06()
	if _, err := GetUserOpHash(opValid, common.Address{}, big.NewInt(1)); err != ErrInvalidEntryPoint {
		t.Fatalf("零地址 EntryPoint 未報錯 ErrInvalidEntryPoint: %v", err)
	}

	// 7. Chain ID 為空或小於等於 0 防禦
	if _, err := GetUserOpHash(opValid, CanonicalEntryPointV06, nil); err != ErrInvalidChainID {
		t.Fatalf("nil ChainID 未報錯 ErrInvalidChainID: %v", err)
	}
	if _, err := GetUserOpHash(opValid, CanonicalEntryPointV06, big.NewInt(0)); err != ErrInvalidChainID {
		t.Fatalf("0 ChainID 未報錯 ErrInvalidChainID: %v", err)
	}
	if _, err := GetUserOpHash(opValid, CanonicalEntryPointV06, big.NewInt(-5)); err != ErrInvalidChainID {
		t.Fatalf("負數 ChainID 未報錯 ErrInvalidChainID: %v", err)
	}

	// 8. v0.7 paymasterAndData 長度小於 52 位元組防禦
	opInvalidPaymaster := getOfficialTestOpV06()
	opInvalidPaymaster.PaymasterAndData = make([]byte, 20)
	if _, err := opInvalidPaymaster.ToPacked(); err != ErrInvalidPaymasterLength {
		t.Fatalf("Paymaster 長度 20 未報錯 ErrInvalidPaymasterLength: %v", err)
	}
}

// TestUserOp_Clone 驗證記憶體深拷貝獨立性
func TestUserOp_Clone(t *testing.T) {
	t.Parallel()
	orig := getOfficialTestOpV06()
	orig.PaymasterAndData = []byte{0x01, 0x02, 0x03}
	orig.Signature = []byte{0xaa, 0xbb}

	cp := orig.Clone()
	assertUserOpEqual(t, orig, cp)

	// 修改副本不應影響原始物件
	cp.Nonce.Add(cp.Nonce, big.NewInt(100))
	cp.CallData[0] = 0xff
	cp.PaymasterAndData[0] = 0xee
	cp.Signature[0] = 0xdd

	if orig.Nonce.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("原始物件 Nonce 被修改")
	}
	if orig.CallData[0] == 0xff {
		t.Fatalf("原始物件 CallData 被修改")
	}
	if orig.PaymasterAndData[0] == 0xee {
		t.Fatalf("原始物件 PaymasterAndData 被修改")
	}
	if orig.Signature[0] == 0xdd {
		t.Fatalf("原始物件 Signature 被修改")
	}

	// 測試 PackedUserOperation 深拷貝
	validOp := getOfficialTestOpV06()
	packedOrig, err := validOp.ToPacked()
	if err != nil {
		t.Fatalf("ToPacked 失敗: %v", err)
	}
	packedOrig.PaymasterAndData = []byte{0x01, 0x02, 0x03}
	packedCp := packedOrig.Clone()
	assertPackedUserOpEqual(t, packedOrig, packedCp)
	packedCp.Nonce.Add(packedCp.Nonce, big.NewInt(50))
	packedCp.PaymasterAndData[0] = 0xff
	if packedOrig.Nonce.Cmp(big.NewInt(0)) != 0 {
		t.Fatalf("原始 Packed 物件 Nonce 被修改")
	}
	if packedOrig.PaymasterAndData[0] == 0xff {
		t.Fatalf("原始 Packed 物件 PaymasterAndData 被修改")
	}
}

// TestUserOp_JSONRoundTrip_Standard 驗證標準 UserOp 之 JSON 雙向無損序列化
func TestUserOp_JSONRoundTrip_Standard(t *testing.T) {
	t.Parallel()
	orig := getOfficialTestOpV06()
	orig.Signature = mustHex("0x8df0873ef40e3dccc92d4f723196a448af8cfcb83a53fbb59d041370fcb510093f05cff054024c8bc35b7b49a4647c6ec5c49865ede12847a635434454702e981c")

	jsonData, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("MarshalJSON 失敗: %v", err)
	}

	// 檢查所有數值與切片均具備 0x 前綴
	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"nonce":"0x0"`) {
		t.Fatalf("nonce 格式不符: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"callGasLimit":"0x186a0"`) {
		t.Fatalf("callGasLimit 格式不符: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"initCode":"0x"`) {
		t.Fatalf("空 initCode 格式不符: %s", jsonStr)
	}

	var restored UserOperation
	if err := json.Unmarshal(jsonData, &restored); err != nil {
		t.Fatalf("UnmarshalJSON 失敗: %v", err)
	}

	assertUserOpEqual(t, orig, &restored)
}

// TestPackedUserOp_JSONRoundTrip 驗證 PackedUserOperation 之 JSON 雙向無損序列化
func TestPackedUserOp_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()
	packedOrig, err := op.ToPacked()
	if err != nil {
		t.Fatalf("ToPacked 失敗: %v", err)
	}

	jsonData, err := json.Marshal(packedOrig)
	if err != nil {
		t.Fatalf("MarshalJSON 失敗: %v", err)
	}

	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"accountGasLimits":"0x000000000000000000000000000249f0000000000000000000000000000186a0"`) {
		t.Fatalf("accountGasLimits 未序列化為 66 字元 hex: %s", jsonStr)
	}

	var restored PackedUserOperation
	if err := json.Unmarshal(jsonData, &restored); err != nil {
		t.Fatalf("UnmarshalJSON 失敗: %v", err)
	}

	assertPackedUserOpEqual(t, packedOrig, &restored)
}

// TestUserOp_JSON_MalformedRejection 驗證畸形 JSON 輸入拒絕機制
func TestUserOp_JSON_MalformedRejection(t *testing.T) {
	t.Parallel()

	// 1. 缺少 0x 前綴報錯
	jsonMissingPrefix := `{"sender":"0x1111111111111111111111111111111111111111","nonce":"100"}`
	var op1 UserOperation
	if err := json.Unmarshal([]byte(jsonMissingPrefix), &op1); err == nil {
		t.Fatalf("缺少 0x 前綴應拒絕反序列化")
	}

	// 2. 非法十六進位字符報錯
	jsonInvalidHex := `{"sender":"0x1111111111111111111111111111111111111111","nonce":"0xZZ"}`
	var op2 UserOperation
	if err := json.Unmarshal([]byte(jsonInvalidHex), &op2); err == nil {
		t.Fatalf("非法 hex 字符應拒絕反序列化")
	}

	// 3. 原生數字型別報錯
	jsonRawNumber := `{"sender":"0x1111111111111111111111111111111111111111","nonce":100}`
	var op3 UserOperation
	if err := json.Unmarshal([]byte(jsonRawNumber), &op3); err == nil {
		t.Fatalf("原生數字型別應拒絕反序列化")
	}
}

// TestUserOp_JSON_CaseInsensitiveHex 驗證大寫十六進位前綴支援
func TestUserOp_JSON_CaseInsensitiveHex(t *testing.T) {
	t.Parallel()
	jsonUpper := `{
		"sender":"0x1111111111111111111111111111111111111111",
		"nonce":"0X1A",
		"initCode":"0X",
		"callData":"0X1234",
		"callGasLimit":"0X10",
		"verificationGasLimit":"0X20",
		"preVerificationGas":"0X30",
		"maxFeePerGas":"0X40",
		"maxPriorityFeePerGas":"0X50",
		"paymasterAndData":"0X",
		"signature":"0X"
	}`

	var op UserOperation
	if err := json.Unmarshal([]byte(jsonUpper), &op); err != nil {
		t.Fatalf("大寫 hex 解析失敗: %v", err)
	}

	if op.Nonce.Int64() != 26 {
		t.Fatalf("nonce 預期 26, 實際 %d", op.Nonce.Int64())
	}
	if op.CallGasLimit.Int64() != 16 {
		t.Fatalf("callGasLimit 預期 16, 實際 %d", op.CallGasLimit.Int64())
	}
	if !bytes.Equal(op.CallData, []byte{0x12, 0x34}) {
		t.Fatalf("callData 解析不符: %x", op.CallData)
	}
}

// TestUserOpTuple_PackUnpack 驗證 ABI Tuple 結構打包與解包還原
func TestUserOpTuple_PackUnpack(t *testing.T) {
	t.Parallel()
	orig := getOfficialTestOpV06()
	orig.Signature = []byte{0xaa, 0xbb}

	tupleBytes, err := PackUserOpTuple(orig)
	if err != nil {
		t.Fatalf("PackUserOpTuple 失敗: %v", err)
	}

	restored, err := UnpackUserOpTuple(tupleBytes)
	if err != nil {
		t.Fatalf("UnpackUserOpTuple 失敗: %v", err)
	}

	assertUserOpEqual(t, orig, restored)
}

// TestUserOp_ConcurrentHashCalculation 驗證 100 goroutines 並發計算雜湊之執行緒安全
func TestUserOp_ConcurrentHashCalculation(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()
	ep := CanonicalEntryPointV06
	chainID := big.NewInt(1)

	const goroutines = 100
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	expectedHash := common.HexToHash("0xf09f5c22caf3315e500bd48ed65191b98ae05ef7840b2e4fabe9b0753e60fc51")

	for i := 0; i < goroutines; i += 1 {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j += 1 {
				hash, err := GetUserOpHash(op, ep, chainID)
				if err != nil {
					t.Errorf("非預期的錯誤: %v", err)
					return
				}
				if hash != expectedHash {
					t.Errorf("雜湊不符: 實際 %s, 預期 %s", hash.Hex(), expectedHash.Hex())
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestUserOp_ConcurrentSerialization 驗證 100 goroutines 並發執行 JSON 與 ABI 序列化
func TestUserOp_ConcurrentSerialization(t *testing.T) {
	t.Parallel()
	op := getOfficialTestOpV06()

	const goroutines = 100
	const iterations = 30

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i += 1 {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j += 1 {
				// 1. JSON Marshal
				data, err := json.Marshal(op)
				if err != nil {
					t.Errorf("並行 MarshalJSON 失敗: %v", err)
					return
				}

				// 2. JSON Unmarshal
				var restored UserOperation
				if err := json.Unmarshal(data, &restored); err != nil {
					t.Errorf("並行 UnmarshalJSON 失敗: %v", err)
					return
				}

				// 3. PackUserOp
				if _, err := PackUserOp(op); err != nil {
					t.Errorf("並行 PackUserOp 失敗: %v", err)
					return
				}

				// 4. ToPacked
				packed, err := op.ToPacked()
				if err != nil {
					t.Errorf("並行 ToPacked 失敗: %v", err)
					return
				}

				// 5. PackPackedUserOp
				if _, err := PackPackedUserOp(packed); err != nil {
					t.Errorf("並行 PackPackedUserOp 失敗: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
