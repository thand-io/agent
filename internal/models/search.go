package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
)

type SearchRequest struct {
	Query string   `json:"query"`
	Terms []string `json:"terms"`
	Limit int      `json:"limit,omitempty" default:"100"`
}

func (sr *SearchRequest) GetLimit() int {
	if sr.Limit < 0 { // Enforce a minimum limit to prevent abuse
		return 100
	} else if sr.Limit > 500 { // Enforce a maximum limit to prevent abuse
		return 500
	}
	return sr.Limit
}

func (sr *SearchRequest) IsEmpty() bool {

	if len(strings.TrimSpace(sr.Query)) == 0 && len(sr.Terms) == 0 {
		return true
	}

	// Check if all terms are empty or whitespace
	if len(sr.Terms) > 0 && len(strings.TrimSpace(strings.Join(sr.Terms, ""))) == 0 {
		return true
	}

	return false

}

type SearchResult[T any] struct {
	ID     string  `json:"_id,omitempty"`
	Score  float64 `json:"_score,omitempty"`
	Reason string  `json:"_reason,omitempty"`
	Result T       `json:"_source"`
}

func ReturnSearchResults[T any](items []T) []SearchResult[T] {
	var results []SearchResult[T]
	for _, item := range items {
		results = append(results, SearchResult[T]{
			// ID and Score are left empty as they are not relevant when returning all items
			Result: item,
		})
	}
	return results
}

func BleveListSearch[T any](
	ctx context.Context,
	searchIndex bleve.Index,
	compareFunction func(a *search.DocumentMatch, b T) bool,
	items []T,
	searchReq *SearchRequest,
) ([]SearchResult[T], error) {

	if searchReq == nil || searchReq.IsEmpty() {
		// return all items
		return ReturnSearchResults(items), nil
	}

	var queryBuilder query.Query
	var queries []query.Query

	if len(searchReq.Terms) > 0 {
		// Build a conjunction query from the terms; each term is a disjunction
		// of MatchQuery (full-word) | PrefixQuery (prefix) | WildcardQuery
		// (*term* — substring) so that "prod" matches "Production".
		termQueries := []query.Query{}
		for _, term := range searchReq.Terms {
			lower := strings.ToLower(term)
			termMatch := bleve.NewDisjunctionQuery(
				bleve.NewMatchQuery(lower),
				bleve.NewPrefixQuery(lower),
				bleve.NewWildcardQuery("*"+lower+"*"),
			)
			termQueries = append(termQueries, termMatch)
		}
		// All terms must be present (conjunction)
		queries = append(queries, bleve.NewConjunctionQuery(termQueries...))
	}

	if len(searchReq.Query) > 0 {
		// Disjunction of match / prefix / wildcard for the free-text query
		// so that partial input like "prod" hits "Production Account".
		lower := strings.ToLower(searchReq.Query)
		queries = append(queries, bleve.NewDisjunctionQuery(
			bleve.NewMatchQuery(lower),
			bleve.NewPrefixQuery(lower),
			bleve.NewWildcardQuery("*"+lower+"*"),
		))
	}

	if len(queries) == 0 {
		return ReturnSearchResults(items), nil
	} else {
		queryBuilder = bleve.NewDisjunctionQuery(queries...)
	}

	searchRequest := bleve.NewSearchRequest(queryBuilder)

	if searchReq.GetLimit() > 0 {
		searchRequest.Size = searchReq.GetLimit() // Return all matches
	} else {
		searchRequest.Size = 100 // Default limit
	}

	searchResults, err := searchIndex.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert search results back to typed items
	var matched []SearchResult[T]
	for _, hit := range searchResults.Hits {
		for _, item := range items {
			if compareFunction(hit, item) {
				found := SearchResult[T]{
					ID:     hit.ID,
					Score:  hit.Score,
					Result: item,
				}

				if hit.Expl != nil {
					found.Reason = hit.Expl.Message
				}

				matched = append(matched, found)
				break
			}
		}
	}

	return matched, nil
}
