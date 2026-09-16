package chain

import (
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Network, Balance, Transaction, Movement, Activity, and ScannedBlock are the
// package's outward-facing representations. Numeric chain values stay typed in
// the domain model and are converted to the existing JSON strings only here.
type Network struct {
	ChainID   int       `json:"chainId"`
	Block     string    `json:"block"`
	BlockTime time.Time `json:"blockTime"`
	CheckedAt time.Time `json:"checkedAt"`
}

type Balance struct {
	Address   string    `json:"address"`
	Wei       string    `json:"wei"`
	ETH       string    `json:"eth"`
	Block     string    `json:"block"`
	CheckedAt time.Time `json:"checkedAt"`
}

type Transaction struct {
	Finalized     bool      `json:"finalized"`
	BlockHash     string    `json:"blockHash,omitempty"`
	Hash          string    `json:"hash"`
	State         string    `json:"state"`
	Block         string    `json:"block,omitempty"`
	Confirmations string    `json:"confirmations,omitempty"`
	GasUsed       string    `json:"gasUsed,omitempty"`
	FeeETH        string    `json:"feeEth,omitempty"`
	CheckedAt     time.Time `json:"checkedAt"`
}

type Movement struct {
	Kind         string `json:"kind"`
	Asset        string `json:"asset"`
	Raw          string `json:"raw"`
	Counterparty string `json:"counterparty"`
	Evidence     string `json:"evidence"`
}

type Activity struct {
	Hash      string     `json:"hash"`
	State     string     `json:"state"`
	Block     string     `json:"block,omitempty"`
	BlockHash string     `json:"blockHash,omitempty"`
	BlockTime string     `json:"blockTime,omitempty"`
	CheckedAt time.Time  `json:"checkedAt"`
	Movements []Movement `json:"movements"`
	Error     string     `json:"error,omitempty"`
}

// ScannedBlock is the storage-facing scanner record consumed by wallet cursor
// persistence. Hashes and addresses are encoded only when crossing that boundary.
type ScannedBlock struct {
	Hashes []string
	Tokens []string
	Hash   string
}

type networkSnapshot struct {
	chainID   int64
	block     *big.Int
	blockTime time.Time
	checkedAt time.Time
}

func networkToAPI(value networkSnapshot) *Network {
	return &Network{
		ChainID:   int(value.chainID),
		Block:     value.block.String(),
		BlockTime: value.blockTime,
		CheckedAt: value.checkedAt,
	}
}

type balanceSnapshot struct {
	address   common.Address
	wei       *big.Int
	block     *big.Int
	checkedAt time.Time
}

func balanceToAPI(value balanceSnapshot) *Balance {
	return &Balance{
		Address:   value.address.Hex(),
		Wei:       value.wei.String(),
		ETH:       FormatETH(value.wei),
		Block:     value.block.String(),
		CheckedAt: value.checkedAt,
	}
}

type transactionState string

const (
	transactionPending            transactionState = "pending"
	transactionReceiptUnavailable transactionState = "receipt_unavailable"
	transactionSucceeded          transactionState = "succeeded"
	transactionReverted           transactionState = "reverted"
	transactionReorgDetected      transactionState = "reorg_detected"
)

type transactionSnapshot struct {
	finalized     bool
	blockHash     common.Hash
	hash          common.Hash
	state         transactionState
	block         *big.Int
	confirmations *big.Int
	gasUsed       uint64
	feeWei        *big.Int
	checkedAt     time.Time
}

func transactionToAPI(value transactionSnapshot) *Transaction {
	result := &Transaction{
		Finalized: value.finalized,
		Hash:      value.hash.Hex(),
		State:     string(value.state),
		CheckedAt: value.checkedAt,
	}
	if value.block != nil {
		result.Block = value.block.String()
		result.BlockHash = value.blockHash.Hex()
	}
	if value.confirmations != nil {
		result.Confirmations = value.confirmations.String()
	}
	if value.feeWei != nil {
		result.GasUsed = new(big.Int).SetUint64(value.gasUsed).String()
		result.FeeETH = FormatETH(value.feeWei)
	}
	return result
}

type movementKind string

const (
	movementFee     movementKind = "fee"
	movementSend    movementKind = "send"
	movementReceive movementKind = "receive"
)

type movementRecord struct {
	kind         movementKind
	asset        string
	raw          *big.Int
	counterparty common.Address
	evidence     string
}

func movementsToAPI(values []movementRecord) []Movement {
	result := make([]Movement, len(values))
	for i, value := range values {
		result[i] = Movement{
			Kind:         string(value.kind),
			Asset:        value.asset,
			Raw:          value.raw.String(),
			Counterparty: value.counterparty.Hex(),
			Evidence:     value.evidence,
		}
	}
	return result
}

type activityRecord struct {
	hash      common.Hash
	state     transactionState
	block     *big.Int
	blockHash common.Hash
	blockTime *time.Time
	checkedAt time.Time
	movements []movementRecord
	err       string
}

func activityToAPI(value activityRecord) *Activity {
	result := &Activity{
		Hash:      value.hash.Hex(),
		State:     string(value.state),
		CheckedAt: value.checkedAt,
		Movements: movementsToAPI(value.movements),
		Error:     value.err,
	}
	if value.block != nil {
		result.Block = value.block.String()
		result.BlockHash = value.blockHash.Hex()
	}
	if value.blockTime != nil {
		result.BlockTime = value.blockTime.UTC().Format(time.RFC3339)
	}
	return result
}

// rpcBlock is the JSON-RPC wire shape used for full block discovery. It is
// deliberately separate from both geth's signed transaction type and scanner
// storage records.
type rpcBlock struct {
	Hash         string           `json:"hash"`
	Transactions []rpcTransaction `json:"transactions"`
}

type rpcTransaction struct {
	Hash string  `json:"hash"`
	From string  `json:"from"`
	To   *string `json:"to"`
}

type blockRecord struct {
	hash         common.Hash
	transactions []transactionReference
}

type transactionReference struct {
	hash common.Hash
	from common.Address
	to   *common.Address
}

func rpcBlockToDomain(value rpcBlock) (blockRecord, error) {
	hash, err := rpcHashToDomain(value.Hash)
	if err != nil {
		return blockRecord{}, err
	}
	result := blockRecord{
		hash:         hash,
		transactions: make([]transactionReference, len(value.Transactions)),
	}
	for i, tx := range value.Transactions {
		txHash, err := rpcHashToDomain(tx.Hash)
		if err != nil || !strings.HasPrefix(tx.From, "0x") || !common.IsHexAddress(tx.From) {
			return blockRecord{}, ErrUnavailable
		}
		result.transactions[i] = transactionReference{
			hash: txHash,
			from: common.HexToAddress(tx.From),
		}
		if tx.To != nil {
			if !strings.HasPrefix(*tx.To, "0x") || !common.IsHexAddress(*tx.To) {
				return blockRecord{}, ErrUnavailable
			}
			result.transactions[i].to = new(common.HexToAddress(*tx.To))
		}
	}
	return result, nil
}

func rpcHashToDomain(value string) (common.Hash, error) {
	if !hashPattern.MatchString(value) {
		return common.Hash{}, ErrUnavailable
	}
	return common.HexToHash(value), nil
}

type opReceiptRPC struct {
	TransactionHash string  `json:"transactionHash"`
	BlockHash       string  `json:"blockHash"`
	L1Fee           *string `json:"l1Fee"`
}

type opReceiptRecord struct {
	transactionHash common.Hash
	blockHash       common.Hash
	l1Fee           *big.Int
}

func opReceiptToDomain(value *opReceiptRPC) (opReceiptRecord, error) {
	if value == nil || value.L1Fee == nil {
		return opReceiptRecord{}, ErrUnavailable
	}
	transactionHash, err := rpcHashToDomain(value.TransactionHash)
	if err != nil {
		return opReceiptRecord{}, err
	}
	blockHash, err := rpcHashToDomain(value.BlockHash)
	if err != nil {
		return opReceiptRecord{}, err
	}
	l1Fee, err := hexutil.DecodeBig(*value.L1Fee)
	if err != nil {
		return opReceiptRecord{}, ErrUnavailable
	}
	return opReceiptRecord{transactionHash: transactionHash, blockHash: blockHash, l1Fee: l1Fee}, nil
}

type scannedBlockRecord struct {
	hash   common.Hash
	hashes []common.Hash
	tokens []common.Address
}

func scannedBlockToStorage(value scannedBlockRecord) *ScannedBlock {
	result := &ScannedBlock{
		Hash:   value.hash.Hex(),
		Hashes: make([]string, len(value.hashes)),
		Tokens: make([]string, len(value.tokens)),
	}
	for i, hash := range value.hashes {
		result.Hashes[i] = hash.Hex()
	}
	for i, token := range value.tokens {
		result.Tokens[i] = token.Hex()
	}
	return result
}
