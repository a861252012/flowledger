package erc4337

import (
	"bytes"
	"encoding/hex"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// userOperationRPC is the primitive JSON-RPC representation of an ERC-4337
// v0.6 user operation. Conversion to UserOperation is the trust boundary.
type userOperationRPC struct {
	Sender               string `json:"sender"`
	Nonce                string `json:"nonce"`
	InitCode             string `json:"initCode"`
	CallData             string `json:"callData"`
	CallGasLimit         string `json:"callGasLimit"`
	VerificationGasLimit string `json:"verificationGasLimit"`
	PreVerificationGas   string `json:"preVerificationGas"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	PaymasterAndData     string `json:"paymasterAndData"`
	Signature            string `json:"signature"`
}

// packedUserOperationRPC is the primitive JSON-RPC representation of an
// ERC-4337 v0.7 packed user operation.
type packedUserOperationRPC struct {
	Sender             string `json:"sender"`
	Nonce              string `json:"nonce"`
	InitCode           string `json:"initCode"`
	CallData           string `json:"callData"`
	AccountGasLimits   string `json:"accountGasLimits"`
	PreVerificationGas string `json:"preVerificationGas"`
	GasFees            string `json:"gasFees"`
	PaymasterAndData   string `json:"paymasterAndData"`
	Signature          string `json:"signature"`
}

func userOperationToRPC(op *UserOperation) userOperationRPC {
	return userOperationRPC{
		Sender:               hexutil.Encode(op.Sender[:]),
		Nonce:                encodeBigHex(op.Nonce),
		InitCode:             encodeBytesHex(op.InitCode),
		CallData:             encodeBytesHex(op.CallData),
		CallGasLimit:         encodeBigHex(op.CallGasLimit),
		VerificationGasLimit: encodeBigHex(op.VerificationGasLimit),
		PreVerificationGas:   encodeBigHex(op.PreVerificationGas),
		MaxFeePerGas:         encodeBigHex(op.MaxFeePerGas),
		MaxPriorityFeePerGas: encodeBigHex(op.MaxPriorityFeePerGas),
		PaymasterAndData:     encodeBytesHex(op.PaymasterAndData),
		Signature:            encodeBytesHex(op.Signature),
	}
}

func packedUserOperationToRPC(op *PackedUserOperation) packedUserOperationRPC {
	return packedUserOperationRPC{
		Sender:             hexutil.Encode(op.Sender[:]),
		Nonce:              encodeBigHex(op.Nonce),
		InitCode:           encodeBytesHex(op.InitCode),
		CallData:           encodeBytesHex(op.CallData),
		AccountGasLimits:   "0x" + hex.EncodeToString(op.AccountGasLimits[:]),
		PreVerificationGas: encodeBigHex(op.PreVerificationGas),
		GasFees:            "0x" + hex.EncodeToString(op.GasFees[:]),
		PaymasterAndData:   encodeBytesHex(op.PaymasterAndData),
		Signature:          encodeBytesHex(op.Signature),
	}
}

func userOperationRPCToDomain(rpc userOperationRPC) (*UserOperation, error) {

	sender, err := decodeRPCAddress(rpc.Sender)
	if err != nil {
		return nil, err
	}
	nonce, err := decodeBigHex(rpc.Nonce)
	if err != nil {
		return nil, err
	}
	initCode, err := decodeBytesHex(rpc.InitCode)
	if err != nil {
		return nil, err
	}
	callData, err := decodeBytesHex(rpc.CallData)
	if err != nil {
		return nil, err
	}
	callGasLimit, err := decodeBigHex(rpc.CallGasLimit)
	if err != nil {
		return nil, err
	}
	verificationGasLimit, err := decodeBigHex(rpc.VerificationGasLimit)
	if err != nil {
		return nil, err
	}
	preVerificationGas, err := decodeBigHex(rpc.PreVerificationGas)
	if err != nil {
		return nil, err
	}
	maxFeePerGas, err := decodeBigHex(rpc.MaxFeePerGas)
	if err != nil {
		return nil, err
	}
	maxPriorityFeePerGas, err := decodeBigHex(rpc.MaxPriorityFeePerGas)
	if err != nil {
		return nil, err
	}
	paymasterAndData, err := decodeBytesHex(rpc.PaymasterAndData)
	if err != nil {
		return nil, err
	}
	signature, err := decodeBytesHex(rpc.Signature)
	if err != nil {
		return nil, err
	}

	return &UserOperation{
		Sender: sender, Nonce: nonce, InitCode: initCode, CallData: callData,
		CallGasLimit: callGasLimit, VerificationGasLimit: verificationGasLimit,
		PreVerificationGas: preVerificationGas, MaxFeePerGas: maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		PaymasterAndData:     paymasterAndData, Signature: signature,
	}, nil
}

func packedUserOperationRPCToDomain(rpc packedUserOperationRPC) (*PackedUserOperation, error) {

	sender, err := decodeRPCAddress(rpc.Sender)
	if err != nil {
		return nil, err
	}
	nonce, err := decodeBigHex(rpc.Nonce)
	if err != nil {
		return nil, err
	}
	initCode, err := decodeBytesHex(rpc.InitCode)
	if err != nil {
		return nil, err
	}
	callData, err := decodeBytesHex(rpc.CallData)
	if err != nil {
		return nil, err
	}
	accountGasLimits, err := decodeRPCBytes32(rpc.AccountGasLimits, ErrInvalidPackedGasLimits)
	if err != nil {
		return nil, err
	}
	preVerificationGas, err := decodeBigHex(rpc.PreVerificationGas)
	if err != nil {
		return nil, err
	}
	gasFees, err := decodeRPCBytes32(rpc.GasFees, ErrInvalidPackedGasFees)
	if err != nil {
		return nil, err
	}
	paymasterAndData, err := decodeBytesHex(rpc.PaymasterAndData)
	if err != nil {
		return nil, err
	}
	signature, err := decodeBytesHex(rpc.Signature)
	if err != nil {
		return nil, err
	}

	return &PackedUserOperation{
		Sender: sender, Nonce: nonce, InitCode: initCode, CallData: callData,
		AccountGasLimits: accountGasLimits, PreVerificationGas: preVerificationGas,
		GasFees: gasFees, PaymasterAndData: paymasterAndData, Signature: signature,
	}, nil
}

func rejectExplicitEmptySender(input []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil {
		return err
	}
	raw, exists := fields["sender"]
	if !exists || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var sender string
	if err := json.Unmarshal(raw, &sender); err != nil {
		return err
	}
	if sender == "" {
		var address common.Address
		return address.UnmarshalText([]byte(sender))
	}
	return nil
}

func decodeRPCAddress(value string) (common.Address, error) {
	var address common.Address
	if value == "" {
		return address, nil
	}
	if err := address.UnmarshalText([]byte(value)); err != nil {
		return common.Address{}, err
	}
	return address, nil
}

func decodeRPCBytes32(value string, invalid error) ([32]byte, error) {
	var result [32]byte
	if value == "" || value == "0x" || value == "0X" {
		return result, nil
	}
	raw, err := decodeBytesHex(value)
	if err != nil || len(raw) != len(result) {
		return result, invalid
	}
	copy(result[:], raw)
	return result, nil
}
