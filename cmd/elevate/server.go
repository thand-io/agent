package main

import (
	"context"
	"errors"

	"github.com/thand-io/agent/cmd/elevate/handler"
)

// Server coordinates IPC accept loop and delegates per-connection handling.
type Server struct {
	ipc     handler.IPCServer
	handler *handler.Handler
}

func NewServer(ipc handler.IPCServer, h *handler.Handler) *Server {
	return &Server{ipc: ipc, handler: h}
}

func (s *Server) Run(ctx context.Context) error {
	if s.ipc == nil {
		return errors.New("ipc server is required")
	}
	if s.handler == nil {
		return errors.New("handler is required")
	}

	if err := s.ipc.Start(ctx); err != nil {
		return err
	}
	defer s.ipc.Close()

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		conn, err := s.ipc.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		if err := s.handler.HandleConnection(ctx, conn); err != nil {
			_ = conn.Close()
			// Chunk 1: no logging/error policy yet.
			continue
		}

		_ = conn.Close()
	}
}
