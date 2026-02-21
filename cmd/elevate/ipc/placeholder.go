package ipc

import (
	"context"
	"errors"

	"github.com/thand-io/agent/cmd/elevate/handler"
)

type PlaceholderServer struct{}

func NewPlaceholderServer() handler.IPCServer {
	return &PlaceholderServer{}
}

func (s *PlaceholderServer) Start(ctx context.Context) error {
	_ = ctx
	return errors.New("ipc transport not implemented")
}

func (s *PlaceholderServer) Accept(ctx context.Context) (handler.IPCConn, error) {
	_ = ctx
	return nil, errors.New("ipc transport not implemented")
}

func (s *PlaceholderServer) Close() error {
	return nil
}
