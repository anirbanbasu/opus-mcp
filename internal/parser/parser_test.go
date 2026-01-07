package parser

import (
	"testing"
)

func TestReconstruct(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		// --- Basic Operators ---
		{"Single term", "a", "(cat:a)", false},
		{"Simple AND (lowercase)", "a and b", "(cat:a+AND+cat:b)", false},
		{"Simple OR (uppercase)", "a OR b", "(cat:a+OR+cat:b)", false},
		{"Simple NOT (mixed case)", "noT a", "(NOT+cat:a)", false},

		// --- Implicit AND Logic ---
		{"Implicit AND two terms", "a b", "(cat:a+AND+cat:b)", false},
		{"Implicit AND three terms", "a b c", "(cat:a+AND+cat:b+AND+cat:c)", false},
		{"Implicit AND with group", "a (b c)", "(cat:a+AND+(cat:b+AND+cat:c))", false},
		{"Implicit AND after group", "(a b) c", "((cat:a+AND+cat:b)+AND+cat:c)", false},
		{"Implicit AND with NOT", "a NOT b", "(cat:a+NOT+cat:b)", false},

		// --- Complex Nesting ---
		{"Original example", "a and b not (c or d)", "(cat:a+AND+cat:b+NOT+(cat:c+OR+cat:d))", false},
		{"Deeply nested", "((a b) OR (c d))", "(((cat:a+AND+cat:b)+OR+(cat:c+AND+cat:d)))", false},
		{"Nested with explicit ops", "a AND (b OR c) AND NOT d", "(cat:a+AND+(cat:b+OR+cat:c)+AND+NOT+cat:d)", false},

		// --- Whitespace & Case Sensitivity ---
		{"Mixed casing", "A and B OR c", "(cat:A+AND+cat:B+OR+cat:c)", false},
		{"Irregular spacing", "  a    b   ( c   d )  ", "(cat:a+AND+cat:b+AND+(cat:c+AND+cat:d))", false},
		{"Tabs and Newlines", "a\tb\nc", "(cat:a+AND+cat:b+AND+cat:c)", false},

		// --- Edge Cases ---
		{"Empty string", "", "", true},
		{"Only operators", "AND OR NOT", "(AND+OR+NOT)", false},
		{"Unbalanced parentheses (open)", "(a b", "((cat:a+AND+cat:b))", false},
		{"Unbalanced parentheses (close)", "a b)", "(cat:a+AND+cat:b)", false},

		// --- with short hands of boolean operators ---
		{"With non-boolean operators", "a + b - c | d", "(cat:a+AND+cat:b+NOT+cat:c+OR+cat:d)", false},
		{"With comparison operators", "a > b <= c", "(cat:a+AND+cat:>+AND+cat:b+AND+cat:<=+AND+cat:c)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseReconstructCategoryExpression(tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("\nInput: %q\nExpected error but got none", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("\nInput: %q\nUnexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("\nInput: %q\nGot:   %q\nWant:  %q", tt.input, got, tt.want)
			}
		})
	}
}
