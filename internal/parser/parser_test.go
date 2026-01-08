package parser

import (
	"testing"
)

func TestReconstructCategoryExpressions(t *testing.T) {
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

func TestReconstructGeneralExpressions(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		prefix    string
		suffix    string
		want      string
		wantError bool
	}{
		// --- Basic Operators with prefix/suffix ---
		{"Single term with prefix/suffix", "a", "pre1-", "-suf1", "(pre1-a-suf1)", false},
		{"Simple AND", "a and b", "abc-", "-xyz", "(abc-a-xyz+AND+abc-b-xyz)", false},
		{"Simple OR", "a OR b", "test-", "-end", "(test-a-end+OR+test-b-end)", false},
		{"Simple NOT", "noT a", "pfx2-", "-sfx2", "(NOT+pfx2-a-sfx2)", false},

		// --- Implicit AND Logic with different prefix/suffix ---
		{"Implicit AND two terms", "a b", "ab12-", "-cd34", "(ab12-a-cd34+AND+ab12-b-cd34)", false},
		{"Implicit AND three terms", "a b c", "pr-", "-sx", "(pr-a-sx+AND+pr-b-sx+AND+pr-c-sx)", false},
		{"Implicit AND with group", "a (b c)", "tag1-", "-end1", "(tag1-a-end1+AND+(tag1-b-end1+AND+tag1-c-end1))", false},
		{"Implicit AND after group", "(a b) c", "pre-", "-suf", "((pre-a-suf+AND+pre-b-suf)+AND+pre-c-suf)", false},

		// --- Complex Nesting with various prefix/suffix ---
		{"Original example", "a and b not (c or d)", "ns1-", "-ns2", "(ns1-a-ns2+AND+ns1-b-ns2+NOT+(ns1-c-ns2+OR+ns1-d-ns2))", false},
		{"Deeply nested", "((a b) OR (c d))", "deep-", "-nest", "(((deep-a-nest+AND+deep-b-nest)+OR+(deep-c-nest+AND+deep-d-nest)))", false},
		{"Nested with explicit ops", "a AND (b OR c) AND NOT d", "op12-", "-op34", "(op12-a-op34+AND+(op12-b-op34+OR+op12-c-op34)+AND+NOT+op12-d-op34)", false},

		// --- Edge Cases with prefix/suffix ---
		{"Empty prefix and suffix", "a b", "", "", "(a+AND+b)", false},
		{"Only prefix", "a OR b", "only-", "", "(only-a+OR+only-b)", false},
		{"Only suffix", "a AND b", "", "-only", "(a-only+AND+b-only)", false},
		{"Longer prefix/suffix", "x y", "prefix-", "-suffix", "(prefix-x-suffix+AND+prefix-y-suffix)", false},

		// --- Whitespace & Case with prefix/suffix ---
		{"Mixed casing", "A and B OR c", "mx1-", "-mx2", "(mx1-A-mx2+AND+mx1-B-mx2+OR+mx1-c-mx2)", false},
		{"Irregular spacing", "  a    b   ", "space-", "-test", "(space-a-test+AND+space-b-test)", false},

		// --- With short hands of boolean operators ---
		{"With shorthand operators", "a + b - c | d", "sh12-", "-sh34", "(sh12-a-sh34+AND+sh12-b-sh34+NOT+sh12-c-sh34+OR+sh12-d-sh34)", false},
		{"Complex with comparison", "a > b <= c", "cmp-", "-val", "(cmp-a-val+AND+cmp->-val+AND+cmp-b-val+AND+cmp-<=-val+AND+cmp-c-val)", false},

		// --- Numeric and special character terms ---
		{"Numeric identifiers", "123 456", "num-", "-id", "(num-123-id+AND+num-456-id)", false},
		{"Terms with dots", "a.b c.d", "dot-", "-sep", "(dot-a.b-sep+AND+dot-c.d-sep)", false},
		{"Special chars in prefix/suffix", "a b", "sp1-", "-sp2", "(sp1-a-sp2+AND+sp1-b-sp2)", false},

		// --- Error cases ---
		{"Empty string", "", "pre-", "-suf", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseReconstructGeneralExpression(tt.input, tt.prefix, tt.suffix)
			if tt.wantError {
				if err == nil {
					t.Errorf("\nInput: %q (prefix=%q, suffix=%q)\nExpected error but got none", tt.input, tt.prefix, tt.suffix)
				}
				return
			}
			if err != nil {
				t.Errorf("\nInput: %q (prefix=%q, suffix=%q)\nUnexpected error: %v", tt.input, tt.prefix, tt.suffix, err)
				return
			}
			if got != tt.want {
				t.Errorf("\nInput: %q (prefix=%q, suffix=%q)\nGot:   %q\nWant:  %q", tt.input, tt.prefix, tt.suffix, got, tt.want)
			}
		})
	}
}
