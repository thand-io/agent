package cli

import (
	"github.com/sirupsen/logrus"
	cli "github.com/thand-io/agent/cmd/cli"
)

func main() {
	if err := cli.GetAgentCommand().Execute(); err != nil {
		logrus.Fatalf("Failed to execute command: %v", err)
	}
}
