package erc4337

import (
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	// DefaultPreVerificationGasOverhead 為計算 preVerificationGas 時之基礎固定開銷
	DefaultPreVerificationGasOverhead = big.NewInt(21000)

	// DefaultPriorityFeePerGas 為 EIP-1559 費用預估時之預設小費 (1 Gwei)
	DefaultPriorityFeePerGas = big.NewInt(1000000000)

	// executeMethodID 為標準 SimpleAccount 之 execute(address,uint256,bytes) 4 位元組選擇器 (0xb61d27f6)
	executeMethodID = crypto.Keccak256([]byte("execute(address,uint256,bytes)"))[:4]

	// executeBatchMethodID 為標準 SimpleAccount 之 executeBatch(address[],uint256[],bytes[]) 4 位元組選擇器 (0x47e1c562)
	executeBatchMethodID = crypto.Keccak256([]byte("executeBatch(address[],uint256[],bytes[])"))[:4]
)

var (
	executeArguments      abi.Arguments
	executeBatchArguments abi.Arguments
)

func init() {
	addrType, err := abi.NewType("address", "", nil)
	if err != nil {
		panic(fmt.Sprintf("erc4337: 初始化 address 型別失敗: %v", err))
	}
	uint256Type, err := abi.NewType("uint256", "", nil)
	if err != nil {
		panic(fmt.Sprintf("erc4337: 初始化 uint256 型別失敗: %v", err))
	}
	bytesType, err := abi.NewType("bytes", "", nil)
	if err != nil {
		panic(fmt.Sprintf("erc4337: 初始化 bytes 型別失敗: %v", err))
	}

	addrSliceType, err := abi.NewType("address[]", "", nil)
	if err != nil {
		panic(fmt.Sprintf("erc4337: 初始化 address[] 型別失敗: %v", err))
	}
	uint256SliceType, err := abi.NewType("uint256[]", "", nil)
	if err != nil {
		panic(fmt.Sprintf("erc4337: 初始化 uint256[] 型別失敗: %v", err))
	}
	bytesSliceType, err := abi.NewType("bytes[]", "", nil)
	if err != nil {
		panic(fmt.Sprintf("erc4337: 初始化 bytes[] 型別失敗: %v", err))
	}

	executeArguments = abi.Arguments{
		{Type: addrType},
		{Type: uint256Type},
		{Type: bytesType},
	}

	executeBatchArguments = abi.Arguments{
		{Type: addrSliceType},
		{Type: uint256SliceType},
		{Type: bytesSliceType},
	}
}

var (
	ErrInvalidExecuteVal   = errors.New("erc4337: execute 轉帳數值不得為負數")
	ErrBatchLengthMismatch = errors.New("erc4337: executeBatch 參數陣列長度不一致")
	ErrEmptyBatch          = errors.New("erc4337: executeBatch 批次清單不得為空")
	ErrNilTransaction      = errors.New("erc4337: 交易物件不得為空")
	ErrContractCreationTx  = errors.New("erc4337: 暫不支援由合約部署交易建構 UserOperation")
	ErrInvalidMultiplier   = errors.New("erc4337: 費用倍數必須大於 0")
)

// Builder 負責建構、估算與組裝 UserOperation
type Builder struct {
	entryPoint           common.Address
	chainID              *big.Int
	sender               common.Address
	nonce                *big.Int
	initCode             []byte
	callData             []byte
	callGasLimit         *big.Int
	verificationGasLimit *big.Int
	preVerificationGas   *big.Int
	maxFeePerGas         *big.Int
	maxPriorityFeePerGas *big.Int
	paymasterAndData     []byte
	signature            []byte
}

// NewBuilder 初始化 UserOperation 建構器
func NewBuilder(entryPoint common.Address, chainID *big.Int) *Builder {
	var cID *big.Int
	if chainID != nil {
		cID = new(big.Int).Set(chainID)
	}
	return &Builder{
		entryPoint:           entryPoint,
		chainID:              cID,
		nonce:                big.NewInt(0),
		callGasLimit:         big.NewInt(0),
		verificationGasLimit: big.NewInt(0),
		preVerificationGas:   big.NewInt(0),
		maxFeePerGas:         big.NewInt(0),
		maxPriorityFeePerGas: big.NewInt(0),
	}
}

// SetSender 設定 Smart Contract Account 發送者地址
func (b *Builder) SetSender(sender common.Address) *Builder {
	b.sender = sender
	return b
}

// SetNonce 設定帳戶 Nonce
func (b *Builder) SetNonce(nonce *big.Int) *Builder {
	if nonce != nil {
		b.nonce = new(big.Int).Set(nonce)
	}
	return b
}

// SetInitCode 設定部署合約用之 InitCode
func (b *Builder) SetInitCode(initCode []byte) *Builder {
	b.initCode = slices.Clone(initCode)
	return b
}

// SetCallData 設定直接執行之 CallData
func (b *Builder) SetCallData(callData []byte) *Builder {
	b.callData = slices.Clone(callData)
	return b
}

// SetExecuteCallData 打包標準 execute(address dest, uint256 value, bytes func) 格式之 CallData
func (b *Builder) SetExecuteCallData(target common.Address, value *big.Int, data []byte) (*Builder, error) {
	if value == nil {
		value = big.NewInt(0)
	}
	if value.Sign() < 0 {
		return nil, ErrInvalidExecuteVal
	}

	callArgs, err := executeArguments.Pack(target, value, data)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 打包 execute 參數失敗: %w", err)
	}

	packed := make([]byte, 0, len(executeMethodID)+len(callArgs))
	packed = append(packed, executeMethodID...)
	packed = append(packed, callArgs...)

	b.callData = packed
	return b, nil
}

// SetExecuteBatchCallData 打包標準 executeBatch(address[] dests, uint256[] values, bytes[] funcs) 格式之 CallData
func (b *Builder) SetExecuteBatchCallData(targets []common.Address, values []*big.Int, datas [][]byte) (*Builder, error) {
	if len(targets) == 0 {
		return nil, ErrEmptyBatch
	}
	if len(targets) != len(values) || len(targets) != len(datas) {
		return nil, ErrBatchLengthMismatch
	}

	cleanValues := make([]*big.Int, len(values))
	for i, val := range values {
		if val == nil {
			cleanValues[i] = big.NewInt(0)
		} else {
			if val.Sign() < 0 {
				return nil, ErrInvalidExecuteVal
			}
			cleanValues[i] = new(big.Int).Set(val)
		}
	}

	callArgs, err := executeBatchArguments.Pack(targets, cleanValues, datas)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 打包 executeBatch 參數失敗: %w", err)
	}

	packed := make([]byte, 0, len(executeBatchMethodID)+len(callArgs))
	packed = append(packed, executeBatchMethodID...)
	packed = append(packed, callArgs...)

	b.callData = packed
	return b, nil
}

// SetTransaction 由以太坊標準交易物件解析並設定 UserOperation
func (b *Builder) SetTransaction(tx *types.Transaction) (*Builder, error) {
	if tx == nil {
		return nil, ErrNilTransaction
	}
	to := tx.To()
	if to == nil {
		return nil, ErrContractCreationTx
	}

	_, err := b.SetExecuteCallData(*to, tx.Value(), tx.Data())
	if err != nil {
		return nil, err
	}

	if tx.Gas() > 0 {
		b.callGasLimit = new(big.Int).SetUint64(tx.Gas())
	}

	switch tx.Type() {
	case types.DynamicFeeTxType:
		feeCap := tx.GasFeeCap()
		tipCap := tx.GasTipCap()
		if feeCap != nil && tipCap != nil {
			b.SetGasFees(feeCap, tipCap)
		}
	default:
		gasPrice := tx.GasPrice()
		if gasPrice != nil && gasPrice.Sign() > 0 {
			b.SetGasFees(gasPrice, gasPrice)
		}
	}

	if tx.Nonce() > 0 && (b.nonce == nil || b.nonce.Sign() == 0) {
		b.nonce = new(big.Int).SetUint64(tx.Nonce())
	}

	return b, nil
}

// SetGasLimits 設定各項 Gas Limit 參數
func (b *Builder) SetGasLimits(callGasLimit, verificationGasLimit, preVerificationGas *big.Int) *Builder {
	if callGasLimit != nil {
		b.callGasLimit = new(big.Int).Set(callGasLimit)
	}
	if verificationGasLimit != nil {
		b.verificationGasLimit = new(big.Int).Set(verificationGasLimit)
	}
	if preVerificationGas != nil {
		b.preVerificationGas = new(big.Int).Set(preVerificationGas)
	}
	return b
}

// SetGasFees 設定 EIP-1559 費用參數
func (b *Builder) SetGasFees(maxFeePerGas, maxPriorityFeePerGas *big.Int) *Builder {
	if maxFeePerGas != nil {
		b.maxFeePerGas = new(big.Int).Set(maxFeePerGas)
	}
	if maxPriorityFeePerGas != nil {
		b.maxPriorityFeePerGas = new(big.Int).Set(maxPriorityFeePerGas)
	}
	return b
}

// CalculateEIP1559Fees 純整數計算 EIP-1559 費用參數：maxPriorityFeePerGas 與 maxFeePerGas
func CalculateEIP1559Fees(baseFee, priorityFee *big.Int, multiplier int64) (*big.Int, *big.Int, error) {
	if multiplier <= 0 {
		return nil, nil, ErrInvalidMultiplier
	}

	var feeBase *big.Int
	if baseFee != nil && baseFee.Sign() > 0 {
		feeBase = new(big.Int).Set(baseFee)
	} else {
		feeBase = big.NewInt(0)
	}

	var tip *big.Int
	if priorityFee != nil && priorityFee.Sign() > 0 {
		tip = new(big.Int).Set(priorityFee)
	} else {
		tip = new(big.Int).Set(DefaultPriorityFeePerGas)
	}

	bufferBaseFee := new(big.Int).Mul(feeBase, big.NewInt(multiplier))
	maxFee := new(big.Int).Add(bufferBaseFee, tip)

	return maxFee, tip, nil
}

// EstimateGasFees 依據鏈上 baseFee 與 priorityFee 進行純整數 EIP-1559 費用預估並更新 Builder
func (b *Builder) EstimateGasFees(baseFee, priorityFee *big.Int) *Builder {
	maxFee, tip, err := CalculateEIP1559Fees(baseFee, priorityFee, 2)
	if err == nil {
		b.SetGasFees(maxFee, tip)
	}
	return b
}

// SetPaymasterAndData 設定 Paymaster 地址與驗證參數
func (b *Builder) SetPaymasterAndData(paymasterAndData []byte) *Builder {
	b.paymasterAndData = slices.Clone(paymasterAndData)
	return b
}

// SetSignature 設定簽名
func (b *Builder) SetSignature(sig []byte) *Builder {
	b.signature = slices.Clone(sig)
	return b
}

// bigIntTo32Bytes 將 big.Int 大端序寫入 32 位元組陣列，高位補零
func bigIntTo32Bytes(val *big.Int) [32]byte {
	var out [32]byte
	if val == nil || val.Sign() <= 0 {
		return out
	}
	bytesVal := val.Bytes()
	if len(bytesVal) > 32 {
		bytesVal = bytesVal[len(bytesVal)-32:]
	}
	copy(out[32-len(bytesVal):], bytesVal)
	return out
}

// EstimatePreVerificationGas 純整數計算傳輸與驗證開銷（零位元組 4 gas，非零位元組 16 gas，計入固定欄位開銷與 overhead，自收斂估算）
func (b *Builder) EstimatePreVerificationGas(overhead *big.Int) *big.Int {
	if overhead == nil {
		overhead = DefaultPreVerificationGasOverhead
	}

	calcOnce := func() *big.Int {
		var zeroCount, nonZeroCount int64
		tally := func(data []byte) {
			for _, byteVal := range data {
				if byteVal == 0 {
					zeroCount += 1
				} else {
					nonZeroCount += 1
				}
			}
		}

		// 1. 統計固定欄位開銷
		tally(b.sender.Bytes())

		nonceWord := bigIntTo32Bytes(b.nonce)
		tally(nonceWord[:])

		callGasWord := bigIntTo32Bytes(b.callGasLimit)
		tally(callGasWord[:])

		verificationGasWord := bigIntTo32Bytes(b.verificationGasLimit)
		tally(verificationGasWord[:])

		preVerificationGasWord := bigIntTo32Bytes(b.preVerificationGas)
		tally(preVerificationGasWord[:])

		maxFeeWord := bigIntTo32Bytes(b.maxFeePerGas)
		tally(maxFeeWord[:])

		maxPriorityFeeWord := bigIntTo32Bytes(b.maxPriorityFeePerGas)
		tally(maxPriorityFeeWord[:])

		// 2. 統計簽名欄位開銷：若已設定則統計實際位元組，否則預設 65 位元組非零簽名開銷
		if len(b.signature) > 0 {
			tally(b.signature)
		} else {
			nonZeroCount += 65
		}

		// 3. 統計動態欄位開銷
		tally(b.callData)
		tally(b.initCode)
		tally(b.paymasterAndData)

		dataCost := zeroCount*4 + nonZeroCount*16
		return new(big.Int).Add(overhead, big.NewInt(dataCost))
	}

	// 第一次計算取得初步開銷
	firstEst := calcOnce()
	b.preVerificationGas = new(big.Int).Set(firstEst)

	// 第二次計算，自收斂 preVerificationGas 自身之位元組開銷
	finalEst := calcOnce()
	b.preVerificationGas = new(big.Int).Set(finalEst)

	return finalEst
}

// CalcPreVerificationGas 純整數計算給定 UserOperation 之 PreVerificationGas
func CalcPreVerificationGas(op *UserOperation, overhead *big.Int) *big.Int {
	if op == nil {
		return big.NewInt(0)
	}
	b := &Builder{
		sender:               op.Sender,
		nonce:                op.Nonce,
		initCode:             op.InitCode,
		callData:             op.CallData,
		callGasLimit:         op.CallGasLimit,
		verificationGasLimit: op.VerificationGasLimit,
		preVerificationGas:   op.PreVerificationGas,
		maxFeePerGas:         op.MaxFeePerGas,
		maxPriorityFeePerGas: op.MaxPriorityFeePerGas,
		paymasterAndData:     op.PaymasterAndData,
		signature:            op.Signature,
	}
	return b.EstimatePreVerificationGas(overhead)
}

// Build 驗證參數並產出標準 UserOperation
func (b *Builder) Build() (*UserOperation, error) {
	if b.sender == (common.Address{}) {
		return nil, ErrInvalidSender
	}
	if b.entryPoint == (common.Address{}) {
		return nil, ErrInvalidEntryPoint
	}
	if b.chainID == nil || b.chainID.Sign() <= 0 {
		return nil, ErrInvalidChainID
	}

	op := &UserOperation{
		Sender:               b.sender,
		Nonce:                new(big.Int).Set(b.nonce),
		InitCode:             slices.Clone(b.initCode),
		CallData:             slices.Clone(b.callData),
		CallGasLimit:         new(big.Int).Set(b.callGasLimit),
		VerificationGasLimit: new(big.Int).Set(b.verificationGasLimit),
		PreVerificationGas:   new(big.Int).Set(b.preVerificationGas),
		MaxFeePerGas:         new(big.Int).Set(b.maxFeePerGas),
		MaxPriorityFeePerGas: new(big.Int).Set(b.maxPriorityFeePerGas),
		PaymasterAndData:     slices.Clone(b.paymasterAndData),
		Signature:            slices.Clone(b.signature),
	}

	if err := op.Validate(); err != nil {
		return nil, err
	}
	return op, nil
}

// BuildAndSign 驗證建構 UserOperation 並呼叫 Signer 簽署
func (b *Builder) BuildAndSign(signer UserOpSigner) (*UserOperation, error) {
	if signer == nil {
		return nil, errors.New("erc4337: signer 不得為空")
	}
	op, err := b.Build()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignUserOp(op, b.entryPoint, b.chainID)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 簽署 UserOperation 失敗: %w", err)
	}
	op.Signature = sig
	b.signature = slices.Clone(sig)
	return op, nil
}
