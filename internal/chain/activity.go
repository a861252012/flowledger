package chain

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const SepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
const SepoliaWETH = "0xfff9976782d46cc05630d1f6ebab18b2324d6b14"

var transferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
var depositTopic = crypto.Keccak256Hash([]byte("Deposit(address,uint256)"))
var withdrawalTopic = crypto.Keccak256Hash([]byte("Withdrawal(address,uint256)"))
var ErrUnrelated = errors.New("此交易沒有可辨識、與本錢包相關的收支")

type Movement struct {
	Kind         string `json:"kind"`
	Asset        string `json:"asset"`
	Raw          string `json:"raw"`
	Counterparty string `json:"counterparty"`
	Evidence     string `json:"evidence"`
}

type Activity struct {
	Hash      string     `json:"hash"`
	State     string     `json:"state"`
	Block     string     `json:"block,omitempty"`
	BlockHash string     `json:"blockHash,omitempty"`
	BlockTime string     `json:"blockTime,omitempty"`
	CheckedAt time.Time  `json:"checkedAt"`
	Movements []Movement `json:"movements"`
	Error     string     `json:"error,omitempty"`
}

// Activity reads canonical receipt evidence. It does not treat pending intent as an asset movement.
func (c *Client) Activity(ctx context.Context, hash string, owner common.Address) (*Activity, error) {
	if !hashPattern.MatchString(hash) {
		return nil, ErrHash
	}
	if err := c.checkNetwork(ctx); err != nil {
		return nil, err
	}
	id := common.HexToHash(hash)
	tx, pending, err := c.rpc.TransactionByHash(ctx, id)
	if errors.Is(err, ethereum.NotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, rpcError(err)
	}
	if tx.Hash() != id || tx.ChainId().Cmp(big.NewInt(SepoliaID)) != 0 {
		return nil, ErrUnavailable
	}
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(SepoliaID)), tx)
	if err != nil {
		return nil, ErrUnavailable
	}
	related := from == owner || (tx.To() != nil && *tx.To() == owner)
	out := &Activity{Hash: id.Hex(), State: "pending", CheckedAt: time.Now().UTC(), Movements: []Movement{}}
	if pending {
		if !related {
			return nil, ErrUnrelated
		}
		return out, nil
	}
	r, err := c.rpc.TransactionReceipt(ctx, id)
	if errors.Is(err, ethereum.NotFound) {
		if !related {
			return nil, ErrUnrelated
		}
		out.State = "receipt_unavailable"
		return out, nil
	}
	if err != nil {
		return nil, rpcError(err)
	}
	if r.TxHash != id || r.BlockNumber == nil || r.EffectiveGasPrice == nil || r.EffectiveGasPrice.Sign() < 0 || r.Status > 1 {
		return nil, ErrUnavailable
	}
	h, err := c.rpc.HeaderByNumber(ctx, r.BlockNumber)
	if err != nil {
		return nil, rpcError(err)
	}
	if h.Hash() != r.BlockHash {
		out.State = "reorg_detected"
		return out, nil
	}
	out.Block, out.BlockHash, out.BlockTime = r.BlockNumber.String(), r.BlockHash.Hex(), time.Unix(int64(h.Time), 0).UTC().Format(time.RFC3339)
	out.State = "reverted"
	if r.Status == types.ReceiptStatusSuccessful {
		out.State = "succeeded"
	}
	moves, err := receiptMovements(tx, r, from, owner)
	if err != nil {
		return nil, err
	}
	if !related && len(moves) == 0 {
		return nil, ErrUnrelated
	}
	// Recheck canonical block after collecting evidence; never publish movements from an observed orphan.
	final, err := c.rpc.HeaderByNumber(ctx, r.BlockNumber)
	if err != nil {
		return nil, rpcError(err)
	}
	if final.Hash() != r.BlockHash {
		out.State = "reorg_detected"
		return out, nil
	}
	out.Movements = moves
	return out, nil
}

func receiptMovements(tx *types.Transaction, r *types.Receipt, from, owner common.Address) ([]Movement, error) {
	moves := []Movement{}
	add := func(kind, asset string, amount *big.Int, other common.Address, evidence string) {
		if amount.Sign() > 0 {
			moves = append(moves, Movement{kind, asset, amount.String(), other.Hex(), evidence})
		}
	}
	if from == owner {
		fee := new(big.Int).Mul(new(big.Int).SetUint64(r.GasUsed), r.EffectiveGasPrice)
		add("fee", "ETH", fee, common.Address{}, "receipt.gasUsed × effectiveGasPrice")
	}
	if r.Status != types.ReceiptStatusSuccessful {
		return moves, nil
	}
	if from == owner {
		target := r.ContractAddress
		if tx.To() != nil {
			target = *tx.To()
		}
		add("send", "ETH", tx.Value(), target, "transaction.value")
	}
	if tx.To() != nil && *tx.To() == owner {
		add("receive", "ETH", tx.Value(), from, "transaction.value")
	}
	ownerTopic := common.BytesToHash(owner.Bytes())
	seen := map[uint]bool{}
	for _, l := range r.Logs {
		if l == nil || l.Removed || l.TxHash != r.TxHash || l.BlockHash != r.BlockHash || l.BlockNumber != r.BlockNumber.Uint64() {
			return nil, ErrUnavailable
		}
		if seen[l.Index] {
			return nil, ErrUnavailable
		}
		seen[l.Index] = true
		if len(l.Data) != 32 {
			continue
		}
		amount := new(big.Int).SetBytes(l.Data)
		evidence := "log:" + strconv.FormatUint(uint64(l.Index), 10)
		if len(l.Topics) == 3 && l.Topics[0] == transferTopic {
			if !bytes.Equal(l.Topics[1][:12], make([]byte, 12)) || !bytes.Equal(l.Topics[2][:12], make([]byte, 12)) {
				continue
			}
			if l.Topics[1] == ownerTopic {
				add("send", l.Address.Hex(), amount, common.BytesToAddress(l.Topics[2].Bytes()), evidence)
			}
			if l.Topics[2] == ownerTopic {
				add("receive", l.Address.Hex(), amount, common.BytesToAddress(l.Topics[1].Bytes()), evidence)
			}
		}
		// WETH9 uses Deposit/Withdrawal rather than mint/burn Transfer events.
		if l.Address == common.HexToAddress(SepoliaWETH) && len(l.Topics) == 2 && l.Topics[1] == ownerTopic {
			if l.Topics[0] == depositTopic {
				add("receive", l.Address.Hex(), amount, l.Address, evidence+":deposit")
			}
			if l.Topics[0] == withdrawalTopic {
				add("send", l.Address.Hex(), amount, l.Address, evidence+":withdraw")
				add("receive", "ETH", amount, l.Address, evidence+":withdraw ETH")
			}
		}
	}
	return moves, nil
}

// DiscoverActivity scans a bounded range of full blocks and ERC-20 incoming logs; no API key or explorer indexer.
func (c *Client) DiscoverActivity(ctx context.Context, owner common.Address, from uint64, contracts ...common.Address) ([]string, uint64, uint64, error) {
	if err := c.checkNetwork(ctx); err != nil {
		return nil, 0, 0, err
	}
	head, err := c.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, 0, 0, rpcError(err)
	}
	end := head.Number.Uint64()
	if from == 0 {
		if end > 19 {
			from = end - 19
		} else {
			from = 1
		}
	}
	if from > end {
		return nil, 0, 0, errors.New("起始區塊超過最新區塊")
	}
	if end-from > 19 {
		end = from + 19
	}
	ids := []string{}
	seen := map[common.Hash]bool{}
	add := func(id common.Hash) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id.Hex())
		}
	}
	for n := from; n <= end; n += 1 {
		b, err := c.rpc.BlockByNumber(ctx, new(big.Int).SetUint64(n))
		if err != nil {
			return nil, 0, 0, rpcError(err)
		}
		for _, tx := range b.Transactions() {
			sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(SepoliaID)), tx)
			if err != nil {
				continue
			}
			if sender == owner || (tx.To() != nil && *tx.To() == owner) {
				add(tx.Hash())
			}
		}
	}
	addresses := []common.Address{common.HexToAddress(SepoliaWETH), common.HexToAddress(SepoliaUSDC)}
	addressSet := map[common.Address]bool{addresses[0]: true, addresses[1]: true}
	for _, address := range contracts {
		if address != (common.Address{}) && !addressSet[address] {
			addresses = append(addresses, address)
			addressSet[address] = true
		}
	}
	if len(addresses) > 20 {
		return nil, 0, 0, errors.New("每次同步最多 20 個代幣合約")
	}
	logs, err := c.rpc.FilterLogs(ctx, ethereum.FilterQuery{Addresses: addresses, FromBlock: new(big.Int).SetUint64(from), ToBlock: new(big.Int).SetUint64(end), Topics: [][]common.Hash{{transferTopic}, {}, {common.BytesToHash(owner.Bytes())}}})
	if err != nil {
		return nil, 0, 0, rpcError(err)
	}
	for _, l := range logs {
		if addressSet[l.Address] && !l.Removed && len(l.Topics) == 3 && l.Topics[0] == transferTopic && l.Topics[2] == common.BytesToHash(owner.Bytes()) && len(l.Data) == 32 && l.BlockNumber >= from && l.BlockNumber <= end {
			add(l.TxHash)
		}
	}
	if len(ids) > 200 {
		return nil, 0, 0, errors.New("此範圍相關交易過多，請用交易雜湊個別匯入")
	}
	return ids, from, end, nil
}
