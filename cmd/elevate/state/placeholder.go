package state

import (
	"context"
	"errors"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/handler"
)

type PlaceholderStore struct{}

func NewPlaceholderStore() handler.StateStore {
	return &PlaceholderStore{}
}

func (s *PlaceholderStore) Put(ctx context.Context, grant domain.GrantState) error {
	_ = ctx
	_ = grant
	return errors.New("state store not implemented")
}

func (s *PlaceholderStore) Delete(ctx context.Context, requestID string) error {
	_ = ctx
	_ = requestID
	return errors.New("state store not implemented")
}

func (s *PlaceholderStore) List(ctx context.Context) ([]domain.GrantState, error) {
	_ = ctx
	return nil, errors.New("state store not implemented")
}
