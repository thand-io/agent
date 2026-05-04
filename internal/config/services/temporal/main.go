package temporal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// DefaultTaskQueue is the shared task queue used by server-mode workers.
const DefaultTaskQueue = "default"

type TemporalClient struct {
	config    *models.TemporalConfig
	client    client.Client
	worker    worker.Worker
	taskQueue string
	vault     models.VaultImpl

	mu             sync.Mutex
	readyCh        chan struct{}
	closeReadyOnce sync.Once
	workersStarted bool
}

func NewTemporalClient(
	config *models.TemporalConfig,
	vault models.VaultImpl,
	taskQueue string,
) *TemporalClient {
	return &TemporalClient{
		config:    config,
		taskQueue: taskQueue,
		vault:     vault,
		readyCh:   make(chan struct{}),
	}
}

func (a *TemporalClient) Initialize() error {

	if len(a.taskQueue) == 0 {
		return fmt.Errorf("temporal client requires a task queue")
	}

	clientOptions := client.Options{
		Logger:    newLogrusLogger(),
		HostPort:  a.GetHostPort(),
		Namespace: a.GetNamespace(),
		Identity:  a.taskQueue,
	}

	// Configure authentication (API key or mTLS)
	if err := a.configureAuth(&clientOptions); err != nil {
		return fmt.Errorf("failed to configure Temporal authentication: %w", err)
	}

	logrus.Infof("Connecting to Temporal at %s in namespace %s", a.GetHostPort(), a.GetNamespace())

	temporalClient, err := client.Dial(clientOptions)

	if err != nil {
		// Common errors:
		// - no children to pick from: The namespace does not exist in temporal cloud.
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

	// Create the single worker for our task queue.
	// Registration must happen before the worker is started.
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.worker != nil {
		logrus.Warn("Temporal worker already created, skipping worker initialization")
		return nil
	}

	a.worker = worker.New(
		temporalClient,
		a.taskQueue,
		workerOptions,
	)

	return nil
}

// StartWorkers starts the registered Temporal worker.
// This must be called only after workflow/activity registration is complete.
func (c *TemporalClient) StartWorkers() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return fmt.Errorf("temporal client is not initialized")
	}

	if c.worker == nil {
		c.markReady()
		return fmt.Errorf("no Temporal worker configured")
	}

	if c.workersStarted {
		logrus.Warn("Temporal worker already started, skipping worker startup")
		return nil
	}

	buildID := common.GetBuildIdentifier()

	logrus.WithFields(logrus.Fields{
		"BuildID":   buildID,
		"taskQueue": c.taskQueue,
	}).Info("Starting Temporal worker")

	if err := c.worker.Start(); err != nil {
		c.markReady()
		return fmt.Errorf("failed to start temporal worker: %w", err)
	}

	c.workersStarted = true

	// If versioning is enabled, confirm our deployment version is registered
	// on the Temporal server before allowing workflow submissions via GetClient().
	if c.config.DisableVersioning {
		c.markReady()
	} else {
		go c.awaitVersionRegistration(buildID)
	}

	return nil
}

func (c *TemporalClient) GetClient() client.Client {
	c.mu.Lock()
	shouldWait := c.workersStarted && !c.config.DisableVersioning
	c.mu.Unlock()

	if shouldWait {
		<-c.readyCh
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func (c *TemporalClient) HasClient() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client != nil
}

func (c *TemporalClient) HasWorker() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.worker != nil
}

// GetWorker returns the underlying Temporal worker, or nil if not initialized.
func (c *TemporalClient) GetWorker() worker.Worker {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.worker
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
	return c.taskQueue
}

func (c *TemporalClient) GetIdentity() string {
	return c.taskQueue
}

func (c *TemporalClient) IsVersioningDisabled() bool {
	return c.config.DisableVersioning
}

// versionRegistrationTimeout is the maximum time to wait for the Temporal
// server to register our deployment version after worker startup.
const versionRegistrationTimeout = 30 * time.Second

// markReady signals that the Temporal client is ready for workflow submission.
// Safe to call multiple times; only the first call has any effect.
func (c *TemporalClient) markReady() {
	c.closeReadyOnce.Do(func() { close(c.readyCh) })
}

// awaitVersionRegistration polls the Temporal server until our deployment
// version is registered and visible, then signals readiness. This prevents
// workflow submissions with PinnedVersioningOverride from failing because
// the server hasn't indexed the version yet.
func (c *TemporalClient) awaitVersionRegistration(buildID string) {
	defer c.markReady()

	deploymentName := sdkConstants.TemporalDeploymentName

	logrus.WithFields(logrus.Fields{
		"BuildID":        buildID,
		"DeploymentName": deploymentName,
	}).Info("Waiting for Temporal deployment version to be registered")

	ctx, cancel := context.WithTimeout(context.Background(), versionRegistrationTimeout)
	defer cancel()

	handle := c.client.WorkerDeploymentClient().GetHandle(deploymentName)

	backoff := 500 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		_, err := handle.DescribeVersion(ctx, client.WorkerDeploymentDescribeVersionOptions{
			BuildID: buildID,
		})
		if err == nil {
			logrus.WithFields(logrus.Fields{
				"BuildID":        buildID,
				"DeploymentName": deploymentName,
			}).Info("Temporal deployment version registered and ready")
			return
		}

		select {
		case <-ctx.Done():
			logrus.WithFields(logrus.Fields{
				"BuildID":        buildID,
				"DeploymentName": deploymentName,
			}).Warn("Timed out waiting for Temporal deployment version, proceeding anyway")
			return
		case <-time.After(backoff):
			if backoff < maxBackoff {
				backoff *= 2
			}
		}
	}
}

func (c *TemporalClient) Shutdown() error {
	c.markReady() // Unblock any waiters
	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop the worker before closing the client
	if c.worker != nil {
		logrus.WithField("taskQueue", c.taskQueue).Info("Stopping Temporal worker")
		c.worker.Stop()
	}
	if c.client != nil {
		c.client.Close()
	}

	c.worker = nil
	c.client = nil
	c.workersStarted = false

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
