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
	"strings"
	"sync"
	"time"

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

	mu      sync.Mutex
	entries map[int64]*registryEntry

	probeMu sync.RWMutex
	probes  map[int64]httpapi.UpstreamHealth

	probeReq chan int64
}

// registryEntry is one instance's live client stack.
//
// EXACTLY ONE of prowlarr and kavita is non-nil, chosen by instance.Kind. They
// are separate fields rather than one interface because the two clients have
// almost nothing in common at the call site — one is searched and grabbed from,
// the other is replicated from — and an interface wide enough for both would be
// the union of two APIs with no shared caller.
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
	}
	return "closed", time.Time{}
}

func newRegistry(st *store.Store, keyring *crypto.Keyring, log *slog.Logger, version string) *registry {
	return &registry{
		st:       st,
		keyring:  keyring,
		log:      log,
		version:  version,
		entries:  map[int64]*registryEntry{},
		probes:   map[int64]httpapi.UpstreamHealth{},
		probeReq: make(chan int64, 32),
	}
}

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

	apiKey, err := g.openCredential(si)
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
// a valid keyring means tampering or an edited base_url, and it is loud.
func (g *registry) openCredential(si store.ServiceInstance) (string, error) {
	aad, err := crypto.ServiceInstanceAAD(si.ID, si.BaseURL)
	if err != nil {
		return "", fmt.Errorf("building AAD for %q: %w", si.Name, err)
	}
	plain, err := g.keyring.Open(si.APIKeyEnc, aad)
	if err != nil {
		if errors.Is(err, crypto.ErrDecrypt) {
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
		if apiKey, err = g.openCredential(stored); err != nil {
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
	case "prowlarr":
		// falls through to the *Arr path below
	default:
		return httpapi.TestResult{
			Message: fmt.Sprintf("UsArr cannot talk to %q yet; v0.1 supports kavita and prowlarr", kind),
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
		health.Error = err.Error()
		g.recordProbe(ctx, instanceID, health, nil)
		return
	}
	health.BreakerState, health.BreakerRetryAt = entry.breakerState()

	if entry.kavita != nil {
		g.probeKavita(ctx, entry, health, now)
		return
	}

	status, err := entry.prowlarr.SystemStatus(ctx)
	if err != nil {
		health.Error = err.Error()
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
		health.Error = err.Error()
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
