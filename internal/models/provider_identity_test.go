package models

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseProvider_ListIdentities_Search(t *testing.T) {

	p := NewBaseProvider("test", Provider{
		Name: "Test Provider",
	}, NewProviderCapabilities().WithDefaultIdentitiesConfiguration())

	userEmail := "hugh@thand.io"
	identity := Identity{
		ID:    "user1",
		Label: "Hugh",
		User: &User{
			Email: userEmail,
			Name:  "Hugh",
		},
	}

	p.SetIdentities([]Identity{identity})

	// Wait for index to be built
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	// Simulate what identities.go does: append *
	searchReq := &SearchRequest{
		Query: userEmail + "*",
		Terms: []string{userEmail},
	}

	results, err := p.ListIdentities(ctx, searchReq)
	assert.NoError(t, err)
	assert.NotEmpty(t, results, "Should find identity by email")

	if len(results) > 0 {
		assert.Equal(t, "user1", results[0].ID)
	}
}
