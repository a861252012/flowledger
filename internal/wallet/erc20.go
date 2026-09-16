package wallet

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const erc20ABIJSON = `[
  {"name":"name","type":"function","inputs":[],"outputs":[{"name":"","type":"string"}],"stateMutability":"view"},
  {"name":"symbol","type":"function","inputs":[],"outputs":[{"name":"","type":"string"}],"stateMutability":"view"},
  {"name":"decimals","type":"function","inputs":[],"outputs":[{"name":"","type":"uint8"}],"stateMutability":"view"},
  {"name":"balanceOf","type":"function","inputs":[{"name":"account","type":"address"}],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},
  {"name":"allowance","type":"function","inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"},
  {"name":"transfer","type":"function","inputs":[{"name":"recipient","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"name":"","type":"bool"}],"stateMutability":"nonpayable"},
  {"name":"approve","type":"function","inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"name":"","type":"bool"}],"stateMutability":"nonpayable"}
]`

var erc20ABI abi.ABI

func trustedEVMToken(chainID int64, contract common.Address) (string, int, bool) {
	if chainID != 11155111 {
		return "", 0, false
	}
	symbol, decimals, err := exchangeToken(contract)
	return symbol, decimals, err == nil
}

// ParseRawTokenAmount parses the exact uint256 value placed in custom-token calldata.
func ParseRawTokenAmount(value string) (*big.Int, error) {
	if value == "" || len(value) > 78 {
		return nil, errors.New("自訂代幣必須輸入最小單位整數")
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return nil, errors.New("自訂代幣最小單位只能是十進位整數")
		}
	}
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok || amount.Cmp(maxUint256) > 0 {
		return nil, errors.New("自訂代幣最小單位超出 uint256 範圍")
	}
	return amount, nil
}

func init() {
	var err error
	erc20ABI, err = abi.JSON(strings.NewReader(erc20ABIJSON))
	if err != nil {
		panic("failed to parse erc20 ABI: " + err.Error())
	}
}

// ChainCaller defines the subset of RPC methods needed for ERC20 queries and simulations.
type ChainCaller interface {
	CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error)
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// DecodedCalldata holds verified independent unpack results from raw transaction data.
type DecodedCalldata struct {
	Method          string
	Target          common.Address
	RawAmount       *big.Int
	FormattedAmount string
}

// VerifyContractBytecode checks that an address contains EVM bytecode.
func VerifyContractBytecode(ctx context.Context, caller ChainCaller, contract common.Address) error {
	code, err := caller.CodeAt(ctx, contract, nil)
	if err != nil {
		return err
	}
	if len(code) == 0 {
		return ErrNotContract
	}
	return nil
}

// QueryERC20Metadata queries symbol and decimals from an on-chain ERC20 contract.
func QueryERC20Metadata(ctx context.Context, caller ChainCaller, contract common.Address) (string, int, error) {
	if err := VerifyContractBytecode(ctx, caller, contract); err != nil {
		return "", 0, err
	}

	// Query symbol()
	symData, err := erc20ABI.Pack("symbol")
	if err != nil {
		return "", 0, err
	}
	symRes, err := caller.CallContract(ctx, ethereum.CallMsg{To: &contract, Data: symData}, nil)
	if err != nil {
		return "", 0, err
	}
	var symbol string
	if err := erc20ABI.UnpackIntoInterface(&symbol, "symbol", symRes); err != nil {
		return "", 0, errors.New("無法解析代幣 symbol 返回值")
	}
	if len(symbol) == 0 || len(symbol) > 32 {
		return "", 0, ErrSymbolTooLong
	}

	// Query decimals()
	decData, err := erc20ABI.Pack("decimals")
	if err != nil {
		return "", 0, err
	}
	decRes, err := caller.CallContract(ctx, ethereum.CallMsg{To: &contract, Data: decData}, nil)
	if err != nil {
		return "", 0, err
	}
	var decimals uint8
	if err := erc20ABI.UnpackIntoInterface(&decimals, "decimals", decRes); err != nil {
		return "", 0, errors.New("無法解析代幣 decimals 返回值")
	}
	if decimals > 36 {
		return "", 0, ErrDecimalsTooLarge
	}

	return symbol, int(decimals), nil
}

// QueryERC20BalanceOf queries the raw balance of an account in the given ERC20 token.
func QueryERC20BalanceOf(ctx context.Context, caller ChainCaller, contract, account common.Address) (*big.Int, error) {
	data, err := erc20ABI.Pack("balanceOf", account)
	if err != nil {
		return nil, err
	}
	res, err := caller.CallContract(ctx, ethereum.CallMsg{To: &contract, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	var balance *big.Int
	if err := erc20ABI.UnpackIntoInterface(&balance, "balanceOf", res); err != nil {
		return nil, errors.New("無法解析代幣 balanceOf 返回值")
	}
	return balance, nil
}

// QueryERC20Allowance queries the current allowance from owner to spender.
func QueryERC20Allowance(ctx context.Context, caller ChainCaller, contract, owner, spender common.Address) (*big.Int, error) {
	data, err := erc20ABI.Pack("allowance", owner, spender)
	if err != nil {
		return nil, err
	}
	res, err := caller.CallContract(ctx, ethereum.CallMsg{To: &contract, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	var allowance *big.Int
	if err := erc20ABI.UnpackIntoInterface(&allowance, "allowance", res); err != nil {
		return nil, errors.New("無法解析代幣 allowance 返回值")
	}
	return allowance, nil
}

// SimulateERC20Call executes an eth_call simulation of a transfer or approve call.
// Accepts bool true or empty response (USDT and tokens omitting return value).
// Rejects false or malformed outputs.
func SimulateERC20Call(ctx context.Context, caller ChainCaller, from, to common.Address, data []byte) error {
	res, err := caller.CallContract(ctx, ethereum.CallMsg{From: from, To: &to, Data: data}, nil)
	if err != nil {
		return ErrSimulationFailed
	}
	// Supported empty result (e.g. USDT omitting return value)
	if len(res) == 0 {
		return nil
	}
	// Expected 32 bytes for bool
	if len(res) == 32 {
		val := new(big.Int).SetBytes(res)
		if val.Cmp(big.NewInt(1)) != 0 {
			return ErrSimulationFailed
		}
		return nil
	}
	return ErrSimulationFailed
}

// DecodeERC20Calldata independently unpacks and verifies the encoded calldata.
func DecodeERC20Calldata(data []byte, decimals int) (*DecodedCalldata, error) {
	if len(data) != 68 {
		return nil, errors.New("標準 transfer/approve calldata 必須為 68 位元組")
	}
	selector := data[:4]

	transferMethod := erc20ABI.Methods["transfer"]
	approveMethod := erc20ABI.Methods["approve"]

	if bytes.Equal(selector, transferMethod.ID) {
		unpacked, err := transferMethod.Inputs.Unpack(data[4:])
		if err != nil || len(unpacked) != 2 {
			return nil, errors.New("無法解析 transfer calldata")
		}
		target, ok1 := unpacked[0].(common.Address)
		amount, ok2 := unpacked[1].(*big.Int)
		if !ok1 || !ok2 {
			return nil, errors.New("transfer calldata 參數型別不符")
		}
		return &DecodedCalldata{
			Method:          "transfer",
			Target:          target,
			RawAmount:       amount,
			FormattedAmount: FormatUnits(amount, decimals),
		}, nil
	}

	if bytes.Equal(selector, approveMethod.ID) {
		unpacked, err := approveMethod.Inputs.Unpack(data[4:])
		if err != nil || len(unpacked) != 2 {
			return nil, errors.New("無法解析 approve calldata")
		}
		target, ok1 := unpacked[0].(common.Address)
		amount, ok2 := unpacked[1].(*big.Int)
		if !ok1 || !ok2 {
			return nil, errors.New("approve calldata 參數型別不符")
		}
		return &DecodedCalldata{
			Method:          "approve",
			Target:          target,
			RawAmount:       amount,
			FormattedAmount: FormatUnits(amount, decimals),
		}, nil
	}

	return nil, errors.New("未知的 ERC20 方法選擇器")
}
