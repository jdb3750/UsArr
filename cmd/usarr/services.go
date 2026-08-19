package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jdb3750/UsArr/internal/bookorbit"
	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/kavita"
	"github.com/jdb3750/UsArr/internal/releases"
	"github.com/jdb3750/UsArr/internal/servarr"
	"github.com/jdb3750/UsArr/internal/servarr/mapping"
	"github.com/jdb3750/UsArr/internal/ssrf"
	"github.com/jdb3750/UsArr/internal/store"
)

// This file is where UsArr's outbound HTTP lives.
//
// internal/httpapi may not import internal/servarr and no browser-facing
// handler may hold an outbound client (ARCHITECTURE.md §2.3 rule 1). The three
// interfaces httpapi declares — ReleaseServices, ConnectionTester and
// HealthProbes — are implemented here, in cmd, which is the only package that
// sees an *Arr client at all.

const (
	// probeInterval is how often each instance's health is refreshed. It is off
	// the render path entirely: the Services screen reads the last snapshot.
	probeInterval = 60 * time.Second

	// probeTimeout bounds one instance's whole probe.
	probeTimeout = 20 * time.Second
)

// registry owns one client stack per service_instance and rebuilds it when the
// row's base_url or credential changes.
//
// One client per instance is required, not incidental (ARCHITECTURE.md §7.5):
// the SSRF policy client is pinned to that instance's validated host:port, and
// the circuit breaker has to count against a stable object or it counts nothing.
type registry struct {
	st      *store.Store
	keyring *crypto.Keyring
	log     *slog.Logger
	version string

	// imageCacheDir is Config.ImageCacheDir(), the same directory
	// GET /img/{key} serves from. The registry holds it because runImport is
	// where the cover pass's pipeline is built, and internal/imagecache is a
	// value over a path rather than a shared handle — httpapi builds its own
	// from the same string. Empty disables the cover pass rather than writing
	// somewhere else: imagecache.Put refuses a rootless cache by design.
	imageCacheDir string

	// hub is the SSE stream, attached by buildApp AFTER the server exists.
	// The registry is constructed first because httpapi.New takes it, so this
	// is a one-way back-reference rather than a constructor argument. It is set
	// once during startup and read afterwards, never reassigned.
	hub *httpapi.Hub

	// importCtx is the process-lifetime context a bootstrap import runs under.
	// It is NOT the caller's: entry() is reached from probe and request paths
	// whose deadlines are seconds, and a full import is minutes. Nil disables
	// the on-connect bootstrap, which is what every test that has not asked for
	// it gets.
	importCtx context.Context //nolint:containedctx // see the comment above: this is a lifetime, not a per-call deadline

	// bootstrapped remembers which instances this process has already offered a
	// bootstrap import, so a client rebuild (an edited base_url) does not queue
	// a second one while the first is still running. It is guarded by mu.
	bootstrapped map[int64]bool

	// importMu guards importing, and it is deliberately NOT mu: mu is taken on
	// the probe and request paths, and an import holds its claim for minutes.
	// See beginImport in import.go for why the claim lives in memory at all.
	importMu  sync.Mutex
	importing map[int64]bool

	mu      sync.Mutex
	entries map[int64]*registryEntry

	probeMu sync.RWMutex
	probes  map[int64]httpapi.UpstreamHealth

	probeReq chan int64
}

// registryEntry is one instance's live client stack.
//
// EXACTLY ONE of prowlarr, kavita and bookorbit is non-nil, chosen by
// instance.Kind. They are separate fields rather than one interface because the
// clients have almost nothing in common at the call site — one is searched and
// grabbed from, the others are replicated from — and an interface wide enough
// for all three would be the union of three APIs with no shared caller.
//
// The two catalogue sources are NOT collapsed into one field either, even though
// they share a role: they share no method set at all today (kavita.Client has
// Libraries, bookorbit.Client has an account scope verdict and neither has the
// other), and a nil-checked pair is what makes `entry.bookorbit == nil` a
// compile-time-obvious gate at every place that means "kavita only".
type registryEntry struct {
	// fingerprint changes whenever base_url or the stored envelope changes, so
	// an edited instance gets a fresh client rather than one pinned to the old
	// host with the old key.
	fingerprint string

	// prowlarr and its Search-and-Grab service. Non-nil only for kind=prowlarr.
	prowlarr *servarr.Client
	service  *releases.Service

	// kavita is the catalogue-source client. Non-nil only for kind=kavita.
	// Nothing reads a library through it yet — ADR-0041's sync channels are the
	// commits after this one — but the health prober does, so an added instance
	// gets a real row on the Services screen instead of a permanent error.
	kavita *kavita.Client

	// bookorbit is ADR-0052's catalogue-source client. Non-nil only for
	// kind=bookorbit.
	//
	// ⚠️ THIS COMMENT USED TO READ "NOTHING READS A CATALOGUE THROUGH IT … there
	// is no bookorbit equivalent to start", WHICH IS NOW FALSE. internal/bookorbit
	// grew the two catalogue reads (catalogue.go) and internal/libsync/bookorbit.go
	// is the adapter over them, so this client is on the import path exactly as
	// the kavita field is. What it still does NOT read is anything past slice 1,
	// and the adapter's own header is where that list lives rather than here, so
	// it cannot go stale twice.
	//
	// ⚠️ THAT LIST USED TO INCLUDE "no covers" HERE TOO, AND IT NO LONGER
	// BELONGS: runImport hands this very client to internal/imagepipeline, which
	// libsync's phase D calls once per imported book. The read does NOT go
	// through libsync.BookOrbitReader — the pipeline takes *bookorbit.Client
	// directly — so the adapter's own list is right to keep saying nothing about
	// it, and this field is now on two paths rather than one.
	bookorbit *bookorbit.Client

	instance store.ServiceInstance
}

// breakerState reports the entry's circuit-breaker state and next retry,
// whichever client it holds. The Services screen renders one column for both.
func (e *registryEntry) breakerState() (string, time.Time) {
	switch {
	case e.prowlarr != nil:
		return e.prowlarr.Breaker().State().String(), e.prowlarr.Breaker().RetryAt()
	case e.kavita != nil:
		return e.kavita.Breaker().State().String(), e.kavita.Breaker().RetryAt()
	case e.bookorbit != nil:
		return e.bookorbit.Breaker().State().String(), e.bookorbit.Breaker().RetryAt()
	}
	return "closed", time.Time{}
}

func newRegistry(
	st *store.Store, keyring *crypto.Keyring, log *slog.Logger, version, imageCacheDir string,
) *registry {
	return &registry{
		st:            st,
		keyring:       keyring,
		log:           log,
		version:       version,
		imageCacheDir: imageCacheDir,
		entries:       map[int64]*registryEntry{},
		probes:        map[int64]httpapi.UpstreamHealth{},
		probeReq:      make(chan int64, 32),

		bootstrapped: map[int64]bool{},
		importing:    map[int64]bool{},
	}
}

// attachEvents gives the registry the SSE hub, once, during startup. See the
// field comment for why this is not a constructor argument.
func (g *registry) attachEvents(h *httpapi.Hub) { g.hub = h }

// enableBootstrapImport arms the on-connect full import under a
// process-lifetime context. main calls it with the prober's context; a test
// that does not want a background import simply never calls it.
func (g *registry) enableBootstrapImport(ctx context.Context) { g.importCtx = ctx }

// ── httpapi.ReleaseServices ─────────────────────────────────────────────────

// For returns the Search-and-Grab service fronting one instance.
//
// THE ROLE GATE LIVES HERE, not in entry(). It used to sit in entry(), where it
// refused every non-indexer instance before a client was ever built — which was
// correct while `indexer` was the only role UsArr could talk to, and became a bug
// the moment a second kind landed: it also blocked the background health prober,
// so an added Kavita would have shown a permanent "only indexer services can be
// searched today" on the Services screen. Searching is what needs an indexer;
// existing is not.
func (g *registry) For(ctx context.Context, instanceID int64) (httpapi.ReleaseSearcher, error) {
	entry, err := g.entry(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if entry.service == nil {
		return nil, fmt.Errorf("%q is a %s service; only indexer services can be searched today",
			entry.instance.Name, entry.instance.Role)
	}
	return entry.service, nil
}

// entry returns a live client stack, building or rebuilding it as needed.
func (g *registry) entry(ctx context.Context, instanceID int64) (*registryEntry, error) {
	// The instance row is read outside the lock: a SQLite read must not be held
	// under a mutex that a second request also wants.
	si, err := g.st.GetServiceInstance(ctx, store.Scope{AllInstances: true}, instanceID)
	if err != nil {
		return nil, fmt.Errorf("service instance %d: %w", instanceID, err)
	}
	if !si.Enabled {
		return nil, fmt.Errorf("%q is disabled", si.Name)
	}
	print := fingerprint(si)

	g.mu.Lock()
	defer g.mu.Unlock()
	if e, ok := g.entries[instanceID]; ok && e.fingerprint == print {
		return e, nil
	}

	apiKey, err := g.openCredential(ctx, si)
	if err != nil {
		return nil, err
	}

	e := &registryEntry{fingerprint: print, instance: si}
	switch si.Kind {
	case "prowlarr":
		client, err := g.newProwlarr(si, apiKey)
		if err != nil {
			return nil, err
		}
		service, err := releases.NewService(releases.Config{
			InstanceID: si.ID,
			Client:     client,
			Store:      releases.NewStoreAdapter(g.st),
			Logger:     g.log.With("instance_id", si.ID, "instance", si.Name),
		})
		if err != nil {
			return nil, fmt.Errorf("building release service for %q: %w", si.Name, err)
		}
		e.prowlarr, e.service = client, service
	case "kavita":
		client, err := g.newKavita(si, apiKey)
		if err != nil {
			return nil, err
		}
		e.kavita = client
		// ON CONNECT, and only here: this is the moment a client stack for a
		// catalogue source comes into existence. bootstrapImport itself is the
		// gate on "has this instance ever completed a full sync" — the map below
		// is only the in-process guard against a second goroutine for the same
		// instance while the first is still running.
		if g.importCtx != nil && !g.bootstrapped[instanceID] {
			g.bootstrapped[instanceID] = true
			go g.bootstrapImport(g.importCtx, instanceID)
		}
	case "bookorbit":
		client, err := g.newBookOrbit(si, apiKey)
		if err != nil {
			return nil, err
		}
		e.bookorbit = client
		// ⚠️ THIS ARM USED TO START NO IMPORT, and the comment that stood here
		// said why: "there is no bookorbit equivalent to start, because
		// internal/libsync has no BookOrbit source yet (ADR-0052 slice 1)".
		// internal/libsync/bookorbit.go IS slice 1, so the reason has expired
		// and the arm now does exactly what the kavita one does, for the same
		// reason and under the same two gates — bootstrapImport's own
		// last_full_sync_at check, and this in-process map against a second
		// goroutine for an instance already importing.
		if g.importCtx != nil && !g.bootstrapped[instanceID] {
			g.bootstrapped[instanceID] = true
			go g.bootstrapImport(g.importCtx, instanceID)
		}
	default:
		// httpapi.serviceKinds refuses an unknown kind at the boundary, so this
		// is a row that predates a kind being removed — or a hand-edited SQLite.
		// Either way it is a configuration problem and must say so rather than
		// producing a nil client that panics three frames later.
		return nil, fmt.Errorf("%q has kind %q, which this build has no client for", si.Name, si.Kind)
	}

	g.entries[instanceID] = e
	return e, nil
}

// openCredential decrypts the stored API key.
//
// The AAD binds the ciphertext to this row AND to this instance's normalised
// host:port, so a credential moved to a row whose base_url an attacker controls
// fails to open rather than being decrypted and transmitted. A failure here with
// a valid keyring means tampering or an edited base_url, and it is loud AND
// durable: see auditFailedOpen for why the second half is not decoration.
//
// It takes a context ONLY so the audit append has one. That is worth stating,
// because the decryption itself is pure CPU over bytes already in memory and
// nothing here is cancellable — a reader who assumes the ctx bounds the open
// would be wrong.
func (g *registry) openCredential(ctx context.Context, si store.ServiceInstance) (string, error) {
	aad, err := crypto.ServiceInstanceAAD(si.ID, si.BaseURL)
	if err != nil {
		return "", fmt.Errorf("building AAD for %q: %w", si.Name, err)
	}
	plain, err := g.keyring.Open(si.APIKeyEnc, aad)
	if err != nil {
		if errors.Is(err, crypto.ErrDecrypt) {
			g.auditFailedOpen(ctx, si)
			// Name the real artifacts. This message used to say "restore the
			// master key that sealed it", which is actively misleading for the
			// most likely cause: an operator who restored a backup, HAS the
			// master key, and is missing keys/kek.salt. It sent them looking
			// for the one file they were already holding. The KEK is derived
			// from secret.key AND kek.salt, so the recoverable unit is the
			// whole keys/ directory.
			return "", fmt.Errorf("the stored API key for %q cannot be opened for %s. "+
				"Either the base URL was edited (the credential is bound to its scheme, host and "+
				"port), or the keys/ directory does not match the one that sealed it — restore ALL "+
				"of keys/ (secret.key AND kek.salt), not just the master key. Otherwise re-enter "+
				"the API key",
				si.Name, si.BaseURL)
		}
		return "", fmt.Errorf("opening the stored API key for %q: %w", si.Name, err)
	}
	return string(plain), nil
}

// auditFailedOpen appends the durable half of reference/security.md §1.2's
// contract: "a decryption failure with a valid KEK … is a loud, audit-logged
// failure, never a silent skip".
//
// WHY IT IS NOT ENOUGH TO RETURN THE ERROR. The two callers of openCredential
// are entry() and Test(). Test() is a user pressing a button and reading the
// answer. entry() is not: it is reached from the 60-second health prober and
// from the bootstrap import, so a credential that has been tampered with is
// discovered by a goroutine nobody is watching, and — before this row — left
// nothing behind but a log line that rotates away. An attempt to move a
// full-admin \*Arr key onto a row whose base_url the attacker controls is
// exactly the event that must survive a restart.
//
// THE ACTOR IS store.SystemUserID, not NULL, and that is load-bearing rather
// than tidy. audit_log.actor_user_id carries no foreign key and is nullable, and
// Scope.userPredicate renders the canonical `actor_user_id IN (0, :uid)` — under
// SQL three-valued logic `NULL IN (0, 1)` is unknown, so a NULL actor makes the
// row invisible to EVERY scoped audit read in the codebase. A tamper-evidence
// row nothing can read is not evidence. The prober has no acting user, and 0 is
// precisely what "written by a path with no acting user" means; the same
// reasoning is spelled out at auditRotation in keyrotate.go.
//
// THE METADATA CARRIES IDS AND CONTEXT ONLY. Not the envelope, not the
// ciphertext, not the plaintext — there is no plaintext, the open failed — and
// not si.BaseURL either, even though the edited base URL is one of the two
// causes: a base URL may carry userinfo (`http://user:pass@host`), and
// audit_log has BEFORE UPDATE and BEFORE DELETE triggers that RAISE(ABORT), so
// a secret written here can never be taken out again. The instance id in
// target_id is enough to find the row that was tampered with.
//
// Best-effort, like internal/httpapi's audit helper and for the same reason:
// the caller is already failing closed with a loud error, and turning a failed
// audit insert into a second failure would replace a diagnosable problem with a
// confusing one. A failure to append is logged at ERROR.
func (g *registry) auditFailedOpen(ctx context.Context, si store.ServiceInstance) {
	metadata, err := json.Marshal(map[string]any{
		"kek_id": si.KEKID,
		"kind":   si.Kind,
		"reason": "authenticated decryption failed under a key the keyring holds",
	})
	if err != nil {
		g.log.Error("credential-open audit row not written",
			"instance_id", si.ID, "err", err)
		return
	}
	if _, err := g.st.AppendAudit(ctx, store.AuditEntry{
		ActorUserID:  sql.NullInt64{Int64: store.SystemUserID, Valid: true},
		Action:       store.AuditActionCredentialOpen,
		TargetType:   "service_instance",
		TargetID:     strconv.FormatInt(si.ID, 10),
		Result:       store.AuditResultFail,
		MetadataJSON: string(metadata),
	}); err != nil {
		g.log.Error("credential-open audit row not written",
			"instance_id", si.ID, "err", err)
	}
}

// newEgressClient builds the SSRF-policy HTTP client for one instance.
//
// It is shared by both service clients on purpose: the egress policy is a
// property of the INSTANCE — its validated host:port, its pin, its byte ceiling —
// not of which protocol runs over it. A second copy of these options is how one
// client ends up with a different ceiling from the other against the same host.
func (g *registry) newEgressClient(si store.ServiceInstance) (*http.Client, error) {
	hostPort, err := crypto.NormalizeHostPort(si.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("normalising %q's base URL: %w", si.Name, err)
	}
	httpClient, err := ssrf.NewClient(ssrf.Options{
		// ClassConfigured: an admin typed this URL into the service form, so it
		// may reach private space — but only this one validated host:port.
		Class:           ssrf.ClassConfigured,
		AllowedHostPort: hostPort,
		SPKIPin:         si.TLSSPKIPin,
		MaxBytes:        ssrf.MaxListBytes,
		TotalTimeout:    ssrf.ListTotalTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("building the egress client for %q: %w", si.Name, err)
	}
	return httpClient, nil
}

// newKavita builds the Kavita client over that instance's egress client.
func (g *registry) newKavita(si store.ServiceInstance, apiKey string) (*kavita.Client, error) {
	httpClient, err := g.newEgressClient(si)
	if err != nil {
		return nil, err
	}
	return kavita.New(kavita.Options{
		BaseURL:    si.BaseURL + si.URLBase,
		APIKey:     apiKey,
		HTTPClient: httpClient,
		AppVersion: g.version,
		// An egress-policy refusal never reached the network, so it is a
		// configuration bug rather than upstream flakiness and must not trip
		// the breaker.
		IsPolicyError: ssrf.IsPolicyError,
		Logger:        g.log.With("instance_id", si.ID),
	})
}

// newBookOrbit builds the BookOrbit client over that instance's egress client.
//
// apiKey here is the raw MAGIC-LINK TOKEN, not an API key. It is the only secret
// stored for this kind, it goes into the request BODY of the login route and
// never into a URL, and the short-lived bearer BookOrbit hands back in exchange
// lives in that client's memory and is never written anywhere (ADR-0052,
// internal/bookorbit/auth.go).
//
// The egress client is newEgressClient's, the same one Kavita and Prowlarr get:
// resolve-then-pin against this instance's one validated host:port, so the
// arbitrary internal URL an admin typed into the form cannot be re-resolved
// somewhere else between validation and connection.
func (g *registry) newBookOrbit(si store.ServiceInstance, apiKey string) (*bookorbit.Client, error) {
	httpClient, err := g.newEgressClient(si)
	if err != nil {
		return nil, err
	}
	return bookorbit.New(bookorbit.Options{
		BaseURL:        si.BaseURL + si.URLBase,
		MagicLinkToken: apiKey,
		HTTPClient:     httpClient,
		AppVersion:     g.version,
		// An egress-policy refusal never reached the network, so it is a
		// configuration bug rather than upstream flakiness and must not trip
		// the breaker.
		IsPolicyError: ssrf.IsPolicyError,
		Logger:        g.log.With("instance_id", si.ID),
	})
}

// newProwlarr builds the *Arr client over that instance's egress client.
func (g *registry) newProwlarr(si store.ServiceInstance, apiKey string) (*servarr.Client, error) {
	httpClient, err := g.newEgressClient(si)
	if err != nil {
		return nil, err
	}
	return servarr.NewProwlarr(servarr.Options{
		BaseURL:    si.BaseURL + si.URLBase,
		APIKey:     apiKey,
		HTTPClient: httpClient,
		AppVersion: g.version,
		// An egress-policy refusal never reached the network, so it is a
		// configuration bug rather than upstream flakiness and must not trip
		// the breaker.
		IsPolicyError: ssrf.IsPolicyError,
		Logger:        g.log.With("instance_id", si.ID),
	})
}

func fingerprint(si store.ServiceInstance) string {
	h := sha256.New()
	h.Write([]byte(si.BaseURL))
	h.Write([]byte{0x1f})
	h.Write([]byte(si.URLBase))
	h.Write([]byte{0x1f})
	h.Write(si.APIKeyEnc)
	return hex.EncodeToString(h.Sum(nil))
}

// ── httpapi.ConnectionTester ────────────────────────────────────────────────

// Test runs the wizard's mandatory connection test.
//
// It builds a THROWAWAY client stack rather than reusing the registry's, which
// is what makes reference/security.md §1.6 hold mechanically: a test against a
// base_url the user has just edited uses only the key typed into the form, and
// there is no cached client pinned to the old host that could accidentally be
// picked up.
func (g *registry) Test(ctx context.Context, req httpapi.TestRequest) (httpapi.TestResult, error) {
	kind := strings.ToLower(strings.TrimSpace(req.Kind))

	si := store.ServiceInstance{
		ID:      req.InstanceID,
		Name:    "this service",
		BaseURL: req.BaseURL,
		URLBase: req.URLBase,
	}

	apiKey := req.APIKey
	if strings.TrimSpace(apiKey) == "" {
		if req.InstanceID == 0 {
			return httpapi.TestResult{
				Message: "no API key was supplied and there is no saved credential to use",
				Action:  "Paste the API key",
			}, nil
		}
		stored, err := g.st.GetServiceInstance(ctx, store.Scope{AllInstances: true}, req.InstanceID)
		if err != nil {
			return httpapi.TestResult{}, fmt.Errorf("reading service instance %d: %w", req.InstanceID, err)
		}
		// httpapi has already refused this combination when the host changed;
		// the AAD would refuse it a second time anyway, which is the design.
		if apiKey, err = g.openCredential(ctx, stored); err != nil {
			return httpapi.TestResult{Message: err.Error(), Action: "Re-enter the API key"}, nil
		}
		si.Name = stored.Name
		si.TLSSPKIPin = stored.TLSSPKIPin
		if kind == "" {
			// A re-test of a saved instance whose caller did not echo the kind
			// back. Take it from the STORED ROW rather than defaulting: while
			// prowlarr was the only kind a default was harmless, and with two it
			// would test a Kavita with an *Arr client and report "did not answer"
			// about a service that is answering perfectly.
			kind = stored.Kind
		}
	}
	if kind == "" {
		return httpapi.TestResult{
			Message: "no service kind was supplied, and there is no saved instance to read it from",
			Action:  "Choose a supported service kind",
		}, nil
	}

	switch kind {
	case "kavita":
		return g.testKavita(ctx, req, si, apiKey)
	case "bookorbit":
		return g.testBookOrbit(ctx, req, si, apiKey)
	case "prowlarr":
		// falls through to the *Arr path below
	default:
		return httpapi.TestResult{
			Message: fmt.Sprintf("UsArr cannot talk to %q yet; v0.1 supports bookorbit, kavita and prowlarr", kind),
			Action:  "Choose a supported service kind",
		}, nil
	}

	client, err := g.newProwlarr(si, apiKey)
	if err != nil {
		return httpapi.TestResult{Message: err.Error(), Action: "Check the base URL"}, nil
	}

	// Ping first. It is [AllowAnonymous], so it distinguishes "host down" from
	// "host up, key wrong" — which is the difference between two completely
	// different fixes.
	if _, err := client.Ping(ctx); err != nil {
		return httpapi.TestResult{
			Message: fmt.Sprintf("%s did not answer: %s", req.BaseURL, err.Error()),
			Action:  "Check the base URL and that the service is running",
		}, nil
	}

	status, err := client.SystemStatus(ctx)
	if err != nil {
		return httpapi.TestResult{
			Reachable: true,
			Message:   err.Error(),
			Action:    testAction(err),
		}, nil
	}
	apiInfo, err := client.CheckAPIVersion(ctx)
	if err != nil {
		return httpapi.TestResult{
			Reachable: true, AppName: status.AppName, AppVersion: status.Version,
			Message: err.Error(),
			Action:  "Upgrade the service, or UsArr",
		}, nil
	}

	result := httpapi.TestResult{
		OK:             true,
		Reachable:      true,
		KeyProvenValid: servarr.KeyProvenValid(status),
		AppName:        status.AppName,
		AppVersion:     status.Version,
		APIVersion:     apiInfo.Current,
	}
	if !result.KeyProvenValid {
		// Never claim a verification that did not happen: with
		// AuthenticationRequired=DisabledForLocalAddresses, a request from a
		// local address succeeds with NO key at all.
		result.Message = fmt.Sprintf(
			"reachable: %s %s. This instance accepts local requests without a key, "+
				"so the API key has NOT been verified", status.AppName, status.Version)
	} else {
		result.Message = fmt.Sprintf("connected to %s %s (api %s)", status.AppName, status.Version, apiInfo.Current)
	}
	return result, nil
}

// testKavita is the connection test for a catalogue source.
//
// It answers three DIFFERENT questions in order, and keeping them apart is the
// whole point — each has a different fix, and §17.3 requires the result to name
// the ONE button that applies:
//
//  1. Is anything there? GET /api/Health is [AllowAnonymous] and returns the bare
//     string Ok, so it separates "host down" from "host up, key wrong". It proves
//     nothing else, including that the host is a Kavita.
//  2. Is the Auth Key valid? GET /api/Library/libraries is [Authorize] with no
//     admin policy, so it answers for ANY valid key and 401s for an invalid one.
//     THIS IS THE CREDENTIAL PROOF, and it is why the test does not stop at (1).
//  3. What version is it? GET /api/Server/server-info-slim is ADMIN-ONLY, so a
//     valid non-admin key gets 403 here. That is NOT a failure: the test still
//     passes, the version is reported as unknown, and the message says why —
//     because telling a user to re-enter a working key is worse than not knowing
//     a version.
//
// Note what is NOT called: GET /api/Plugin/version, the non-admin version
// fallback, puts the Auth Key in the query string. A version number does not
// justify writing a credential into every access log between here and there.
func (g *registry) testKavita(ctx context.Context, req httpapi.TestRequest, si store.ServiceInstance, apiKey string) (httpapi.TestResult, error) {
	client, err := g.newKavita(si, apiKey)
	if err != nil {
		return httpapi.TestResult{Message: err.Error(), Action: "Check the base URL"}, nil
	}

	if err := client.Health(ctx); err != nil {
		return httpapi.TestResult{
			Message: fmt.Sprintf("%s did not answer: %s", req.BaseURL, err.Error()),
			Action:  "Check the base URL and that Kavita is running",
		}, nil
	}

	libs, err := client.Libraries(ctx)
	if err != nil {
		return httpapi.TestResult{
			Reachable: true,
			Message:   err.Error(),
			Action:    kavitaTestAction(err),
		}, nil
	}

	// Past here the key is PROVEN: an invalid one cannot reach this line, because
	// LibraryController refuses it with a 401 before the handler runs. Unlike an
	// *Arr, Kavita has no "disabled for local addresses" mode that would let an
	// unauthenticated request through, so there is no honesty caveat to attach.
	result := httpapi.TestResult{
		OK: true, Reachable: true, KeyProvenValid: true,
		AppName: "Kavita",
	}

	info, err := client.ServerInfo(ctx)
	switch {
	case err == nil:
		result.AppVersion = info.KavitaVersion
		result.Message = fmt.Sprintf("connected to Kavita %s; %s visible to this account",
			info.KavitaVersion, pluralLibraries(len(libs)))
	case errors.Is(err, kavita.ErrForbidden):
		// A valid NON-ADMIN Auth Key. Everything the sync needs works; only the
		// version endpoint is out of reach. Say so exactly rather than failing.
		result.Message = fmt.Sprintf(
			"connected to Kavita; %s visible to this account. The version could not be read: "+
				"server-info-slim is admin-only, so this Auth Key belongs to a non-admin account",
			pluralLibraries(len(libs)))
	default:
		result.Message = fmt.Sprintf("connected to Kavita; %s visible to this account. "+
			"The version could not be read: %s", pluralLibraries(len(libs)), err.Error())
	}

	if len(libs) == 0 {
		// Reachable, credential proven, and nothing to sync. That is a real
		// configuration problem and it must not read as success with no comment.
		result.Message += ". This account can see NO libraries, so there is nothing for UsArr to replicate"
		result.Action = "Grant this Kavita account access to a library"
	}
	return result, nil
}

// testBookOrbit is the connection test for ADR-0052's catalogue source.
//
// It answers three DIFFERENT questions in order, the same discipline testKavita
// keeps and for the same §17.3 reason — each has a different fix, and the result
// must name the ONE button that applies:
//
//  1. Is anything there? GET /api/v1/health is @Public() at class level, so it
//     answers with NO credential and separates "host down" from "host up, token
//     wrong". Unlike Kavita's bare `Ok` it returns a Terminus report naming each
//     indicator, so a 503 here is a DIAGNOSIS and is carried forward rather than
//     ending the test: a failing indicator does not stop a login, and the
//     credential answer is the more useful half.
//  2. Is the token good? POST /api/v1/auth/magic-links/login is the ONLY thing
//     that can answer it. MagicLinkService.loginWithToken checks the hash, the
//     revocation, the activation, the expiry and the user, and 401s on all five
//     with one message. THIS IS THE CREDENTIAL PROOF, and it is why the test does
//     not stop at (1).
//  3. Does the bearer actually work? GET /api/v1/app-info declares no
//     @RequirePermission and no library guard, so it answers for any valid JWT.
//     A mint proves nothing about a GUARDED route: the login route is @Public(),
//     so JwtAuthGuard never ran at (2). The link can be revoked, deactivated or
//     expired in the window after it — the client re-mints once, so a 401 that
//     survives is the stored token failing — and a 403 is the account behind the
//     link being refused rather than the token. Either is what a "green tick
//     after step 2" would hide. So an auth-class failure HERE fails the test;
//     anything else is soft and costs only the version.
//
// WHAT IT NEVER DOES is read a library list. testKavita's credential proof is
// GET /api/Library/libraries and this one's deliberately is not: knowing what a
// BookOrbit library is belongs to slice 1, and a connection test that already
// enumerated containers would have quietly started the catalogue adapter.
// app-info is the equivalent proof with none of that reach.
func (g *registry) testBookOrbit(ctx context.Context, req httpapi.TestRequest, si store.ServiceInstance, token string) (httpapi.TestResult, error) {
	client, err := g.newBookOrbit(si, token)
	if err != nil {
		return httpapi.TestResult{Message: err.Error(), Action: "Check the base URL"}, nil
	}

	// (1) Reachability. A 503 with a parsed report is ANSWERED, not failed.
	healthNote := ""
	if _, err := client.Health(ctx); err != nil {
		if !errors.Is(err, bookorbit.ErrUnhealthy) {
			return httpapi.TestResult{
				Message: fmt.Sprintf("%s did not answer: %s", req.BaseURL, err.Error()),
				Action:  "Check the base URL and that BookOrbit is running",
			}, nil
		}
		// Carried, not fatal: BookOrbit is up and has named what is wrong with
		// itself. Swallowing it would drop the one thing on this screen that
		// explains a sync that later goes quiet.
		healthNote = " BookOrbit reports a failing health check: " + err.Error()
	}

	// (2) The credential proof, and the §14 scope verdict that rides on it for
	// free — buildUserResponse ships the account's flags and permissions in the
	// same body as the accessToken, so classification costs no second request.
	verdict, err := client.Authenticate(ctx)
	if err != nil {
		return httpapi.TestResult{
			Reachable: true,
			Message:   err.Error() + healthNote,
			Action:    bookOrbitTestAction(err),
		}, nil
	}

	// Past here the TOKEN is proven: an unknown, revoked, deactivated, expired or
	// orphaned one cannot reach this line. That stays true below even when the
	// test goes on to fail, and the field says so — "the token is fine, the
	// account behind it is not" is a different bug report from "bad token".
	result := httpapi.TestResult{
		OK: true, Reachable: true, KeyProvenValid: true,
		AppName: "BookOrbit",
		// The api PATH, observed rather than assumed: every call above was made
		// under main.ts's global prefix and answered there. It is not probed the
		// way an *Arr's is because BookOrbit publishes no version-negotiation
		// endpoint to probe — there is one prefix and this is it.
		APIVersion: "v1",
	}

	// (3) The authenticated read.
	info, err := client.AppInfo(ctx)
	switch {
	case err == nil:
		if info.VersionKnown() {
			result.AppVersion = info.Version
			result.Message = "connected to BookOrbit " + info.Version
		} else {
			// `process.env.APP_VERSION ?? 'Local build'`. Reporting "Local build"
			// as a version would make a self-built instance look like a release
			// called that, and any future version floor must read it as UNKNOWN
			// rather than as too old.
			result.Message = "connected to BookOrbit; it reports no release version, " +
				"which is what a self-built or development instance does"
		}
	case errors.Is(err, bookorbit.ErrUnauthorized),
		errors.Is(err, bookorbit.ErrForbidden),
		errors.Is(err, bookorbit.ErrPasswordChangeRequired):
		// The mint worked and the bearer it produced is refused on the most
		// permissive route BookOrbit has. Nothing this adapter will ever need can
		// work, so this is a FAILED test — with key_proven_valid left true,
		// because the stored token is not the thing that is wrong.
		return httpapi.TestResult{
			Reachable: true, KeyProvenValid: true, AppName: "BookOrbit",
			Message: "the magic-link token is valid and BookOrbit issued an access token, but that " +
				"token was refused on GET /api/v1/app-info, which needs no permission at all: " +
				err.Error() + healthNote,
			Action: bookOrbitTestAction(err),
		}, nil
	default:
		// Soft, exactly as testKavita treats an admin-only version endpoint: the
		// credential is proven and only the version is missing, and telling a
		// user to re-enter a working token is worse than not knowing a version.
		result.Message = "connected to BookOrbit. The version could not be read: " + err.Error()
	}

	result.Message += healthNote
	if note, action := bookOrbitScopeNote(verdict); note != "" {
		result.Message += note
		result.Action = action
	}
	return result, nil
}

// bookOrbitScopeNote renders the §14 scope verdict for a PASSING test, and the
// judgement it encodes is deliberate: an over-scoped credential is REPORTED AND
// STORED, never refused.
//
// Three reasons, in the order they decide it:
//
//  1. internal/bookorbit already settled the policy for this exact verdict —
//     "This client REPORTS rather than refuses" (doc.go) — and a connection test
//     that refused would overturn it from the outside, leaving a user with a
//     working token, a service they cannot add, and a fix that is entirely on
//     BookOrbit's side of the fence.
//  2. ADR-0052's gate is on the CATALOGUE READ, not on storing the credential:
//     "may not read a catalogue under a shared-account credential until the scope
//     that account grants has been enumerated against §14". Enumerating is what
//     scope.go does. Slice 1 is the caller that has to consult Elevated() before
//     its first read, and it can only do that if the instance exists.
//  3. CLAUDE.md's third principle. A refusal here would be a screen that says no
//     without saying what to do, for a service that is answering perfectly.
//
// What it is NOT is silent. The finding count and the elevated grants are in the
// message, and Action names the fix, so the warning is in front of the user at the
// one moment the fix is one step away — while they are still holding the BookOrbit
// admin screen that mints links. The prober repeats it on the Services screen
// afterwards (probeBookOrbit), so it does not evaporate with the panel.
//
// NOTHING PERSONAL CROSSES. AccountView is already an allowlist with no email,
// name or avatar on it, and this reads only the permission names and the findings'
// own prose — never Username, which is the one remaining field that could name a
// human.
func bookOrbitScopeNote(v bookorbit.ScopeVerdict) (note, action string) {
	if v.Minimal() {
		return "", ""
	}
	var elevated, unneeded []string
	for _, f := range v.Findings {
		label := bookOrbitFindingLabel(f)
		if f.Severity == bookorbit.ScopeElevated {
			elevated = append(elevated, label)
			continue
		}
		unneeded = append(unneeded, label)
	}

	if len(elevated) == 0 {
		return fmt.Sprintf(" This account carries %s UsArr does not use: %s. Harmless, "+
			"but a narrower account is a smaller blast radius.",
			plural(len(unneeded), "grant", "grants"), joinCapped(unneeded)), ""
	}
	return fmt.Sprintf(" THE ACCOUNT BEHIND THIS TOKEN IS OVER-SCOPED: %s with write or admin "+
			"reach beyond a catalogue read — %s. UsArr has stored the token and will use it, because "+
			"refusing a working credential fixes nothing; but everything UsArr does here is read-only, "+
			"so nothing above an empty permission set is needed.",
			plural(len(elevated), "grant", "grants"), joinCapped(elevated)),
		"Mint the link against a shared BookOrbit account with NO permissions and an explicit library grant"
}

// bookOrbitFindingLabel is one finding in a few words.
//
// A permission finding IS its permission name. The three that are not about a
// permission — the superuser flag, the provisioning method, an inactive account
// — carry a whole sentence in Detail, written headline-first with the evidence
// after a semicolon (scope.go's classifyScope). Taking the headline keeps the
// message readable; the full reasoning stays where a reader who wants it will
// look, which is the grader.
func bookOrbitFindingLabel(f bookorbit.ScopeFinding) string {
	if f.Permission != "" {
		return string(f.Permission)
	}
	if head, _, ok := strings.Cut(f.Detail, ";"); ok {
		return head
	}
	return f.Detail
}

// joinCapped renders at most a handful of findings. BookOrbit has more than
// twenty permissions and a superuser holds all of them; a message that listed
// every one would be a wall of text nobody reads, which is the same failure as
// saying nothing.
func joinCapped(items []string) string {
	const shown = 4
	if len(items) <= shown {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:shown], ", "), len(items)-shown)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// bookOrbitTestAction maps a BookOrbit failure onto the ONE button that fixes it.
func bookOrbitTestAction(err error) string {
	switch {
	case errors.Is(err, bookorbit.ErrPasswordChangeRequired):
		// Not a credential failure, so it must not read as one — re-pasting the
		// token will not help. But nor is it a link minted against an ordinary
		// account: MagicLinkService.createToken refuses any target that is not
		// shared-provisioned, and no shared account carries isDefaultPassword.
		// The account is in a state the mint path cannot produce, which puts the
		// fix upstream rather than in anything the user can retype here.
		return "Check this account in BookOrbit: it demands a password change, which a shared magic-link account cannot require — the fix is on the BookOrbit side, not the stored token"
	case errors.Is(err, bookorbit.ErrUnauthorized):
		// NOT "mint a new one", which is what this said until ADR-0060 measured
		// BookOrbit at 73b7877: `magic_access_tokens.raw_token` is plaintext
		// beside the hash, and MagicLinkRepository.findAll returns every row's
		// rawToken to any superuser. The token IS re-readable, so comparing it is
		// both possible and strictly more informative than rotating — a rotation
		// throws away the only evidence of what was actually wrong.
		return "Compare the stored token against BookOrbit's own list (Settings → Magic Links, superuser only): this one is unknown, revoked, deactivated or expired"
	case errors.Is(err, bookorbit.ErrForbidden):
		return "Use a magic-link token whose account is active and shared-provisioned"
	case errors.Is(err, bookorbit.ErrThrottled):
		// 10 logins a minute on the login route. Not a credential problem at all.
		return "Wait a minute: BookOrbit throttles the login route to 10 requests per minute"
	case errors.Is(err, bookorbit.ErrTimeout):
		return "Check the base URL and that BookOrbit is running"
	case errors.Is(err, bookorbit.ErrWrongService), errors.Is(err, bookorbit.ErrDecode):
		return "Check the base URL: that host answered, but not like a BookOrbit"
	}
	return "Test connection"
}

func pluralLibraries(n int) string {
	if n == 1 {
		return "1 library"
	}
	return fmt.Sprintf("%d libraries", n)
}

// kavitaTestAction maps a Kavita failure onto the ONE button that fixes it.
func kavitaTestAction(err error) string {
	switch {
	case errors.Is(err, kavita.ErrUnauthorized):
		return "Update the Auth Key (Kavita → User Settings → Manage Auth Keys)"
	case errors.Is(err, kavita.ErrForbidden):
		return "Use an Auth Key whose account can see at least one library"
	case errors.Is(err, kavita.ErrTimeout):
		return "Check the base URL and that Kavita is running"
	case errors.Is(err, kavita.ErrDecode):
		// A 200 that is not the shape Kavita returns almost always means the URL
		// points at something else — a reverse-proxy landing page, another app.
		return "Check the base URL: that host answered, but not like a Kavita"
	}
	return "Test connection"
}

// testAction maps a failure onto the ONE button that fixes it (§17.3).
func testAction(err error) string {
	switch {
	case errors.Is(err, servarr.ErrUnauthorized):
		return "Update API key"
	case errors.Is(err, servarr.ErrWrongService):
		return "Check the base URL: that host is a different application"
	case errors.Is(err, servarr.ErrTimeout):
		return "Check the base URL and that the service is running"
	case errors.Is(err, servarr.ErrUnsupportedAPIVersion):
		return "Upgrade the service, or UsArr"
	}
	return "Test connection"
}

// ── httpapi.HealthProbes ────────────────────────────────────────────────────

// Snapshot returns the last probe. It takes a read lock and nothing else: it is
// called from a render path.
func (g *registry) Snapshot(instanceID int64) (httpapi.UpstreamHealth, bool) {
	g.probeMu.RLock()
	defer g.probeMu.RUnlock()
	h, ok := g.probes[instanceID]
	return h, ok
}

// ProbeNow asks for an out-of-band refresh and returns immediately. A full
// channel means a probe is already queued, which is the same outcome.
func (g *registry) ProbeNow(instanceID int64) {
	select {
	case g.probeReq <- instanceID:
	default:
	}
}

// RunProber is the background loop. It is the ONLY thing that talks to an *Arr
// for health, so no render path ever does.
func (g *registry) RunProber(ctx context.Context) {
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()

	g.probeAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.probeAll(ctx)
		case id := <-g.probeReq:
			g.probe(ctx, id)
		}
	}
}

func (g *registry) probeAll(ctx context.Context) {
	instances, err := g.st.ListServiceInstances(ctx, store.Scope{AllInstances: true})
	if err != nil {
		g.log.Warn("health probe: services could not be listed", "err", err)
		return
	}
	for _, si := range instances {
		if !si.Enabled {
			continue
		}
		g.probe(ctx, si.ID)
	}
}

// probe refreshes one instance and writes both the in-memory snapshot and the
// persisted health columns.
//
// Persisting matters: a restart must not blank the Services screen, and
// last_error is what §17.3's Problem column renders when no probe has run yet.
func (g *registry) probe(ctx context.Context, instanceID int64) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	now := time.Now()
	health := httpapi.UpstreamHealth{InstanceID: instanceID, ObservedAt: now}

	entry, err := g.entry(ctx, instanceID)
	if err != nil {
		// Redact BEFORE the value reaches the row. last_error is persisted, and a
		// stored credential is retroactive: it survives in every backup taken before
		// the fix. Redacting on read-out does not reach the bytes on disk.
		health.Error = ssrf.RedactText(err.Error())
		g.recordProbe(ctx, instanceID, health, nil)
		return
	}
	health.BreakerState, health.BreakerRetryAt = entry.breakerState()

	if entry.kavita != nil {
		g.probeKavita(ctx, entry, health, now)
		return
	}
	if entry.bookorbit != nil {
		g.probeBookOrbit(ctx, entry, health, now)
		return
	}

	status, err := entry.prowlarr.SystemStatus(ctx)
	if err != nil {
		// Redacted before the row, as above.
		health.Error = ssrf.RedactText(err.Error())
		health.BreakerState, health.BreakerRetryAt = entry.breakerState()
		g.recordProbe(ctx, instanceID, health, nil)
		return
	}
	health.AppVersion = status.Version

	// The *Arr's own health warnings. A failure here is soft: a missing warning
	// list must not turn a healthy instance into a red row.
	if warnings, err := entry.prowlarr.Health(ctx); err != nil {
		g.log.Debug("upstream health warnings unavailable", "instance_id", instanceID, "err", err)
	} else {
		for _, wr := range warnings {
			if wr.Type == servarr.HealthOK {
				continue
			}
			health.Warnings = append(health.Warnings, httpapi.UpstreamWarning{
				Source:  wr.Source,
				Type:    string(wr.Type),
				Message: wr.Message,
				WikiURL: wr.WikiURL,
			})
		}
	}

	// The indexer catalogue. This is the ONLY place UsArr reads an indexer list,
	// and it is off every render path by construction — which is what lets
	// GET /api/v1/indexers populate a picker from SQLite instead of blocking a
	// browser on Prowlarr (ARCHITECTURE.md §2.3 rule 1, migration 0004).
	//
	// A failure here is SOFT and leaves the replica alone: the previous good
	// copy is better than an empty picker, and indexers_fetched_at then keeps
	// saying how old it is rather than claiming a refresh that did not happen.
	// The names are reused below so the blocked list costs no second call.
	names := map[int32]string{}
	if ixs, err := entry.prowlarr.Indexers(ctx); err != nil {
		g.log.Debug("indexer list unavailable; the local catalogue keeps its last good copy",
			"instance_id", instanceID, "err", err)
	} else {
		for _, ix := range ixs {
			names[ix.ID] = ix.Name
		}
		if err := g.replicateIndexers(ctx, instanceID, ixs); err != nil {
			g.log.Warn("the indexer catalogue could not be replicated",
				"instance_id", instanceID, "err", err)
		}
	}

	// Blocked indexers. GET /api/v1/indexerstatus returns ONLY blocked ones, so
	// an empty array means everything is healthy — that is not the same as the
	// call having failed, and the two are reported differently.
	if statuses, err := entry.prowlarr.IndexerStatus(ctx); err != nil {
		g.log.Debug("indexerstatus unavailable", "instance_id", instanceID, "err", err)
	} else if len(statuses) > 0 {
		for _, st := range statuses {
			health.BlockedIndexers = append(health.BlockedIndexers, httpapi.BlockedIndexer{
				IndexerID:         st.IndexerID,
				Name:              names[st.IndexerID],
				DisabledTill:      st.DisabledTill,
				MostRecentFailure: st.MostRecentFailure,
			})
		}
	}

	health.BreakerState, health.BreakerRetryAt = entry.breakerState()
	g.recordProbe(ctx, instanceID, health, &now)
}

// probeKavita is the catalogue source's health probe.
//
// It is deliberately SMALLER than the *Arr one, and the difference is not an
// omission: Kavita has no health-warnings endpoint and no indexer catalogue to
// replicate, so there is nothing to project. What it does read is
// GET /api/Library/libraries, for the same two reasons the connection test does —
// it PROVES THE AUTH KEY still works (an expired one fails here and nowhere
// else), and it is the container list §17.8's library binding is drawn from.
//
// The version read is a soft failure. server-info-slim is admin-only, so a valid
// non-admin key gets a 403 that must leave the row HEALTHY with an unknown
// version rather than painting a red row over a service that is working.
func (g *registry) probeKavita(ctx context.Context, entry *registryEntry, health httpapi.UpstreamHealth, now time.Time) {
	instanceID := entry.instance.ID

	libs, err := entry.kavita.Libraries(ctx)
	if err != nil {
		// Redacted before the row, for the reason given in probe.
		health.Error = ssrf.RedactText(err.Error())
		health.BreakerState, health.BreakerRetryAt = entry.breakerState()
		g.recordProbe(ctx, instanceID, health, nil)
		return
	}

	if info, err := entry.kavita.ServerInfo(ctx); err != nil {
		g.log.Debug("kavita version unavailable (server-info-slim is admin-only)",
			"instance_id", instanceID, "err", err)
	} else {
		health.AppVersion = info.KavitaVersion
	}

	if len(libs) == 0 {
		// Reachable and authenticated, with nothing to replicate. Surfaced as a
		// warning rather than swallowed: an empty catalogue that looks healthy is
		// exactly the "empty screen that looks broken" CLAUDE.md's third principle
		// refuses.
		health.Warnings = append(health.Warnings, httpapi.UpstreamWarning{
			Source:  "UsArr",
			Type:    "warning",
			Message: "this Kavita account can see no libraries, so there is nothing to replicate",
		})
	}

	health.BreakerState, health.BreakerRetryAt = entry.breakerState()
	g.recordProbe(ctx, instanceID, health, &now)
}

// probeBookOrbit is the BookOrbit catalogue source's health probe.
//
// It reads two things, and the pair is chosen rather than inherited:
//
//   - GET /api/v1/health, which needs NO credential and returns a Terminus
//     report naming each indicator. This is the closest thing any v0.1 service
//     has to an *Arr's own health warnings, and surfacing those is the single
//     most valuable thing on the Services screen — so a failing indicator
//     becomes a WARNING on a row that stays otherwise honest, not a red row and
//     not a swallowed detail.
//   - GET /api/v1/app-info, which is the authenticated read. It is what keeps
//     the credential under continuous proof: a magic-link token that is revoked
//     upstream at 3am fails HERE and nowhere else, because nothing else in this
//     build calls BookOrbit at all. It also carries the version.
//
// It does NOT read a library list, which is the one structural difference from
// probeKavita. probeKavita reads GET /api/Library/libraries for two reasons —
// proving the key, and fetching the container list §17.8's library binding
// needs — and here the first is app-info's job while the second belongs to
// slice 1. A prober that enumerated containers now would be the catalogue
// adapter arriving through the back door.
//
// The mint is nearly free: Authenticate reuses the in-memory access token and
// only re-mints ~60s before its ~15-minute expiry, so a 60-second probe interval
// costs roughly one login per fifteen probes against a 10-per-minute throttle.
func (g *registry) probeBookOrbit(ctx context.Context, entry *registryEntry, health httpapi.UpstreamHealth, now time.Time) {
	instanceID := entry.instance.ID

	if report, err := entry.bookorbit.Health(ctx); err != nil {
		if !errors.Is(err, bookorbit.ErrUnhealthy) {
			// Redacted before the row, for the reason given in probe.
			health.Error = ssrf.RedactText(err.Error())
			health.BreakerState, health.BreakerRetryAt = entry.breakerState()
			g.recordProbe(ctx, instanceID, health, nil)
			return
		}
		// Answered, and told us which indicator is down. The instance is
		// reachable, so this is a warning on a live row rather than a failure —
		// and it is the diagnosis, so it must not be dropped.
		for _, ind := range report.Failing() {
			message := ind.Name + " is " + ind.Status
			if ind.Message != "" {
				message = ind.Name + ": " + ind.Message
			}
			health.Warnings = append(health.Warnings, httpapi.UpstreamWarning{
				Source: "BookOrbit", Type: "warning",
				// Redacted like every other upstream string that reaches a
				// browser-facing row, even though internal/bookorbit has already
				// cleaned it: the second pass costs nothing and the day the first
				// one stops running is not announced.
				Message: ssrf.RedactText(message),
			})
		}
	}

	info, err := entry.bookorbit.AppInfo(ctx)
	if err != nil {
		// The authenticated read is where a revoked or expired magic-link token
		// surfaces, so this one IS a failure: unlike Kavita's admin-only version
		// endpoint, app-info needs no permission and any account can read it.
		health.Error = ssrf.RedactText(err.Error())
		health.BreakerState, health.BreakerRetryAt = entry.breakerState()
		g.recordProbe(ctx, instanceID, health, nil)
		return
	}
	if info.VersionKnown() {
		// "Local build" is deliberately not written here: it is not a version,
		// and a column showing it as one would make a development instance look
		// like a release nobody can find.
		health.AppVersion = info.Version
	}

	// The §14 scope verdict, repeated on the Services screen so it does not
	// evaporate with the connection-test panel the user has long since closed.
	// It costs no request — it was computed from the login body.
	if verdict, ok := entry.bookorbit.Scope(); ok {
		if note, _ := bookOrbitScopeNote(verdict); note != "" {
			health.Warnings = append(health.Warnings, httpapi.UpstreamWarning{
				Source: "UsArr", Type: "warning", Message: strings.TrimSpace(note),
			})
		}
	}

	health.BreakerState, health.BreakerRetryAt = entry.breakerState()
	g.recordProbe(ctx, instanceID, health, &now)
}

// replicateIndexers projects the upstream indexer list and writes it to
// indexer_catalog, replacing this instance's rows wholesale.
//
// THE PROJECTION IS THE SECURITY BOUNDARY, and it is why this function does not
// simply hand servarr resources to the store. IndexerResource carries
// `fields[]`, whose entries have privacy ∈ {password, apiKey, userName}: for a
// private tracker that is the user's PASSKEY, its RSS key, its API key or its
// session cookie, and a leaked passkey is account termination. This project has
// already written one credential to SQLite once — release_candidate.info_url,
// redacted after the fact, in every backup until it was — so the rule applied
// here is the stronger one: the value never reaches the row at all.
//
// mapping.FromProwlarrIndexer names every member it copies, store.Indexer has
// no member that could hold a credential, and the two JSON columns are
// marshalled from mapping's own projected structs rather than from anything
// upstream sent. Three allowlists, no redaction pass, nothing passed through.
func (g *registry) replicateIndexers(ctx context.Context, instanceID int64, ixs []servarr.IndexerResource) error {
	now := time.Now()
	rows := make([]store.Indexer, 0, len(ixs))
	for _, raw := range ixs {
		ix := mapping.FromProwlarrIndexer(raw)

		searchTypes, err := marshalJSONArray(ix.SearchTypes)
		if err != nil {
			return fmt.Errorf("encoding search types for indexer %d: %w", ix.ID, err)
		}
		categories, err := marshalJSONArray(ix.Categories)
		if err != nil {
			return fmt.Errorf("encoding categories for indexer %d: %w", ix.ID, err)
		}

		row := store.Indexer{
			ServiceInstanceID:  instanceID,
			IndexerID:          int64(ix.ID),
			Name:               ix.Name,
			Protocol:           ix.Protocol,
			Privacy:            ix.Privacy,
			Enabled:            ix.Enabled,
			Priority:           int64(ix.Priority),
			SupportsSearch:     ix.SupportsSearch,
			SupportsRSS:        ix.SupportsRSS,
			SupportsPagination: ix.SupportsPagination,
			SearchTypesJSON:    searchTypes,
			CategoriesJSON:     categories,
		}
		// nil and 0 are different: an indexer that advertised no limit must not
		// be recorded as one that returns nothing.
		if ix.LimitsMax != nil {
			row.LimitsMax = sql.NullInt64{Int64: int64(*ix.LimitsMax), Valid: true}
		}
		if ix.LimitsDefault != nil {
			row.LimitsDefault = sql.NullInt64{Int64: int64(*ix.LimitsDefault), Valid: true}
		}
		rows = append(rows, row)
	}
	return g.st.ReplaceIndexers(ctx, instanceID, rows, now)
}

// marshalJSONArray renders a slice as JSON, mapping an empty or nil slice onto
// `[]` rather than encoding/json's `null`. The columns are NOT NULL with a `[]`
// default, and a reader that has to treat null and [] alike is one branch away
// from treating one of them as "unknown".
func marshalJSONArray[T any](v []T) (string, error) {
	if len(v) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (g *registry) recordProbe(ctx context.Context, instanceID int64, health httpapi.UpstreamHealth, lastOK *time.Time) {
	g.probeMu.Lock()
	g.probes[instanceID] = health
	g.probeMu.Unlock()

	// The State column is the BREAKER's opinion of the instance
	// (ARCHITECTURE.md §17.3), not the instance's opinion of itself. An *Arr
	// with its own health warnings is still perfectly reachable and its data is
	// still fresh, so it stays healthy here and its warnings are rendered in
	// their own column. Conflating the two would paint a permanent amber row on
	// every install that has one indexer with an expired cookie.
	state := "healthy"
	var failures int64
	if health.Error != "" {
		state, failures = "down", 1
	}
	breaker := health.BreakerState
	if breaker == "" {
		breaker = "closed"
	}
	if err := g.st.UpdateServiceInstanceHealth(ctx, instanceID, state, breaker, failures, lastOK, health.Error); err != nil {
		g.log.Debug("health columns not updated", "instance_id", instanceID, "err", err)
	}
}

// The registry reads instances with a full scope on purpose: it is process
// machinery, not a user-facing read. Every caller that reaches it from a request
// has already applied the caller's access scope in internal/httpapi, and the
// background prober has no user at all.
var (
	_ httpapi.ReleaseServices  = (*registry)(nil)
	_ httpapi.ConnectionTester = (*registry)(nil)
	_ httpapi.HealthProbes     = (*registry)(nil)
)
