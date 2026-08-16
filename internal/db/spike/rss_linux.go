//go:build bench && linux

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// On Linux the numbers come from the kernel, not from the Go runtime.
//
// runtime.MemStats cannot answer this question at all: SQLite's page cache and
// any mmap'd database pages live outside the Go heap, and mmap is one of the two
// settings the spike exists to decide. VmRSS is the process's current resident
// set — Go heap, SQLite page cache, mmap'd pages the process has touched, stacks
// and the binary's own text. VmHWM is the kernel's own high-water mark for the
// same figure, which is the only trustworthy "peak" here: sampling VmRSS from a
// goroutine would miss the spike between two samples.
const (
	rssAvailable = true
	rssSource    = "/proc/self/status VmRSS (current) and VmHWM (kernel high-water mark)"
)

// currentRSSKB returns VmRSS in KiB.
func currentRSSKB() int64 { return procStatusKB("VmRSS:") }

// peakRSSKB returns VmHWM in KiB.
func peakRSSKB() int64 { return procStatusKB("VmHWM:") }

func procStatusKB(field string) int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, field) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	return 0
}

// totalMemKB returns MemTotal from /proc/meminfo, in KiB.
func totalMemKB() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	return 0
}
