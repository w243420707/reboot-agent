package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonTriggersRebootWhenMemoryExceedsThreshold(t *testing.T) {
	configDirectory := filepath.Join(t.TempDir(), "config")
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", configDirectory)
	t.Setenv("REBOOT_AGENT_RUNTIME_DIR", runtimeDirectory)

	if err := saveConfig(Config{Hours: 1, MemoryThreshold: 80, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	restoreDaemonTestHooks(t)
	memoryCheckInterval = time.Millisecond
	rootCheckFunc = func() bool { return true }
	memoryReader = func() (MemoryUsage, error) {
		return MemoryUsage{Percent: 81, Source: "test"}, nil
	}
	rebootCalls := 0
	rebootFunc = func() error {
		rebootCalls++
		return nil
	}

	if err := runDaemon(); err != nil {
		t.Fatal(err)
	}
	if rebootCalls != 1 {
		t.Fatalf("reboot called %d times, want 1", rebootCalls)
	}
}

func TestDaemonRecordsFailedMemoryReboot(t *testing.T) {
	configDirectory := filepath.Join(t.TempDir(), "config")
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", configDirectory)
	t.Setenv("REBOOT_AGENT_RUNTIME_DIR", runtimeDirectory)

	if err := saveConfig(Config{Hours: 1, MemoryThreshold: 80, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	restoreDaemonTestHooks(t)
	memoryCheckInterval = time.Millisecond
	rootCheckFunc = func() bool { return true }
	memoryReader = func() (MemoryUsage, error) {
		return MemoryUsage{Percent: 81, Source: "test"}, nil
	}
	rebootFunc = func() error {
		if err := saveConfig(Config{Hours: 1, MemoryThreshold: 80, Enabled: false}); err != nil {
			return err
		}
		return errors.New("operation not permitted")
	}

	if err := runDaemon(); err != nil {
		t.Fatal(err)
	}
	status, err := loadRuntimeStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.LastError != "operation not permitted" {
		t.Fatalf("last error = %q", status.LastError)
	}
}

func TestDaemonSkipsScheduledRebootWhenHoursDisabled(t *testing.T) {
	configDirectory := filepath.Join(t.TempDir(), "config")
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", configDirectory)
	t.Setenv("REBOOT_AGENT_RUNTIME_DIR", runtimeDirectory)

	if err := saveConfig(Config{Hours: 0, MemoryThreshold: 90, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	restoreDaemonTestHooks(t)
	memoryCheckInterval = time.Millisecond
	rootCheckFunc = func() bool { return true }
	checks := 0
	memoryReader = func() (MemoryUsage, error) {
		checks++
		if checks == 1 {
			if err := saveConfig(Config{Hours: 0, MemoryThreshold: 90, Enabled: false}); err != nil {
				t.Errorf("disable config: %v", err)
			}
		}
		return MemoryUsage{Percent: 10, Source: "test"}, nil
	}
	rebootCalls := 0
	rebootFunc = func() error {
		rebootCalls++
		return nil
	}

	if err := runDaemon(); err != nil {
		t.Fatal(err)
	}
	if rebootCalls != 0 {
		t.Fatalf("reboot called %d times with the schedule disabled", rebootCalls)
	}
	status, err := loadRuntimeStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.NextRebootAt != 0 {
		t.Fatalf("next reboot at = %d, want 0", status.NextRebootAt)
	}
}

func TestDaemonTriggersMemoryRebootWithScheduleDisabled(t *testing.T) {
	configDirectory := filepath.Join(t.TempDir(), "config")
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("REBOOT_AGENT_CONFIG_DIR", configDirectory)
	t.Setenv("REBOOT_AGENT_RUNTIME_DIR", runtimeDirectory)

	if err := saveConfig(Config{Hours: 0, MemoryThreshold: 80, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	restoreDaemonTestHooks(t)
	memoryCheckInterval = time.Millisecond
	rootCheckFunc = func() bool { return true }
	memoryReader = func() (MemoryUsage, error) {
		return MemoryUsage{Percent: 81, Source: "test"}, nil
	}
	rebootCalls := 0
	rebootFunc = func() error {
		rebootCalls++
		return nil
	}

	if err := runDaemon(); err != nil {
		t.Fatal(err)
	}
	if rebootCalls != 1 {
		t.Fatalf("reboot called %d times, want 1", rebootCalls)
	}
}

func restoreDaemonTestHooks(t *testing.T) {
	t.Helper()
	oldInterval := memoryCheckInterval
	oldReader := memoryReader
	oldReboot := rebootFunc
	oldRootCheck := rootCheckFunc
	t.Cleanup(func() {
		memoryCheckInterval = oldInterval
		memoryReader = oldReader
		rebootFunc = oldReboot
		rootCheckFunc = oldRootCheck
		_ = os.RemoveAll(runtimeDir())
	})
}
