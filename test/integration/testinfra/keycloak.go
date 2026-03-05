// Package testinfra provides shared container infrastructure for integration tests.
// This file adds optional Keycloak container support for UI E2E tests.
package testinfra

import (
	"context"
	"fmt"
	"time"

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
	// KeycloakAdminUser is the default admin user for Keycloak.
	KeycloakAdminUser = "admin"
	// KeycloakAdminPassword is the default admin password for Keycloak.
	KeycloakAdminPassword = "admin"
)

// startKeycloak starts a Keycloak container with the given realm import file.
// The realmFilePath should be the host path to the realm JSON export file.
func (infra *TestInfrastructure) startKeycloak(ctx context.Context, realmFilePath string) {
	infra.t.Log("Starting Keycloak container...")

	req := testcontainers.ContainerRequest{
		Image: KeycloakImage,
		Cmd: []string{
			"start-dev",
			"--import-realm",
			"--health-enabled=true",
		},
		ExposedPorts: []string{KeycloakPort + "/tcp"},
		Env: map[string]string{
			"KEYCLOAK_ADMIN":          KeycloakAdminUser,
			"KEYCLOAK_ADMIN_PASSWORD": KeycloakAdminPassword,
			"KC_HTTP_PORT":            KeycloakPort,
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
	require.NoError(infra.t, err, "Failed to start Keycloak container")
	infra.keycloakContainer = container

	host, err := container.Host(ctx)
	require.NoError(infra.t, err, "Failed to get Keycloak host")
	mappedPort, err := container.MappedPort(ctx, KeycloakPort+"/tcp")
	require.NoError(infra.t, err, "Failed to get Keycloak port")

	infra.KeycloakEndpoint = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	infra.t.Logf("Keycloak started at %s", infra.KeycloakEndpoint)
}

// KeycloakOIDCIssuerURL returns the OIDC issuer URL for the test realm.
func (infra *TestInfrastructure) KeycloakOIDCIssuerURL() string {
	return infra.KeycloakEndpoint + "/realms/" + KeycloakTestRealm
}

// KeycloakSAMLMetadataURL returns the SAML IdP metadata URL for the test realm.
func (infra *TestInfrastructure) KeycloakSAMLMetadataURL() string {
	return infra.KeycloakEndpoint + "/realms/" + KeycloakTestRealm + "/protocol/saml/descriptor"
}
