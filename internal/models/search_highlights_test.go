package models

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/blevesearch/bleve/v2"
	blsearch "github.com/blevesearch/bleve/v2/search"
)

type highlightTestRole struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Inherits    string `json:"inherits"`
	Ops         string `json:"operations"`
}

// TestHighlightTermsAreIndexedTokens verifies that _highlights contains the actual
// matched indexed token from the store (e.g. "administratoraccess"), NOT the
// echoed query string (e.g. "admin").
//
// This mirrors Elasticsearch highlight behaviour: the returned term is what exists
// in the indexed text, not what the user typed.
func TestHighlightTermsAreIndexedTokens(t *testing.T) {
	idx, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx.Index("aws-admin", highlightTestRole{
		Name:     "AWS Admin",
		Inherits: "administratoraccess poweruseraccess",
		Ops:      "sts:assumerole",
	})

	items := []highlightTestRole{
		{Name: "AWS Admin", Inherits: "administratoraccess poweruseraccess", Ops: "sts:assumerole"},
	}
	cmp := func(a *blsearch.DocumentMatch, b highlightTestRole) bool { return a.ID == "aws-admin" }

	cases := []struct {
		name          string
		query         string
		wantField     string
		wantFullToken string // the EXACT full indexed token that must appear in highlights
	}{
		{
			// Partial match — user types "admin", full indexed token is "administratoraccess".
			// Must return "administratoraccess", NOT "admin".
			name:          "partial_must_return_full_token_not_query",
			query:         "admin",
			wantField:     "inherits",
			wantFullToken: "administratoraccess",
		},
		{
			// Full match — token equals query.
			name:          "exact_match_returns_token",
			query:         "administratoraccess",
			wantField:     "inherits",
			wantFullToken: "administratoraccess",
		},
		{
			// Prefix match — user types "power", full indexed token is "poweruseraccess".
			// Must return "poweruseraccess", NOT "power".
			name:          "prefix_must_return_full_token_not_query",
			query:         "power",
			wantField:     "inherits",
			wantFullToken: "poweruseraccess",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sr := &SearchRequest{Query: tc.query, Terms: []string{tc.query}}
			results, err := BleveListSearch(
				context.Background(), idx, cmp, items, sr,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) == 0 {
				t.Fatalf("expected a result for query %q, got none", tc.query)
			}

			r := results[0]
			b, _ := json.MarshalIndent(r, "", "  ")
			t.Logf("Query=%q response:\n%s", tc.query, b)

			if r.Highlights == nil {
				t.Fatalf("_highlights is nil for query %q", tc.query)
			}
			terms, ok := r.Highlights[tc.wantField]
			if !ok {
				t.Fatalf("expected field %q in _highlights, got keys: %v", tc.wantField, highlightKeys(r.Highlights))
			}
			if len(terms) == 0 {
				t.Fatalf("highlight terms for field %q are empty", tc.wantField)
			}

			// Fail if we're just echoing the query back when it differs from the token.
			for _, tok := range terms {
				if tok == tc.query && tok != tc.wantFullToken {
					t.Errorf("_highlights echoed back the query %q instead of the indexed token %q", tc.query, tc.wantFullToken)
					return
				}
			}

			var found bool
			for _, tok := range terms {
				if tok == tc.wantFullToken {
					found = true
					t.Logf("PASS: _highlights[%q] = %q (full indexed token)", tc.wantField, tok)
				}
			}
			if !found {
				t.Errorf("expected full indexed token %q in _highlights[%q], got: %v (query was %q)",
					tc.wantFullToken, tc.wantField, terms, tc.query)
			}
		})
	}
}

// TestHighlightDoesNotEchoQueryWhenTokenDiffers verifies that mixed-case queries
// return the analysed indexed token, not the original casing of the query.
func TestHighlightDoesNotEchoQueryWhenTokenDiffers(t *testing.T) {
	idx, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx.Index("r1", highlightTestRole{
		Name:     "ReadOnly",
		Inherits: "arn:aws:iam::aws:policy/administratoraccess",
		Ops:      "ec2:describeinstances s3:getobject",
	})

	items := []highlightTestRole{
		{Name: "ReadOnly", Inherits: "arn:aws:iam::aws:policy/administratoraccess", Ops: "ec2:describeinstances s3:getobject"},
	}
	cmp := func(a *blsearch.DocumentMatch, b highlightTestRole) bool { return a.ID == "r1" }

	// Query is mixed-case; stored/analysed token is lowercase "administratoraccess".
	sr := &SearchRequest{Query: "AdministratorAccess", Terms: []string{"AdministratorAccess"}}
	results, err := BleveListSearch(
		context.Background(), idx, cmp, items, sr,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected result for AdministratorAccess query")
	}

	r := results[0]
	b, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("Response:\n%s", b)

	terms := r.Highlights["inherits"]
	if len(terms) == 0 {
		t.Fatal("expected highlight terms for 'inherits'")
	}

	// Must be the analysed stored token, not the original mixed-case query.
	for _, tok := range terms {
		if tok == "AdministratorAccess" {
			t.Errorf("highlight echoed original query casing %q — must return analysed indexed token %q", tok, "administratoraccess")
		}
		if tok == "administratoraccess" {
			t.Logf("PASS: returned analysed indexed token %q, not echoed query", tok)
		}
	}
}

// TestHighlightMultipleFieldsAndTerms verifies that when multiple fields match,
// all distinct matched indexed tokens per field are returned.
func TestHighlightMultipleFieldsAndTerms(t *testing.T) {
	idx, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx.Index("r1", highlightTestRole{
		Name:     "EC2 Admin",
		Inherits: "ec2fullaccess",
		Ops:      "ec2:describeinstances ec2:startinstances",
	})

	items := []highlightTestRole{
		{Name: "EC2 Admin", Inherits: "ec2fullaccess", Ops: "ec2:describeinstances ec2:startinstances"},
	}
	cmp := func(a *blsearch.DocumentMatch, b highlightTestRole) bool { return a.ID == "r1" }

	sr := &SearchRequest{Query: "ec2", Terms: []string{"ec2"}}
	results, err := BleveListSearch(
		context.Background(), idx, cmp, items, sr,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected result")
	}

	r := results[0]
	b, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("Response:\n%s", b)

	if len(r.Highlights) < 2 {
		t.Errorf("expected highlights for multiple fields (inherits + permissions), got: %v", r.Highlights)
	}

	for field, terms := range r.Highlights {
		sorted := make([]string, len(terms))
		copy(sorted, terms)
		sort.Strings(sorted)
		t.Logf("Field=%q indexed_tokens=%v", field, sorted)

		// Tokens must be actual indexed content from the stored text.
		for _, tok := range terms {
			if tok != "ec2" && tok != "ec2fullaccess" && tok != "ec2:describeinstances" && tok != "ec2:startinstances" {
				t.Logf("  unexpected token %q in field %q — may be a sub-token from analyser", tok, field)
			}
		}
	}
}

func highlightKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestHighlightOnlyRawIDsAreSuppressed verifies that only raw technical ID fields
// ("id", "identifier") are suppressed from _highlights. Human-readable fields like
// name, label, and description surface when they match.
func TestHighlightOnlyRawIDsAreSuppressed(t *testing.T) {
	idx, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx.Index("aws_admin", highlightTestRole{
		Name:     "admin",
		Inherits: "aws_user arn:aws:iam::aws:policy/administratoraccess",
		Ops:      "ec2:describeinstances s3:listbuckets",
	})

	items := []highlightTestRole{
		{Name: "admin", Inherits: "aws_user arn:aws:iam::aws:policy/administratoraccess", Ops: "ec2:describeinstances s3:listbuckets"},
	}
	cmp := func(a *blsearch.DocumentMatch, b highlightTestRole) bool { return a.ID == "aws_admin" }

	sr := &SearchRequest{Query: "admin", Terms: []string{"admin"}}
	results, err := BleveListSearch(context.Background(), idx, cmp, items, sr)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected a result for query 'admin'")
	}

	r := results[0]
	b, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("Response:\n%s", b)

	// name is human-readable — must appear in highlights.
	if _, hasName := r.Highlights["name"]; !hasName {
		t.Errorf("expected 'name' in _highlights (it's human-readable, not a raw ID), got keys: %v", highlightKeys(r.Highlights))
	} else {
		t.Logf("PASS: 'name' correctly appears in _highlights")
	}

	// inherits must also appear.
	if _, hasInherits := r.Highlights["inherits"]; !hasInherits {
		t.Errorf("expected 'inherits' in _highlights, got keys: %v", highlightKeys(r.Highlights))
	} else {
		t.Logf("PASS: 'inherits' appears in _highlights")
	}

	t.Logf("_reason = %q", r.Reason)
}

// TestHighlightAdminSearchHitsMultipleFields is a regression test mirroring
// config/roles/aws.yaml: searching "admin" must return highlights from name,
// description (when it contains "admin"), and inherits — all in one result.
func TestHighlightAdminSearchHitsMultipleFields(t *testing.T) {
	idx, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		t.Fatal(err)
	}
	idx.Index("aws_admin", highlightTestRole{
		Name:        "admin",
		Description: "full admin access to all resources and capabilities",
		Inherits:    "arn:aws:iam::aws:policy/administratoraccess",
		Ops:         "ec2:describeinstances s3:listbuckets",
	})

	items := []highlightTestRole{{
		Name:        "admin",
		Description: "full admin access to all resources and capabilities",
		Inherits:    "arn:aws:iam::aws:policy/administratoraccess",
		Ops:         "ec2:describeinstances s3:listbuckets",
	}}
	cmp := func(a *blsearch.DocumentMatch, b highlightTestRole) bool { return a.ID == "aws_admin" }

	sr := &SearchRequest{Query: "admin", Terms: []string{"admin"}}
	results, err := BleveListSearch(context.Background(), idx, cmp, items, sr)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected a result for query 'admin'")
	}

	r := results[0]
	b, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("Response:\n%s", b)

	wantFields := map[string]string{
		"name":        "admin",
		"description": "admin",
		"inherits":    "administratoraccess",
	}

	for field, wantToken := range wantFields {
		terms, ok := r.Highlights[field]
		if !ok {
			t.Errorf("expected field %q in _highlights, got keys: %v", field, highlightKeys(r.Highlights))
			continue
		}
		found := false
		for _, tok := range terms {
			if tok == wantToken {
				found = true
			}
		}
		if !found {
			t.Errorf("_highlights[%q]: expected token %q, got %v", field, wantToken, terms)
		} else {
			t.Logf("PASS: _highlights[%q] contains %q", field, wantToken)
		}
	}

	t.Logf("_reason = %q", r.Reason)
}
