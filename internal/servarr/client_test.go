package servarr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestPingIsAnonymous(t *testing.T) {
	c := newTestClient(t, "prowlarr_ping")
	got, err := c.Ping(ctx(t))
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got.Status != "OK" {
		t.Errorf("status = %q, want OK", got.Status)
	}
}

func TestSystemStatusHandshake(t *testing.T) {
	c := newTestClient(t, "prowlarr_system_status")
	got, err := c.SystemStatus(ctx(t))
	if err != nil {
		t.Fatalf("SystemStatus: %v", err)
	}
	if got.AppName != "Prowlarr" {
		t.Errorf("appName = %q", got.AppName)
	}
	if got.Version != "2.6.2.5052" {
		t.Errorf("version = %q", got.Version)
	}
	// X-Application-Version is on every /api/* response, so version detection is
	// free and must be picked up without a second call.
	if c.AppVersion() != "2.6.2.5052" {
		t.Errorf("AppVersion() = %q, want the X-Application-Version header value", c.AppVersion())
	}
	if !KeyProvenValid(got) {
		t.Errorf("with authentication=forms, a 200 does prove the key was accepted")
	}
}

// A 200 from system/status does NOT prove the API key is valid when the instance
// disables authentication for local addresses. Claiming otherwise in the health UI
// is a lie the user finds out about later, from a failed sync.
func TestSystemStatusLocalAuthDisabledDoesNotProveTheKey(t *testing.T) {
	c := newTestClient(t, "prowlarr_system_status_local_auth_disabled")
	got, err := c.SystemStatus(ctx(t))
	if err != nil {
		t.Fatalf("SystemStatus: %v", err)
	}
	if KeyProvenValid(got) {
		t.Fatalf("authentication=%q must not count as key verification", got.Authentication)
	}
}

// A bad key is 401 with an EMPTY BODY. Attempting to decode it would produce
// "unexpected end of JSON input" and bury the real cause.
func TestUnauthorizedHasNoBody(t *testing.T) {
	c := newTestClient(t, "prowlarr_system_status_unauthorized")
	_, err := c.SystemStatus(ctx(t))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if errors.Is(err, ErrDecode) {
		t.Fatalf("the empty 401 body was parsed as JSON: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err is not *APIError: %T", err)
	}
	if apiErr.Status != 401 {
		t.Errorf("status = %d", apiErr.Status)
	}
	assertNoCredential(t, "401 error string", err.Error())

	// A 4xx says nothing about instance health, so it must not push the breaker
	// toward open: a user with a typo'd key would otherwise find the instance
	// circuit-broken and be told it is "down".
	if got := c.Breaker().State(); got != BreakerClosed {
		t.Errorf("breaker = %s after a 401, want closed", got)
	}
}

func TestAPIVersionCheck(t *testing.T) {
	c := newTestClient(t, "prowlarr_api_version")
	got, err := c.CheckAPIVersion(ctx(t))
	if err != nil {
		t.Fatalf("CheckAPIVersion: %v", err)
	}
	if got.Current != "v1" {
		t.Errorf("current = %q", got.Current)
	}
}

func TestIndexersReturnsDisabledOnesToo(t *testing.T) {
	c := newTestClient(t, "prowlarr_indexers")
	ixs, err := c.Indexers(ctx(t))
	if err != nil {
		t.Fatalf("Indexers: %v", err)
	}
	if len(ixs) != 4 {
		t.Fatalf("got %d indexers, want 4", len(ixs))
	}
	var disabled int
	for _, ix := range ixs {
		if !ix.Enable {
			disabled++
		}
	}
	if disabled == 0 {
		t.Error("the endpoint has no filter parameter; a disabled indexer must come back")
	}

	// fields[] carries the indexer's own passkey. SanitizeIndexer is the boundary.
	if len(ixs[0].Fields) == 0 {
		t.Fatal("fixture should carry a fields array")
	}
	if got := SanitizeIndexer(ixs[0]); got.Fields != nil {
		t.Error("SanitizeIndexer left fields[] in place")
	}

	// capabilities.limitsMax caps what UsArr may ask for.
	if got := EffectiveLimit(ixs[0].Capabilities, 500); got != 100 {
		t.Errorf("EffectiveLimit(want 500, limitsMax 100) = %d, want 100", got)
	}
	if got := EffectiveLimit(ixs[0].Capabilities, 0); got != 50 {
		t.Errorf("EffectiveLimit(unspecified) = %d, want limitsDefault 50", got)
	}
}

func TestIndexerStatusReturnsOnlyBlocked(t *testing.T) {
	c := newTestClient(t, "prowlarr_indexerstatus_blocked")
	sts, err := c.IndexerStatus(ctx(t))
	if err != nil {
		t.Fatalf("IndexerStatus: %v", err)
	}
	if len(sts) != 1 {
		t.Fatalf("got %d statuses, want 1 (only blocked indexers are listed)", len(sts))
	}
	if sts[0].IndexerID != 4 {
		t.Errorf("indexerId = %d", sts[0].IndexerID)
	}
	if sts[0].DisabledTill == nil {
		t.Fatal("disabledTill should be present")
	}
}

func TestDownloadClients(t *testing.T) {
	c := newTestClient(t, "prowlarr_downloadclients")
	dcs, err := c.DownloadClients(ctx(t))
	if err != nil {
		t.Fatalf("DownloadClients: %v", err)
	}
	if len(dcs) != 2 {
		t.Fatalf("got %d clients", len(dcs))
	}
	if got := SanitizeDownloadClient(dcs[0]); got.Fields != nil {
		t.Error("SanitizeDownloadClient left fields[] in place")
	}
}

// Prowlarr answers 200 with [] when every indexer failed, when all are
// rate-limited, when no enabled indexer supports the categories, and when there
// genuinely is nothing. The client cannot resolve that; it must not pretend to.
func TestSearchEmptyIsAmbiguousNotAnError(t *testing.T) {
	c := newTestClient(t, "prowlarr_search_empty")
	rels, err := c.Search(ctx(t), SearchRequest{Text: "dune", Limit: Int32(100)})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(rels) != 0 {
		t.Fatalf("got %d releases, want 0", len(rels))
	}
	// The resolution lives one layer up, in internal/releases, by correlating with
	// GET /api/v1/indexerstatus. This test exists to pin that the client returns a
	// plain empty slice and no error, so nobody "fixes" it into a sentinel.
}

func TestSearchDecodesOmittedNullsAsNilPointers(t *testing.T) {
	c := newTestClient(t, "prowlarr_search_results")
	rels, err := c.Search(ctx(t), SearchRequest{Text: "dune", Limit: Int32(100)})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("got %d releases, want 2", len(rels))
	}

	torrent, usenet := rels[0], rels[1]

	if torrent.Seeders == nil || *torrent.Seeders != 148 {
		t.Errorf("torrent seeders = %v, want 148", torrent.Seeders)
	}
	if torrent.InfoHash == nil || *torrent.InfoHash == "" {
		t.Error("torrent infoHash should be present")
	}

	// The usenet result OMITS seeders/leechers/files/grabs/infoHash/magnetUrl
	// entirely, because WhenWritingNull drops nulls. Decoding them as 0 would make
	// "usenet, no seed concept" indistinguishable from "torrent with zero seeds".
	if usenet.Seeders != nil {
		t.Errorf("usenet seeders = %v, want nil (field absent, not zero)", *usenet.Seeders)
	}
	if usenet.Leechers != nil {
		t.Errorf("usenet leechers = %v, want nil", *usenet.Leechers)
	}
	if usenet.Files != nil || usenet.Grabs != nil {
		t.Error("usenet files/grabs should be nil")
	}
	if usenet.InfoHash != nil || usenet.MagnetURL != nil {
		t.Error("usenet infoHash/magnetUrl should be nil")
	}

	// imdbId is NUMERIC on Prowlarr's ReleaseResource, and 0 means absent.
	if torrent.IMDbID != 15239678 {
		t.Errorf("imdbId = %d", torrent.IMDbID)
	}
	if usenet.IMDbID != 0 {
		t.Errorf("usenet imdbId = %d, want 0 (absent)", usenet.IMDbID)
	}

	// Categories are raw and are never collapsed.
	if got := torrent.CategoryIDs(); len(got) != 2 || got[0] != 2000 || got[1] != 2045 {
		t.Errorf("categories = %v, want [2000 2045]", got)
	}
	// publishDate is second-precision UTC and parses as RFC3339.
	if torrent.PublishDate.IsZero() {
		t.Error("publishDate did not parse")
	}
}

// The single most dangerous field pair in this integration.
func TestSanitizeReleaseDropsTheEmbeddedAPIKey(t *testing.T) {
	c := newTestClient(t, "prowlarr_search_results")
	rels, err := c.Search(ctx(t), SearchRequest{Text: "dune", Limit: Int32(100)})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	rel := rels[0]
	if rel.DownloadURL == nil || !strings.Contains(*rel.DownloadURL, "apikey=") {
		t.Fatal("fixture must carry a proxy download URL with an apikey parameter")
	}

	safe := SanitizeRelease(rel)
	if safe.DownloadURL != nil {
		t.Error("SanitizeRelease left downloadUrl in place")
	}
	if safe.MagnetURL != nil {
		t.Error("SanitizeRelease left magnetUrl in place")
	}
	// Everything a client actually needs survives.
	if safe.GUID != rel.GUID || safe.Title != rel.Title || safe.IndexerID != rel.IndexerID {
		t.Error("SanitizeRelease dropped a field a client needs")
	}

	// magnetUrl is an http(s) proxy link, NOT a magnet: URI, so it cannot be given
	// to a torrent client. Pin that so nobody wires it up as one.
	if rel.MagnetURL == nil || strings.HasPrefix(*rel.MagnetURL, "magnet:") {
		t.Error("magnetUrl fixture should be an http proxy link, not a magnet URI")
	}

	// GrabBody is what actually goes back over the wire, and it carries neither.
	//
	// Asserted on the MARSHALLED BYTES, not on the struct: the bug this pins was
	// invisible at the struct level (a zeroed field looks like "not set") and only
	// existed once encoding/json turned it into `"protocol":""`.
	body := rel.GrabBody()
	if body.GUID != rel.GUID || body.IndexerID != rel.IndexerID {
		t.Error("GrabBody dropped a field grab validation requires")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshalling the grab body: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("re-decoding the grab body: %v", err)
	}
	// Prowlarr reads exactly these three. Anything else on the wire is at best
	// noise and at worst a value its model binder rejects.
	allowed := map[string]bool{"guid": true, "indexerId": true, "downloadClientId": true}
	for k := range got {
		if !allowed[k] {
			t.Errorf("the grab body sends %q; Prowlarr reads only guid, indexerId and downloadClientId (got %s)", k, raw)
		}
	}
}

// TestSanitizeReleaseRedactsTheIndexerSuppliedURLs covers the OTHER credential
// in a search result.
//
// infoUrl, commentUrl and posterUrl come from the INDEXER, not from Prowlarr,
// and a private tracker puts the user's personal passkey in them. They are
// redacted rather than dropped: the tracker and the release path are what makes
// a stored candidate and a permanent provenance row readable by a human, and
// only the secret has to go.
func TestSanitizeReleaseRedactsTheIndexerSuppliedURLs(t *testing.T) {
	const passkey = "d41d8cd98f00b204e9800998ecf8427e"
	rel := ReleaseResource{
		GUID:       "g1",
		InfoURL:    "https://tracker.test/details/1?passkey=" + passkey,
		CommentURL: "https://tracker.test/comments/1?torrent_pass=" + passkey,
		PosterURL:  "https://tracker.test/cover/1.jpg?authkey=" + passkey,
	}

	safe := SanitizeRelease(rel)
	for name, got := range map[string]string{
		"infoUrl":    safe.InfoURL,
		"commentUrl": safe.CommentURL,
		"posterUrl":  safe.PosterURL,
	} {
		if strings.Contains(got, passkey) {
			t.Errorf("SanitizeRelease left the tracker passkey in %s: %s", name, got)
		}
		if !strings.Contains(got, "REDACTED") {
			t.Errorf("SanitizeRelease did not redact %s through the shared deny-list: %s", name, got)
		}
		if !strings.Contains(got, "tracker.test") {
			t.Errorf("SanitizeRelease dropped %s entirely; it must be redacted, not removed: %s", name, got)
		}
	}

	// An empty field stays empty rather than becoming a placeholder URL.
	if got := SanitizeRelease(ReleaseResource{}); got.InfoURL != "" || got.CommentURL != "" || got.PosterURL != "" {
		t.Errorf("SanitizeRelease invented a URL for an absent field: %+v", got)
	}
}

func TestGrabSucceeds(t *testing.T) {
	c := newTestClient(t, "prowlarr_grab_ok")
	rel := ReleaseResource{
		GUID:             "https://fake-torrents.test/details/1001",
		IndexerID:        1,
		DownloadClientID: Int32(2),
	}
	got, err := c.Grab(ctx(t), rel)
	if err != nil {
		t.Fatalf("Grab: %v", err)
	}
	// There is no download id and no queue id — the echoed resource IS the receipt.
	if got.GUID != rel.GUID {
		t.Errorf("guid = %q", got.GUID)
	}
}

func TestGrabRejectsInvalidBodyBeforeSending(t *testing.T) {
	// No cassette interaction is consumed: these must fail client-side.
	c := newTestClient(t, "prowlarr_grab_ok")
	if _, err := c.Grab(ctx(t), ReleaseResource{GUID: "x"}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("indexerId 0: err = %v, want ErrInvalidRequest", err)
	}
	if _, err := c.Grab(ctx(t), ReleaseResource{IndexerID: 1}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("empty guid: err = %v, want ErrInvalidRequest", err)
	}
	// Consume the interaction so the recorder does not complain about it.
	if _, err := c.Grab(ctx(t), ReleaseResource{GUID: "https://fake-torrents.test/details/1001", IndexerID: 1, DownloadClientID: Int32(2)}); err != nil {
		t.Fatalf("valid grab: %v", err)
	}
}

func TestGrabCacheMissIs404(t *testing.T) {
	c := newTestClient(t, "prowlarr_grab_cache_miss")
	_, err := c.Grab(ctx(t), ReleaseResource{
		GUID: "https://fake-torrents.test/details/1001", IndexerID: 1, DownloadClientID: Int32(2),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("not an *APIError: %T", err)
	}
	if !strings.Contains(apiErr.Message, "try searching again") {
		t.Errorf("message = %q", apiErr.Message)
	}
	// The stack trace is captured but must never appear in the user-facing string.
	if apiErr.Description == "" {
		t.Error("description (the stack trace) should be captured for debug logging")
	}
	if strings.Contains(err.Error(), "NzbDrone.Core") {
		t.Error("Error() leaked the .NET stack trace")
	}
}

func TestGrabNoDownloadClientIsTypedNotA500(t *testing.T) {
	c := newTestClient(t, "prowlarr_grab_no_download_client")
	_, err := c.Grab(ctx(t), ReleaseResource{
		GUID: "https://fake-torrents.test/details/1001", IndexerID: 1, DownloadClientID: Int32(2),
	})
	if !errors.Is(err, ErrNoDownloadClient) {
		t.Fatalf("err = %v, want ErrNoDownloadClient", err)
	}
	var apiErr *APIError
	errors.As(err, &apiErr)
	if apiErr.Status != 500 {
		t.Errorf("status = %d, want 500", apiErr.Status)
	}
	if apiErr.Description != "" {
		t.Error("the stack trace should be dropped once the cause is identified")
	}
	if strings.Contains(err.Error(), "DownloadClientUnavailableException") {
		t.Error("Error() leaked the .NET exception type")
	}
}

// Envelope shape (2): a bare JSON array. json.Unmarshal into a struct fails on
// this outright, so the decoder must sniff the first byte.
func TestGrabValidationBareArray(t *testing.T) {
	c := newTestClient(t, "prowlarr_grab_validation_array")
	_, err := c.Grab(ctx(t), ReleaseResource{GUID: "x", IndexerID: 1})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	var apiErr *APIError
	errors.As(err, &apiErr)
	if len(apiErr.Validation) != 2 {
		t.Fatalf("got %d validation failures, want 2", len(apiErr.Validation))
	}
	if apiErr.Validation[0].PropertyName != "IndexerId" {
		t.Errorf("propertyName = %q", apiErr.Validation[0].PropertyName)
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("Error() should surface every failure: %q", err.Error())
	}
}

// Envelope shape (3): ASP.NET ValidationProblemDetails, from the model binder.
func TestGrabValidationProblemDetails(t *testing.T) {
	c := newTestClient(t, "prowlarr_grab_validation_problem_details")
	_, err := c.Grab(ctx(t), ReleaseResource{GUID: "x", IndexerID: 1})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	var apiErr *APIError
	errors.As(err, &apiErr)
	if !strings.Contains(apiErr.Message, "validation errors occurred") {
		t.Errorf("message = %q", apiErr.Message)
	}
	if len(apiErr.Validation) != 1 || apiErr.Validation[0].PropertyName != "$.indexerId" {
		t.Errorf("validation = %+v", apiErr.Validation)
	}
}

func TestParseErrorBodyFallsBackToRawBody(t *testing.T) {
	// A reverse proxy in front of Prowlarr answers with HTML, not JSON.
	e := parseErrorBody("Search", "GET", "/api/v1/search", 502, []byte("<html><body>502 Bad Gateway</body></html>"))
	if !errors.Is(e, ErrServer) {
		t.Fatalf("err = %v, want ErrServer", e)
	}
	if !strings.Contains(e.Message, "502 Bad Gateway") {
		t.Errorf("message = %q", e.Message)
	}
}

func TestParseErrorBodyTruncatesGiantBodies(t *testing.T) {
	e := parseErrorBody("Search", "GET", "/api/v1/search", 502, []byte(strings.Repeat("x", 10000)))
	if len(e.Message) > 600 {
		t.Errorf("message length %d; an unrecognised body must be bounded", len(e.Message))
	}
}

func TestNewProwlarrRequiresAnInjectedClient(t *testing.T) {
	if _, err := NewProwlarr(Options{BaseURL: testBaseURL, APIKey: testAPIKey}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v; there must be no fallback to http.DefaultClient", err)
	}
}

func TestBaseURLStripsCredentials(t *testing.T) {
	c := newTestClient(t, "prowlarr_ping")
	// The base URL is what gets logged and stored; it must never carry userinfo or
	// a query string.
	if strings.Contains(c.BaseURL(), "@") || strings.Contains(c.BaseURL(), "?") {
		t.Errorf("BaseURL() = %q", c.BaseURL())
	}
	if _, err := c.Ping(ctx(t)); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
