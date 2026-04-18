package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestGetIAMPrincipalUsernames(t *testing.T) {
	provider := &awsProvider{}

	t.Run("email username includes prefix first", func(t *testing.T) {
		user := &models.User{
			Username: "testuser@thand.io",
			Email:    "testuser@thand.io",
		}

		assert.Equal(t,
			[]string{"testuser", "testuser@thand.io"},
			provider.getIAMPrincipalUsernames(user),
		)
		assert.Equal(t, "testuser", provider.getPreferredIAMUsername(user))
	})

	t.Run("explicit iam username stays preferred", func(t *testing.T) {
		user := &models.User{
			Username: "engineer",
			Email:    "engineer@thand.io",
		}

		assert.Equal(t,
			[]string{"engineer", "engineer@thand.io"},
			provider.getIAMPrincipalUsernames(user),
		)
		assert.Equal(t, "engineer", provider.getPreferredIAMUsername(user))
	})
}

func TestBuildUnboundAssumeRolePolicy(t *testing.T) {
	t.Run("removes email prefix match and falls back to deny", func(t *testing.T) {
		currentPolicy := PolicyDocument{
			Version: "2012-10-17",
			Statement: []Statement{
				{
					Effect: "Allow",
					Principal: map[string]any{
						"AWS": "arn:aws:iam::000000000000:user/testuser",
					},
					Action: "sts:AssumeRole",
				},
			},
		}
		userArns := map[string]struct{}{
			"arn:aws:iam::000000000000:user/testuser":          {},
			"arn:aws:iam::000000000000:user/testuser@thand.io": {},
		}

		updatedPolicy := buildUnboundAssumeRolePolicy(currentPolicy, userArns)
		assert.Equal(t, "2012-10-17", updatedPolicy.Version)
		assert.Len(t, updatedPolicy.Statement, 1)
		assert.Equal(t, "Deny", updatedPolicy.Statement[0].Effect)
		assert.Equal(t, "sts:AssumeRole", updatedPolicy.Statement[0].Action)
		assert.Equal(t, map[string]string{
			"AWS": "*",
		}, updatedPolicy.Statement[0].Principal)
	})

	t.Run("keeps non matching statements", func(t *testing.T) {
		currentPolicy := PolicyDocument{
			Version: "2012-10-17",
			Statement: []Statement{
				{
					Effect: "Allow",
					Principal: map[string]any{
						"AWS": "arn:aws:iam::000000000000:user/other-user",
					},
					Action: "sts:AssumeRole",
				},
				{
					Effect: "Allow",
					Principal: map[string]any{
						"AWS": "arn:aws:iam::000000000000:user/engineer",
					},
					Action: "sts:AssumeRole",
				},
			},
		}
		userArns := map[string]struct{}{
			"arn:aws:iam::000000000000:user/engineer":          {},
			"arn:aws:iam::000000000000:user/engineer@thand.io": {},
		}

		updatedPolicy := buildUnboundAssumeRolePolicy(currentPolicy, userArns)
		assert.Len(t, updatedPolicy.Statement, 1)
		assert.Equal(t, "Allow", updatedPolicy.Statement[0].Effect)
		assert.Equal(t, map[string]any{
			"AWS": "arn:aws:iam::000000000000:user/other-user",
		}, updatedPolicy.Statement[0].Principal)
	})
}

func TestRemoveIAMUserStatements(t *testing.T) {
	t.Run("removes matching statements", func(t *testing.T) {
		statements := []Statement{
			{
				Effect: "Allow",
				Principal: map[string]any{
					"AWS": "arn:aws:iam::000000000000:user/testuser",
				},
				Action: "sts:AssumeRole",
			},
		}
		userArns := map[string]struct{}{
			"arn:aws:iam::000000000000:user/testuser":          {},
			"arn:aws:iam::000000000000:user/testuser@thand.io": {},
		}

		remaining := removeIAMUserStatements(statements, userArns)
		assert.Empty(t, remaining)
	})
}
