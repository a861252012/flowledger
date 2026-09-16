package erc4337

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestBuilder_BuildAndSign(t *testing.T) {
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("生成私鑰失敗: %v", err)
	}
	signer, err := NewPrivateKeySigner(privKey)
	if err != nil {
		t.Fatalf("建立簽署者失敗: %v", err)
	}

	entryPoint := EntryPointV06
	chainID := big.NewInt(11155111)

	builder := NewBuilder(entryPoint, chainID)
	builder.SetSender(signer.Address()).
		SetNonce(big.NewInt(1)).
		SetGasLimits(big.NewInt(150000), big.NewInt(100000), big.NewInt(45000)).
		SetGasFees(big.NewInt(2500000000), big.NewInt(1500000000))

	target := common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8")
	value := big.NewInt(10000000000000000) // 0.01 ETH
	data := []byte{0xaa, 0xbb, 0xcc}

	_, err = builder.SetExecuteCallData(target, value, data)
	if err != nil {
		t.Fatalf("SetExecuteCallData 失敗: %v", err)
	}

	// 測試估算 PreVerificationGas
	estGas := builder.EstimatePreVerificationGas(nil)
	if estGas.Cmp(DefaultPreVerificationGasOverhead) <= 0 {
		t.Fatalf("估算之 preVerificationGas 應大於 overhead，得到 %s", estGas.String())
	}

	op, err := builder.BuildAndSign(signer)
	if err != nil {
		t.Fatalf("BuildAndSign 失敗: %v", err)
	}

	if op.Sender != signer.Address() {
		t.Fatalf("Sender 不符: 預期 %s, 得到 %s", signer.Address().Hex(), op.Sender.Hex())
	}
	if len(op.Signature) != 65 {
		t.Fatalf("簽章長度必須為 65，得到 %d", len(op.Signature))
	}

	// 驗證簽名正確性
	hash, err := GetUserOpHash(op, entryPoint, chainID)
	if err != nil {
		t.Fatalf("計算 hash 失敗: %v", err)
	}

	valid, err := VerifySignature(hash, op.Signature, signer.Address())
	if err != nil || !valid {
		t.Fatalf("還原 UserOp 簽名失敗")
	}
}

func TestBuilder_ValidationFailures(t *testing.T) {
	entryPoint := EntryPointV06
	chainID := big.NewInt(11155111)

	// 缺乏 sender
	b := NewBuilder(entryPoint, chainID)
	_, err := b.Build()
	if err != ErrInvalidSender {
		t.Fatalf("缺乏 sender 應回傳 ErrInvalidSender，得到 %v", err)
	}

	// 零 entryPoint
	b = NewBuilder(common.Address{}, chainID)
	b.SetSender(common.HexToAddress("0x1234567890123456789012345678901234567890"))
	_, err = b.Build()
	if err != ErrInvalidEntryPoint {
		t.Fatalf("零 entryPoint 應回傳 ErrInvalidEntryPoint，得到 %v", err)
	}

	// 無效 chainID
	b = NewBuilder(entryPoint, big.NewInt(0))
	b.SetSender(common.HexToAddress("0x1234567890123456789012345678901234567890"))
	_, err = b.Build()
	if err != ErrInvalidChainID {
		t.Fatalf("零 chainID 應回傳 ErrInvalidChainID，得到 %v", err)
	}

	// PriorityFee > MaxFee
	b = NewBuilder(entryPoint, chainID)
	b.SetSender(common.HexToAddress("0x1234567890123456789012345678901234567890")).
		SetGasFees(big.NewInt(100), big.NewInt(200))
	_, err = b.Build()
	if err != ErrGasFeeInconsistent {
		t.Fatalf("費用不一致應回傳 ErrGasFeeInconsistent，得到 %v", err)
	}

	// 負數轉帳值
	b = NewBuilder(entryPoint, chainID)
	_, err = b.SetExecuteCallData(common.Address{}, big.NewInt(-1), nil)
	if err != ErrInvalidExecuteVal {
		t.Fatalf("負數轉帳金額應回傳 ErrInvalidExecuteVal，得到 %v", err)
	}
}

func TestBuilder_AllSettersAndSign(t *testing.T) {
	entryPoint := EntryPointV06
	chainID := big.NewInt(1)
	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")

	initCode := []byte{0x01, 0x02, 0x03, 0x04}
	callData := []byte{0xa9, 0x05, 0x9c, 0xbb}
	paymasterAndData := []byte{0xde, 0xad, 0xbe, 0xef}
	signature := make([]byte, 65)
	signature[64] = 27

	b := NewBuilder(entryPoint, chainID).
		SetSender(sender).
		SetNonce(big.NewInt(42)).
		SetInitCode(initCode).
		SetCallData(callData).
		SetGasLimits(big.NewInt(100000), big.NewInt(200000), big.NewInt(30000)).
		SetGasFees(big.NewInt(5000000000), big.NewInt(2000000000)).
		SetPaymasterAndData(paymasterAndData).
		SetSignature(signature)

	op, err := b.Build()
	if err != nil {
		t.Fatalf("Build 失敗: %v", err)
	}

	if !bytes.Equal(op.InitCode, initCode) {
		t.Fatalf("InitCode 不符")
	}
	if !bytes.Equal(op.CallData, callData) {
		t.Fatalf("CallData 不符")
	}
	if !bytes.Equal(op.PaymasterAndData, paymasterAndData) {
		t.Fatalf("PaymasterAndData 不符")
	}
	if !bytes.Equal(op.Signature, signature) {
		t.Fatalf("Signature 不符")
	}

	// 驗證深拷貝隔離
	initCode[0] = 0xff
	callData[0] = 0xff
	paymasterAndData[0] = 0xff
	signature[0] = 0xff

	if op.InitCode[0] == 0xff || op.CallData[0] == 0xff || op.PaymasterAndData[0] == 0xff || op.Signature[0] == 0xff {
		t.Fatalf("深拷貝隔離失效，外部修改影響了 UserOperation 內容")
	}
}

func TestBuilder_SetExecuteCallData_EdgeCases(t *testing.T) {
	b := NewBuilder(EntryPointV06, big.NewInt(1))
	target := common.HexToAddress("0x2222222222222222222222222222222222222222")

	// 1. value 為 nil，自動轉為 0
	_, err := b.SetExecuteCallData(target, nil, []byte{0x12})
	if err != nil {
		t.Fatalf("value 為 nil 應成功打包: %v", err)
	}
	if !bytes.Equal(b.callData[:4], executeMethodID) {
		t.Fatalf("選擇器不符，預期 %x，得到 %x", executeMethodID, b.callData[:4])
	}

	// 2. data 為 nil，空 payload
	_, err = b.SetExecuteCallData(target, big.NewInt(0), nil)
	if err != nil {
		t.Fatalf("data 為 nil 應成功打包: %v", err)
	}

	// 3. 最大 uint256 金額
	maxUint256Val := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	_, err = b.SetExecuteCallData(target, maxUint256Val, []byte("transfer"))
	if err != nil {
		t.Fatalf("最大 uint256 應成功打包: %v", err)
	}

	// 4. ABI 反向解包驗證
	unpacked, err := executeArguments.Unpack(b.callData[4:])
	if err != nil {
		t.Fatalf("解包 execute calldata 失敗: %v", err)
	}
	if len(unpacked) != 3 {
		t.Fatalf("解包參數個數不符: %d", len(unpacked))
	}
	unpackedTarget := unpacked[0].(common.Address)
	unpackedValue := unpacked[1].(*big.Int)
	unpackedData := unpacked[2].([]byte)

	if unpackedTarget != target {
		t.Fatalf("解包目標不符")
	}
	if unpackedValue.Cmp(maxUint256Val) != 0 {
		t.Fatalf("解包金額不符")
	}
	if !bytes.Equal(unpackedData, []byte("transfer")) {
		t.Fatalf("解包資料不符")
	}
}

func TestBuilder_SetExecuteBatchCallData(t *testing.T) {
	b := NewBuilder(EntryPointV06, big.NewInt(1))

	targets := []common.Address{
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
	}
	values := []*big.Int{
		big.NewInt(1000),
		nil, // 應自動正規化為 0
	}
	datas := [][]byte{
		[]byte{0x01, 0x02},
		[]byte{0x03, 0x04, 0x05},
	}

	_, err := b.SetExecuteBatchCallData(targets, values, datas)
	if err != nil {
		t.Fatalf("SetExecuteBatchCallData 失敗: %v", err)
	}

	// 驗證 selector 為 executeBatchMethodID (0x47e1da2a)
	if !bytes.Equal(b.callData[:4], executeBatchMethodID) {
		t.Fatalf("executeBatch 選擇器不符，預期 %x，得到 %x", executeBatchMethodID, b.callData[:4])
	}

	// ABI 解包驗證
	unpacked, err := executeBatchArguments.Unpack(b.callData[4:])
	if err != nil {
		t.Fatalf("解包 executeBatch 失敗: %v", err)
	}
	unpackedTargets := unpacked[0].([]common.Address)
	unpackedValues := unpacked[1].([]*big.Int)
	unpackedDatas := unpacked[2].([][]byte)

	if len(unpackedTargets) != 2 || len(unpackedValues) != 2 || len(unpackedDatas) != 2 {
		t.Fatalf("解包批次數量不符")
	}
	if unpackedTargets[0] != targets[0] || unpackedTargets[1] != targets[1] {
		t.Fatalf("解包批次目標不符")
	}
	if unpackedValues[0].Int64() != 1000 || unpackedValues[1].Int64() != 0 {
		t.Fatalf("解包批次金額不符")
	}
	if !bytes.Equal(unpackedDatas[0], datas[0]) || !bytes.Equal(unpackedDatas[1], datas[1]) {
		t.Fatalf("解包批次資料不符")
	}

	// 異常分支 1: 空批次
	_, err = b.SetExecuteBatchCallData(nil, nil, nil)
	if err != ErrEmptyBatch {
		t.Fatalf("空批次應回傳 ErrEmptyBatch，得到 %v", err)
	}

	// 異常分支 2: 長度不一致
	_, err = b.SetExecuteBatchCallData(targets, values[:1], datas)
	if err != ErrBatchLengthMismatch {
		t.Fatalf("長度不一致應回傳 ErrBatchLengthMismatch，得到 %v", err)
	}

	// 異常分支 3: 包含負數金額
	negValues := []*big.Int{big.NewInt(-1), big.NewInt(0)}
	_, err = b.SetExecuteBatchCallData(targets, negValues, datas)
	if err != ErrInvalidExecuteVal {
		t.Fatalf("負數金額應回傳 ErrInvalidExecuteVal，得到 %v", err)
	}
}

func TestBuilder_SetTransaction(t *testing.T) {
	b := NewBuilder(EntryPointV06, big.NewInt(11155111))

	// 1. tx 為空
	_, err := b.SetTransaction(nil)
	if err != ErrNilTransaction {
		t.Fatalf("nil 交易應回傳 ErrNilTransaction，得到 %v", err)
	}

	// 2. 合約部署交易（To 為 nil）
	deployTx := types.NewContractCreation(1, big.NewInt(0), 100000, big.NewInt(1000000000), []byte{0x60, 0x80})
	_, err = b.SetTransaction(deployTx)
	if err != ErrContractCreationTx {
		t.Fatalf("合約部署交易應回傳 ErrContractCreationTx，得到 %v", err)
	}

	// 3. EIP-1559 動態費用交易
	to := common.HexToAddress("0x3333333333333333333333333333333333333333")
	dynamicTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(11155111),
		Nonce:     5,
		GasTipCap: big.NewInt(1500000000),
		GasFeeCap: big.NewInt(3000000000),
		Gas:       80000,
		To:        &to,
		Value:     big.NewInt(500000),
		Data:      []byte{0xaa, 0xbb},
	})

	_, err = b.SetTransaction(dynamicTx)
	if err != nil {
		t.Fatalf("SetTransaction 失敗: %v", err)
	}

	if b.nonce.Int64() != 5 {
		t.Fatalf("Nonce 設定不符，預期 5，得到 %s", b.nonce.String())
	}
	if b.callGasLimit.Int64() != 80000 {
		t.Fatalf("callGasLimit 不符，預期 80000，得到 %s", b.callGasLimit.String())
	}
	if b.maxFeePerGas.Int64() != 3000000000 {
		t.Fatalf("maxFeePerGas 不符")
	}
	if b.maxPriorityFeePerGas.Int64() != 1500000000 {
		t.Fatalf("maxPriorityFeePerGas 不符")
	}

	// 4. Legacy 交易
	legacyTx := types.NewTx(&types.LegacyTx{
		Nonce:    10,
		GasPrice: big.NewInt(2000000000),
		Gas:      50000,
		To:       &to,
		Value:    big.NewInt(100),
		Data:     []byte{0xcc},
	})
	bLegacy := NewBuilder(EntryPointV06, big.NewInt(11155111))
	_, err = bLegacy.SetTransaction(legacyTx)
	if err != nil {
		t.Fatalf("Legacy SetTransaction 失敗: %v", err)
	}
	if bLegacy.maxFeePerGas.Int64() != 2000000000 || bLegacy.maxPriorityFeePerGas.Int64() != 2000000000 {
		t.Fatalf("Legacy gas fees 不符")
	}
}

func TestBuilder_EstimatePreVerificationGas_Precise(t *testing.T) {
	b := NewBuilder(EntryPointV06, big.NewInt(1))
	sender := common.HexToAddress("0x0000000000000000000000000000000000000001") // 19 個 0, 1 個非 0
	b.SetSender(sender)

	// 設定已知固定欄位值
	b.SetNonce(big.NewInt(1))               // 32 bytes word: 31 個 0, 1 個非 0
	b.SetGasLimits(big.NewInt(0), nil, nil) // callGasLimit 32 個 0; verificationGasLimit 32 個 0; preVerificationGas 初始 32 個 0
	b.SetGasFees(big.NewInt(0), nil)        // maxFee 32 個 0; maxPriorityFee 32 個 0

	// 簽名未設定：預設 65 個非零位元組
	// 動態欄位：
	// callData: 2 個 0, 2 個非 0
	b.SetCallData([]byte{0x00, 0x00, 0x01, 0x02})
	// initCode: 1 個 0, 1 個非 0
	b.SetInitCode([]byte{0x00, 0x05})
	// paymasterAndData: 3 個 0, 1 個非 0
	b.SetPaymasterAndData([]byte{0x00, 0x00, 0x00, 0x09})

	// 固定欄位 0 與非 0 統計：
	// sender: 19 zeros, 1 non-zero
	// nonce: 31 zeros, 1 non-zero
	// callGasLimit: 32 zeros, 0 non-zero
	// verificationGasLimit: 32 zeros, 0 non-zero
	// maxFeePerGas: 32 zeros, 0 non-zero
	// maxPriorityFeePerGas: 32 zeros, 0 non-zero
	// 簽名 (預設 65 non-zeros): 0 zeros, 65 non-zeros
	// 動態欄位 zeros = 2 + 1 + 3 = 6 zeros
	// 動態欄位 non-zeros = 2 + 1 + 1 = 4 non-zeros
	//
	// 第一輪初步計算 (preVerificationGas 為 0，即 32 個 0):
	// 固定欄位合計 zeros = 19 + 31 + 32*5 = 210 zeros
	// 總 zeros = 210 + 6 = 216 zeros
	// 總 non-zeros = 2 + 65 + 4 = 71 non-zeros
	// data cost = 216 * 4 + 71 * 16 = 864 + 1136 = 2000
	// 基礎 overhead = 21000
	// 初步估算 = 23000 (0x59d8，佔 2 個 non-zeros，30 個 zeros)
	//
	// 第二輪自收斂計算 (preVerificationGas 更新為 23000):
	// preVerificationGasWord 轉為 30 個 zeros + 2 個 non-zeros
	// 總 zeros = 214 zeros, 總 non-zeros = 73 non-zeros
	// data cost = 214 * 4 + 73 * 16 = 856 + 1168 = 2024
	// 總預期 Gas = 21000 + 2024 = 23024

	overhead := big.NewInt(21000)
	estGas := b.EstimatePreVerificationGas(overhead)

	expectedGas := big.NewInt(23024)
	if estGas.Cmp(expectedGas) != 0 {
		t.Fatalf("EstimatePreVerificationGas 精確數值不符: 預期 %s, 得到 %s", expectedGas.String(), estGas.String())
	}

	// 驗證 CalcPreVerificationGas
	op, err := b.Build()
	if err != nil {
		t.Fatalf("Build 失敗: %v", err)
	}
	calcGas := CalcPreVerificationGas(op, overhead)
	if calcGas.Cmp(expectedGas) != 0 {
		t.Fatalf("CalcPreVerificationGas 結果不符: 預期 %s, 得到 %s", expectedGas.String(), calcGas.String())
	}

	// 自訂 overhead 驗證
	customOverhead := big.NewInt(50000)
	estCustom := b.EstimatePreVerificationGas(customOverhead)
	if estCustom.Cmp(big.NewInt(52024)) != 0 {
		t.Fatalf("自訂 overhead 結果不符，預期 52024，得到 %s", estCustom.String())
	}
}

func TestBuilder_FeeEstimation(t *testing.T) {
	baseFee := big.NewInt(20000000000)    // 20 Gwei
	priorityFee := big.NewInt(2000000000) // 2 Gwei

	maxFee, tip, err := CalculateEIP1559Fees(baseFee, priorityFee, 2)
	if err != nil {
		t.Fatalf("CalculateEIP1559Fees 失敗: %v", err)
	}
	// 預期 maxFee = 2 * 20 Gwei + 2 Gwei = 42 Gwei
	expectedMaxFee := big.NewInt(42000000000)
	if maxFee.Cmp(expectedMaxFee) != 0 {
		t.Fatalf("預估 maxFee 不符，預期 %s，得到 %s", expectedMaxFee.String(), maxFee.String())
	}
	if tip.Cmp(priorityFee) != 0 {
		t.Fatalf("預估 tip 不符")
	}

	// 測試 Builder 的 EstimateGasFees
	b := NewBuilder(EntryPointV06, big.NewInt(1))
	b.EstimateGasFees(baseFee, priorityFee)
	if b.maxFeePerGas.Cmp(expectedMaxFee) != 0 || b.maxPriorityFeePerGas.Cmp(priorityFee) != 0 {
		t.Fatalf("Builder EstimateGasFees 設定不符")
	}

	// 測試邊界：priorityFee 為 nil（自動使用預設 1 Gwei）
	bDefault := NewBuilder(EntryPointV06, big.NewInt(1))
	bDefault.EstimateGasFees(baseFee, nil)
	expectedDefaultMaxFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), DefaultPriorityFeePerGas)
	if bDefault.maxFeePerGas.Cmp(expectedDefaultMaxFee) != 0 {
		t.Fatalf("預設 tip 費用不符")
	}

	// 異常 multiplier
	_, _, err = CalculateEIP1559Fees(baseFee, priorityFee, 0)
	if err != ErrInvalidMultiplier {
		t.Fatalf("multiplier <= 0 應報錯 ErrInvalidMultiplier，得到 %v", err)
	}
}

func TestBuilder_BuildAndSign_NilSigner(t *testing.T) {
	b := NewBuilder(EntryPointV06, big.NewInt(1)).
		SetSender(common.HexToAddress("0x1111111111111111111111111111111111111111"))

	_, err := b.BuildAndSign(nil)
	if err == nil || err.Error() != "erc4337: signer 不得為空" {
		t.Fatalf("nil signer 應回傳明確錯誤，得到 %v", err)
	}
}

func TestBuilder_BuildAndSign_V07(t *testing.T) {
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("生成私鑰失敗: %v", err)
	}
	signer, err := NewPrivateKeySigner(privKey)
	if err != nil {
		t.Fatalf("建立簽署者失敗: %v", err)
	}

	entryPoint := CanonicalEntryPointV07
	chainID := big.NewInt(1)

	b := NewBuilder(entryPoint, chainID).
		SetSender(signer.Address()).
		SetNonce(big.NewInt(0)).
		SetGasLimits(big.NewInt(50000), big.NewInt(60000), big.NewInt(21000)).
		SetGasFees(big.NewInt(2000000000), big.NewInt(1000000000))

	op, err := b.BuildAndSign(signer)
	if err != nil {
		t.Fatalf("v0.7 BuildAndSign 失敗: %v", err)
	}

	// 驗證簽章
	hash, err := GetUserOpHash(op, entryPoint, chainID)
	if err != nil {
		t.Fatalf("計算 v0.7 hash 失敗: %v", err)
	}

	valid, err := VerifySignature(hash, op.Signature, signer.Address())
	if err != nil || !valid {
		t.Fatalf("v0.7 簽名驗證失敗")
	}
}
