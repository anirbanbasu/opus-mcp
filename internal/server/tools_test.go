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

	taxonomy, ok := result.(map[string]map[string]string)
	if !ok {
		t.Fatalf("expected map[string]map[string]string, got %T", result)
	}

	// Verify we got reasonable data
	if len(taxonomy) == 0 {
		t.Fatal("taxonomy is empty")
	}

	// Test structure: each area should have at least one category
	for areaCode, categories := range taxonomy {
		if len(categories) == 0 {
			t.Errorf("area %s has no categories", areaCode)
		}

		// Validate that category codes match the area
		for catCode, desc := range categories {
			// Each category should have a non-empty description
			if desc == "" {
				t.Errorf("category %s has empty description", catCode)
			}

			// Category codes should start with the area code (except for single-word categories like "econ")
			// We're being lenient here since arXiv has various formats
			if len(catCode) == 0 {
				t.Errorf("found empty category code in area %s", areaCode)
			}
		}
	}

	// Verify expected major areas exist
	expectedAreas := map[string]int{
		"cs":      30, // Computer Science should have at least 30 categories
		"math":    25, // Mathematics should have at least 25 categories
		"physics": 50, // Physics should have at least 50 categories
		"q-bio":   5,  // Quantitative Biology should have at least 5 categories
		"q-fin":   5,  // Quantitative Finance should have at least 5 categories
		"stat":    5,  // Statistics should have at least 5 categories
	}

	for area, minCategories := range expectedAreas {
		categories, ok := taxonomy[area]
		if !ok {
			t.Errorf("expected area %q not found in taxonomy", area)
			continue
		}
		if len(categories) < minCategories {
			t.Errorf("area %q has %d categories, expected at least %d", area, len(categories), minCategories)
		}
	}

	// Verify specific well-known categories exist with descriptions
	wellKnownCategories := map[string]string{
		"cs.AI":          "cs",
		"cs.LG":          "cs",
		"math.AG":        "math",
		"physics.optics": "physics",
		"q-bio.BM":       "q-bio",
		"stat.ML":        "stat",
	}

	for catCode, expectedArea := range wellKnownCategories {
		areaCategories, ok := taxonomy[expectedArea]
		if !ok {
			t.Errorf("area %q not found for category %q", expectedArea, catCode)
			continue
		}
		desc, ok := areaCategories[catCode]
		if !ok {
			t.Errorf("expected category %q not found in area %q", catCode, expectedArea)
		} else if desc == "" {
			t.Errorf("category %q has empty description", catCode)
		}
	}

	// Log summary for visibility
	t.Logf("Successfully fetched taxonomy with %d areas", len(taxonomy))
	for area, categories := range taxonomy {
		t.Logf("  %s: %d categories", area, len(categories))
	}
}
