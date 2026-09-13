package wallet

import (
	"errors"
	"math/big"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

var (
	decimalRegex = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
	maxUint256   = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

// ValidateAddress checks that an Ethereum address is 42 hex characters, not zero address,
// and if mixed-case conforms to EIP-55 checksum.
func ValidateAddress(addr string) (common.Address, error) {
	if len(addr) != 42 || !strings.HasPrefix(addr, "0x") {
		return common.Address{}, ErrInvalidAddress
	}
	hexPart := addr[2:]
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return common.Address{}, ErrInvalidAddress
		}
	}
	if addr == "0x0000000000000000000000000000000000000000" {
		return common.Address{}, ErrZeroAddress
	}
	isLower := hexPart == strings.ToLower(hexPart)
	isUpper := hexPart == strings.ToUpper(hexPart)
	if !isLower && !isUpper {
		checksummed := common.HexToAddress(addr).Hex()
		if addr != checksummed {
			return common.Address{}, ErrMalformedChecksum
		}
	}
	return common.HexToAddress(addr), nil
}

// ParseUnits parses a decimal string amount into integer units according to the specified decimals.
// Uses integer math exclusively, rejecting scientific notation, negative numbers, and uint256 overflow.
func ParseUnits(amountStr string, decimals int) (*big.Int, error) {
	if decimals < 0 || decimals > 36 {
		return nil, ErrDecimalsTooLarge
	}
	if !decimalRegex.MatchString(amountStr) {
		return nil, errors.New("金額格式不正確，僅支援無正負號的十進位數字，不可包含科學記號或非數字字元")
	}
	parts := strings.Split(amountStr, ".")
	wholeStr := parts[0]
	var fracStr string
	if len(parts) == 2 {
		fracStr = parts[1]
	}
	if len(fracStr) > decimals {
		return nil, errors.New("金額小數位數超出該資產支援的上限")
	}
	fracStrPadded := fracStr + strings.Repeat("0", decimals-len(fracStr))
	combined := strings.TrimLeft(wholeStr+fracStrPadded, "0")
	if combined == "" {
		return big.NewInt(0), nil
	}
	val, ok := new(big.Int).SetString(combined, 10)
	if !ok {
		return nil, errors.New("金額數值解析失敗")
	}
	if val.Cmp(maxUint256) > 0 {
		return nil, errors.New("金額超出 uint256 上限")
	}
	return val, nil
}

// FormatUnits converts raw integer units to a human-readable decimal string without floating point operations.
func FormatUnits(raw *big.Int, decimals int) string {
	if raw == nil || raw.Sign() == 0 {
		return "0"
	}
	if decimals == 0 {
		return raw.String()
	}
	tenPower := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int)
	frac := new(big.Int)
	whole.QuoRem(raw, tenPower, frac)
	if frac.Sign() == 0 {
		return whole.String()
	}
	digits := frac.String()
	padded := strings.Repeat("0", decimals-len(digits)) + digits
	trimmed := strings.TrimRight(padded, "0")
	return whole.String() + "." + trimmed
}
