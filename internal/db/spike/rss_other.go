//go:build bench && !linux

package main

// Off Linux there is no /proc/self/status, and this harness deliberately does
// NOT substitute a Go-runtime number for it.
//
// runtime.MemStats measures the Go heap. SQLite's page cache and its mmap'd
// database pages are not on the Go heap, so a MemStats figure would be a
// confident, precise, wrong answer to the exact question being asked — and it
// would be pasted into ADR-0001 as if it were an RSS measurement. The harness
// still runs the fixture and the workload here (useful for checking the harness
// itself), reports every RSS column as "n/a", and states on every output that
// the run does not satisfy the spike.
//
// Adding a macOS path (task_info / `ps -o rss=`) is a small, well-defined change
// if anyone needs one. UsArr is deployed on Linux, so it is not written
// speculatively.
const (
	rssAvailable = false
	rssSource    = "unavailable: no /proc/self/status on this GOOS. RSS columns read n/a"
)

func currentRSSKB() int64 { return 0 }
func peakRSSKB() int64    { return 0 }
func totalMemKB() int64   { return 0 }
