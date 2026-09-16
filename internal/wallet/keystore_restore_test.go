package wallet

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestKeystoreRestoreAndPasswordChange(t *testing.T) {
	original := NewKeystoreManager(t.TempDir(), 2, 1)
	created, err := original.Create("old password 123")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := original.Backup("old password 123")
	if err != nil {
		t.Fatal(err)
	}
	restored := NewKeystoreManager(t.TempDir(), 2, 1)
	if _, err := restored.ImportKeystore(backup, "wrong", "new password 123"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatal(err)
	}
	if restored.Exists() {
		t.Fatal("failed import wrote a wallet")
	}
	result, err := restored.ImportKeystore(backup, "old password 123", "new password 123")
	if err != nil || result.Address != created.Address {
		t.Fatalf("restore: %v %v", result, err)
	}
	if _, err := restored.ImportKeystore(backup, "old password 123", "new password 123"); !errors.Is(err, ErrWalletExists) {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(restored.keystorePath())
	if err := restored.ChangePassword("wrong", "changed password 123"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(restored.keystorePath())
	if string(before) != string(after) {
		t.Fatal("wrong password changed file")
	}
	if err := restored.ChangePassword("new password 123", "changed password 123"); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Backup("new password 123"); !errors.Is(err, ErrPasswordMismatch) {
		t.Fatal(err)
	}
	changed, err := restored.Backup("changed password 123")
	if err != nil {
		t.Fatal(err)
	}
	again := NewKeystoreManager(t.TempDir(), 2, 1)
	if result, err := again.ImportKeystore(changed, "changed password 123", "restored password 123"); err != nil || result.Address != created.Address {
		t.Fatalf("new backup restore: %v", err)
	}
}

func TestKeystoreImportRejectsMalformedCryptoWithoutPanic(t *testing.T) {
	original := NewKeystoreManager(t.TempDir(), 2, 1)
	if _, err := original.Create("test-password-123"); err != nil {
		t.Fatal(err)
	}
	backup, err := original.Backup("test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"uppercase-n", "missing-salt", "numeric-salt", "short-iv", "huge-n", "fraction-n", "short-key"} {
		t.Run(field, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal(backup, &data); err != nil {
				t.Fatal(err)
			}
			crypto := data["crypto"].(map[string]any)
			params := crypto["kdfparams"].(map[string]any)
			switch field {
			case "uppercase-n":
				params["N"] = params["n"]
				delete(params, "n")
			case "missing-salt":
				delete(params, "salt")
			case "numeric-salt":
				params["salt"] = 42
			case "short-iv":
				crypto["cipherparams"].(map[string]any)["iv"] = "00"
			case "huge-n":
				params["n"] = 1 << 30
			case "fraction-n":
				params["n"] = 2.5
			case "short-key":
				params["dklen"] = 16
			}
			encoded, _ := json.Marshal(data)
			target := NewKeystoreManager(t.TempDir(), 2, 1)
			if _, err := target.ImportKeystore(encoded, "test-password-123", "new-password-123"); err == nil {
				t.Fatal("accepted malformed keystore")
			}
			if target.Exists() {
				t.Fatal("invalid import wrote wallet")
			}
		})
	}
}
