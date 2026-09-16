package erc4337

import (
	"encoding/json"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	// EntryPointV06AddressHex 為 ERC-4337 v0.6 官方合約地址十六進位字串
	EntryPointV06AddressHex = "0x5FF137D4b0FDCD49DcA30c7CF57E578a026d2789"

	// EntryPointV07AddressHex 為 ERC-4337 v0.7 官方合約地址十六進位字串
	EntryPointV07AddressHex = "0x0000000071727De22E5E9d8BAf0edAc6f37da032"
)

var (
	// EntryPointV06 為 EIP-4337 v0.6 官方權威合約地址
	EntryPointV06 = common.HexToAddress(EntryPointV06AddressHex)

	// EntryPointV07 為 EIP-4337 v0.7 官方權威合約地址
	EntryPointV07 = common.HexToAddress(EntryPointV07AddressHex)

	// CanonicalEntryPointV06 為 EntryPointV06 之別名
	CanonicalEntryPointV06 = EntryPointV06

	// CanonicalEntryPointV07 為 EntryPointV07 之別名
	CanonicalEntryPointV07 = EntryPointV07
)

var (
	// 最大 uint128 邊界值 (2^128 - 1)
	maxUint128 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))

	// 最大 uint256 邊界值 (2^256 - 1)
	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

var (
	ErrNilUserOp              = errors.New("erc4337: user operation 為空指標")
	ErrNilPackedUserOp        = errors.New("erc4337: packed user operation 為空指標")
	ErrInvalidSender          = errors.New("erc4337: sender 地址不得為零地址")
	ErrNilField               = errors.New("erc4337: 數值欄位不得為 nil")
	ErrNegativeValue          = errors.New("erc4337: 數值欄位不得為負數")
	ErrGasFeeInconsistent     = errors.New("erc4337: maxPriorityFeePerGas 不得大於 maxFeePerGas")
	ErrGasLimitOverflow       = errors.New("erc4337: gas limit 超出 uint128 最大上限")
	ErrGasFeeOverflow         = errors.New("erc4337: gas fee 超出 uint128 最大上限")
	ErrUint256Overflow        = errors.New("erc4337: 數值超出 uint256 最大上限")
	ErrInvalidPaymasterLength = errors.New("erc4337: v0.7 paymasterAndData 若存在則長度不得小於 52 位元組")
	ErrInvalidPackedGasLimits = errors.New("erc4337: packed gas limits 格式無效")
	ErrInvalidPackedGasFees   = errors.New("erc4337: packed gas fees 格式無效")
	ErrInvalidHexFormat       = errors.New("erc4337: 十六進位字串格式無效")
	ErrInvalidChainID         = errors.New("erc4337: chainId 無效或為空指標")
	ErrInvalidEntryPoint      = errors.New("erc4337: entryPoint 不得為零地址")
)

// UserOperation 代表 ERC-4337 v0.6 標準虛擬交易結構
type UserOperation struct {
	Sender               common.Address `json:"sender" abi:"sender"`
	Nonce                *big.Int       `json:"nonce" abi:"nonce"`
	InitCode             []byte         `json:"initCode" abi:"initCode"`
	CallData             []byte         `json:"callData" abi:"callData"`
	CallGasLimit         *big.Int       `json:"callGasLimit" abi:"callGasLimit"`
	VerificationGasLimit *big.Int       `json:"verificationGasLimit" abi:"verificationGasLimit"`
	PreVerificationGas   *big.Int       `json:"preVerificationGas" abi:"preVerificationGas"`
	MaxFeePerGas         *big.Int       `json:"maxFeePerGas" abi:"maxFeePerGas"`
	MaxPriorityFeePerGas *big.Int       `json:"maxPriorityFeePerGas" abi:"maxPriorityFeePerGas"`
	PaymasterAndData     []byte         `json:"paymasterAndData" abi:"paymasterAndData"`
	Signature            []byte         `json:"signature" abi:"signature"`
}

// PackedUserOperation 代表 ERC-4337 v0.7 鏈上緊密打包結構
type PackedUserOperation struct {
	Sender             common.Address `json:"sender" abi:"sender"`
	Nonce              *big.Int       `json:"nonce" abi:"nonce"`
	InitCode           []byte         `json:"initCode" abi:"initCode"`
	CallData           []byte         `json:"callData" abi:"callData"`
	AccountGasLimits   [32]byte       `json:"accountGasLimits" abi:"accountGasLimits"`
	PreVerificationGas *big.Int       `json:"preVerificationGas" abi:"preVerificationGas"`
	GasFees            [32]byte       `json:"gasFees" abi:"gasFees"`
	PaymasterAndData   []byte         `json:"paymasterAndData" abi:"paymasterAndData"`
	Signature          []byte         `json:"signature" abi:"signature"`
}

// copyBytes 進行位元組切片之深拷貝
func copyBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

// IsCanonicalEntryPoint 檢查給定之合約地址是否為官方認可之 Canonical EntryPoint 地址
func IsCanonicalEntryPoint(addr common.Address) bool {
	return addr == CanonicalEntryPointV06 || addr == CanonicalEntryPointV07
}

// Clone 產生 UserOperation 之完整深拷貝，避免切片與指標共用
func (op *UserOperation) Clone() *UserOperation {
	if op == nil {
		return nil
	}
	cp := &UserOperation{
		Sender:           op.Sender,
		InitCode:         copyBytes(op.InitCode),
		CallData:         copyBytes(op.CallData),
		PaymasterAndData: copyBytes(op.PaymasterAndData),
		Signature:        copyBytes(op.Signature),
	}
	if op.Nonce != nil {
		cp.Nonce = new(big.Int).Set(op.Nonce)
	}
	if op.CallGasLimit != nil {
		cp.CallGasLimit = new(big.Int).Set(op.CallGasLimit)
	}
	if op.VerificationGasLimit != nil {
		cp.VerificationGasLimit = new(big.Int).Set(op.VerificationGasLimit)
	}
	if op.PreVerificationGas != nil {
		cp.PreVerificationGas = new(big.Int).Set(op.PreVerificationGas)
	}
	if op.MaxFeePerGas != nil {
		cp.MaxFeePerGas = new(big.Int).Set(op.MaxFeePerGas)
	}
	if op.MaxPriorityFeePerGas != nil {
		cp.MaxPriorityFeePerGas = new(big.Int).Set(op.MaxPriorityFeePerGas)
	}
	return cp
}

// Clone 產生 PackedUserOperation 之完整深拷貝
func (op *PackedUserOperation) Clone() *PackedUserOperation {
	if op == nil {
		return nil
	}
	cp := &PackedUserOperation{
		Sender:           op.Sender,
		AccountGasLimits: op.AccountGasLimits,
		GasFees:          op.GasFees,
		InitCode:         copyBytes(op.InitCode),
		CallData:         copyBytes(op.CallData),
		PaymasterAndData: copyBytes(op.PaymasterAndData),
		Signature:        copyBytes(op.Signature),
	}
	if op.Nonce != nil {
		cp.Nonce = new(big.Int).Set(op.Nonce)
	}
	if op.PreVerificationGas != nil {
		cp.PreVerificationGas = new(big.Int).Set(op.PreVerificationGas)
	}
	return cp
}

// Validate 執行基礎資料校驗
func (op *UserOperation) Validate() error {
	if op == nil {
		return ErrNilUserOp
	}
	if op.Sender == (common.Address{}) {
		return ErrInvalidSender
	}
	if op.Nonce == nil || op.CallGasLimit == nil || op.VerificationGasLimit == nil ||
		op.PreVerificationGas == nil || op.MaxFeePerGas == nil || op.MaxPriorityFeePerGas == nil {
		return ErrNilField
	}
	if op.Nonce.Sign() < 0 || op.CallGasLimit.Sign() < 0 || op.VerificationGasLimit.Sign() < 0 ||
		op.PreVerificationGas.Sign() < 0 || op.MaxFeePerGas.Sign() < 0 || op.MaxPriorityFeePerGas.Sign() < 0 {
		return ErrNegativeValue
	}
	if op.Nonce.BitLen() > 256 || op.CallGasLimit.BitLen() > 256 || op.VerificationGasLimit.BitLen() > 256 ||
		op.PreVerificationGas.BitLen() > 256 || op.MaxFeePerGas.BitLen() > 256 || op.MaxPriorityFeePerGas.BitLen() > 256 {
		return ErrUint256Overflow
	}
	if op.MaxPriorityFeePerGas.Cmp(op.MaxFeePerGas) > 0 {
		return ErrGasFeeInconsistent
	}
	return nil
}

// Validate checks the packed representation before hashing or unpacking.
// Economic validity is checked by the operation/EntryPoint validation path.
func (op *PackedUserOperation) Validate() error {
	if op == nil {
		return ErrNilPackedUserOp
	}
	if op.Sender == (common.Address{}) {
		return ErrInvalidSender
	}
	if op.Nonce == nil || op.PreVerificationGas == nil {
		return ErrNilField
	}
	if op.Nonce.Sign() < 0 || op.PreVerificationGas.Sign() < 0 {
		return ErrNegativeValue
	}
	if op.Nonce.BitLen() > 256 || op.PreVerificationGas.BitLen() > 256 {
		return ErrUint256Overflow
	}
	return nil
}

// ToPacked 將 UserOperation 轉換為 v0.7 之 PackedUserOperation
func (op *UserOperation) ToPacked() (*PackedUserOperation, error) {
	if err := op.Validate(); err != nil {
		return nil, err
	}

	if op.VerificationGasLimit.Cmp(maxUint128) > 0 || op.CallGasLimit.Cmp(maxUint128) > 0 {
		return nil, ErrGasLimitOverflow
	}
	accountGasLimits, err := PackUint128Pair(op.VerificationGasLimit, op.CallGasLimit)
	if err != nil {
		return nil, err
	}

	if op.MaxPriorityFeePerGas.Cmp(maxUint128) > 0 || op.MaxFeePerGas.Cmp(maxUint128) > 0 {
		return nil, ErrGasFeeOverflow
	}
	gasFees, err := PackUint128Pair(op.MaxPriorityFeePerGas, op.MaxFeePerGas)
	if err != nil {
		return nil, err
	}

	if len(op.PaymasterAndData) > 0 && len(op.PaymasterAndData) < 52 {
		return nil, ErrInvalidPaymasterLength
	}

	packed := &PackedUserOperation{
		Sender:             op.Sender,
		Nonce:              new(big.Int).Set(op.Nonce),
		InitCode:           copyBytes(op.InitCode),
		CallData:           copyBytes(op.CallData),
		AccountGasLimits:   accountGasLimits,
		PreVerificationGas: new(big.Int).Set(op.PreVerificationGas),
		GasFees:            gasFees,
		PaymasterAndData:   copyBytes(op.PaymasterAndData),
		Signature:          copyBytes(op.Signature),
	}
	return packed, nil
}

// Unpack 將 PackedUserOperation 解碼還原為 UserOperation
func (op *PackedUserOperation) Unpack() (*UserOperation, error) {
	if err := op.Validate(); err != nil {
		return nil, err
	}

	verificationGasLimit, callGasLimit := UnpackUint128Pair(op.AccountGasLimits)
	maxPriorityFeePerGas, maxFeePerGas := UnpackUint128Pair(op.GasFees)

	userOp := &UserOperation{
		Sender:               op.Sender,
		Nonce:                new(big.Int).Set(op.Nonce),
		InitCode:             copyBytes(op.InitCode),
		CallData:             copyBytes(op.CallData),
		CallGasLimit:         callGasLimit,
		VerificationGasLimit: verificationGasLimit,
		PreVerificationGas:   new(big.Int).Set(op.PreVerificationGas),
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		PaymasterAndData:     copyBytes(op.PaymasterAndData),
		Signature:            copyBytes(op.Signature),
	}
	return userOp, nil
}

// encodeBigHex 將 *big.Int 編碼為 0x 前綴十六進位字串
func encodeBigHex(b *big.Int) string {
	if b == nil {
		return "0x0"
	}
	return hexutil.EncodeBig(b)
}

// decodeBigHex 將 0x 前綴十六進位字串解碼為 *big.Int
func decodeBigHex(s string) (*big.Int, error) {
	if s == "" || s == "0x" || s == "0X" {
		return big.NewInt(0), nil
	}
	if strings.HasPrefix(s, "0X") {
		s = "0x" + s[2:]
	}
	return hexutil.DecodeBig(s)
}

// encodeBytesHex 將位元組切片編碼為 0x 前綴十六進位字串，空切片輸出 "0x"
func encodeBytesHex(b []byte) string {
	if len(b) == 0 {
		return "0x"
	}
	return hexutil.Encode(b)
}

// decodeBytesHex 將 0x 前綴十六進位字串解碼為位元組切片，"0x" 或 "" 輸出空切片
func decodeBytesHex(s string) ([]byte, error) {
	if s == "" || s == "0x" || s == "0X" {
		return []byte{}, nil
	}
	if strings.HasPrefix(s, "0X") {
		s = "0x" + s[2:]
	}
	return hexutil.Decode(s)
}

// MarshalJSON 實作 UserOperation 之自訂 JSON-RPC 十六進位序列化
func (op *UserOperation) MarshalJSON() ([]byte, error) {
	if op == nil {
		return []byte("null"), nil
	}
	return json.Marshal(userOperationToRPC(op))
}

// UnmarshalJSON 實作 UserOperation 之自訂 JSON-RPC 十六進位反序列化
func (op *UserOperation) UnmarshalJSON(input []byte) error {
	var rpc userOperationRPC
	if err := json.Unmarshal(input, &rpc); err != nil {
		return err
	}
	if err := rejectExplicitEmptySender(input); err != nil {
		return err
	}
	domain, err := userOperationRPCToDomain(rpc)
	if err != nil {
		return err
	}
	*op = *domain
	return nil
}

// MarshalJSON 實作 PackedUserOperation 之自訂 JSON-RPC 十六進位序列化
func (op *PackedUserOperation) MarshalJSON() ([]byte, error) {
	if op == nil {
		return []byte("null"), nil
	}
	return json.Marshal(packedUserOperationToRPC(op))
}

// UnmarshalJSON 實作 PackedUserOperation 之自訂 JSON-RPC 十六進位反序列化
func (op *PackedUserOperation) UnmarshalJSON(input []byte) error {
	var rpc packedUserOperationRPC
	if err := json.Unmarshal(input, &rpc); err != nil {
		return err
	}
	if err := rejectExplicitEmptySender(input); err != nil {
		return err
	}
	domain, err := packedUserOperationRPCToDomain(rpc)
	if err != nil {
		return err
	}
	*op = *domain
	return nil
}
