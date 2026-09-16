package chain

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// NewFallback retries transport failures with identical request bytes. Each endpoint is checked before use.
func NewFallback(endpoints []string) (*Client, error) { return NewNetwork(SepoliaID, endpoints) }

func NewNetwork(chainID int64, endpoints []string) (*Client, error) {
	if chainID != SepoliaID && chainID != 421614 && chainID != 84532 && chainID != 11155420 && chainID != 80002 {
		return nil, ErrNetwork
	}
	if len(endpoints) == 0 || len(endpoints) > 4 {
		return nil, ErrUnavailable
	}
	targets := make([]*url.URL, len(endpoints))
	for i, endpoint := range endpoints {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, ErrUnavailable
		}
		targets[i] = u
	}
	transport := &fallbackTransport{targets: targets, base: http.DefaultTransport, chainID: chainID}
	c, err := rpc.DialOptions(context.Background(), endpoints[0], rpc.WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Client{rpc: ethclient.NewClient(c), chainID: chainID, transport: transport}, nil
}

type fallbackTransport struct {
	requests  atomic.Uint64
	failures  atomic.Uint64
	failovers atomic.Uint64
	lastMS    atomic.Int64
	active    atomic.Uint32
	chainID   int64
	targets   []*url.URL
	base      http.RoundTripper
}

func (t *fallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	started := time.Now()
	success := false
	t.requests.Add(1)
	defer func() {
		t.lastMS.Store(time.Since(started).Milliseconds())
		if !success {
			t.failures.Add(1)
		}
	}()
	payload, err := io.ReadAll(io.LimitReader(req.Body, 1024*1024+1))
	req.Body.Close()
	if err != nil || len(payload) > 1024*1024 {
		return nil, ErrUnavailable
	}
	start := int(t.active.Load())
	for offset := range t.targets {
		index := (start + offset) % len(t.targets)
		endpoint := t.targets[index]
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
		probe := req.Clone(ctx)
		probe.URL = endpoint
		probe.Host = endpoint.Host
		probe.Header.Del("Authorization")
		if endpoint.User != nil {
			password, _ := endpoint.User.Password()
			probe.SetBasicAuth(endpoint.User.Username(), password)
		}
		probe.Body = io.NopCloser(bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}`))
		probe.ContentLength = -1
		response, err := t.base.RoundTrip(probe)
		var result struct {
			Result string `json:"result"`
		}
		valid := false
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
			response.Body.Close()
			valid = readErr == nil && response.StatusCode == 200 && json.Unmarshal(data, &result) == nil
		}
		cancel()
		id, ok := new(big.Int).SetString(result.Result, 0)
		if !valid || !ok {
			continue
		}
		if id.Cmp(big.NewInt(t.chainID)) != 0 {
			continue
		}
		ctx, cancel = context.WithTimeout(req.Context(), 8*time.Second)
		attempt := req.Clone(ctx)
		attempt.URL = endpoint
		attempt.Host = endpoint.Host
		attempt.Header.Del("Authorization")
		if endpoint.User != nil {
			password, _ := endpoint.User.Password()
			attempt.SetBasicAuth(endpoint.User.Username(), password)
		}
		attempt.Body = io.NopCloser(bytes.NewReader(payload))
		attempt.ContentLength = int64(len(payload))
		response, err = t.base.RoundTrip(attempt)
		if err != nil {
			cancel()
			continue
		}
		if response.StatusCode == 429 || response.StatusCode >= 500 {
			response.Body.Close()
			cancel()
			continue
		}
		success = true
		if offset > 0 {
			t.failovers.Add(1)
		}
		t.active.Store(uint32(index))
		// Keep the deadline alive until the RPC decoder closes its response.
		response.Body = &cancelBody{ReadCloser: response.Body, cancel: cancel}
		return response, nil
	}
	return nil, ErrUnavailable
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }
