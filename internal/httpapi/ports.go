package httpapi

import (
	"context"
	"errors"
	"time"

	"github.com/jdb3750/UsArr/internal/releases"
)

// The interfaces here are the whole reason this package can obey "no handler
// holds an outbound HTTP client". Each one is declared by the consumer — this
// package — and implemented in cmd/usarr, which owns internal/servarr and
// internal/ssrf. Nothing below names an *Arr type.

// ReleaseSearcher is the Search-and-Grab surface over one indexer instance.
// *releases.Service satisfies it.
type ReleaseSearcher interface {
	InstanceID() int64
	Search(ctx context.Context, scope releases.Scope, q releases.Query) (<-chan releases.Event, error)
	Grab(ctx context.Context, scope releases.Scope, candidateID int64) (releases.GrabResult, error)
}

// ReleaseServices resolves the searcher fronting one service_instance.
//
// It returns an error rather than a nil searcher when the instance is not an
// indexer, is disabled, or cannot be opened — "Prowlarr is configured but its
// credential will not decrypt" is a thing the user must be told, not an empty
// screen (principle 3).
type ReleaseServices interface {
	For(ctx context.Context, instanceID int64) (ReleaseSearcher, error)
}

// ErrNoIndexerService means no enabled indexer-role instance is configured, so
// there is nothing to search. Handlers turn it into a 409 that says exactly
// that, never into an empty result set.
var ErrNoIndexerService = errors.New("httpapi: no enabled indexer service is configured")

// TestRequest is one connection test from the §11 wizard.
//
// APIKey is the key TYPED INTO THE FORM. When BaseURL's scheme, host or port
// differs from the stored row's, the stored credential is not merely unused —
// it is unusable, because the AAD binds it to the old host:port. Sending the
// stored key to a host the user just typed is the exfiltration bug in
// reference/security.md §1.6, and it is blocked in the handler, not here.
type TestRequest struct {
	// InstanceID is 0 for an instance that has not been saved yet.
	InstanceID int64
	Kind       string
	BaseURL    string
	URLBase    string

	// APIKey empty means "use the credential already stored for InstanceID".
	// The handler only permits that when base_url is unchanged.
	APIKey string
}

// TestResult is what the wizard shows. Message carries the upstream error
// VERBATIM — ARCHITECTURE.md §17.3 is explicit that "an error occurred" is not
// acceptable — after redaction, which removes credentials and nothing else.
type TestResult struct {
	OK bool

	// Reachable is true when the host answered at all, which is a different
	// question from whether the key was accepted.
	Reachable bool

	// KeyProvenValid is false when the instance answered but its
	// AuthenticationRequired is DisabledForLocalAddresses — a 200 then proves
	// reachability and nothing about the credential. Say "reachable", not
	// "credential verified".
	KeyProvenValid bool

	AppName    string
	AppVersion string
	APIVersion string

	Message string
	Action  string

	// There is deliberately NO TLSSPKIPin field.
	//
	// One existed, no ConnectionTester ever populated it, and the service-create
	// handler copied it into service_instance.tls_spki_pin — so the column was
	// always NULL and the TOFU path read as implemented while being dead code.
	// It is removed rather than left in place because a field nothing fills is
	// an invitation to trust it.
	//
	// TOFU enrolment is unimplemented on purpose; the reasoning, and what has to
	// land first, is on the si literal in handleCreateService. Pin ENFORCEMENT
	// is implemented and tested — see ssrf.Options.SPKIPin — so a pin seeded
	// into the column by hand is honoured.
}

// ConnectionTester runs the wizard's mandatory connection test.
type ConnectionTester interface {
	Test(ctx context.Context, req TestRequest) (TestResult, error)
}

// UpstreamWarning is one entry from an *Arr's own /health endpoint. Surfacing
// these is the single most valuable thing on the Services screen: today you
// visit five web UIs to notice one is unhappy (§17.3).
type UpstreamWarning struct {
	Source  string `json:"source,omitempty"`
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
	WikiURL string `json:"wiki_url,omitempty"`
}

// BlockedIndexer is one entry from Prowlarr's /api/v1/indexerstatus, which
// returns ONLY blocked indexers. This is what lets the Requests screen — where
// the release search lives (§17.5) — say what is missing instead of rendering
// an empty result set.
type BlockedIndexer struct {
	IndexerID         int32      `json:"indexer_id"`
	Name              string     `json:"name,omitempty"`
	DisabledTill      *time.Time `json:"disabled_till,omitempty"`
	MostRecentFailure *time.Time `json:"most_recent_failure,omitempty"`
}

// UpstreamHealth is one instance's last probe, taken OFF the render path by a
// background prober. The health handler reads snapshots; it never probes.
type UpstreamHealth struct {
	InstanceID int64
	ObservedAt time.Time

	AppVersion string
	APIVersion string

	Warnings        []UpstreamWarning
	BlockedIndexers []BlockedIndexer

	// BreakerState is the per-instance circuit breaker's own state, which is
	// what §17.3's State column renders.
	BreakerState   string
	BreakerRetryAt time.Time

	// Error is the verbatim upstream error text from the last failed probe,
	// already redacted. Empty when the probe succeeded.
	Error string
}

// HealthProbes hands out the last probe for an instance. Snapshot must not
// block on anything remote: it is called from a render path.
type HealthProbes interface {
	Snapshot(instanceID int64) (UpstreamHealth, bool)

	// ProbeNow refreshes one instance out of band. Handlers call it after a
	// successful connection test so the Services screen is not stale for a
	// whole probe interval; it must return promptly and do its work elsewhere.
	ProbeNow(instanceID int64)
}

// CatalogueImports re-runs the catalogue full import for one service instance —
// the Services screen's "Run full sync now" (ARCHITECTURE.md §17.3).
//
// It is a THIRD port rather than a method on HealthProbes because it is a
// different kind of thing: a probe is a read this package may take at will, and
// an import is a write that rewrites the catalogue rows an instance contributes.
// The implementation is in cmd/usarr, which is the only package that may hold an
// outbound client (§2.3 rule 1).
type CatalogueImports interface {
	// StartImport begins an import and RETURNS WITHOUT WAITING FOR IT. A
	// handler on a render path may call it; a handler that waited for it would
	// be principle 1's violation, since an import is minutes.
	//
	// Nil means an import is now running. The two states a caller must be able
	// to tell apart are the sentinels below, and both must be decided BEFORE
	// this returns — an implementation that decides "already running" inside
	// its own goroutine cannot report it.
	StartImport(instanceID int64) error
}

// ErrImportInProgress means this instance already has an import running, so a
// second one was refused. It is the double-click answer, and it is a refusal
// rather than a queued second run: two concurrent writers over one instance
// would interleave batches for no gain, and the running import is already doing
// the work the caller asked for.
var ErrImportInProgress = errors.New("httpapi: a catalogue import is already running for this instance")

// ErrNotCatalogueSource means the instance exists and is fine, and simply has no
// catalogue to import — a Prowlarr is an indexer, not a library (ADR-0041). It
// is distinct from ErrImportInProgress because the fixes are opposite: wait, vs
// stop asking this service for something it does not have.
var ErrNotCatalogueSource = errors.New("httpapi: this service instance carries no catalogue")
