package temporal

import (
	"crypto/tls"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
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

	return &TemporalClient{
		config:   config,
		identity: identity,
	}
}

func (a *TemporalClient) Initialize() error {

	a.identity = fmt.Sprintf("thand-agent-%s", a.config.Namespace)

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
