package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type startupInfo struct {
	Type string
	Path string
}

func startAgent() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	config.Enabled = true
	if err := saveConfig(config); err != nil {
		return err
	}

	status, statusErr := loadRuntimeStatus()
	if statusErr == nil && processAlive(status.PID) {
		fmt.Printf("reboot-agent 已在运行（PID %d）。\n", status.PID)
		return nil
	}
	if err := startBackground(); err != nil {
		return err
	}
	fmt.Println("reboot-agent 已启动。")
	return nil
}

func stopAgent() error {
	config, err := loadConfig()
	if err != nil {
		return err
	}
	config.Enabled = false
	if err := saveConfig(config); err != nil {
		return err
	}
	if err := shutdownAgent(); err != nil {
		return err
	}
	fmt.Println("reboot-agent 已停止。")
	return nil
}

func shutdownAgent() error {
	status, statusErr := loadRuntimeStatus()
	if statusErr == nil && processAlive(status.PID) {
		if err := signalProcess(status.PID); err != nil {
			return err
		}
		return nil
	}
	if statusErr != nil && !errors.Is(statusErr, os.ErrNotExist) {
		return statusErr
	}
	return nil
}

func uninstallAgent() error {
	if _, err := os.Stat(configPath()); err == nil {
		if err := stopAgent(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := removeStartupEntry(); err != nil {
		return err
	}
	if err := os.RemoveAll(configDir()); err != nil {
		return err
	}
	if err := os.RemoveAll(runtimeDir()); err != nil {
		return err
	}

	executable, err := os.Executable()
	if err == nil && filepath.Base(executable) == "reboot-agent" {
		_ = os.Remove(executable)
	}
	fmt.Println("reboot-agent 已卸载。")
	return nil
}

func readStartupInfo() (startupInfo, error) {
	contents, err := os.ReadFile(startupPath())
	if err != nil {
		return startupInfo{}, err
	}
	values, err := parseKeyValue(contents)
	if err != nil {
		return startupInfo{}, err
	}
	if values["type"] == "" || values["path"] == "" {
		return startupInfo{}, errors.New("invalid startup descriptor")
	}
	return startupInfo{Type: values["type"], Path: values["path"]}, nil
}

func removeStartupEntry() error {
	info, err := readStartupInfo()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	switch info.Type {
	case "systemd":
		_ = runCommand("systemctl", "disable", "--now", "reboot-agent.service")
		_ = runCommand("systemctl", "daemon-reload")
		return os.Remove(info.Path)
	case "openwrt":
		_ = runCommand(info.Path, "disable")
		_ = runCommand(info.Path, "stop")
		return os.Remove(info.Path)
	case "openrc":
		_ = runCommand("rc-update", "del", "reboot-agent", "default")
		return os.Remove(info.Path)
	case "sysv":
		return os.Remove(info.Path)
	case "rc-local":
		return removeMarkedBlock(info.Path)
	case "inittab":
		return removeExactLine(info.Path, "ra:2345:respawn:")
	default:
		return fmt.Errorf("unsupported startup type %q", info.Type)
	}
}

func removeMarkedBlock(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(contents), "\n")
	result := make([]string, 0, len(lines))
	inside := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "# reboot-agent begin" {
			inside = true
			continue
		}
		if strings.TrimSpace(line) == "# reboot-agent end" {
			inside = false
			continue
		}
		if !inside {
			result = append(result, line)
		}
	}
	return writeAtomic(path, []byte(strings.Join(result, "\n")), 0o755)
}

func removeExactLine(path, phrase string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(contents), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, phrase) {
			continue
		}
		result = append(result, line)
	}
	return writeAtomic(path, []byte(strings.Join(result, "\n")), 0o644)
}

func runCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdout = nil
	command.Stderr = nil
	return command.Run()
}
