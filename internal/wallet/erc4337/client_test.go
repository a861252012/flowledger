package erc4337

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestClient_SendUserOperation_Success(t *testing.T) {
	mockServer := NewMockBundlerServer()
	defer mockServer.Close()

	client, err := NewClient(mockServer.URL())
	if err != nil {
		t.Fatalf("建立 Client 失敗: %v", err)
	}

	privKey, _ := crypto.GenerateKey()
	signer, _ := NewPrivateKeySigner(privKey)
	entryPoint := EntryPointV06
	chainID := big.NewInt(11155111)

	builder := NewBuilder(entryPoint, chainID).
		SetSender(signer.Address()).
		SetNonce(big.NewInt(0)).
		SetGasLimits(big.NewInt(100000), big.NewInt(100000), big.NewInt(21000)).
		SetGasFees(big.NewInt(2000000000), big.NewInt(1000000000))

	op, err := builder.BuildAndSign(signer)
	if err != nil {
		t.Fatalf("建構 UserOp 失敗: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hash, err := client.SendUserOperation(ctx, op, entryPoint)
	if err != nil {
		t.Fatalf("SendUserOperation 失敗: %v", err)
	}

	expectedHash, _ := GetUserOpHash(op, entryPoint, chainID)
	if hash != expectedHash {
		t.Fatalf("回傳之 UserOpHash 不符: 預期 %s, 得到 %s", expectedHash.Hex(), hash.Hex())
	}

	// 檢查 Server 是否收到請求
	requests := mockServer.RecordedRequests()
	if len(requests) != 1 || requests[0].Method != "eth_sendUserOperation" {
		t.Fatalf("Mock 伺服器未收到預期請求: %+v", requests)
	}
}

func TestClient_EstimateUserOperationGas(t *testing.T) {
	mockServer := NewMockBundlerServer()
	defer mockServer.Close()

	client, err := NewClient(mockServer.URL())
	if err != nil {
		t.Fatalf("建立 Client 失敗: %v", err)
	}

	op := &UserOperation{
		Sender: common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8"),
		Nonce:  big.NewInt(0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	est, err := client.EstimateUserOperationGas(ctx, op, EntryPointV06)
	if err != nil {
		t.Fatalf("EstimateUserOperationGas 失敗: %v", err)
	}

	if est.PreVerificationGas.Cmp(big.NewInt(0)) <= 0 {
		t.Fatalf("PreVerificationGas 必須大於 0")
	}
}

func TestClient_ReceiptAndPolling(t *testing.T) {
	mockServer := NewMockBundlerServer()
	defer mockServer.Close()

	client, _ := NewClient(mockServer.URL())
	testHash := common.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 尚未產生收據時應回傳 ErrReceiptNotFound
	_, err := client.GetUserOperationReceipt(ctx, testHash)
	if err != ErrReceiptNotFound {
		t.Fatalf("未完成時應回傳 ErrReceiptNotFound，得到 %v", err)
	}

	// 模擬背景 200ms 後收據出爐
	go func() {
		time.Sleep(200 * time.Millisecond)
		mockServer.SetReceipt(testHash, &UserOperationReceipt{
			UserOpHash:    testHash,
			EntryPoint:    EntryPointV06,
			Sender:        common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8"),
			Nonce:         big.NewInt(1),
			ActualGasCost: big.NewInt(50000000000000),
			ActualGasUsed: big.NewInt(75000),
			Success:       true,
			Receipt:       &types.Receipt{Status: 1, Logs: []*types.Log{}},
			Logs:          []*types.Log{},
		})
	}()

	receipt, err := client.WaitForUserOperationReceipt(ctx, testHash, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("輪詢收據失敗: %v", err)
	}
	if !receipt.Success || receipt.UserOpHash != testHash {
		t.Fatalf("收據內容不符: %+v", receipt)
	}
}

func TestClient_AAErrorParsing(t *testing.T) {
	mockServer := NewMockBundlerServer()
	defer mockServer.Close()

	client, _ := NewClient(mockServer.URL())
	mockServer.SetError("eth_sendUserOperation", ErrCodeValidationFailed, "AA21 didn't pay prefund")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	op := &UserOperation{
		Sender: common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8"),
		Nonce:  big.NewInt(0),
	}
	_, err := client.SendUserOperation(ctx, op, EntryPointV06)
	if err == nil {
		t.Fatalf("預期應回傳 AA 驗證失敗錯誤")
	}

	code := ParseAAErrorCode(err.Error())
	if code != "AA21" {
		t.Fatalf("解析 AA 錯誤碼失敗: 預期 AA21, 得到 %s", code)
	}
}
