//go:build linux

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func isRoot() bool {
	return os.Geteuid() == 0
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func signalProcess(pid int) error {
	if pid <= 0 {
		return os.ErrProcessDone
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

func startBackground() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()

	command := exec.Command(executable, "daemon")
	command.Stdin = devNull
	command.Stdout = devNull
	command.Stderr = devNull
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return command.Start()
}
