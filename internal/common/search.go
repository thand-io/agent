package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
)

func BleveListSearch[T any](
	ctx context.Context,
	searchIndex bleve.Index,
	compareFunction func(a *search.DocumentMatch, b T) bool,
	items []T,
	filters ...string,
) ([]T, error) {

	if len(filters) == 0 {
		// return all items
		return items, nil
	}

	// Build search query from filters
	queryString := strings.TrimSpace(strings.Join(filters, " "))

	if len(queryString) == 0 {
		// return all items if query is empty after trimming
		return items, nil
	}

	query := bleve.NewQueryStringQuery(queryString)
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Size = len(items) // Return all matches

	searchResults, err := searchIndex.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert search results back to permissions
	var matched []T
	for _, hit := range searchResults.Hits {
		for _, item := range items {
			if compareFunction(hit, item) {
				matched = append(matched, item)
				break
			}
		}
	}

	return matched, nil
}
