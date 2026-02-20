package service

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ParseAmount parses a string amount and validates it according to business rules:
// - Trims leading/trailing spaces
// - Rejects empty strings
// - Rejects scientific notation (e/E) to keep contract simple
// - Validates max 5 decimal places based on the original string after trimming
// Returns ErrInvalidInput wrapped with context on validation failure
func ParseAmount(amountStr string) (decimal.Decimal, error) {
	// Trim spaces
	trimmed := strings.TrimSpace(amountStr)

	// Reject empty string
	if trimmed == "" {
		return decimal.Zero, fmt.Errorf("%w: amount cannot be empty", ErrInvalidInput)
	}

	// Reject scientific notation (e or E)
	if strings.ContainsAny(trimmed, "eE") {
		return decimal.Zero, fmt.Errorf("%w: scientific notation not allowed", ErrInvalidInput)
	}

	// Parse the amount
	amount, err := decimal.NewFromString(trimmed)
	if err != nil {
		return decimal.Zero, fmt.Errorf("%w: invalid amount format", ErrInvalidInput)
	}

	// Check decimal places (scale) in the original trimmed string
	// Count decimal places to catch cases like "10.123456"
	if idx := strings.Index(trimmed, "."); idx != -1 {
		decimalPlaces := len(trimmed) - idx - 1
		if decimalPlaces > 5 {
			return decimal.Zero, fmt.Errorf("%w: amount cannot have more than 5 decimal places", ErrInvalidInput)
		}
	}

	return amount, nil
}
