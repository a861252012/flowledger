package wallet

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sol "github.com/gagliardetto/solana-go"
)

func TestSLIP10PublishedEd25519Vector(t *testing.T) {
	seed, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	key := deriveEd25519(seed, []uint32{0})
	if hex.EncodeToString(key) != "68e0fe46dfb67e368c75379acec591dad19df3cde26e63b93a8e704f1dade7a3" {
		t.Fatal("SLIP-0010 vector mismatch")
	}
}

func TestSolanaConfirmedFailureRefreshesUntilFinalized(t *testing.T) {
	for _, executionFailed := range []bool{true, false} {
		name := "finalized_success"
		if executionFailed {
			name = "finalized_failure"
		}
		t.Run(name, func(t *testing.T) {
			var queries atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     any    `json:"id"`
					Method string `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				var result any
				switch req.Method {
				case "getGenesisHash":
					result = SolanaDevnetGenesis
				case "getBlockHeight":
					result = 100
				case "getSignatureStatuses":
					query := queries.Add(1)
					status := map[string]any{"slot": 100, "err": nil, "confirmationStatus": "finalized", "confirmations": nil}
					if query == 1 {
						status["confirmationStatus"] = "confirmed"
					}
					if query == 1 || executionFailed {
						status["err"] = map[string]any{"InstructionError": []any{0, "InvalidArgument"}}
					}
					result = map[string]any{"context": map[string]int{"slot": 100}, "value": []any{status}}
				default:
					t.Errorf("unexpected Solana RPC %s", req.Method)
				}
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
			}))
			defer server.Close()
			s, err := NewSolanaService(server.URL, t.TempDir(), 2)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			s.records = []solanaJournalRecord{{Signature: solanaSignature(sol.Signature{}.String()), State: "submitted", LastValid: 200}}
			history, err := s.History(context.Background())
			if err != nil || len(history) != 1 {
				t.Fatalf("confirmed history: %+v, %v", history, err)
			}
			if history[0].State != "execution_failed" || history[0].Finalized {
				t.Fatalf("confirmed failure must remain unfinalized: %+v", history[0])
			}
			if s.hasPending() {
				t.Fatal("execution_failed transaction must not block hasPending")
			}
			wantState := "finalized"
			if executionFailed {
				wantState = "execution_failed"
			}
			for range 2 {
				history, err = s.History(context.Background())
				if err != nil || len(history) != 1 {
					t.Fatalf("finalized history: %+v, %v", history, err)
				}
				if history[0].State != wantState || !history[0].Finalized {
					t.Fatalf("expected finalized state %q: %+v", wantState, history[0])
				}
			}
			if queries.Load() != 2 {
				t.Fatalf("expected two status queries, got %d", queries.Load())
			}
		})
	}
}
func TestSolanaBackupAndBoundBroadcast(t *testing.T) {
	var broadcasts []string
	genesis := SolanaDevnetGenesis
	expired := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any               `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "getGenesisHash":
			result = genesis
		case "getBalance":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": 1000000000}
		case "getLatestBlockhash":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": map[string]any{"blockhash": SolanaDevnetGenesis, "lastValidBlockHeight": 200}}
		case "getFeeForMessage":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": 5000}
		case "simulateTransaction":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": map[string]any{"err": nil}}
		case "getBlockHeight":
			if expired {
				result = 201
			} else {
				result = 100
			}
		case "sendTransaction":
			var raw string
			json.Unmarshal(req.Params[0], &raw)
			broadcasts = append(broadcasts, raw)
			tx, err := sol.TransactionFromBase64(raw)
			if err != nil {
				t.Error(err)
				return
			}
			if tx.VerifySignatures() != nil {
				t.Error("invalid signature")
			}
			result = tx.Signatures[0].String()
		case "getSignatureStatuses":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": []any{map[string]any{"slot": 100, "err": nil, "confirmationStatus": "finalized", "confirmations": nil}}}
		default:
			t.Errorf("unexpected Solana RPC %s", req.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	dir := t.TempDir()
	s, err := NewSolanaService(server.URL, dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	created, err := s.Create("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", "solana-test-password")
	if err != nil {
		t.Fatal(err)
	}
	backup, err := s.Backup("solana-test-password")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewSolanaService(server.URL, t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreBackup(backup, "solana-test-password", "restored-test-password"); err != nil {
		t.Fatal(err)
	}
	status, _ := restored.Status()
	if status.Address != created.Address {
		t.Fatal("restore changed address")
	}
	if err := restored.ChangePassword("restored-test-password", "changed-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Backup("restored-test-password"); err == nil {
		t.Fatal("old password accepted")
	}
	if _, err := s.Quote(context.Background(), created.Address, "0.0000000001"); err == nil {
		t.Fatal("accepted fractional lamport")
	}
	q, err := s.Quote(context.Background(), created.Address, "0.000001")
	if err != nil {
		t.Fatal(err)
	}
	expired = true
	if _, err := s.Send(context.Background(), q.ID, "solana-test-password"); err == nil {
		t.Fatal("expired blockhash accepted")
	}
	expired = false
	genesis = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
	if _, err := s.Send(context.Background(), q.ID, "solana-test-password"); err == nil {
		t.Fatal("mainnet accepted")
	}
	genesis = SolanaDevnetGenesis
	sent, err := s.Send(context.Background(), q.ID, "solana-test-password")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Send(context.Background(), q.ID, "ignored-duplicate")
	if err != nil || sent.Signature != duplicate.Signature || len(broadcasts) != 1 {
		t.Fatal("duplicate send")
	}
	raw, _ := base64.StdEncoding.DecodeString(broadcasts[0])
	if len(raw) == 0 {
		t.Fatal("no signed transaction")
	}
	s.Close()
	s, err = NewSolanaService(server.URL, dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retry(context.Background(), sent.Signature); err != nil {
		t.Fatal(err)
	}
	if len(broadcasts) != 2 || broadcasts[0] != broadcasts[1] {
		t.Fatal("retry changed signed bytes")
	}
	history, err := s.History(context.Background())
	if err != nil || len(history) != 1 || !history[0].Finalized {
		t.Fatalf("history %+v %v", history, err)
	}
}

func TestSolanaExpiredUnconfirmedUnlocksPendingAndQuote(t *testing.T) {
	currentHeight := uint64(100)
	var sigStatuses []any
	statusesFail := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any               `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var result any
		switch req.Method {
		case "getGenesisHash":
			result = SolanaDevnetGenesis
		case "getBalance":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": 10000000000}
		case "getLatestBlockhash":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": map[string]any{"blockhash": SolanaDevnetGenesis, "lastValidBlockHeight": currentHeight + 100}}
		case "getFeeForMessage":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": 5000}
		case "simulateTransaction":
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": map[string]any{"err": nil}}
		case "getBlockHeight":
			result = currentHeight
		case "sendTransaction":
			var raw string
			json.Unmarshal(req.Params[0], &raw)
			tx, _ := sol.TransactionFromBase64(raw)
			result = tx.Signatures[0].String()
		case "getSignatureStatuses":
			if statusesFail {
				http.Error(w, "RPC timeout", http.StatusGatewayTimeout)
				return
			}
			result = map[string]any{"context": map[string]int{"slot": 100}, "value": sigStatuses}
		default:
			t.Errorf("unexpected Solana RPC %s", req.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()

	dir := t.TempDir()
	s, err := NewSolanaService(server.URL, dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	created, err := s.Create("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about", "solana-test-password")
	if err != nil {
		t.Fatal(err)
	}

	q, err := s.Quote(context.Background(), created.Address, "0.000001")
	if err != nil {
		t.Fatal(err)
	}

	sent, err := s.Send(context.Background(), q.ID, "solana-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if sent.State != "submitted" {
		t.Fatalf("expected state submitted, got %s", sent.State)
	}
	if s.records[0].State != "submitted" {
		t.Fatalf("expected memory record submitted, got %s", s.records[0].State)
	}

	// Close s to release flock, then verify disk record was updated to submitted
	s.Close()
	s, err = NewSolanaService(server.URL, dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.records[0].State != "submitted" {
		t.Fatalf("expected persisted disk record submitted, got %s", s.records[0].State)
	}

	// Pending transaction prevents new quote
	if _, err := s.Quote(context.Background(), created.Address, "0.000001"); err == nil {
		t.Fatal("expected pending transaction error when quoting, got nil")
	}

	// In-flight transaction check: height (150) <= LastValid (200) and nil status
	// History() must preserve "submitted" state, never reverting to "broadcast_unknown"
	currentHeight = 150
	sigStatuses = []any{nil}
	histInFlight, err := s.History(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(histInFlight) != 1 || histInFlight[0].State != "submitted" {
		t.Fatalf("expected in-flight state to remain submitted, got %+v", histInFlight)
	}
	if !s.hasPending() {
		t.Fatal("in-flight transaction must remain pending")
	}

	// Signature status returns nil (not confirmed), but block height exceeds LastValid (200)
	currentHeight = 250
	sigStatuses = []any{nil}

	history, err := s.History(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(history))
	}
	if history[0].State != "expired_unconfirmed" {
		t.Fatalf("expected expired_unconfirmed, got %s", history[0].State)
	}
	if history[0].Finalized {
		t.Fatal("expired transaction should not be finalized")
	}

	// hasPending must now be false
	if s.hasPending() {
		t.Fatal("hasPending should be false after expiration")
	}

	// Retry on expired transaction must be rejected
	if _, err := s.Retry(context.Background(), sent.Signature); err == nil {
		t.Fatal("expected retry on expired transaction to fail, got nil")
	}

	// Quoting should now succeed
	q2, err := s.Quote(context.Background(), created.Address, "0.000002")
	if err != nil {
		t.Fatalf("quote failed after expiration unlock: %v", err)
	}
	if q2.Amount != "0.000002" {
		t.Fatalf("unexpected quote amount: %s", q2.Amount)
	}

	// Reopen service from disk and verify expired_unconfirmed persisted
	s.Close()
	s, err = NewSolanaService(server.URL, dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	if s.hasPending() {
		t.Fatal("reloaded service should not have pending")
	}
	if s.records[0].State != "expired_unconfirmed" {
		t.Fatalf("reloaded state expected expired_unconfirmed, got %s", s.records[0].State)
	}

	// Verify single signature query failure does not abort History() or corrupt expired_unconfirmed state
	statusesFail = true
	histFail, err := s.History(context.Background())
	if err != nil {
		t.Fatalf("History returned error on single signature failure: %v", err)
	}
	if len(histFail) != 1 {
		t.Fatalf("expected history to retain records even on query failure, got %d", len(histFail))
	}
	if histFail[0].State != "expired_unconfirmed" {
		t.Fatalf("query failure must not corrupt expired_unconfirmed state, got %s", histFail[0].State)
	}
	if s.hasPending() {
		t.Fatal("query failure must not resurrect pending state")
	}
	statusesFail = false

	// Test execution failure: quote and send a transaction that fails on-chain
	q3, err := s.Quote(context.Background(), created.Address, "0.000003")
	if err != nil {
		t.Fatalf("quote on reloaded service failed: %v", err)
	}
	sent2, err := s.Send(context.Background(), q3.ID, "solana-test-password")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	sigStatuses = []any{map[string]any{
		"slot":               110,
		"err":                map[string]any{"InstructionError": []any{0, "Custom"}},
		"confirmationStatus": "confirmed",
		"confirmations":      nil,
	}}
	histFailed, err := s.History(context.Background())
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	var failedRec *SolanaRecord
	for _, rec := range histFailed {
		if rec.Signature == sent2.Signature {
			failedRec = &rec
			break
		}
	}
	if failedRec == nil || failedRec.State != "execution_failed" {
		t.Fatalf("expected execution_failed state, got %+v", failedRec)
	}
	if s.hasPending() {
		t.Fatal("execution_failed transaction must not block hasPending")
	}
}
