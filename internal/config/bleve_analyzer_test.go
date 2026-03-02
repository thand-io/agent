package config

import (
	"strings"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBleveStandardAnalyzer(t *testing.T) {
	// Create index with standard analyzer
	mapping := bleve.NewIndexMapping()
	mapping.DefaultAnalyzer = "standard"

	index, err := bleve.NewMemOnly(mapping)
	require.NoError(t, err)

	// Create flattened test document
	searchDoc := map[string]any{
		"name":       "GCP Admin",
		"operations": strings.Join([]string{"compute.instances.list", "storage.buckets.get"}, " "),
	}

	// Index the document
	err = index.Index("gcp-admin", searchDoc)
	require.NoError(t, err)

	// Try searching for "compute" with MatchQuery
	query := bleve.NewMatchQuery("compute")
	search := bleve.NewSearchRequest(query)
	results, err := index.Search(search)
	require.NoError(t, err)

	t.Logf("MatchQuery for 'compute': %d results", results.Total)
	for _, hit := range results.Hits {
		t.Logf("  - %s (score: %.2f)", hit.ID, hit.Score)
	}

	// Try with wildcard query
	query2 := bleve.NewWildcardQuery("compute*")
	search2 := bleve.NewSearchRequest(query2)
	results2, err := index.Search(search2)
	require.NoError(t, err)

	t.Logf("WildcardQuery for 'compute*': %d results", results2.Total)
	for _, hit := range results2.Hits {
		t.Logf("  - %s (score: %.2f)", hit.ID, hit.Score)
	}

	// Try with prefix query
	query3 := bleve.NewPrefixQuery("compute")
	search3 := bleve.NewSearchRequest(query3)
	results3, err := index.Search(search3)
	require.NoError(t, err)

	t.Logf("PrefixQuery for 'compute': %d results", results3.Total)
	for _, hit := range results3.Hits {
		t.Logf("  - %s (score: %.2f)", hit.ID, hit.Score)
	}

	assert.True(t, results.Total > 0 || results2.Total > 0 || results3.Total > 0, "Should find at least one result")
}
