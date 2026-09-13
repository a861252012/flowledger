package wallet

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/a861252012/flowledger/internal/chain"
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
	if json.Unmarshal(data, &ids) != nil || len(ids) > 1000 {
		return nil, errors.New("收支索引檔格式錯誤")
	}
	for _, id := range ids {
		if len(id) != 66 || !strings.HasPrefix(id, "0x") {
			return nil, errors.New("收支索引檔雜湊錯誤")
		}
		if _, err := hex.DecodeString(id[2:]); err != nil {
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
			count++
		}
	}
	if len(old) > 1000 {
		return 0, errors.New("收支索引已達 1000 筆上限")
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
	if result.State != "succeeded" && result.State != "reverted" {
		return nil, errors.New("交易尚未取得有效的鏈上收據，請稍後再匯入")
	}
	if _, err := s.addActivityHashes([]string{result.Hash}); err != nil {
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
	for i := len(history) - 1; i >= 0; i-- {
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
	end := begin + 20
	if end > len(unique) {
		end = len(unique)
	}
	response := &ActivityResponse{Page: page, Pages: pages, TotalTransactions: len(unique), Transactions: make([]*chain.Activity, end-begin), Totals: []ActivityTotal{}}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := begin; i < end; i++ {
		hash := unique[len(unique)-1-i]
		index := i - begin
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			item, err := s.client.Activity(ctx, hash, common.HexToAddress(address))
			if err != nil {
				item = &chain.Activity{Hash: hash, State: "unverified", Movements: []chain.Movement{}, Error: err.Error()}
			}
			response.Transactions[index] = item
		}()
	}
	wg.Wait()
	type totals struct{ received, sent, fee *big.Int }
	sums := map[string]*totals{}
	for _, tx := range response.Transactions {
		if tx.State != "succeeded" && tx.State != "reverted" {
			response.Incomplete = true
			continue
		}
		for _, move := range tx.Movements {
			amount, ok := new(big.Int).SetString(move.Raw, 10)
			if !ok {
				return nil, errors.New("收支金額格式錯誤")
			}
			sum := sums[move.Asset]
			if sum == nil {
				sum = &totals{big.NewInt(0), big.NewInt(0), big.NewInt(0)}
				sums[move.Asset] = sum
			}
			switch move.Kind {
			case "receive":
				sum.received.Add(sum.received, amount)
			case "send":
				sum.sent.Add(sum.sent, amount)
			case "fee":
				sum.fee.Add(sum.fee, amount)
			}
		}
	}
	for asset, sum := range sums {
		net := new(big.Int).Sub(sum.received, sum.sent)
		net.Sub(net, sum.fee)
		response.Totals = append(response.Totals, ActivityTotal{asset, sum.received.String(), sum.sent.String(), sum.fee.String(), net.String()})
	}
	sort.Slice(response.Totals, func(i, j int) bool { return response.Totals[i].Asset < response.Totals[j].Asset })
	return response, nil
}

func WriteActivityCSV(w io.Writer, response *ActivityResponse) error {
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{"chain_id", "hash", "state", "block", "block_time", "kind", "asset", "amount_raw", "counterparty", "evidence"}); err != nil {
		return err
	}
	for _, tx := range response.Transactions {
		if len(tx.Movements) == 0 {
			if err := writer.Write([]string{strconv.Itoa(chain.SepoliaID), tx.Hash, tx.State, tx.Block, tx.BlockTime, "", "", "", "", ""}); err != nil {
				return err
			}
		}
		for _, m := range tx.Movements {
			if err := writer.Write([]string{strconv.Itoa(chain.SepoliaID), tx.Hash, tx.State, tx.Block, tx.BlockTime, m.Kind, m.Asset, m.Raw, m.Counterparty, m.Evidence}); err != nil {
				return err
			}
		}
	}
	writer.Flush()
	return writer.Error()
}
