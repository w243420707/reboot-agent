package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureDefaultsMemoryThresholdTo80(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", filepath.Join(tempDir, "config"))

	if err := configureInteractive(strings.NewReader("6\n\n"), &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hours != 6 || config.MemoryThreshold != 80 || !config.Enabled {
		t.Fatalf("unexpected default config: %+v", config)
	}
}

func TestConfigAllowsDisabledSchedule(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", filepath.Join(tempDir, "config"))

	want := Config{Hours: 0, MemoryThreshold: 80, Enabled: true}
	if err := saveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestConfigureAcceptsZeroHours(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", filepath.Join(tempDir, "config"))

	if err := configureInteractive(strings.NewReader("0\n\n"), &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hours != 0 || config.MemoryThreshold != 80 || !config.Enabled {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	oldDir := os.Getenv("REBOOT_AGENT_CONFIG_DIR")
	if err := os.Setenv("REBOOT_AGENT_CONFIG_DIR", filepath.Join(tempDir, "config")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("REBOOT_AGENT_CONFIG_DIR", oldDir) })

	want := Config{Hours: 12, MemoryThreshold: 80, Enabled: true}
	if err := saveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
