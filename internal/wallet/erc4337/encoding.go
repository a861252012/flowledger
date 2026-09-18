package erc4337

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	typeAddress, _ = abi.NewType("address", "", nil)
	typeUint256, _ = abi.NewType("uint256", "", nil)
	typeBytes32, _ = abi.NewType("bytes32", "", nil)
)

// userOpV06HashArgs 用於 v0.6 UserOperation 內部打包 (320 位元組)
var userOpV06HashArgs = abi.Arguments{
	{Type: typeAddress}, // sender
	{Type: typeUint256}, // nonce
	{Type: typeBytes32}, // keccak256(initCode)
	{Type: typeBytes32}, // keccak256(callData)
	{Type: typeUint256}, // callGasLimit
	{Type: typeUint256}, // verificationGasLimit
	{Type: typeUint256}, // preVerificationGas
	{Type: typeUint256}, // maxFeePerGas
	{Type: typeUint256}, // maxPriorityFeePerGas
	{Type: typeBytes32}, // keccak256(paymasterAndData)
}

// userOpV07HashArgs 用於 v0.7 PackedUserOperation 內部打包 (256 位元組)
var userOpV07HashArgs = abi.Arguments{
	{Type: typeAddress}, // sender
	{Type: typeUint256}, // nonce
	{Type: typeBytes32}, // keccak256(initCode)
	{Type: typeBytes32}, // keccak256(callData)
	{Type: typeBytes32}, // accountGasLimits
	{Type: typeUint256}, // preVerificationGas
	{Type: typeBytes32}, // gasFees
	{Type: typeBytes32}, // keccak256(paymasterAndData)
}

// userOpOuterHashArgs 用於外層最終 UserOpHash 計算 (96 位元組)
var userOpOuterHashArgs = abi.Arguments{
	{Type: typeBytes32}, // innerHash
	{Type: typeAddress}, // entryPoint
	{Type: typeUint256}, // chainId
}

// userOpTupleType 定義 UserOperation 作為 ABI Tuple 結構
var userOpTupleType, _ = abi.NewType("tuple", "struct UserOperation", []abi.ArgumentMarshaling{
	{Name: "sender", Type: "address"},
	{Name: "nonce", Type: "uint256"},
	{Name: "initCode", Type: "bytes"},
	{Name: "callData", Type: "bytes"},
	{Name: "callGasLimit", Type: "uint256"},
	{Name: "verificationGasLimit", Type: "uint256"},
	{Name: "preVerificationGas", Type: "uint256"},
	{Name: "maxFeePerGas", Type: "uint256"},
	{Name: "maxPriorityFeePerGas", Type: "uint256"},
	{Name: "paymasterAndData", Type: "bytes"},
	{Name: "signature", Type: "bytes"},
})

var userOpTupleArgs = abi.Arguments{
	{Type: userOpTupleType},
}

// PackUint128Pair 將兩個 uint128 大數緊湊打包進 32 位元組陣列
// high 填入 bytes [0:16]，low 填入 bytes [16:32]
func PackUint128Pair(high, low *big.Int) ([32]byte, error) {
	var out [32]byte
	if high == nil || low == nil {
		return out, ErrNilField
	}
	if high.Sign() < 0 || low.Sign() < 0 {
		return out, ErrNegativeValue
	}
	if high.Cmp(maxUint128) > 0 || low.Cmp(maxUint128) > 0 {
		return out, ErrGasLimitOverflow
	}

	highBytes := high.Bytes()
	lowBytes := low.Bytes()

	copy(out[16-len(highBytes):16], highBytes)
	copy(out[32-len(lowBytes):32], lowBytes)
	return out, nil
}

// UnpackUint128Pair 將 32 位元組陣列解碼還原為兩個 uint128 大數
func UnpackUint128Pair(packed [32]byte) (*big.Int, *big.Int) {
	high := new(big.Int).SetBytes(packed[0:16])
	low := new(big.Int).SetBytes(packed[16:32])
	return high, low
}

// PackUserOpForHashV06 對 UserOperation 進行 v0.6 標準 10 欄位 ABI 打包 (320 位元組)
func PackUserOpForHashV06(op *UserOperation) ([]byte, error) {
	if op == nil {
		return nil, ErrNilUserOp
	}
	if err := op.Validate(); err != nil {
		return nil, err
	}

	hashInitCode := crypto.Keccak256Hash(op.InitCode)
	hashCallData := crypto.Keccak256Hash(op.CallData)
	hashPaymasterAndData := crypto.Keccak256Hash(op.PaymasterAndData)

	return userOpV06HashArgs.Pack(
		op.Sender,
		op.Nonce,
		hashInitCode,
		hashCallData,
		op.CallGasLimit,
		op.VerificationGasLimit,
		op.PreVerificationGas,
		op.MaxFeePerGas,
		op.MaxPriorityFeePerGas,
		hashPaymasterAndData,
	)
}

// PackUserOpForHashV07 對 PackedUserOperation 進行 v0.7 標準 8 欄位 ABI 打包 (256 位元組)
func PackUserOpForHashV07(op *PackedUserOperation) ([]byte, error) {
	if err := op.Validate(); err != nil {
		return nil, err
	}

	hashInitCode := crypto.Keccak256Hash(op.InitCode)
	hashCallData := crypto.Keccak256Hash(op.CallData)
	hashPaymasterAndData := crypto.Keccak256Hash(op.PaymasterAndData)

	return userOpV07HashArgs.Pack(
		op.Sender,
		op.Nonce,
		hashInitCode,
		hashCallData,
		op.AccountGasLimits,
		op.PreVerificationGas,
		op.GasFees,
		hashPaymasterAndData,
	)
}

// PackOuterUserOpHash 對 innerHash, entryPoint, chainId 進行外層 ABI 打包 (96 位元組)
func PackOuterUserOpHash(innerHash common.Hash, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	if entryPoint == (common.Address{}) {
		return nil, ErrInvalidEntryPoint
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, ErrInvalidChainID
	}
	if chainID.BitLen() > 256 {
		return nil, ErrUint256Overflow
	}
	return userOpOuterHashArgs.Pack(innerHash, entryPoint, chainID)
}

// PackUserOpTuple 將 UserOperation 結構體打包為 ABI Tuple 位元組切片
func PackUserOpTuple(op *UserOperation) ([]byte, error) {
	if op == nil {
		return nil, ErrNilUserOp
	}
	if err := op.Validate(); err != nil {
		return nil, err
	}
	return userOpTupleArgs.Pack(op)
}

// UnpackUserOpTuple 將 ABI Tuple 位元組切片解碼還原為 UserOperation 結構體
func UnpackUserOpTuple(data []byte) (*UserOperation, error) {
	out, err := userOpTupleArgs.Unpack(data)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("erc4337: unpack tuple 失敗，輸出長度為 0")
	}

	val, ok := abi.ConvertType(out[0], new(UserOperation)).(*UserOperation)
	if !ok {
		return nil, errors.New("erc4337: unpack tuple 型別轉換失敗")
	}
	return val, nil
}
