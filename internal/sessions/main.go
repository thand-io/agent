package sessions

// Load Sessions from the local filesystem stored as yaml

import (
	"os"

	"github.com/thand-io/agent/internal/models"
	yaml "gopkg.in/yaml.v3"
)

func LoadSessionsFromFile(filePath string) (*models.LocalSessionConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var sessions models.LocalSessionConfig
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&sessions); err != nil {
		return nil, err
	}

	return &sessions, nil
}
