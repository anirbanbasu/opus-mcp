package server

import (
	"context"
	"encoding/json"
	"testing"
)

// TestFetchCategoryTaxonomy tests the taxonomy fetcher against the real arXiv website.
// This validates both the HTTP fetching and HTML parsing logic work correctly.
func TestFetchCategoryTaxonomy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode - requires network access")
	}

	ctx := context.Background()
	input := json.RawMessage(`{}`)

	result, err := fetchCategoryTaxonomy(ctx, input)
	if err != nil {
		t.Fatalf("failed to fetch taxonomy: %v", err)
	}

	taxonomy, ok := result.(Taxonomy)
	if !ok {
		t.Fatalf("expected Taxonomy, got %T", result)
	}

	// Verify we got reasonable data
	if len(taxonomy.Groups) == 0 {
		t.Fatal("taxonomy has no groups")
	}
	if len(taxonomy.Categories) == 0 {
		t.Fatal("taxonomy has no categories")
	}

	// Test structure: each category should have valid data
	for catCode, category := range taxonomy.Categories {
		// Each category should have a non-empty name
		if category.Name == "" {
			t.Errorf("category %s has empty name", catCode)
		}

		// Category code should match the key
		if category.Code != catCode {
			t.Errorf("category %s has mismatched code %s", catCode, category.Code)
		}

		// Verify category code references a valid group by extracting group code
		groupCode := deriveGroupCode(catCode)
		if _, ok := taxonomy.Groups[groupCode]; !ok {
			t.Errorf("category %s has group prefix %s which doesn't exist in groups", catCode, groupCode)
		}
	}

	// Test groups structure
	for groupCode, group := range taxonomy.Groups {
		// Each group should have a non-empty name
		if group.Name == "" {
			t.Errorf("group %s has empty name", groupCode)
		}

		// Group code should match the key
		if group.Code != groupCode {
			t.Errorf("group %s has mismatched code %s", groupCode, group.Code)
		}
	}

	// Verify expected major groups exist
	expectedGroups := []string{"cs", "math", "physics", "q-bio", "q-fin", "stat"}
	for _, groupCode := range expectedGroups {
		if _, ok := taxonomy.Groups[groupCode]; !ok {
			t.Errorf("expected group %q not found", groupCode)
		}
	}

	// Verify minimum category counts
	minCategories := 100 // Should have at least 100 total categories
	if len(taxonomy.Categories) < minCategories {
		t.Errorf("expected at least %d categories, got %d", minCategories, len(taxonomy.Categories))
	}

	// Verify specific well-known categories exist with correct structure
	wellKnownCategories := map[string]string{
		"cs.AI":          "cs",
		"cs.LG":          "cs",
		"math.AG":        "math",
		"physics.optics": "physics",
		"q-bio.BM":       "q-bio",
		"stat.ML":        "stat",
	}

	for catCode, expectedGroup := range wellKnownCategories {
		category, ok := taxonomy.Categories[catCode]
		if !ok {
			t.Errorf("expected category %q not found", catCode)
			continue
		}
		// Verify the category code prefix matches expected group
		groupCode := deriveGroupCode(catCode)
		if groupCode != expectedGroup {
			t.Errorf("category %q has group prefix %q, expected %q", catCode, groupCode, expectedGroup)
		}
		if category.Name == "" {
			t.Errorf("category %q has empty name", catCode)
		}
	}

	// Log summary for visibility
	groupsWord := "groups"
	if len(taxonomy.Groups) == 1 {
		groupsWord = "group"
	}
	categoriesWord := "categories"
	if len(taxonomy.Categories) == 1 {
		categoriesWord = "category"
	}
	t.Logf("Successfully fetched taxonomy with %d %s and %d %s", len(taxonomy.Groups), groupsWord, len(taxonomy.Categories), categoriesWord)

	// Count categories per group
	groupCounts := make(map[string]int)
	for catCode := range taxonomy.Categories {
		groupCode := deriveGroupCode(catCode)
		groupCounts[groupCode]++
	}
	for groupCode, count := range groupCounts {
		if group, ok := taxonomy.Groups[groupCode]; ok {
			categoriesWord = "categories"
			if count == 1 {
				categoriesWord = "category"
			}
			t.Logf("  %s (%s): %d %s", groupCode, group.Name, count, categoriesWord)
		}
	}
}
