//go:build !linux

package main

import "errors"

func isRoot() bool { return false }

func processAlive(pid int) bool { return false }

func signalProcess(pid int) error { return errors.New("process signals are only supported on Linux") }

func startBackground() error { return errors.New("background agent is only supported on Linux") }
