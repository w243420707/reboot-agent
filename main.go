package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}

	if runtime.GOOS != "linux" {
		fatalf("only Linux is supported")
	}

	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	command := os.Args[1]
	var err error
	switch command {
	case "daemon":
		err = runDaemon()
	case "configure", "set":
		if !isRoot() {
			err = fmt.Errorf("root privileges are required")
		} else {
			err = configureInteractive(os.Stdin, os.Stdout)
		}
	case "status":
		err = printStatus(os.Stdout)
	case "start":
		if !isRoot() {
			err = fmt.Errorf("root privileges are required")
		} else {
			err = startAgent()
		}
	case "stop":
		if !isRoot() {
			err = fmt.Errorf("root privileges are required")
		} else {
			err = stopAgent()
		}
	case "shutdown":
		if !isRoot() {
			err = fmt.Errorf("root privileges are required")
		} else {
			err = shutdownAgent()
		}
	case "uninstall":
		if !isRoot() {
			err = fmt.Errorf("root privileges are required")
		} else {
			err = uninstallAgent()
		}
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return
	default:
		printUsage(os.Stderr)
		err = fmt.Errorf("unknown command: %s", command)
	}

	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "reboot-agent: "+format+"\n", args...)
	os.Exit(1)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: reboot-agent <command>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  configure       Interactively configure restart hours and memory threshold")
	fmt.Fprintln(w, "  set             Alias for configure")
	fmt.Fprintln(w, "  status          Show configuration and current runtime status")
	fmt.Fprintln(w, "  start           Enable and start the agent")
	fmt.Fprintln(w, "  stop            Disable and stop the agent")
	fmt.Fprintln(w, "  uninstall       Remove the agent and its startup entry")
	fmt.Fprintln(w, "  daemon          Run the background worker (used by startup scripts)")
	fmt.Fprintln(w, "  shutdown        Stop the running process without changing the configuration")
	fmt.Fprintln(w, "  --version       Print version")
}

func configureInteractive(in io.Reader, out io.Writer) error {
	config, err := loadConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		config = Config{MemoryThreshold: 80, Enabled: true}
	}

	reader := bufio.NewReader(in)
	hours, err := promptInt(reader, out, "定时重启间隔（小时，1-8760）", config.Hours, 1, 8760, true)
	if err != nil {
		return err
	}
	memoryThreshold, err := promptInt(reader, out, "内存阈值（0 关闭；1-99 为百分比）", config.MemoryThreshold, 0, 99, false)
	if err != nil {
		return err
	}

	config.Hours = hours
	config.MemoryThreshold = memoryThreshold
	config.Enabled = true
	if err := saveConfig(config); err != nil {
		return err
	}

	fmt.Fprintf(out, "已保存：每 %d 小时定时重启，内存阈值 %s。\n", hours, memoryThresholdLabel(memoryThreshold))
	return nil
}

func promptInt(reader *bufio.Reader, out io.Writer, label string, current, min, max int, required bool) (int, error) {
	for {
		if current > 0 || (!required && current == 0) {
			fmt.Fprintf(out, "%s [当前 %d]: ", label, current)
		} else {
			fmt.Fprintf(out, "%s: ", label)
		}

		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		value := strings.TrimSpace(line)
		if value == "" {
			if current >= min && current <= max {
				return current, nil
			}
			if errors.Is(err, io.EOF) {
				return 0, errors.New("missing required value")
			}
			fmt.Fprintln(out, "请输入一个有效数字。")
			continue
		}

		parsed, parseErr := strconv.Atoi(value)
		if parseErr == nil && parsed >= min && parsed <= max {
			return parsed, nil
		}
		fmt.Fprintf(out, "请输入 %d 到 %d 之间的整数。\n", min, max)
		if errors.Is(err, io.EOF) {
			return 0, errors.New("invalid value")
		}
	}
}

func memoryThresholdLabel(value int) string {
	if value == 0 {
		return "已关闭"
	}
	return fmt.Sprintf("%d%%", value)
}
