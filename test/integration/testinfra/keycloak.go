// Package testinfra provides shared container infrastructure for integration tests.
// This file adds optional Keycloak container support for UI E2E tests.
package testinfra

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// KeycloakPort is the default HTTP port for Keycloak.
	KeycloakPort = "8080"
	// KeycloakImage is the Keycloak container image.
	KeycloakImage = "quay.io/keycloak/keycloak:26.1"
	// KeycloakTestRealm is the realm name used for tests.
	KeycloakTestRealm = "thand-test"
	// KeycloakSharedHostname is the browser- and container-resolvable hostname used in frontend tests.
	KeycloakSharedHostname = "keycloak.test"
	// KeycloakAdminUser is the default admin user for Keycloak.
	KeycloakAdminUser = "admin"
	// KeycloakAdminPassword is the default admin password for Keycloak.
	KeycloakAdminPassword = "admin"
	keycloakBindMaxAttempts = 5
)

// startKeycloak starts a Keycloak container with the given realm import file.
// The realmFilePath should be the host path to the realm JSON export file.
func (infra *TestInfrastructure) startKeycloak(ctx context.Context, realmFilePath string) {
	infra.t.Log("Starting Keycloak container...")

	for attempt := 1; attempt <= keycloakBindMaxAttempts; attempt++ {
		allocatedPort, err := allocateFreeHostPort()
		require.NoError(infra.t, err, "Failed to allocate a free port for Keycloak")

		advertisedBaseURL := fmt.Sprintf("http://%s:%d", KeycloakSharedHostname, allocatedPort)
		req := testcontainers.ContainerRequest{
			Image: KeycloakImage,
			Cmd: []string{
				"start-dev",
				"--import-realm",
				"--health-enabled=true",
				"--hostname=" + advertisedBaseURL,
			},
			ExposedPorts: []string{KeycloakPort + "/tcp"},
			Env: map[string]string{
				"KEYCLOAK_ADMIN":          KeycloakAdminUser,
				"KEYCLOAK_ADMIN_PASSWORD": KeycloakAdminPassword,
				"KC_HTTP_PORT":            KeycloakPort,
			},
			HostConfigModifier: func(hc *dockercontainer.HostConfig) {
				hc.PortBindings = nat.PortMap{
					nat.Port(KeycloakPort + "/tcp"): []nat.PortBinding{
						{HostIP: "0.0.0.0", HostPort: strconv.Itoa(allocatedPort)},
					},
				}
			},
			Files: []testcontainers.ContainerFile{
				{
					HostFilePath:      realmFilePath,
					ContainerFilePath: "/opt/keycloak/data/import/thand-test-realm.json",
					FileMode:          0644,
				},
			},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort(KeycloakPort+"/tcp").WithStartupTimeout(120*time.Second),
				wait.ForHTTP("/realms/"+KeycloakTestRealm+"/.well-known/openid-configuration").
					WithPort(KeycloakPort+"/tcp").
					WithStartupTimeout(120*time.Second).
					WithPollInterval(2*time.Second),
			).WithDeadline(180 * time.Second),
		}

		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			if attempt < keycloakBindMaxAttempts && isKeycloakBindConflict(err) {
				infra.t.Logf("Keycloak port %d was claimed before Docker could bind it, retrying (%d/%d): %v", allocatedPort, attempt, keycloakBindMaxAttempts, err)
				continue
			}
			require.NoError(infra.t, err, "Failed to start Keycloak container")
		}

		infra.keycloakContainer = container
		infra.allocatedKeycloakPort = allocatedPort

		host, err := container.Host(ctx)
		require.NoError(infra.t, err, "Failed to get Keycloak host")
		mappedPort, err := container.MappedPort(ctx, KeycloakPort+"/tcp")
		require.NoError(infra.t, err, "Failed to get Keycloak port")

		infra.KeycloakEndpoint = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
		infra.KeycloakAdvertisedURL = advertisedBaseURL
		infra.t.Logf("Keycloak started at %s (advertised as %s)", infra.KeycloakEndpoint, infra.KeycloakAdvertisedURL)
		return
	}
}

func allocateFreeHostPort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func isKeycloakBindConflict(err error) bool {
	message := err.Error()
	return strings.Contains(message, "port is already allocated") ||
		strings.Contains(message, "bind: address already in use") ||
		strings.Contains(message, "Ports are not available")
}

func (infra *TestInfrastructure) keycloakPublicBaseURL() string {
	baseURL := infra.KeycloakAdvertisedURL
	if baseURL == "" {
		baseURL = infra.KeycloakEndpoint
	}
	return baseURL
}

// KeycloakOIDCIssuerURL returns the OIDC issuer URL for the test realm.
func (infra *TestInfrastructure) KeycloakOIDCIssuerURL() string {
	baseURL := infra.keycloakPublicBaseURL()
	return baseURL + "/realms/" + KeycloakTestRealm
}

// KeycloakSAMLMetadataURL returns the SAML IdP metadata URL for the test realm.
func (infra *TestInfrastructure) KeycloakSAMLMetadataURL() string {
	baseURL := infra.keycloakPublicBaseURL()
	return baseURL + "/realms/" + KeycloakTestRealm + "/protocol/saml/descriptor"
}
