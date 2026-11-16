package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// nextRebootAt returns the zero time when hours is 0, which disables the
// time-based schedule and leaves only the memory trigger active.
func nextRebootAt(hours int, from time.Time) (time.Time, error) {
	if hours == 0 {
		return time.Time{}, nil
	}
	if hours < 0 || hours > 8760 {
		return time.Time{}, fmt.Errorf("hours must be between 0 and 8760")
	}

	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return time.Time{}, fmt.Errorf("generate random delay: %w", err)
	}
	randomValue := binary.LittleEndian.Uint64(randomBytes[:])
	minutes := time.Duration(randomValue%59+1) * time.Minute
	seconds := time.Duration((randomValue/59)%60) * time.Second
	delay := time.Duration(hours)*time.Hour + minutes + seconds
	return from.Add(delay), nil
}

// rebootAt converts a schedule deadline into the status timestamp, where 0
// means there is no pending scheduled reboot.
func rebootAt(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.Unix()
}
