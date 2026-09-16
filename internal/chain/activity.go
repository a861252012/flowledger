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
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const SepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
const SepoliaWETH = "0xfff9976782d46cc05630d1f6ebab18b2324d6b14"

var transferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
var depositTopic = crypto.Keccak256Hash([]byte("Deposit(address,uint256)"))
var withdrawalTopic = crypto.Keccak256Hash([]byte("Withdrawal(address,uint256)"))
var ErrUnrelated = errors.New("此交易沒有可辨識、與本錢包相關的收支")
var zeroBytes12 [12]byte

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
	if tx.Hash() != id || tx.ChainId().Cmp(big.NewInt(c.ChainID())) != 0 {
		return nil, ErrUnavailable
	}
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(c.ChainID())), tx)
	if err != nil {
		return nil, ErrUnavailable
	}
	related := from == owner || (tx.To() != nil && *tx.To() == owner)
	out := activityRecord{hash: id, state: transactionPending, checkedAt: time.Now().UTC(), movements: []movementRecord{}}
	if pending {
		if !related {
			return nil, ErrUnrelated
		}
		return activityToAPI(out), nil
	}
	r, err := c.rpc.TransactionReceipt(ctx, id)
	if errors.Is(err, ethereum.NotFound) {
		if !related {
			return nil, ErrUnrelated
		}
		out.state = transactionReceiptUnavailable
		return activityToAPI(out), nil
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
		out.state = transactionReorgDetected
		return activityToAPI(out), nil
	}
	blockTime := time.Unix(int64(h.Time), 0).UTC()
	out.block, out.blockHash, out.blockTime = r.BlockNumber, r.BlockHash, &blockTime
	out.state = transactionReverted
	if r.Status == types.ReceiptStatusSuccessful {
		out.state = transactionSucceeded
	}
	moves, err := receiptMovements(tx, r, from, owner, c.ChainID())
	if err != nil {
		return nil, err
	}
	if from == owner && c.IsOPStack() {
		fee, err := c.ReceiptFee(ctx, r)
		if err != nil {
			return nil, err
		}
		for i := range moves {
			if moves[i].kind == movementFee {
				moves[i].raw = fee
				moves[i].evidence = "receipt execution + L1 fee + historical operator fee"
			}
		}
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
		out.state = transactionReorgDetected
		out.movements = nil
		return activityToAPI(out), nil
	}
	out.movements = moves
	return activityToAPI(out), nil
}

func receiptMovements(tx *types.Transaction, r *types.Receipt, from, owner common.Address, chainIDs ...int64) ([]movementRecord, error) {
	moves := []movementRecord{}
	native := "ETH"
	if len(chainIDs) > 0 && chainIDs[0] == 80002 {
		native = "POL"
	}
	add := func(kind movementKind, asset string, amount *big.Int, other common.Address, evidence string) {
		if amount.Sign() > 0 {
			moves = append(moves, movementRecord{kind: kind, asset: asset, raw: new(big.Int).Set(amount), counterparty: other, evidence: evidence})
		}
	}
	if from == owner {
		fee := new(big.Int).Mul(new(big.Int).SetUint64(r.GasUsed), r.EffectiveGasPrice)
		add(movementFee, native, fee, common.Address{}, "receipt.gasUsed × effectiveGasPrice")
	}
	if r.Status != types.ReceiptStatusSuccessful {
		return moves, nil
	}
	if from == owner {
		target := r.ContractAddress
		if tx.To() != nil {
			target = *tx.To()
		}
		add(movementSend, native, tx.Value(), target, "transaction.value")
	}
	if tx.To() != nil && *tx.To() == owner {
		add(movementReceive, native, tx.Value(), from, "transaction.value")
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
			if !bytes.Equal(l.Topics[1][:12], zeroBytes12[:]) || !bytes.Equal(l.Topics[2][:12], zeroBytes12[:]) {
				continue
			}
			if l.Topics[1] == ownerTopic {
				add(movementSend, l.Address.Hex(), amount, common.BytesToAddress(l.Topics[2].Bytes()), evidence)
			}
			if l.Topics[2] == ownerTopic {
				add(movementReceive, l.Address.Hex(), amount, common.BytesToAddress(l.Topics[1].Bytes()), evidence)
			}
		}
		// WETH9 uses Deposit/Withdrawal rather than mint/burn Transfer events.
		if (len(chainIDs) == 0 || chainIDs[0] == SepoliaID) && l.Address == common.HexToAddress(SepoliaWETH) && len(l.Topics) == 2 && l.Topics[1] == ownerTopic {
			if l.Topics[0] == depositTopic {
				add(movementReceive, l.Address.Hex(), amount, l.Address, evidence+":deposit")
			}
			if l.Topics[0] == withdrawalTopic {
				add(movementSend, l.Address.Hex(), amount, l.Address, evidence+":withdraw")
				add(movementReceive, "ETH", amount, l.Address, evidence+":withdraw ETH")
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
		if c.ChainID() != SepoliaID {
			// Nitro blocks contain system transaction types absent from upstream geth's decoder.
			// Discover candidate hashes here; Activity verifies signed user transactions later.
			var block rpcBlock
			if err := c.rpc.Client().CallContext(ctx, &block, "eth_getBlockByNumber", hexutil.EncodeUint64(n), true); err != nil {
				return nil, 0, 0, rpcError(err)
			}
			domainBlock, err := rpcBlockToDomain(block)
			if err != nil {
				return nil, 0, 0, err
			}
			canonical, err := c.rpc.HeaderByNumber(ctx, new(big.Int).SetUint64(n))
			if err != nil || canonical.Hash() != domainBlock.hash {
				return nil, 0, 0, ErrUnavailable
			}
			for _, tx := range domainBlock.transactions {
				if tx.from == owner || (tx.to != nil && *tx.to == owner) {
					add(tx.hash)
				}
			}
			continue
		}

		b, err := c.rpc.BlockByNumber(ctx, new(big.Int).SetUint64(n))
		if err != nil {
			return nil, 0, 0, rpcError(err)
		}
		for _, tx := range b.Transactions() {
			sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(c.ChainID())), tx)
			if err != nil {
				continue
			}
			if sender == owner || (tx.To() != nil && *tx.To() == owner) {
				add(tx.Hash())
			}
		}
	}
	addresses := []common.Address{common.HexToAddress(SepoliaWETH), common.HexToAddress(SepoliaUSDC)}
	if c.ChainID() != SepoliaID {
		addresses = nil
	}
	addressSet := map[common.Address]bool{}
	for _, address := range addresses {
		addressSet[address] = true
	}
	for _, address := range contracts {
		if address != (common.Address{}) && !addressSet[address] {
			addresses = append(addresses, address)
			addressSet[address] = true
		}
	}
	if len(addresses) > 20 {
		return nil, 0, 0, errors.New("每次同步最多 20 個代幣合約")
	}
	if len(addresses) == 0 {
		return ids, from, end, nil
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
