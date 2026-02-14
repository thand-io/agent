package temporal

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// MaxWorkers is the maximum number of identity-specific workers per client.
const MaxWorkers = 5

type TemporalClient struct {
	config     *models.TemporalConfig
	client     client.Client
	workers    map[string]worker.Worker
	identities []string
	vault      models.VaultImpl

	mu sync.Mutex
}

func NewTemporalClient(
	config *models.TemporalConfig,
	vault models.VaultImpl,
	identities ...string,
) *TemporalClient {
	// Deduplicate identities to prevent orphaned workers
	seen := make(map[string]struct{}, len(identities))
	unique := make([]string, 0, len(identities))
	for _, id := range identities {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			unique = append(unique, id)
		}
	}
	if len(unique) > MaxWorkers {
		unique = unique[:MaxWorkers]
	}
	return &TemporalClient{
		config:     config,
		identities: unique,
		vault:      vault,
		workers:    make(map[string]worker.Worker, len(unique)),
	}
}

func (a *TemporalClient) Initialize() error {

	if len(a.identities) == 0 {
		return fmt.Errorf("temporal client requires at least one identity")
	}

	clientOptions := client.Options{
		Logger:    newLogrusLogger(),
		HostPort:  a.GetHostPort(),
		Namespace: a.GetNamespace(),
		Identity:  a.identities[0],
	}

	// Configure authentication (API key or mTLS)
	if err := a.configureAuth(&clientOptions); err != nil {
		return fmt.Errorf("failed to configure Temporal authentication: %w", err)
	}

	logrus.Infof("Connecting to Temporal at %s in namespace %s", a.GetHostPort(), a.GetNamespace())

	temporalClient, err := client.Dial(clientOptions)

	if err != nil {
		logrus.WithError(err).
			WithFields(logrus.Fields{
				"endpoint":  a.GetHostPort(),
				"namespace": a.GetNamespace(),
			}).
			Errorln("failed to create Temporal client")
		return err
	}

	a.mu.Lock()
	a.client = temporalClient
	a.mu.Unlock()

	// Now that we have a client, lets validate the configuraiton of the external namespace
	err = a.validateTemporalNamespace()

	if err != nil {
		logrus.WithError(err).Errorln("failed to validate Temporal namespace")
	}

	// Get agent version for Worker Build ID
	buildID := common.GetBuildIdentifier()

	workerOptions := worker.Options{
		Identity:                         a.GetIdentity(),
		MaxConcurrentActivityTaskPollers: 5,
	}

	if !a.config.DisableVersioning {
		logrus.WithFields(logrus.Fields{
			"BuildID":        buildID,
			"DeploymentName": sdkConstants.TemporalDeploymentName,
		}).Info("Configuring Worker with versioning")

		workerOptions.DeploymentOptions = worker.DeploymentOptions{
			UseVersioning: true,
			Version: worker.WorkerDeploymentVersion{
				DeploymentName: sdkConstants.TemporalDeploymentName,
				BuildID:        buildID,
			},
			// Default workflows to Pinned behavior
			DefaultVersioningBehavior: workflow.VersioningBehaviorPinned,
		}
	}

	// Create and start a worker for each identity (task queue)
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.workers) > 0 {
		logrus.Warn("Temporal workers already started, skipping worker initialization")
		return nil
	}

	for _, identity := range a.identities {
		newWorker := worker.New(
			temporalClient,
			identity,
			workerOptions,
		)

		logrus.WithFields(logrus.Fields{
			"BuildID":   buildID,
			"taskQueue": identity,
		}).Infof("Starting Temporal worker")

		if err := newWorker.Start(); err != nil {
			logrus.WithError(err).WithField("taskQueue", identity).Error("Failed to start temporal worker")
			// Stop any workers that were already started
			for _, w := range a.workers {
				w.Stop()
			}
			a.workers = make(map[string]worker.Worker)
			return err
		}

		a.workers[identity] = newWorker
	}

	return nil
}

func (c *TemporalClient) GetClient() client.Client {
	return c.client
}

func (c *TemporalClient) HasClient() bool {
	return c.client != nil
}

func (c *TemporalClient) HasWorker() bool {
	return len(c.workers) > 0
}

// GetWorker returns a synthetic worker that broadcasts registration calls
// across all (or a filtered subset of) identity-specific workers.
// If identities are provided, only matching workers are included.
// Returns nil if no matching workers are found.
func (c *TemporalClient) GetWorker(identities ...string) worker.Worker {
	if len(c.workers) == 0 {
		return nil
	}

	// No filter: return all workers
	if len(identities) == 0 {
		workers := make([]worker.Worker, 0, len(c.workers))
		for _, w := range c.workers {
			workers = append(workers, w)
		}
		return &multiWorker{workers: workers}
	}

	// Filtered: return only matching workers
	workers := make([]worker.Worker, 0, len(identities))
	for _, id := range identities {
		if w, ok := c.workers[id]; ok {
			workers = append(workers, w)
		}
	}
	if len(workers) == 0 {
		return nil
	}
	return &multiWorker{workers: workers}
}

func (c *TemporalClient) GetHostPort() string {
	return fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
}

func (c *TemporalClient) GetNamespace() string {
	if len(c.config.Namespace) == 0 {
		return "default"
	}
	return c.config.Namespace
}

func (c *TemporalClient) GetTaskQueue() string {
	if len(c.identities) == 0 {
		return ""
	}
	return c.identities[0]
}

func (c *TemporalClient) GetIdentity() string {
	if len(c.identities) == 0 {
		return ""
	}
	return c.identities[0]
}

func (c *TemporalClient) IsVersioningDisabled() bool {
	return c.config.DisableVersioning
}

func (c *TemporalClient) Shutdown() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop all workers before closing the client
	for id, w := range c.workers {
		logrus.WithField("taskQueue", id).Info("Stopping Temporal worker")
		w.Stop()
	}
	if c.client != nil {
		c.client.Close()
	}

	c.workers = nil
	c.client = nil

	return nil
}

// configureAuth configures client authentication using API key or mTLS
func (a *TemporalClient) configureAuth(options *client.Options) error {
	// API Key takes precedence over mTLS
	if a.hasAPIKeyAuth() {
		return a.configureAPIKeyAuth(options)
	}

	// Check for mTLS configuration
	if a.config.HasMtlsConfig() {
		return a.configureMTLSAuth(options)
	}

	// No authentication configured (insecure - typically for local development)
	logrus.Warn("No Temporal authentication configured (API key or mTLS)")
	return nil
}

func (c *TemporalClient) validateTemporalNamespace() error {

	// Check if the namespace exists
	namespace := c.GetNamespace()
	if len(namespace) == 0 {
		return fmt.Errorf("namespace is not set")
	}

	// Validate the namespace with the Temporal server
	namespaceResponse, err := c.client.WorkflowService().DescribeNamespace(context.Background(), &workflowservice.DescribeNamespaceRequest{
		Namespace: namespace,
	})

	if err != nil {
		return fmt.Errorf("failed to describe Temporal namespace '%s': %w", namespace, err)
	}

	// Get search attributes for the namespace
	searchAttributesResponse, err := c.client.WorkflowService().GetSearchAttributes(context.Background(), &workflowservice.GetSearchAttributesRequest{})
	if err != nil {
		return fmt.Errorf("failed to get search attributes for namespace '%s': %w", namespace, err)
	}

	// Define required typed search attributes
	requiredSearchAttributes := []any{
		sdkConstants.TypedSearchAttributeStatus,
		sdkConstants.TypedSearchAttributeTask,
		sdkConstants.TypedSearchAttributeUser,
		sdkConstants.TypedSearchAttributeRole,
		sdkConstants.TypedSearchAttributeWorkflow,
		sdkConstants.TypedSearchAttributeProviders,
		sdkConstants.TypedSearchAttributeReason,
		sdkConstants.TypedSearchAttributeDuration,
		sdkConstants.TypedSearchAttributeIdentities,
		sdkConstants.TypedSearchAttributeApproved,
	}

	// Check if all required search attributes are defined
	missingAttributes := []string{}
	for _, attr := range requiredSearchAttributes {
		// Use type assertion to access the SearchAttributeKey interface methods
		if searchAttr, ok := attr.(interface {
			GetName() string
			GetValueType() any
		}); ok {
			attributeName := searchAttr.GetName()
			expectedType := searchAttr.GetValueType()

			if actualType, exists := searchAttributesResponse.GetKeys()[attributeName]; !exists {
				missingAttributes = append(missingAttributes, attributeName)
			} else {
				// Compare the enum values directly
				if actualType != expectedType {
					return fmt.Errorf("search attribute '%s' has incorrect type. Expected: %v, Actual: %v",
						attributeName, expectedType, actualType)
				}
			}
		}
	}

	if len(missingAttributes) > 0 {
		return fmt.Errorf("namespace '%s' is missing required typed search attributes: %v",
			namespace, missingAttributes)
	}

	logrus.WithFields(logrus.Fields{
		"namespace": namespace,
		"state":     namespaceResponse.GetNamespaceInfo().GetState().String(),
	}).Info("Temporal namespace validation successful")

	return nil
}
