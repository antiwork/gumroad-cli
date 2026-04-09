package cmdutil

import (
	"strings"
	"testing"
)

func TestParseMoney(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		value    string
		noun     string
		currency string
		want     int
		wantErr  string
	}{
		{name: "whole number", flag: "price", value: "10", noun: "price", want: 1000},
		{name: "two decimals", flag: "price", value: "10.00", noun: "price", want: 1000},
		{name: "cents", flag: "price", value: "9.99", noun: "price", want: 999},
		{name: "one decimal", flag: "price", value: "5.5", noun: "price", want: 550},
		{name: "zero", flag: "price", value: "0", noun: "price", want: 0},
		{name: "zero decimal", flag: "price", value: "0.00", noun: "price", want: 0},
		{name: "large", flag: "price", value: "999.99", noun: "price", want: 99999},
		{name: "negative", flag: "price", value: "-10", noun: "price", wantErr: "cannot be negative"},
		{name: "three decimals", flag: "price", value: "10.999", noun: "price", wantErr: "too many decimal places"},
		{name: "letters", flag: "price", value: "abc", noun: "price", wantErr: "not a valid price"},
		{name: "empty", flag: "price", value: "", noun: "price", wantErr: "cannot be empty"},
		{name: "dollar sign", flag: "price", value: "$10", noun: "price", wantErr: "not a valid price"},
		{name: "comma", flag: "price", value: "1,000", noun: "price", wantErr: "not a valid price"},
		{name: "just dot", flag: "price", value: "10.", noun: "price", wantErr: "not a valid price"},

		// Currency-aware tests
		{name: "usd explicit", flag: "price", value: "10", noun: "price", currency: "usd", want: 1000},
		{name: "eur with cents", flag: "price", value: "10.99", noun: "price", currency: "eur", want: 1099},
		{name: "jpy whole", flag: "price", value: "1000", noun: "price", currency: "jpy", want: 1000},
		{name: "jpy uppercase", flag: "price", value: "1000", noun: "price", currency: "JPY", want: 1000},
		{name: "jpy rejects decimals", flag: "price", value: "10.99", noun: "price", currency: "jpy", wantErr: "JPY amounts cannot have decimal places"},
		{name: "jpy rejects single decimal", flag: "price", value: "10.5", noun: "price", currency: "jpy", wantErr: "JPY amounts cannot have decimal places"},
		{name: "empty currency defaults x100", flag: "price", value: "10", noun: "price", currency: "", want: 1000},

		// Overflow protection
		{name: "very large value", flag: "price", value: "99999999999999", noun: "price", wantErr: "too large"},

		// Noun in error messages
		{name: "noun in error", flag: "amount", value: "abc", noun: "amount", wantErr: "not a valid amount"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMoney(tt.flag, tt.value, tt.noun, tt.currency)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseSignedMoney(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		value    string
		noun     string
		currency string
		want     int
		wantErr  string
	}{
		{name: "positive whole", flag: "price-difference", value: "10", noun: "price", want: 1000},
		{name: "positive decimal", flag: "price-difference", value: "5.50", noun: "price", want: 550},
		{name: "negative whole", flag: "price-difference", value: "-10", noun: "price", want: -1000},
		{name: "negative decimal", flag: "price-difference", value: "-1.50", noun: "price", want: -150},
		{name: "negative zero", flag: "price-difference", value: "-0", noun: "price", want: 0},
		{name: "zero", flag: "price-difference", value: "0", noun: "price", want: 0},
		{name: "letters", flag: "price-difference", value: "abc", noun: "price", wantErr: "not a valid price"},
		{name: "three decimals", flag: "price-difference", value: "-10.999", noun: "price", wantErr: "too many decimal places"},

		// JPY signed
		{name: "jpy positive", flag: "price-difference", value: "500", noun: "price", currency: "jpy", want: 500},
		{name: "jpy negative", flag: "price-difference", value: "-500", noun: "price", currency: "jpy", want: -500},
		{name: "jpy rejects decimals", flag: "price-difference", value: "-10.5", noun: "price", currency: "jpy", wantErr: "JPY amounts cannot have decimal places"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSignedMoney(tt.flag, tt.value, tt.noun, tt.currency)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFormatMoney(t *testing.T) {
	tests := []struct {
		name     string
		cents    int
		currency string
		want     string
	}{
		{name: "whole dollars", cents: 1000, want: "10.00"},
		{name: "with cents", cents: 1099, want: "10.99"},
		{name: "zero", cents: 0, want: "0.00"},
		{name: "sub-dollar", cents: 50, want: "0.50"},
		{name: "negative", cents: -150, want: "-1.50"},
		{name: "jpy", cents: 1000, currency: "jpy", want: "1000"},
		{name: "jpy negative", cents: -500, currency: "jpy", want: "-500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMoney(tt.cents, tt.currency)
			if got != tt.want {
				t.Fatalf("FormatMoney(%d, %q) = %q, want %q", tt.cents, tt.currency, got, tt.want)
			}
		})
	}
}

func TestIsSingleUnitCurrency(t *testing.T) {
	tests := []struct {
		currency string
		want     bool
	}{
		{"jpy", true},
		{"JPY", true},
		{"Jpy", true},
		{"usd", false},
		{"eur", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.currency, func(t *testing.T) {
			if got := IsSingleUnitCurrency(tt.currency); got != tt.want {
				t.Fatalf("IsSingleUnitCurrency(%q) = %v, want %v", tt.currency, got, tt.want)
			}
		})
	}
}
