package models

import (
	internal "github.com/thand-io/agent/internal/models"
)

// SearchRequest represents a request to search for resources, users, or other entities
// within the system. It includes various filters, pagination options, and sorting preferences
// to refine the search results.
type SearchRequest = internal.SearchRequest

// SearchResult represents the outcome of a search operation, containing
// the matched items along with metadata such as total count and pagination details.
type SearchResult[T any] = internal.SearchResult[T]

// ReturnSearchResults converts a slice of items into a slice of SearchResult.
// This is a generic function that must be called with a type parameter, e.g.:
//   results := ReturnSearchResults[MyType](items)
func ReturnSearchResults[T any](items []T) []SearchResult[T] {
	return internal.ReturnSearchResults[T](items)
}
