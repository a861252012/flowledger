package erc4337

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"runtime"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrNilPrivateKey    = errors.New("erc4337: 私鑰指標不得為空")
	ErrInvalidSigLen    = errors.New("erc4337: 簽章長度無效，必須為 65 位元組")
	ErrNonLowSSignature = errors.New("erc4337: 簽章之 S 數值超出 Low-S 上限 (EIP-2)")
	ErrNilDecrypter     = errors.New("erc4337: decrypter 不得為空")
)

var (
	// secp256k1HalfN 為 secp256k1 曲線階數 N 的一半，用於 Low-S 簽名延展性防護 (EIP-2)
	secp256k1HalfN = new(big.Int).Rsh(crypto.S256().Params().N, 1)
)

// UserOpSigner 定義 ERC-4337 簽署者介面
type UserOpSigner interface {
	Address() common.Address
	SignHash(hash common.Hash) ([]byte, error)
	SignUserOp(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error)
}

// wipePrivateKey 清除 ECDSA 私鑰中的大數記憶體，並調用 runtime.KeepAlive 防範編譯器死碼消除
func wipePrivateKey(k *ecdsa.PrivateKey) {
	if k == nil || k.D == nil {
		return
	}
	bits := k.D.Bits()
	for i := range bits {
		bits[i] = 0
	}
	runtime.KeepAlive(bits)
	k.D.SetInt64(0)
}

// EthSignedMessageHash 計算 \x19Ethereum Signed Message:\n32 前綴之哈希
func EthSignedMessageHash(hash common.Hash) common.Hash {
	msgPrefix := []byte("\x19Ethereum Signed Message:\n32")
	return crypto.Keccak256Hash(msgPrefix, hash.Bytes())
}

// ToEthSignedMessageHash 為 EthSignedMessageHash 之別名
func ToEthSignedMessageHash(hash common.Hash) common.Hash {
	return EthSignedMessageHash(hash)
}

// SignUserOpWithEthPrefix 通用輔助函式：先計算 UserOpHash，再加上個人簽名前綴後進行簽署
func SignUserOpWithEthPrefix(signer UserOpSigner, userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	if signer == nil {
		return nil, errors.New("erc4337: signer 不得為空")
	}
	hash, err := GetUserOpHash(userOp, entryPoint, chainID)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 計算 userOpHash 失敗: %w", err)
	}
	return signer.SignHash(EthSignedMessageHash(hash))
}

// PrivateKeySigner 以 ECDSA 私鑰實作 UserOpSigner
type PrivateKeySigner struct {
	privKey *ecdsa.PrivateKey
	address common.Address
}

// NewPrivateKeySigner 建立以私鑰驅動之簽署者
func NewPrivateKeySigner(privKey *ecdsa.PrivateKey) (*PrivateKeySigner, error) {
	if privKey == nil || privKey.D == nil {
		return nil, ErrNilPrivateKey
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey)
	return &PrivateKeySigner{
		privKey: privKey,
		address: addr,
	}, nil
}

// Address 回傳簽署者之以太坊地址
func (s *PrivateKeySigner) Address() common.Address {
	return s.address
}

// SignHash 對 32 位元組哈希進行 ECDSA 簽署，產出 65 位元組 [R || S || V]（V 標準化為 27 或 28）
func (s *PrivateKeySigner) SignHash(hash common.Hash) ([]byte, error) {
	if s.privKey == nil || s.privKey.D == nil {
		return nil, ErrNilPrivateKey
	}
	sig, err := crypto.Sign(hash.Bytes(), s.privKey)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 簽署哈希失敗: %w", err)
	}
	if len(sig) != 65 {
		return nil, ErrInvalidSigLen
	}
	// crypto.Sign 回傳的 V 為 0 或 1，以太坊合約 ecrecover 需要 27 或 28
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}

// SignUserOp 計算 UserOpHash 並完成簽署
func (s *PrivateKeySigner) SignUserOp(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	hash, err := GetUserOpHash(userOp, entryPoint, chainID)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 計算 userOpHash 失敗: %w", err)
	}
	return s.SignHash(hash)
}

// SignUserOpWithEthPrefix 支援以以太坊簽署訊息前綴簽署 UserOperation
func (s *PrivateKeySigner) SignUserOpWithEthPrefix(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	return SignUserOpWithEthPrefix(s, userOp, entryPoint, chainID)
}

// Wipe 抹除私鑰記憶體資料
func (s *PrivateKeySigner) Wipe() {
	if s.privKey != nil {
		wipePrivateKey(s.privKey)
		s.privKey = nil
	}
}

// EphemeralKeyProvider 定義取得臨時私鑰之回呼函式
type EphemeralKeyProvider func() (*ecdsa.PrivateKey, error)

// EphemeralSigner 支援動態解密私鑰、完成簽署後立即抹除私鑰之簽署者
type EphemeralSigner struct {
	address  common.Address
	provider EphemeralKeyProvider
}

// NewEphemeralSigner 建立臨時簽署者
func NewEphemeralSigner(address common.Address, provider EphemeralKeyProvider) *EphemeralSigner {
	return &EphemeralSigner{
		address:  address,
		provider: provider,
	}
}

// Address 回傳簽署者之以太坊地址
func (s *EphemeralSigner) Address() common.Address {
	return s.address
}

// SignHash 透過 provider 取得私鑰簽署，簽署完畢後立即執行記憶體抹除
func (s *EphemeralSigner) SignHash(hash common.Hash) ([]byte, error) {
	if s.provider == nil {
		return nil, errors.New("erc4337: provider 不得為空")
	}
	privKey, err := s.provider()
	if err != nil {
		return nil, fmt.Errorf("erc4337: 取得簽署私鑰失敗: %w", err)
	}
	if privKey == nil || privKey.D == nil {
		return nil, ErrNilPrivateKey
	}
	defer func() {
		wipePrivateKey(privKey)
	}()

	derivedAddr := crypto.PubkeyToAddress(privKey.PublicKey)
	if derivedAddr != s.address {
		return nil, fmt.Errorf("erc4337: 私鑰地址 (%s) 與預期地址 (%s) 不符", derivedAddr.Hex(), s.address.Hex())
	}

	sig, err := crypto.Sign(hash.Bytes(), privKey)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 簽署失敗: %w", err)
	}
	if len(sig) != 65 {
		return nil, ErrInvalidSigLen
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}

// SignUserOp 計算 UserOpHash 並完成簽署
func (s *EphemeralSigner) SignUserOp(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	hash, err := GetUserOpHash(userOp, entryPoint, chainID)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 計算 userOpHash 失敗: %w", err)
	}
	return s.SignHash(hash)
}

// SignUserOpWithEthPrefix 支援以以太坊簽署訊息前綴簽署 UserOperation
func (s *EphemeralSigner) SignUserOpWithEthPrefix(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	return SignUserOpWithEthPrefix(s, userOp, entryPoint, chainID)
}

// KeystoreDecrypter 定義 Keystore 抽象解密介面，以依賴倒置避免與 internal/wallet 產生循環依賴
type KeystoreDecrypter interface {
	Address() (string, error)
	DecryptKey(password string) (*keystore.Key, error)
}

// KeystoreSigner 整合 Keystore 並確保簽署時私鑰即用即抹
type KeystoreSigner struct {
	decrypter KeystoreDecrypter
	password  string
	address   common.Address
}

// NewKeystoreSigner 建立以 Keystore 為基礎之 UserOp 簽署者
func NewKeystoreSigner(decrypter KeystoreDecrypter, password string) (*KeystoreSigner, error) {
	if decrypter == nil {
		return nil, ErrNilDecrypter
	}
	addrHex, err := decrypter.Address()
	if err != nil {
		return nil, fmt.Errorf("erc4337: 讀取 keystore 地址失敗: %w", err)
	}
	return &KeystoreSigner{
		decrypter: decrypter,
		password:  password,
		address:   common.HexToAddress(addrHex),
	}, nil
}

// Address 回傳簽署者之以太坊地址
func (s *KeystoreSigner) Address() common.Address {
	return s.address
}

// SignHash 解密 Keystore 金鑰進行簽署，並於 defer 中立即抹除私鑰記憶體
func (s *KeystoreSigner) SignHash(hash common.Hash) ([]byte, error) {
	key, err := s.decrypter.DecryptKey(s.password)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 解密 keystore 失敗: %w", err)
	}
	if key == nil || key.PrivateKey == nil || key.PrivateKey.D == nil {
		return nil, ErrNilPrivateKey
	}
	defer func() {
		wipePrivateKey(key.PrivateKey)
	}()

	derivedAddr := crypto.PubkeyToAddress(key.PrivateKey.PublicKey)
	if derivedAddr != s.address {
		return nil, fmt.Errorf("erc4337: 私鑰地址 (%s) 與預期地址 (%s) 不符", derivedAddr.Hex(), s.address.Hex())
	}

	sig, err := crypto.Sign(hash.Bytes(), key.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 簽署失敗: %w", err)
	}
	if len(sig) != 65 {
		return nil, ErrInvalidSigLen
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}

// SignUserOp 計算 UserOpHash 並完成簽署
func (s *KeystoreSigner) SignUserOp(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	hash, err := GetUserOpHash(userOp, entryPoint, chainID)
	if err != nil {
		return nil, fmt.Errorf("erc4337: 計算 userOpHash 失敗: %w", err)
	}
	return s.SignHash(hash)
}

// SignUserOpWithEthPrefix 支援以以太坊簽署訊息前綴簽署 UserOperation
func (s *KeystoreSigner) SignUserOpWithEthPrefix(userOp *UserOperation, entryPoint common.Address, chainID *big.Int) ([]byte, error) {
	return SignUserOpWithEthPrefix(s, userOp, entryPoint, chainID)
}

// VerifySignature 驗證給定簽章是否由指定以太坊地址針對 hash 所簽發（包含 Low-S 防護）
func VerifySignature(hash common.Hash, signature []byte, expectedAddress common.Address) (bool, error) {
	if len(signature) != 65 {
		return false, ErrInvalidSigLen
	}

	// 檢查 Low-S (EIP-2)
	sVal := new(big.Int).SetBytes(signature[32:64])
	if sVal.Cmp(secp256k1HalfN) > 0 {
		return false, ErrNonLowSSignature
	}

	sigCopy := make([]byte, 65)
	copy(sigCopy, signature)

	// 本地驗證器依循以太坊傳統 ecrecover 慣例，將 ECDSA 簽章之 V 數值限定為 27 或 28
	if sigCopy[64] != 27 && sigCopy[64] != 28 {
		return false, errors.New("erc4337: 簽章之 V 數值無效，必須為 27 或 28")
	}
	sigCopy[64] -= 27

	pubKey, err := crypto.SigToPub(hash.Bytes(), sigCopy)
	if err != nil {
		return false, fmt.Errorf("erc4337: 無法自簽章還原公鑰: %w", err)
	}

	recoveredAddr := crypto.PubkeyToAddress(*pubKey)
	return recoveredAddr == expectedAddress, nil
}
