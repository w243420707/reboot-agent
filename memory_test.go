package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMeminfoLine(t *testing.T) {
	key, value, ok := parseMeminfoLine("MemTotal:       1024 kB")
	if !ok || key != "MemTotal" || value != 1024*1024 {
		t.Fatalf("got %q, %d, %t", key, value, ok)
	}
}

func TestReadProcMemoryUsesRAMAndSwap(t *testing.T) {
	tempDir := t.TempDir()
	meminfo := filepath.Join(tempDir, "meminfo")
	if err := os.WriteFile(meminfo, []byte("MemTotal: 100 kB\nMemAvailable: 20 kB\nSwapTotal: 50 kB\nSwapFree: 10 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldPath := procMeminfoPath
	procMeminfoPath = meminfo
	t.Cleanup(func() { procMeminfoPath = oldPath })

	memory, err := readProcMemory()
	if err != nil {
		t.Fatal(err)
	}
	if memory.UsedBytes != 120*1024 || memory.TotalBytes != 150*1024 || memory.Percent != 80 {
		t.Fatalf("unexpected memory usage: %+v", memory)
	}
}

func TestReadProcMemoryFallsBackWhenMemAvailableIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	meminfo := filepath.Join(tempDir, "meminfo")
	if err := os.WriteFile(meminfo, []byte("MemTotal: 100 kB\nMemFree: 20 kB\nBuffers: 10 kB\nCached: 20 kB\nSReclaimable: 5 kB\nShmem: 5 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldPath := procMeminfoPath
	procMeminfoPath = meminfo
	t.Cleanup(func() { procMeminfoPath = oldPath })

	memory, err := readProcMemory()
	if err != nil {
		t.Fatal(err)
	}
	if memory.UsedBytes != 50*1024 || memory.TotalBytes != 100*1024 || memory.Percent != 50 {
		t.Fatalf("unexpected memory usage: %+v", memory)
	}
}

func TestMakeMemoryUsageRejectsInvalidTotals(t *testing.T) {
	if _, err := makeMemoryUsage(2, 1, "test"); err == nil {
		t.Fatal("expected invalid totals error")
	}
}

func TestReadCgroupV2MemoryUsesMemoryAndSwapLimits(t *testing.T) {
	tempDir := t.TempDir()
	writeCgroupFile(t, tempDir, "cgroup.controllers", "memory")
	writeCgroupFile(t, tempDir, "memory.current", "81920")
	writeCgroupFile(t, tempDir, "memory.max", "102400")
	writeCgroupFile(t, tempDir, "memory.swap.current", "10240")
	writeCgroupFile(t, tempDir, "memory.swap.max", "51200")

	oldRoot, oldProc := cgroupV2Root, procCgroupPath
	cgroupV2Root, procCgroupPath = tempDir, filepath.Join(tempDir, "missing-cgroup")
	t.Cleanup(func() {
		cgroupV2Root, procCgroupPath = oldRoot, oldProc
	})

	memory, err := readCgroupV2Memory()
	if err != nil {
		t.Fatal(err)
	}
	if memory.UsedBytes != 92160 || memory.TotalBytes != 153600 || memory.Percent != 60 {
		t.Fatalf("unexpected memory usage: %+v", memory)
	}
}

func TestReadCgroupV1MemoryUsesMemsw(t *testing.T) {
	tempDir := t.TempDir()
	writeCgroupFile(t, tempDir, "memory.usage_in_bytes", "81920")
	writeCgroupFile(t, tempDir, "memory.limit_in_bytes", "102400")
	writeCgroupFile(t, tempDir, "memory.memsw.usage_in_bytes", "102400")
	writeCgroupFile(t, tempDir, "memory.memsw.limit_in_bytes", "204800")

	oldRoot, oldProc := cgroupV1Root, procCgroupPath
	cgroupV1Root, procCgroupPath = tempDir, filepath.Join(tempDir, "missing-cgroup")
	t.Cleanup(func() {
		cgroupV1Root, procCgroupPath = oldRoot, oldProc
	})

	memory, err := readCgroupV1Memory()
	if err != nil {
		t.Fatal(err)
	}
	if memory.UsedBytes != 102400 || memory.TotalBytes != 204800 || memory.Percent != 50 {
		t.Fatalf("unexpected memory usage: %+v", memory)
	}
}

func writeCgroupFile(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
