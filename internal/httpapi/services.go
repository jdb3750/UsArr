package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jdb3750/UsArr/internal/crypto"
	"github.com/jdb3750/UsArr/internal/store"
)

// Service-instance CRUD, the wizard's connection test, and the Services screen.
//
// Three rules run through the whole file and none of them are the UI's job:
//
//  1. The API key is WRITE-ONLY. It is accepted on create and update and is not
//     in any response shape here. There is no "reveal" endpoint.
//  2. Changing base_url's scheme, host or port INVALIDATES the stored
//     credential and requires re-entry (reference/security.md §1.6). Enforced
//     server-side. The attack is: point base_url at a listener you control,
//     click Test, read the X-Api-Key off your own log. A UI that merely masks
//     the key is cosmetic against it.
//  3. Every read takes an access scope in its signature.

// serviceKinds is what v0.1 can actually talk to. Rejecting an unknown kind at
// the API boundary beats storing a row nothing can ever open.
var serviceKinds = map[string]string{
	"prowlarr": "indexer",
}

type serviceResponse struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	URLBase string `json:"url_base,omitempty"`

	APIVersion string `json:"api_version,omitempty"`
	Enabled    bool   `json:"enabled"`
	Priority   int64  `json:"priority"`
	ManagedBy  string `json:"managed_by"`
	VerifyTLS  bool   `json:"verify_tls"`

	// HasCredential says a credential is stored. The credential itself is never
	// rendered — not in full and not masked, because producing "••••••1a2b"
	// means decrypting a full-admin key on a render path to show four
	// characters of it.
	HasCredential bool `json:"has_credential"`

	HealthState  string `json:"health_state"`
	BreakerState string `json:"breaker_state"`
}

func toServiceResponse(si store.ServiceInstance) serviceResponse {
	return serviceResponse{
		ID:            si.ID,
		Kind:          si.Kind,
		Role:          si.Role,
		Name:          si.Name,
		BaseURL:       redactURLField(si.BaseURL),
		URLBase:       si.URLBase,
		APIVersion:    si.APIVersion,
		Enabled:       si.Enabled,
		Priority:      si.Priority,
		ManagedBy:     si.ManagedBy,
		VerifyTLS:     si.VerifyTLS,
		HasCredential: len(si.APIKeyEnc) >= crypto.MinEnvelopeLen,
		HealthState:   si.HealthState,
		BreakerState:  si.BreakerState,
	}
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	instances, err := s.store.ListServiceInstances(r.Context(), storeScope(a))
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"the configured services could not be read").wrapping(err)
	}
	out := make([]serviceResponse, 0, len(instances))
	for _, si := range instances {
		out = append(out, toServiceResponse(si))
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": out})
	return nil
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	id, err := pathInt64(r, "id")
	if err != nil {
		return err
	}
	si, err := s.store.GetServiceInstance(r.Context(), storeScope(a), id)
	if err != nil {
		return notFoundOr(err, "service")
	}
	writeJSON(w, http.StatusOK, toServiceResponse(si))
	return nil
}

type createServiceRequest struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	URLBase  string `json:"url_base"`
	APIKey   string `json:"api_key"`
	Enabled  *bool  `json:"enabled"`
	Priority *int64 `json:"priority"`
}

func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) error {
	var req createServiceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	role, ok := serviceKinds[kind]
	if !ok {
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			fmt.Sprintf("%q is not a service kind UsArr can talk to yet; v0.1 supports: %s",
				req.Kind, knownKinds())).withAction("Choose a supported service kind")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return errStatus(http.StatusBadRequest, CodeBadRequest, "name must not be empty")
	}
	baseURL, err := normalizeBaseURL(req.BaseURL)
	if err != nil {
		return err
	}
	urlBase, err := normalizeServiceURLBase(req.URLBase)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"an API key is required: UsArr cannot talk to an *Arr without one").
			withAction("Paste the key from the service's Settings → General")
	}

	// The wizard's connection test is MANDATORY (ARCHITECTURE.md §11.3, §17.7),
	// and it is enforced here rather than only in the UI: a client that skips
	// the test would otherwise store a credential against a host nobody has
	// established is the service it claims to be.
	result, err := s.runTest(r, TestRequest{
		Kind: kind, BaseURL: baseURL, URLBase: urlBase, APIKey: req.APIKey,
	})
	if err != nil {
		return err
	}
	if !result.OK {
		s.audit(r, "service.credential.added", "service_instance", 0, "fail",
			fmt.Sprintf(`{"name":%q,"reason":"connection test failed"}`, name))
		return errStatus(http.StatusBadGateway, CodeConnectionTestFailed, result.Message).
			withAction(defaultString(result.Action, "Test connection"))
	}

	// TLSSPKIPin is deliberately NOT set here, and the column is left NULL.
	//
	// TOFU pinning is UNIMPLEMENTED, not half-implemented. This line used to
	// copy a TestResult field that nothing ever populated, so the pin was always
	// NULL anyway while the code read as though enrolment worked. Enrolment is
	// the easy half; the flow providers.md §4 and CONFIGURATION.md §7.1 describe
	// is "show the fingerprint and have the user accept it", and none of that
	// UI exists. Recording a pin silently, here, would be worse than recording
	// nothing:
	//
	//  1. A pin REPLACES chain verification — internal/ssrf sets
	//     InsecureSkipVerify on the pinned path, by design, because the
	//     certificate it exists for is a self-signed homelab one. Pinning every
	//     instance that passes a connection test would therefore DOWNGRADE an
	//     instance behind a publicly-trusted certificate, dropping hostname,
	//     expiry and revocation checking in exchange for nothing.
	//  2. There is no way back. store.ServiceInstanceUpdate carries no pin
	//     field, so nothing in the API can clear or re-accept one. A routine
	//     key rotation — an ACME renewal, a container regenerating its
	//     self-signed certificate on restart — would lock the instance out
	//     permanently with ErrPinMismatch and no supported recovery short of
	//     hand-editing SQLite.
	//
	// The enforcement half is real and tested (ssrf.Options.SPKIPin,
	// TestSPKIPinIsEnforcedOnAResumedSession): a pin that IS in the column is
	// checked on every handshake, including resumed ones. What is missing is the
	// enrolment UI, and that is what has to land before anything writes here.
	si := store.ServiceInstance{
		Kind:       kind,
		Role:       role,
		Name:       name,
		BaseURL:    baseURL,
		URLBase:    urlBase,
		APIVersion: result.APIVersion,
		VerifyTLS:  true,
		Enabled:    boolOr(req.Enabled, true),
		Priority:   int64Or(req.Priority, 0),
		ManagedBy:  "ui",
	}

	// CreateServiceInstanceSealed exists because of the ordering problem: the
	// AAD binds the ciphertext to the row's primary key, which does not exist
	// until the insert. Both halves are one transaction, so a row never exists
	// carrying an unsealed credential.
	id, err := s.store.CreateServiceInstanceSealed(r.Context(), si,
		func(id int64) ([]byte, uint32, error) {
			return s.sealKey(id, baseURL, req.APIKey)
		})
	if err != nil {
		return errStatus(http.StatusConflict, CodeConflict,
			"that service could not be saved: "+redactText(err.Error())).wrapping(err)
	}

	// The VALUE is never logged; the name and the host are (security.md §6).
	s.audit(r, "service.credential.added", "service_instance", id, "ok",
		fmt.Sprintf(`{"kind":%q,"name":%q,"host":%q}`, kind, name, hostOf(baseURL)))

	if s.cfg.Probes != nil {
		s.cfg.Probes.ProbeNow(id)
	}

	si.ID = id
	si.APIKeyEnc = make([]byte, crypto.MinEnvelopeLen) // sealed above; length is all the DTO reads
	writeJSON(w, http.StatusCreated, toServiceResponse(si))
	return nil
}

type updateServiceRequest struct {
	Name     *string `json:"name"`
	BaseURL  *string `json:"base_url"`
	URLBase  *string `json:"url_base"`
	APIKey   *string `json:"api_key"`
	Enabled  *bool   `json:"enabled"`
	Priority *int64  `json:"priority"`
}

func (s *Server) handleUpdateService(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	id, err := pathInt64(r, "id")
	if err != nil {
		return err
	}
	var req updateServiceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	existing, err := s.store.GetServiceInstance(r.Context(), storeScope(a), id)
	if err != nil {
		return notFoundOr(err, "service")
	}

	update := store.ServiceInstanceUpdate{
		Name:     trimmedPtr(req.Name),
		Enabled:  req.Enabled,
		Priority: req.Priority,
	}

	// url_base is validated HERE and not only on create, because this is the path
	// that used to save a broken one in silence: with no api_key in the body no
	// connection test runs, so `prowlarr` instead of `/prowlarr` was stored, later
	// concatenated onto base_url as http://host:9696prowlarr, and surfaced only
	// when the background prober reported the instance down.
	if req.URLBase != nil {
		normalized, uerr := normalizeServiceURLBase(*req.URLBase)
		if uerr != nil {
			return uerr
		}
		update.URLBase = &normalized
	}

	effectiveBaseURL := existing.BaseURL
	hostChanged := false
	if req.BaseURL != nil {
		newBase, nerr := normalizeBaseURL(*req.BaseURL)
		if nerr != nil {
			return nerr
		}
		// NormalizeOrigin, not NormalizeHostPort: this comparison decides whether
		// re-entry is required, and it MUST use the same normalisation the AAD is
		// built from. The AAD binds scheme://host:port, so an https->http edit on
		// the same port changes the AAD. Comparing only host:port here would let
		// that edit through without re-entry and leave a credential that can
		// never be opened again. security.md §1.6.
		oldHost, herr := crypto.NormalizeOrigin(existing.BaseURL)
		if herr != nil {
			return errStatus(http.StatusInternalServerError, CodeInternal,
				"the stored base URL cannot be parsed").wrapping(herr)
		}
		newHost, herr := crypto.NormalizeOrigin(newBase)
		if herr != nil {
			return errStatus(http.StatusBadRequest, CodeBadRequest, redactText(herr.Error()))
		}
		hostChanged = oldHost != newHost
		effectiveBaseURL = newBase
		update.BaseURL = &newBase
	}

	// The rule from reference/security.md §1.6, enforced server-side and not
	// left to the UI. Refusing is not pedantry: the stored ciphertext is bound
	// by AAD to the OLD host:port, so it could not be opened for the new host
	// even if UsArr were willing to try — and the failure mode of "try anyway"
	// is sending a full-admin key to whatever the user just typed.
	if hostChanged && strings.TrimSpace(derefString(req.APIKey)) == "" {
		return errStatus(http.StatusBadRequest, CodeCredentialReentryRequired,
			"changing this service's scheme, host or port invalidates the stored API key: "+
				"the key is bound to the old host and cannot be moved").
			withAction("Re-enter the API key")
	}

	var reseal func(int64) ([]byte, uint32, error)
	if key := strings.TrimSpace(derefString(req.APIKey)); key != "" {
		// A test against a MODIFIED base_url uses ONLY the key typed into the
		// form. That is what runTest gets here, and it is why req.APIKey is
		// passed rather than "" — "" would mean "use the stored one".
		result, terr := s.runTest(r, TestRequest{
			InstanceID: id,
			Kind:       existing.Kind,
			BaseURL:    effectiveBaseURL,
			URLBase:    derefOr(update.URLBase, existing.URLBase),
			APIKey:     key,
		})
		if terr != nil {
			return terr
		}
		if !result.OK {
			return errStatus(http.StatusBadGateway, CodeConnectionTestFailed, result.Message).
				withAction(defaultString(result.Action, "Test connection"))
		}
		reseal = func(rowID int64) ([]byte, uint32, error) {
			return s.sealKey(rowID, effectiveBaseURL, key)
		}
	}

	if err := s.store.UpdateServiceInstance(r.Context(), id, update, reseal); err != nil {
		return notFoundOr(err, "service")
	}

	if hostChanged {
		// result=warn, with old and new host and no credential
		// (reference/security.md §1.6).
		s.audit(r, "service.base_url.changed", "service_instance", id, "warn",
			fmt.Sprintf(`{"old_host":%q,"new_host":%q}`, hostOf(existing.BaseURL), hostOf(effectiveBaseURL)))
	}
	if reseal != nil {
		s.audit(r, "service.credential.changed", "service_instance", id, "ok",
			fmt.Sprintf(`{"host":%q}`, hostOf(effectiveBaseURL)))
	}
	if s.cfg.Probes != nil {
		s.cfg.Probes.ProbeNow(id)
	}

	updated, err := s.store.GetServiceInstance(r.Context(), storeScope(a), id)
	if err != nil {
		return notFoundOr(err, "service")
	}
	writeJSON(w, http.StatusOK, toServiceResponse(updated))
	return nil
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	id, err := pathInt64(r, "id")
	if err != nil {
		return err
	}
	existing, err := s.store.GetServiceInstance(r.Context(), storeScope(a), id)
	if err != nil {
		return notFoundOr(err, "service")
	}
	// Soft delete: the id stays burned and is never reused, so a stale
	// northbound reference resolves to "gone" rather than to some other
	// service.
	if err := s.store.SoftDeleteServiceInstance(r.Context(), id, s.now()); err != nil {
		return notFoundOr(err, "service")
	}
	s.audit(r, "service.credential.removed", "service_instance", id, "ok",
		fmt.Sprintf(`{"name":%q,"host":%q}`, existing.Name, hostOf(existing.BaseURL)))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ── connection test ─────────────────────────────────────────────────────────

type testRequestBody struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	URLBase string `json:"url_base"`
	APIKey  string `json:"api_key"`
}

type testResponse struct {
	OK             bool   `json:"ok"`
	Reachable      bool   `json:"reachable"`
	KeyProvenValid bool   `json:"key_proven_valid"`
	AppName        string `json:"app_name,omitempty"`
	AppVersion     string `json:"app_version,omitempty"`
	APIVersion     string `json:"api_version,omitempty"`
	Message        string `json:"message"`
	Action         string `json:"action,omitempty"`
}

// handleTestUnsaved is the wizard's test before a row exists.
func (s *Server) handleTestUnsaved(w http.ResponseWriter, r *http.Request) error {
	var req testRequestBody
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	baseURL, err := normalizeBaseURL(req.BaseURL)
	if err != nil {
		return err
	}
	urlBase, err := normalizeServiceURLBase(req.URLBase)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return errStatus(http.StatusBadRequest, CodeBadRequest,
			"an API key is required to test a service that has not been saved yet")
	}
	result, err := s.runTest(r, TestRequest{
		Kind: strings.ToLower(strings.TrimSpace(req.Kind)), BaseURL: baseURL,
		URLBase: urlBase, APIKey: req.APIKey,
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toTestResponse(result))
	return nil
}

// handleTestService tests a saved instance, optionally against edited values.
//
// This is the endpoint reference/security.md §1.6 is about. If the body names a
// base_url whose scheme, host or port differs from the stored one, an API key
// must be supplied in the same body and it is the ONLY key sent — the stored one
// is never used against a host the user has just typed.
func (s *Server) handleTestService(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	id, err := pathInt64(r, "id")
	if err != nil {
		return err
	}
	var req testRequestBody
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}
	existing, err := s.store.GetServiceInstance(r.Context(), storeScope(a), id)
	if err != nil {
		return notFoundOr(err, "service")
	}

	baseURL := existing.BaseURL
	if strings.TrimSpace(req.BaseURL) != "" {
		if baseURL, err = normalizeBaseURL(req.BaseURL); err != nil {
			return err
		}
	}
	// Validated even though an empty value here means "keep the stored sub-path":
	// this endpoint is how the screen checks an edit BEFORE saving it, so a test
	// that quietly ignored a malformed sub-path would report a healthy connection
	// for a value the save is about to refuse.
	urlBase, err := normalizeServiceURLBase(req.URLBase)
	if err != nil {
		return err
	}
	// NormalizeOrigin: same reason as in the update path — the scheme is part of
	// what the stored credential is bound to, so a scheme change is a host change
	// for the purposes of the re-entry rule.
	oldHost, herr := crypto.NormalizeOrigin(existing.BaseURL)
	if herr != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"the stored base URL cannot be parsed").wrapping(herr)
	}
	newHost, herr := crypto.NormalizeOrigin(baseURL)
	if herr != nil {
		return errStatus(http.StatusBadRequest, CodeBadRequest, redactText(herr.Error()))
	}

	if oldHost != newHost && strings.TrimSpace(req.APIKey) == "" {
		return errStatus(http.StatusBadRequest, CodeCredentialReentryRequired,
			"a connection test against a changed host uses only the key typed into the form, "+
				"never the stored one").
			withAction("Re-enter the API key")
	}

	result, err := s.runTest(r, TestRequest{
		InstanceID: id,
		Kind:       existing.Kind,
		BaseURL:    baseURL,
		URLBase:    stringOr(urlBase, existing.URLBase),
		APIKey:     req.APIKey, // empty means "the stored one", allowed only when the host is unchanged
	})
	if err != nil {
		return err
	}
	s.audit(r, "service.test", "service_instance", id, resultWord(result.OK),
		fmt.Sprintf(`{"host":%q}`, newHost))
	if result.OK && s.cfg.Probes != nil {
		s.cfg.Probes.ProbeNow(id)
	}
	writeJSON(w, http.StatusOK, toTestResponse(result))
	return nil
}

// runTest is the ONE place this package makes something happen upstream. It is
// a user action with its own deadline, not a render path (see doc.go).
func (s *Server) runTest(r *http.Request, req TestRequest) (TestResult, error) {
	if s.cfg.Tester == nil {
		return TestResult{}, errStatus(http.StatusNotImplemented, CodeNotConfigured,
			"this build has no connection tester wired in")
	}
	ctx, cancel := context.WithTimeout(r.Context(), connectionTestTimeout)
	defer cancel()

	result, err := s.cfg.Tester.Test(ctx, req)
	if err != nil {
		// A tester error is a failed test, not a broken endpoint: the user
		// needs the verbatim reason, which is the entire point of the screen.
		return TestResult{Message: redactText(err.Error()), Action: "Test connection"}, nil
	}
	result.Message = redactText(result.Message)
	result.Action = redactText(result.Action)
	return result, nil
}

// connectionTestTimeout bounds the wizard's test. Long enough for a slow *Arr
// on a Pi, short enough that a wrong hostname fails while the user is still
// looking at the form.
const connectionTestTimeout = 15 * time.Second

func toTestResponse(t TestResult) testResponse {
	msg := t.Message
	if msg == "" {
		if t.OK {
			msg = "connected"
			if !t.KeyProvenValid {
				// Never claim a verification that did not happen: an instance
				// with AuthenticationRequired=DisabledForLocalAddresses answers
				// 200 to a request from a local address with NO key at all.
				msg = "reachable, but this instance accepts local requests without a key, " +
					"so the API key has not been verified"
			}
		} else {
			msg = "the service did not answer"
		}
	}
	return testResponse{
		OK:             t.OK,
		Reachable:      t.Reachable,
		KeyProvenValid: t.KeyProvenValid,
		AppName:        t.AppName,
		AppVersion:     t.AppVersion,
		APIVersion:     t.APIVersion,
		Message:        msg,
		Action:         t.Action,
	}
}

// ── the Services screen ─────────────────────────────────────────────────────

// Health states, from ARCHITECTURE.md §17.3.
const (
	stateHealthy  = "healthy"
	stateDegraded = "degraded"
	stateDown     = "down"
	stateReID     = "needs re-identification"
	stateUnknown  = "unknown"
)

type serviceHealthResponse struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Role    string `json:"role"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`

	State string `json:"state"`

	// Problem is the ACTUAL error text, verbatim — not "an error occurred"
	// (§17.3). Credentials are removed and nothing else is.
	Problem string `json:"problem,omitempty"`
	Action  string `json:"action,omitempty"`

	BreakerState        string     `json:"breaker_state"`
	BreakerRetryAt      *time.Time `json:"breaker_retry_at,omitempty"`
	ConsecutiveFailures int64      `json:"consecutive_failures"`
	LastOKAt            *time.Time `json:"last_ok_at,omitempty"`

	AppVersion string `json:"app_version,omitempty"`

	// Warnings are the *Arr's OWN health warnings. Surfacing them is the point
	// of the screen: today you visit five web UIs to notice one is unhappy.
	Warnings []UpstreamWarning `json:"warnings,omitempty"`

	// BlockedIndexers rides here as well as on the search stream, so a user who
	// gets thin results has one place that explains why.
	BlockedIndexers []BlockedIndexer `json:"blocked_indexers,omitempty"`

	ObservedAt *time.Time `json:"observed_at,omitempty"`

	// Stale is true when no probe has been taken yet, or the last one predates
	// the last recorded change. An honest "not measured yet" beats a green tick
	// that means nothing.
	Stale bool `json:"stale"`
}

// handleServicesHealth renders the Services screen ENTIRELY from SQLite plus the
// last probe snapshot. It makes no upstream call: the *Arr's own health warnings
// and its blocked indexers were fetched by the background prober, off this path.
func (s *Server) handleServicesHealth(w http.ResponseWriter, r *http.Request) error {
	a, _ := sessionFrom(r)
	instances, err := s.store.ListServiceInstances(r.Context(), storeScope(a))
	if err != nil {
		return errStatus(http.StatusInternalServerError, CodeInternal,
			"the configured services could not be read").wrapping(err)
	}

	out := make([]serviceHealthResponse, 0, len(instances))
	anyUnhealthy := false
	for _, si := range instances {
		row := s.healthRow(si)
		if row.State != stateHealthy {
			anyUnhealthy = true
		}
		out = append(out, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"services": out,
		// The global banner elsewhere in the app hangs off this (§17.3).
		"any_unhealthy": anyUnhealthy,
		// Principle 3: with nothing configured, say so and name the fix rather
		// than rendering an empty table that looks broken.
		"setup_required": len(instances) == 0,
	})
	return nil
}

func (s *Server) healthRow(si store.ServiceInstance) serviceHealthResponse {
	row := serviceHealthResponse{
		ID:                  si.ID,
		Name:                si.Name,
		Kind:                si.Kind,
		Role:                si.Role,
		BaseURL:             redactURLField(si.BaseURL),
		Enabled:             si.Enabled,
		BreakerState:        si.BreakerState,
		ConsecutiveFailures: si.ConsecutiveFailures,
		Problem:             redactText(si.LastError.String),
		Stale:               true,
	}
	if si.LastOKAt.Valid {
		if t, err := store.ParseTime(si.LastOKAt.String); err == nil {
			row.LastOKAt = &t
		}
	}
	if si.BreakerUntil.Valid {
		if t, err := store.ParseTime(si.BreakerUntil.String); err == nil {
			row.BreakerRetryAt = &t
		}
	}

	if s.cfg.Probes != nil {
		if snap, ok := s.cfg.Probes.Snapshot(si.ID); ok {
			row.Stale = false
			row.AppVersion = snap.AppVersion
			row.Warnings = snap.Warnings
			row.BlockedIndexers = snap.BlockedIndexers
			if snap.BreakerState != "" {
				row.BreakerState = snap.BreakerState
			}
			if !snap.BreakerRetryAt.IsZero() {
				retry := snap.BreakerRetryAt
				row.BreakerRetryAt = &retry
			}
			if snap.Error != "" {
				row.Problem = redactText(snap.Error)
			}
			observed := snap.ObservedAt
			row.ObservedAt = &observed
		}
	}

	row.State = healthState(si, row)
	row.Action = healthAction(row)
	return row
}

// healthState derives the four states in §17.3 from the breaker and the row.
//
// `needs re-identification` arrives through health_state because the sync
// engine writes it there (sync.md §4 guard 2): the instance's identity changed,
// sync is paused deliberately, and that must be loud rather than inferred.
func healthState(si store.ServiceInstance, row serviceHealthResponse) string {
	switch {
	case strings.EqualFold(si.HealthState, "needs_reidentification"),
		strings.EqualFold(si.HealthState, stateReID):
		return stateReID
	case !si.Enabled:
		return stateUnknown
	case strings.EqualFold(row.BreakerState, "open"), strings.EqualFold(si.HealthState, stateDown):
		return stateDown
	case strings.EqualFold(si.HealthState, stateDegraded):
		return stateDegraded
	case row.Problem != "":
		// Half-open with a stored error, or a probe that failed but has not
		// tripped the breaker yet. Degraded is the honest label: the catalogue
		// still renders, from the replica.
		return stateDegraded
	case si.ConsecutiveFailures > 0:
		return stateDegraded
	case row.LastOKAt != nil || strings.EqualFold(si.HealthState, "healthy"):
		return stateHealthy
	default:
		return stateUnknown
	}
}

// healthAction names the ONE button that fixes this state (§17.3: "The actions
// are the point"). Each mapping is the fix, not a generic retry.
func healthAction(row serviceHealthResponse) string {
	problem := strings.ToLower(row.Problem)
	switch {
	case row.State == stateReID:
		return "Re-link this instance"
	case strings.Contains(problem, "unauthorized"), strings.Contains(problem, "401"),
		strings.Contains(problem, "403"), strings.Contains(problem, "api key"):
		return "Update API key"
	case strings.Contains(problem, "certificate"), strings.Contains(problem, "tls"),
		strings.Contains(problem, "pin"):
		return "Review the TLS fingerprint"
	case row.State == stateDown:
		return "Test connection"
	case row.State == stateDegraded:
		return "Test connection"
	case row.State == stateUnknown && !row.Enabled:
		return "Enable this service"
	case row.State == stateUnknown:
		return "Test connection"
	}
	return ""
}

// ── helpers ─────────────────────────────────────────────────────────────────

// sealKey encrypts an API key for one row.
//
// The AAD binds the ciphertext to this row AND to this instance's normalised
// host:port, which is the cryptographic half of "changing base_url invalidates
// the credential" (reference/security.md §1.2).
func (s *Server) sealKey(rowID int64, baseURL, apiKey string) ([]byte, uint32, error) {
	aad, err := crypto.ServiceInstanceAAD(rowID, baseURL)
	if err != nil {
		return nil, 0, fmt.Errorf("building AAD for service_instance %d: %w", rowID, err)
	}
	envelope, err := s.keyring.Seal([]byte(apiKey), aad)
	if err != nil {
		return nil, 0, fmt.Errorf("sealing api key for service_instance %d: %w", rowID, err)
	}
	return envelope, s.keyring.PrimaryID(), nil
}

// normalizeBaseURL validates a user-supplied service URL and trims it to
// scheme://host[:port][/path].
func normalizeBaseURL(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", errStatus(http.StatusBadRequest, CodeBadRequest, "base_url must not be empty")
	}
	if _, err := crypto.NormalizeHostPort(v); err != nil {
		return "", errStatus(http.StatusBadRequest, CodeBadRequest,
			"that base URL is not usable: "+redactText(err.Error())).
			withAction("Use a full URL, e.g. http://prowlarr:9696")
	}
	return strings.TrimSuffix(v, "/"), nil
}

// normalizeServiceURLBase validates the reverse-proxy sub-path an instance is
// served under and returns it as "" (the root) or "/segment[/segment…]".
//
// It exists because cmd/usarr/services.go builds the upstream address by plain
// concatenation — BaseURL + URLBase — so `prowlarr` where `/prowlarr` was meant
// produces http://host:9696prowlarr. On create the mandatory connection test
// catches that loudly; on PATCH with no api_key no test runs, so it used to be
// stored in silence and only reported later, by the background prober, as an
// instance that is down for no visible reason.
//
// This is deliberately NOT config.normalizeURLBase, and the two are not wired
// together. That one governs USARR_URL_BASE — the prefix UsArr serves ITSELF
// under, read once at startup from an operator's environment, where a bad value
// fails the process at boot. This one governs a per-instance column describing
// where somebody else's *Arr lives, arriving from a browser on four endpoints,
// and it is stricter: it rejects a host, a query and a fragment as well. Sharing
// one function would mean either weakening this rule or silently changing the
// meaning of an environment variable the next time this one is tightened, and it
// would make internal/httpapi depend on internal/config for a string rule that
// fits on a screen. The duplication is the cheaper of the two couplings.
//
// A missing leading slash is REPAIRED rather than rejected: a bare path segment
// has exactly one possible meaning here, and repairing it is precisely what
// stops the concatenation above. Everything whose intent is ambiguous — a
// scheme, a host, a query, a fragment, a traversal — is rejected, because
// guessing which half of `http://host/prowlarr` the user meant would be
// inventing configuration on their behalf.
func normalizeServiceURLBase(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	reject := func(why string) error {
		return errStatus(http.StatusBadRequest, CodeInvalidURLBase,
			fmt.Sprintf("%q is not a sub-path: %s", raw, why)).
			withAction("Enter a path such as /prowlarr, or leave it blank")
	}
	switch {
	case strings.Contains(v, "://"):
		return "", reject("it is a full URL, and the address belongs in the base URL instead")
	case strings.HasPrefix(v, "//"):
		return "", reject("a leading // names a host, not a path")
	case strings.ContainsAny(v, "?#"):
		return "", reject("a sub-path carries no query string and no fragment")
	case strings.Contains(v, ":"):
		return "", reject("it names a host and port, and that belongs in the base URL instead")
	}

	// Trailing slashes are corrected rather than rejected for the same reason the
	// leading one is added: there is only one thing "/prowlarr/" can mean.
	v = "/" + strings.Trim(v, "/")
	if v == "/" {
		return "", nil
	}
	for _, seg := range strings.Split(v[1:], "/") {
		switch seg {
		case "":
			return "", reject("it has an empty path segment")
		case ".", "..":
			return "", reject(`it must not contain a "." or ".." segment`)
		}
		if i := strings.IndexFunc(seg, func(r rune) bool { return !isURLBaseRune(r) }); i >= 0 {
			return "", reject(fmt.Sprintf("%q is not allowed in a path; percent-encode it",
				string([]rune(seg[i:])[0])))
		}
	}
	return v, nil
}

// isURLBaseRune is RFC 3986's `pchar` minus ":" — which is rejected above as the
// tell of a host:port — and minus pct-encoding's own decoding, since "%" is
// simply passed through: this validates the shape of a path, it does not resolve
// one.
func isURLBaseRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("-._~%!$&'()*+,;=@", r)
}

// hostOf renders a base URL for the audit log's metadata. It uses the full
// origin so that a scheme-only edit — https://nas:443 -> http://nas:443, the
// downgrade the AAD now binds against — shows up as an actual change in
// {"old_host":…,"new_host":…} instead of two identical strings.
func hostOf(rawURL string) string {
	origin, err := crypto.NormalizeOrigin(rawURL)
	if err != nil {
		return ""
	}
	return origin
}

func notFoundOr(err error, what string) error {
	if errors.Is(err, store.ErrNotFound) {
		// "Does not exist" and "exists but is outside your scope" are
		// deliberately indistinguishable: the difference is an existence
		// oracle.
		return errStatus(http.StatusNotFound, CodeNotFound, "no such "+what)
	}
	return errStatus(http.StatusInternalServerError, CodeInternal,
		"the "+what+" could not be read").wrapping(err)
}

func knownKinds() string {
	names := make([]string, 0, len(serviceKinds))
	for k := range serviceKinds {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func int64Or(p *int64, def int64) int64 {
	if p == nil {
		return def
	}
	return *p
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func trimmedPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

func stringOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func resultWord(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}
