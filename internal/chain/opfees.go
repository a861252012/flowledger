package chain

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	opGasPriceOracleAddr    = common.HexToAddress("0x420000000000000000000000000000000000000F")
	l1FeeUpperBoundSelector = crypto.Keccak256([]byte("getL1FeeUpperBound(uint256)"))[:4]
	operatorFeeSelector     = crypto.Keccak256([]byte("getOperatorFee(uint256)"))[:4]
)

func (c *Client) IsOPStack() bool { return c.ChainID() == 84532 || c.ChainID() == 11155420 }

// RollupFee estimates fees outside EIP-1559's execution fee cap. It is not an on-chain spending limit.
func (c *Client) RollupFee(ctx context.Context, tx *types.Transaction) (*big.Int, error) {
	if !c.IsOPStack() {
		return new(big.Int), nil
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	l1, err := c.oracleFee(ctx, l1FeeUpperBoundSelector, uint64(len(raw)), nil)
	if err != nil {
		return nil, err
	}
	operator, err := c.oracleFee(ctx, operatorFeeSelector, tx.Gas(), nil)
	if err != nil {
		return nil, err
	}
	return new(big.Int).Add(l1, operator), nil
}

func (c *Client) oracleFee(ctx context.Context, selector []byte, value uint64, blockHash *common.Hash) (*big.Int, error) {
	data := append(append([]byte(nil), selector...), common.LeftPadBytes(new(big.Int).SetUint64(value).Bytes(), 32)...)
	msg := ethereum.CallMsg{To: &opGasPriceOracleAddr, Data: data}
	var raw []byte
	var err error
	if blockHash == nil {
		raw, err = c.CallContract(ctx, msg, nil)
	} else {
		raw, err = c.rpc.CallContractAtHash(ctx, msg, *blockHash)
	}
	if err != nil {
		return nil, rpcError(err)
	}
	if len(raw) != 32 {
		return nil, ErrUnavailable
	}
	return new(big.Int).SetBytes(raw), nil
}

// ReceiptFee includes OP Stack L1 and operator charges, bound to the same receipt block.
func (c *Client) ReceiptFee(ctx context.Context, receipt *types.Receipt) (*big.Int, error) {
	fee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	if !c.IsOPStack() {
		return fee, nil
	}
	var wire *opReceiptRPC
	if err := c.rpc.Client().CallContext(ctx, &wire, "eth_getTransactionReceipt", receipt.TxHash); err != nil {
		return nil, rpcError(err)
	}
	extra, err := opReceiptToDomain(wire)
	if err != nil || extra.transactionHash != receipt.TxHash || extra.blockHash != receipt.BlockHash {
		return nil, ErrUnavailable
	}
	operator, err := c.oracleFee(ctx, operatorFeeSelector, receipt.GasUsed, &receipt.BlockHash)
	if err != nil {
		return nil, err
	}
	return fee.Add(fee, new(big.Int).Add(extra.l1Fee, operator)), nil
}
