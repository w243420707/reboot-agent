package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type MemoryUsage struct {
	UsedBytes  uint64
	TotalBytes uint64
	Percent    float64
	Source     string
}

var (
	procMeminfoPath = "/proc/meminfo"
	cgroupV2Root    = "/sys/fs/cgroup"
	cgroupV1Root    = "/sys/fs/cgroup/memory"
	procCgroupPath  = "/proc/self/cgroup"
	mountInfoPath   = "/proc/self/mountinfo"
)

func detectMemoryUsage() (MemoryUsage, error) {
	if memory, err := readCgroupV2Memory(); err == nil {
		return memory, nil
	}
	if memory, err := readCgroupV1Memory(); err == nil {
		return memory, nil
	}
	return readProcMemory()
}

func readProcMemory() (MemoryUsage, error) {
	file, err := os.Open(procMeminfoPath)
	if err != nil {
		return MemoryUsage{}, err
	}
	defer file.Close()

	values := make(map[string]uint64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseMeminfoLine(scanner.Text())
		if ok {
			values[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return MemoryUsage{}, err
	}

	ramTotal, ok := values["MemTotal"]
	if !ok || ramTotal == 0 {
		return MemoryUsage{}, errors.New("MemTotal is unavailable")
	}
	ramAvailable, hasMemAvailable := values["MemAvailable"]
	if !hasMemAvailable {
		ramAvailable = values["MemFree"] + values["Buffers"] + values["Cached"] + values["SReclaimable"]
		shmem := values["Shmem"]
		if shmem < ramAvailable {
			ramAvailable -= shmem
		} else {
			ramAvailable = 0
		}
	}
	if ramAvailable > ramTotal {
		ramAvailable = ramTotal
	}
	swapTotal := values["SwapTotal"]
	swapFree := values["SwapFree"]
	if swapFree > swapTotal {
		swapFree = swapTotal
	}

	used := (ramTotal - ramAvailable) + (swapTotal - swapFree)
	total := ramTotal + swapTotal
	return makeMemoryUsage(used, total, "procfs (RAM + swap)")
}

func parseMeminfoLine(line string) (string, uint64, bool) {
	key, rawValue, found := strings.Cut(line, ":")
	if !found {
		return "", 0, false
	}
	fields := strings.Fields(rawValue)
	if len(fields) == 0 {
		return "", 0, false
	}
	value, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return "", 0, false
	}
	if len(fields) > 1 && fields[1] == "kB" {
		value *= 1024
	}
	return strings.TrimSpace(key), value, true
}

func readCgroupV2Memory() (MemoryUsage, error) {
	root := discoverCgroupRoot("", true)
	if _, err := os.Stat(filepath.Join(root, "cgroup.controllers")); err != nil {
		return MemoryUsage{}, err
	}

	ramLimit, finiteRAMLimit, err := readCgroupLimit(filepath.Join(root, "memory.max"))
	if err != nil || !finiteRAMLimit {
		return MemoryUsage{}, errors.New("cgroup v2 has no finite memory limit")
	}
	ramUsed, err := readUintFile(filepath.Join(root, "memory.current"))
	if err != nil {
		return MemoryUsage{}, err
	}

	swapLimit, finiteSwapLimit, err := readCgroupLimit(filepath.Join(root, "memory.swap.max"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return MemoryUsage{}, err
		}
		swapLimit = 0
		finiteSwapLimit = true
	}
	swapUsed, err := readUintFile(filepath.Join(root, "memory.swap.current"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return MemoryUsage{}, err
		}
		swapUsed = 0
	}
	if !finiteSwapLimit {
		hostMemory, hostErr := readProcMemoryParts()
		if hostErr != nil {
			return MemoryUsage{}, hostErr
		}
		swapLimit = hostMemory.swapTotal
		if swapUsed == 0 {
			swapUsed = hostMemory.swapUsed
		}
	}
	if swapUsed > swapLimit {
		swapUsed = swapLimit
	}

	return makeMemoryUsage(ramUsed+swapUsed, ramLimit+swapLimit, "cgroup v2 (RAM + swap)")
}

func readCgroupV1Memory() (MemoryUsage, error) {
	root := discoverCgroupRoot("memory", false)
	usage, err := readUintFile(filepath.Join(root, "memory.usage_in_bytes"))
	if err != nil {
		return MemoryUsage{}, err
	}
	ramLimit, err := readFiniteCgroupLimit(filepath.Join(root, "memory.limit_in_bytes"))
	if err != nil {
		return MemoryUsage{}, err
	}

	memswUsage, memswUsageErr := readUintFile(filepath.Join(root, "memory.memsw.usage_in_bytes"))
	memswLimit, memswLimitErr := readFiniteCgroupLimit(filepath.Join(root, "memory.memsw.limit_in_bytes"))
	if memswUsageErr == nil && memswLimitErr == nil {
		return makeMemoryUsage(memswUsage, memswLimit, "cgroup v1 memsw (RAM + swap)")
	}

	return makeMemoryUsage(usage, ramLimit, "cgroup v1 (swap unavailable)")
}

type procMemoryParts struct {
	swapTotal uint64
	swapUsed  uint64
}

func readProcMemoryParts() (procMemoryParts, error) {
	file, err := os.Open(procMeminfoPath)
	if err != nil {
		return procMemoryParts{}, err
	}
	defer file.Close()

	values := make(map[string]uint64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseMeminfoLine(scanner.Text())
		if ok {
			values[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return procMemoryParts{}, err
	}
	swapTotal := values["SwapTotal"]
	swapFree := values["SwapFree"]
	if swapFree > swapTotal {
		swapFree = swapTotal
	}
	return procMemoryParts{swapTotal: swapTotal, swapUsed: swapTotal - swapFree}, nil
}

func readCgroupLimit(path string) (uint64, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, false, err
	}
	value := strings.TrimSpace(string(contents))
	if value == "max" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if parsed >= ^uint64(0)/2 {
		return 0, false, nil
	}
	return parsed, true, err
}

func readFiniteCgroupLimit(path string) (uint64, error) {
	value, finite, err := readCgroupLimit(path)
	if err != nil {
		return 0, err
	}
	if !finite || value >= ^uint64(0)/2 {
		return 0, fmt.Errorf("unlimited cgroup memory limit")
	}
	return value, nil
}

func readUintFile(path string) (uint64, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(contents)), 10, 64)
}

func makeMemoryUsage(used, total uint64, source string) (MemoryUsage, error) {
	if total == 0 || used > total {
		return MemoryUsage{}, errors.New("invalid memory totals")
	}
	return MemoryUsage{
		UsedBytes:  used,
		TotalBytes: total,
		Percent:    float64(used) * 100 / float64(total),
		Source:     source,
	}, nil
}

type cgroupMount struct {
	root       string
	mountPoint string
	filesystem string
	options    string
}

func discoverCgroupRoot(controller string, v2 bool) string {
	fallback := cgroupV1Root
	if v2 {
		fallback = cgroupV2Root
	}

	cgroupPath, err := currentCgroupPath(controller, v2)
	if err != nil {
		return fallback
	}
	mounts, err := readCgroupMounts()
	if err != nil {
		return fallback
	}
	for _, mount := range mounts {
		if v2 && mount.filesystem != "cgroup2" {
			continue
		}
		if !v2 && (mount.filesystem != "cgroup" || !containsController(mount.options, controller)) {
			continue
		}
		root := joinCgroupPath(mount.root, mount.mountPoint, cgroupPath)
		if _, err := os.Stat(root); err == nil {
			return root
		}
	}
	return fallback
}

func currentCgroupPath(controller string, v2 bool) (string, error) {
	contents, err := os.ReadFile(procCgroupPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		if v2 && parts[0] == "0" {
			return unescapeMountPath(parts[2]), nil
		}
		if !v2 && containsController(parts[1], controller) {
			return unescapeMountPath(parts[2]), nil
		}
	}
	return "", errors.New("current cgroup path not found")
}

func readCgroupMounts() ([]cgroupMount, error) {
	contents, err := os.ReadFile(mountInfoPath)
	if err != nil {
		return nil, err
	}
	var mounts []cgroupMount
	for _, line := range strings.Split(string(contents), "\n") {
		before, after, found := strings.Cut(line, " - ")
		if !found {
			continue
		}
		beforeFields := strings.Fields(before)
		afterFields := strings.Fields(after)
		if len(beforeFields) < 6 || len(afterFields) < 3 {
			continue
		}
		filesystem := afterFields[0]
		if filesystem != "cgroup" && filesystem != "cgroup2" {
			continue
		}
		mounts = append(mounts, cgroupMount{
			root:       unescapeMountPath(beforeFields[3]),
			mountPoint: unescapeMountPath(beforeFields[4]),
			filesystem: filesystem,
			options:    afterFields[2],
		})
	}
	return mounts, nil
}

func joinCgroupPath(root, mountPoint, relative string) string {
	relative = strings.TrimPrefix(relative, "/")
	root = strings.TrimSuffix(root, "/")
	if root != "" && root != "/" && strings.HasPrefix(relative, strings.TrimPrefix(root, "/")) {
		relative = strings.TrimPrefix(relative, strings.TrimPrefix(root, "/"))
		relative = strings.TrimPrefix(relative, "/")
	}
	if relative == "" {
		return mountPoint
	}
	return filepath.Join(mountPoint, relative)
}

func containsController(options, controller string) bool {
	for _, option := range strings.Split(options, ",") {
		if option == controller {
			return true
		}
	}
	return false
}

func unescapeMountPath(value string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(value)
}
