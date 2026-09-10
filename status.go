package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type RuntimeStatus struct {
	PID           int
	StartedAt     int64
	NextRebootAt  int64
	LastEvent     string
	LastError     string
	MemoryPercent float64
	MemorySource  string
	UpdatedAt     int64
}

func saveRuntimeStatus(status RuntimeStatus) error {
	contents := fmt.Sprintf(
		"pid=%d\nstarted_at=%d\nnext_reboot_at=%d\nlast_event=%s\nlast_error=%s\nmemory_percent=%.2f\nmemory_source=%s\nupdated_at=%d\n",
		status.PID,
		status.StartedAt,
		status.NextRebootAt,
		sanitizeStatusValue(status.LastEvent),
		sanitizeStatusValue(status.LastError),
		status.MemoryPercent,
		sanitizeStatusValue(status.MemorySource),
		status.UpdatedAt,
	)
	return writeAtomic(statusPath(), []byte(contents), 0o644)
}

func loadRuntimeStatus() (RuntimeStatus, error) {
	contents, err := os.ReadFile(statusPath())
	if err != nil {
		return RuntimeStatus{}, err
	}
	values, err := parseKeyValue(contents)
	if err != nil {
		return RuntimeStatus{}, err
	}

	status := RuntimeStatus{
		LastEvent:    values["last_event"],
		LastError:    values["last_error"],
		MemorySource: values["memory_source"],
	}
	if status.PID, err = optionalInt(values, "pid"); err != nil {
		return RuntimeStatus{}, err
	}
	if status.StartedAt, err = optionalInt64(values, "started_at"); err != nil {
		return RuntimeStatus{}, err
	}
	if status.NextRebootAt, err = optionalInt64(values, "next_reboot_at"); err != nil {
		return RuntimeStatus{}, err
	}
	if status.UpdatedAt, err = optionalInt64(values, "updated_at"); err != nil {
		return RuntimeStatus{}, err
	}
	if raw := values["memory_percent"]; raw != "" {
		status.MemoryPercent, err = strconv.ParseFloat(raw, 64)
		if err != nil {
			return RuntimeStatus{}, err
		}
	}
	return status, nil
}

func optionalInt(values map[string]string, key string) (int, error) {
	if values[key] == "" {
		return 0, nil
	}
	return strconv.Atoi(values[key])
}

func optionalInt64(values map[string]string, key string) (int64, error) {
	if values[key] == "" {
		return 0, nil
	}
	return strconv.ParseInt(values[key], 10, 64)
}

func sanitizeStatusValue(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.TrimSpace(value)
}

func printStatus(out io.Writer) error {
	config, err := loadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("the agent has not been configured")
		}
		return err
	}

	fmt.Fprintf(out, "状态：%s\n", enabledLabel(config.Enabled))
	fmt.Fprintf(out, "定时重启：每 %d 小时，加随机 1-59 分钟和 0-59 秒\n", config.Hours)
	fmt.Fprintf(out, "内存阈值：%s\n", memoryThresholdLabel(config.MemoryThreshold))

	if memory, memoryErr := detectMemoryUsage(); memoryErr == nil {
		fmt.Fprintf(out, "当前内存：%.2f%%（%s）\n", memory.Percent, memory.Source)
	} else {
		fmt.Fprintf(out, "当前内存：读取失败（%v）\n", memoryErr)
	}

	status, err := loadRuntimeStatus()
	if errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(out, "运行状态：当前未启动")
		return nil
	}
	if err != nil {
		return fmt.Errorf("read runtime status: %w", err)
	}

	fmt.Fprintf(out, "运行状态：%s\n", runningLabel(status.PID))
	if status.NextRebootAt > 0 {
		fmt.Fprintf(out, "下次定时重启：%s\n", time.Unix(status.NextRebootAt, 0).Format(time.RFC3339))
	}
	if status.LastEvent != "" {
		fmt.Fprintf(out, "上次事件：%s\n", status.LastEvent)
	}
	if status.LastError != "" {
		fmt.Fprintf(out, "上次错误：%s\n", status.LastError)
	}
	return nil
}

func enabledLabel(enabled bool) string {
	if enabled {
		return "已启用"
	}
	return "已停止"
}

func runningLabel(pid int) string {
	if pid > 0 && processAlive(pid) {
		return fmt.Sprintf("运行中（PID %d）", pid)
	}
	return "未运行"
}
