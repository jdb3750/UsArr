//go:build bench

// Command spike measures UsArr's resident-set size on a 500,000-row SQLite
// database, across candidate values of the two pragmas that are still marked
// "pending measurement".
//
// WHY THIS EXISTS. ARCHITECTURE §13's idle-RSS budget was justified by citing
// Navidrome; Navidrome uses a cgo SQLite driver and UsArr does not, so the
// budget rested on nothing measured (ADR-0001, correction 2). reference/sync.md
// §6 marks cache_size and mmap_size "pending measurement" for the same reason:
// under ncruces/go-sqlite3 it was undetermined whether cache_size is
// per-connection or shared, and whether mmap_size does anything at all. With a
// read pool of NumCPU*2 those two questions are worth tens of megabytes.
//
// It is NOT architecture-specific. Memory behaviour differs by architecture,
// page size and core count, so a result belongs to the machine that produced it
// — the report says which one that was. Run it wherever UsArr will actually run;
// it matters most where RAM is scarce.
//
// HOW TO RUN IT: `make bench-rss`. See docs/DEVELOPMENT.md §5.
//
// WHAT IT IS NOT. It is not a latency benchmark and it is not in `make bench`,
// which runs Go benchmarks; it is not in `make check` and must never join it. It
// is a measurement tool with one output: a table to paste into an ADR.
//
// PROCESS-PER-CELL, deliberately. VmHWM is a per-process high-water mark and it
// never falls, so measuring nine pragma combinations in one process would give
// one peak and eight lies. Each row of the sweep is a fresh child process
// running this same binary with -child=read.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jdb3750/UsArr/internal/db"
)

// resultPrefix marks the one line of child stdout the parent parses.
const resultPrefix = "SPIKE-RESULT "

// The shipped defaults, so the report can point at the row that is current
// behaviour. Kept as strings because that is what goes into the DSN.
const (
	shippedCacheSize = "-32000"    // 32 MiB, reference/sync.md §6
	shippedMmapSize  = "134217728" // 128 MiB
)

// writeBurstBatches is how many §7.7-sized batches the peak measurement writes.
// 5 × 2000 = 10,000 rows: enough that the writer, its journal and six indexes
// are all on the peak path, small enough that a sweep cell stays under a minute.
const writeBurstBatches = 5

type childResult struct {
	Mode      string `json:"mode"`
	CacheSize string `json:"cache_size"`
	MmapSize  string `json:"mmap_size"`

	// What the driver reports back, per connection, after the DSN was applied.
	// A requested value that does not read back is the answer to "does this
	// pragma do anything under this driver".
	EffCacheSize int64  `json:"eff_cache_size"`
	EffMmapSize  int64  `json:"eff_mmap_size"`
	JournalMode  string `json:"journal_mode"`

	// Every PRAGMA compile_options entry mentioning MMAP, from the build under
	// test. This is the evidence behind any "mmap_size did nothing" claim.
	MmapCompileOptions []string `json:"mmap_compile_options,omitempty"`

	Readers int           `json:"readers"`
	Rows    fixtureCounts `json:"rows"`

	Built        bool    `json:"built"`
	BuildSeconds float64 `json:"build_seconds"`

	PageSize  int64 `json:"page_size"`
	PageCount int64 `json:"page_count"`
	FileBytes int64 `json:"file_bytes"`

	IdleKB       int64 `json:"idle_kb"`
	SingleKB     int64 `json:"single_kb"`
	ConcurrentKB int64 `json:"concurrent_kb"`
	WriteKB      int64 `json:"write_kb"`
	PeakKB       int64 `json:"peak_kb"`
	GoHeapKB     int64 `json:"go_heap_kb"`

	Queries         int     `json:"queries"`
	WorkloadSeconds float64 `json:"workload_seconds"`

	Err string `json:"err,omitempty"`
}

func main() {
	var (
		child      = flag.String("child", "", "internal: child mode (build|read)")
		childDB    = flag.String("child-db", "", "internal: database path for the child")
		childCache = flag.String("child-cache", shippedCacheSize, "internal: cache_size for the child")
		childMmap  = flag.String("child-mmap", shippedMmapSize, "internal: mmap_size for the child")

		dir     = flag.String("dir", ".dev/rss-spike", "working directory for the fixture (gitignored)")
		out     = flag.String("out", ".dev/rss-spike.md", "write the result table here")
		rows    = flag.Int("rows", 500_000, "fixture size in rows (ARCHITECTURE §13 says 500k)")
		cacheL  = flag.String("cache", "-2000,-8000,-32000", "cache_size values to sweep (negative = KiB)")
		mmapL   = flag.String("mmap", "0,67108864,134217728", "mmap_size values to sweep (bytes)")
		rebuild = flag.Bool("rebuild", false, "rebuild the fixture even if a complete one exists")
	)
	flag.Parse()

	ctx := context.Background()

	var err error
	switch *child {
	case "":
		err = runParent(ctx, *dir, *out, *rows, split(*cacheL), split(*mmapL), *rebuild)
	case "build":
		err = runChildBuild(ctx, *childDB, *rows, *rebuild)
	case "read":
		err = runChildRead(ctx, *childDB, *childCache, *childMmap, *rows)
	default:
		err = fmt.Errorf("unknown -child mode %q", *child)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "spike: %v\n", err)
		os.Exit(1)
	}
}

// ─── Parent ──────────────────────────────────────────────────────────────────

func runParent(ctx context.Context, dir, out string, rows int, cacheVals, mmapVals []string, rebuild bool) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	header := environmentReport(rows)
	fmt.Print(header)

	// Step 1 — the fixture, built in its own child so its import peak is a
	// figure in its own right (§13's "peak RSS during a 10k-item import").
	fixturePath := filepath.Join(dir, "fixture.db")
	fmt.Printf("\nbuilding fixture at %s (this is the slow step)\n", fixturePath)
	buildStart := time.Now()
	build, err := runChild(ctx, self, "build", fixturePath, "", "", rows, rebuild)
	if err != nil {
		return err
	}
	if build.Built {
		fmt.Printf("  built %d rows in %.1fs; import peak RSS %s MiB\n",
			build.Rows.total(), build.BuildSeconds, mib(build.PeakKB))
	} else {
		fmt.Printf("  reused an existing complete fixture (%d rows) in %.1fs — "+
			"import peak NOT measured this run; pass -rebuild for that number\n",
			build.Rows.total(), time.Since(buildStart).Seconds())
	}
	fmt.Printf("  database %s, page_size %d, page_count %d\n",
		bytesMiB(build.FileBytes), build.PageSize, build.PageCount)

	// Step 2 — the sweep. One child process per cell, each on its own copy of
	// the fixture so a cell that writes cannot bias the next one.
	cellPath := filepath.Join(dir, "cell.db")
	results := make([]childResult, 0, len(cacheVals)*len(mmapVals))
	fmt.Printf("\nsweeping %d cells (cache_size × mmap_size)\n", len(cacheVals)*len(mmapVals))
	for _, c := range cacheVals {
		for _, m := range mmapVals {
			if err := copyFixture(fixturePath, cellPath); err != nil {
				return err
			}
			fmt.Printf("  cache_size=%-8s mmap_size=%-10s ", c, m)
			r, err := runChild(ctx, self, "read", cellPath, c, m, rows, false)
			if err != nil {
				removeDB(cellPath)
				return err
			}
			fmt.Printf("idle %s MiB  peak %s MiB\n", mib(r.IdleKB), mib(r.PeakKB))
			results = append(results, r)
			removeDB(cellPath)
		}
	}

	report := renderReport(header, build, results)
	fmt.Print("\n" + report)

	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
	}
	if err := os.WriteFile(out, []byte(report), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	fmt.Printf("\nwrote %s — record it in docs/DECISIONS.md ADR-0001 with the hardware named.\n", out)
	if !rssAvailable {
		fmt.Print("\nTHIS RUN MEASURES NOTHING: no process RSS on this platform. See rss_other.go.\n")
	}
	fmt.Printf("\nthese numbers belong to GOOS=%s GOARCH=%s with %d CPUs. They do not "+
		"transfer to another architecture or another core count.\n",
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	return nil
}

// runChild executes this same binary in child mode and parses its one result
// line. The child's other output is passed through on failure, so a fixture
// error is readable rather than swallowed.
func runChild(ctx context.Context, self, mode, dbPath, cache, mmap string, rows int, rebuild bool) (childResult, error) {
	args := []string{
		"-child=" + mode,
		"-child-db=" + dbPath,
		"-rows=" + strconv.Itoa(rows),
	}
	if cache != "" {
		args = append(args, "-child-cache="+cache)
	}
	if mmap != "" {
		args = append(args, "-child-mmap="+mmap)
	}
	if rebuild {
		args = append(args, "-rebuild")
	}

	// Re-exec of THIS binary with flags this process built. Nothing here comes
	// from outside the harness.
	cmd := exec.CommandContext(ctx, self, args...) //nolint:gosec // G204: self, with our own args
	cmd.Env = os.Environ()
	outBytes, err := cmd.CombinedOutput()
	text := string(outBytes)

	var res childResult
	found := false
	for line := range strings.SplitSeq(text, "\n") {
		if after, ok := strings.CutPrefix(line, resultPrefix); ok {
			if jsonErr := json.Unmarshal([]byte(after), &res); jsonErr != nil {
				return res, fmt.Errorf("child %s: parse result: %w", mode, jsonErr)
			}
			found = true
		}
	}
	if err != nil {
		return res, fmt.Errorf("child %s failed: %w\n%s", mode, err, text)
	}
	if !found {
		return res, fmt.Errorf("child %s produced no result line:\n%s", mode, text)
	}
	if res.Err != "" {
		return res, fmt.Errorf("child %s: %s", mode, res.Err)
	}
	return res, nil
}

// ─── Children ────────────────────────────────────────────────────────────────

// runChildBuild builds (or verifies) the fixture with the SHIPPED pragma
// defaults, and reports its own peak RSS as the import figure.
func runChildBuild(ctx context.Context, path string, rows int, rebuild bool) error {
	res := childResult{Mode: "build", CacheSize: shippedCacheSize, MmapSize: shippedMmapSize}
	defer func() { emit(res) }()

	want := planFixture(rows)
	res.Rows = want

	if rebuild {
		removeDB(path)
	}
	d, err := db.Open(ctx, path)
	if err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}
	defer func() { _ = d.Close() }()

	if fixtureComplete(ctx, d, want) {
		res.Built = false
	} else {
		// A partial fixture is worse than none: row counts would be wrong and
		// unique indexes would collide. Start clean.
		_ = d.Close()
		removeDB(path)
		if d, err = db.Open(ctx, path); err != nil {
			res.Err = err.Error()
			return errors.New(res.Err)
		}
		start := time.Now()
		if err := buildFixture(ctx, d, want); err != nil {
			res.Err = err.Error()
			return errors.New(res.Err)
		}
		res.BuildSeconds = time.Since(start).Seconds()
		res.Built = true
	}

	if err := describeDB(ctx, d, path, &res); err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}
	res.PeakKB = peakRSSKB()
	res.IdleKB = currentRSSKB()
	return nil
}

// runChildRead is one cell of the sweep: open with the cell's pragmas, then
// walk idle → one reader → all readers → a write burst, sampling RSS at each
// step and taking the kernel's high-water mark at the end.
func runChildRead(ctx context.Context, path, cache, mmap string, rows int) error {
	res := childResult{Mode: "read", CacheSize: cache, MmapSize: mmap, Rows: planFixture(rows)}
	defer func() { emit(res) }()

	// The two pragmas under test, substituted into the REAL list db.Open uses.
	// Everything else stays exactly as the shipped binary sets it.
	if !db.SetPragmaForSpike("cache_size", cache) {
		res.Err = "cache_size is not in the pragma list"
		return errors.New(res.Err)
	}
	if !db.SetPragmaForSpike("mmap_size", mmap) {
		res.Err = "mmap_size is not in the pragma list"
		return errors.New(res.Err)
	}

	d, err := db.Open(ctx, path)
	if err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}
	defer func() { _ = d.Close() }()

	res.Readers = runtime.NumCPU() * 2
	if err := describeDB(ctx, d, path, &res); err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}

	// 1. Idle: open, migrated, nothing served. This is the headline number the
	// "< 80 MB idle" budget is about.
	res.IdleKB = settledRSS()

	start := time.Now()

	// 2. One pinned connection. Compared against step 3 this is what says
	// whether the page cache is per-connection or shared.
	conn, err := d.Read().Conn(ctx)
	if err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}
	n, err := readPass(ctx, conn, res.Rows, 1)
	_ = conn.Close()
	if err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}
	res.Queries += n
	res.SingleKB = settledRSS()

	// 3. Every reader in the pool, concurrently — the pool size the binary
	// really uses, NumCPU*2.
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for i := range res.Readers {
		wg.Add(1)
		go func(seed uint64) {
			defer wg.Done()
			n, err := readPass(ctx, d.Read(), res.Rows, seed)
			mu.Lock()
			defer mu.Unlock()
			res.Queries += n
			if err != nil {
				errs = append(errs, err)
			}
		}(uint64(i) + 2) //nolint:gosec // G115: i is a bounded loop index over the read pool
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}
	res.ConcurrentKB = settledRSS()

	// 4. A write burst at the import batch size, so peak includes the writer.
	if _, err := writeBurst(ctx, d, res.Rows, writeBurstBatches); err != nil {
		res.Err = err.Error()
		return errors.New(res.Err)
	}
	res.WriteKB = settledRSS()
	res.WorkloadSeconds = time.Since(start).Seconds()

	// Go heap at the END of the cell, next to the RSS it explains only part of.
	// The gap between the two columns is the point of the exercise: MemStats
	// cannot see SQLite's page cache or its mmap'd pages.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	res.GoHeapKB = int64(ms.HeapAlloc / 1024) //nolint:gosec // G115: a heap that overflows int64 KiB is not a memory question

	res.PeakKB = peakRSSKB()
	return nil
}

// settledRSS runs a GC, pauses briefly, then samples the resident set.
//
// It deliberately does NOT call debug.FreeOSMemory(). That would hand pages back
// to the kernel on demand and produce a best-case number no running UsArr would
// ever show — the real process returns memory on the scavenger's schedule, not
// on a benchmark's. The figures here therefore include heap the runtime has not
// yet released, which errs high, which is the safe direction for a budget.
func settledRSS() int64 {
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	return currentRSSKB()
}

func describeDB(ctx context.Context, d *db.DB, path string, res *childResult) error {
	var err error
	if res.PageSize, err = db.PragmaInt(ctx, d.Read(), "page_size"); err != nil {
		return err
	}
	if res.PageCount, err = db.PragmaInt(ctx, d.Read(), "page_count"); err != nil {
		return err
	}
	if res.EffCacheSize, err = db.PragmaInt(ctx, d.Read(), "cache_size"); err != nil {
		return err
	}
	if res.EffMmapSize, err = db.PragmaInt(ctx, d.Read(), "mmap_size"); err != nil {
		return err
	}
	if err := d.Read().QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&res.JournalMode); err != nil {
		return fmt.Errorf("read journal_mode: %w", err)
	}

	// Every compile option with MMAP in it. If a requested mmap_size does not
	// read back, this is the evidence for why, straight from the build under
	// test rather than from an assumption about it.
	var ignored int
	if err := query(ctx, d.Read(), &ignored, "PRAGMA compile_options", nil,
		func(rows *sql.Rows) error {
			var opt string
			if err := rows.Scan(&opt); err != nil {
				return err
			}
			if strings.Contains(opt, "MMAP") {
				res.MmapCompileOptions = append(res.MmapCompileOptions, opt)
			}
			return nil
		}); err != nil {
		return fmt.Errorf("read compile_options: %w", err)
	}

	var total int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if fi, statErr := os.Stat(path + suffix); statErr == nil {
			total += fi.Size()
		}
	}
	res.FileBytes = total

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	res.GoHeapKB = int64(ms.HeapAlloc / 1024) //nolint:gosec // G115: a heap that overflows int64 KiB is not a memory question
	return nil
}

func emit(res childResult) {
	b, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spike: marshal result: %v\n", err)
		return
	}
	fmt.Println(resultPrefix + string(b))
}

// ─── Report ──────────────────────────────────────────────────────────────────

func environmentReport(rows int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "UsArr SQLite RSS measurement — ARCHITECTURE §13, ADR-0001\n")
	fmt.Fprintf(&b, "  GOOS=%s GOARCH=%s  NumCPU=%d (read pool %d, write pool 1)\n",
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.NumCPU()*2)
	fmt.Fprintf(&b, "  page size %d B", os.Getpagesize())
	if kb := totalMemKB(); kb > 0 {
		fmt.Fprintf(&b, "   system memory %.2f GiB", float64(kb)/(1024*1024))
	} else {
		fmt.Fprintf(&b, "   system memory unknown")
	}
	fmt.Fprintf(&b, "\n  %s %s\n", runtime.Version(), driverVersion())
	fmt.Fprintf(&b, "  RSS source: %s\n", rssSource)
	fmt.Fprintf(&b, "  fixture target: %d rows\n", rows)
	if !rssAvailable {
		fmt.Fprintf(&b, "  ⚠ NO PROCESS RSS ON THIS PLATFORM — every RSS figure below is n/a.\n")
	}
	return b.String()
}

func driverVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "driver version unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/ncruces/go-sqlite3" {
			return "ncruces/go-sqlite3 " + dep.Version
		}
	}
	return "ncruces/go-sqlite3 (version unknown; built from a replaced module?)"
}

func renderReport(header string, build childResult, results []childResult) string {
	var b strings.Builder

	b.WriteString("### SQLite RSS spike — result\n\n")
	b.WriteString("```\n" + header + "```\n\n")

	readers := 0
	if len(results) > 0 {
		readers = results[0].Readers
	}
	fmt.Fprintf(&b, "**Fixture.** %d rows: %d tag_assignment, %d provenance, %d audit_log, "+
		"%d write_queue, %d release_candidate. Database %s (%d pages of %d B), journal_mode=%s.\n",
		build.Rows.total(), build.Rows.TagAssignment, build.Rows.Provenance, build.Rows.AuditLog,
		build.Rows.WriteQueue, build.Rows.ReleaseCandidate,
		bytesMiB(build.FileBytes), build.PageCount, build.PageSize, build.JournalMode)
	if build.Built {
		fmt.Fprintf(&b, "Built in %.1f s through the real single-writer path in batches of %d; "+
			"peak RSS during that import was **%s MiB** at the shipped pragma defaults.\n\n",
			build.BuildSeconds, importBatch, mib(build.PeakKB))
	} else {
		b.WriteString("Reused from a previous run, so the import peak was not measured " +
			"(re-run with `-rebuild` for that figure).\n\n")
	}

	fmt.Fprintf(&b, "**Sweep.** All figures MiB, from %s. One child process per row. "+
		"Each row walks: open+migrate (idle) → one pinned connection → all %d pool readers "+
		"concurrently → a %d-row write burst.\n\n", rssSource, readers, writeBurstBatches*importBatch)

	b.WriteString("| cache_size | mmap_size | idle | 1 reader | " +
		strconv.Itoa(readers) + " readers | after write | peak (VmHWM) | Go heap |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, r := range results {
		mark := ""
		if r.CacheSize == shippedCacheSize && r.MmapSize == shippedMmapSize {
			mark = " ←shipped"
		}
		fmt.Fprintf(&b, "| %s%s | %s | %s | %s | %s | %s | **%s** | %s |\n",
			describeCache(r.CacheSize), mark, describeMmap(r.MmapSize),
			mib(r.IdleKB), mib(r.SingleKB), mib(r.ConcurrentKB),
			mib(r.WriteKB), mib(r.PeakKB), mib(r.GoHeapKB))
	}

	b.WriteString("\n**What the numbers answer.**\n\n")
	b.WriteString(pragmaFindings(results, readers))

	b.WriteString("\n**Caveats, so nobody over-reads this.**\n\n")
	b.WriteString("- Migration 0001 has no `work`, `edition`, `media_file`, `search_doc` or " +
		"`search_fts` — they land with library sync. The fixture uses the tables that exist, " +
		"chosen for comparable page and index pressure. **FTS5 memory is unmeasured**; re-run " +
		"this after the migration that adds the search tables.\n")
	fmt.Fprintf(&b, "- The read pool is `NumCPU*2` = %d here. A per-connection page cache scales "+
		"with that number, so a result from a machine with a different core count is not "+
		"transferable.\n", readers)
	b.WriteString("- Peak is the kernel's `VmHWM` for the whole process, which is the right " +
		"figure and also an inclusive one: it contains the Go runtime, the binary's text and " +
		"the harness itself, not only SQLite.\n")
	b.WriteString("- No SPA, no HTTP server, no *Arr client, no sync workers are running. This " +
		"is the storage layer's floor, not the binary's idle RSS.\n")
	return b.String()
}

// pragmaFindings states what the sweep decided, and marks the parts that are
// inference rather than measurement.
func pragmaFindings(results []childResult, readers int) string {
	if len(results) == 0 {
		return "- No cells ran.\n"
	}
	var b strings.Builder

	// mmap_size: does the driver honour it at all? Reported once per distinct
	// requested value, not once per cell — the same finding nine times reads
	// like nine findings.
	var ignoredVals []string
	seen := map[string]bool{}
	honoured := 0
	for _, r := range results {
		if seen[r.MmapSize] {
			continue
		}
		seen[r.MmapSize] = true
		want, _ := strconv.ParseInt(r.MmapSize, 10, 64)
		if r.EffMmapSize == want {
			honoured++
			continue
		}
		ignoredVals = append(ignoredVals,
			fmt.Sprintf("`%s` read back as `%d`", r.MmapSize, r.EffMmapSize))
	}
	switch {
	case len(ignoredVals) == 0:
		fmt.Fprintf(&b, "- **`mmap_size` is honoured**: every requested value read back "+
			"unchanged from a pool connection (%d distinct values).\n", honoured)
	default:
		fmt.Fprintf(&b, "- **`mmap_size` is a no-op under this driver**: %s. The mmap column of "+
			"this sweep therefore measures nothing, and any difference along it is noise — which "+
			"is itself the answer reference/sync.md §6 asked for. Evidence from the build under "+
			"test: `PRAGMA compile_options` reports %s.\n",
			strings.Join(ignoredVals, ", "), compileOptionEvidence(results))
	}

	// cache_size: per-connection or shared? The evidence is how the
	// 1-reader → N-reader growth changes as cache_size grows. If each
	// connection has its own cache, that growth must scale with cache_size; if
	// the cache is shared, or simply never filled, the deltas stay together.
	// The conclusion is inference from those numbers, and is labelled as such.
	if rssAvailable && readers > 1 {
		var (
			lines   []string
			ratios  []float64
			seenCch = map[string]bool{}
		)
		for _, r := range results {
			if seenCch[r.CacheSize] {
				continue
			}
			seenCch[r.CacheSize] = true
			c, err := strconv.ParseInt(r.CacheSize, 10, 64)
			if err != nil || c >= 0 {
				continue // a positive cache_size is pages, not KiB; not comparable here
			}
			observed := r.ConcurrentKB - r.SingleKB
			// A per-connection cache means the other readers each fund one more
			// cache_size worth of pages.
			predicted := -c * int64(readers-1)
			ratio := float64(observed) / float64(predicted)
			ratios = append(ratios, ratio)
			lines = append(lines, fmt.Sprintf("`%s` observed +%s MiB vs +%s MiB predicted (%.2f×)",
				r.CacheSize, mib(observed), mib(predicted), ratio))
		}

		fmt.Fprintf(&b, "- **cache_size is %s.** RSS added by going from one pinned connection "+
			"to all %d pool readers, against what a strictly per-connection page cache would "+
			"predict: %s.%s *The verdict is inference from those ratios, not a reading of the "+
			"driver's internals.*\n",
			cacheVerdict(ratios), readers, strings.Join(lines, "; "), cacheConsequence(ratios, readers))
	}

	// The cheapest cell that is not materially slower is the one to ship.
	best := results[0]
	for _, r := range results[1:] {
		if r.PeakKB < best.PeakKB {
			best = r
		}
	}
	fmt.Fprintf(&b, "- Lowest peak in this sweep: `cache_size=%s`, `mmap_size=%s` at %s MiB. "+
		"Compare against the shipped row before changing anything — peak RSS is one axis and "+
		"§13's latency budgets are the other, and this harness measures only the first.\n",
		best.CacheSize, best.MmapSize, mib(best.PeakKB))
	return b.String()
}

// cacheVerdict turns the observed/predicted ratios into the sentence
// reference/sync.md §6 actually asked for. The thresholds are deliberately wide:
// this decides between "each connection funds its own cache" and "it does not",
// which is a factor-of-N difference, not a percentage one.
func cacheVerdict(ratios []float64) string {
	if len(ratios) == 0 {
		return "not measured here"
	}
	perConn, shared := 0, 0
	for _, r := range ratios {
		switch {
		case r >= 0.7:
			perConn++
		case r <= 0.25:
			shared++
		}
	}
	switch {
	case perConn == len(ratios):
		return "PER-CONNECTION"
	case shared == len(ratios):
		return "shared, or never filled by this workload"
	default:
		return "inconclusive on this run — the ratios below disagree"
	}
}

// cacheConsequence spells out the arithmetic a per-connection cache imposes,
// because that is the part that decides the shipped default.
func cacheConsequence(ratios []float64, readers int) string {
	if len(ratios) == 0 {
		return ""
	}
	for _, r := range ratios {
		if r < 0.7 {
			return ""
		}
	}
	return fmt.Sprintf(" At full read concurrency the process therefore pays roughly "+
		"`cache_size` × %d (the %d-connection read pool plus the writer), so the setting is a "+
		"multiplier on RSS, not a fixed cost.", readers+1, readers)
}

// compileOptionEvidence renders the MMAP-related compile options the build
// under test reports, so a "mmap does nothing here" claim carries its source.
func compileOptionEvidence(results []childResult) string {
	for _, r := range results {
		if len(r.MmapCompileOptions) > 0 {
			return "`" + strings.Join(r.MmapCompileOptions, "`, `") + "`"
		}
	}
	return "no MMAP-related option at all"
}

// ─── Small helpers ───────────────────────────────────────────────────────────

// describeCache spells out what a cache_size value means. SQLite reads a
// NEGATIVE value as KiB and a positive one as a page count, and mixing the two
// up is the classic way to request a thousand times more memory than intended.
func describeCache(v string) string {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n >= 0 {
		return "`" + v + "` (pages)"
	}
	return fmt.Sprintf("`%s` (%.1f MiB)", v, float64(-n)/1024)
}

func describeMmap(v string) string {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return "`" + v + "`"
	}
	if n == 0 {
		return "`0` (off)"
	}
	return fmt.Sprintf("`%s` (%d MiB)", v, n/(1024*1024))
}

func mib(kb int64) string {
	if !rssAvailable && kb == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", float64(kb)/1024)
}

func bytesMiB(b int64) string { return fmt.Sprintf("%.1f MiB", float64(b)/(1024*1024)) }

func split(s string) []string {
	var out []string
	for f := range strings.SplitSeq(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// copyFixture gives each sweep cell an identical, private database, so the
// write burst in one cell cannot bias the next.
func copyFixture(src, dst string) error {
	removeDB(dst)
	in, err := os.Open(src) //nolint:gosec // G304: paths are this harness's own flags
	if err != nil {
		return fmt.Errorf("open fixture: %w", err)
	}
	defer func() { _ = in.Close() }()
	outF, err := os.Create(dst) //nolint:gosec // G304: same
	if err != nil {
		return fmt.Errorf("create cell db: %w", err)
	}
	if _, err := io.Copy(outF, in); err != nil {
		_ = outF.Close()
		return fmt.Errorf("copy fixture: %w", err)
	}
	if err := outF.Close(); err != nil {
		return fmt.Errorf("close cell db: %w", err)
	}
	return nil
}

// removeDB removes a database and its WAL sidecars. Leaving a -wal behind means
// the next open replays it, which is not the clean start a cell needs.
func removeDB(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}
