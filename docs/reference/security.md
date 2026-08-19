# Reference — Security model

**Status:** partly implemented, and this line deliberately does not enumerate which parts — read
that off the tree, per `CLAUDE.md`. `internal/crypto` and `internal/config` own §1, `cmd/usarr`
owns its rotation command, `internal/ssrf` owns §2 and §5's parameter list, and
`internal/httpapi`'s route table in `server.go` owns which of §3, §4 and §6's surfaces exist at
all. **A guard named in this document is not evidence that anything calls it.** Several here are
built ahead of the surface that would reach them; each says so where it is described, and a
sentence that does not say so is claiming a live caller.
**Scope:** §1 (credential encryption, including rotation), §2 (SSRF) and §5 (redaction) are **v0.1**.
§4's authorization checks land with the surfaces they protect.
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
AAD = table_name || ":" || column_name || ":" || primary_key || ":" || sha256(normalised scheme://host:port)
```

Without AAD, AES-256-GCM authenticates the ciphertext but **not where it sits**. Anyone with
database write access — a restored backup, a NAS share, an operator, a SQL-injection-equivalent bug
— can copy the Radarr row's ciphertext into a `service_instance` row whose `base_url` they control,
and UsArr will decrypt it and transmit it to them. Including the normalised origin in the AAD
means changing `base_url` makes the stored ciphertext **fail to open** rather than silently succeed,
which is the cryptographic half of the rule in §1.5.

**The bound value is the full origin, scheme included — not just `host:port`.** It was `host:port`
only, and that left exactly one edit uncaught by the cryptographic layer: flipping
`https://nas:443` to `http://nas:443` changed nothing in the AAD, so the envelope still opened and
UsArr would send a full-admin `X-Api-Key` over **plaintext** to a host the attacker can now MITM.
§1.6 is normative that a **scheme** change invalidates the credential, and the §1.6 re-entry rule
was the only thing enforcing it — but that is an application-layer control, and this section's whole
argument is that the cryptographic layer has to hold *when the application layer is bypassed*. A
defence-in-depth control cannot be the sole control against the threat it was written to back up.

Consequence for implementers: anything that compares two base URLs to decide whether the stored
credential is still valid must normalise the same way the AAD does (`crypto.NormalizeOrigin`), not
to a bare `host:port` (`crypto.NormalizeHostPort`, which exists for `ssrf.Options.AllowedHostPort`).
If the re-entry check says "unchanged" while the AAD says "changed", a legal edit produces a
credential that can never be opened again.

**A decryption failure with a valid KEK means tampering.** It fails loudly and names the recovery,
and it is never a silent skip: `cmd/usarr`'s `openCredential` returns an error naming both causes —
an edited `base_url`, or a `keys/` directory that does not match the one that sealed the row —
rather than proceeding without a credential. **It also appends one `credential.open` / `fail` audit
row**, so a tampering attempt discovered on a background path — the 60-second health prober, a
bootstrap import — leaves a durable trace rather than a log line that rotates away. Only failures
are audited: a successful open happens on every client build and every probe, and a row a minute
per instance would bury the one row that matters.

The row is written by `cmd/usarr`'s `auditFailedOpen`, and three things about it are load-bearing
rather than incidental:

- **The actor is the system sentinel (`store.SystemUserID`, 0), not NULL.** `audit_log.actor_user_id`
  is nullable and carries no foreign key, and every scoped audit read renders
  `actor_user_id IN (0, :uid)` — under SQL three-valued logic `NULL IN (0, 1)` is *unknown*, so a
  NULL actor makes the row invisible to every reader in the codebase. A tamper-evidence row nothing
  can read is not evidence. The prober has no acting user, and 0 is exactly what "written by a path
  with no acting user" means.
- **The metadata carries ids and context only** — `kek_id`, `kind`, and a fixed reason. Never the
  envelope, never the ciphertext, never a plaintext (there is none; the open failed), and **not
  `base_url` either**, even though an edited base URL is one of the two causes: a base URL may carry
  userinfo (`http://user:pass@host`), and `audit_log` has `BEFORE UPDATE`/`BEFORE DELETE` triggers
  that `RAISE(ABORT)`, so anything written here is written forever. The instance id in `target_id`
  identifies the row that was tampered with.
- **Only `ErrDecrypt` is audited**, not `ErrMalformed`. A structurally broken envelope is detectable
  with no key at all and is corruption; a GCM or key-wrap failure under a KEK the keyring *holds* is
  the tampering signal this section is about.

`cmd/usarr`'s `TestATamperedCredentialWritesAnAuditRow` is the drill: it flips one byte of the GCM
tag in a real sealed envelope, asserts the open fails, and asserts the row exists *through the
scoped read* — so the NULL-actor invisibility above cannot pass it silently.

### 1.3 KEK derivation

```
KEK        = HKDF-SHA256(USARR_SECRET_KEY, salt=<per-install random, stored>, info="usarr/kek/v1")
stream key = HKDF-SHA256(USARR_SECRET_KEY, salt=<same>,                       info="usarr/stream-token/v1")
```

Distinct `info` labels for the credential KEK and the URL-signing key, so the two can be rotated
independently. The earlier design said only "derived", which left the two purposes sharing one
secret — meaning rotating the vault key would silently invalidate every outstanding stream URL.

**The salt is `$USARR_CONFIG_DIR/kek.salt`, and your backup must contain it.** 32 random bytes,
generated once per install, written at mode 0600 beside `usarr.db`. It is **not a secret** — its
value does not depend on staying confidential, only on being per-install and stable — but it is
**not regenerable** either, because a different salt is a different KEK. Lose it and every stored
credential is exactly as unrecoverable as if the master key were lost; `CONFIGURATION.md` §3.5
covers both under one procedure for that reason.

**The recoverable unit is the whole set: ciphertext + salt + master key.** Any two of the three
restore nothing. That is what puts the salt beside the database — inside the archive an operator is
already told to take — and the master key outside it, in a different place entirely
(`CONFIGURATION.md` §5, §6.1). Two of the three travel together on purpose; the third travels
separately on purpose.

⚠️ **On an install created before the salt moved, `keys/kek.salt` still exists, and it stays.**
Startup **copies** it to `$USARR_CONFIG_DIR/kek.salt` — it is a copy, never a move, and the legacy
file is deliberately kept forever. A move would have a window in which neither path holds the salt,
and a crash inside that window destroys the credentials the change exists to protect; the copy has
no such window and costs one 45-byte file. So do not read the new path as a replacement for the old
one, and do not delete the old one to tidy up. `internal/config.ResolveKEKSalt` carries the full
argument, and `CONFIGURATION.md` §3.2 states the operator-facing rule.

### 1.4 `USARR_SECRET_KEY` lifecycle — one behaviour, stated once

| Situation | Behaviour |
|---|---|
| Absent, no key file | **Generate** 32 random bytes, write the key file mode 0600, log a loud "back this up" warning, continue. |
| Present | **Validate**: must base64-decode to exactly 32 bytes, or startup **fails** with a named error. No hashing-to-length fallback, no lenient padding. |
| Empty string | **Refuse to start**, naming the variable. `CONFIGURATION.md` §3.2 owns this behaviour and this row restates it; if the two ever disagree again, §3.2 wins. (A compose file passing `${USARR_SECRET_KEY}` from an unset host variable resolves to empty. Treating that as "absent" would silently generate a *new* master key and orphan every stored credential, so the variable being *set but empty* is a hard error — distinct from it being unset, which is the ordinary first-run path.) |
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

### 1.5 Rotation — two-phase, resumable, and built

`usarr key rotate` **is implemented** (`cmd/usarr/keyrotate.go`). `usarr keygen` is not; §3.5 of
`CONFIGURATION.md` remains the route for a lost key, and rotation is the route for a *compromised*
one. A documented recovery path that is on no milestone is a recovery path that does not exist when
the first user needs it, which is why this one was on v0.1.

**There is no "keyring file".** An earlier draft of this section named one; nothing ever wrote it and
nothing needs it. The key files on disk **are** the keyring: `keys/secret.key` is the live key,
`keys/secret.key.new` is the pending one, and the keyring is rebuilt from whichever of them exist at
every start. That is the whole of the persisted rotation state, and it is what makes the procedure
crash-safe — there is no second artifact that can disagree with the key material, because key ids
are derived from the key material itself (ADR-0049).

**Rotation re-wraps; it never re-encrypts.** `crypto.Keyring.Rewrap` unwraps the row's DEK under the
old KEK and re-wraps it under the new one, leaving the nonce, the ciphertext, the GCM tag and the
AAD untouched. It never sees a plaintext credential and never needs the AAD, which is what makes a
batch interruptible at any row boundary and what makes a row whose `base_url` was edited (§1.6)
rotate normally instead of blocking the whole procedure.

```
1. Refuse if the key came from USARR_SECRET_KEY or USARR_SECRET_KEY_FILE. The command can only
   manage keys/secret.key; a key UsArr does not own cannot be renamed into place.
2. Resume from an existing secret.key.new if there is one; otherwise generate KEK_new and write
   secret.key.new through the O_EXCL never-clobbering writer (mode 0600, fsync the file, fsync
   the directory).
3. Register BOTH keys. Primary is the new one; the old one and the legacy id 1 stay registered
   for decryption. From here, either key opens any row.
4. Re-wrap in keyset-paginated batches, one bounded transaction each:
       BEGIN IMMEDIATE
         re-wrap the DEK: kek_id = old  →  kek_id = new (nonce, ciphertext, tag, AAD unchanged)
         UPDATE service_instance SET api_key_enc = ?, kek_id = :new WHERE id = ?
       COMMIT
   Progress is durable, and the remaining work is
   SELECT count(*) WHERE kek_id <> :new — "not at the new id", not "at the old id", so a row
   left at some third id by an earlier attempt is counted as work rather than skipped.
   TOMBSTONED ROWS ARE INCLUDED; see REVIEW-LOG.md RK-01 for why that has to be said out loud.
5. VERIFY. Re-read every row and prove the new material opens it: the kek_id column names the new
   id, the id inside the envelope agrees with the column, and the wrapped DEK unwraps under it
   (crypto.Keyring.Verify, which returns no plaintext). Any failure aborts before any key file is
   touched.
6. Only then promote: fsync secret.key.new, rename(2) it over secret.key, fsync the directory,
   drop the superseded keys from the keyring. rename(2) — not the link(2) that writeSaltFile uses
   — because promotion needs replace semantics and writeSaltFile refuses to clobber by design.
7. One audit_log row per phase: key.rotate.prepare, key.rotate.rewrap (with the count),
   key.rotate.promote. Metadata carries counts and key ids only.
```

**On restart mid-rotation the server registers both keys and continues serving; it does NOT rotate.**
Every row stays openable, a warning names `keys/secret.key.new`, and the primary stays on
`secret.key` so nothing new is sealed under material the operator has not promoted. Resuming is an
operator action with an audit trail — re-run `usarr key rotate`, which adopts the existing
`secret.key.new` rather than generating a third key.

The earlier design promised rotation "inside one transaction" — but the SQLite transaction and the
key-file write are **not one atomic unit**, so commit-then-crash-before-keyfile-write (or the
reverse) is unrecoverable loss of every stored credential. Two-phase with both keys active removes
the window entirely.

**Never return stored secrets to any client, in any form — masked included.** This paragraph used
to prescribe a `••••••1a2b` display, and the code refused it: rendering even those four characters
means decrypting a full-admin credential on a render path to show a fragment of it. The services
API carries a `has_credential` boolean instead, there is no reveal endpoint, and no response field
could carry one. Provide a server-side test button, subject to §1.6. `Field.privacy` from the \*Arr
schema tells you exactly which upstream fields to redact — and the indexer replication path goes
further than redaction, projecting through named allowlists so a private value never reaches the
row at all.

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

`internal/ssrf` implements all three classes and their policies, and tests all three. **Only
`configured` has a production caller today** — the per-instance egress client, built from the
instance's validated host:port. The `provider` and `derived` policies are built ahead of the
metadata and image pipelines that would select them, so nothing in the tree currently exercises
either. **A fetcher for either owes its class at construction, taken from the row that produced the
URL and never from the URL string.**

**Why `derived` is the dangerous one:** nobody configured those URLs. A compromised, malicious or
merely attacker-influenced backend returns `posterUrl: "http://169.254.169.254/latest/meta-data/"`
and UsArr becomes a private-network scanner and cloud-metadata reader on behalf of anything that can
write a string into a backend's metadata field — which on the \*Arr side includes indexer-supplied
and NFO-supplied data. That is CVE-2021-29490's shape with the attacker moved one hop upstream.
"Deny non-HTTP(S) schemes" does not help, and "only admins configure service URLs" does not apply,
because no admin configured this URL.

**Implementation, normative:** `image_asset` carries `origin_class` and
`origin_service_instance_id` (schema.md §12 is authoritative for the columns), and **the fetcher
owes its policy selection to the row, not to the URL string.** **No writer for that table exists
yet**, so no row has ever carried a class and `internal/store`'s format guard has nothing to
guard; the first writer owes both columns on every row it inserts. The same rule binds any metadata
cache in `cache.db`, which likewise has no writer today. \*Arr `MediaCover.url` is
**instance-relative** and must be resolved **only** by joining to the owning instance's base URL —
never by trusting an absolute URL in that field.

### 2.1 The tailnet range is not a denylist entry

The earlier design added `100.64.0.0/10` to the metadata denylist with the rationale that *"a poster
URL has no business resolving to a tailnet peer"*. **On the default deployment that sentence is
false**: the user's Komga, Jellyfin, Audiobookshelf and Navidrome *are* tailnet devices at
`100.x.y.z`, their cover art is served from those hosts, and UsArr is required to proxy and cache
it. The predictable outcomes were both bad — covers silently failing on the flagship deployment (a
support-load bug that gets "fixed" by disabling the denylist wholesale), or the denylist bypassed
for anything image-shaped, which reopens the whole class.

**Resolve it with provenance, not CIDRs**, per the table above — a requirement on the artwork
pipeline, which does not exist yet, rather than a description of a fetch that runs. An artwork
fetch whose `origin_service_instance_id` is set is validated against **that instance's own
resolved host:port only**, and is permitted there regardless of range. Artwork with no originating
instance gets the strict public-only policy — which does include `100.64.0.0/10`, because a *TMDB*
URL resolving to a tailnet peer is genuinely wrong.

### 2.2 The rest of the egress policy

1. **Only admins may configure service URLs.** Never expose URL configuration to a non-admin role.
   There is one role in the tree today and it is the owner's, so no role gate exists to inspect;
   the service routes are gated on authentication and sudo (§6). This binds whatever adds the
   second role.
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
   the layered timeouts from sync.md §4. All three are declared in `internal/ssrf`; **only the list
   cap has a caller today**, so the artwork and metadata figures bind the pipelines that will fetch
   those bodies rather than describing traffic that flows. Never proxy a raw upstream body back to
   a client verbatim.
7. **Defence in depth at the network layer** — document a recommended egress policy and a dedicated
   Docker network. Nothing to attach that to yet: there is no container build in the tree (§7).
8. **A TLS pin must be checked with `VerifyConnection`, not `VerifyPeerCertificate` alone.** A
   pin in `service_instance.tls_spki_pin` would be the *only* server authentication on a pinned
   instance, because the pinned path also sets `InsecureSkipVerify` to drop chain verification for
   a self-signed homelab certificate. `crypto/tls` does **not** re-verify certificates on a resumed
   session: it skips `verifyServerCertificate`, and with it `VerifyPeerCertificate`, calling only
   `VerifyConnection` (Go 1.25 `handshake_client_tls13.go` `readServerCertificate`, and the TLS 1.2
   resumption branch of `handshake_client.go`; both cite golang/go#31641). A pin installed only via
   `VerifyPeerCertificate` is therefore **silently skipped on every resumed handshake** — the
   connection succeeds with no pin check at all, which is a fail-*open* hole in the one control
   standing between a full-admin \*Arr API key and whatever answered the socket. `internal/ssrf`
   sets both hooks and shares one constant-time comparison between them, and
   `TestSPKIPinIsEnforcedOnAResumedSession` is the regression guard: it asserts the rejection came
   from the resumed path, so it cannot pass by quietly falling back to a full handshake. Reported
   by gosec **G123**, which is only present in golangci-lint ≥ 2.9 — see §7 on running the pinned
   linter.

   **Nothing writes `service_instance.tls_spki_pin`, so the pinned path is never taken in
   production.** The create handler leaves the column NULL by design and `ServiceInstanceUpdate`
   carries no pin field, both with the reasoning on them: a pin recorded silently would *downgrade*
   an instance behind a publicly-trusted certificate, and with no way to clear one an ACME renewal
   would lock the instance out permanently. The enforcement is built ahead of the TOFU enrolment
   UI, and **that UI, when it lands, owes a way to clear and re-accept a pin as well as set one.**
   Nor does the shipped transport set a `ClientSessionCache`, so it does not resume at all;
   resumption here is neither enforced nor disabled but unavailable, and the resumed path is
   guarded because **any** future transport that enables it must not silently lose the pin — not
   because anything exercises it now.

**Tier 1 manifests are inside this boundary, not outside it** — see providers.md §3.1, which
specifies URL confinement at the template level.

---

## 3. Northbound credentials

- **`client_credential` keys must be server-generated**, ≥128 bits from `crypto/rand`, stored as
  **HMAC-SHA256 under a server-side key**, looked up by the unique `key_prefix`, compared with
  `crypto/subtle.ConstantTimeCompare`. `internal/crypto` implements exactly that, and the table and
  its unique prefix index exist from migration 0001 — **but nothing issues, stores, revokes or
  verifies a credential, so no code path outside `internal/crypto` reaches any of it.** The
  northbound surface that lands first owes the issue and revoke flows, their audit rows and their
  sudo gate, and owes them on these helpers rather than a second implementation.
- **Argon2id is wrong here, and the reason must be recorded so nobody "fixes" it back.** A
  `client_credential` is not a password; it is a server-generated high-entropy bearer token.
  Memory-hard KDFs exist to defend *low-entropy* secrets against offline cracking. Applying one to a
  128-bit random token buys nothing and costs the entire performance budget: OpenSubsonic clients
  authenticate **per request**, a Symfonium poster grid issues ~60 `getCoverArt` calls, and at
  OWASP's m = 19 MiB that is 1.1 GB of transient allocation and 60 sequential memory-hard runs on a
  Raspberry Pi. Worse, verification must run *before* the key is known to be valid, so any device on
  the tailnet can drive the process into OOM with `GET /rest/ping?apiKey=garbage` in a loop.
- **Argon2id stays for `user.password_hash` only** — m = 19456 KiB, t = 2, p = 1, full PHC string
  stored, so raising the cost later does not invalidate existing hashes and `NeedsRehash` can
  upgrade one on login. **The parameters are compile-time constants**; making them admin-tunable is
  a configuration key that does not exist, and `CONFIGURATION.md` owns it if it ever does. **There
  is no pepper**: it was referenced twice in prose and specified nowhere
  (no env var, no storage location, no rotation procedure), and a pepper silently absent on one
  deploy and present on another locks users out. Per-hash salts are the design.
- `/rest/*` and `/opds/*` belong in the **tighter rate-limit bucket**, keyed on `key_prefix`
  **and** peer IP, with a cheap "does this prefix exist at all" pre-check before any crypto. Owed,
  not built, on both halves: neither route exists, and **there is no rate limiter anywhere in the
  tree** (§6).

---

## 4. Authorization

> **Every UsArr API response — northbound protocol surfaces included — is filtered by UsArr's own
> permission model, with the backend's policy as a second layer. Never construct a UI that hides
> items the API would still return.**

This is the strongest statement in the security model, and it has structural preconditions that
must hold **before the surface each one protects ships**, or they cannot be honoured later without
a redesign. Three of the five hold in code today; two name surfaces that do not exist yet, and are
requirements on whatever builds them.

1. **Every cross-instance read path takes an access-scope parameter** (ARCHITECTURE §1.3 rule 2) —
   the grid, search, the client prefix index, the availability rollup. Held: `store.Scope` is a
   parameter on every scoped read and it fails closed on an empty instance set. In v0.1 there is
   one scope and it is the owner's; that is the degenerate case, not the design.
2. **A client prefix index, when one is built, is built per access scope** and ETagged by
   `(user_id, access_scope_version)`. Nothing builds one today, and neither the index nor that
   version counter exists anywhere in the tree. A whole-library titles-and-years payload shipped to
   a `kids` role is the doc's own anti-pattern rendered literally.
3. **`search_doc` carries the instance scope so FTS results are filtered in the join**, never
   post-filtered. Held: the library semi-join and the instance predicate are both inside the
   retrieval legs, and the guard breaks each predicate in turn rather than asserting a proxy.
   Post-filtering silently breaks keyset page sizes and leaks existence through result counts and
   ranking positions.
4. **`work.availability` and `have_count` are computed within the caller's scope.** Held, for the
   one scope that flushes them; `internal/store`'s rollup states that single-answer-per-scope limit
   and names the two ways out of it. A rollup computed across instances the viewer is not entitled
   to is an oracle for restricted content — and that badge is the flagship demo, so it would be a
   *prominent* oracle.
5. **Every `usarr_id` resolution owes the check before any backend call**, returning the
   protocol-native not-found (Subsonic 70), never 403. No northbound surface exists yet, so there
   is no `usarr_id` in the tree to resolve. Authorization **must never depend on ID secrecy**:
   base32 decodes in one line and \*Arr native ids are small sequential integers, so the ID space is
   enumerable in a few thousand round trips (gateway.md §3).

**An authenticated `/img/*` is owed by the image pipeline**, authorized against the owning item
and served `Cache-Control: private, max-age=31536000, immutable`. A content-derived cache key
justifies *immutability*, not *publicness* — those are different properties, and `public` on a
per-user-authorized resource tells shared caches (a reverse proxy, a corporate middlebox, a service
worker shared across profiles) to store and re-serve it across users. Genuinely public provider
artwork belongs on a distinct `/img/public/*` path so the distinction is structural.

⚠️ **This paragraph used to end "Neither route exists today, and neither does the pipeline behind
them", and the first half is now false.** `GET /img/{key}` is registered in
`internal/httpapi/server.go` and `internal/httpapi/images.go` serves it: authorized through
`store.LookupImageAsset` against whichever `work` points at the asset, `private` as a constant that
no `origin_class` branches on, and one `404` for "no such key" and "not yours" alike. `/img/public/*`
is still not registered, deliberately — nothing produces provider artwork, and an unauthenticated
route with nothing behind it is a hole waiting for content — and the structural half of the
requirement holds from the other side: publicness is not expressible on the private route, so there
is nothing for a later change to flip. **The pipeline BEHIND the route is still absent**: nothing
fetches, encodes or writes an `image_asset` row, so every request answers `not_cached`.
`reference/http-api.md` §9 is the wire contract; read the route table for what exists, per this
document's own header.

**A backup endpoint and any UI download owe** a gate on `admin.system.backup`, an audit row with
actor and IP, a rate limit, and reachability **only** through the cookie-session path — never a
`client_credential`, never a forwarded auth header. None of that is built: there is no such route,
no UI download, no rate limiter, and no permission-string vocabulary at all — `admin.system.backup`
names a constant that does not exist. A Subsonic API key that reached an unprotected backup
endpoint would download the entire vault: every wrapped DEK, every password hash, every session row
and the full audit log. The automatic pre-migration backup that *does* exist is already written
**mode 0600 in a 0700 directory, independent of `UMASK`**, and an operator-triggered one inherits
that.

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

> **A fixed deny-list of query parameters — `apikey`, `api_key`, `token`, `access_token`,
> `accesstoken`, `auth_token`, `authtoken`, `refresh_token`, `refreshtoken`, `sig`, `signature`,
> `secret`, `secret_key`, `secretkey`, `p`,
> `t`, `s`, `passkey`,
> `torrent_pass`, `torrentpass`, `rsskey`, `authkey`, `apipasskey`, `cookie` (matched
> case-insensitively) — and the `Authorization` and `X-Api-Key` headers is redacted to
> `<redacted>` BEFORE any log line, audit row, error message, SSE payload or support bundle is
> produced, at every log level including `trace`. This is a middleware, not a convention.**

The list lives in exactly one place, `internal/ssrf`'s `credentialParams`, and there is deliberately
no second copy: a drifting duplicate is how a parameter ends up redacted on one code path and not the
other. Update the code and this list together.

🚩 **"Case-insensitively" folds CASE and nothing else, which is why every underscored name above sits
beside its underscore-free twin.** `isCredentialParam` lowercases the incoming name and does a map
lookup, so `accessToken` lowercases to `accesstoken` — **not** to `access_token`. An underscored
entry on its own therefore covers only the spelling that uses an underscore, which is the spelling a
JSON API is least likely to emit. The list was inconsistent with itself here for some time:
`apikey`, `refreshtoken` and `torrentpass` were present while `accesstoken`, `authtoken` and
`secretkey` were not, so it read as complete and was not.

That gap was **found by reading `redact.go` and then confirmed by a drill fired in both
directions** — `internal/vcrscrub`'s `TestAccessTokenDrillArmed` recorded a freshly generated
JWT-shaped probe **verbatim into a cassette through the fully armed scrubber** while `accesstoken`
was absent, and redacts it once the name is present; `TestAccessTokenDrillNeutered` runs the same
interaction with the `BeforeSave` hook removed, so the armed half's silence cannot be go-vcr
declining to persist the body. The invariant itself is pinned structurally rather than by
enumeration, in `internal/ssrf`'s `TestUnderscoredNamesCarryTheirCamelCaseTwin`, which walks the map
and fails on any underscored key whose twin is missing — so the next name added without one fails a
test rather than waiting for a cassette to capture it.

The shape is not speculative in either direction. **BookOrbit** — v0.1's catalogue source under
[ADR-0052](../DECISIONS.md#adr-0052) — returns its JWT as `{"accessToken": …}` from `login()` and
`issueTokensForUser()` (`bookorbit/bookorbit@main`,
`server/src/modules/auth/auth.service.ts`), which is what `POST auth/login`, `POST auth/refresh` and
`POST auth/magic-links/login` all resolve to. And it predates that choice: **Kavita's own vendored
spec** lists `accessToken` as required on `MalUserInfoDto` (`api/specs/kavita-v0.9.0.2.json`), a
MyAnimeList OAuth token, so a cassette in this tree could have captured the shape already.

**The private-tracker passkeys are not optional and were the gap.** The list originally stopped at
the provider and OpenSubsonic names. But Prowlarr's `ReleaseResource.infoUrl` and `.commentUrl` are
**indexer-supplied**, and UsArr surfaces `infoUrl` to the browser as `info_url` on every search
result. Private trackers put the user's personal passkey in the query string of exactly those
details and RSS URLs, under names like `passkey`, `torrent_pass`, `authkey` or `rsskey` — the names
Prowlarr's own indexer definitions use. Without them a tracker credential shipped straight to the
client. On a private tracker that is not a minor leak: the passkey is what the tracker attributes
traffic by, so a leaked one means account termination.

**Widening the list has a cost, so prefer long, specific names.** These parameters are also stripped
outright from redirect targets (`stripCredentials`), not just redacted in logs. A short generic name
such as `t` or `s` is a legitimate cache-buster or size parameter on many CDNs, so including it can
silently change which resource a redirect resolves to. The tracker-specific names carry no such risk.

- **`key_prefix`, never the key**, is what appears in logs and in the audit trail.
- **URLs stored in the database are in scope too**: `image_asset.source_url` and the `http_cache`
  keys store the **credential-stripped** URL, and no row may be written whose `source_url` still
  carries a credential parameter — the ingest path that writes these rows owes that assertion.
  ✅ **Discharged 2026-08-19 in `7e5934d`.** ⚠️ This paragraph used to read *"it is still owed and is owed by
  nothing that exists … no production code writes `image_asset` at all"*, which was true and is not
  any more. `internal/store`'s `PutPosterAsset` is the writer, and `checkImageSourceURL` is the
  assertion: it **refuses** a row whose `source_url` carries a credential parameter or userinfo —
  it does not strip and proceed, because §5 asks for an assertion and a sanitiser would leave the
  bug that produced the URL in place. `internal/imagecache`'s `Key` applies the same refusal where
  `cache_key` is derived, so a credentialled URL cannot yield a key either. Both consult
  `internal/ssrf`'s `IsCredentialParam` — the one deny-list — and both were drilled red before being
  trusted. **Limit:** query parameters and userinfo only, not path segments; `internal/ssrf`'s
  path-segment rule is a deliberately lossy heuristic and a false positive here would permanently
  refuse a legitimate cover. ⚠️ **The writer has never met a real cover**: `internal/imagepipeline`
  has been exercised only against images its own tests fabricate.
  **Which names those are is `internal/ssrf/redact.go`'s `credentialParams` to say, not this
  bullet's** — a shorter list restated here is the second deny-list, and the one that drifts is the
  one that leaks. TMDB v3, Fanart.tv and Comic Vine all authenticate by query parameter, so without
  this the key persists in the database, in every `VACUUM INTO` backup and in any support bundle —
  and, because `cache_key = sha256(source_url)[:16]`, rotating the provider key would silently
  invalidate the entire image cache. Derive `cache_key` from the stripped URL.
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
>
>    ✅ **Met, as of LS-170.** It was not: `internal/kavita`'s `parseErrorBody` put Kavita body text
>    into the error message with no redaction on any branch, and it reached three `cmd/usarr` log
>    lines and `service_instance.last_error` unredacted. Every branch of `parseErrorBody` now runs
>    `ssrf.RedactText` and then bounds the result; `service_instance.last_error` is redacted **before**
>    the row rather than on read-out; and `cmd/usarr`'s slog handlers are wrapped in a redacting
>    `slog.Handler`, which makes *"a middleware, not a convention"* true for that package for the
>    first time. `REVIEW-LOG.md` LS-170 § *Applied* has the detail, including the guard that was fired
>    against the unfixed code. **This closes the leak, not the requirement**: `ssrf.RedactText` finds
>    secrets inside URLs, so the *field-name-keyed* half stated above — a provider declaring which
>    response fields are secret, which is what the Mylar3 case needs — is still unbuilt.
> 2. **Secrets in a URL *path segment*.** **Kavita carries its API key in the path**, not the query
>    string: `/api/Opds/{apiKey}/…`, and the same for its KOReader routes. A query-parameter
>    deny-list catches nothing, and the key then lands in proxy logs, browser history and
>    `Referer` headers. **Redaction must operate on the path as well as the query**, using each
>    provider's declared credential placement rather than a fixed parameter list — and UsArr must
>    **never** copy this pattern on its own surfaces (its OPDS credential is HTTP Basic, and its
>    OpenSubsonic key is a query parameter the spec fixes).
>
>    **Half of this shipped.** `internal/ssrf`'s `redactPathSegments` now redacts credential-shaped
>    path segments on every URL that goes through `RedactURL`, which is what closes the
>    *private-tracker* case: `https://tracker.example/rss/<passkey>/torrents` was untouched by the
>    parameter deny-list and is the same leak as the query-parameter one with a different URL
>    layout. What has **not** shipped is the *provider-declared placement* half — Kavita's
>    `/api/Opds/{apiKey}/…` is matched only if the key happens to fit the heuristic's shape, and
>    that must not be relied on. When the Kavita client lands it declares where its credential sits
>    and redacts by position, not by shape.

**The path half is a heuristic, and it is calibrated to miss rather than to over-match.** There is no
parameter name to key on in a path, so a dot-separated path part is redacted only when it is at least
**20 characters**, is an unbroken run of `[A-Za-z0-9]` with no separator of any kind, holds at least
one letter *and* at least one digit, and does not read like words (vowel ratio and consonant-run
test). The thresholds come from Prowlarr's own log scrubber,
`src/NzbDrone.Common/Instrumentation/CleanseLogMessage.cs` on `develop`, which had to enumerate real
tracker URL layouts in order to stop leaking them: its unanchored path rules use `[a-z0-9]{16,}`, and
the shortest concrete key it names is TorrentLeech's 20-character RSS key. 16 is raised to 20 here
because upstream's rules are anchored to a host or a path prefix and this one is not — it runs on
every segment of every URL.

**The direction of error is deliberate.** `provenance` rows are immutable and permanent, so a false
positive corrupts a record that can never be repaired, while a false negative loses a passkey from a
log line. The accepted cost is that an opaque high-entropy identifier in a path — a ULID, a
hyphen-free UUID, a git sha — is redacted too, because it is indistinguishable by shape from a
passkey. UsArr's own routes carry no path segment near 20 characters, so nothing of its own is
affected. The heuristic is **not** applied to `stripCredentials`: removing a path segment from a
redirect target changes which resource is being requested.

### 5.1 Credentials already written to `provenance`

`provenance.nzb_info_url` is indexer-supplied and **permanent by design** — it records which tracker
and which release page a grab came from, and provenance rows are never overwritten. Rows written
before the grab path started redacting therefore hold a private tracker's passkey verbatim.
`release_candidate` heals itself (a 25-minute TTL and the sweeper), and `provenance` has no such
mechanism, so the repair is explicit.

> **`redactStoredCredentials` runs on every start, in `buildApp`, before the HTTP server exists.**
> It puts every stored `nzb_info_url` through `servarr.RedactURL` — the *same* helper the write path
> calls, so a stored value and a served value cannot disagree about what a secret is — and updates
> only the rows whose redacted form differs. It logs at **warn** when it changed anything, because
> that says a credential *was* on disk and the operator has to act.

It is in Go and not in a migration, deliberately: the repair needs the one deny-list *and* the path
heuristic, both of which live in `internal/ssrf`. Expressing either in SQL would create the second,
drifting copy that the deny-list's own comment forbids, and SQLite has no regular expressions, so the
path half could not have been written there at all. It runs every start rather than once because it
is idempotent by construction and because a run-once gate has the wrong failure mode for a security
repair: a process killed mid-pass would leave the remaining rows unredacted with the gate already
satisfied.

> ⚠️ **This repairs the live database only, and the problem is not fully erased.** A passkey already
> copied into a `VACUUM INTO` backup, a filesystem or volume snapshot, or a support bundle is still
> in that copy, and nothing UsArr does can reach it. **Rotate the passkey at the tracker and delete
> the affected backups.** Pre-migration backups (§`ARCHITECTURE.md` §15) are exactly the copies most
> likely to hold one, because one is taken immediately before the upgrade that lands this pass.
>
> Related, and already covered correctly by the existing rules — noted because it is a live example
> of the class the earlier threat model omitted: **Kavita exposes `GET /api/Image/web-link?url=…`**,
> an image proxy taking an arbitrary URL. Any cover URL UsArr follows from a Kavita response is
> origin class **`derived`** (§2) and is allowlisted to Kavita's own resolved host.

---

## 6. Sessions, CSRF, rate limiting, audit

⚠️ **If you arrived here looking for the credential-stripped `source_url` rule, it is §5, not this
section.** `internal/db/migrations/00005_library_sync.sql`'s `image_asset.source_url` comment
(*"CREDENTIAL-STRIPPED. See security.md §6."*, `:221` as of `ef3f041`) cites the wrong section, and
`internal/db/testdata/schema.sql` carries the identical string because SQLite stores an
intra-statement comment verbatim in `sqlite_schema` and the round-trip snapshot is dumped from
there. **Neither is corrected** — a merged migration is never edited, and that comment is a byte
`TestMigrationRoundTrip` compares — so this pointer is the fix. `docs/REVIEW-LOG.md` **LS-321**
carries the evidence. Not every §6 citation in the migrations is wrong: `00001_initial.sql`'s
`audit_log` comment (*"who deleted this"*) means this section and means it correctly.

- **Sessions:** an opaque server-side id in a `HttpOnly; Secure; SameSite=Lax` cookie — **not a JWT
  in localStorage**, per OWASP, because one XSS discloses every token. Both **idle and absolute**
  timeouts (different failure modes), both in the lookup predicate. Regenerate the id on privilege
  change, and **both of UsArr's privilege changes now do**: login mints a fresh session row, and
  opening the sudo window renames the existing one via `store.RegenerateSessionForSudo`, which
  rewrites `session.id` and sets `sudo_until` in **one** `UPDATE` so no instant exists with the new
  id and no window, or the window under the old id. `store.GrantSudo` still exists and still
  updates in place; it is correct only where the id is already fresh, which today means exactly one
  caller — `startSession`, opening the window on a row it has just inserted. ⚠️ This bullet used to
  read *"login mints a fresh session and opening the sudo window does not"*; that was true and is
  no longer.
  Two things the fix has to do together, because the server half alone is a logout: the new cookie
  is re-issued in the **same** response, and the CSRF token is rotated beside it exactly as
  `startSession` does. The absolute expiry is deliberately not extended — this is the same session
  under a new name, so re-authenticating never buys time past `SessionAbsoluteTimeout`.
  `internal/httpapi`'s `TestSudoRegeneratesTheSessionID` is the drill: it asserts the post-sudo
  cookie differs, that the **pre**-sudo value no longer authenticates, that the caller is still
  signed in under the new one, and that `created_at` is untouched.
  Anything that adds a second privilege level owes regeneration at the transition. Server-side
  state is needed anyway for logout, an active-sessions list and admin revocation.
- **Sudo mode.** A **5-minute re-authentication window**, recorded in `session.sudo_until` and
  audit-logged, is required before: adding or changing a service credential, changing a `base_url`
  (§1.6), downloading a backup, and issuing or revoking a `client_credential`. The first two gates
  are in force on the service routes; the other two name surfaces that do not exist yet and inherit
  the requirement when they land. **Key rotation is a CLI command with no session**, so a
  session-scoped window cannot gate it — its control is filesystem access to `keys/` plus the audit
  rows each phase writes. Without sudo a single stolen cookie is a month-long window over the whole
  vault.
- **CSRF:** cookie sessions mean CSRF applies. `SameSite=Lax` blocks common cases but is **not
  sufficient alone** — use a synchronizer/double-submit token for all state-changing requests and
  require `Content-Type: application/json` (which blocks simple cross-origin form POSTs). Keep the
  cookie and bearer/API-key auth paths **strictly separate**, so a browser cannot accidentally
  authenticate an API endpoint with an ambient credential.
- **Rate limiting is owed and unbuilt; the KDF concurrency bound is built.** These are two
  different controls against two different problems, and only one of them is here. No limiter
  exists anywhere in the tree, so `POST /api/v1/auth/login` is unthrottled today — reachable by
  anything that can reach the app port. **Tailnet-only exposure is a control the deployment
  provides and the code does not.** What a limiter owes when it lands: strict per-IP *and*
  per-username limits on auth endpoints with exponential backoff and temporary lockout, and a
  tighter bucket for expensive endpoints — search, scan trigger, image resize, release fan-out,
  **and `/rest/*` and `/opds/*`**. `FUTURE.md` names the seam it attaches to.
  What *is* built is the memory half. Every login attempt runs Argon2id at m = 19 MiB **including
  for a username that does not exist**, because the dummy hash below is burned deliberately, so
  request volume converted directly into memory pressure. `internal/crypto` now runs every KDF call
  behind a semaphore of `min(NumCPU, 8)` permits, capping peak KDF memory at `permits × 19 MiB`
  regardless of arrival rate — no state, no policy, and nowhere for state to live. The wait is
  bounded (2 s) and a shed call answers **503 `busy`**, so a flood does not become an
  unbounded-goroutine problem. The trade is stated rather than hidden: under load, login gets slow
  instead of the box falling over, and a queue long enough to shed load sheds legitimate logins
  first, because an attacker retries and a person gives up.
  **The bound is on the primitive, not on the login handler.** `HashPassword` and `VerifyPassword`
  each take a permit, so first-run setup and any future password change inherit the cap without
  their author knowing it exists. `handleLogin` makes **exactly one** KDF call, shared by the
  known and unknown branches, and answers the shed case identically for both — the semaphore
  around only the real verify would have let the unknown-account path (the cheapest request an
  attacker can send) skip the cap *and* would have reintroduced a 401-vs-503 asymmetry between the
  branches, which is precisely the enumeration oracle the dummy hash exists to close. Two drills
  pin it: `crypto.TestEveryKDFCallIsBounded` and
  `httpapi.TestASaturatedKDFShedsUnknownAndKnownUsersIdentically`, the second driving unknown
  usernames specifically.
  The rest of this bullet **is** in force and unchanged: constant-time comparison, and **identical
  error text and timing** for unknown-user vs bad-password, with one error value on every failing
  branch and a dummy PHC hash burning the equivalent Argon2id work on the unknown-account path.
  That equaliser is not negotiable — trading a user-enumeration oracle for a memory bound would be
  a bad swap.
- **Trusted headers are a footgun.** The `X-Forwarded-For` half is built: the header is stripped
  from every inbound request and re-read only from a peer inside the configured trusted-proxy CIDR
  allowlist, with a startup warning for an over-wide prefix. **`Remote-User`-style header
  authentication does not exist**, so the rules below are what it owes if it is ever added, not a
  description of running code. If UsArr trusted `Remote-User` unconditionally, anyone who could
  reach the app port directly — bypassing the proxy — would instantly be any user, including the
  owner.
  Navidrome learned this and now requires a CIDR allowlist. Non-negotiable rules: off by default; an
  explicit trusted-proxy CIDR allowlist; an explicit header-name configuration; **strip the
  configured headers from every inbound request** before processing and re-read only from the
  trusted path; **never** apply trusted-header auth to API-key endpoints or the OpenSubsonic/OPDS
  surfaces. The same allowlist governs `X-Forwarded-For`. **Never grant privileges based on "looks
  like a LAN IP"** — GHSA-qcmf-gmhm-rfv9 is exactly this bug in Jellyfin, where a spoofed source IP
  made an attacker look local and let them restart the server unauthenticated.
- **Audit log:** the actions written today are whatever `internal/httpapi` and `cmd/usarr` pass to
  `AppendAudit` — read them there rather than here, because this list has to grow with every
  surface, and the ones it named for surfaces that do not exist (permission change, user delete,
  request submitted/approved/declined, admin settings change, client credential issued/revoked,
  backup downloaded) were never written. The rule that does not change: **every state-changing
  operation owes a row, and a credential value is never in one.** **The audit log has no read
  surface** — no API route and no screen — so "who deleted this" is currently answerable only by
  opening SQLite. A **plain paginated list** in the admin UI is what it owes first; the filtered
  audit UI is deferred (FUTURE.md).
  **"Append-only" is a mechanism, not an aspiration:** no `UPDATE`/`DELETE` statements against
  `audit_log` **in production code**, enforced by `TestNoCodeMutatesTheAuditLog`
  (`internal/store/auditlint_test.go`, which walks every non-test `.go` file's string literals)
  **and** `BEFORE UPDATE/DELETE` triggers that raise (schema.md §9), plus a rolling `prev_hash`
  chain so tampering is detectable.
  Two corrections to what this paragraph used to say. First, the static check is a test in the
  `make check` gate, not a `.golangci.yml` rule: golangci-lint's `forbidigo` matches function-call
  identifiers, not the contents of string literals, and every such statement is a string literal —
  so the rule this document claimed could not have existed in the form it described, and for a
  while did not exist at all. Second, the scope is **production code**, not "anywhere in the
  codebase": `TestAuditChainDetectsTampering` and `TestAuditLogIsAppendOnly` mutate `audit_log`
  deliberately, to prove the triggers reject it and the chain notices. Forbidding those statements
  in tests would delete the tests that verify the guarantee.
  Say plainly that this is tamper-**evident**, not tamper-**proof** — anyone with the volume can
  still edit the file, and the stated purpose ("who deleted this") is exactly the case where the
  actor has that access.

---

## 7. Deployment posture

**There is no container build in this tree** — no Dockerfile, no compose file — and no tsnet
integration, so the first two bullets and the image half of the third are requirements on that work
when it lands, not descriptions of a shipped posture. The supply-chain bullet's tool pinning and
gating vulnerability scan do run, in the `Makefile`; so does most of the response-header half of
the internet-exposure bullet. Each says which below.

- **Distroless, non-root from PID 1, no PUID/PGID chown.** `gcr.io/distroless/static` has no shell
  and no `chown`, so an LSIO-style entrypoint that chowns `/config` and drops privileges requires a
  shell base and starting as **root** — and the resolution a developer reaches for when those two
  requirements collide is "use alpine and start as root", which is the weaker choice made by
  accident. Starting as root means a container escape or a compromised entrypoint runs as root with
  the master key in its environment. UsArr documents `chown 65532:65532 ./config` instead.
- **The health listener is health-only.** `/api/health/live` and `/api/health/ready` exist, but on
  the single main mux — there is no second listener, and no tsnet integration for one to answer a
  problem that has therefore not arisen yet. With tsnet enabled nothing would listen on localhost,
  which kills container healthchecks; the answer then is a loopback listener exposing exactly
  `/api/health/live` and `/api/health/ready` and **nothing else, ever**. Inside a container
  "loopback" is shared by every process in the network namespace — including any sidecar the user
  adds and anything with `network_mode: service:usarr` — so an unauthenticated *admin* surface there
  would be a privilege-escalation path.
- **Supply chain:** base image pinned by digest, `--provenance=true --sbom=true`, cosign signing,
  gating vulnerability scanning in the pre-commit gate, and pinned tool versions. Enforcement lives
  in the `Makefile`, outside this document.
  **Pinning a version is not the same as running it.** `make tools` installs into `$GOBIN`, which
  is very often not on `$PATH` — it is not on `$PATH` in this repo's own agent container — so a
  recipe that invokes a bare tool name runs whatever `$PATH` resolves first, and the pins decorate
  a file nothing consults. When that was live here, `$PATH` resolved a system-wide golangci-lint
  v2.5.0 against a pinned v2.12.2: **0** issues found where the pin finds **11**, because G123 and
  G124 did not exist yet in the older gosec, and the gate reported `check: OK` throughout. A
  security gate that silently degrades to a weaker gate is worse than no gate, since it produces a
  green result nobody re-examines. **The standing requirement: every pinned tool is invoked by
  absolute path *and*, where it can report its own version, asserted against the pin before it
  runs — both, because path alone runs a stale binary left by an older pin, and assertion alone has
  already executed the wrong binary in order to ask it what it is.** The `Makefile` is
  authoritative for whether that holds today; do not read this paragraph as a live bug report.
- **Internet exposure**, if a user chooses it. Partly in force already, on every response and not
  only an exposed one: the strict CSP, `X-Content-Type-Options: nosniff`, `Referrer-Policy:
  no-referrer` and `frame-ancestors 'none'` are set by `internal/httpapi`'s security-headers
  middleware, the session cookie carries the secure flags (§6), the SSRF policy is on with no
  switch to turn it off, and first-run owner creation is one-shot and closes once an owner exists.
  Not built: HSTS, the plaintext-HTTP refusal and its "behind a TLS-terminating proxy" mode, login
  rate limiting (§6 — the Argon2id concurrency bound there is a memory control, not a request
  control, and does not substitute for one), the trusted-proxy requirement as a *precondition* rather than a default, and
  a published security policy. The full list, as owed: HTTPS enforced with HSTS and secure cookie
  flags; reject plaintext HTTP for auth (with an explicit "behind a TLS-terminating proxy" mode); a
  forced admin setup wizard on first run; login rate limiting on by default; a strict CSP (no
  `unsafe-inline`, no `unsafe-eval`), `X-Content-Type-Options: nosniff`, `Referrer-Policy:
  no-referrer`, `frame-ancestors 'none'`; the SSRF policy on by default; a trusted-proxy allowlist
  required before honouring any forwarded header; a published security policy and advisory process.
  **In exposed mode, refuse to auto-generate the master key** — require an explicit one, which is
  the only thing the exposure checklist adds over §1.4.
