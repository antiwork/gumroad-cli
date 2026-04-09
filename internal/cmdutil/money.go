package cmdutil

import (
	"fmt"
	"strconv"
	"strings"
)

// singleUnitCurrencies are currencies where 1 unit = 1 minor unit (no cents).
// Source: config/currencies.json in the Gumroad codebase.
var singleUnitCurrencies = map[string]bool{
	"jpy": true,
}

// IsSingleUnitCurrency reports whether the given currency code has no
// minor unit (e.g. JPY where ¥1 = 1 minor unit).
func IsSingleUnitCurrency(currency string) bool {
	return singleUnitCurrencies[strings.ToLower(currency)]
}

// scalingFactor returns the multiplier to convert user-facing amounts to
// API minor units: 1 for single-unit currencies, 100 otherwise.
func scalingFactor(currency string) int {
	if IsSingleUnitCurrency(currency) {
		return 1
	}
	return 100
}

// ParseMoney converts a user-friendly price string to minor units using
// integer math (no floats). The scaling factor depends on the currency:
// ×100 for most currencies, ×1 for single-unit currencies like JPY.
//
// If currency is empty, defaults to ×100 (standard behavior).
// Rejects negative values — use ParseSignedMoney for those.
func ParseMoney(flag, value, noun, currency string) (int, error) {
	cents, err := parseMoney(flag, value, noun, currency)
	if err != nil {
		return 0, err
	}
	if cents < 0 {
		return 0, fmt.Errorf("--%s cannot be negative", flag)
	}
	return cents, nil
}

// ParseSignedMoney is like ParseMoney but allows negative values.
func ParseSignedMoney(flag, value, noun, currency string) (int, error) {
	return parseMoney(flag, value, noun, currency)
}


// FormatMoney converts a minor-unit amount back to a user-friendly string
// (e.g. 1099 → "10.99", 1000 → "10.00", -150 → "-1.50").
// For single-unit currencies like JPY, returns the amount as-is (e.g. 1000 → "1000").
func FormatMoney(cents int, currency string) string {
	if IsSingleUnitCurrency(currency) {
		return strconv.Itoa(cents)
	}
	negative := cents < 0
	if negative {
		cents = -cents
	}
	s := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if negative {
		return "-" + s
	}
	return s
}

func parseMoney(flag, value, noun, currency string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("--%s cannot be empty", flag)
	}

	negative := false
	s := value
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}

	factor := scalingFactor(currency)
	singleUnit := factor == 1

	parts := strings.SplitN(s, ".", 2)

	// Reject unreasonably large values that could overflow int arithmetic.
	// Count only digits, excluding the decimal point.
	const maxDigits = 12
	digitCount := len(parts[0])
	if len(parts) == 2 {
		digitCount += len(parts[1])
	}
	if digitCount > maxDigits {
		return 0, fmt.Errorf("--%s value is too large", flag)
	}
	if len(parts) == 1 {
		whole, ok := parseDigits(parts[0])
		if !ok {
			return 0, fmt.Errorf("%q is not a valid %s", value, noun)
		}
		result := whole * factor
		if negative {
			result = -result
		}
		return result, nil
	}

	if singleUnit {
		return 0, fmt.Errorf("--%s: JPY amounts cannot have decimal places", flag)
	}

	wholePart := parts[0]
	fracPart := parts[1]

	if len(fracPart) > 2 {
		return 0, fmt.Errorf("--%s has too many decimal places (max 2)", flag)
	}
	if len(fracPart) == 0 {
		return 0, fmt.Errorf("%q is not a valid %s", value, noun)
	}

	whole, ok := parseDigits(wholePart)
	if !ok {
		if wholePart != "" {
			return 0, fmt.Errorf("%q is not a valid %s", value, noun)
		}
	}

	frac, ok := parseDigits(fracPart)
	if !ok {
		return 0, fmt.Errorf("%q is not a valid %s", value, noun)
	}

	if len(fracPart) == 1 {
		frac *= 10
	}

	result := whole*100 + frac
	if negative {
		result = -result
	}
	return result, nil
}

// parseDigits parses a string of digits into a non-negative integer.
// Returns false for empty strings or non-digit input.
func parseDigits(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	if n < 0 {
		return 0, false
	}
	return n, true
}
