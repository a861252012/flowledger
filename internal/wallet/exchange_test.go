package wallet

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestExchangeSignsBoundPayload(t *testing.T) {
	for _, action := range []string{"wrap", "unwrap", "swap"} {
		t.Run(action, func(t *testing.T) {
			s, state := guardedFixture(t)
			address, _ := s.keystore.Address()
			req := &QuoteRequest{Action: action, To: address, Amount: "0.000000000000000001"}
			if action == "swap" {
				req.Contract = WETHAddress
				req.TokenOut = USDCAddress
				req.PoolFee = 3000
				req.SlippageBPS = 50
				state.allowance = big.NewInt(1)
			}
			q, err := s.Quote(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if q.Exchange == nil {
				t.Fatal("missing exchange preview")
			}
			sent, err := s.Send(context.Background(), q.ID, "fixture-password-123")
			if err != nil {
				t.Fatal(err)
			}
			duplicate, err := s.Send(context.Background(), q.ID, "fixture-password-123")
			if err != nil {
				t.Fatal(err)
			}
			if len(state.raws) != 1 || sent.Hash != duplicate.Hash {
				t.Fatal("duplicate broadcast")
			}
			raw, _ := hexutil.Decode(state.raws[0])
			var tx types.Transaction
			if err := tx.UnmarshalBinary(raw); err != nil {
				t.Fatal(err)
			}
			if tx.ChainId().Int64() != 11155111 || tx.Type() != 2 {
				t.Fatal("wrong chain or tx type")
			}
			if action == "wrap" {
				if *tx.To() != common.HexToAddress(WETHAddress) || tx.Value().Cmp(big.NewInt(1)) != 0 || hexutil.Encode(tx.Data()) != "0xd0e30db0" {
					t.Fatal("invalid deposit")
				}
			} else if action == "unwrap" {
				if *tx.To() != common.HexToAddress(WETHAddress) || tx.Value().Sign() != 0 || hexutil.Encode(tx.Data()[:4]) != "0x2e1a7d4d" {
					t.Fatal("invalid withdrawal")
				}
				args, err := exchangeABI.Methods["withdraw"].Inputs.Unpack(tx.Data()[4:])
				if err != nil || args[0].(*big.Int).Cmp(big.NewInt(1)) != 0 {
					t.Fatal("withdraw amount")
				}
			} else {
				if *tx.To() != common.HexToAddress(RouterAddress) || tx.Value().Sign() != 0 {
					t.Fatal("invalid swap target/value")
				}
				method, err := exchangeABI.MethodById(tx.Data()[:4])
				if err != nil || method.Sig != "multicall(uint256,bytes[])" {
					t.Fatal("missing on-chain deadline")
				}
				args, err := method.Inputs.Unpack(tx.Data()[4:])
				if err != nil {
					t.Fatal(err)
				}
				deadline := args[0].(*big.Int).Int64()
				if deadline <= time.Now().Unix() || deadline > time.Now().Unix()+120 {
					t.Fatal("bad deadline")
				}
				calls := args[1].([][]byte)
				if len(calls) != 1 {
					t.Fatal("unexpected router calls")
				}
				inner, err := exchangeABI.MethodById(calls[0][:4])
				if err != nil || inner.Name != "exactInputSingle" {
					t.Fatal("wrong swap method")
				}
				params, err := inner.Inputs.Unpack(calls[0][4:])
				if err != nil {
					t.Fatal(err)
				}
				p := abi.ConvertType(params[0], new(swapParams)).(*swapParams)
				if p.Recipient != common.HexToAddress(address) || p.TokenIn != common.HexToAddress(WETHAddress) || p.TokenOut != common.HexToAddress(USDCAddress) || p.AmountIn.Int64() != 1 || p.AmountOutMinimum.Int64() != 995000 || p.Fee.Int64() != 3000 || p.SqrtPriceLimitX96.Sign() != 0 {
					t.Fatalf("unbound swap %+v", p)
				}
			}
		})
	}
}

func TestExchangeRejectsUnsafeOrUnavailableInputs(t *testing.T) {
	for _, kind := range []string{"mainnet-token", "recipient", "zero", "slippage-zero", "slippage-high", "fee", "pool", "approval", "funds", "simulation", "output-zero"} {
		t.Run(kind, func(t *testing.T) {
			s, state := guardedFixture(t)
			address, _ := s.keystore.Address()
			state.allowance = big.NewInt(1)
			req := &QuoteRequest{Action: "swap", To: address, Contract: WETHAddress, TokenOut: USDCAddress, Amount: "0.000000000000000001", PoolFee: 3000, SlippageBPS: 50}
			switch kind {
			case "mainnet-token":
				req.TokenOut = "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
			case "recipient":
				req.To = "0x1111111111111111111111111111111111111111"
			case "zero":
				req.Amount = "0"
			case "slippage-zero":
				req.SlippageBPS = 0
			case "slippage-high":
				req.SlippageBPS = 501
			case "fee":
				req.PoolFee = 1
			case "pool":
				state.missingPool = true
			case "approval":
				state.allowance = big.NewInt(0)
			case "funds":
				state.tokenBalance = big.NewInt(0)
			case "simulation":
				state.failSimulation = true
			case "output-zero":
				state.swapOutput = big.NewInt(0)
			}
			if _, err := s.Quote(context.Background(), req); err == nil {
				t.Fatal("accepted invalid swap")
			}
			if len(state.raws) != 0 {
				t.Fatal("broadcast during preview")
			}
		})
	}
}

func TestExchangeRechecksBeforeSigning(t *testing.T) {
	for _, kind := range []string{"allowance", "balance", "simulation", "slippage", "chain"} {
		t.Run(kind, func(t *testing.T) {
			s, state := guardedFixture(t)
			address, _ := s.keystore.Address()
			state.allowance = big.NewInt(1)
			q, err := s.Quote(context.Background(), &QuoteRequest{Action: "swap", To: address, Contract: WETHAddress, TokenOut: USDCAddress, Amount: "0.000000000000000001", PoolFee: 3000, SlippageBPS: 50})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "allowance":
				state.allowance = big.NewInt(0)
			case "balance":
				state.tokenBalance = big.NewInt(0)
			case "simulation":
				state.failSimulation = true
			case "slippage":
				state.swapOutput = big.NewInt(994999)
			case "chain":
				state.chainID = "0x1"
			}
			if _, err := s.Send(context.Background(), q.ID, "fixture-password-123"); err == nil {
				t.Fatal("accepted changed condition")
			}
			if len(state.raws) != 0 {
				t.Fatal("broadcast despite changed condition")
			}
		})
	}
}
