package chain

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func checkRevertError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := ethclient.RevertErrorData(err); ok {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "execution reverted") || strings.Contains(lower, "vm execution error")
}

// CheckNetwork verifies that the connected node is on Ethereum Sepolia.
func (c *Client) CheckNetwork(ctx context.Context) error {
	return c.verifyNetwork(ctx)
}

// HeaderByNumber returns the block header for the specified number, or latest if nil.
func (c *Client) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	h, err := c.rpc.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, rpcError(err)
	}
	return h, nil
}

// SuggestGasTipCap retrieves the suggested gas tip cap from Sepolia.
func (c *Client) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	tip, err := c.rpc.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, rpcError(err)
	}
	return tip, nil
}

// EstimateGas estimates the gas required for a call on Sepolia.
func (c *Client) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return 0, err
	}
	gas, err := c.rpc.EstimateGas(ctx, msg)
	if err != nil {
		return 0, rpcError(err)
	}
	return gas, nil
}

// CallContract executes an eth_call simulation on Sepolia.
func (c *Client) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	res, err := c.rpc.CallContract(ctx, msg, blockNumber)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, ErrTimeout
		}
		if checkRevertError(err) {
			return nil, err
		}
		return nil, rpcError(err)
	}
	return res, nil
}

// CodeAt returns the contract bytecode at the given address on Sepolia.
func (c *Client) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	code, err := c.rpc.CodeAt(ctx, account, blockNumber)
	if err != nil {
		return nil, rpcError(err)
	}
	return code, nil
}

// PendingNonceAt returns the pending nonce for an account on Sepolia.
func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return 0, err
	}
	nonce, err := c.rpc.PendingNonceAt(ctx, account)
	if err != nil {
		return 0, rpcError(err)
	}
	return nonce, nil
}

// BalanceAt returns the wei balance of an account on Sepolia.
func (c *Client) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	bal, err := c.rpc.BalanceAt(ctx, account, blockNumber)
	if err != nil {
		return nil, rpcError(err)
	}
	return bal, nil
}

// SendTransaction broadcasts a signed transaction to Sepolia.
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := c.checkNetwork(ctx); err != nil {
		return err
	}
	if tx.ChainId().Cmp(big.NewInt(c.ChainID())) != 0 {
		return ErrNetwork
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		return ErrUnavailable
	}
	var wireHash string
	if err := c.rpc.Client().CallContext(ctx, &wireHash, "eth_sendRawTransaction", hexutil.Encode(raw)); err != nil {
		return rpcError(err)
	}
	hash, err := rpcHashToDomain(wireHash)
	if err != nil {
		return err
	}
	if hash != tx.Hash() {
		return ErrUnavailable
	}
	return nil
}

func (c *Client) NonceAt(ctx context.Context, account common.Address) (uint64, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return 0, err
	}
	nonce, err := c.rpc.NonceAt(ctx, account, nil)
	if err != nil {
		return 0, rpcError(err)
	}
	return nonce, nil
}
