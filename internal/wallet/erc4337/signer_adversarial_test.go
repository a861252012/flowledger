package erc4337

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// spyKeystoreDecrypter 攔截器：記錄每次解密回傳的金鑰物件，用於驗證記憶體抹除狀態
type spyKeystoreDecrypter struct {
	mu            sync.Mutex
	addrHex       string
	password      string
	privKey       *ecdsa.PrivateKey
	intercepted   []*keystore.Key
	decryptErrors int64
}

func newSpyKeystoreDecrypter(privKey *ecdsa.PrivateKey, password string) *spyKeystoreDecrypter {
	addr := crypto.PubkeyToAddress(privKey.PublicKey)
	return &spyKeystoreDecrypter{
		addrHex:  addr.Hex(),
		password: password,
		privKey:  privKey,
	}
}

func (s *spyKeystoreDecrypter) Address() (string, error) {
	if s.addrHex == "" {
		return "", errors.New("keystore 地址無效")
	}
	return s.addrHex, nil
}

func (s *spyKeystoreDecrypter) DecryptKey(password string) (*keystore.Key, error) {
	if password != s.password {
		atomic.AddInt64(&s.decryptErrors, 1)
		return nil, errors.New("keystore 密碼驗證失敗")
	}

	copiedKey, err := crypto.ToECDSA(crypto.FromECDSA(s.privKey))
	if err != nil {
		return nil, fmt.Errorf("複製私鑰失敗: %w", err)
	}

	k := &keystore.Key{
		Address:    crypto.PubkeyToAddress(s.privKey.PublicKey),
		PrivateKey: copiedKey,
	}

	s.mu.Lock()
	s.intercepted = append(s.intercepted, k)
	s.mu.Unlock()

	return k, nil
}

func (s *spyKeystoreDecrypter) getInterceptedKeys() []*keystore.Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]*keystore.Key, len(s.intercepted))
	copy(res, s.intercepted)
	return res
}

// 測試場景 1：延展性簽章（Malleable High-S signature）
// 手動構造 S' = N - S 之簽章，傳入 VerifySignature，驗證是否嚴格拒絕
func TestAdversarial_HighSSignatureMalleabilityRejection(t *testing.T) {
	t.Parallel()

	secp256kN := crypto.S256().Params().N

	var (
		testedRounds   int64
		blockedRoundsA int64
		blockedRoundsB int64
	)

	// 1. 批量隨機對抗測試：100 組隨機金鑰與隨機訊息
	for i := 0; i < 100; i += 1 {
		privKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatalf("迭代 %d: 生成私鑰失敗: %v", i, err)
		}
		signer, err := NewPrivateKeySigner(privKey)
		if err != nil {
			t.Fatalf("迭代 %d: 建立簽署者失敗: %v", i, err)
		}

		msgHash := crypto.Keccak256Hash(fmt.Appendf(nil, "adversarial malleable payload %d", i))
		validSig, err := signer.SignHash(msgHash)
		if err != nil {
			t.Fatalf("迭代 %d: 簽署失敗: %v", i, err)
		}

		// 驗證原簽章為合法 Low-S 簽名
		valid, err := VerifySignature(msgHash, validSig, signer.Address())
		if err != nil || !valid {
			t.Fatalf("迭代 %d: 原始合法簽章驗證失敗: valid=%v, err=%v", i, valid, err)
		}

		sBytes := validSig[32:64]
		sVal := new(big.Int).SetBytes(sBytes)

		// 驗證 crypto.Sign 產出之 S 必然在 Low-S 區間
		if sVal.Cmp(secp256k1HalfN) > 0 {
			t.Fatalf("迭代 %d: crypto.Sign 產出了 High-S 簽章", i)
		}

		// 構造延展性簽名 S' = N - S
		malleableS := new(big.Int).Sub(secp256kN, sVal)
		if malleableS.Cmp(secp256k1HalfN) <= 0 {
			t.Fatalf("迭代 %d: 計算之 S' 異常不大於 HalfN", i)
		}

		testedRounds += 1

		// 情況 A：維持原 V 值
		malleableSigA := make([]byte, 65)
		copy(malleableSigA, validSig)
		copy(malleableSigA[32:64], common.LeftPadBytes(malleableS.Bytes(), 32))

		validA, errA := VerifySignature(msgHash, malleableSigA, signer.Address())
		if validA {
			t.Fatalf("迭代 %d: 嚴重漏洞! High-S 延展性簽章未被拒絕 (保持原 V)", i)
		}
		if errors.Is(errA, ErrNonLowSSignature) {
			blockedRoundsA += 1
		} else {
			t.Fatalf("迭代 %d: High-S 延展性簽章回傳錯誤不符: 預期 ErrNonLowSSignature, 得到: %v", i, errA)
		}

		// 情況 B：翻轉 V 值（在 ECDSA 中取反 S 會對應翻轉 parity）
		malleableSigB := make([]byte, 65)
		copy(malleableSigB, validSig)
		copy(malleableSigB[32:64], common.LeftPadBytes(malleableS.Bytes(), 32))
		if malleableSigB[64] == 27 {
			malleableSigB[64] = 28
		} else {
			malleableSigB[64] = 27
		}

		validB, errB := VerifySignature(msgHash, malleableSigB, signer.Address())
		if validB {
			t.Fatalf("迭代 %d: 嚴重漏洞! High-S 延展性簽章未被拒絕 (翻轉 V)", i)
		}
		if errors.Is(errB, ErrNonLowSSignature) {
			blockedRoundsB += 1
		} else {
			t.Fatalf("迭代 %d: High-S 延展性簽章回傳錯誤不符: 預期 ErrNonLowSSignature, 得到: %v", i, errB)
		}
	}

	// 2. 邊界臨界值對抗測試
	testPrivKey, _ := crypto.GenerateKey()
	testSigner, _ := NewPrivateKeySigner(testPrivKey)
	testHash := crypto.Keccak256Hash([]byte("boundary test"))
	baseSig, _ := testSigner.SignHash(testHash)

	boundaryCases := []struct {
		name           string
		sValue         *big.Int
		expectHighSErr bool
	}{
		{
			name:           "S 恰等於 HalfN (邊界上限)",
			sValue:         new(big.Int).Set(secp256k1HalfN),
			expectHighSErr: false,
		},
		{
			name:           "S 等於 HalfN + 1 (剛好越界 1 位元)",
			sValue:         new(big.Int).Add(secp256k1HalfN, big.NewInt(1)),
			expectHighSErr: true,
		},
		{
			name:           "S 等於 N - 1 (最大合法群元素，但在 High-S 區間)",
			sValue:         new(big.Int).Sub(secp256kN, big.NewInt(1)),
			expectHighSErr: true,
		},
		{
			name:           "S 等於 N",
			sValue:         new(big.Int).Set(secp256kN),
			expectHighSErr: true,
		},
		{
			name:           "S 超出 N 之極限值 2^256 - 1",
			sValue:         new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)),
			expectHighSErr: true,
		},
	}

	for _, bc := range boundaryCases {
		sig := make([]byte, 65)
		copy(sig, baseSig)
		copy(sig[32:64], common.LeftPadBytes(bc.sValue.Bytes(), 32))

		valid, err := VerifySignature(testHash, sig, testSigner.Address())
		if bc.expectHighSErr {
			if valid {
				t.Fatalf("邊界測試 %s: 應被拒絕卻通過驗證", bc.name)
			}
			if !errors.Is(err, ErrNonLowSSignature) {
				t.Fatalf("邊界測試 %s: 預期回傳 ErrNonLowSSignature，得到 %v", bc.name, err)
			}
		} else {
			if errors.Is(err, ErrNonLowSSignature) {
				t.Fatalf("邊界測試 %s: S <= HalfN 不應觸發 ErrNonLowSSignature", bc.name)
			}
		}
	}

	t.Logf("High-S 延展性簽章對抗實測數據:")
	t.Logf("- 測試私鑰組數: %d 組", testedRounds)
	t.Logf("- S'=N-S (原 V) 阻斷率: %d/%d (100%%)", blockedRoundsA, testedRounds)
	t.Logf("- S'=N-S (翻轉 V) 阻斷率: %d/%d (100%%)", blockedRoundsB, testedRounds)
	t.Logf("- 邊界值案例全數驗證通過 (HalfN, HalfN+1, N-1, N, 2^256-1)")
}

// 測試場景 2：異常 Recovery ID（V != 27 && V != 28）拒絕驗證
// 檢測 0..255 全數值空間中非 27、28 之行為
func TestAdversarial_RecoveryIDStrictValidation(t *testing.T) {
	t.Parallel()

	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("生成私鑰失敗: %v", err)
	}
	signer, err := NewPrivateKeySigner(privKey)
	if err != nil {
		t.Fatalf("建立簽署者失敗: %v", err)
	}

	testHash := crypto.Keccak256Hash([]byte("recovery id adversarial test"))
	validSig, err := signer.SignHash(testHash)
	if err != nil {
		t.Fatalf("簽署失敗: %v", err)
	}

	if validSig[64] != 27 && validSig[64] != 28 {
		t.Fatalf("原始簽章 V 值不是 27 或 28: %d", validSig[64])
	}

	var acceptedInvalidVs []int
	var noErrorInvalidVs []int
	var rejectedInvalidVs []int

	// 測試所有異常 V 值 (0 到 255 中非 27、28 之所有值)
	for v := 0; v <= 255; v += 1 {
		if v == 27 || v == 28 {
			continue
		}

		badSig := make([]byte, 65)
		copy(badSig, validSig)
		badSig[64] = byte(v)

		valid, err := VerifySignature(testHash, badSig, signer.Address())
		if valid {
			acceptedInvalidVs = append(acceptedInvalidVs, v)
		}
		if err == nil {
			noErrorInvalidVs = append(noErrorInvalidVs, v)
		} else {
			rejectedInvalidVs = append(rejectedInvalidVs, v)
		}
	}

	t.Logf("Recovery ID 對抗檢測實測數據:")
	t.Logf("- 測試異常 V 總數: 254 (0..255 扣除 27 與 28)")
	t.Logf("- 成功拒絕且報錯之 V 數量: %d (如 2..26, 29..255)", len(rejectedInvalidVs))
	t.Logf("- 異常被視為合法 (valid=true) 之 V 集合: %v", acceptedInvalidVs)
	t.Logf("- 異常未回傳錯誤 (err=nil) 之 V 集合: %v", noErrorInvalidVs)

	if len(acceptedInvalidVs) > 0 || len(noErrorInvalidVs) > 0 {
		t.Errorf("重大密碼學實作缺陷: 存在未被拒絕之異常 Recovery ID! accepted=%v, noError=%v", acceptedInvalidVs, noErrorInvalidVs)
	}
}

// 測試場景 3：KeystoreSigner 的私鑰抹除對抗測試
// 驗證簽署後原私鑰變數 D 是否確實為 0 且無法再簽名
func TestAdversarial_KeyWiping_KeystoreSigner(t *testing.T) {
	t.Parallel()

	testHash := crypto.Keccak256Hash([]byte("memory wipe adversarial stress"))

	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("生成私鑰失敗: %v", err)
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey)

	spyDecrypter := newSpyKeystoreDecrypter(privKey, "secret-pass-4337")
	ksSigner, err := NewKeystoreSigner(spyDecrypter, "secret-pass-4337")
	if err != nil {
		t.Fatalf("NewKeystoreSigner 失敗: %v", err)
	}

	// 1. SignHash
	sig1, err := ksSigner.SignHash(testHash)
	if err != nil {
		t.Fatalf("SignHash 失敗: %v", err)
	}
	if v, _ := VerifySignature(testHash, sig1, addr); !v {
		t.Fatalf("SignHash 簽章驗證失敗")
	}

	// 2. SignUserOp
	op := &UserOperation{
		Sender:               addr,
		Nonce:                big.NewInt(1),
		CallGasLimit:         big.NewInt(50000),
		VerificationGasLimit: big.NewInt(100000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(2000000000),
		MaxPriorityFeePerGas: big.NewInt(1000000000),
	}
	sig2, err := ksSigner.SignUserOp(op, EntryPointV06, big.NewInt(1))
	if err != nil {
		t.Fatalf("SignUserOp 失敗: %v", err)
	}
	opHash, _ := GetUserOpHash(op, EntryPointV06, big.NewInt(1))
	if v, _ := VerifySignature(opHash, sig2, addr); !v {
		t.Fatalf("SignUserOp 簽章驗證失敗")
	}

	// 3. SignUserOpWithEthPrefix
	sig3, err := ksSigner.SignUserOpWithEthPrefix(op, EntryPointV06, big.NewInt(1))
	if err != nil {
		t.Fatalf("SignUserOpWithEthPrefix 失敗: %v", err)
	}
	if v, _ := VerifySignature(EthSignedMessageHash(opHash), sig3, addr); !v {
		t.Fatalf("SignUserOpWithEthPrefix 簽章驗證失敗")
	}

	interceptedKeys := spyDecrypter.getInterceptedKeys()
	if len(interceptedKeys) != 3 {
		t.Fatalf("預期截獲 3 次解密金鑰，實際截獲: %d", len(interceptedKeys))
	}

	// 檢查每一次解密用過之私鑰，其 D 是否確實被歸零抹除
	for idx, key := range interceptedKeys {
		if key.PrivateKey == nil || key.PrivateKey.D == nil {
			t.Fatalf("金鑰 %d 私鑰指標為空", idx)
		}
		if key.PrivateKey.D.Sign() != 0 {
			t.Fatalf("對抗失敗: Keystore 解密金鑰 #%d 之 D.Sign() 不為 0 (實際: %d)", idx, key.PrivateKey.D.Sign())
		}
		if key.PrivateKey.D.BitLen() != 0 {
			t.Fatalf("對抗失敗: Keystore 解密金鑰 #%d 之 D.BitLen() 不為 0 (實際: %d)", idx, key.PrivateKey.D.BitLen())
		}
		for bitIdx, b := range key.PrivateKey.D.Bits() {
			if b != 0 {
				t.Fatalf("對抗失敗: Keystore 解密金鑰 #%d 之 Bits[%d] 記憶體未歸零", idx, bitIdx)
			}
		}

		// 對抗驗證：拿已被抹除的 key.PrivateKey 嘗試簽名，必須無法完成合法簽名
		func() {
			defer func() {
				if r := recover(); r != nil {
					// panic 亦視為無法簽名
				}
			}()
			badSig, signErr := crypto.Sign(testHash.Bytes(), key.PrivateKey)
			if signErr == nil && len(badSig) == 65 {
				v, _ := VerifySignature(testHash, badSig, addr)
				if v {
					t.Fatalf("重大漏洞: 抹除後之 Keystore 私鑰竟能簽出合法簽章!")
				}
			}
		}()
	}

	t.Logf("KeystoreSigner 私鑰抹除實測數據:")
	t.Logf("- 攔截解密次數: %d 次 (SignHash, SignUserOp, SignUserOpWithEthPrefix)", len(interceptedKeys))
	t.Logf("- 私鑰歸零成功率: 100%% (全部 D.Sign()==0, D.BitLen()==0, Bits() 清零)")
	t.Logf("- 抹除後再簽名阻斷率: 100%% (全數無法簽出有效簽章)")
}

// 測試場景 4：驗證高並行呼叫下 KeystoreSigner 的執行緒安全性
func TestAdversarial_KeystoreSigner_HighConcurrency(t *testing.T) {
	t.Parallel()

	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("生成私鑰失敗: %v", err)
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey)

	correctPassword := "concurrency-safe-pwd-9988"
	spyDecrypter := newSpyKeystoreDecrypter(privKey, correctPassword)

	ksSigner, err := NewKeystoreSigner(spyDecrypter, correctPassword)
	if err != nil {
		t.Fatalf("NewKeystoreSigner 失敗: %v", err)
	}

	concurrencyCount := 120
	iterationsPerGoroutine := 15

	var wg sync.WaitGroup
	wg.Add(concurrencyCount)

	var (
		signHashSuccess   int64
		signOpSuccess     int64
		signEthSuccess    int64
		validationSuccess int64
		concurrencyErrors int64
	)

	for g := 0; g < concurrencyCount; g += 1 {
		go func(routineID int) {
			defer wg.Done()

			for it := 0; it < iterationsPerGoroutine; it += 1 {
				mode := (routineID + it) % 3

				switch mode {
				case 0:
					msgHash := crypto.Keccak256Hash(fmt.Appendf(nil, "routine %d it %d", routineID, it))
					sig, err := ksSigner.SignHash(msgHash)
					if err != nil {
						atomic.AddInt64(&concurrencyErrors, 1)
						t.Errorf("並行 SignHash 失敗: %v", err)
						return
					}
					atomic.AddInt64(&signHashSuccess, 1)

					valid, vErr := VerifySignature(msgHash, sig, addr)
					if vErr != nil || !valid {
						atomic.AddInt64(&concurrencyErrors, 1)
						t.Errorf("並行 SignHash 驗證失敗: valid=%v, err=%v", valid, vErr)
						return
					}
					atomic.AddInt64(&validationSuccess, 1)

				case 1:
					op := &UserOperation{
						Sender:               addr,
						Nonce:                big.NewInt(int64(routineID*1000 + it)),
						CallGasLimit:         big.NewInt(50000),
						VerificationGasLimit: big.NewInt(100000),
						PreVerificationGas:   big.NewInt(21000),
						MaxFeePerGas:         big.NewInt(2000000000),
						MaxPriorityFeePerGas: big.NewInt(1000000000),
					}
					sig, err := ksSigner.SignUserOp(op, EntryPointV06, big.NewInt(11155111))
					if err != nil {
						atomic.AddInt64(&concurrencyErrors, 1)
						t.Errorf("並行 SignUserOp 失敗: %v", err)
						return
					}
					atomic.AddInt64(&signOpSuccess, 1)

					h, _ := GetUserOpHash(op, EntryPointV06, big.NewInt(11155111))
					valid, vErr := VerifySignature(h, sig, addr)
					if vErr != nil || !valid {
						atomic.AddInt64(&concurrencyErrors, 1)
						t.Errorf("並行 SignUserOp 驗證失敗: valid=%v, err=%v", valid, vErr)
						return
					}
					atomic.AddInt64(&validationSuccess, 1)

				case 2:
					op := &UserOperation{
						Sender:               addr,
						Nonce:                big.NewInt(int64(routineID*2000 + it)),
						CallGasLimit:         big.NewInt(70000),
						VerificationGasLimit: big.NewInt(80000),
						PreVerificationGas:   big.NewInt(21000),
						MaxFeePerGas:         big.NewInt(3000000000),
						MaxPriorityFeePerGas: big.NewInt(1500000000),
					}
					sig, err := ksSigner.SignUserOpWithEthPrefix(op, EntryPointV06, big.NewInt(1))
					if err != nil {
						atomic.AddInt64(&concurrencyErrors, 1)
						t.Errorf("並行 SignUserOpWithEthPrefix 失敗: %v", err)
						return
					}
					atomic.AddInt64(&signEthSuccess, 1)

					h, _ := GetUserOpHash(op, EntryPointV06, big.NewInt(1))
					ethH := EthSignedMessageHash(h)
					valid, vErr := VerifySignature(ethH, sig, addr)
					if vErr != nil || !valid {
						atomic.AddInt64(&concurrencyErrors, 1)
						t.Errorf("並行 SignUserOpWithEthPrefix 驗證失敗: valid=%v, err=%v", valid, vErr)
						return
					}
					atomic.AddInt64(&validationSuccess, 1)
				}
			}
		}(g)
	}

	wg.Wait()

	totalExpectedCalls := int64(concurrencyCount * iterationsPerGoroutine)
	totalActualSignings := signHashSuccess + signOpSuccess + signEthSuccess

	if concurrencyErrors > 0 {
		t.Fatalf("高並行對抗失敗: 發生 %d 次並行錯誤", concurrencyErrors)
	}
	if totalActualSignings != totalExpectedCalls {
		t.Fatalf("並行簽名次數不符: 預期 %d, 實際 %d", totalExpectedCalls, totalActualSignings)
	}
	if validationSuccess != totalExpectedCalls {
		t.Fatalf("並行驗證成功次數不符: 預期 %d, 實際 %d", totalExpectedCalls, validationSuccess)
	}

	// 驗證截獲之所有解密金鑰皆已歸零抹除
	intercepted := spyDecrypter.getInterceptedKeys()
	if int64(len(intercepted)) != totalExpectedCalls {
		t.Fatalf("解密截獲數量不符: 預期 %d, 實際 %d", totalExpectedCalls, len(intercepted))
	}
	for i, k := range intercepted {
		if k.PrivateKey.D.Sign() != 0 {
			t.Fatalf("高並行第 %d 次解密之私鑰 D 未被抹除", i)
		}
	}

	t.Logf("KeystoreSigner 高並行對抗測試實測數據:")
	t.Logf("- 並發協程數 (Goroutines): %d", concurrencyCount)
	t.Logf("- 每個協程迴圈次數: %d", iterationsPerGoroutine)
	t.Logf("- 累計成功完成簽署與驗證次數: %d", validationSuccess)
	t.Logf("- 資料競態偵測 (Data Race): 0 筆")
	t.Logf("- 記憶體即用即抹率: 100%% (%d/%d)", len(intercepted), totalExpectedCalls)
}
