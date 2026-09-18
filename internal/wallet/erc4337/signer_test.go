package erc4337

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestPrivateKeySigner_SignAndVerify(t *testing.T) {
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("生成私鑰失敗: %v", err)
	}

	signer, err := NewPrivateKeySigner(privKey)
	if err != nil {
		t.Fatalf("建立簽署者失敗: %v", err)
	}

	expectedAddr := crypto.PubkeyToAddress(privKey.PublicKey)
	if signer.Address() != expectedAddr {
		t.Fatalf("地址不相符: 預期 %s, 得到 %s", expectedAddr.Hex(), signer.Address().Hex())
	}

	testHash := crypto.Keccak256Hash([]byte("hello erc4337"))
	sig, err := signer.SignHash(testHash)
	if err != nil {
		t.Fatalf("簽署哈希失敗: %v", err)
	}
	if len(sig) != 65 {
		t.Fatalf("簽章長度必須為 65 位元組，得到 %d", len(sig))
	}
	if sig[64] != 27 && sig[64] != 28 {
		t.Fatalf("簽章 V 值必須為 27 或 28，得到 %d", sig[64])
	}

	valid, err := VerifySignature(testHash, sig, expectedAddr)
	if err != nil || !valid {
		t.Fatalf("驗證簽章失敗: valid=%v, err=%v", valid, err)
	}

	// 驗證錯誤地址還原失敗
	wrongAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	validWrong, err := VerifySignature(testHash, sig, wrongAddr)
	if err != nil || validWrong {
		t.Fatalf("錯誤地址不應驗證成功")
	}

	// 測試 Wipe 抹除功能
	signer.Wipe()
	_, err = signer.SignHash(testHash)
	if err == nil {
		t.Fatalf("抹除後簽署應報錯")
	}
}

// mockKeystoreDecrypter 模擬 KeystoreManager 解密行為
type mockKeystoreDecrypter struct {
	addrHex  string
	key      *keystore.Key
	password string
}

func (m *mockKeystoreDecrypter) Address() (string, error) {
	if m.addrHex == "" {
		return "", errors.New("keystore 無有效地址")
	}
	return m.addrHex, nil
}

func (m *mockKeystoreDecrypter) DecryptKey(password string) (*keystore.Key, error) {
	if password != m.password {
		return nil, errors.New("密碼錯誤")
	}
	// 回傳複製金鑰，模擬 DecryptKey 解密產出
	copiedPrivKey, err := crypto.ToECDSA(crypto.FromECDSA(m.key.PrivateKey))
	if err != nil {
		return nil, err
	}
	return &keystore.Key{
		Id:         m.key.Id,
		Address:    m.key.Address,
		PrivateKey: copiedPrivKey,
	}, nil
}

func TestKeystoreSigner_CompleteLifecycle(t *testing.T) {
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("生成私鑰失敗: %v", err)
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey)

	mockKey := &keystore.Key{
		Address:    addr,
		PrivateKey: privKey,
	}
	decrypter := &mockKeystoreDecrypter{
		addrHex:  addr.Hex(),
		key:      mockKey,
		password: "correct-password-123",
	}

	// 1. 建構與 Address 測試
	ksSigner, err := NewKeystoreSigner(decrypter, "correct-password-123")
	if err != nil {
		t.Fatalf("NewKeystoreSigner 失敗: %v", err)
	}
	if ksSigner.Address() != addr {
		t.Fatalf("KeystoreSigner Address 不符")
	}

	// 2. 正常簽署哈希與即時抹除驗證
	testHash := crypto.Keccak256Hash([]byte("keystore signer test"))
	sig, err := ksSigner.SignHash(testHash)
	if err != nil {
		t.Fatalf("KeystoreSigner SignHash 失敗: %v", err)
	}
	if len(sig) != 65 {
		t.Fatalf("簽章長度必須為 65 位元組")
	}
	valid, err := VerifySignature(testHash, sig, addr)
	if err != nil || !valid {
		t.Fatalf("簽章還原驗證失敗")
	}

	// 3. 測試錯誤密碼解密失敗
	ksWrongPass, err := NewKeystoreSigner(decrypter, "wrong-password")
	if err != nil {
		t.Fatalf("建立錯誤密碼 signer 失敗: %v", err)
	}
	_, err = ksWrongPass.SignHash(testHash)
	if err == nil {
		t.Fatalf("錯誤密碼簽署應報錯")
	}

	// 4. 測試 SignUserOp
	op := &UserOperation{
		Sender:               addr,
		Nonce:                big.NewInt(10),
		CallGasLimit:         big.NewInt(80000),
		VerificationGasLimit: big.NewInt(120000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(3000000000),
		MaxPriorityFeePerGas: big.NewInt(1500000000),
	}
	userOpSig, err := ksSigner.SignUserOp(op, EntryPointV06, big.NewInt(1))
	if err != nil {
		t.Fatalf("SignUserOp 失敗: %v", err)
	}
	userOpHash, _ := GetUserOpHash(op, EntryPointV06, big.NewInt(1))
	validUserOp, err := VerifySignature(userOpHash, userOpSig, addr)
	if err != nil || !validUserOp {
		t.Fatalf("UserOp 簽章驗證失敗")
	}

	// 5. 測試 SignUserOpWithEthPrefix
	ethPrefixSig, err := ksSigner.SignUserOpWithEthPrefix(op, EntryPointV06, big.NewInt(1))
	if err != nil {
		t.Fatalf("SignUserOpWithEthPrefix 失敗: %v", err)
	}
	ethHash := EthSignedMessageHash(userOpHash)
	validEthSig, err := VerifySignature(ethHash, ethPrefixSig, addr)
	if err != nil || !validEthSig {
		t.Fatalf("前綴簽章驗證失敗")
	}

	// 6. 建構異常防禦測試
	_, err = NewKeystoreSigner(nil, "pass")
	if err != ErrNilDecrypter {
		t.Fatalf("nil decrypter 應回傳 ErrNilDecrypter，得到 %v", err)
	}

	brokenDecrypter := &mockKeystoreDecrypter{
		addrHex:  "",
		key:      mockKey,
		password: "pass",
	}
	_, err = NewKeystoreSigner(brokenDecrypter, "pass")
	if err == nil {
		t.Fatalf("無效地址解密器應報錯")
	}
}

func TestKeystoreSigner_RejectsMismatchedKey(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	decrypter := newSpyKeystoreDecrypter(key, "test-password")
	decrypter.addrHex = "0x1111111111111111111111111111111111111111"
	signer, err := NewKeystoreSigner(decrypter, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signer.SignHash(crypto.Keccak256Hash([]byte("mismatched key"))); err == nil {
		t.Fatal("私鑰地址不符應拒絕簽署")
	}
	keys := decrypter.getInterceptedKeys()
	if len(keys) != 1 || keys[0].PrivateKey.D.Sign() != 0 {
		t.Fatal("拒絕簽署後仍應抹除解密的私鑰")
	}
}

func TestSigner_EthSignedMessagePrefix(t *testing.T) {
	// 驗證以太坊前綴計算符合官方 \x19Ethereum Signed Message:\n32
	rawHash := common.HexToHash("0x1234567890123456789012345678901234567890123456789012345678901234")
	prefixExpected := crypto.Keccak256Hash([]byte("\x19Ethereum Signed Message:\n32"), rawHash.Bytes())

	actualHash := EthSignedMessageHash(rawHash)
	if actualHash != prefixExpected {
		t.Fatalf("EthSignedMessageHash 計算結果不符")
	}

	privKey, _ := crypto.GenerateKey()
	signer, _ := NewPrivateKeySigner(privKey)

	op := &UserOperation{
		Sender:               signer.Address(),
		Nonce:                big.NewInt(0),
		CallGasLimit:         big.NewInt(50000),
		VerificationGasLimit: big.NewInt(60000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(1000000000),
		MaxPriorityFeePerGas: big.NewInt(1000000000),
	}

	sig, err := signer.SignUserOpWithEthPrefix(op, EntryPointV06, big.NewInt(1))
	if err != nil {
		t.Fatalf("SignUserOpWithEthPrefix 失敗: %v", err)
	}

	opHash, _ := GetUserOpHash(op, EntryPointV06, big.NewInt(1))
	ethOpHash := EthSignedMessageHash(opHash)
	valid, err := VerifySignature(ethOpHash, sig, signer.Address())
	if err != nil || !valid {
		t.Fatalf("前綴簽章還原驗證失敗")
	}
}

func TestSigner_VerifySignature_Defenses(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	signer, _ := NewPrivateKeySigner(privKey)
	hash := crypto.Keccak256Hash([]byte("low-s defense test"))

	sig, err := signer.SignHash(hash)
	if err != nil {
		t.Fatalf("簽署失敗: %v", err)
	}

	// 1. 正常簽章驗證通過
	valid, err := VerifySignature(hash, sig, signer.Address())
	if err != nil || !valid {
		t.Fatalf("正常簽章驗證應通過")
	}

	// 2. 測試 Low-S 防護 (EIP-2)
	// 構造一個延展性簽章：S' = N - S
	sBytes := sig[32:64]
	sVal := new(big.Int).SetBytes(sBytes)
	n := crypto.S256().Params().N
	malleableS := new(big.Int).Sub(n, sVal)

	malleableSig := make([]byte, 65)
	copy(malleableSig, sig)
	copy(malleableSig[32:64], common.LeftPadBytes(malleableS.Bytes(), 32))
	// 翻轉 V 值
	if malleableSig[64] == 27 {
		malleableSig[64] = 28
	} else {
		malleableSig[64] = 27
	}

	validMalleable, err := VerifySignature(hash, malleableSig, signer.Address())
	if validMalleable || err != ErrNonLowSSignature {
		t.Fatalf("延展性 High-S 簽章應被拒絕並回傳 ErrNonLowSSignature，得到 valid=%v, err=%v", validMalleable, err)
	}

	// 3. 非法 V 數值測試（嚴格拒絕非 27/28 的數值，包含裸 recovery id 0 與 1）
	invalidVSig := make([]byte, 65)
	copy(invalidVSig, sig)
	for _, badV := range []byte{0, 1, 2, 26, 29, 255} {
		invalidVSig[64] = badV
		validV, err := VerifySignature(hash, invalidVSig, signer.Address())
		if validV || err == nil {
			t.Fatalf("非法 V 數值 %d 應報錯，得到 valid=%v, err=%v", badV, validV, err)
		}
	}

	// 4. 翻轉合法 V (例如 27 變 28)，還原之地址應不符預期
	flippedVSig := make([]byte, 65)
	copy(flippedVSig, sig)
	if flippedVSig[64] == 27 {
		flippedVSig[64] = 28
	} else {
		flippedVSig[64] = 27
	}
	validFlipped, err := VerifySignature(hash, flippedVSig, signer.Address())
	if validFlipped {
		t.Fatalf("翻轉 V 值後不應驗證通過")
	}

	// 4. 簽章長度無效測試
	_, err = VerifySignature(hash, sig[:64], signer.Address())
	if err != ErrInvalidSigLen {
		t.Fatalf("非 65 位元組簽章應報錯 ErrInvalidSigLen")
	}
}

func TestSigner_NilDefenses(t *testing.T) {
	_, err := NewPrivateKeySigner(nil)
	if err == nil {
		t.Fatalf("nil 私鑰應回傳錯誤")
	}

	var emptyKey ecdsa.PrivateKey
	_, err = NewPrivateKeySigner(&emptyKey)
	if err == nil {
		t.Fatalf("未初始化私鑰應回傳錯誤")
	}

	testHash := common.HexToHash("0x1234")
	_, err = VerifySignature(testHash, []byte{1, 2, 3}, common.Address{})
	if err == nil {
		t.Fatalf("非法簽章長度應報錯")
	}

	// 測試通用前綴簽署 nil signer
	_, err = SignUserOpWithEthPrefix(nil, &UserOperation{}, common.Address{}, big.NewInt(1))
	if err == nil {
		t.Fatalf("nil signer 應報錯")
	}
}

func TestSigner_SignUserOp(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	signer, _ := NewPrivateKeySigner(privKey)

	op := &UserOperation{
		Sender:               signer.Address(),
		Nonce:                big.NewInt(0),
		CallGasLimit:         big.NewInt(100000),
		VerificationGasLimit: big.NewInt(150000),
		PreVerificationGas:   big.NewInt(21000),
		MaxFeePerGas:         big.NewInt(2000000000),
		MaxPriorityFeePerGas: big.NewInt(1000000000),
	}

	sig, err := signer.SignUserOp(op, EntryPointV06, big.NewInt(11155111))
	if err != nil {
		t.Fatalf("SignUserOp 失敗: %v", err)
	}

	hash, err := GetUserOpHash(op, EntryPointV06, big.NewInt(11155111))
	if err != nil {
		t.Fatalf("計算 hash 失敗: %v", err)
	}

	valid, err := VerifySignature(hash, sig, signer.Address())
	if err != nil || !valid {
		t.Fatalf("UserOp 簽章還原驗證失敗")
	}
}
