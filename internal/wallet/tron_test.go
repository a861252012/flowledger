package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Captured from Shasta /wallet/createtransaction, an unsigned construction only.
func TestTronOfficialUnsignedVector(t *testing.T) {
	owner, _ := hex.DecodeString("41f16412b9a17ee9408646e2a21e16478f72ed1e95")
	to, _ := hex.DecodeString("41e552f6487585c2b58bc2c9bb4492bc1f17132cd0")
	raw, err := tronRaw(owner, to, big.NewInt(1), nil, "000000000000b1123e705e3c5900778f"+strings.Repeat("0", 32), 1789457971808, 1789458030000, 0)
	want := "0a02b11222083e705e3c5900778f40b0bb8aa08a345a65080112610a2d747970652e676f6f676c65617069732e636f6d2f70726f746f636f6c2e5472616e73666572436f6e747261637412300a1541f16412b9a17ee9408646e2a21e16478f72ed1e95121541e552f6487585c2b58bc2c9bb4492bc1f17132cd0180170e0f486a08a34"
	hash := sha256.Sum256(raw)
	if err != nil || hex.EncodeToString(raw) != want || hex.EncodeToString(hash[:]) != "3a894f8f31a6db67d75cca83ee0aa1a31aba1044d0a786042797879d685a0963" {
		t.Fatalf("official wire mismatch: %x %v", raw, err)
	}
}
func TestTronAddressAndDerivation(t *testing.T) {
	addr, key, err := deriveCoinKey("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", 195)
	if err != nil {
		t.Fatal(err)
	}
	defer wipePrivateKey(key)
	if got := TronAddress(addr); got != "TUEZSdKsoDHQMeZwihtdoBiN46zxhGWYdH" {
		t.Fatalf("BIP44 TRON address %s", got)
	}
	raw, err := ParseTronAddress(TronAddress(addr))
	if err != nil || common.BytesToAddress(raw[1:]) != addr {
		t.Fatal("address roundtrip")
	}
	for _, bad := range []string{"", addr.Hex(), "TUEZSdKsoDHQMeZwihtdoBiN46zxhGWYdX", TronAddress(common.Address{})} {
		if _, err := ParseTronAddress(bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

type tronFixture struct {
	service                                            *TronService
	address, contract, recipient                       string
	solidTime                                          int64
	solidError                                         bool
	wrongNetwork, broadcastFail, finalized, tokenFalse bool
	broadcasts                                         int
	tokenDecimals                                      uint8
	signed                                             string
}

func newTronFixture(t *testing.T) *tronFixture {
	t.Helper()
	f := &tronFixture{contract: TronAddress(common.HexToAddress("0x2222222222222222222222222222222222222222")), tokenDecimals: 6}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		var result any
		switch r.URL.Path {
		case "/wallet/getblockbynum":
			id := TronShastaGenesis
			if f.wrongNetwork {
				id = strings.Repeat("0", 64)
			}
			result = map[string]any{"blockID": id}
		case "/wallet/getaccount":
			result = map[string]any{"address": body["address"], "balance": 1000000000}
		case "/wallet/getaccountresource":
			result = map[string]any{"freeNetLimit": 600, "freeNetUsed": 100, "EnergyLimit": 20000, "EnergyUsed": 2}
		case "/wallet/getchainparameters":
			result = map[string]any{"chainParameter": []any{map[string]any{"key": "getTransactionFee", "value": 1000}, map[string]any{"key": "getEnergyFee", "value": 100}, map[string]any{"key": "getCreateNewAccountFeeInSystemContract", "value": 1000000}, map[string]any{"key": "getCreateAccountFee", "value": 100000}}}
		case "/walletsolidity/getnowblock":
			if f.solidError {
				w.WriteHeader(503)
				return
			}
			stamp := f.solidTime
			if stamp == 0 {
				stamp = time.Now().Add(-time.Minute).UnixMilli()
			}
			result = map[string]any{"blockID": strings.Repeat("2", 64), "block_header": map[string]any{"raw_data": map[string]any{"timestamp": stamp}}}
		case "/wallet/getnowblock":
			result = map[string]any{"blockID": strings.Repeat("1", 64), "block_header": map[string]any{"raw_data": map[string]any{"timestamp": time.Now().UnixMilli()}}}
		case "/wallet/triggerconstantcontract":
			method := strings.Split(body["function_selector"].(string), "(")[0]
			var value any
			switch method {
			case "symbol":
				value = "TEST"
			case "decimals":
				value = f.tokenDecimals
			case "balanceOf":
				value = big.NewInt(1000000)
			case "transfer":
				value = !f.tokenFalse
			default:
				t.Errorf("unexpected method %s", method)
			}
			raw, err := erc20ABI.Methods[method].Outputs.Pack(value)
			if err != nil {
				t.Error(err)
			}
			result = map[string]any{"result": map[string]bool{"result": true}, "constant_result": []string{hex.EncodeToString(raw)}, "energy_used": 15000}
		case "/wallet/broadcasthex":
			f.broadcasts += 1
			f.signed = body["transaction"].(string)
			// Broadcasting must never precede durable storage.
			data, err := os.ReadFile(filepath.Join(f.service.dir, "transactions.json"))
			if err != nil || !strings.Contains(string(data), f.signed) {
				t.Error("broadcast before persistence")
			}
			if f.broadcastFail {
				w.WriteHeader(504)
				return
			}
			result = map[string]any{"result": true}
		case "/walletsolidity/gettransactioninfobyid":
			result = map[string]any{}
			if f.finalized {
				result = map[string]any{"id": body["value"], "blockNumber": 100, "fee": 1000, "receipt": map[string]string{"result": "SUCCESS"}, "contractResult": []string{strings.Repeat("0", 63) + map[bool]string{true: "0", false: "1"}[f.tokenFalse]}}
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(server.Close)
	service, err := NewTronService(server.URL, "", t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	t.Cleanup(func() { service.Close() })
	created, err := service.Create("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	f.address = created.Address
	f.recipient = TronAddress(common.HexToAddress("0x3333333333333333333333333333333333333333"))
	return f
}
func TestTronSignPersistRetryFinalize(t *testing.T) {
	f := newTronFixture(t)
	s := f.service
	ctx := context.Background()
	balance, err := s.Balance(ctx, f.address)
	if err != nil || balance.TRX != "1000" || balance.Bandwidth != 500 {
		t.Fatalf("balance %v %v", balance, err)
	}
	q, err := s.Quote(ctx, f.recipient, "0.000001", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(ctx, q.ID, "wrong"); !errors.Is(err, ErrPasswordMismatch) || f.broadcasts != 0 {
		t.Fatal("wrong password broadcast")
	}
	f.broadcastFail = true
	r, err := s.Send(ctx, q.ID, "fixture-password")
	if err != nil || r.State != "broadcast_unknown" {
		t.Fatalf("send %v %v", r, err)
	}
	signed := f.signed
	if _, err := s.Send(ctx, q.ID, "wrong"); err != nil || f.broadcasts != 1 {
		t.Fatal("idempotency failed")
	}
	if _, err := s.Quote(ctx, f.recipient, "1", "", ""); !errors.Is(err, ErrTxInFlight) {
		t.Fatal("pending protection")
	}
	f.broadcastFail = false
	r, err = s.Retry(ctx, r.Signature)
	if err != nil || r.State != "submitted" || signed != f.signed {
		t.Fatal("retry changed signed transaction")
	}
	f.finalized = true
	history, err := s.History(ctx)
	if err != nil || len(history) != 1 || !history[0].Finalized || history[0].FeeTRX != "0.001" {
		t.Fatalf("history %v %v", history, err)
	}
	if _, err := s.Retry(ctx, r.Signature); err != nil || f.broadcasts != 2 {
		t.Fatal("finalized rebroadcast")
	}
	// Restart validates signatures and preserves records, without touching a runtime wallet volume.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewTronService(s.endpoint, "", s.dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.records) != 1 || !reopened.records[0].Finalized {
		t.Fatal("restart lost journal")
	}
}
func TestTronTRC20AndGuards(t *testing.T) {
	f := newTronFixture(t)
	s := f.service
	ctx := context.Background()
	q, err := s.Quote(ctx, f.recipient, "", f.contract, "100000")
	if err != nil || q.Symbol != "TEST" || q.FeeLimitTRX != "3" {
		t.Fatalf("token quote %v %v", q, err)
	}
	to, _ := ParseTronAddress(f.recipient)
	data, _ := erc20ABI.Pack("transfer", common.BytesToAddress(to[1:]), big.NewInt(100000))
	if !strings.Contains(hex.EncodeToString(q.raw), hex.EncodeToString(data)) {
		t.Fatal("wrong token recipient/amount")
	}
	f.wrongNetwork = true
	if _, err := s.Send(ctx, q.ID, "fixture-password"); err == nil || f.broadcasts != 0 {
		t.Fatal("wrong network signed")
	}
	f.wrongNetwork = false
	f.tokenFalse = true
	if _, err := s.Quote(ctx, f.recipient, "", f.contract, "100000"); err == nil {
		t.Fatal("accepted false transfer simulation")
	}
	f.tokenFalse = false
	for _, amount := range []string{"0", "-1", "1e3", "0.0000001", "999999999999999999999"} {
		if _, err := s.Quote(ctx, f.recipient, amount, "", ""); err == nil {
			t.Fatalf("accepted TRX %s", amount)
		}
	}
	q.Expires = time.Now().Add(-time.Second)
	if _, err := s.Send(ctx, q.ID, "fixture-password"); !errors.Is(err, ErrQuoteExpired) {
		t.Fatal("expired quote accepted")
	}
	q, err = s.Quote(ctx, f.recipient, "", f.contract, "100000")
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Send(ctx, q.ID, "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := hex.DecodeString(f.signed)
	hash, _ := hex.DecodeString(r.Signature)
	pub, err := crypto.SigToPub(hash, raw[len(raw)-65:])
	if err != nil || TronAddress(crypto.PubkeyToAddress(*pub)) != f.address {
		t.Fatal("invalid signer")
	}
	s.records[0].Expires = time.Now().Add(-time.Second)
	count := f.broadcasts
	if _, err := s.Retry(ctx, r.Signature); err == nil || f.broadcasts != count {
		t.Fatal("expired raw transaction retried")
	}
}

func TestCustomTRC20RawAmountDoesNotDependOnRPCDecimals(t *testing.T) {
	f := newTronFixture(t)
	for _, decimals := range []uint8{6, 18} {
		f.tokenDecimals = decimals
		q, err := f.service.Quote(context.Background(), f.recipient, "", f.contract, "1")
		if err != nil {
			t.Fatal(err)
		}
		if q.AmountRaw != "1" || q.amountRaw.String() != "1" {
			t.Fatalf("RPC decimals %d changed raw amount: %+v", decimals, q)
		}
		to, _ := ParseTronAddress(f.recipient)
		data, _ := erc20ABI.Pack("transfer", common.BytesToAddress(to[1:]), big.NewInt(1))
		if !strings.Contains(hex.EncodeToString(q.raw), hex.EncodeToString(data)) {
			t.Fatalf("RPC decimals %d changed TRC-20 calldata", decimals)
		}
	}
	if _, err := f.service.Quote(context.Background(), f.recipient, "1", f.contract, ""); err == nil {
		t.Fatal("custom TRC-20 quote accepted without exact raw amount")
	}
}
func TestTronStorageFailureAndBackup(t *testing.T) {
	f := newTronFixture(t)
	s := f.service
	ctx := context.Background()
	backup, err := s.Backup("fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewTronService(s.endpoint, "", t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreBackup(backup, "fixture-password", "new-password-123"); err != nil {
		t.Fatal(err)
	}
	status, _ := restored.Status()
	if status.Address != f.address {
		t.Fatal("restored another address")
	}
	if _, err := restored.Create("", "another-password"); !errors.Is(err, ErrWalletExists) {
		t.Fatal("overwrote key")
	}
	q, err := s.Quote(ctx, f.recipient, "1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.dir, "transactions.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(ctx, q.ID, "fixture-password"); err == nil || !s.fault || f.broadcasts != 0 {
		t.Fatal("storage failure broadcast")
	}
}

func TestTronReceiptFalseDoesNotClaimTokenTransfer(t *testing.T) {
	f := newTronFixture(t)
	ctx := context.Background()
	q, err := f.service.Quote(ctx, f.recipient, "", f.contract, "100000")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Send(ctx, q.ID, "fixture-password"); err != nil {
		t.Fatal(err)
	}
	f.finalized = true
	f.tokenFalse = true
	history, err := f.service.History(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if history[0].State != "execution_failed" || history[0].Result != "TRC20_RETURNED_FALSE" || !history[0].Finalized {
		t.Fatal("falsely reported successful token transfer", history)
	}
}

func TestTronRejectsNativeSelfTransfer(t *testing.T) {
	f := newTronFixture(t)
	if _, err := f.service.Quote(context.Background(), f.address, "0.000001", "", ""); err == nil {
		t.Fatal("accepted forbidden TRX self-transfer")
	}
	if len(f.service.quotes) != 0 || f.broadcasts != 0 {
		t.Fatal("self-transfer had side effects")
	}
}
func TestTronExpiryRequiresSolidChainEvidence(t *testing.T) {
	for _, scenario := range []string{"local-clock-only", "rpc-failure", "solid-past-expiry", "receipt-wins"} {
		t.Run(scenario, func(t *testing.T) {
			f := newTronFixture(t)
			s := f.service
			ctx := context.Background()
			q, err := s.Quote(ctx, f.recipient, "0.000001", "", "")
			if err != nil {
				t.Fatal(err)
			}
			f.broadcastFail = true
			if _, err = s.Send(ctx, q.ID, "fixture-password"); err != nil {
				t.Fatal(err)
			}
			s.records[0].Expires = time.Now().Add(-30 * time.Second)
			f.solidTime = time.Now().Add(-time.Minute).UnixMilli()
			if scenario == "solid-past-expiry" || scenario == "receipt-wins" {
				f.solidTime = time.Now().Add(-time.Second).UnixMilli()
			}
			f.solidError = scenario == "rpc-failure"
			f.finalized = scenario == "receipt-wins"
			_, err = s.History(ctx)
			if f.solidError && err == nil {
				t.Fatal("silenced solid query failure")
			}
			if !f.solidError && err != nil {
				t.Fatal(err)
			}
			r := s.records[0]
			switch scenario {
			case "solid-past-expiry":
				if r.State != "expired_unconfirmed" || r.Finalized || s.pending() || r.ExpiryCheckedBlock == "" {
					t.Fatalf("bad expiry evidence: %+v", tronRecordResponse(r))
				}
				if len(s.records) != 1 || r.Signed == "" {
					t.Fatal("deleted journal")
				}
				f.finalized = true
				if _, err = s.History(ctx); err != nil || s.records[0].State != "finalized" {
					t.Fatal("later receipt not reconciled", err)
				}
			case "receipt-wins":
				if r.State != "finalized" || !r.Finalized {
					t.Fatal("expiry overrode receipt")
				}
			default:
				if r.State != "broadcast_unknown" || !s.pending() {
					t.Fatal("local time or RPC error unblocked wallet")
				}
			}
		})
	}
}
