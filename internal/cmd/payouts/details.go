package payouts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/antiwork/gumroad-cli/internal/output"
)

func payoutPlainDetailColumns(noSales, includeTransactions bool, transactions json.RawMessage) []string {
	if !noSales && !includeTransactions {
		return nil
	}

	columns := make([]string, 0, 2)
	if noSales {
		columns = append(columns, "omitted")
	}
	if includeTransactions {
		columns = append(columns, payoutTransactionCell(transactions))
	}
	return columns
}

func writePayoutDetailLines(w io.Writer, noSales, includeTransactions bool, transactions json.RawMessage) error {
	if noSales {
		if err := output.Writeln(w, "Sales: omitted (--no-sales)"); err != nil {
			return err
		}
	}
	if includeTransactions {
		if err := output.Writeln(w, "Transactions: "+payoutTransactionSummary(transactions)); err != nil {
			return err
		}
	}
	return nil
}

func payoutTransactionCell(transactions json.RawMessage) string {
	count, ok := jsonArrayCount(transactions)
	if !ok {
		return "included"
	}
	return fmt.Sprintf("%d", count)
}

func payoutTransactionSummary(transactions json.RawMessage) string {
	count, ok := jsonArrayCount(transactions)
	if !ok {
		return "included"
	}
	return fmt.Sprintf("%d included", count)
}

func jsonArrayCount(raw json.RawMessage) (int, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, false
	}

	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return 0, false
	}
	return len(items), true
}
