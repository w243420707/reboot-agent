package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Hours           int
	MemoryThreshold int
	Enabled         bool
}

func configDir() string {
	if directory := os.Getenv("REBOOT_AGENT_CONFIG_DIR"); directory != "" {
		return directory
	}
	return "/etc/reboot-agent"
}

func runtimeDir() string {
	if directory := os.Getenv("REBOOT_AGENT_RUNTIME_DIR"); directory != "" {
		return directory
	}
	return "/run/reboot-agent"
}

func configPath() string {
	return filepath.Join(configDir(), "config")
}

func startupPath() string {
	return filepath.Join(configDir(), "startup")
}

func pidPath() string {
	return filepath.Join(runtimeDir(), "reboot-agent.pid")
}

func statusPath() string {
	return filepath.Join(runtimeDir(), "status")
}

func loadConfig() (Config, error) {
	contents, err := os.ReadFile(configPath())
	if err != nil {
		return Config{}, err
	}

	values, err := parseKeyValue(contents)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	hours, err := requiredInt(values, "hours", 0, 8760)
	if err != nil {
		return Config{}, err
	}
	memoryThreshold, err := requiredInt(values, "memory_threshold", 0, 99)
	if err != nil {
		return Config{}, err
	}
	enabled, err := strconv.ParseBool(values["enabled"])
	if err != nil {
		return Config{}, fmt.Errorf("invalid enabled setting")
	}

	return Config{Hours: hours, MemoryThreshold: memoryThreshold, Enabled: enabled}, nil
}

func saveConfig(config Config) error {
	if config.Hours < 0 || config.Hours > 8760 {
		return errors.New("hours must be between 0 and 8760")
	}
	if config.MemoryThreshold < 0 || config.MemoryThreshold > 99 {
		return errors.New("memory threshold must be between 0 and 99")
	}

	contents := fmt.Sprintf("hours=%d\nmemory_threshold=%d\nenabled=%t\n", config.Hours, config.MemoryThreshold, config.Enabled)
	return writeAtomic(configPath(), []byte(contents), 0o600)
}

func parseKeyValue(contents []byte) (map[string]string, error) {
	values := make(map[string]string)
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("invalid line %q", line)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values, nil
}

func requiredInt(values map[string]string, key string, min, max int) (int, error) {
	raw, exists := values[key]
	if !exists {
		return 0, fmt.Errorf("missing %s", key)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return value, nil
}

func writeAtomic(path string, contents []byte, permission os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(directory, ".reboot-agent-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := temporary.Chmod(permission); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
