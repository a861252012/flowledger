package wallet

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

// KeystoreManager handles wallet generation, derivation, encryption, and local disk persistence.
type KeystoreManager struct {
	mu        sync.Mutex
	walletDir string
	scryptN   int
	scryptP   int
	scryptSem chan struct{}
}

func NewKeystoreManager(walletDir string, scryptN, scryptP int) *KeystoreManager {
	if scryptN <= 0 {
		scryptN = keystore.StandardScryptN
	}
	if scryptP <= 0 {
		scryptP = keystore.StandardScryptP
	}
	return &KeystoreManager{
		walletDir: walletDir,
		scryptN:   scryptN,
		scryptP:   scryptP,
		scryptSem: make(chan struct{}, 1), // Bounded concurrency: max 1 simultaneous scrypt operations
	}
}

func (km *KeystoreManager) keystorePath() string {
	return filepath.Join(km.walletDir, "keystore.json")
}

// ValidatePassword enforces:
// Password must be 12 to 128 characters, spaces allowed, never trimmed.
func ValidatePassword(password string) error {
	count := utf8.RuneCountInString(password)
	if count < 12 || count > 128 {
		return ErrInvalidPassword
	}
	return nil
}

// DeriveKey derives an Ethereum address and private key using BIP39 mnemonic and BIP44 path m/44'/60'/0'/0/0.
// The BIP39 passphrase is fixed to empty string ("").
func DeriveKey(mnemonic string) (common.Address, *ecdsa.PrivateKey, error) {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(mnemonic)), " ")
	if !bip39.IsMnemonicValid(normalized) {
		return common.Address{}, nil, ErrInvalidMnemonic
	}

	// Empty optional BIP39 passphrase ONLY, explicitly named scope.
	seed := bip39.NewSeed(normalized, "")
	defer wipeBytes(seed)

	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		return common.Address{}, nil, err
	}

	defer wipeBytes(masterKey.Key)
	// BIP44 path: m/44'/60'/0'/0/0
	// 44' (purpose)
	purpose, err := masterKey.NewChildKey(bip32.FirstHardenedChild + 44)
	if err != nil {
		return common.Address{}, nil, err
	}
	defer wipeBytes(purpose.Key)
	// 60' (coin_type: Ethereum)
	coinType, err := purpose.NewChildKey(bip32.FirstHardenedChild + 60)
	if err != nil {
		return common.Address{}, nil, err
	}
	defer wipeBytes(coinType.Key)
	// 0' (account 0)
	account, err := coinType.NewChildKey(bip32.FirstHardenedChild + 0)
	if err != nil {
		return common.Address{}, nil, err
	}
	defer wipeBytes(account.Key)
	// 0 (external change)
	change, err := account.NewChildKey(0)
	if err != nil {
		return common.Address{}, nil, err
	}
	defer wipeBytes(change.Key)
	// 0 (address index 0)
	addressKey, err := change.NewChildKey(0)
	if err != nil {
		return common.Address{}, nil, err
	}
	defer wipeBytes(addressKey.Key)

	privKey, err := crypto.ToECDSA(addressKey.Key)
	if err != nil {
		return common.Address{}, nil, err
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey)
	return addr, privKey, nil
}

func (km *KeystoreManager) acquireScrypt() error {
	select {
	case km.scryptSem <- struct{}{}:
		return nil
	default:
		return ErrTooManyScryptRequests
	}
}

func (km *KeystoreManager) releaseScrypt() {
	<-km.scryptSem
}

// Exists returns true if the keystore file exists on disk.
func (km *KeystoreManager) Exists() bool {
	km.mu.Lock()
	defer km.mu.Unlock()
	_, err := os.Stat(km.keystorePath())
	return err == nil
}

// Address returns the checksummed Ethereum address of the stored wallet.
func (km *KeystoreManager) Address() (string, error) {
	km.mu.Lock()
	defer km.mu.Unlock()
	data, err := os.ReadFile(km.keystorePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrWalletNotFound
		}
		return "", err
	}
	var meta struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", errors.New("無法讀取既有金鑰檔")
	}
	if !strings.HasPrefix(meta.Address, "0x") {
		meta.Address = "0x" + meta.Address
	}
	addr, err := ValidateAddress(meta.Address)
	if err != nil {
		return "", errors.New("金鑰檔地址格式錯誤")
	}
	return addr.Hex(), nil
}

// Create generates a new BIP39 12-word mnemonic, derives m/44'/60'/0'/0/0,
// encrypts using StandardScrypt into keystore.json, and returns address and mnemonic.
// Never overwrites an existing wallet. Mnemonic is returned once and never persisted.
func (km *KeystoreManager) Create(password string) (*CreateResponse, error) {
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}

	km.mu.Lock()
	defer km.mu.Unlock()

	if _, err := os.Lstat(km.keystorePath()); !os.IsNotExist(err) {
		if err != nil {
			return nil, errors.New("無法安全讀取錢包儲存狀態")
		}
		return nil, ErrWalletExists
	}

	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return nil, err
	}
	defer wipeBytes(entropy)

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}

	addr, privKey, err := DeriveKey(mnemonic)
	if err != nil {
		return nil, err
	}
	defer wipePrivateKey(privKey)

	if err := km.acquireScrypt(); err != nil {
		return nil, err
	}
	defer km.releaseScrypt()

	key := &keystore.Key{
		Id:         uuid.New(),
		Address:    addr,
		PrivateKey: privKey,
	}
	keyJSON, err := keystore.EncryptKey(key, password, km.scryptN, km.scryptP)
	if err != nil {
		return nil, err
	}

	if err := km.atomicWriteFile(km.keystorePath(), keyJSON, 0600); err != nil {
		return nil, err
	}

	return &CreateResponse{
		Address:  addr.Hex(),
		Mnemonic: mnemonic,
		Path:     "m/44'/60'/0'/0/0",
	}, nil
}

// Import restores a wallet from an existing 12/15/18/21/24-word mnemonic.
// Never overwrites an existing wallet.
func (km *KeystoreManager) Import(mnemonic, password string) (*ImportResponse, error) {
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}

	words := strings.Fields(strings.TrimSpace(mnemonic))
	wCount := len(words)
	if wCount != 12 && wCount != 15 && wCount != 18 && wCount != 21 && wCount != 24 {
		return nil, ErrInvalidMnemonic
	}

	addr, privKey, err := DeriveKey(mnemonic)
	if err != nil {
		return nil, err
	}
	defer wipePrivateKey(privKey)

	km.mu.Lock()
	defer km.mu.Unlock()

	if _, err := os.Lstat(km.keystorePath()); !os.IsNotExist(err) {
		if err != nil {
			return nil, errors.New("無法安全讀取錢包儲存狀態")
		}
		return nil, ErrWalletExists
	}

	if err := km.acquireScrypt(); err != nil {
		return nil, err
	}
	defer km.releaseScrypt()

	key := &keystore.Key{
		Id:         uuid.New(),
		Address:    addr,
		PrivateKey: privKey,
	}
	keyJSON, err := keystore.EncryptKey(key, password, km.scryptN, km.scryptP)
	if err != nil {
		return nil, err
	}

	if err := km.atomicWriteFile(km.keystorePath(), keyJSON, 0600); err != nil {
		return nil, err
	}

	return &ImportResponse{
		Address: addr.Hex(),
		Path:    "m/44'/60'/0'/0/0",
	}, nil
}

// Backup verifies the password by decrypting, then returns the raw encrypted keystore JSON object.
func (km *KeystoreManager) Backup(password string) (json.RawMessage, error) {
	// Existing keystores may use passwords accepted by earlier creation rules.
	km.mu.Lock()
	data, err := os.ReadFile(km.keystorePath())
	km.mu.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrWalletNotFound
		}
		return nil, err
	}

	if err := km.acquireScrypt(); err != nil {
		return nil, err
	}
	defer km.releaseScrypt()

	key, err := keystore.DecryptKey(data, password)
	if err != nil {
		return nil, ErrPasswordMismatch
	}
	wipePrivateKey(key.PrivateKey)

	return json.RawMessage(data), nil
}

// DecryptKey decrypts the keystore file using the provided password.
// The caller is responsible for wiping the returned private key after use.
func (km *KeystoreManager) DecryptKey(password string) (*keystore.Key, error) {
	// Existing keystores may use passwords accepted by earlier creation rules.
	km.mu.Lock()
	data, err := os.ReadFile(km.keystorePath())
	km.mu.Unlock()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrWalletNotFound
		}
		return nil, err
	}

	if err := km.acquireScrypt(); err != nil {
		return nil, err
	}
	defer km.releaseScrypt()

	key, err := keystore.DecryptKey(data, password)
	if err != nil {
		return nil, ErrPasswordMismatch
	}
	return key, nil
}

// atomicWriteFile writes data to a temp file, syncs to disk, renames to dest, and syncs the parent directory.
func (km *KeystoreManager) atomicWriteFile(dest string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return err
	}

	// Sync parent directory to persist directory entry metadata
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func wipePrivateKey(k *ecdsa.PrivateKey) {
	if k == nil || k.D == nil {
		return
	}
	b := k.D.Bits()
	for i := range b {
		b[i] = 0
	}
}
