package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(value any) *http.Response {
	data, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}
}
func TestEVMEvidence(t *testing.T) {
	for _, mode := range []string{"included", "finalized", "wrong signed chain", "reverted", "orphan", "foreign sender", "wrong amount", "swap", "small swap", "foreign swap", "unknown", "RPC error", "wrong target", "wrong calldata", "missing receipt"} {
		t.Run(mode, func(t *testing.T) {
			hash := "0x" + strings.Repeat("1", 64)
			sender := "0x" + strings.Repeat("2", 40)
			row := evidence{Network: "base", ChainID: 84532, Hash: hash, From: sender, To: sender, Operation: "base", AmountRaw: "1000000000000"}
			receipt := map[string]any{"transactionHash": hash, "status": "0x1", "blockNumber": "0xa", "blockHash": "canonical"}
			tx := map[string]any{"hash": hash, "from": sender, "to": sender, "value": "0xe8d4a51000", "chainId": "0x14a34", "blockHash": "canonical"}
			block := map[string]any{"hash": "canonical", "transactions": []string{hash}}
			final := "0x9"
			switch mode {
			case "finalized":
				final = "0xa"
			case "wrong signed chain":
				tx["chainId"] = "0x1"
			case "reverted":
				receipt["status"] = "0x0"
			case "orphan":
				block["hash"] = "orphan"
			case "foreign sender":
				tx["from"] = "other"
			case "wrong amount":
				tx["value"] = "0x1"
			case "unknown":
				row.Network = "mainnet"
			case "wrong target":
				row.TxTo = "other"
			case "wrong calldata":
				row.Calldata = "0xab"
			case "missing receipt":
				receipt = nil
			}
			if strings.Contains(mode, "swap") {
				row.Operation = "swap"
				row.TokenOut = "0x" + strings.Repeat("4", 40)
				row.MinimumOutRaw = "100"
				amount := "0x64"
				recipient := "0x" + strings.Repeat("0", 24) + sender[2:]
				if mode == "small swap" {
					amount = "0x63"
				}
				if mode == "foreign swap" {
					recipient = "other"
				}
				receipt["logs"] = []any{map[string]any{"address": row.TokenOut, "data": amount, "topics": []string{"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", "0x0", recipient}}}
			}
			calls := 0
			v := verifier{&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				var req struct {
					Method string
					Params []json.RawMessage
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				if mode == "RPC error" {
					return reply(map[string]any{"error": map[string]any{"code": -1}}), nil
				}
				var value any
				switch req.Method {
				case "eth_chainId":
					value = "0x14a34"
				case "eth_getTransactionReceipt":
					value = receipt
				case "eth_getTransactionByHash":
					value = tx
				case "eth_getBlockByNumber":
					if string(req.Params[0]) == `"finalized"` {
						value = map[string]string{"number": final}
					} else {
						value = block
					}
				default:
					t.Fatalf("unexpected RPC %s", req.Method)
				}
				return reply(map[string]any{"result": value}), nil
			})}}
			result, err := v.verify(row)
			success := mode == "included" || mode == "finalized" || mode == "swap"
			if (err == nil) != success {
				t.Fatalf("error %v", err)
			}
			if success && (result.Block != 10 || result.Finalized != (mode == "finalized")) {
				t.Fatalf("result %+v", result)
			}
			if mode == "unknown" && calls != 0 {
				t.Fatal("unknown network made request")
			}
		})
	}
}
func TestTronEvidence(t *testing.T) {
	for _, mode := range []string{"native", "token", "wrong genesis", "failed", "missing receipt", "wrong sender", "wrong recipient", "wrong contract", "false token", "missing contract"} {
		t.Run(mode, func(t *testing.T) {
			row := evidence{Network: "tron", Hash: strings.Repeat("1", 64), From: "owner", To: "recipient"}
			if mode == "token" || mode == "wrong contract" || mode == "false token" {
				row.Contract = "token"
			}
			v := verifier{&http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/wallet/getblockbynum":
					id := genesis
					if mode == "wrong genesis" {
						id = "other"
					}
					return reply(map[string]string{"blockID": id}), nil
				case "/walletsolidity/gettransactioninfobyid":
					value := map[string]any{"id": row.Hash, "blockNumber": 10, "fee": 5, "contractResult": []string{strings.Repeat("0", 63) + "1"}}
					if mode == "failed" {
						value["result"] = "FAILED"
					}
					if mode == "missing receipt" {
						delete(value, "id")
					}
					if mode == "false token" {
						value["contractResult"] = []string{strings.Repeat("0", 64)}
					}
					return reply(value), nil
				case "/walletsolidity/gettransactionbyid":
					owner, to, contract := "owner", "recipient", "token"
					if mode == "wrong sender" {
						owner = "other"
					}
					if mode == "wrong recipient" {
						to = "other"
					}
					if mode == "wrong contract" {
						contract = "other"
					}
					contracts := []any{map[string]any{"parameter": map[string]any{"value": map[string]string{"owner_address": owner, "to_address": to, "contract_address": contract}}}}
					if mode == "missing contract" {
						contracts = nil
					}
					return reply(map[string]any{"txID": row.Hash, "raw_data": map[string]any{"contract": contracts}}), nil
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
					return nil, nil
				}
			})}}
			result, err := v.verify(row)
			success := mode == "native" || mode == "token"
			if (err == nil) != success {
				t.Fatal(err)
			}
			if success && (!result.Finalized || result.Block != 10 || result.FeeSun == nil || *result.FeeSun != 5) {
				t.Fatalf("%+v", result)
			}
		})
	}
}
func TestManifestFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(path, []byte(`{"transactions":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network"); return nil, nil })}
	for _, args := range [][]string{{"--manifest", path}, {"--manifest", path, "--network", "mainnet"}} {
		if err := run(args, client, &bytes.Buffer{}); err == nil {
			t.Fatal("expected failure")
		}
	}
	if err := os.WriteFile(path, []byte(`{"transactions":[{"network":"unknown","hash":"bad"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"--manifest", path}, client, &output); err == nil || !strings.Contains(output.String(), "unknown network") {
		t.Fatalf("%v %s", err, output.String())
	}
}
