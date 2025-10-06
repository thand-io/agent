package services

import (
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/services/temporal"
	"github.com/thand-io/agent/internal/models"
)

type localClient struct {
	environment *models.EnvironmentConfig
	config      *models.ServicesConfig

	encrypt   models.EncryptionImpl
	vault     models.VaultImpl
	scheduler models.SchedulerImpl
	llm       models.LargeLanguageModelImpl
	temporal  models.TemporalImpl
}

func NewServicesClient(
	environment *models.EnvironmentConfig,
	config *models.ServicesConfig,
) *localClient {
	return &localClient{
		environment: environment,
		config:      config,
	}
}

func (e *localClient) GetServicesConfig() *models.ServicesConfig {
	return e.config
}

func (e *localClient) GetEnvironmentConfig() *models.EnvironmentConfig {
	return e.environment
}

func (e *localClient) Initialize() error {

	logrus.Infof("Creating services client")

	// Anything defined in the environment config should be provided as a base
	// config for all services. To then be overridden by any specific service config
	// defined in the services config.

	// First lets figure out which platform and clients we want to configure
	// By default we'll use local.

	e.encrypt = e.configureEncryption()
	e.vault = e.configureVault()
	e.scheduler = e.configureScheduler()

	// Lets in parallel initialise all the internal services we need
	var wg sync.WaitGroup

	wg.Go(func() {

		logrus.Infof("Initializing encryption...")

		if e.encrypt != nil {
			if err := e.encrypt.Initialize(); err != nil {
				logrus.Errorf("Error initializing encryption: %v", err)
				e.encrypt = nil // Disable encryption if initialization fails
			}
		}
	})

	wg.Go(func() {

		logrus.Infof("Initializing vault...")

		if e.vault != nil {
			if err := e.vault.Initialize(); err != nil {
				logrus.Errorf("Error initializing vault: %v", err)
				e.vault = nil // Disable vault if initialization fails
			}
		}
	})

	wg.Go(func() {

		logrus.Infof("Initializing scheduler...")

		if e.scheduler != nil {
			if err := e.scheduler.Initialize(); err != nil {
				logrus.Errorf("Error initializing scheduler: %v", err)
				e.scheduler = nil // Disable scheduler if initialization fails
			}
		}
	})

	if e.config.LargeLanguageModel != nil {

		wg.Go(func() {

			logrus.Infof("Initializing large language model...")

			e.llm = e.configureLargeLanguageModel()
			if e.llm != nil {
				if err := e.llm.Initialize(); err != nil {
					logrus.Errorf("Error initializing large language model: %v", err)
					e.llm = nil // Disable LLM if initialization fails
				}
			}
		})

	}

	if e.config.Temporal != nil {

		wg.Go(func() {

			logrus.Infof("Initializing temporal...")

			e.temporal = temporal.NewTemporalClient(e.config.Temporal, e.environment.GetIdentifier())
			if err := e.temporal.Initialize(); err != nil {
				logrus.Errorf("Error initializing temporal: %v", err)
				e.temporal = nil // Disable temporal if initialization fails
			}
		})

	}

	logrus.Infof("Waiting for all services to initialize...")

	wg.Wait()

	logrus.Infof("All services initialized")

	return nil
}

func (e *localClient) Shutdown() error {
	if e.temporal.HasClient() {
		e.temporal.Shutdown()
	}
	return nil
}

func (e *localClient) GetLargeLanguageModel() models.LargeLanguageModelImpl {
	return e.llm
}

func (e *localClient) HasLargeLanguageModel() bool {
	return e.llm != nil
}

func (e *localClient) GetTemporal() models.TemporalImpl {
	return e.temporal
}

func (e *localClient) HasTemporal() bool {
	return e.temporal != nil
}

func (e *localClient) GetEncryption() models.EncryptionImpl {
	return e.encrypt
}

func (e *localClient) HasEncryption() bool {
	return e.encrypt != nil
}

func (e *localClient) GetVault() models.VaultImpl {
	return e.vault
}

func (e *localClient) HasVault() bool {
	return e.vault != nil
}

func (e *localClient) GetStorage() models.StorageImpl {
	return nil
}

func (e *localClient) HasStorage() bool {
	return false
}

func (e *localClient) GetScheduler() models.SchedulerImpl {
	return e.scheduler
}

func (e *localClient) HasScheduler() bool {
	return e.scheduler != nil
}
