package wallet

import (
	"errors"
	"time"

	sol "github.com/gagliardetto/solana-go"
)

type solanaSignature string
type solanaAddress string
type tronTransactionID string
type tronAddress string
type crosschainState string

func parseCrosschainState(value string) (crosschainState, error) {
	switch value {
	case "broadcast_unknown", "submitted", "processed", "confirmed", "finalized", "execution_failed", "expired_unconfirmed", "reverted":
		return crosschainState(value), nil
	default:
		return "", errors.New("跨鏈交易日誌狀態無效")
	}
}

type solanaJournalRecord struct {
	Signature solanaSignature
	QuoteID   QuoteID
	To        solanaAddress
	Amount    string
	State     crosschainState
	Finalized bool
	LastValid uint64
	CreatedAt time.Time
	SignedRaw string
}

func solanaRecordFromDisk(stored solanaDiskRecord) (solanaJournalRecord, error) {
	state, err := parseCrosschainState(stored.State)
	if err != nil {
		return solanaJournalRecord{}, err
	}
	quoteID, err := ParseLegacyQuoteID(stored.QuoteID)
	if err != nil {
		return solanaJournalRecord{}, err
	}
	if _, err := sol.SignatureFromBase58(stored.Signature); err != nil {
		return solanaJournalRecord{}, err
	}
	if stored.To != "" {
		if _, err := sol.PublicKeyFromBase58(stored.To); err != nil {
			return solanaJournalRecord{}, err
		}
	}
	return solanaJournalRecord{
		Signature: solanaSignature(stored.Signature),
		QuoteID:   quoteID,
		To:        solanaAddress(stored.To),
		Amount:    stored.Amount,
		State:     state,
		Finalized: stored.Finalized,
		LastValid: stored.LastValid,
		CreatedAt: stored.CreatedAt,
		SignedRaw: stored.SignedRaw,
	}, nil
}

func solanaRecordToDisk(record solanaJournalRecord) solanaDiskRecord {
	return solanaDiskRecord{
		Signature: string(record.Signature),
		QuoteID:   string(record.QuoteID),
		To:        string(record.To),
		Amount:    record.Amount,
		State:     string(record.State),
		Finalized: record.Finalized,
		LastValid: record.LastValid,
		CreatedAt: record.CreatedAt,
		SignedRaw: record.SignedRaw,
	}
}

func solanaRecordResponse(record solanaJournalRecord) SolanaRecord {
	return SolanaRecord{
		Signature: string(record.Signature),
		QuoteID:   string(record.QuoteID),
		To:        string(record.To),
		Amount:    record.Amount,
		State:     string(record.State),
		Finalized: record.Finalized,
		LastValid: record.LastValid,
		CreatedAt: record.CreatedAt,
	}
}

type tronJournalRecord struct {
	Reused             bool
	Signature          tronTransactionID
	QuoteID            QuoteID
	From               tronAddress
	To                 tronAddress
	Contract           tronAddress
	Symbol             string
	Amount             string
	State              crosschainState
	Finalized          bool
	FeeTRX             string
	ExpiryCheckedBlock string
	ExpiryCheckedAt    int64
	Result             string
	CreatedAt          time.Time
	Expires            time.Time
	Raw                string
	Signed             string
}

func tronRecordFromDisk(stored tronDiskRecord) (tronJournalRecord, error) {
	state, err := parseCrosschainState(stored.State)
	if err != nil {
		return tronJournalRecord{}, err
	}
	quoteID, err := ParseLegacyQuoteID(stored.QuoteID)
	if err != nil {
		return tronJournalRecord{}, err
	}
	if !validTronHash(stored.Signature) {
		return tronJournalRecord{}, errors.New("TRON 交易 ID 格式錯誤")
	}
	for _, address := range []string{stored.From, stored.To, stored.Contract} {
		if address != "" {
			if _, err := ParseTronAddress(address); err != nil {
				return tronJournalRecord{}, err
			}
		}
	}
	return tronJournalRecord{
		Reused:             stored.Reused,
		Signature:          tronTransactionID(stored.Signature),
		QuoteID:            quoteID,
		From:               tronAddress(stored.From),
		To:                 tronAddress(stored.To),
		Contract:           tronAddress(stored.Contract),
		Symbol:             stored.Symbol,
		Amount:             stored.Amount,
		State:              state,
		Finalized:          stored.Finalized,
		FeeTRX:             stored.FeeTRX,
		ExpiryCheckedBlock: stored.ExpiryCheckedBlock,
		ExpiryCheckedAt:    stored.ExpiryCheckedAt,
		Result:             stored.Result,
		CreatedAt:          stored.CreatedAt,
		Expires:            stored.Expires,
		Raw:                stored.Raw,
		Signed:             stored.Signed,
	}, nil
}

func tronRecordToDisk(record tronJournalRecord) tronDiskRecord {
	return tronDiskRecord{
		Reused:             record.Reused,
		Signature:          string(record.Signature),
		QuoteID:            string(record.QuoteID),
		From:               string(record.From),
		To:                 string(record.To),
		Contract:           string(record.Contract),
		Symbol:             record.Symbol,
		Amount:             record.Amount,
		State:              string(record.State),
		Finalized:          record.Finalized,
		FeeTRX:             record.FeeTRX,
		ExpiryCheckedBlock: record.ExpiryCheckedBlock,
		ExpiryCheckedAt:    record.ExpiryCheckedAt,
		Result:             record.Result,
		CreatedAt:          record.CreatedAt,
		Expires:            record.Expires,
		Raw:                record.Raw,
		Signed:             record.Signed,
	}
}

func tronRecordResponse(record tronJournalRecord) TronRecord {
	return TronRecord{
		Reused:             record.Reused,
		Signature:          string(record.Signature),
		QuoteID:            string(record.QuoteID),
		From:               string(record.From),
		To:                 string(record.To),
		Contract:           string(record.Contract),
		Symbol:             record.Symbol,
		Amount:             record.Amount,
		State:              string(record.State),
		Finalized:          record.Finalized,
		FeeTRX:             record.FeeTRX,
		ExpiryCheckedBlock: record.ExpiryCheckedBlock,
		ExpiryCheckedAt:    record.ExpiryCheckedAt,
		Result:             record.Result,
		CreatedAt:          record.CreatedAt,
		Expires:            record.Expires,
	}
}
