package wallet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mr-tron/base58"
)

const TronShastaGenesis = "0000000000000000de1aa88295e1fcf982742f773e0419c5a9c134c994a9059e"

var errTronRPC = errors.New("TRON Shasta RPC 無法完成查核；結果未知，請稍後更新")

func TronAddress(address common.Address) string {
	payload := append([]byte{0x41}, address.Bytes()...)
	first := sha256.Sum256(payload)
	second := sha256.Sum256(first[:])
	return base58.Encode(append(payload, second[:4]...))
}
func ParseTronAddress(address string) ([]byte, error) {
	if len(address) != 34 {
		return nil, errors.New("請輸入完整的 TRON Base58Check 地址")
	}
	data, err := base58.Decode(address)
	if err != nil || len(data) != 25 || data[0] != 0x41 {
		return nil, errors.New("TRON 地址格式錯誤")
	}
	first := sha256.Sum256(data[:21])
	second := sha256.Sum256(first[:])
	if !bytes.Equal(data[21:], second[:4]) || bytes.Equal(data[1:21], make([]byte, 20)) {
		return nil, errors.New("TRON 地址校驗失敗或為零地址")
	}
	return data[:21], nil
}
func (s *TronService) call(ctx context.Context, path string, body, output any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", s.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return errTronRPC
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", s.apiKey)
	}
	response, err := s.http.Do(req)
	if err != nil {
		return errTronRPC
	}
	defer response.Body.Close()
	data, err = io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || response.StatusCode != 200 || len(data) > 2*1024*1024 {
		return errTronRPC
	}
	var failure struct {
		Error string `json:"Error"`
	}
	if json.Unmarshal(data, &failure) != nil || failure.Error != "" {
		return errTronRPC
	}
	if json.Unmarshal(data, output) != nil {
		return errTronRPC
	}
	return nil
}
func (s *TronService) check(ctx context.Context) error {
	var block struct {
		ID string `json:"blockID"`
	}
	if err := s.call(ctx, "/wallet/getblockbynum", map[string]int{"num": 0}, &block); err != nil {
		return err
	}
	if block.ID != TronShastaGenesis {
		return errors.New("RPC 並非 TRON Shasta，已停止操作")
	}
	return nil
}

type tronAccount struct {
	Address string `json:"address"`
	Balance int64  `json:"balance"`
}

func (s *TronService) account(ctx context.Context, address string) (*tronAccount, error) {
	if _, err := ParseTronAddress(address); err != nil {
		return nil, err
	}
	var result tronAccount
	if err := s.call(ctx, "/wallet/getaccount", map[string]any{"address": address, "visible": true}, &result); err != nil {
		return nil, err
	}
	if result.Balance < 0 {
		return nil, errTronRPC
	}
	return &result, nil
}
func (s *TronService) Balance(ctx context.Context, address string) (*TronBalance, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	account, err := s.account(ctx, address)
	if err != nil {
		return nil, err
	}
	var resources struct {
		NetLimit     int64
		NetUsed      int64
		FreeNetLimit int64
		FreeNetUsed  int64
		EnergyLimit  int64
		EnergyUsed   int64
	}
	if err := s.call(ctx, "/wallet/getaccountresource", map[string]any{"address": address, "visible": true}, &resources); err != nil {
		return nil, err
	}
	return &TronBalance{
		Address: address, TRX: FormatUnits(big.NewInt(account.Balance), 6), Active: account.Address != "",
		Bandwidth: max(0, resources.NetLimit-resources.NetUsed) + max(0, resources.FreeNetLimit-resources.FreeNetUsed),
		Energy:    max(0, resources.EnergyLimit-resources.EnergyUsed),
	}, nil
}
func (s *TronService) constant(ctx context.Context, owner, contract, method string, args ...any) ([]byte, int64, error) {
	if _, err := ParseTronAddress(contract); err != nil {
		return nil, 0, err
	}
	data, err := erc20ABI.Pack(method, args...)
	if err != nil {
		return nil, 0, err
	}
	var result struct {
		Result         struct{ Result bool }
		ConstantResult []string `json:"constant_result"`
		Energy         int64    `json:"energy_used"`
	}
	err = s.call(ctx, "/wallet/triggerconstantcontract", map[string]any{"owner_address": owner, "contract_address": contract, "function_selector": erc20ABI.Methods[method].Sig, "parameter": hex.EncodeToString(data[4:]), "visible": true}, &result)
	if err != nil || !result.Result.Result || len(result.ConstantResult) != 1 {
		return nil, 0, ErrSimulationFailed
	}
	raw, err := hex.DecodeString(result.ConstantResult[0])
	if err != nil {
		return nil, 0, ErrSimulationFailed
	}
	return raw, result.Energy, nil
}

type TronToken struct {
	Contract string   `json:"contract"`
	Symbol   string   `json:"symbol"`
	Decimals int      `json:"decimals"`
	Balance  string   `json:"balance"`
	Raw      *big.Int `json:"-"`
}

func (s *TronService) token(ctx context.Context, owner, contract string) (*TronToken, error) {
	a, err := ParseTronAddress(owner)
	if err != nil {
		return nil, err
	}
	token := &TronToken{Contract: contract}
	for _, method := range []string{"symbol", "decimals", "balanceOf"} {
		var args []any
		if method == "balanceOf" {
			args = []any{common.BytesToAddress(a[1:])}
		}
		raw, _, err := s.constant(ctx, owner, contract, method, args...)
		if err != nil {
			return nil, err
		}
		values, err := erc20ABI.Unpack(method, raw)
		if err != nil || len(values) != 1 {
			return nil, ErrSimulationFailed
		}
		switch method {
		case "symbol":
			token.Symbol = values[0].(string)
			if len(token.Symbol) == 0 || len(token.Symbol) > 32 {
				return nil, ErrSymbolTooLong
			}
		case "decimals":
			token.Decimals = int(values[0].(uint8))
			if token.Decimals > 36 {
				return nil, ErrDecimalsTooLarge
			}
		case "balanceOf":
			token.Raw = values[0].(*big.Int)
			token.Balance = FormatUnits(token.Raw, token.Decimals)
		}
	}
	return token, nil
}
func (s *TronService) Token(ctx context.Context, owner, contract string) (*TronToken, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	return s.token(ctx, owner, contract)
}

// Wire fields follow tronprotocol/protocol core/Tron.proto and contract/*.proto.
// Only locally constructed TransferContract and TriggerSmartContract are signed.
func tronBytes(dst []byte, field uint64, value []byte) []byte {
	dst = binary.AppendUvarint(dst, field<<3|2)
	dst = binary.AppendUvarint(dst, uint64(len(value)))
	return append(dst, value...)
}
func tronInt(dst []byte, field uint64, value int64) []byte {
	if value == 0 {
		return dst
	}
	dst = binary.AppendUvarint(dst, field<<3)
	return binary.AppendUvarint(dst, uint64(value))
}
func tronRaw(owner, to []byte, amount *big.Int, contract []byte, blockID string, now, expires, feeLimit int64) ([]byte, error) {
	block, err := hex.DecodeString(blockID)
	if err != nil || len(block) != 32 {
		return nil, errTronRPC
	}
	payload := tronBytes(nil, 1, owner)
	name := "TransferContract"
	kind := int64(1)
	if len(contract) == 0 {
		payload = tronBytes(payload, 2, to)
		payload = tronInt(payload, 3, amount.Int64())
	} else {
		name = "TriggerSmartContract"
		kind = 31
		data, err := erc20ABI.Pack("transfer", common.BytesToAddress(to[1:]), amount)
		if err != nil {
			return nil, err
		}
		payload = tronBytes(payload, 2, contract)
		payload = tronBytes(payload, 4, data)
	}
	parameter := tronBytes(nil, 1, []byte("type.googleapis.com/protocol."+name))
	parameter = tronBytes(parameter, 2, payload)
	operation := tronInt(nil, 1, kind)
	operation = tronBytes(operation, 2, parameter)
	raw := tronBytes(nil, 1, block[6:8])
	raw = tronBytes(raw, 4, block[8:16])
	raw = tronInt(raw, 8, expires)
	raw = tronBytes(raw, 11, operation)
	raw = tronInt(raw, 14, now)
	raw = tronInt(raw, 18, feeLimit)
	return raw, nil
}
func validTronHash(hash string) bool {
	raw, err := hex.DecodeString(hash)
	return err == nil && len(raw) == 32 && hash == strings.ToLower(hash)
}
