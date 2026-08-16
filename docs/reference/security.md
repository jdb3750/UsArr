# Reference — Security model

**Status:** designed, not implemented. **Scope:** §1 (credential encryption, including rotation),
§2 (SSRF) and §5 (redaction) are **v0.1**. §4's authorization checks land with the surfaces they
protect.
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §14.

UsArr's threat profile is unusual: it is **a credential vault for a dozen services and an SSRF
cannon aimed at the user's own LAN.** The tailnet assumption removes the "internet-exposed by
design" leg of that. It removes neither of the other two: a hostile or buggy client on the tailnet,
or a hostile YAML manifest, reaches the same internals.

An \*Arr API key grants **full admin** — delete the library, change root folders, restart the
process. It is a single opaque 32-hex string with **no scoping whatsoever**. Everything below
follows from that.

---

## 1. Credential encryption at rest

**Envelope encryption.** Per-record nonce with **AES-256-GCM**, DEK wrapped by a KEK. Column-level,
on the sensitive fields only — today that is `service_instance.api_key_enc`.

### 1.1 The stored format is versioned

```
api_key_enc := kek_id (uint32 BE) || nonce (12 B) || wrapped_dek (40 B) || ciphertext || tag (16 B)
```

`kek_id` is **also stored as a plain column** (`service_instance.kek_id`), so a rotation can
`SELECT … WHERE kek_id = :old` and resume.

**Why this is not optional.** Without a key version, a rotation interrupted by SIGKILL, OOM, a
container restart or a full disk leaves a database in which some records are under KEK-old and some
under KEK-new **and nothing can tell them apart**. AEAD makes a wrong-key decrypt indistinguishable
from corruption, so the operator sees "decryption failed" with no way to know which half is which,
and recovery requires trial decryption against every historical key — which the design does not
keep.

### 1.2 AAD binds ciphertext to its location

```
AAD = table_name || ":" || column_name || ":" || primary_key || ":" || sha256(normalised host:port)
```

Without AAD, AES-256-GCM authenticates the ciphertext but **not where it sits**. Anyone with
database write access — a restored backup, a NAS share, an operator, a SQL-injection-equivalent bug
— can copy the Radarr row's ciphertext into a `service_instance` row whose `base_url` they control,
and UsArr will decrypt it and transmit it to them. Including the normalised host:port in the AAD
means changing `base_url` makes the stored ciphertext **fail to open** rather than silently succeed,
which is the cryptographic half of the rule in §1.5.

**A decryption failure with a valid KEK means tampering.** It is a loud, audit-logged failure, never
a silent skip.

### 1.3 KEK derivation

```
KEK        = HKDF-SHA256(USARR_SECRET_KEY, salt=<per-install random, stored>, info="usarr/kek/v1")
stream key = HKDF-SHA256(USARR_SECRET_KEY, salt=<same>,                       info="usarr/stream-token/v1")
```

Distinct `info` labels for the credential KEK and the URL-signing key, so the two can be rotated
independently. The earlier design said only "derived", which left the two purposes sharing one
secret — meaning rotating the vault key would silently invalidate every outstanding stream URL.

### 1.4 `USARR_SECRET_KEY` lifecycle — one behaviour, stated once

| Situation | Behaviour |
|---|---|
| Absent, no key file | **Generate** 32 random bytes, write the key file mode 0600, log a loud "back this up" warning, continue. |
| Present | **Validate**: must base64-decode to exactly 32 bytes, or startup **fails** with a named error. No hashing-to-length fallback, no lenient padding. |
| Empty string | **Treated as absent**, not as a key. (A compose file passing `${USARR_SECRET_KEY}` from an unset host variable resolves to empty, which must not be mistaken for "supplied".) |
| Matches a known placeholder | **Startup fails**, with a message naming the file the placeholder came from. The reject-list is hardcoded and contains every placeholder ever shipped in a released example file. |
| Present but wrong for existing rows | Loud failure plus the documented "re-enter your credentials" recovery flow. Never a cryptic decrypt error. |

**There is never a shipped default.** Homarr's `SECRET_ENCRYPTION_KEY` invalidating every stored
credential on redeploy, and arr-dashboard's equivalent failure with a lost `secrets.json`, are the
precedents. UsArr also **explicitly rejects Navidrome's compromise** — a hardcoded fallback
encryption key — which exists only because the Subsonic protocol forced recoverable passwords, a
forcing function UsArr avoids entirely by implementing `apiKeyAuthentication` only.

**Where the key lives, ranked:** the file at mode 0600 (the realistic default) or
`USARR_SECRET_KEY`/`USARR_SECRET_KEY_FILE`/a Docker secret; an OS keyring (fine for desktop, useless
in headless Docker — support, do not require); derived from an admin passphrase at unlock (best
security, breaks unattended restart — offer as an opt-in "locked mode"); an external KMS via a
pluggable provider. **The key file must not sit inside the directory the backup job and the
host-migration instructions copy** — otherwise "back up this volume" and "never store the key with
the ciphertext" are the same instruction, and users follow the one in the layout diagram.
`CONFIGURATION.md` §5 is authoritative for where that is; the nightly job and the API backup exclude
it explicitly, and the host-migration procedure names the key as a **separate step**.

### 1.5 Rotation — two-phase, resumable, and on the v0.1 milestone

`usarr key rotate` and `usarr keygen` are **v0.1 deliverables**. A documented recovery path that is
on no milestone is a recovery path that does not exist when the first user needs it.

```
1. Generate KEK_new. Write it to secret.key.new (mode 0600, fsync, fsync the directory).
2. Register BOTH keys as active in the keyring file. Commit that state to disk before touching
   the database. From here, either key can open any row.
3. Re-wrap in batches, per row:
       BEGIN IMMEDIATE
         decrypt with kek_id = old  →  re-encrypt with KEK_new (new AAD unchanged)
         UPDATE … SET api_key_enc = ?, kek_id = :new WHERE id = ?
       COMMIT
   Progress is durable: SELECT count(*) WHERE kek_id = :old is the remaining work.
4. Only when zero rows remain at the old kek_id: atomically rename secret.key.new → secret.key,
   drop the old key from the keyring, fsync.
5. On restart mid-rotation, the keyring shows two active keys and step 3 simply resumes.
```

The earlier design promised rotation "inside one transaction" — but the SQLite transaction and the
key-file write are **not one atomic unit**, so commit-then-crash-before-keyfile-write (or the
reverse) is unrecoverable loss of every stored credential. Two-phase with both keys active removes
the window entirely.

**Never return stored secrets to any client.** Display `••••••1a2b`. Provide a server-side test
button, subject to §1.6. `Field.privacy` from the \*Arr schema tells you exactly which upstream
fields to redact.

### 1.6 Re-testing a mutated `base_url` must not use the stored credential

> **Changing `service_instance.base_url`'s scheme, host or port invalidates the stored credential.**
> Saving requires re-entering the key, and a connection test against a modified `base_url` uses
> **only the key typed into the form** — never the stored one.

The attack this closes: the UI lets an actor change `base_url` while leaving `api_key_enc` intact,
then trigger a server-side request that sends the decrypted key to the new host. Point `base_url` at
`http://attacker.example`, click Test, read the `X-Api-Key` header off your own listener. The masked
display is cosmetic; the credential is exfiltrated in full. It matters even for a single owner
(session theft or a CSRF gap becomes full vault exfiltration without ever touching
`USARR_SECRET_KEY`) and more so under multi-user, where `admin.services.configure` is separable from
key knowledge. §14.2's SSRF policy explicitly *permits* integration fetches to arbitrary hosts
including private space, so nothing else blocks it.

Additionally: `service.base_url.changed` is audit-logged with old and new host at `result=warn`, and
it requires sudo mode (§6).

---

## 2. SSRF — three URL classes, not two

Users configure arbitrary URLs and UsArr fetches them **server-side from inside the LAN**. That is
textbook SSRF-by-design, and Jellyfin shipped exactly this bug: GHSA-rgjw-4fwc-9v96 /
CVE-2021-29490 let unauthenticated attackers enumerate the server's private network via remote-image
endpoints.

The earlier model had two classes — admin-configured integration URLs, and metadata/artwork URLs.
**It omitted the largest class.**

| Class | Source | Policy |
|---|---|---|
| **`configured`** | An admin typed it into the service form | May reach private space, **but only** the exact validated host:port of that `service_instance`. Never a URL from a request parameter. |
| **`provider`** | A known metadata provider (TMDB, Fanart.tv, Cover Art Archive), constructed by UsArr from a template | **Public only.** Deny cloud-metadata (`169.254.169.254`, `100.100.100.100`), link-local (`169.254/16`, `fe80::/10`), loopback, `0.0.0.0`, and any address not publicly routable. |
| **`derived`** | **Read out of an upstream response body** — `MediaCover.url`, `MediaCover.remoteUrl`, a Jellyfin/Komga/ABS cover field, a Prowlarr `ReleaseResource.posterUrl`, a Tier 1 manifest's `map.posterUrl` | **Allowlisted to the originating instance's own resolved host:port, or nothing.** If the URL is host-and-port-identical to the owning `service_instance.base_url`, treat it as a `configured` fetch to that one host. Otherwise apply the strict `provider` policy. Nothing in between. |

**Why `derived` is the dangerous one:** nobody configured those URLs. A compromised, malicious or
merely attacker-influenced backend returns `posterUrl: "http://169.254.169.254/latest/meta-data/"`
and UsArr becomes a private-network scanner and cloud-metadata reader on behalf of anything that can
write a string into a backend's metadata field — which on the \*Arr side includes indexer-supplied
and NFO-supplied data. That is CVE-2021-29490's shape with the attacker moved one hop upstream.
"Deny non-HTTP(S) schemes" does not help, and "only admins configure service URLs" does not apply,
because no admin configured this URL.

**Implementation, normative:** `image_asset` carries `origin_class` and
`origin_service_instance_id` (schema.md §12), and **the fetcher selects policy from the row, not
from the URL string.** The same applies to the metadata cache in `cache.db`. \*Arr `MediaCover.url`
is **instance-relative** and must be resolved **only** by joining to the owning instance's base URL
— never by trusting an absolute URL in that field.

### 2.1 The tailnet range is not a denylist entry

The earlier design added `100.64.0.0/10` to the metadata denylist with the rationale that *"a poster
URL has no business resolving to a tailnet peer"*. **On the default deployment that sentence is
false**: the user's Komga, Jellyfin, Audiobookshelf and Navidrome *are* tailnet devices at
`100.x.y.z`, their cover art is served from those hosts, and UsArr is required to proxy and cache
it. The predictable outcomes were both bad — covers silently failing on the flagship deployment (a
support-load bug that gets "fixed" by disabling the denylist wholesale), or the denylist bypassed
for anything image-shaped, which reopens the whole class.

**Resolve it with provenance, not CIDRs**, per the table above. An artwork fetch whose
`origin_service_instance_id` is set is validated against **that instance's own resolved host:port
only**, and is permitted there regardless of range. Artwork with no originating instance gets the
strict public-only policy — which does include `100.64.0.0/10`, because a *TMDB* URL resolving to a
tailnet peer is genuinely wrong.

### 2.2 The rest of the egress policy

1. **Only admins may configure service URLs.** Never expose URL configuration to a non-admin role.
2. **Resolve the hostname yourself, validate the resulting IP, then connect to that IP** — pin it.
   This closes the DNS-rebinding/TOCTOU window. Retrieve **both A and AAAA** records and validate
   the resolved addresses, not the domain string. **Validation re-runs on every dial**, not once at
   configuration time; reusing an already-validated keep-alive connection is fine, a new dial is
   not.
   **The TLS handshake must still use the original hostname** for `Host`, SNI and certificate
   verification. Implement with a custom `DialContext` that connects to the validated IP while the
   `http.Request` retains the original host. The naive "connect to the pinned IP" implementation
   fails certificate verification, and the fix a developer reaches for is `InsecureSkipVerify`.
3. **Redirects: follow at most 3 hops, revalidating at every hop.** The earlier text offered
   "disable redirect following **or** re-validate every hop", and only the second branch is viable —
   TMDB, Cover Art Archive (Internet-Archive-backed) and Fanart.tv all redirect, so an implementer
   who takes the first branch ships a broken image pipeline and then "fixes" it by re-enabling
   redirects globally, losing the control entirely. Reject any cross-scheme or downgrade-to-`http`
   hop. **Never carry `Authorization` or credential query parameters across a hop.**
4. **Use a battle-tested IP parser.** Encoding bypasses (`0x7f.1`, `2130706433`,
   `[::ffff:127.0.0.1]`, octal, dword, mixed) defeat naive string checks, and parser behaviour
   genuinely differs between libraries. Parse to a `netip.Addr` and test the address, never the
   string.
5. **Deny non-HTTP(S) schemes** — no `file:`, `gopher:`, `dict:`, `ftp:`.
6. **Numeric caps, not "cap response size and time":** artwork ≤ **20 MB**, metadata JSON ≤
   **32 MB**, \*Arr list endpoints ≤ **200 MB** (§7.3's own payload estimates imply 30–80 MB), with
   the layered timeouts from sync.md §4. Never proxy a raw upstream body back to a client verbatim.
7. **Defence in depth at the network layer** — document a recommended egress policy and a dedicated
   Docker network.

**Tier 1 manifests are inside this boundary, not outside it** — see providers.md §3.1, which
specifies URL confinement at the template level.

---

## 3. Northbound credentials

- **`client_credential` keys are server-generated**, ≥128 bits from `crypto/rand`, stored as
  **HMAC-SHA256 under a server-side key**, looked up by the unique `key_prefix`, compared with
  `crypto/subtle.ConstantTimeCompare`.
- **Argon2id is wrong here, and the reason must be recorded so nobody "fixes" it back.** A
  `client_credential` is not a password; it is a server-generated high-entropy bearer token.
  Memory-hard KDFs exist to defend *low-entropy* secrets against offline cracking. Applying one to a
  128-bit random token buys nothing and costs the entire performance budget: OpenSubsonic clients
  authenticate **per request**, a Symfonium poster grid issues ~60 `getCoverArt` calls, and at
  OWASP's m = 19 MiB that is 1.1 GB of transient allocation and 60 sequential memory-hard runs on a
  Raspberry Pi. Worse, verification must run *before* the key is known to be valid, so any device on
  the tailnet can drive the process into OOM with `GET /rest/ping?apiKey=garbage` in a loop.
- **Argon2id stays for `user.password_hash` only** — m = 19456 KiB, t = 2, p = 1, admin-tunable, full
  PHC string stored. **There is no pepper**: it was referenced twice in prose and specified nowhere
  (no env var, no storage location, no rotation procedure), and a pepper silently absent on one
  deploy and present on another locks users out. Per-hash salts are the design.
- `/rest/*` and `/opds/*` are in the **tighter rate-limit bucket**, keyed on `key_prefix` **and**
  peer IP, with a cheap "does this prefix exist at all" pre-check before any crypto.

---

## 4. Authorization

> **Every UsArr API response — northbound protocol surfaces included — is filtered by UsArr's own
> permission model, with the backend's policy as a second layer. Never construct a UI that hides
> items the API would still return.**

This is the strongest statement in the security model, and it has structural preconditions that must
hold from v0.1 or it cannot be honoured later without a redesign:

1. **Every cross-instance read path takes an access-scope parameter** (ARCHITECTURE §1.3 rule 2) —
   the grid, search, the client prefix index, the availability rollup. In v0.1 there is one scope
   and it is the owner's; that is the degenerate case, not the design.
2. **The client prefix index is built per access scope** and ETagged by
   `(user_id, access_scope_version)`. A whole-library titles-and-years payload shipped to a `kids`
   role is the doc's own anti-pattern rendered literally.
3. **`search_doc` carries the instance scope so FTS results are filtered in the join**, never
   post-filtered. Post-filtering silently breaks keyset page sizes and leaks existence through
   result counts and ranking positions.
4. **`work.availability` and `have_count` are computed within the caller's scope.** A rollup
   computed across instances the viewer is not entitled to is an oracle for restricted content — and
   that badge is the flagship demo, so it would be a *prominent* oracle.
5. **Every `usarr_id` resolution performs the check before any backend call**, returning the
   protocol-native not-found (Subsonic 70), never 403. Authorization **must never depend on ID
   secrecy**: base32 decodes in one line and \*Arr native ids are small sequential integers, so the
   ID space is enumerable in a few thousand round trips (gateway.md §3).

**`/img/*` is authenticated** and authorized against the owning item, served
`Cache-Control: private, max-age=31536000, immutable`. A content-derived cache key justifies
*immutability*, not *publicness* — those are different properties, and `public` on a
per-user-authorized resource tells shared caches (a reverse proxy, a corporate middlebox, a service
worker shared across profiles) to store and re-serve it across users. Genuinely public provider
artwork is served from a distinct `/img/public/*` path so the distinction is structural.

**`POST /api/v1/system/backup` and the UI download** are gated on `admin.system.backup`,
audit-logged with actor and IP, rate-limited, and reachable **only** through the cookie-session path
— never a `client_credential`, never a forwarded auth header. A Subsonic API key that reached an
unprotected backup endpoint would download the entire vault: every wrapped DEK, every password hash,
every session row and the full audit log. Backup files are written **mode 0600 in a 0700 directory,
independent of `UMASK`**.

---

## 5. Redaction is middleware, not a convention

✅ Verified against the OpenSubsonic spec: *"An API key is used as a query parameter
`apiKey=<api key>`."* The spec does not require TLS. **So UsArr's northbound bearer secret is in the
request line of every Subsonic call**, and the stream token is in a URL by construction. Where that
lands by default: UsArr's own access/`debug`/`trace` logs; `audit_log.metadata_json` if request URLs
are recorded; any reverse proxy in front; the client's own logs; `Referer` on anything an
acquisition link reaches.

The earlier redaction rule covered **southbound** `Field.privacy` fields only, and said nothing about
UsArr's own inbound URLs — which is exactly where the northbound credential lives.

> **A fixed deny-list of query parameters — `apiKey`, `p`, `t`, `s`, `access_token`, `api_key`,
> `apikey`, `token`, `sig` — and the `Authorization` and `X-Api-Key` headers is redacted to
> `<redacted>` BEFORE any log line, audit row, error message, SSE payload or support bundle is
> produced, at every log level including `trace`. This is a middleware, not a convention.**

- **`key_prefix`, never the key**, is what appears in logs and in the audit trail.
- **URLs stored in the database are in scope too**: `image_asset.source_url` and the `http_cache`
  keys store the **credential-stripped** URL, and an ingest assertion rejects writing a `source_url`
  containing `api_key`, `apikey`, `token` or `key=`. TMDB v3, Fanart.tv and Comic Vine all
  authenticate by query parameter, so without this the key persists in the database, in every
  `VACUUM INTO` backup and in any support bundle — and, because `cache_key = sha256(source_url)[:16]`,
  rotating the provider key would silently invalidate the entire image cache. Derive `cache_key`
  from the stripped URL.
- **Recommend TLS for the OpenSubsonic and OPDS surfaces** (tsnet certificates or a proxy) and warn
  that plain HTTP exposes the credential on every request.

> 🚩 **A query-parameter deny-list plus two headers does not cover two secret locations that
> genuinely exist southbound (added 2026-08-16, from the comics research). Both are one-line
> additions now and expensive to discover from a leaked support bundle later.**
>
> 1. **Secrets in a response *body*.** **Mylar3's `listProviders` command returns its configured
>    indexer API keys in the response body.** A deny-list over the request URL never sees them, and
>    UsArr logs upstream response bodies on error by design — §17.3's "Problem" column is **verbatim
>    upstream text** by requirement. So the redaction middleware must also run over **upstream
>    response bodies before they are logged, stored in `sync_report`, shown in the Services column or
>    put in a support bundle**, keyed on the field names a provider declares as secret. This is the
>    same class as `Field.privacy` and must not be left to the request path alone.
> 2. **Secrets in a URL *path segment*.** **Kavita carries its API key in the path**, not the query
>    string: `/api/Opds/{apiKey}/…`, and the same for its KOReader routes. A query-parameter
>    deny-list catches nothing, and the key then lands in proxy logs, browser history and
>    `Referer` headers. **Redaction must operate on the path as well as the query**, using each
>    provider's declared credential placement rather than a fixed parameter list — and UsArr must
>    **never** copy this pattern on its own surfaces (its OPDS credential is HTTP Basic, and its
>    OpenSubsonic key is a query parameter the spec fixes).
>
> Related, and already covered correctly by the existing rules — noted because it is a live example
> of the class the earlier threat model omitted: **Kavita exposes `GET /api/Image/web-link?url=…`**,
> an image proxy taking an arbitrary URL. Any cover URL UsArr follows from a Kavita response is
> origin class **`derived`** (§2) and is allowlisted to Kavita's own resolved host.

---

## 6. Sessions, CSRF, rate limiting, audit

- **Sessions:** an opaque server-side id in a `HttpOnly; Secure; SameSite=Lax` cookie — **not a JWT
  in localStorage**, per OWASP, because one XSS discloses every token. Both **idle and absolute**
  timeouts (different failure modes). Regenerate the id on privilege change. Server-side state is
  needed anyway for logout, an active-sessions list and admin revocation.
- **Sudo mode.** A **5-minute re-authentication window**, recorded in `session.sudo_until` and
  audit-logged, is required before: adding or changing a service credential, changing a `base_url`
  (§1.6), downloading a backup, issuing or revoking a `client_credential`, and rotating the key.
  Without it a single stolen cookie is a month-long window over the whole vault.
- **CSRF:** cookie sessions mean CSRF applies. `SameSite=Lax` blocks common cases but is **not
  sufficient alone** — use a synchronizer/double-submit token for all state-changing requests and
  require `Content-Type: application/json` (which blocks simple cross-origin form POSTs). Keep the
  cookie and bearer/API-key auth paths **strictly separate**, so a browser cannot accidentally
  authenticate an API endpoint with an ambient credential.
- **Rate limiting:** strict per-IP *and* per-username limits on auth endpoints with exponential
  backoff and temporary lockout; constant-time comparison; **identical error text and timing** for
  unknown-user vs bad-password. A tighter bucket for expensive endpoints: search, scan trigger,
  image resize, release fan-out, **and `/rest/*` and `/opds/*`**.
- **Trusted headers are a footgun.** If UsArr trusts `Remote-User` unconditionally, anyone who can
  reach the app port directly — bypassing the proxy — is instantly any user, including the owner.
  Navidrome learned this and now requires a CIDR allowlist. Non-negotiable rules: off by default; an
  explicit trusted-proxy CIDR allowlist; an explicit header-name configuration; **strip the
  configured headers from every inbound request** before processing and re-read only from the
  trusted path; **never** apply trusted-header auth to API-key endpoints or the OpenSubsonic/OPDS
  surfaces. The same allowlist governs `X-Forwarded-For`. **Never grant privileges based on "looks
  like a LAN IP"** — GHSA-qcmf-gmhm-rfv9 is exactly this bug in Jellyfin, where a spoofed source IP
  made an attacker look local and let them restart the server unauthenticated.
- **Audit log:** covers login success/failure, session created/revoked, permission change, user
  create/delete, service credential added/changed/removed (**value never logged**), `base_url`
  changed, request submitted/approved/declined, admin settings change, client credential
  issued/revoked, backup downloaded, and key rotation. Exposed in the admin UI as a **plain
  paginated list** — the filtered audit UI is deferred (FUTURE.md).
  **"Append-only" is a mechanism, not an aspiration:** no `UPDATE`/`DELETE` statements against
  `audit_log` anywhere in the codebase, enforced by a lint rule **and** `BEFORE UPDATE/DELETE`
  triggers that raise (schema.md §9), plus a rolling `prev_hash` chain so tampering is detectable.
  Say plainly that this is tamper-**evident**, not tamper-**proof** — anyone with the volume can
  still edit the file, and the stated purpose ("who deleted this") is exactly the case where the
  actor has that access.

---

## 7. Deployment posture

- **Distroless, non-root from PID 1, no PUID/PGID chown.** `gcr.io/distroless/static` has no shell
  and no `chown`, so an LSIO-style entrypoint that chowns `/config` and drops privileges requires a
  shell base and starting as **root** — and the resolution a developer reaches for when those two
  requirements collide is "use alpine and start as root", which is the weaker choice made by
  accident. Starting as root means a container escape or a compromised entrypoint runs as root with
  the master key in its environment. UsArr documents `chown 65532:65532 ./config` instead.
- **The health listener is health-only.** With tsnet enabled nothing listens on localhost, which
  kills container healthchecks; the answer is a loopback listener exposing exactly
  `/api/health/live` and `/api/health/ready` and **nothing else, ever**. Inside a container
  "loopback" is shared by every process in the network namespace — including any sidecar the user
  adds and anything with `network_mode: service:usarr` — so an unauthenticated *admin* surface there
  would be a privilege-escalation path.
- **Supply chain:** base image pinned by digest, `--provenance=true --sbom=true`, cosign signing,
  gating vulnerability scanning in the pre-commit gate, and pinned tool versions. Enforcement lives
  in the `Makefile` and CI, outside this document.
- **Internet exposure**, if a user chooses it: HTTPS enforced with HSTS and secure cookie flags;
  reject plaintext HTTP for auth (with an explicit "behind a TLS-terminating proxy" mode); a forced
  admin setup wizard on first run; login rate limiting on by default; a strict CSP (no
  `unsafe-inline`, no `unsafe-eval`), `X-Content-Type-Options: nosniff`, `Referrer-Policy:
  no-referrer`, `frame-ancestors 'none'`; the SSRF policy on by default; a trusted-proxy allowlist
  required before honouring any forwarded header; a published security policy and advisory process.
  **In exposed mode, refuse to auto-generate the master key** — require an explicit one, which is
  the only thing the exposure checklist adds over §1.4.
