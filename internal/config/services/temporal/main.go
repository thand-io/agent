package temporal

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

type TemporalClient struct {
	config   *models.TemporalConfig
	client   client.Client
	worker   worker.Worker
	identity string
}

func NewTemporalClient(config *models.TemporalConfig, identity string) *TemporalClient {

	if len(identity) == 0 {
		logrus.Fatalln("identity must be provided")
	}

	return &TemporalClient{
		config:   config,
		identity: identity,
	}
}

func (a *TemporalClient) Initialize() error {

	clientOptions := client.Options{
		Logger:    newLogrusLogger(),
		HostPort:  a.GetHostPort(),
		Namespace: a.GetNamespace(),
		Identity:  a.identity,
	}

	if len(a.config.ApiKey) > 0 {

		clientOptions.ConnectionOptions = client.ConnectionOptions{
			TLS: &tls.Config{},
		}
		clientOptions.Credentials = client.NewAPIKeyStaticCredentials(a.config.ApiKey)

	} else if len(a.config.MtlsCertificate) > 0 || len(a.config.MtlsCertificatePath) > 0 {

		// TODO load certs
		clientOptions.ConnectionOptions = client.ConnectionOptions{
			TLS: &tls.Config{Certificates: []tls.Certificate{{
				Certificate: [][]byte{},
			}}},
		}

	}

	logrus.Infof("Connecting to Temporal at %s in namespace %s", a.GetHostPort(), a.GetNamespace())

	temporalClient, err := client.Dial(clientOptions)

	if err != nil {
		logrus.WithError(err).Errorln("failed to create Temporal client")
		return err
	}

	a.client = temporalClient

	// Now that we have a client, lets validate the configuraiton of the external namespace
	err = a.validateTemporalNamespace()

	if err != nil {
		logrus.WithError(err).Errorln("failed to validate Temporal namespace")
	}

	// Lets register the worker
	a.worker = worker.New(
		temporalClient,
		a.GetTaskQueue(),
		worker.Options{
			Identity: a.GetIdentity(),
		},
	)

	go func() {

		logrus.Infof("Starting Temporal worker")

		err := a.worker.Run(worker.InterruptCh())
		if err != nil {
			logrus.WithError(err).Errorln("failed to start Temporal worker")
		}
	}()

	return nil
}

func (c *TemporalClient) GetClient() client.Client {
	return c.client
}

func (c *TemporalClient) HasClient() bool {
	return c.client != nil
}

func (c *TemporalClient) HasWorker() bool {
	return c.worker != nil
}

func (c *TemporalClient) GetWorker() worker.Worker {
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
	return c.identity
}

func (c *TemporalClient) GetIdentity() string {
	return c.identity
}

func (c *TemporalClient) Shutdown() error {
	c.client.Close()
	c.worker.Stop()
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
		models.TypedSearchAttributeStatus,
		models.TypedSearchAttributeTask,
		models.TypedSearchAttributeUser,
		models.TypedSearchAttributeRole,
		models.TypedSearchAttributeWorkflow,
		models.TypedSearchAttributeProviders,
		models.TypedSearchAttributeReason,
		models.TypedSearchAttributeDuration,
		models.TypedSearchAttributeIdentities,
		models.TypedSearchAttributeApproved,
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
