package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

func nextRebootAt(hours int, from time.Time) (time.Time, error) {
	if hours < 1 || hours > 8760 {
		return time.Time{}, fmt.Errorf("hours must be between 1 and 8760")
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
