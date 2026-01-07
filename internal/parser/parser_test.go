package parser

import (
	"testing"
)

func TestReconstruct(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// --- Basic Operators ---
		{"Single term", "a", "(cat:a)"},
		{"Simple AND (lowercase)", "a and b", "(cat:a AND cat:b)"},
		{"Simple OR (uppercase)", "a OR b", "(cat:a OR cat:b)"},
		{"Simple NOT (mixed case)", "noT a", "(NOT cat:a)"},

		// --- Implicit AND Logic ---
		{"Implicit AND two terms", "a b", "(cat:a AND cat:b)"},
		{"Implicit AND three terms", "a b c", "(cat:a AND cat:b AND cat:c)"},
		{"Implicit AND with group", "a (b c)", "(cat:a AND (cat:b AND cat:c))"},
		{"Implicit AND after group", "(a b) c", "((cat:a AND cat:b) AND cat:c)"},
		{"Implicit AND with NOT", "a NOT b", "(cat:a NOT cat:b)"},

		// --- Complex Nesting ---
		{"Original example", "a and b not (c or d)", "(cat:a AND cat:b NOT (cat:c OR cat:d))"},
		{"Deeply nested", "((a b) OR (c d))", "(((cat:a AND cat:b) OR (cat:c AND cat:d)))"},
		{"Nested with explicit ops", "a AND (b OR c) AND NOT d", "(cat:a AND (cat:b OR cat:c) AND NOT cat:d)"},

		// --- Whitespace & Case Sensitivity ---
		{"Mixed casing", "A and B OR c", "(cat:A AND cat:B OR cat:c)"},
		{"Irregular spacing", "  a    b   ( c   d )  ", "(cat:a AND cat:b AND (cat:c AND cat:d))"},
		{"Tabs and Newlines", "a\tb\nc", "(cat:a AND cat:b AND cat:c)"},

		// --- Edge Cases ---
		{"Empty string", "", "()"},
		{"Only operators", "AND OR NOT", "(AND OR NOT)"},
		{"Unbalanced parentheses (open)", "(a b", "((cat:a AND cat:b))"},
		{"Unbalanced parentheses (close)", "a b)", "(cat:a AND cat:b)"},

		// --- with short hands of boolean operators ---
		{"With non-boolean operators", "a + b - c | d", "(cat:a AND cat:b NOT cat:c OR cat:d)"},
		{"With comparison operators", "a > b <= c", "(cat:a AND cat:> AND cat:b AND cat:<= AND cat:c)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseReconstructCategoryExpression(tt.input)
			if got != tt.want {
				t.Errorf("\nInput: %q\nGot:   %q\nWant:  %q", tt.input, got, tt.want)
			}
		})
	}
}
