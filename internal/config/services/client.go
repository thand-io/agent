package services

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config/services/temporal"
	"github.com/thand-io/agent/internal/models"
)

type localClient struct {
	config models.ConfigImpl

	Analytics models.Analytics
	encrypt   models.EncryptionImpl
	vault     models.VaultImpl
	scheduler models.SchedulerImpl
	llm       models.LargeLanguageModelImpl
	pki       models.PublicKeyInfrastructure
	temporal  models.TemporalImpl

	mu sync.Mutex
}

func NewServicesClient(config models.ConfigImpl) *localClient {
	return &localClient{
		config: config,
	}
}

func (e *localClient) GetServicesConfig() *models.ServicesConfig {
	return e.config.GetServicesConfig()
}

func (e *localClient) GetEnvironmentConfig() *models.EnvironmentConfig {
	return e.config.GetEnvironmentConfig()
}

func (e *localClient) GetSecret() string {
	return e.config.GetSecret()
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
	var wg sync.WaitGroup

	wg.Go(func() {
		e.ReloadAnalytics()
	})

	wg.Go(func() {
		e.ReloadEncryption()
	})

	wg.Go(func() {
		e.ReloadVault()
	})

	wg.Go(func() {
		e.ReloadScheduler()
	})

	servicesConfig := e.config.GetServicesConfig()
	if servicesConfig != nil && servicesConfig.GetLargeLanguageModelConfig() != nil {
		wg.Go(func() {
			e.ReloadLargeLanguageModel()
		})
	}

	if servicesConfig != nil && servicesConfig.GetPublicKeyInfrastructureConfig() != nil {
		wg.Go(func() {
			e.ReloadPublicKeyInfrastructure()
		})
	}

	if servicesConfig != nil && servicesConfig.GetTemporalConfig() != nil {
		wg.Go(func() {
			e.ReloadTemporal()
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

func (e *localClient) GetAnalytics() models.Analytics {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Analytics
}

func (e *localClient) HasAnalytics() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Analytics != nil
}

func (e *localClient) GetLargeLanguageModel() models.LargeLanguageModelImpl {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.llm
}

func (e *localClient) HasLargeLanguageModel() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.llm != nil
}

func (e *localClient) GetTemporal() models.TemporalImpl {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.temporal
}

func (e *localClient) HasTemporal() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.temporal != nil
}

func (e *localClient) GetEncryption() models.EncryptionImpl {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.encrypt
}

func (e *localClient) HasEncryption() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.encrypt != nil
}

func (e *localClient) GetVault() models.VaultImpl {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.vault
}

func (e *localClient) HasVault() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.vault != nil
}

func (e *localClient) GetStorage() models.StorageImpl {
	e.mu.Lock()
	defer e.mu.Unlock()
	return nil
}

func (e *localClient) HasStorage() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return false
}

func (e *localClient) GetScheduler() models.SchedulerImpl {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.scheduler
}

func (e *localClient) HasScheduler() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.scheduler != nil
}

func (e *localClient) ReloadAnalytics() error {

	if e.Analytics != nil {
		logrus.Infoln("Reloading analytics service...")
		e.mu.Lock()
		e.Analytics = nil
		e.mu.Unlock()
	}

	logrus.Infof("Initializing Analytics...")

	analyticsService := e.configureAnalytics()
	if analyticsService != nil {
		if err := analyticsService.Initialize(); err != nil {
			logrus.Errorf("Error initializing Analytics: %v", err)
			return err
		} else {
			e.mu.Lock()
			e.Analytics = analyticsService
			e.mu.Unlock()
		}
	}

	return nil
}

func (e *localClient) ReloadEncryption() error {

	if e.encrypt != nil {
		logrus.Infoln("Reloading encryption service...")
		e.mu.Lock()
		e.encrypt = nil
		e.mu.Unlock()
	}

	logrus.Infof("Initializing encryption...")

	encryptionService := e.configureEncryption()
	if encryptionService != nil {
		if err := encryptionService.Initialize(); err != nil {
			logrus.Errorf("Error initializing encryption: %v", err)
			return err
		} else {
			e.mu.Lock()
			e.encrypt = encryptionService
			e.mu.Unlock()
		}
	}

	return nil
}

func (e *localClient) ReloadVault() error {

	if e.vault != nil {
		logrus.Infoln("Reloading vault service...")
		e.mu.Lock()
		e.vault = nil
		e.mu.Unlock()
	}

	logrus.Infof("Initializing vault...")

	vaultService := e.configureVault()
	if vaultService != nil {
		if err := vaultService.Initialize(); err != nil {
			logrus.Errorf("Error initializing vault: %v", err)
			return err
		} else {
			e.mu.Lock()
			e.vault = vaultService
			e.mu.Unlock()
		}
	}

	return nil
}

func (e *localClient) ReloadScheduler() error {

	if e.scheduler != nil {
		logrus.Infoln("Reloading scheduler service...")
		e.mu.Lock()
		e.scheduler = nil
		e.mu.Unlock()
	}

	logrus.Infof("Initializing scheduler...")

	schedulerService := e.configureScheduler()
	if schedulerService != nil {
		if err := schedulerService.Initialize(); err != nil {
			logrus.Errorf("Error initializing scheduler: %v", err)
			return err
		} else {
			e.mu.Lock()
			e.scheduler = schedulerService
			e.mu.Unlock()
		}
	}

	return nil
}

func (e *localClient) ReloadLargeLanguageModel() error {

	if e.llm != nil {
		logrus.Infoln("Reloading large language model service...")
		e.mu.Lock()
		e.llm = nil
		e.mu.Unlock()
	}

	logrus.Infof("Initializing large language model...")

	llmService := e.configureLargeLanguageModel()
	if llmService != nil {
		if err := llmService.Initialize(); err != nil {
			logrus.Errorf("Error initializing large language model: %v", err)
			return err
		} else {
			e.mu.Lock()
			e.llm = llmService
			e.mu.Unlock()
		}
	}

	return nil
}

func (e *localClient) ReloadPublicKeyInfrastructure() error {

	if e.pki != nil {
		logrus.Infoln("Reloading public key infrastructure service...")
		e.mu.Lock()
		e.pki = nil
		e.mu.Unlock()
	}

	// Fist check that we have encryption and vault services available
	if !e.HasEncryption() || !e.HasVault() {
		logrus.Infof("Skipping public key infrastructure initialization as encryption or vault service is not available")
		return nil
	}

	logrus.Infof("Initializing public key infrastructure...")

	pkiService := e.configurePublicKeyInfrastructure()
	if pkiService != nil {
		if err := pkiService.Initialize(e.GetEncryption(), e.GetVault()); err != nil {
			logrus.Errorf("Error initializing public key infrastructure: %v", err)
			return err
		} else {
			e.mu.Lock()
			e.pki = pkiService
			e.mu.Unlock()
		}
	}

	return nil
}

func (e *localClient) ReloadTemporal() error {

	if e.temporal != nil {

		logrus.Infoln("Reloading temporal service...")
		err := e.temporal.Shutdown()

		if err != nil {
			logrus.WithError(err).Errorf("Failed to shutdown existing temporal service: %v", err)
			return err
		}

		e.mu.Lock()
		e.temporal = nil
		e.mu.Unlock()
	}

	logrus.Infof("Initializing temporal...")

	// Determine task queue based on mode:
	// - Server: shared default task queue
	// - Agent / Client: per-client task queue derived from the client identifier
	taskQueue := temporal.DefaultTaskQueue
	if e.config.IsAgent() || e.config.IsClient() {
		taskQueue = common.GetClientIdentifier().String()
	}

	logrus.WithField("taskQueue", taskQueue).Info("Configuring Temporal worker")

	// Get Temporal config from services
	servicesConfig := e.config.GetServicesConfig()

	if servicesConfig == nil {
		return fmt.Errorf("Services config is missing")
	}

	temporalConfig := servicesConfig.GetTemporalConfig()

	temporalService := temporal.NewTemporalClient(
		temporalConfig,
		e.vault,
		taskQueue,
	)
	if err := temporalService.Initialize(); err != nil {
		logrus.Errorf("Error initializing temporal: %v", err)
		return err
	}

	e.mu.Lock()
	e.temporal = temporalService
	e.mu.Unlock()

	return nil
}
