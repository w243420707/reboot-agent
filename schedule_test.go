package main

import (
	"testing"
	"time"
)

func TestNextRebootAtStaysWithinRandomRange(t *testing.T) {
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		next, err := nextRebootAt(2, from)
		if err != nil {
			t.Fatalf("nextRebootAt returned error: %v", err)
		}
		delay := next.Sub(from)
		minimum := 2*time.Hour + time.Minute
		maximum := 2*time.Hour + 59*time.Minute + 59*time.Second
		if delay < minimum || delay > maximum {
			t.Fatalf("delay %s is outside %s..%s", delay, minimum, maximum)
		}
	}
}

func TestNextRebootAtDisablesScheduleForZeroHours(t *testing.T) {
	next, err := nextRebootAt(0, time.Now())
	if err != nil {
		t.Fatalf("nextRebootAt returned error: %v", err)
	}
	if !next.IsZero() {
		t.Fatalf("expected zero deadline, got %s", next)
	}
}

func TestNextRebootAtRejectsInvalidHours(t *testing.T) {
	if _, err := nextRebootAt(-1, time.Now()); err == nil {
		t.Fatal("expected invalid hours error")
	}
	if _, err := nextRebootAt(8761, time.Now()); err == nil {
		t.Fatal("expected invalid hours error")
	}
}
