package api

import (
	"fmt"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
)

// Service is the transport-agnostic API service for elevation.
// Construct it with NewApiService and inject it wherever elevation logic is needed.
type Service struct {
	cfg       models.ConfigImpl
	workflows manager.ThandWorkflowBroker
}

// NewApiService creates a Service.
// Returns an error if the underlying workflow manager cannot be initialised;
// the caller is responsible for deciding how to handle the failure.
func NewApiService(cfg models.ConfigImpl) (*Service, error) {

	workflows, err := manager.NewThandWorkflowManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("api: failed to create workflow manager: %w", err)
	}

	return &Service{cfg: cfg, workflows: workflows}, nil
}
