package workflows

import (
	"fmt"
	"strconv"
	"strings"
)

var workflowDelayUnits = []string{"hour", "day", "week", "month"}

// parseDelayFlag parses a "<amount> <unit>" delay like "4 weeks" or "1 hour"
// into the API's delay_amount / delay_unit pair. Units match the server's
// InstallmentRule vocabulary (hour, day, week, month); a trailing "s" is
// accepted so natural plurals work.
func parseDelayFlag(value string) (int, string, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return 0, "", fmt.Errorf("expected \"<amount> <unit>\" (e.g. \"4 weeks\"), got %q", value)
	}

	amount, err := strconv.Atoi(fields[0])
	if err != nil || amount < 0 {
		return 0, "", fmt.Errorf("amount must be a non-negative integer, got %q", fields[0])
	}

	unit := strings.ToLower(strings.TrimSuffix(fields[1], "s"))
	for _, valid := range workflowDelayUnits {
		if unit == valid {
			return amount, unit, nil
		}
	}
	return 0, "", fmt.Errorf("unit must be one of: %s (got %q)", strings.Join(workflowDelayUnits, ", "), fields[1])
}
