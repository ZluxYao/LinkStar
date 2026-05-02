package stun

import (
	"fmt"
	"testing"
)

func TestServiceEntryLogsKeepLatest15(t *testing.T) {
	entry := newServiceEntry(func() {}, "device", "service")

	for i := 0; i < 16; i++ {
		entry.setState(PhaseRestarting, 0, fmt.Sprintf("error-%02d", i))
	}

	snapshot := entry.snapshot("1-1")
	if len(snapshot.Logs) != maxServiceLogs {
		t.Fatalf("expected %d error logs, got %d", maxServiceLogs, len(snapshot.Logs))
	}
	if snapshot.Logs[0].Message != "error-01" {
		t.Fatalf("expected oldest retained error-01, got %q", snapshot.Logs[0].Message)
	}
	if snapshot.Logs[len(snapshot.Logs)-1].Message != "error-15" {
		t.Fatalf("expected newest retained error-15, got %q", snapshot.Logs[len(snapshot.Logs)-1].Message)
	}
}

func TestServiceEntryLogsSkipEmptyAndDuplicate(t *testing.T) {
	entry := newServiceEntry(func() {}, "device", "service")

	entry.setState(PhaseProbing, 0, "")
	entry.setState(PhaseRestarting, 0, "same error")
	entry.setState(PhaseProbing, 0, "same error")

	snapshot := entry.snapshot("1-1")
	if len(snapshot.Logs) != 1 {
		t.Fatalf("expected 1 error log, got %d", len(snapshot.Logs))
	}
	if snapshot.Logs[0].Message != "same error" {
		t.Fatalf("expected same error, got %q", snapshot.Logs[0].Message)
	}
}

func TestServiceEntrySnapshotCopiesLogs(t *testing.T) {
	entry := newServiceEntry(func() {}, "device", "service")
	entry.setState(PhaseRestarting, 0, "first error")

	snapshot := entry.snapshot("1-1")
	snapshot.Logs[0].Message = "mutated"

	nextSnapshot := entry.snapshot("1-1")
	if nextSnapshot.Logs[0].Message != "first error" {
		t.Fatalf("expected internal error log to be unchanged, got %q", nextSnapshot.Logs[0].Message)
	}
}
