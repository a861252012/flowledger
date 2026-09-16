package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type ScanProgress struct {
	Enabled   bool      `json:"enabled"`
	Start     uint64    `json:"start"`
	Next      uint64    `json:"next"`
	Finalized uint64    `json:"finalized"`
	Tokens    []string  `json:"tokens"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type scanProgressDisk struct {
	Enabled   bool      `json:"enabled"`
	Start     uint64    `json:"start"`
	Next      uint64    `json:"next"`
	Finalized uint64    `json:"finalized"`
	Tokens    []string  `json:"tokens"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type scanState struct {
	Enabled   bool
	Start     uint64
	Next      uint64
	Finalized uint64
	Tokens    []EVMAddress
	Error     string
	UpdatedAt time.Time
}

func scanProgressResponse(state *scanState) *ScanProgress {
	var tokens []string
	if state.Tokens != nil {
		tokens = make([]string, len(state.Tokens))
	}
	for i, token := range state.Tokens {
		tokens[i] = string(token)
	}
	return &ScanProgress{Enabled: state.Enabled, Start: state.Start, Next: state.Next, Finalized: state.Finalized, Tokens: tokens, Error: state.Error, UpdatedAt: state.UpdatedAt}
}

func scanProgressFromDisk(stored scanProgressDisk) (*scanState, error) {
	if len(stored.Tokens) > 200 {
		return nil, errors.New("同步進度檔代幣數量超過上限")
	}
	for _, token := range stored.Tokens {
		if !common.IsHexAddress(token) || common.HexToAddress(token) == (common.Address{}) {
			return nil, errors.New("同步進度檔包含無效代幣地址")
		}
	}
	var tokens []EVMAddress
	if stored.Tokens != nil {
		tokens = make([]EVMAddress, len(stored.Tokens))
	}
	for i, token := range stored.Tokens {
		tokens[i] = EVMAddress(token)
	}
	return &scanState{
		Enabled: stored.Enabled, Start: stored.Start, Next: stored.Next, Finalized: stored.Finalized,
		Tokens: tokens, Error: stored.Error, UpdatedAt: stored.UpdatedAt,
	}, nil
}

func scanProgressToDisk(state *scanState) scanProgressDisk {
	var tokens []string
	if state.Tokens != nil {
		tokens = make([]string, len(state.Tokens))
		for i, token := range state.Tokens {
			tokens[i] = string(token)
		}
	}
	return scanProgressDisk{
		Enabled: state.Enabled, Start: state.Start, Next: state.Next, Finalized: state.Finalized,
		Tokens: tokens, Error: state.Error, UpdatedAt: state.UpdatedAt,
	}
}

func (s *Service) scanProgress() (*scanState, error) {
	data, err := os.ReadFile(filepath.Join(s.walletDir, "scan.json"))
	if os.IsNotExist(err) {
		return &scanState{Tokens: []EVMAddress{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored scanProgressDisk
	if json.Unmarshal(data, &stored) != nil {
		return nil, errors.New("同步進度檔格式錯誤")
	}
	return scanProgressFromDisk(stored)
}

func (s *Service) ScanProgress() (*ScanProgress, error) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	state, err := s.scanProgress()
	if err != nil {
		return nil, err
	}
	return scanProgressResponse(state), nil
}

func (s *Service) ConfigureScan(enabled bool, start *uint64) (*ScanProgress, error) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	state, err := s.scanProgress()
	if err != nil {
		return nil, err
	}
	if start != nil {
		state.Start = *start
		state.Next = *start
	}
	state.Enabled = enabled
	state.Error = ""
	data, err := json.Marshal(scanProgressToDisk(state))
	if err != nil {
		return nil, err
	}
	if err := atomicWriteFile(filepath.Join(s.walletDir, "scan.json"), data, 0600); err != nil {
		return nil, err
	}
	return scanProgressResponse(state), nil
}

func (s *Service) ScanOnce(ctx context.Context) error {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	state, err := s.scanProgress()
	if err != nil || !state.Enabled {
		return err
	}
	address, err := s.keystore.Address()
	if err != nil {
		return err
	}
	final, err := s.client.FinalizedNumber(ctx)
	if err == nil {
		state.Finalized = final
		if state.Next == 0 {
			state.Next = final
			if final > 19 {
				state.Next = final - 19
			}
			state.Start = state.Next
		}
		if state.Next <= final {
			block, scanErr := s.client.ScanFinalizedBlock(ctx, state.Next, common.HexToAddress(address))
			err = scanErr
			if err == nil {
				_, err = s.addActivityHashes(block.Hashes)
			}
			if err == nil {
				known := map[EVMAddress]bool{}
				for _, token := range state.Tokens {
					known[token] = true
				}
				for _, raw := range block.Tokens {
					token, parseErr := ParseEVMAddress(raw)
					if parseErr != nil {
						return parseErr
					}
					if !known[token] && len(state.Tokens) < 200 {
						state.Tokens = append(state.Tokens, token)
						known[token] = true
					}
				}
				state.Next += 1
			}
		}
	}
	state.Error = ""
	if err != nil {
		state.Error = err.Error()
	}
	state.UpdatedAt = time.Now().UTC()
	data, marshalErr := json.Marshal(scanProgressToDisk(state))
	if marshalErr != nil {
		return marshalErr
	}
	if saveErr := atomicWriteFile(filepath.Join(s.walletDir, "scan.json"), data, 0600); saveErr != nil {
		return saveErr
	}
	return err
}

// RunMaintenance is cancelled and joined by the server before closing the wallet.
func (s *Service) RunMaintenance(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	nextHistory := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.keystore.Exists() {
				continue
			}
			if time.Now().After(nextHistory) {
				check, cancel := context.WithTimeout(ctx, 10*time.Second)
				_, _ = s.History(check)
				cancel()
				nextHistory = time.Now().Add(15 * time.Second)
			}
			check, cancel := context.WithTimeout(ctx, 40*time.Second)
			_ = s.ScanOnce(check)
			cancel()
		}
	}
}
