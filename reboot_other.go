//go:build !linux

package main

import "errors"

func syncAndReboot() error {
	return errors.New("direct reboot is only supported on Linux")
}
