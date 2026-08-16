package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/httpapi"
	"github.com/jdb3750/UsArr/internal/releases"
	"github.com/jdb3750/UsArr/internal/servarr"
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

type registryEntry struct {
	// fingerprint changes whenever base_url or the stored envelope changes, so
	// an edited instance gets a fresh client rather than one pinned to the old
	// host with the old key.
	fingerprint string
	client      *servarr.Client
	service     *releases.Service
	instance    store.ServiceInstance
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
func (g *registry) For(ctx context.Context, instanceID int64) (httpapi.ReleaseSearcher, error) {
	entry, err := g.entry(ctx, instanceID)
	if err != nil {
		return nil, err
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
	if si.Role != "indexer" {
		return nil, fmt.Errorf("%q is a %s service; only indexer services can be searched today", si.Name, si.Role)
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
	client, err := g.newClient(si, apiKey)
	if err != nil {
		return nil, err
	}
	service, err := releases.NewService(releases.Config{
		InstanceID: si.ID,
		Client:     client,
		Store:      newReleaseStore(g.st),
		Logger:     g.log.With("instance_id", si.ID, "instance", si.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("building release service for %q: %w", si.Name, err)
	}

	e := &registryEntry{fingerprint: print, client: client, service: service, instance: si}
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
			return "", fmt.Errorf("the stored API key for %q cannot be opened for %s: "+
				"re-enter it, or restore the master key that sealed it",
				si.Name, si.BaseURL)
		}
		return "", fmt.Errorf("opening the stored API key for %q: %w", si.Name, err)
	}
	return string(plain), nil
}

// newClient builds the SSRF-policy HTTP client and the *Arr client over it.
func (g *registry) newClient(si store.ServiceInstance, apiKey string) (*servarr.Client, error) {
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
	if kind != "" && kind != "prowlarr" {
		return httpapi.TestResult{
			Message: fmt.Sprintf("UsArr cannot talk to %q yet; v0.1 supports prowlarr", req.Kind),
			Action:  "Choose a supported service kind",
		}, nil
	}

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
	}

	client, err := g.newClient(si, apiKey)
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
	health.BreakerState = entry.client.Breaker().State().String()
	health.BreakerRetryAt = entry.client.Breaker().RetryAt()

	status, err := entry.client.SystemStatus(ctx)
	if err != nil {
		health.Error = err.Error()
		health.BreakerState = entry.client.Breaker().State().String()
		health.BreakerRetryAt = entry.client.Breaker().RetryAt()
		g.recordProbe(ctx, instanceID, health, nil)
		return
	}
	health.AppVersion = status.Version

	// The *Arr's own health warnings. A failure here is soft: a missing warning
	// list must not turn a healthy instance into a red row.
	if warnings, err := entry.client.Health(ctx); err != nil {
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

	// Blocked indexers. GET /api/v1/indexerstatus returns ONLY blocked ones, so
	// an empty array means everything is healthy — that is not the same as the
	// call having failed, and the two are reported differently.
	if statuses, err := entry.client.IndexerStatus(ctx); err != nil {
		g.log.Debug("indexerstatus unavailable", "instance_id", instanceID, "err", err)
	} else if len(statuses) > 0 {
		names := map[int32]string{}
		if ixs, err := entry.client.Indexers(ctx); err == nil {
			for _, ix := range ixs {
				names[ix.ID] = ix.Name
			}
		}
		for _, st := range statuses {
			health.BlockedIndexers = append(health.BlockedIndexers, httpapi.BlockedIndexer{
				IndexerID:         st.IndexerID,
				Name:              names[st.IndexerID],
				DisabledTill:      st.DisabledTill,
				MostRecentFailure: st.MostRecentFailure,
			})
		}
	}

	health.BreakerState = entry.client.Breaker().State().String()
	health.BreakerRetryAt = entry.client.Breaker().RetryAt()
	g.recordProbe(ctx, instanceID, health, &now)
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
