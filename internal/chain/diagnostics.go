package chain

type Diagnostics struct {
	ChainID      int64          `json:"chainId"`
	Instrumented bool           `json:"instrumented"`
	RPC          RPCDiagnostics `json:"rpc"`
}

type RPCDiagnostics struct {
	Requests          uint64 `json:"requests"`
	TransportFailures uint64 `json:"transportFailures"`
	Failovers         uint64 `json:"failovers"`
	LastRequestMS     int64  `json:"lastRequestMs"`
	ActiveEndpoint    int    `json:"activeEndpoint"`
	EndpointCount     int    `json:"endpointCount"`
}

// Diagnostics exposes counters without provider URLs, API keys, or signed payloads.
func (c *Client) Diagnostics() Diagnostics {
	result := Diagnostics{ChainID: c.ChainID(), Instrumented: c.transport != nil}
	if t := c.transport; t != nil {
		result.RPC = RPCDiagnostics{
			Requests:          t.requests.Load(),
			TransportFailures: t.failures.Load(),
			Failovers:         t.failovers.Load(),
			LastRequestMS:     t.lastMS.Load(),
			ActiveEndpoint:    int(t.active.Load()) + 1,
			EndpointCount:     len(t.targets),
		}
	}
	return result
}
