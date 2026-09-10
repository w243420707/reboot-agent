package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShutdownAgentKeepsConfigurationEnabled(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", filepath.Join(tempDir, "config"))
	t.Setenv("REBOOT_AGENT_RUNTIME_DIR", filepath.Join(tempDir, "runtime"))

	want := Config{Hours: 1, MemoryThreshold: 80, Enabled: true}
	if err := saveConfig(want); err != nil {
		t.Fatal(err)
	}
	if err := shutdownAgent(); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if _, err := os.Stat(statusPath()); !os.IsNotExist(err) {
		t.Fatalf("expected no runtime status, got %v", err)
	}
}
