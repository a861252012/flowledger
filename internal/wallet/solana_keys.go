package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	sol "github.com/gagliardetto/solana-go"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/scrypt"
)

type solanaKeyFile struct {
	Version    int    `json:"version"`
	Address    string `json:"address"`
	N          int    `json:"scryptN"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// SLIP-0010 hardened Ed25519 derivation; no secp256k1/EVM key is reused.
func deriveEd25519(seed []byte, path []uint32) []byte {
	mac := hmac.New(sha512.New, []byte("ed25519 seed"))
	mac.Write(seed)
	material := mac.Sum(nil)
	for _, index := range path {
		data := make([]byte, 37)
		copy(data[1:33], material[:32])
		binary.BigEndian.PutUint32(data[33:], index|0x80000000)
		mac = hmac.New(sha512.New, material[32:])
		mac.Write(data)
		wipeBytes(data)
		wipeBytes(material)
		material = mac.Sum(nil)
	}
	key := append([]byte(nil), material[:32]...)
	wipeBytes(material)
	return key
}

func (s *SolanaService) keyFile() (*solanaKeyFile, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, "key.json"))
	if err != nil {
		return nil, err
	}
	return parseSolanaKey(data)
}

func parseSolanaKey(data []byte) (*solanaKeyFile, error) {
	var key solanaKeyFile
	if len(data) > 8192 || json.Unmarshal(data, &key) != nil || key.Version != 1 || key.N < 2 || key.N > 262144 || key.N&(key.N-1) != 0 {
		return nil, errors.New("Solana 備份格式無效")
	}
	if _, err := sol.PublicKeyFromBase58(key.Address); err != nil {
		return nil, errors.New("Solana 備份地址無效")
	}
	return &key, nil
}

func (s *SolanaService) encryptSeed(seed []byte, password string) (*solanaKeyFile, error) {
	salt, nonce := make([]byte, 32), make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(password), salt, s.n, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	defer wipeBytes(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	private := ed25519.NewKeyFromSeed(seed)
	defer wipeBytes(private)
	address := sol.PrivateKey(private).PublicKey().String()
	encrypted := gcm.Seal(nil, nonce, seed, []byte("flowledger-solana-v1:"+address))
	return &solanaKeyFile{1, address, s.n, hex.EncodeToString(salt), hex.EncodeToString(nonce), hex.EncodeToString(encrypted)}, nil
}

func (s *SolanaService) decryptSeed(password string) ([]byte, error) {
	data, err := s.keyFile()
	if err != nil {
		return nil, err
	}
	return decryptSolanaKey(data, password)
}

func decryptSolanaKey(data *solanaKeyFile, password string) ([]byte, error) {
	salt, e1 := hex.DecodeString(data.Salt)
	nonce, e2 := hex.DecodeString(data.Nonce)
	encrypted, e3 := hex.DecodeString(data.Ciphertext)
	if e1 != nil || e2 != nil || e3 != nil || len(salt) != 32 || len(nonce) != 12 || len(encrypted) != 48 {
		return nil, errors.New("Solana 備份資料損壞")
	}
	key, err := scrypt.Key([]byte(password), salt, data.N, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	defer wipeBytes(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	seed, err := gcm.Open(nil, nonce, encrypted, []byte("flowledger-solana-v1:"+data.Address))
	if err != nil {
		return nil, ErrPasswordMismatch
	}
	private := ed25519.NewKeyFromSeed(seed)
	defer wipeBytes(private)
	if sol.PrivateKey(private).PublicKey().String() != data.Address {
		wipeBytes(seed)
		return nil, errors.New("Solana 備份地址不符")
	}
	return seed, nil
}

func (s *SolanaService) Create(mnemonic, password string) (*SolanaCreateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(s.dir, "key.json")); !os.IsNotExist(err) {
		return nil, ErrWalletExists
	}
	generated := mnemonic == ""
	if generated {
		entropy, err := bip39.NewEntropy(128)
		if err != nil {
			return nil, err
		}
		mnemonic, err = bip39.NewMnemonic(entropy)
		wipeBytes(entropy)
		if err != nil {
			return nil, err
		}
	}
	mnemonic = strings.Join(strings.Fields(mnemonic), " ")
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, ErrInvalidMnemonic
	}
	seed := bip39.NewSeed(mnemonic, "")
	defer wipeBytes(seed)
	child := deriveEd25519(seed, []uint32{44, 501, 0, 0})
	defer wipeBytes(child)
	encrypted, err := s.encryptSeed(child, password)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(encrypted)
	if err != nil {
		return nil, err
	}
	if err = atomicWriteFile(filepath.Join(s.dir, "key.json"), data, 0600); err != nil {
		return nil, err
	}
	result := &SolanaCreateResponse{Address: encrypted.Address}
	if generated {
		result.Mnemonic = mnemonic
	}
	return result, nil
}

func (s *SolanaService) Backup(password string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seed, err := s.decryptSeed(password)
	if err != nil {
		return nil, err
	}
	wipeBytes(seed)
	return os.ReadFile(filepath.Join(s.dir, "key.json"))
}

func (s *SolanaService) RestoreBackup(raw json.RawMessage, password, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.dir, "key.json")); !os.IsNotExist(err) {
		return ErrWalletExists
	}
	file, err := parseSolanaKey(raw)
	if err != nil {
		return err
	}
	seed, err := decryptSolanaKey(file, password)
	if err != nil {
		return err
	}
	defer wipeBytes(seed)
	encrypted, err := s.encryptSeed(seed, newPassword)
	if err != nil {
		return err
	}
	data, err := json.Marshal(encrypted)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(s.dir, "key.json"), data, 0600)
}
func (s *SolanaService) ChangePassword(password, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	seed, err := s.decryptSeed(password)
	if err != nil {
		return err
	}
	defer wipeBytes(seed)
	encrypted, err := s.encryptSeed(seed, newPassword)
	if err != nil {
		return err
	}
	data, err := json.Marshal(encrypted)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(s.dir, "key.json"), data, 0600)
}
