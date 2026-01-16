package services

import (
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config/services/temporal"
	"github.com/thand-io/agent/internal/models"
)

type localClient struct {
	environment *models.EnvironmentConfig
	config      *models.ServicesConfig
	secret      *string

	encrypt   models.EncryptionImpl
	vault     models.VaultImpl
	scheduler models.SchedulerImpl
	llm       models.LargeLanguageModelImpl
	pki       models.PublicKeyInfrastructure
	temporal  models.TemporalImpl
}

func NewServicesClient(
	environment *models.EnvironmentConfig,
	config *models.ServicesConfig,
	secret *string,
) *localClient {
	return &localClient{
		environment: environment,
		config:      config,
		secret:      secret,
	}
}

func (e *localClient) GetServicesConfig() *models.ServicesConfig {
	return e.config
}

func (e *localClient) GetEnvironmentConfig() *models.EnvironmentConfig {
	return e.environment
}

func (e *localClient) GetSecret() string {
	if e.secret == nil {
		return common.DefaultServerSecret
	}
	return *e.secret
}

func (e *localClient) Initialize() error {

	logrus.Infof("Creating services client")

	// Anything defined in the environment config should be provided as a base
	// config for all services. To then be overridden by any specific service config
	// defined in the services config.

	// First lets figure out which platform and clients we want to configure
	// By default we'll use local.

	// These are code services and are not dependent on each other
	// so we can initialise them in parallel
	encryptionService := e.configureEncryption()
	vaultService := e.configureVault()
	schedulerService := e.configureScheduler()

	// Lets in parallel initialise all the internal services we need
	var wg sync.WaitGroup

	wg.Go(func() {

		logrus.Infof("Initializing encryption...")

		if encryptionService != nil {
			if err := encryptionService.Initialize(); err != nil {
				logrus.Errorf("Error initializing encryption: %v", err)
			} else {
				e.encrypt = encryptionService
			}
		}
	})

	wg.Go(func() {

		logrus.Infof("Initializing vault...")

		if vaultService != nil {
			if err := vaultService.Initialize(); err != nil {
				logrus.Errorf("Error initializing vault: %v", err)
			} else {
				e.vault = vaultService
			}
		}
	})

	wg.Go(func() {

		logrus.Infof("Initializing scheduler...")

		if schedulerService != nil {
			if err := schedulerService.Initialize(); err != nil {
				logrus.Errorf("Error initializing scheduler: %v", err)
			} else {
				e.scheduler = schedulerService
			}
		}
	})

	if e.config.LargeLanguageModel != nil {

		wg.Go(func() {

			logrus.Infof("Initializing large language model...")

			llmService := e.configureLargeLanguageModel()
			if llmService != nil {
				if err := llmService.Initialize(); err != nil {
					logrus.Errorf("Error initializing large language model: %v", err)
				} else {
					e.llm = llmService
				}
			}
		})

	}

	if e.config.PublicKeyInfrastructure != nil {

		wg.Go(func() {

			logrus.Infof("Initializing public key infrastructure...")

			pkiService := e.configurePublicKeyInfrastructure()
			if pkiService != nil {
				if err := e.pki.Initialize(e.encrypt, e.vault); err != nil {
					logrus.Errorf("Error initializing public key infrastructure: %v", err)
				} else {
					e.pki = pkiService
				}
			}
		})

	}

	if e.config.Temporal != nil {

		wg.Go(func() {

			logrus.Infof("Initializing temporal...")

			temporalService := temporal.NewTemporalClient(
				e.config.Temporal,
				e.environment.GetIdentifier(),
				e.vault,
			)
			if err := temporalService.Initialize(); err != nil {
				logrus.Errorf("Error initializing temporal: %v", err)
			} else {
				e.temporal = temporalService
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
