package wallet

import (
	"context"
	"testing"
)

func TestPoolComparisonDoesNotRequireAllowanceOrBroadcast(t *testing.T) {
	s, state := guardedFixture(t)
	result, err := s.ComparePools(context.Background(), &QuoteRequest{Contract: WETHAddress, TokenOut: USDCAddress, Amount: "0.000001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pools) != 4 || result.BestFee == 0 {
		t.Fatalf("missing comparison: %+v", result)
	}
	if len(state.raws) != 0 || state.allowance.Sign() != 0 {
		t.Fatal("comparison mutated chain")
	}
	if _, err := s.ComparePools(context.Background(), &QuoteRequest{Contract: WETHAddress, TokenOut: WETHAddress, Amount: "1"}); err == nil {
		t.Fatal("accepted same asset")
	}
	if _, err := s.ComparePools(context.Background(), &QuoteRequest{Contract: WETHAddress, TokenOut: USDCAddress, Amount: "0"}); err == nil {
		t.Fatal("accepted zero")
	}
}
