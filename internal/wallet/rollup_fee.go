package wallet

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
)

// A buffered estimate is checked again before signing; OP data fees cannot be capped by a type-2 transaction.
func (s *Service) rollupFee(ctx context.Context, q *BoundQuote, bind bool) error {
	if !s.client.IsOPStack() {
		return nil
	}
	tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(s.client.ChainID()), Nonce: q.Nonce, To: &q.TxTo, Value: q.TxValue, Data: q.Data, Gas: q.GasLimit, GasFeeCap: q.MaxFeePerGas, GasTipCap: q.MaxPriorityFeePerGas})
	estimate, err := s.client.RollupFee(ctx, tx)
	if err != nil {
		return err
	}
	if !bind {
		if q.RollupFeeWei == nil || estimate.Cmp(q.RollupFeeWei) > 0 {
			return errors.New("L1／營運費已超過預留估算，請重新報價")
		}
		return nil
	}
	q.RollupFeeWei = new(big.Int).Mul(estimate, big.NewInt(2))
	q.TotalETHWei = new(big.Int).Add(q.TotalETHWei, q.RollupFeeWei)
	q.TotalETH = FormatUnits(q.TotalETHWei, 18)
	balance, err := s.client.BalanceAt(ctx, q.From, nil)
	if err != nil {
		return err
	}
	if balance.Cmp(q.TotalETHWei) < 0 {
		return ErrInsufficientFunds
	}
	return nil
}
