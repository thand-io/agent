package grant

import (
	"context"
	"errors"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/handler"
)

type PlaceholderEngine struct{}

func NewPlaceholderEngine() handler.GrantEngine {
	return &PlaceholderEngine{}
}

func (g *PlaceholderEngine) Grant(ctx context.Context, req domain.GrantRequest) (domain.GrantResult, error) {
	_ = ctx
	_ = req
	return domain.GrantResult{}, errors.New("grant engine not implemented")
}

func (g *PlaceholderEngine) Revoke(ctx context.Context, req domain.RevokeRequest) error {
	_ = ctx
	_ = req
	return errors.New("grant engine not implemented")
}
