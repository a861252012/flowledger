package erc4337

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestUserOp_JSON_EmptyObject 測試空 JSON 物件 "{}" 與 "null" 之反序列化與後續校驗防禦
func TestUserOp_JSON_EmptyObject(t *testing.T) {
	t.Parallel()

	// 1. 空 JSON 物件 "{}" 反序列化
	var op UserOperation
	emptyJSON := []byte("{}")
	if err := json.Unmarshal(emptyJSON, &op); err != nil {
		t.Fatalf("空 JSON 物件反序列化不應發生錯誤: %v", err)
	}

	// 驗證空物件反序列化後之欄位狀態：數值欄位應為 0，切片應為空切片，Sender 應為零地址
	if op.Sender != (common.Address{}) {
		t.Fatalf("Sender 預期為零地址，實際為: %s", op.Sender.Hex())
	}
	if op.Nonce == nil || op.Nonce.Sign() != 0 {
		t.Fatalf("Nonce 預期為 0，實際為: %v", op.Nonce)
	}
	if op.CallGasLimit == nil || op.CallGasLimit.Sign() != 0 {
		t.Fatalf("CallGasLimit 預期為 0，實際為: %v", op.CallGasLimit)
	}
	if len(op.InitCode) != 0 {
		t.Fatalf("InitCode 預期為空切片，實際長度: %d", len(op.InitCode))
	}

	// 呼叫 Validate() 必須安全攔截零地址 sender，絕不得 panic
	if err := op.Validate(); err != ErrInvalidSender {
		t.Fatalf("空物件校驗預期回傳 ErrInvalidSender，實際為: %v", err)
	}

	// 2. "null" 反序列化
	var nullOp UserOperation
	if err := json.Unmarshal([]byte("null"), &nullOp); err != nil {
		t.Fatalf("null 反序列化不應發生錯誤: %v", err)
	}

	// 3. PackedUserOperation 之空 JSON 物件 "{}" 反序列化
	var packedOp PackedUserOperation
	if err := json.Unmarshal(emptyJSON, &packedOp); err != nil {
		t.Fatalf("PackedUserOp 空 JSON 反序列化不應發生錯誤: %v", err)
	}
	if packedOp.Sender != (common.Address{}) {
		t.Fatalf("PackedUserOp Sender 預期為零地址")
	}
	if _, err := packedOp.Unpack(); err != ErrInvalidSender {
		t.Fatalf("PackedUserOp 空物件 Unpack 預期回傳 ErrInvalidSender，實際為: %v", err)
	}
}

// TestUserOp_JSON_MalformedHexAndMissingPrefix 測試畸形 Hex 輸入與缺乏 0x 前綴之拒絕能力
func TestUserOp_JSON_MalformedHexAndMissingPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		jsonPayload string
		expectErr   bool
		errContains string
	}{
		{
			name: "缺少 0x 前綴之純十進位 Nonce",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"nonce": "12345"
			}`,
			expectErr: true,
		},
		{
			name: "缺少 0x 前綴之十六進位 CallData",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"callData": "abcdef"
			}`,
			expectErr: true,
		},
		{
			name: "包含非十六進位字元之 Nonce",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"nonce": "0x12G4"
			}`,
			expectErr: true,
		},
		{
			name: "包含空白字元之 CallGasLimit",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"callGasLimit": "0x 186a0"
			}`,
			expectErr: true,
		},
		{
			name: "包含換行字元之 InitCode",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"initCode": "0x12\n34"
			}`,
			expectErr: true,
		},
		{
			name: "奇數長度之位元組切片 CallData",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"callData": "0x123"
			}`,
			expectErr: true,
		},
		{
			name: "負數十六進位 Nonce",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"nonce": "-0x1"
			}`,
			expectErr: true,
		},
		{
			name: "布林值型別欄位",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"nonce": true
			}`,
			expectErr: true,
		},
		{
			name: "陣列型別欄位",
			jsonPayload: `{
				"sender": "0x1111111111111111111111111111111111111111",
				"callData": ["0x12", "0x34"]
			}`,
			expectErr: true,
		},
		{
			name:        "截斷之不完整 JSON",
			jsonPayload: `{"sender": "0x1111111111111111111111111111111111111111", "nonce": `,
			expectErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var op UserOperation
			err := json.Unmarshal([]byte(tc.jsonPayload), &op)
			if tc.expectErr && err == nil {
				t.Fatalf("測試案例 [%s] 預期反序列化失敗，但成功通過", tc.name)
			}
		})
	}
}

// TestUserOp_JSON_Huge256BitAndBoundary 測試超大 256 位元數值之反序列化與校驗防禦
func TestUserOp_JSON_Huge256BitAndBoundary(t *testing.T) {
	t.Parallel()

	// 1. 剛好 256 位元 (2^256 - 1)
	maxValHex := "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	validJSON := fmt.Sprintf(`{
		"sender": "0x1111111111111111111111111111111111111111",
		"nonce": "%s",
		"initCode": "0x",
		"callData": "0x",
		"callGasLimit": "%s",
		"verificationGasLimit": "%s",
		"preVerificationGas": "%s",
		"maxFeePerGas": "%s",
		"maxPriorityFeePerGas": "%s",
		"paymasterAndData": "0x",
		"signature": "0x"
	}`, maxValHex, maxValHex, maxValHex, maxValHex, maxValHex, maxValHex)

	var opMax UserOperation
	if err := json.Unmarshal([]byte(validJSON), &opMax); err != nil {
		t.Fatalf("2^256 - 1 反序列化失敗: %v", err)
	}
	if err := opMax.Validate(); err != nil {
		t.Fatalf("2^256 - 1 校驗應通過，但失敗: %v", err)
	}

	// 2. 超出 256 位元 (2^256) 之十六進位字串
	overflowValHex := "0x10000000000000000000000000000000000000000000000000000000000000000"
	overflowJSON := fmt.Sprintf(`{
		"sender": "0x1111111111111111111111111111111111111111",
		"nonce": "%s",
		"initCode": "0x",
		"callData": "0x",
		"callGasLimit": "0x1",
		"verificationGasLimit": "0x1",
		"preVerificationGas": "0x1",
		"maxFeePerGas": "0x1",
		"maxPriorityFeePerGas": "0x1",
		"paymasterAndData": "0x",
		"signature": "0x"
	}`, overflowValHex)

	var opOverflow UserOperation
	err := json.Unmarshal([]byte(overflowJSON), &opOverflow)
	// 第一層防禦：hexutil.DecodeBig 在反序列化時即直接拒絕 > 256 bits 數值
	if err == nil || !strings.Contains(err.Error(), "hex number > 256 bits") {
		t.Fatalf("超出 256 位元數值之反序列化應被拒絕且包含 'hex number > 256 bits'，實際為: %v", err)
	}

	// 3. 超巨大 1024 位元數值反序列化拒絕
	hugeValHex := "0x" + strings.Repeat("ff", 128)
	hugeJSON := fmt.Sprintf(`{
		"sender": "0x1111111111111111111111111111111111111111",
		"nonce": "0x1",
		"callGasLimit": "%s",
		"verificationGasLimit": "0x1",
		"preVerificationGas": "0x1",
		"maxFeePerGas": "0x1",
		"maxPriorityFeePerGas": "0x1"
	}`, hugeValHex)

	var opHuge UserOperation
	err = json.Unmarshal([]byte(hugeJSON), &opHuge)
	if err == nil || !strings.Contains(err.Error(), "hex number > 256 bits") {
		t.Fatalf("1024 位元數值之反序列化應被拒絕且包含 'hex number > 256 bits'，實際為: %v", err)
	}

	// 4. 第二層防禦：直接從記憶體構建超出 256 位元之 UserOperation，驗證 Validate、PackUserOp 與 GetUserOpHash 均嚴格阻擋
	overflowBig := new(big.Int).Lsh(big.NewInt(1), 256)
	memoryOp := getOfficialTestOpV06()
	memoryOp.Nonce = new(big.Int).Set(overflowBig)

	if err := memoryOp.Validate(); err != ErrUint256Overflow {
		t.Fatalf("記憶體中超出 256 位元之 Validate 應回傳 ErrUint256Overflow，實際為: %v", err)
	}
	if _, err := PackUserOp(memoryOp); err != ErrUint256Overflow {
		t.Fatalf("記憶體中超出 256 位元之 PackUserOp 應回傳 ErrUint256Overflow，實際為: %v", err)
	}
	if _, err := GetUserOpHash(memoryOp, CanonicalEntryPointV06, big.NewInt(1)); err != ErrUint256Overflow {
		t.Fatalf("記憶體中超出 256 位元之 GetUserOpHash 應回傳 ErrUint256Overflow，實際為: %v", err)
	}
}

// TestPackedUserOp_JSON_MalformedFields 測試 PackedUserOperation 畸形欄位之反序列化拒絕
func TestPackedUserOp_JSON_MalformedFields(t *testing.T) {
	t.Parallel()

	// 1. AccountGasLimits 位元組長度不足 32 位元組（例如 16 位元組）
	shortLimitsJSON := `{
		"sender": "0x1111111111111111111111111111111111111111",
		"accountGasLimits": "0x00000000000000000000000000000001"
	}`
	var p1 PackedUserOperation
	if err := json.Unmarshal([]byte(shortLimitsJSON), &p1); err != ErrInvalidPackedGasLimits {
		t.Fatalf("AccountGasLimits 長度不足 32 位元組應回傳 ErrInvalidPackedGasLimits，實際為: %v", err)
	}

	// 2. AccountGasLimits 位元組長度超出 32 位元組（例如 33 位元組）
	longLimitsJSON := fmt.Sprintf(`{
		"sender": "0x1111111111111111111111111111111111111111",
		"accountGasLimits": "0x%s"
	}`, strings.Repeat("aa", 33))
	var p2 PackedUserOperation
	if err := json.Unmarshal([]byte(longLimitsJSON), &p2); err != ErrInvalidPackedGasLimits {
		t.Fatalf("AccountGasLimits 長度超過 32 位元組應回傳 ErrInvalidPackedGasLimits，實際為: %v", err)
	}

	// 3. GasFees 位元組長度不足 32 位元組
	shortFeesJSON := `{
		"sender": "0x1111111111111111111111111111111111111111",
		"gasFees": "0x00000000000000000000000000000001"
	}`
	var p3 PackedUserOperation
	if err := json.Unmarshal([]byte(shortFeesJSON), &p3); err != ErrInvalidPackedGasFees {
		t.Fatalf("GasFees 長度不足 32 位元組應回傳 ErrInvalidPackedGasFees，實際為: %v", err)
	}

	// 4. GasFees 包含非法字符
	invalidFeesJSON := fmt.Sprintf(`{
		"sender": "0x1111111111111111111111111111111111111111",
		"gasFees": "0x%sZZ"
	}`, strings.Repeat("00", 31))
	var p4 PackedUserOperation
	if err := json.Unmarshal([]byte(invalidFeesJSON), &p4); err != ErrInvalidPackedGasFees {
		t.Fatalf("GasFees 包含非法字符應回傳 ErrInvalidPackedGasFees，實際為: %v", err)
	}
}

// TestUserOp_FuzzLike_MalformedInputs 測試隨機與各類異常邊界字串反序列化防禦（不 panic）
func TestUserOp_FuzzLike_MalformedInputs(t *testing.T) {
	t.Parallel()

	corruptedInputs := [][]byte{
		[]byte(""),
		[]byte("   "),
		[]byte("{"),
		[]byte("}"),
		[]byte("[]"),
		[]byte("12345"),
		[]byte(`"0x1234"`),
		[]byte(`{"sender": null}`),
		[]byte(`{"sender": 123}`),
		[]byte(`{"nonce": null}`),
		[]byte(`{"nonce": ""}`),
		[]byte(`{"initCode": null}`),
		[]byte(`{"callData": "0x"}`),
		[]byte(`{"paymasterAndData": "0X"}`),
		[]byte(`{"signature": "0x"}`),
		[]byte(`{"sender": "0x0000000000000000000000000000000000000000", "extra_field": "ignore_me"}`),
		[]byte(`{"sender": "0x1111111111111111111111111111111111111111", "callGasLimit": "0x0"}`),
		bytes.Repeat([]byte("0"), 10000),
		bytes.Repeat([]byte("{"), 500),
	}

	for i, input := range corruptedInputs {
		var op UserOperation
		_ = json.Unmarshal(input, &op)

		var packedOp PackedUserOperation
		_ = json.Unmarshal(input, &packedOp)

		if i < 0 {
			t.Fatalf("不可到達分支")
		}
	}
}

// FuzzUserOpUnmarshalJSON 原生 Go Fuzz 測試：測試隨機位元組輸入不造成崩潰
func FuzzUserOpUnmarshalJSON(f *testing.F) {
	// 提供種子語料庫 (Seed Corpus)
	f.Add([]byte("{}"))
	f.Add([]byte(`{"sender":"0x1111111111111111111111111111111111111111","nonce":"0x0"}`))
	f.Add([]byte(`{"nonce":"12345"}`))
	f.Add([]byte(`{"callData":"0x123"}`))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var op UserOperation
		if err := json.Unmarshal(data, &op); err == nil {
			// 若反序列化成功，執行 Validate() 與 Clone() 驗證內部狀態安全
			_ = op.Validate()
			_ = op.Clone()
		}

		var packedOp PackedUserOperation
		if err := json.Unmarshal(data, &packedOp); err == nil {
			_ = packedOp.Clone()
			_, _ = packedOp.Unpack()
		}
	})
}

// TestUserOp_Adversarial_256Goroutines_RaceStress 測試以 256 個 goroutines 同時執行各項操作
// 檢測 Clone, Validate, MarshalJSON, UnmarshalJSON, GetUserOpHash 是否有資料競態
func TestUserOp_Adversarial_256Goroutines_RaceStress(t *testing.T) {
	t.Parallel()

	baseOp := getOfficialTestOpV06()
	baseOp.Signature = mustHex("0x8df0873ef40e3dccc92d4f723196a448af8cfcb83a53fbb59d041370fcb510093f05cff054024c8bc35b7b49a4647c6ec5c49865ede12847a635434454702e981c")

	packedBase, err := baseOp.ToPacked()
	if err != nil {
		t.Fatalf("初始化 ToPacked 失敗: %v", err)
	}

	marshaledJSON, err := json.Marshal(baseOp)
	if err != nil {
		t.Fatalf("初始化 Marshal 失敗: %v", err)
	}

	packedJSON, err := json.Marshal(packedBase)
	if err != nil {
		t.Fatalf("初始化 Packed Marshal 失敗: %v", err)
	}

	const totalGoroutines = 256
	const iterations = 40

	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

	expectedHashV06 := common.HexToHash("0xf09f5c22caf3315e500bd48ed65191b98ae05ef7840b2e4fabe9b0753e60fc51")
	expectedHashV07 := common.HexToHash("0xed3feb815570598eca9c065493d7dc58b8301a61018c8b4e9814ea6277ce3e91")
	chainID := big.NewInt(1)

	// 分配 4 組任務（每組 64 個 goroutines）
	for g := 0; g < totalGoroutines; g += 1 {
		group := g % 4
		go func(groupID, routineID int) {
			defer wg.Done()

			for iter := 0; iter < iterations; iter += 1 {
				switch groupID {
				case 0:
					// Group 0: 執行 Clone 與深拷貝修改驗證
					cloneOp := baseOp.Clone()
					if cloneOp == nil {
						t.Errorf("routine %d: Clone 回傳 nil", routineID)
						return
					}
					// 在拷貝本修改資料，驗證與其他 goroutines 完全隔離
					cloneOp.Nonce.Add(cloneOp.Nonce, big.NewInt(int64(routineID+1)))
					if len(cloneOp.CallData) > 0 {
						cloneOp.CallData[0] = byte(routineID % 256)
					}

					clonePacked := packedBase.Clone()
					if clonePacked == nil {
						t.Errorf("routine %d: Packed Clone 回傳 nil", routineID)
						return
					}

				case 1:
					// Group 1: 執行 Validate 與 ToPacked / Unpack
					if valErr := baseOp.Validate(); valErr != nil {
						t.Errorf("routine %d: Validate 失敗: %v", routineID, valErr)
						return
					}
					toPackedOp, pErr := baseOp.ToPacked()
					if pErr != nil {
						t.Errorf("routine %d: ToPacked 失敗: %v", routineID, pErr)
						return
					}
					unpackedOp, uErr := toPackedOp.Unpack()
					if uErr != nil {
						t.Errorf("routine %d: Unpack 失敗: %v", routineID, uErr)
						return
					}
					if unpackedOp.Sender != baseOp.Sender {
						t.Errorf("routine %d: Sender 不符", routineID)
						return
					}

				case 2:
					// Group 2: 執行 MarshalJSON 與 UnmarshalJSON
					data, mErr := json.Marshal(baseOp)
					if mErr != nil {
						t.Errorf("routine %d: MarshalJSON 失敗: %v", routineID, mErr)
						return
					}
					var restored UserOperation
					if umErr := json.Unmarshal(marshaledJSON, &restored); umErr != nil {
						t.Errorf("routine %d: UnmarshalJSON 失敗: %v", routineID, umErr)
						return
					}
					if len(data) == 0 {
						t.Errorf("routine %d: MarshalJSON 資料為空", routineID)
						return
					}

					var restoredPacked PackedUserOperation
					if upErr := json.Unmarshal(packedJSON, &restoredPacked); upErr != nil {
						t.Errorf("routine %d: Packed UnmarshalJSON 失敗: %v", routineID, upErr)
						return
					}

				case 3:
					// Group 3: 執行 GetUserOpHash (v0.6 與 v0.7)
					h06, hErr06 := GetUserOpHash(baseOp, CanonicalEntryPointV06, chainID)
					if hErr06 != nil || h06 != expectedHashV06 {
						t.Errorf("routine %d: Hash v0.6 計算錯誤: %v", routineID, hErr06)
						return
					}
					h07, hErr07 := GetUserOpHash(baseOp, CanonicalEntryPointV07, chainID)
					if hErr07 != nil || h07 != expectedHashV07 {
						t.Errorf("routine %d: Hash v0.7 計算錯誤: %v", routineID, hErr07)
						return
					}
				}
			}
		}(group, g)
	}

	wg.Wait()
}

// TestUserOp_Adversarial_LifecyclePipeline_200Goroutines 測試 200 個 goroutines 同時執行全生命週期流水線
func TestUserOp_Adversarial_LifecyclePipeline_200Goroutines(t *testing.T) {
	t.Parallel()

	const totalGoroutines = 200
	const iterations = 25

	baseOp := getOfficialTestOpV06()
	chainID := big.NewInt(1)
	entryPoint := CanonicalEntryPointV06

	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

	for g := 0; g < totalGoroutines; g += 1 {
		go func(routineID int) {
			defer wg.Done()

			for iter := 0; iter < iterations; iter += 1 {
				// 1. 深拷貝母體
				localOp := baseOp.Clone()

				// 2. 局部個性化變更（確保每個 goroutine 的資料具有唯一性）
				nonceDelta := int64(routineID*1000 + iter)
				localOp.Nonce = big.NewInt(nonceDelta)

				// 3. 校驗資料
				if err := localOp.Validate(); err != nil {
					t.Errorf("routine %d: Validate 失敗: %v", routineID, err)
					return
				}

				// 4. 計算 UserOpHash
				opHash, err := GetUserOpHash(localOp, entryPoint, chainID)
				if err != nil {
					t.Errorf("routine %d: GetUserOpHash 失敗: %v", routineID, err)
					return
				}
				if opHash == (common.Hash{}) {
					t.Errorf("routine %d: 產出空雜湊", routineID)
					return
				}

				// 5. JSON 序列化
				marshaled, err := json.Marshal(localOp)
				if err != nil {
					t.Errorf("routine %d: json.Marshal 失敗: %v", routineID, err)
					return
				}

				// 6. JSON 反序列化至新物件
				var restored UserOperation
				if err := json.Unmarshal(marshaled, &restored); err != nil {
					t.Errorf("routine %d: json.Unmarshal 失敗: %v", routineID, err)
					return
				}

				// 7. 驗證反序列化後一致性
				if restored.Nonce.Cmp(localOp.Nonce) != 0 {
					t.Errorf("routine %d: Nonce 不一致", routineID)
					return
				}

				// 8. 轉換為 Packed 格式
				packed, err := restored.ToPacked()
				if err != nil {
					t.Errorf("routine %d: ToPacked 失敗: %v", routineID, err)
					return
				}

				// 9. 反轉為 UserOperation 格式
				unpacked, err := packed.Unpack()
				if err != nil {
					t.Errorf("routine %d: Unpack 失敗: %v", routineID, err)
					return
				}
				if unpacked.Nonce.Cmp(localOp.Nonce) != 0 {
					t.Errorf("routine %d: Unpack Nonce 不一致", routineID)
					return
				}
			}
		}(g)
	}

	wg.Wait()
}

// TestUserOp_Adversarial_DeepCopy_IsolationStress 測試多 goroutines 併發拷貝並原地竄改，驗證母體 100% 免疫污染
func TestUserOp_Adversarial_DeepCopy_IsolationStress(t *testing.T) {
	t.Parallel()

	// 原始母體資料
	originalCallData := mustHex("0xb61d27f60000000000000000000000000000000000000000000000000000000000000001")
	originalNonce := big.NewInt(42)
	originalSig := []byte{0x01, 0x02, 0x03, 0x04}

	baseOp := &UserOperation{
		Sender:               common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:                new(big.Int).Set(originalNonce),
		InitCode:             []byte{0xaa, 0xbb},
		CallData:             slices.Clone(originalCallData),
		CallGasLimit:         big.NewInt(100000),
		VerificationGasLimit: big.NewInt(150000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(1000000000),
		MaxPriorityFeePerGas: big.NewInt(1000000000),
		PaymasterAndData:     []byte{0xcc, 0xdd},
		Signature:            slices.Clone(originalSig),
	}

	const goroutines = 128
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g += 1 {
		go func(routineID int) {
			defer wg.Done()
			for iter := 0; iter < iterations; iter += 1 {
				clone := baseOp.Clone()

				// 刻意竄改 clone 的切片內部資料與 big.Int 指標
				for idx := 0; idx < len(clone.CallData); idx += 1 {
					clone.CallData[idx] = byte((routineID + iter + idx) % 256)
				}
				for idx := 0; idx < len(clone.Signature); idx += 1 {
					clone.Signature[idx] = 0xff
				}
				clone.Nonce.SetInt64(999999)
				clone.CallGasLimit.SetInt64(888888)

				// 檢查 clone 本身確實已被修改
				if clone.Nonce.Int64() != 999999 {
					t.Errorf("routine %d: clone 竄改失敗", routineID)
					return
				}
			}
		}(g)
	}

	wg.Wait()

	// 驗證母體 baseOp 之資料絕對未受任何影響
	if baseOp.Nonce.Cmp(originalNonce) != 0 {
		t.Fatalf("母體 Nonce 遭受污染: 預期 %s, 實際 %s", originalNonce.String(), baseOp.Nonce.String())
	}
	if !bytes.Equal(baseOp.CallData, originalCallData) {
		t.Fatalf("母體 CallData 遭受切片污染！")
	}
	if !bytes.Equal(baseOp.Signature, originalSig) {
		t.Fatalf("母體 Signature 遭受切片污染！")
	}
	if baseOp.CallGasLimit.Int64() != 100000 {
		t.Fatalf("母體 CallGasLimit 遭受污染: %d", baseOp.CallGasLimit.Int64())
	}
}

// TestUserOp_JSON_MalformedSender 測試各類畸形 sender 地址之反序列化防禦
func TestUserOp_JSON_MalformedSender(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		sender string
	}{
		{name: "缺乏 0x 前綴", sender: `"1111111111111111111111111111111111111111"`},
		{name: "長度過短 (10 位元組)", sender: `"0x11111111111111111111"`},
		{name: "長度過長 (21 位元組)", sender: `"0x111111111111111111111111111111111111111122"`},
		{name: "包含非十六進位字元", sender: `"0x11111111111111111111111111111111111111ZZ"`},
		{name: "空字串", sender: `""`},
		{name: "僅有 0x", sender: `"0x"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			jsonPayload := fmt.Sprintf(`{"sender": %s}`, c.sender)
			var op UserOperation
			if err := json.Unmarshal([]byte(jsonPayload), &op); err == nil {
				t.Fatalf("案例 [%s] 預期反序列化失敗，但未回傳錯誤", c.name)
			}
		})
	}
}

// FuzzPackedUserOpUnmarshalJSON 原生 Go Fuzz 測試：測試 PackedUserOperation 隨機位元組不崩潰
func FuzzPackedUserOpUnmarshalJSON(f *testing.F) {
	f.Add([]byte("{}"))
	f.Add([]byte(`{"sender":"0x1111111111111111111111111111111111111111","accountGasLimits":"0x0000000000000000000000000000000000000000000000000000000000000000"}`))
	f.Add([]byte(`{"accountGasLimits":"0x1234"}`))
	f.Add([]byte(`{"gasFees":"0xZZ"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var packedOp PackedUserOperation
		if err := json.Unmarshal(data, &packedOp); err == nil {
			_ = packedOp.Clone()
			_, _ = packedOp.Unpack()
			_, _ = json.Marshal(&packedOp)
		}
	})
}

// TestUserOp_Adversarial_512Goroutines_MassiveRaceStress 測試以 512 個 goroutines 混和極限並發執行
func TestUserOp_Adversarial_512Goroutines_MassiveRaceStress(t *testing.T) {
	t.Parallel()

	baseOp := getOfficialTestOpV06()
	ep := CanonicalEntryPointV06
	chainID := big.NewInt(1)

	const totalGoroutines = 512
	const iterations = 20

	var wg sync.WaitGroup
	wg.Add(totalGoroutines)

	for g := 0; g < totalGoroutines; g += 1 {
		go func(routineID int) {
			defer wg.Done()
			for iter := 0; iter < iterations; iter += 1 {
				// 依照 routineID 輪替不同操作
				mode := (routineID + iter) % 5
				switch mode {
				case 0:
					_ = baseOp.Clone()
				case 1:
					_ = baseOp.Validate()
				case 2:
					data, err := json.Marshal(baseOp)
					if err == nil {
						var restored UserOperation
						_ = json.Unmarshal(data, &restored)
					}
				case 3:
					_, _ = GetUserOpHash(baseOp, ep, chainID)
				case 4:
					packed, err := baseOp.ToPacked()
					if err == nil {
						_, _ = packed.Unpack()
					}
				}
			}
		}(g)
	}

	wg.Wait()
}
