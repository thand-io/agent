package temporal

import (
	"github.com/nexus-rpc/sdk-go/nexus"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// multiWorker implements worker.Worker by broadcasting registration calls
// across multiple underlying workers. This allows a single GetWorker() call
// to register workflows and activities on all (or a filtered subset of)
// identity-specific task queues.
//
// Lifecycle methods (Start/Run/Stop) are no-ops because TemporalClient
// manages worker lifecycle directly in Initialize() and Shutdown().
type multiWorker struct {
	workers []worker.Worker
}

// Compile-time assertion that multiWorker implements worker.Worker.
var _ worker.Worker = (*multiWorker)(nil)

// --- WorkflowRegistry ---

func (m *multiWorker) RegisterWorkflow(w interface{}) {
	for _, wr := range m.workers {
		wr.RegisterWorkflow(w)
	}
}

func (m *multiWorker) RegisterWorkflowWithOptions(w interface{}, options workflow.RegisterOptions) {
	for _, wr := range m.workers {
		wr.RegisterWorkflowWithOptions(w, options)
	}
}

func (m *multiWorker) RegisterDynamicWorkflow(w interface{}, options workflow.DynamicRegisterOptions) {
	for _, wr := range m.workers {
		wr.RegisterDynamicWorkflow(w, options)
	}
}

// --- ActivityRegistry ---

func (m *multiWorker) RegisterActivity(a interface{}) {
	for _, wr := range m.workers {
		wr.RegisterActivity(a)
	}
}

func (m *multiWorker) RegisterActivityWithOptions(a interface{}, options activity.RegisterOptions) {
	for _, wr := range m.workers {
		wr.RegisterActivityWithOptions(a, options)
	}
}

func (m *multiWorker) RegisterDynamicActivity(a interface{}, options activity.DynamicRegisterOptions) {
	for _, wr := range m.workers {
		wr.RegisterDynamicActivity(a, options)
	}
}

// --- NexusServiceRegistry ---

func (m *multiWorker) RegisterNexusService(s *nexus.Service) {
	for _, wr := range m.workers {
		wr.RegisterNexusService(s)
	}
}

// --- Lifecycle (no-ops; managed by TemporalClient) ---

func (m *multiWorker) Start() error { return nil }

func (m *multiWorker) Run(interruptCh <-chan interface{}) error { return nil }

func (m *multiWorker) Stop() {}
