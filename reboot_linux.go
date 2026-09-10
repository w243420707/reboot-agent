//go:build linux

package main

import "syscall"

func syncAndReboot() error {
	syscall.Sync()
	return syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
}
