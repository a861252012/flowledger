package wallet

import (
	"cmp"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/a861252012/testnet-wallet-lab/internal/chain"
	"github.com/ethereum/go-ethereum/common"
)

type ActivityTotal struct {
	Asset       string `json:"asset"`
	ReceivedRaw string `json:"receivedRaw"`
	SentRaw     string `json:"sentRaw"`
	FeeRaw      string `json:"feeRaw"`
	NetRaw      string `json:"netRaw"`
}
type ActivityResponse struct {
	ChainID           int64             `json:"chainId"`
	Transactions      []*chain.Activity `json:"transactions"`
	Totals            []ActivityTotal   `json:"totals"`
	Page              int               `json:"page"`
	Pages             int               `json:"pages"`
	TotalTransactions int               `json:"totalTransactions"`
	Incomplete        bool              `json:"incomplete"`
}

type SyncResponse struct {
	From  uint64 `json:"from"`
	To    uint64 `json:"to"`
	Added int    `json:"added"`
}

func (s *Service) activityHashes() ([]string, error) {
	data, err := os.ReadFile(filepath.Join(s.walletDir, "activity.json"))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	if json.Unmarshal(data, &ids) != nil {
		return nil, errors.New("收支索引檔格式錯誤")
	}
	for _, id := range ids {
		if _, err := ParseTransactionHash(id); err != nil {
			return nil, errors.New("收支索引檔雜湊錯誤")
		}
	}
	return ids, nil
}

// addActivityHashes only persists public hashes; amounts are always reconstructed from current RPC evidence.
func (s *Service) addActivityHashes(ids []string) (int, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	old, err := s.activityHashes()
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, id := range old {
		seen[id] = true
	}
	count := 0
	for _, id := range ids {
		if !seen[id] {
			old = append(old, id)
			seen[id] = true
			count += 1
		}
	}
	if count == 0 {
		return 0, nil
	}
	data, err := json.Marshal(old)
	if err != nil {
		return 0, err
	}
	if err := s.keystore.atomicWriteFile(filepath.Join(s.walletDir, "activity.json"), data, 0600); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) ImportActivity(ctx context.Context, hash string) (*chain.Activity, error) {
	address, err := s.keystore.Address()
	if err != nil {
		return nil, err
	}
	result, err := s.client.Activity(ctx, hash, common.HexToAddress(address))
	if err != nil {
		return nil, err
	}
	domain, err := chainActivityToDomain(result)
	if err != nil {
		return nil, err
	}
	if domain.state != activitySucceeded && domain.state != activityReverted {
		return nil, errors.New("交易尚未取得有效的鏈上收據，請稍後再匯入")
	}
	if _, err := s.addActivityHashes([]string{string(domain.hash)}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) SyncActivity(ctx context.Context, from uint64, contracts []string) (*SyncResponse, error) {
	address, err := s.keystore.Address()
	if err != nil {
		return nil, err
	}
	if len(contracts) > 20 {
		return nil, errors.New("每次同步最多 20 個代幣合約")
	}
	addresses := []common.Address{}
	for _, contract := range contracts {
		parsed, err := ValidateAddress(contract)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, parsed)
	}
	ids, start, end, err := s.client.DiscoverActivity(ctx, common.HexToAddress(address), from, addresses...)
	if err != nil {
		return nil, err
	}
	count, err := s.addActivityHashes(ids)
	if err != nil {
		return nil, err
	}
	return &SyncResponse{start, end, count}, nil
}

func (s *Service) Activity(ctx context.Context, page int) (*ActivityResponse, error) {
	if page < 1 {
		return nil, errors.New("頁碼必須大於 0")
	}
	address, err := s.keystore.Address()
	if err != nil {
		return nil, err
	}
	s.sendMu.Lock()
	ids, err := s.activityHashes()
	s.sendMu.Unlock()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	unique := []string{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	history := s.journal.ListHistory()
	for i := len(history) - 1; i >= 0; i -= 1 {
		item := history[i]
		if !seen[item.Hash] {
			seen[item.Hash] = true
			unique = append(unique, item.Hash)
		}
	}
	// Pagination follows index insertion order, not block time; each row shows its verified block time.
	pages := (len(unique) + 19) / 20
	if pages == 0 {
		pages = 1
	}
	if page > pages {
		return nil, errors.New("頁碼超出範圍")
	}
	begin := (page - 1) * 20
	end := min(begin+20, len(unique))
	response := &ActivityResponse{ChainID: s.client.ChainID(), Page: page, Pages: pages, TotalTransactions: len(unique), Transactions: make([]*chain.Activity, end-begin), Totals: []ActivityTotal{}}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := begin; i < end; i += 1 {
		hash := unique[len(unique)-1-i]
		index := i - begin
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			item, err := s.client.Activity(ctx, hash, common.HexToAddress(address))
			if err != nil {
				item = &chain.Activity{Hash: hash, State: "unverified", Movements: []chain.Movement{}, Error: err.Error()}
			}
			response.Transactions[index] = item
		})
	}
	wg.Wait()
	type totals struct{ received, sent, fee *big.Int }
	sums := map[string]*totals{}
	for _, tx := range response.Transactions {
		domain, err := chainActivityToDomain(tx)
		if err != nil {
			return nil, err
		}
		if domain.state != activitySucceeded && domain.state != activityReverted {
			response.Incomplete = true
			continue
		}
		for _, move := range domain.movements {
			sum := sums[move.asset]
			if sum == nil {
				sum = &totals{big.NewInt(0), big.NewInt(0), big.NewInt(0)}
				sums[move.asset] = sum
			}
			switch move.kind {
			case activityReceive:
				sum.received.Add(sum.received, move.amount)
			case activitySend:
				sum.sent.Add(sum.sent, move.amount)
			case activityFee:
				sum.fee.Add(sum.fee, move.amount)
			}
		}
	}
	for asset, sum := range sums {
		net := new(big.Int).Sub(sum.received, sum.sent)
		net.Sub(net, sum.fee)
		response.Totals = append(response.Totals, ActivityTotal{asset, sum.received.String(), sum.sent.String(), sum.fee.String(), net.String()})
	}
	slices.SortFunc(response.Totals, func(a, b ActivityTotal) int { return cmp.Compare(a.Asset, b.Asset) })
	return response, nil
}

func WriteActivityCSV(w io.Writer, response *ActivityResponse) error {
	writer := csv.NewWriter(w)
	chainID := response.ChainID
	if chainID == 0 {
		chainID = chain.SepoliaID
	}
	if err := writer.Write([]string{"chain_id", "hash", "state", "block", "block_time", "kind", "asset", "amount_raw", "counterparty", "evidence"}); err != nil {
		return err
	}
	for _, tx := range response.Transactions {
		if len(tx.Movements) == 0 {
			if err := writer.Write([]string{strconv.FormatInt(chainID, 10), tx.Hash, tx.State, tx.Block, tx.BlockTime, "", "", "", "", ""}); err != nil {
				return err
			}
		}
		for _, m := range tx.Movements {
			if err := writer.Write([]string{strconv.FormatInt(chainID, 10), tx.Hash, tx.State, tx.Block, tx.BlockTime, m.Kind, m.Asset, m.Raw, m.Counterparty, m.Evidence}); err != nil {
				return err
			}
		}
	}
	writer.Flush()
	return writer.Error()
}
