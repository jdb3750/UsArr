package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/jdb3750/UsArr/internal/store"
)

// The §7.4 schedule as a LOOP and as a WIRING, which is the half
// reconcile_schedule_test.go does not reach.
//
// ⚠️ EVERY TEST IN reconcile_schedule_test.go CALLS reconcileOnce DIRECTLY, and
// that was measured rather than supposed: at the tip that shipped the schedule,
// `startReconciler` had no test caller anywhere in the tree, and deleting its
// call from main.go left the whole cmd/usarr suite green. The headline claim of
// the slice — "the sweep runs on a six-hourly clock" — was therefore pinned by
// nothing: a refactor of run()'s shutdown block would have deleted the feature
// and every test that gave the slice its confidence would have stayed green.
// That is the same deaf-guard shape web/src/lib/services-screen.test.ts records
// for the Services screen's `Synced` column, which shipped as a hardcoded word
// under seven green unit tests of the helper that was supposed to draw it.
//
// The three pins here are, in the order a reader should trust them:
//
//  1. THE LOOP RUNS A PASS BEFORE ITS FIRST TICK — behavioural, real clock, real
//     upstream double.
//  2. THE RETURNED CHANNEL DOES NOT CLOSE UNTIL THE LOOP HAS STOPPED —
//     behavioural, and it is what makes main.go's `<-reconcilerDone` mean
//     anything at all.
//  3. run() CALLS startReconciler AND WAITS ON WHAT IT RETURNS — structural, an
//     AST read of cmd/usarr/main.go.
//
// ⚠️ PIN 3 IS A SOURCE READ AND SAYS SO. A behavioural pin would have to run
// run() itself — a flag parse, a bound listener and a signal — which is a
// restructuring larger than the schedule it would be guarding. It is an AST walk
// rather than a grep, so a call inside a comment or a string literal cannot
// satisfy it; what it still cannot see is a call that is present but
// unreachable, and it does not claim otherwise.

// TestTheReconcilerRunsAPassBeforeItsFirstTick pins THE FIRST PASS, which the
// loop's own doc comment calls out and which nothing was checking.
//
// ⚠️ FIRED. Moving `g.reconcileOnce(ctx, time.Now())` below the select — so the
// loop waits a full reconcileTick before its first pass — turns this red:
// "the reconciler's first pass never ran: 30m0s is one reconcileTick, so a loop
// that ticks before it sweeps cannot be caught inside a test's lifetime."
// Nothing else in the suite notices, because every other test calls
// reconcileOnce itself.
//
// THE CLOCK IS REAL HERE, unlike its siblings: startReconciler passes
// time.Now() and there is no seam to stub it through. Due-ness is therefore
// arranged in the DATABASE — last_full_sync_at is backdated past
// reconcileInterval — which is the same fact from the other side and is what a
// process restarting after a day away actually finds.
func TestTheReconcilerRunsAPassBeforeItsFirstTick(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = twoSeries()

	env := newTestApp(t)
	id := importedKavita(t, env, kav)
	backdateLastFullSync(t, env, id, 4*reconcileInterval)

	// The upstream loses a series. Only a pass that actually runs can notice.
	kav.setSeries([]map[string]any{kavitaSeries(41, 1, "Frieren", nil)})

	ctx, cancel := context.WithCancel(context.Background())
	done := env.app.registry.startReconciler(ctx)
	t.Cleanup(func() {
		cancel()
		<-done
	})

	deadline := time.Now().Add(20 * time.Second)
	for !linkTombstoned(t, env, id, "42") {
		if time.Now().After(deadline) {
			t.Fatalf("the reconciler's first pass never ran: %s is one reconcileTick, so a loop "+
				"that ticks before it sweeps cannot be caught inside a test's lifetime.\nProcess log:\n%s",
				reconcileTick, env.logs())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestTheReconcilersDoneChannelStaysOpenWhileAPassIsRunning pins THE SHUTDOWN
// WAIT — the property main.go's `<-reconcilerDone` is standing on, and the one
// that keeps a.Close() from closing the database under a live catalogue writer.
//
// ⚠️ FIRED. Closing `done` at the top of startReconciler's goroutine — so the
// channel is already closed before a single pass has run — turns this red:
// "startReconciler's channel was already closed while a pass was still parked
// inside the upstream read." Without this test that mutation is invisible:
// main.go still receives from the channel, the receive still returns, and the
// receive returns having waited for nothing.
//
// THE PASS IS HELD OPEN RATHER THAN RACED. fakeKavita.holdSeriesList parks the
// catalogue read after the request has been recorded, so "a pass is in flight"
// is a state this test is standing in rather than a window it is hoping for. The
// context is NOT cancelled while the hold is on: a cancel would abort the HTTP
// round trip and the goroutine would finish in microseconds, which is exactly
// the race that would make this assertion a coin flip.
func TestTheReconcilersDoneChannelStaysOpenWhileAPassIsRunning(t *testing.T) {
	kav := newFakeKavita(t, importAuthKey)
	kav.libraries = 1
	kav.series = twoSeries()

	env := newTestApp(t)
	id := importedKavita(t, env, kav)
	backdateLastFullSync(t, env, id, 4*reconcileInterval)

	before := kav.countPath("/api/Series/all-v2")
	kav.holdSeriesList()

	ctx, cancel := context.WithCancel(context.Background())
	done := env.app.registry.startReconciler(ctx)

	// Wait until the scheduled pass is parked inside the upstream read.
	deadline := time.Now().Add(20 * time.Second)
	for kav.countPath("/api/Series/all-v2") == before {
		if time.Now().After(deadline) {
			kav.releaseSeriesList()
			cancel()
			<-done
			t.Fatalf("no scheduled pass reached the upstream, so there is no in-flight run to "+
				"assert about.\nProcess log:\n%s", env.logs())
		}
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case <-done:
		kav.releaseSeriesList()
		t.Fatalf("startReconciler's channel was already closed while a pass was still parked "+
			"inside the upstream read. main.go receives from that channel before a.Close(), so a "+
			"channel that closes early means the database can close under a running import — the "+
			"one failure the ordering in run() exists to prevent.\nProcess log:\n%s", env.logs())
	default:
	}

	kav.releaseSeriesList()
	cancel()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("the reconciler did not stop when its context was cancelled; shutdown would hang "+
			"on <-reconcilerDone.\nProcess log:\n%s", env.logs())
	}
}

// ── the wiring pin ──────────────────────────────────────────────────────────

// runBodyFloor is a floor on the number of statement nodes in run(), asserted
// before anything is asserted ABOUT run(). It is far below the real size and
// nowhere near far enough to survive a parse that resolved to the wrong
// function or a body that lost most of itself — the same reasoning
// services-screen.test.ts states for its `?raw` corpus floors. The count is
// logged rather than pinned exactly, so ordinary editing of run() does not
// touch this number.
const runBodyFloor = 25

// TestTheReconcilerIsStartedFromRun is the wiring pin: run() must CALL
// startReconciler and must WAIT on what it returns.
//
// ⚠️ FIRED, and the drill is the reason this file exists. Deleting the
// `reconcilerCtx`/`reconcilerDone` block from run() turns this red:
// "cmd/usarr/main.go's run() no longer calls startReconciler." Before this test
// existed that same deletion left `go test ./cmd/usarr/` green in 11s.
//
// # What it asserts, and why each half
//
//   - THE CALL. A reconciler that is defined and never started is the exact
//     defect maintenance.go's header records for ExpireReleaseCandidates and
//     EvictExpired: both existed, neither had a production caller, and
//     release_candidate grew without bound for the life of the process while
//     every unit test of the statement passed.
//   - THE WAIT. The call alone is not the feature. main.go's own comment says
//     the loop is waited for "because it writes the catalogue and a database
//     closed under a running import is the one failure this ordering exists to
//     prevent", so a run() that started the loop and dropped its channel would
//     satisfy half a claim. The identifier is READ OUT OF THE ASSIGNMENT rather
//     than spelt here, so renaming `reconcilerDone` breaks nothing and dropping
//     the receive breaks the assertion standing on it.
//
// # The controls, which come first
//
// A walk that inspected the wrong node reports no calls and no receives, and
// every "must contain" assertion below would fail for a reason that has nothing
// to do with the schedule. So the walk is proved on two anchors in run() that
// this slice does not own — the `buildApp` call and the `serveErr` receive —
// before the reconciler is looked for at all.
func TestTheReconcilerIsStartedFromRun(t *testing.T) {
	fset := token.NewFileSet()
	// No parser.ParseComments: a call that exists only in a comment must not be
	// able to satisfy this, which is the difference between an AST read and a
	// grep.
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse cmd/usarr/main.go: %v", err)
	}

	var run *ast.FuncDecl
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if ok && fd.Recv == nil && fd.Name.Name == "run" && fd.Body != nil {
			run = fd
			break
		}
	}
	if run == nil {
		t.Fatal("cmd/usarr/main.go declares no func run() with a body. Everything below reads " +
			"that function; a walk over nothing reports nothing wrong, which is indistinguishable " +
			"from a clean tree.")
	}

	stmts, calls, received, assignedFrom := walkRun(run)

	if stmts < runBodyFloor {
		t.Fatalf("run() holds %d statement nodes, under the floor of %d. Either the parse "+
			"resolved to a different function or run() lost most of itself; either way nothing "+
			"below this line can be trusted.", stmts, runBodyFloor)
	}
	if !calls["buildApp"] {
		t.Fatal("the call walk did not find buildApp in run(), which run() cannot do its job " +
			"without. The walk is not seeing the function body, so every assertion below is vacuous.")
	}
	if !received["serveErr"] {
		t.Fatal("the receive walk did not find <-serveErr in run(), which run() blocks on before " +
			"it returns. The receive detection is broken, so the wait assertion below is vacuous.")
	}
	t.Logf("run(): %d statement nodes, %d distinct callees, %d distinct identifiers received from",
		stmts, len(calls), len(received))

	if !calls["startReconciler"] {
		t.Fatal("cmd/usarr/main.go's run() no longer calls startReconciler. §7.4's sweep is then " +
			"defined and never started — the defect maintenance.go's header records for " +
			"ExpireReleaseCandidates, which existed with no production caller while every unit " +
			"test of it passed. Every other test of this schedule calls reconcileOnce directly " +
			"and stays green through exactly this deletion.")
	}

	name := assignedFrom["startReconciler"]
	if name == "" {
		t.Fatal("run() calls startReconciler but does not bind what it returns to an identifier. " +
			"The returned channel is the only way to know the loop has stopped, and a run() that " +
			"discards it closes the database under a live catalogue writer.")
	}
	if !received[name] {
		t.Errorf("run() binds startReconciler's channel to %q and never receives from it. "+
			"main.go's own comment says the loop is waited for because a database closed under a "+
			"running import is the failure the ordering exists to prevent; without the receive "+
			"that ordering is a comment.", name)
	}
}

// walkRun reads run()'s body once and reports the four facts asserted above:
// how many statement nodes it holds, which functions it calls by name, which
// identifiers it receives from, and — for each callee — the identifier its
// result was bound to, if any.
//
// Callees are keyed on the FUNCTION NAME ONLY, so `a.registry.startReconciler`
// and a hypothetical bare `startReconciler` are the same entry: the subject is
// whether the loop is started, never which value it hangs off.
func walkRun(run *ast.FuncDecl) (stmts int, calls, received map[string]bool, assignedFrom map[string]string) {
	calls = map[string]bool{}
	received = map[string]bool{}
	assignedFrom = map[string]string{}

	ast.Inspect(run.Body, func(n ast.Node) bool {
		// Counted before the type switch below, not as an arm of it: every
		// statement node is an ast.Stmt, so a `case ast.Stmt` would swallow the
		// *ast.AssignStmt arm — a type switch takes the first matching case, and
		// this cost one red run to find.
		if _, ok := n.(ast.Stmt); ok {
			stmts++
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			if name := calleeName(node); name != "" {
				calls[name] = true
			}
		case *ast.UnaryExpr:
			if node.Op == token.ARROW {
				if id, ok := node.X.(*ast.Ident); ok {
					received[id.Name] = true
				}
			}
		case *ast.AssignStmt:
			if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
				return true
			}
			call, ok := node.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			lhs, ok := node.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			if name := calleeName(call); name != "" {
				assignedFrom[name] = lhs.Name
			}
		}
		return true
	})
	return stmts, calls, received, assignedFrom
}

// calleeName is the bare function name a call expression names, for both
// `f(...)` and `x.y.f(...)`, and the empty string for anything else.
func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

// backdateLastFullSync moves one instance's last completed full sync into the
// past, which is how a test arranges DUE-NESS for a loop that reads the real
// clock. The column is written directly because the only production writer is a
// successful import, and a test that had to run one to age it could not age it
// at all.
func backdateLastFullSync(t *testing.T, env *testEnv, instanceID int64, age time.Duration) {
	t.Helper()
	if _, err := env.app.store.DB().Writer().ExecContext(t.Context(),
		`UPDATE service_instance SET last_full_sync_at = ? WHERE id = ?`,
		store.FormatTime(time.Now().Add(-age)), instanceID); err != nil {
		t.Fatalf("backdate last_full_sync_at on instance %d: %v", instanceID, err)
	}
}
