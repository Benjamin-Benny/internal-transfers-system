package service

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string // expected decimal value as string
		wantError bool
		errorMsg  string // substring to check in error message
	}{
		// Valid cases
		{
			name:      "valid: zero",
			input:     "0",
			want:      "0",
			wantError: false,
		},
		{
			name:      "valid: zero with decimal",
			input:     "0.0",
			want:      "0.0",
			wantError: false,
		},
		{
			name:      "valid: one",
			input:     "1",
			want:      "1",
			wantError: false,
		},
		{
			name:      "valid: one decimal place",
			input:     "1.2",
			want:      "1.2",
			wantError: false,
		},
		{
			name:      "valid: five decimal places",
			input:     "1.23456",
			want:      "1.23456",
			wantError: false,
		},
		{
			name:      "valid: leading zeros",
			input:     "001.23000",
			want:      "1.23000",
			wantError: false,
		},
		{
			name:      "valid: with leading and trailing spaces",
			input:     " 10.00000 ",
			want:      "10.00000",
			wantError: false,
		},
		{
			name:      "valid: large number with 5 decimals",
			input:     "999999.99999",
			want:      "999999.99999",
			wantError: false,
		},
		{
			name:      "valid: negative allowed by ParseAmount",
			input:     "-1.00000",
			want:      "-1.00000",
			wantError: false,
		},
		{
			name:      "valid: negative zero",
			input:     "-0",
			want:      "0",
			wantError: false,
		},

		// Invalid cases
		{
			name:      "invalid: empty string",
			input:     "",
			wantError: true,
			errorMsg:  "amount cannot be empty",
		},
		{
			name:      "invalid: only spaces",
			input:     "   ",
			wantError: true,
			errorMsg:  "amount cannot be empty",
		},
		{
			name:      "invalid: non-numeric",
			input:     "abc",
			wantError: true,
			errorMsg:  "invalid amount format",
		},
		{
			name:      "invalid: six decimal places",
			input:     "1.234567",
			wantError: true,
			errorMsg:  "cannot have more than 5 decimal places",
		},
		{
			name:      "invalid: seven decimal places",
			input:     "1.2345678",
			wantError: true,
			errorMsg:  "cannot have more than 5 decimal places",
		},
		{
			name:      "invalid: scientific notation lowercase e",
			input:     "1e-3",
			wantError: true,
			errorMsg:  "scientific notation not allowed",
		},
		{
			name:      "invalid: scientific notation uppercase E",
			input:     "1E3",
			wantError: true,
			errorMsg:  "scientific notation not allowed",
		},
		{
			name:      "invalid: scientific notation with decimal",
			input:     "1.5e2",
			wantError: true,
			errorMsg:  "scientific notation not allowed",
		},
		{
			name:      "invalid: multiple decimal points",
			input:     "1.2.3",
			wantError: true,
			errorMsg:  "invalid amount format",
		},
		{
			name:      "invalid: special characters",
			input:     "$100.00",
			wantError: true,
			errorMsg:  "invalid amount format",
		},
		{
			name:      "invalid: comma separator",
			input:     "1,000.00",
			wantError: true,
			errorMsg:  "invalid amount format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAmount(tt.input)

			if tt.wantError {
				if err == nil {
					t.Errorf("ParseAmount(%q) expected error, got nil", tt.input)
					return
				}

				// Check that error wraps ErrInvalidInput
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("ParseAmount(%q) error should wrap ErrInvalidInput, got: %v", tt.input, err)
				}

				// Check error message contains expected substring
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("ParseAmount(%q) error = %q, want substring %q", tt.input, err.Error(), tt.errorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseAmount(%q) unexpected error: %v", tt.input, err)
				return
			}

			// Compare decimal values
			want, err := decimal.NewFromString(tt.want)
			if err != nil {
				t.Fatalf("test setup error: invalid want value %q: %v", tt.want, err)
			}

			if !got.Equal(want) {
				t.Errorf("ParseAmount(%q) = %v, want %v", tt.input, got, want)
			}
		})
	}
}

// TestParseAmount_EdgeCases tests additional edge cases
func TestParseAmount_EdgeCases(t *testing.T) {
	t.Run("exactly 5 decimal places should pass", func(t *testing.T) {
		inputs := []string{"0.12345", "123.45678", "999.99999"}
		for _, input := range inputs {
			_, err := ParseAmount(input)
			if err != nil {
				t.Errorf("ParseAmount(%q) with exactly 5 decimals should pass, got error: %v", input, err)
			}
		}
	})

	t.Run("more than 5 decimal places should fail", func(t *testing.T) {
		inputs := []string{"0.123456", "1.1234567", "99.999999"}
		for _, input := range inputs {
			_, err := ParseAmount(input)
			if err == nil {
				t.Errorf("ParseAmount(%q) with >5 decimals should fail, got nil error", input)
			}
		}
	})

	t.Run("trailing zeros count as decimal places", func(t *testing.T) {
		// "1.00000" has 5 decimal places - should pass
		_, err := ParseAmount("1.00000")
		if err != nil {
			t.Errorf("ParseAmount(\"1.00000\") should pass, got error: %v", err)
		}

		// "1.000000" has 6 decimal places - should fail
		_, err = ParseAmount("1.000000")
		if err == nil {
			t.Errorf("ParseAmount(\"1.000000\") should fail, got nil error")
		}
	})
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
