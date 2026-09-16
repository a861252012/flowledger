package wallet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCrosschainStorageBoundaries(t *testing.T) {
	const solanaJSON = `{"signature":"1111111111111111111111111111111111111111111111111111111111111111","quoteId":"legacy-id","to":"11111111111111111111111111111111","amount":"0.1","state":"expired_unconfirmed","finalized":false,"lastValidBlockHeight":200,"createdAt":"2026-09-16T01:02:03Z","signedRaw":"opaque signed bytes"}`
	const tronJSON = `{"signature":"3a894f8f31a6db67d75cca83ee0aa1a31aba1044d0a786042797879d685a0963","quoteId":"legacy-id","from":"TUEZSdKsoDHQMeZwihtdoBiN46zxhGWYdH","to":"TUEZSdKsoDHQMeZwihtdoBiN46zxhGWYdH","symbol":"TRX","amount":"1","state":"expired_unconfirmed","finalized":false,"expiryCheckedBlock":"evidence","expiryCheckedAt":100,"createdAt":"2026-09-16T01:02:03Z","expiresAt":"2026-09-16T01:03:03Z","raw":"00ab","signed":"00abcd"}`
	// Conversion preserves bytes and optional fields. Service reopen tests separately
	// authenticate the signed transaction; these fixtures exercise the storage shape.
	t.Run("solana", func(t *testing.T) {
		var stored solanaDiskRecord
		if err := json.Unmarshal([]byte(solanaJSON), &stored); err != nil {
			t.Fatal(err)
		}
		record, err := solanaRecordFromDisk(stored)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(solanaRecordToDisk(record))
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != solanaJSON {
			t.Fatal("Solana storage contract changed")
		}
		public, err := json.Marshal(solanaRecordResponse(record))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(public), "signedRaw") || strings.Contains(string(public), "opaque") {
			t.Fatal("signed bytes exposed")
		}
		stored.State = "invented"
		if _, err := solanaRecordFromDisk(stored); err == nil {
			t.Fatal("unknown state accepted")
		}
		stored.State = "submitted"
		stored.Signature = "invalid"
		if _, err := solanaRecordFromDisk(stored); err == nil {
			t.Fatal("invalid signature accepted")
		}
	})
	t.Run("tron", func(t *testing.T) {
		var stored tronDiskRecord
		if err := json.Unmarshal([]byte(tronJSON), &stored); err != nil {
			t.Fatal(err)
		}
		record, err := tronRecordFromDisk(stored)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(tronRecordToDisk(record))
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != tronJSON {
			t.Fatal("TRON storage contract changed")
		}
		public, err := json.Marshal(tronRecordResponse(record))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(public), `"signed"`) || strings.Contains(string(public), `"raw"`) {
			t.Fatal("signed bytes exposed")
		}
		stored.To = "invalid"
		if _, err := tronRecordFromDisk(stored); err == nil {
			t.Fatal("invalid recipient accepted")
		}
		stored.To = string(record.To)
		stored.State = "invented"
		if _, err := tronRecordFromDisk(stored); err == nil {
			t.Fatal("unknown state accepted")
		}
	})
}
