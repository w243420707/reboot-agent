package main

import (
	"errors"
	"fmt"
	"os"
	"time"
)

var (
	memoryCheckInterval = 5 * time.Second
	memoryReader        = detectMemoryUsage
	rebootFunc          = syncAndReboot
	rootCheckFunc       = isRoot
)

func runDaemon() error {
	if !rootCheckFunc() {
		return errors.New("root privileges are required")
	}
	config, err := loadConfig()
	if err != nil {
		return err
	}
	if !config.Enabled {
		return nil
	}

	releaseInstance, err := acquireInstance()
	if err != nil {
		return err
	}
	defer releaseInstance()

	startedAt := time.Now()
	nextReboot, err := nextRebootAt(config.Hours, startedAt)
	if err != nil {
		return err
	}
	status := RuntimeStatus{
		PID:          os.Getpid(),
		StartedAt:    startedAt.Unix(),
		NextRebootAt: rebootAt(nextReboot),
		UpdatedAt:    startedAt.Unix(),
		LastEvent:    "agent started",
	}
	_ = saveRuntimeStatus(status)

	ticker := time.NewTicker(memoryCheckInterval)
	defer ticker.Stop()
	activeHours := config.Hours
	for checkTime := range ticker.C {
		latestConfig, configErr := loadConfig()
		if configErr != nil {
			if errors.Is(configErr, os.ErrNotExist) {
				status.LastEvent = "agent uninstalled"
				status.LastError = ""
				status.UpdatedAt = checkTime.Unix()
				_ = saveRuntimeStatus(status)
				return nil
			}
			status.LastError = fmt.Sprintf("read config: %v", configErr)
			status.UpdatedAt = checkTime.Unix()
			_ = saveRuntimeStatus(status)
			continue
		}
		if !latestConfig.Enabled {
			status.LastEvent = "agent stopped"
			status.UpdatedAt = checkTime.Unix()
			_ = saveRuntimeStatus(status)
			return nil
		}
		if latestConfig.Hours != activeHours {
			nextReboot, err = nextRebootAt(latestConfig.Hours, checkTime)
			if err != nil {
				return err
			}
			activeHours = latestConfig.Hours
			status.NextRebootAt = rebootAt(nextReboot)
			status.LastEvent = "schedule reloaded"
		}

		memory, memoryErr := memoryReader()
		if memoryErr != nil {
			status.LastError = fmt.Sprintf("memory detection: %v", memoryErr)
		} else {
			status.MemoryPercent = memory.Percent
			status.MemorySource = memory.Source
			status.LastError = ""
			if latestConfig.MemoryThreshold > 0 && memory.Percent > float64(latestConfig.MemoryThreshold) {
				status.NextRebootAt = 0
				status.LastEvent = fmt.Sprintf("memory threshold exceeded: %.2f%% > %d%%", memory.Percent, latestConfig.MemoryThreshold)
				status.LastError = ""
				status.UpdatedAt = checkTime.Unix()
				_ = saveRuntimeStatus(status)
				rebootErr := rebootFunc()
				if rebootErr != nil {
					status.LastEvent = "memory-triggered reboot failed"
					status.LastError = rebootErr.Error()
					status.NextRebootAt = rebootAt(nextReboot)
				}
				if rebootErr == nil {
					return nil
				}
			}
		}

		if !nextReboot.IsZero() && !checkTime.Before(nextReboot) {
			status.LastEvent = "scheduled reboot reached"
			status.LastError = ""
			status.UpdatedAt = checkTime.Unix()
			_ = saveRuntimeStatus(status)
			if rebootErr := rebootFunc(); rebootErr != nil {
				status.LastEvent = "scheduled reboot failed"
				status.LastError = rebootErr.Error()
				nextReboot, err = nextRebootAt(activeHours, checkTime)
				if err != nil {
					return err
				}
				status.NextRebootAt = nextReboot.Unix()
			} else {
				return nil
			}
		}

		status.UpdatedAt = checkTime.Unix()
		_ = saveRuntimeStatus(status)
	}
	return nil
}

func acquireInstance() (func(), error) {
	if err := os.MkdirAll(runtimeDir(), 0o755); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(pidPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if _, writeErr := fmt.Fprintf(file, "%d\n", os.Getpid()); writeErr != nil {
				file.Close()
				os.Remove(pidPath())
				return nil, writeErr
			}
			if closeErr := file.Close(); closeErr != nil {
				os.Remove(pidPath())
				return nil, closeErr
			}
			return func() { _ = os.Remove(pidPath()) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		pidContents, readErr := os.ReadFile(pidPath())
		if readErr == nil {
			var pid int
			if _, scanErr := fmt.Sscanf(string(pidContents), "%d", &pid); scanErr == nil && processAlive(pid) {
				return nil, fmt.Errorf("agent is already running with PID %d", pid)
			}
		}
		if removeErr := os.Remove(pidPath()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, removeErr
		}
	}
	return nil, errors.New("could not acquire agent lock")
}
