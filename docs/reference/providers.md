# Reference — The provider model

**Status:** designed, not implemented. **Scope:** Tier 0 (Sonarr, Radarr, Prowlarr) is **v0.1**;
Tier 1 manifests are **v0.3**. A WASM tier is **deferred, not rejected** — see
[`../FUTURE.md`](../FUTURE.md) §1.
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §11.

---

## 1. The requirement, and the registry seam

UsArr must work with (a) Prowlarr alone, (b) a full stack, and (c) **a service nobody has written Go
code for.** Requirement (c) is the interesting one, and the insight that makes it tractable is that
**90% of "add a new service" is not code, it is HTTP plumbing** — a base URL, an auth header, a few
endpoint paths, and a field mapping.

**The registry is the seam that keeps a third tier cheap.** Providers are not referenced by
concrete type anywhere in the sync engine; they are resolved from a registry:

```go
// A tier is just a factory. Tier 0 compiles in; Tier 1 reads YAML; a future Tier 2 would
// instantiate a WASM module. The sync engine never learns which produced a Provider.
type ProviderFactory interface {
    // Kinds this factory can construct, e.g. ["sonarr","radarr"] or ["lazylibrarian"].
    Kinds() []string
    New(ctx context.Context, inst Instance) (Provider, error)
}

type Registry struct{ factories []ProviderFactory }

func (r *Registry) Register(f ProviderFactory)                    { … }
func (r *Registry) For(ctx context.Context, inst Instance) (Provider, error) { … }
```

Two properties follow, and both are worth the near-zero cost of writing the interface this way now:

1. **Adding a tier is registering a factory.** A WASM host, a gRPC host, or an in-tree fork adapter
   are all `ProviderFactory` implementations. Nothing in `sync`, `store` or `api` changes.
2. **`RemoteItem` is the neutral wire type every tier produces.** The canonical mapper consumes it
   and knows nothing about which tier produced it.

Retrofitting either property later means touching every call site in the sync engine. Writing them
now costs one interface and one indirection.

---

## 2. The Go provider interface

```go
// Every provider implements this much.
type Provider interface {
    // Static identity: id, display name, supported media kinds, API versions.
    Descriptor(ctx context.Context) (Descriptor, error)

    // Probes the LIVE instance. Never inferred from `kind` — a Sonarr fork, an old version,
    // or a manifest-described service may differ. Cached on service_instance.capabilities
    // and refreshed on health probe.
    Capabilities(ctx context.Context, inst Instance) (Caps, error)

    // Liveness plus the upstream's OWN health warnings, surfaced into UsArr's UI.
    Health(ctx context.Context, inst Instance) (Health, error)
}

// A media kind is a (work kind, edition format) pair, because "can this instance take an
// audiobook?" is not answerable from a work kind alone — an audiobook is an edition of a
// book work (ARCHITECTURE §6.1). Format "" means "any format of this kind".
type MediaKind struct {
    Kind   string // movie|series|season|episode|artist|album|track|book|comic|game
    Format string // ""|print|ebook|audiobook|bluray|web|flac|…
}

type Caps struct {
    Search, LibrarySync, DeltaSync, Push, Add, Monitor,
    Delete, Queue, Grab, Images bool
    // NOTE: there is deliberately no `Stream` capability. UsArr's stream path proxies bytes
    // from its own surfaces and does not depend on a backend minting anything (gateway.md §4).
    MediaKinds []MediaKind
    APIVersion string        // "v1" | "v3" | "v5" — per app, NOT the app version
    RateLimit  *RateLimit
}

// ---- optional capability interfaces, type-asserted at runtime ----

type Searcher interface {
    Search(ctx context.Context, inst Instance, q SearchQuery) ([]SearchResult, error)
}

// SearchQuery carries free text AND, optionally, a work reference. The free-text form is
// what makes Search-and-Grab mode possible against Prowlarr, which has no library and
// therefore no WorkRef to hand in (ARCHITECTURE §8.5).
type SearchQuery struct {
    Text       string
    Work       *WorkRef    // nil in Search-and-Grab mode
    Categories []int       // raw Newznab category ints
    Kinds      []MediaKind
    Limit, Offset int
}

type LibrarySyncer interface {
    // cursor == "" means full import. STREAMS, so a 10k-item library never buffers.
    // Returns the next cursor, or "" when complete.
    SyncLibrary(ctx context.Context, inst Instance, cursor string,
        out chan<- RemoteItem) (next string, err error)
}

type DeltaSyncer interface {   // maps to /history/since
    SyncDelta(ctx context.Context, inst Instance, since time.Time) (
        []ChangeEvent, time.Time, error)
}

type Pusher interface {        // SignalR, webhooks, long-poll — v1.0
    Subscribe(ctx context.Context, inst Instance, out chan<- ChangeEvent) error
}

type Requester interface {     // the write path and requests
    Add(ctx context.Context, inst Instance, r AddRequest) (remoteID string, err error)
    SetMonitored(ctx context.Context, inst Instance, remoteID string, m bool) error
    Delete(ctx context.Context, inst Instance, remoteID string, deleteFiles bool) error
}

type Grabber interface {
    Releases(ctx context.Context, inst Instance, q SearchQuery) ([]Release, error)
    Grab(ctx context.Context, inst Instance, rel Release) error
}
```

`Grabber.Releases` takes a `SearchQuery`, not a `WorkRef`, for the same reason `Searcher` does:
Prowlarr-only mode has no work rows. `Grab` takes the whole `Release` because Prowlarr's grab is
`POST /api/v1/search` with the original `ReleaseResource` body echoed back (search.md §6).

**Capability discovery is what makes the UI honest.** "Just Prowlarr" is not a special case in the
code — Prowlarr advertises `Search` and `Health` and not `LibrarySync`, and the UI renders
accordingly. What the earlier design got wrong was calling that *sufficient*: capability discovery
correctly makes the UI hide things; it does not by itself make a product. That is what
Search-and-Grab mode (ARCHITECTURE §8.5) is for.

---

## 3. Tier 1: the declarative service manifest

A YAML file in `$USARR_CONFIG_DIR/providers/` describing a REST service well enough to sync it.

```yaml
apiVersion: usarr.dev/v1
kind: ServiceDefinition
metadata:
  name: lazylibrarian
  displayName: LazyLibrarian
  mediaKinds:
    - {kind: book, format: ebook}
    - {kind: book, format: audiobook}
  role: acquisition

auth:
  type: query_param          # header | query_param | basic | bearer
  name: apikey
  secretField: api_key

capabilities:                # probed at connect time, cached on service_instance
  search: true
  librarySync: true
  add: true
  monitor: false
  delete: false
  health: true
  deltaSync: false

# LazyLibrarian returns HTTP 200 WITH an error object, so success must be asserted on a
# body predicate, not on the status code. This is why `expect` takes both.
errorSignalling:
  successPath: $.Success        # optional; when present, must be truthy
  messagePath: $.Error.Message

endpoints:
  health:
    method: GET
    path: /api?cmd=getVersion
    expect: { status: 200 }

  librarySync:
    method: GET
    path: /api?cmd=getAllBooks
    pagination: { type: none }   # none | page_param | offset_limit | cursor_header
    itemsPath: $.data
    map:
      remoteId:  $.BookID
      title:     $.BookName
      year:      $.BookDate | year
      overview:  $.BookDesc
      posterUrl: $.BookImg
      kind:      book
      format:    ebook
      externalIds:
        goodreads_work: $.BookID
        isbn13:         $.BookIsbn

  search:
    method: GET
    path: /api?cmd=searchBook&name={{ .Query | urlquery }}
    itemsPath: $.data
    map:
      title: $.bookname
      externalIds: { isbn13: $.bookisbn }

  add:
    method: GET
    path: /api?cmd=addBook&id={{ .ExternalIds.goodreads_work | urlquery }}
    expect: { status: 200 }

sync:
  strategy: poll
  interval: 15m
  deltaSupported: false

rateLimit:
  requestsPerSecond: 2
  burst: 4
```

A user can add Komga, Kavita, Audiobookshelf, a Sonarr fork or a homebrew HTTP service in ~40 lines
and a reload.

### 3.1 A manifest is not a sandbox

> **Delete the phrase "fully sandboxed (no code runs)" wherever it appears.** A manifest is a
> **server-side HTTP request generator that runs with the instance's stored credential.** Treating
> it as inert data is the most dangerous possible reading, because it drives an implementation that
> does no validation at all.

Four normative properties:

**1. URL construction is confined by construction, not by intention.**

```go
// REQUIRED shape. url.ResolveReference is FORBIDDEN here.
func buildURL(base *url.URL, renderedPath string, q url.Values) (*url.URL, error) {
    clean := path.Clean(path.Join(base.EscapedPath(), renderedPath))
    if !strings.HasPrefix(clean, path.Clean(base.EscapedPath())) {
        return nil, ErrManifestPathEscape
    }
    return &url.URL{
        Scheme:   base.Scheme,   // from base_url ONLY
        Host:     base.Host,     // from base_url ONLY
        Path:     clean,
        RawQuery: q.Encode(),    // rendered into query values, never into a URL string
    }, nil
}
```

`url.Parse(base).ResolveReference(rendered)` is the naive implementation and it is exploitable:
`ResolveReference("//evil.example/x")` against `http://sonarr:8989` yields `http://evil.example/x`,
and because `auth:` attaches the secret to *whatever URL is produced*, the instance credential goes
with it. **Rejected at load time:** any rendered path template that can contain `://`, a leading
`//` or `/\`, `..`, or a userinfo `@`. Render into path and query **segments** separately and
percent-encode both; never render into a whole URL string.

**2. Every interpolation must carry an escaping filter.** A manifest with a bare `{{ }}` is rejected
at load. Without it, a user's search box becomes parameter injection against the backend — on
LazyLibrarian's query-string RPC, which is this manifest's own example, that is
`&cmd=deleteBook&id=…`.

**3. A manifest may never write a strong identity.** `map.externalIds` output is capped at
`confidence < 1.0`, so it can never satisfy `ux_extid_work_strong` and can never drive a tier-1
merge. Without the cap, a manifest emitting a constant `imdb: tt0111161` for every item collapses
the user's whole library into one work (schema.md §4).

**4. Distribution is reviewed, not viral.** The bundled manifests ship **embedded in the repo,
reviewed**. There is **no "share it as a gist" story** — a manifest chooses which endpoint on the
configured host receives the credential and in what form, so a gist manifest for "Komga" whose
`health` endpoint is `/api/v1/…?apikey={{secret}}` against a base URL the author talks the user into
setting is a straight credential disclosure. A manifest found in `providers/` is displayed in the UI
with its endpoint list, its auth placement and its target host, and requires **explicit admin
confirmation before it is bound to a credential**.

### 3.2 Stated scope — what a manifest cannot express

> A manifest describes a **read-mostly JSON-over-HTTP service with stateless auth**: header key,
> query key, bearer, or basic.

**Out of scope, requiring a compiled-in Tier 0 provider:**

| Axis | Out of scope | Named services |
|---|---|---|
| Session establishment | login → cookie; no login endpoint construct exists | qBittorrent (`SID`), Deluge |
| Challenge/retry | 409-challenge handshake; no retry construct exists | Transmission (`X-Transmission-Session-Id`) |
| Request bodies | JSON-RPC method envelopes; no `request.body` construct exists | NZBGet, Transmission, Deluge |
| Transport | XML; `itemsPath` is JSONPath | Plex |

**Manifests do cover:** LazyLibrarian, Komga, Kavita, Audiobookshelf and \*Arr forks.
**They do not cover:** qBittorrent, Deluge, Transmission, NZBGet, Plex, **Calibre-Web**,
**Suwayomi** or **Navidrome**.

> 🚩 **Calibre-Web was on the covered list and is removed, because it has no REST API.** It exposes
> OPDS (Atom) and `/ajax/listbooks`, which is session-cookie authenticated; neither is a manifest
> target, and reconstructing a library by parsing Atom on a schedule is slow, fragile and lossy,
> since no identifiers survive the feed. The right adapter is **Tier 0 Go code opening Calibre's own
> `metadata.db` read-only** — `identifiers(book, type, val)` is a native typed external-id table,
> `books.uuid` is durable, `data(book, format)` is genuinely multi-format, `series_index REAL` sorts
> correctly, and `last_modified` is a real delta key. **That is a filesystem read and it is an
> explicit, written-down exception** to ARCHITECTURE §11.2 and ADR-0026's *"UsArr never touches a
> filesystem"*: a read-only handle on one SQLite file, not a scanner and not a library concept.
> Calibre-Web stays as the link-out target and byte server. **Suwayomi** is removed for a different
> reason — it is GraphQL. **Navidrome** is not manifest-describable either: `POST /auth/login`
> returning a JWT plus a `(subsonicSalt, subsonicToken)` pair is session establishment, which §3.2
> above places out of scope. (ARCHITECTURE §11.2 was corrected on this in an earlier round and this
> file was not; both lists now say the same thing.)
>
> **And "covered" describes what a manifest *could* express, not how anything ships.** The manifest
> tier does not exist until v0.3, so Komga and Audiobookshelf are **hand-written Tier 0 Go adapters
> in v0.1**, and Kavita is one in v0.2 (ARCHITECTURE §16).

This replaces an earlier "real diversity of the ecosystem" table that listed cookie-session,
409-challenge, JSON-RPC and XML as *axes the manifest accommodates*, immediately next to a manifest
grammar that cannot express any of them.

**One construct may be added later, and only on demand:** a `request.body` template, which unblocks
JSON-RPC read paths. Add it if and only if a service on the target list needs it; otherwise add
nothing.

> **The discipline is resisting DSL growth.** When a feature request would add control flow, the
> answer is "write a Tier 0 provider". With the WASM tier deferred (FUTURE.md §1) that is the only
> answer today, which is a real cost and is accepted deliberately rather than papered over.

---

## 4. Connection wizard rules

- **Mandatory connection test before save.** Homarr requires this and it eliminates an enormous
  class of "why is my dashboard blank" support load. Do not make it skippable.
- **Changing `base_url`'s scheme, host or port invalidates the stored credential.** The save
  requires re-entering the key, and a test against a modified `base_url` uses **only** the key typed
  into the form, never the stored one. Without this rule the masked display is cosmetic: change
  `base_url` to a listener you control, click Test, and read the `X-Api-Key` off your own log. The
  AAD binding in security.md §1 is the second layer.
- **Probe the URL base.** `UrlBase=/sonarr` makes *every* path `/sonarr/api/v3/…`,
  `/sonarr/signalr/messages`, `/sonarr/ping` — but `SystemResource.urlBase` reports it and you need
  the right path to read it. Probe `{base}/ping` and `{base}/api/v3/system/status` **with and
  without a trailing path segment** and store the resolved base.
- **`GET /ping` is the ideal probe.** `[AllowAnonymous]`, returns `{status: "OK"|"Error"}`, and
  returns 500 with `status:"Error"` if the \*Arr's own DB is unreachable — so it distinguishes "not
  a Servarr app" from "Servarr app that is sick".
- **`system/status.appName` + `version` + `urlBase` is the handshake**, and the
  `X-Application-Version` header is present on *every* API response, so version detection is free.
- **API version is per app and does not track app version.** Sonarr 4.x / Radarr 5.x / Whisparr
  2.x–3.x → `/api/v3`. Lidarr / Readarr / Prowlarr → `/api/v1`. **There is no "Radarr v5 API."**
- **Ship the OpenAPI specs with UsArr.** `app.UseSwagger()` is guarded by `if (BuildInfo.IsDebug)`,
  so a production instance does **not** serve `/docs/v3/openapi.json`. Do not try to fetch it.
- **Record what each instance actually returned** for `Content-Type` on the big list endpoints
  (sync.md §2) so the parser has ground truth rather than a spec reading.
- **TLS: TOFU SPKI pinning, not `verify_tls=0`.** A blanket `verify_tls=0` means no server
  authentication at all, and UsArr then sends a full-admin \*Arr key to whatever answers — a
  MagicDNS race, a poisoned container DNS lookup, or a released container IP is enough. On first
  connect to an untrusted certificate, show the fingerprint and store `service_instance.tls_spki_pin`;
  reject a silently changed pin with a loud error thereafter. Any global insecure-hosts env var is a
  bootstrap seed for that column and logs a warning on every use.
  **Also:** when the SSRF layer dials a validated IP (security.md §2), the request must retain the
  original hostname for `Host`, SNI and certificate verification. The naive "connect to the pinned
  IP" implementation verifies the certificate against the IP, fails, and the fix a developer reaches
  for is `InsecureSkipVerify`.
- **Warn on plaintext to a non-tailnet host.** A tailnet encrypts device-to-device traffic, so plain
  HTTP to a tailnet peer is acceptable; plain HTTP to a Docker-bridge or LAN backend puts a
  full-admin credential in cleartext on a shared segment. Warn when `base_url` is `http://` and the
  resolved address is neither loopback nor inside the tailnet range.
- **Render provider settings generically from `/schema`.** The shared `Field` model
  (`{order, name, label, helpText, value, type, advanced, selectOptions, privacy, …}`) is the key to
  a generic settings editor, and **`Field.privacy ∈ {normal, password, apiKey, userName}` tells you
  exactly which fields to redact** in the UI and in logs.
- **POST/PUT of provider resources must round-trip the whole `fields[]` array** from `/schema`.
  Partial updates are rejected upstream.
- **`DELETE /api/v3/series/{id}?deleteFiles=&addImportListExclusion=`** — destructive defaults
  matter. **Always send them explicitly**, never rely on the server default.
- **Custom formats are externally managed.** Recyclarr and Configarr write TRaSH custom formats and
  quality profiles into Sonarr/Radarr. UsArr must read them and **never clobber them.**
