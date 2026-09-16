package erc4337

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	// InnerPackLengthV06 定義 v0.6 內部打包位元組長度 (10 個 32-byte words)
	InnerPackLengthV06 = 320

	// InnerPackLengthV07 定義 v0.7 內部打包位元組長度 (8 個 32-byte words)
	InnerPackLengthV07 = 256

	// OuterPackLength 定義外層打包位元組長度 (3 個 32-byte words)
	OuterPackLength = 96
)

// fillUint256Word 將 big.Int 大端序寫入 32 位元組目標切片中，並進行非負與 256 位元邊界檢查
func fillUint256Word(dest []byte, val *big.Int) error {
	if val == nil {
		return ErrNilField
	}
	if val.Sign() < 0 {
		return ErrNegativeValue
	}
	if val.BitLen() > 256 {
		return ErrUint256Overflow
	}
	for i := 0; i < 32; i += 1 {
		dest[i] = 0
	}
	bytes := val.Bytes()
	copy(dest[32-len(bytes):32], bytes)
	return nil
}

// PackUserOp 對 UserOperation 進行標準 v0.6 內部 ABI 打包 (320 位元組)
// 符合 PROJECT.md 介面契約規範
func PackUserOp(userOp *UserOperation) ([]byte, error) {
	if userOp == nil {
		return nil, ErrNilUserOp
	}
	if err := userOp.Validate(); err != nil {
		return nil, err
	}

	buf := make([]byte, InnerPackLengthV06)

	// Word 0 [0:32]: sender (左補 12 位元組 0x00)
	copy(buf[12:32], userOp.Sender.Bytes())

	// Word 1 [32:64]: nonce
	if err := fillUint256Word(buf[32:64], userOp.Nonce); err != nil {
		return nil, err
	}

	// Word 2 [64:96]: keccak256(initCode)
	hashInitCode := crypto.Keccak256(userOp.InitCode)
	copy(buf[64:96], hashInitCode)

	// Word 3 [96:128]: keccak256(callData)
	hashCallData := crypto.Keccak256(userOp.CallData)
	copy(buf[96:128], hashCallData)

	// Word 4 [128:160]: callGasLimit
	if err := fillUint256Word(buf[128:160], userOp.CallGasLimit); err != nil {
		return nil, err
	}

	// Word 5 [160:192]: verificationGasLimit
	if err := fillUint256Word(buf[160:192], userOp.VerificationGasLimit); err != nil {
		return nil, err
	}

	// Word 6 [192:224]: preVerificationGas
	if err := fillUint256Word(buf[192:224], userOp.PreVerificationGas); err != nil {
		return nil, err
	}

	// Word 7 [224:256]: maxFeePerGas
	if err := fillUint256Word(buf[224:256], userOp.MaxFeePerGas); err != nil {
		return nil, err
	}

	// Word 8 [256:288]: maxPriorityFeePerGas
	if err := fillUint256Word(buf[256:288], userOp.MaxPriorityFeePerGas); err != nil {
		return nil, err
	}

	// Word 9 [288:320]: keccak256(paymasterAndData)
	hashPaymasterAndData := crypto.Keccak256(userOp.PaymasterAndData)
	copy(buf[288:320], hashPaymasterAndData)

	return buf, nil
}

// PackPackedUserOp 對 PackedUserOperation 進行標準 v0.7 內部 ABI 打包 (256 位元組)
func PackPackedUserOp(userOp *PackedUserOperation) ([]byte, error) {
	if err := userOp.Validate(); err != nil {
		return nil, err
	}

	buf := make([]byte, InnerPackLengthV07)

	// Word 0 [0:32]: sender
	copy(buf[12:32], userOp.Sender.Bytes())

	// Word 1 [32:64]: nonce
	if err := fillUint256Word(buf[32:64], userOp.Nonce); err != nil {
		return nil, err
	}

	// Word 2 [64:96]: keccak256(initCode)
	hashInitCode := crypto.Keccak256(userOp.InitCode)
	copy(buf[64:96], hashInitCode)

	// Word 3 [96:128]: keccak256(callData)
	hashCallData := crypto.Keccak256(userOp.CallData)
	copy(buf[96:128], hashCallData)

	// Word 4 [128:160]: accountGasLimits
	copy(buf[128:160], userOp.AccountGasLimits[:])

	// Word 5 [160:192]: preVerificationGas
	if err := fillUint256Word(buf[160:192], userOp.PreVerificationGas); err != nil {
		return nil, err
	}

	// Word 6 [192:224]: gasFees
	copy(buf[192:224], userOp.GasFees[:])

	// Word 7 [224:256]: keccak256(paymasterAndData)
	hashPaymasterAndData := crypto.Keccak256(userOp.PaymasterAndData)
	copy(buf[224:256], hashPaymasterAndData)

	return buf, nil
}

// CalculateInnerHash 計算 v0.6 UserOperation 之內部雜湊 (userOp.hash())
func CalculateInnerHash(userOp *UserOperation) (common.Hash, error) {
	packed, err := PackUserOp(userOp)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(packed), nil
}

// CalculatePackedInnerHash 計算 v0.7 PackedUserOperation 之內部雜湊 (packedUserOp.hash())
func CalculatePackedInnerHash(userOp *PackedUserOperation) (common.Hash, error) {
	packed, err := PackPackedUserOp(userOp)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(packed), nil
}

// PackUserOpHashData 對 innerHash, entryPoint, chainId 進行外層 96 位元組 ABI 編碼
func PackUserOpHashData(innerHash common.Hash, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	if entryPoint == (common.Address{}) {
		return nil, ErrInvalidEntryPoint
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, ErrInvalidChainID
	}
	if chainID.BitLen() > 256 {
		return nil, ErrUint256Overflow
	}

	buf := make([]byte, OuterPackLength)

	// Word 0 [0:32]: innerHash
	copy(buf[0:32], innerHash.Bytes())

	// Word 1 [32:64]: entryPoint (左補 12 位元組 0x00，右側 20 位元組為地址)
	copy(buf[44:64], entryPoint.Bytes())

	// Word 2 [64:96]: chainID (大端序 32 位元組)
	if err := fillUint256Word(buf[64:96], chainID); err != nil {
		return nil, err
	}

	return buf, nil
}

// GetUserOpHash 計算標準 UserOpHash
// 支援 EntryPoint v0.6 與 v0.7 自動適配：若 entryPoint 為 CanonicalEntryPointV07，
// 則自動轉換為 PackedUserOperation 並依照 v0.7 規範計算；否則依照 v0.6 規範計算。
func GetUserOpHash(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) (common.Hash, error) {
	if userOp == nil {
		return common.Hash{}, ErrNilUserOp
	}
	if entryPoint == (common.Address{}) {
		return common.Hash{}, ErrInvalidEntryPoint
	}
	if chainID == nil || chainID.Sign() <= 0 {
		return common.Hash{}, ErrInvalidChainID
	}

	if entryPoint == CanonicalEntryPointV07 {
		packed, err := userOp.ToPacked()
		if err != nil {
			return common.Hash{}, err
		}
		return GetUserOpHashV07(packed, entryPoint, chainID)
	}

	return GetUserOpHashV06(userOp, entryPoint, chainID)
}

// GetUserOpHashV06 明確以 ERC-4337 v0.6 規範計算 UserOpHash
func GetUserOpHashV06(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) (common.Hash, error) {
	innerHash, err := CalculateInnerHash(userOp)
	if err != nil {
		return common.Hash{}, err
	}

	outerBytes, err := PackUserOpHashData(innerHash, entryPoint, chainID)
	if err != nil {
		return common.Hash{}, err
	}

	return crypto.Keccak256Hash(outerBytes), nil
}

// GetUserOpHashV07 明確以 ERC-4337 v0.7 規範計算 PackedUserOpHash
func GetUserOpHashV07(userOp *PackedUserOperation, entryPoint common.Address, chainID *big.Int) (common.Hash, error) {
	innerHash, err := CalculatePackedInnerHash(userOp)
	if err != nil {
		return common.Hash{}, err
	}

	outerBytes, err := PackUserOpHashData(innerHash, entryPoint, chainID)
	if err != nil {
		return common.Hash{}, err
	}

	return crypto.Keccak256Hash(outerBytes), nil
}

// GetPackedUserOpHash 為 GetUserOpHashV07 之別名，提供符合 v0.7 命名風格之介面
func GetPackedUserOpHash(userOp *PackedUserOperation, entryPoint common.Address, chainID *big.Int) (common.Hash, error) {
	return GetUserOpHashV07(userOp, entryPoint, chainID)
}
