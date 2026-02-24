package api

import (
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
)

// Service is the transport-agnostic API service for elevation.
// Construct it with NewService and inject it wherever elevation logic is needed.
type Service struct {
	cfg       models.ConfigImpl
	workflows *manager.ThandWorkflowManager
}

// NewApiService creates a Service.
func NewApiService(cfg models.ConfigImpl) *Service {

	workflows, err := manager.NewThandWorkflowManager(cfg)

	if err != nil {
		logrus.WithError(err).Fatal("Failed to create workflow manager")
		return nil
	}

	return &Service{cfg: cfg, workflows: workflows}
}
