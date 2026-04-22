package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
)

func TestConfigDeviceIDCommandPrintsEffectiveDeviceID(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := configDeviceIDCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := common.GetDeviceID().String()

	if got != want {
		t.Fatalf("printed device ID = %q, want %q", got, want)
	}
}

func TestConfigDeviceIDCommandWritesOnlyToStdout(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)

	if err := configDeviceIDCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("stdout was empty")
	}
}
