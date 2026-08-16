# Reference — The northbound gateway

**Status:** designed, not implemented. **Scope:** the OpenSubsonic read-only subset is **v0.4**;
OPDS, multi-instance aggregation and write-back are **v1.0**.
**Parent:** [`../ARCHITECTURE.md`](../ARCHITECTURE.md) §5.

---

## 1. The v0.4 endpoint subset — and why it is a subset

The OpenSubsonic spec is ~100 endpoints across ~15 categories plus ~28 negotiated extensions.
Specifying "an OpenSubsonic server" in one paragraph and calling it a milestone is how a
one-to-two-person project loses a year. The gateway milestone implements a **~20-method subset**, and
its success criterion is a sentence: *Symfonium connects with one API key, browses, searches and
plays.*

⚠️ **This table lists the thirteen the subset was first scoped around.**
[`../ARCHITECTURE.md`](../ARCHITECTURE.md) §16 v0.4 — which is **authoritative for scope** — raised
the count to ~20 on client evidence, adding `getGenres`, `getPlaylists`, `getPlaylist`, `getUser`,
`download` and `getMusicDirectory`, plus a spec-correct error responder for every unimplemented
method. Where this file and §16 disagree on the method list, **§16 wins**; the rows below are still
correct about tables and degradation for the methods they cover.

| Method | UsArr tables | Degradation rule |
|---|---|---|
| `ping` | none | Always 200 with the standard envelope |
| `getLicense` | none | Always `valid=true`; UsArr has no licence concept |
| `getOpenSubsonicExtensions` | none | Returns **UsArr's** implemented set, not any backend's. `apiKeyAuthentication` always. |
| `getMusicFolders` | `service_instance` | One folder per configured music-bearing instance, `id` = the instance id |
| `getIndexes` | `work` where `kind='artist'` | Alphabetical buckets from `sort_title`; `lastModified` = `max(updated_at)` |
| `getArtists` | `work` where `kind='artist'` | Same data, ID3 shape |
| `getArtist` | `work` + children where `kind='album'` | Missing children → empty list, never a 500 |
| `getAlbum` | `work` (`album`) + `work_track` | **Requires `work_track`** for disc/track ordering (schema.md §1.1) |
| `getAlbumList2` | `work` where `kind='album'`, keyset | `type=newest|alphabeticalByName|frequent|recent` map to existing indexes; unsupported `type` → error 0 |
| `getSong` | `work` (`track`) + `work_track` | — |
| `search3` | `search_doc` + FTS (search.md) | Corpus is top-level works; tracks come from a scoped `parent_work_id` query |
| `getCoverArt` | `image_asset` via the addressed link | Serves the proxied, downscaled image. `size` maps onto the width allowlist; an unlisted size snaps to the nearest allowed. |
| `stream` | `service_item_link` → provider | `io.Copy` proxy (§4) |

**Response envelope**, required on every response: `subsonic-response` with `status`, `version`
(the OpenSubsonic version UsArr implements), `type` (`"usarr"`), `serverVersion`, and
`openSubsonic: true`. Errors use the spec's own codes, and the list leads with the one an
api-key-only server actually emits: **44** invalid API key · **43** conflicting authentication
parameters (`apiKey` together with `u`) · **42** unsupported mechanism (plaintext `p`) · **41**
token auth removed (`u`+`t`+`s`) · **50** not authorized · **70** not found. **40 "wrong username or
password" is meaningless on a server with no usernames** and is listed last rather than first, which
is where it used to be. **Never a 500**, and never a 403 where 70 is the honest answer (see §3).
Every auth refusal carries `helpUrl`.

**Not in the gateway milestone, each its own later milestone with its own criterion:** OPDS (whose
auth scheme is settled in §2), multi-instance aggregation (whose ID format ships from day one
regardless — §3, because it cannot change later), write-back (§6), and the wider client matrix.

> ✅ **One Navidrome is a checked assumption, not a guess.** The owner confirmed on **2026-08-16**
> that he runs Navidrome for music and Audiobookshelf for audiobooks — a single Navidrome, which is
> the install this subset was scoped against. 🔍 Other users' installs are still an assumption, and
> multi-instance aggregation stays v1.0 either way. See [`../ARCHITECTURE.md`](../ARCHITECTURE.md)
> §16, v0.4.

---

## 2. Authentication

### OpenSubsonic

> **`apiKeyAuthentication` ONLY. Salt/token auth is actively refused.**

| Rule | Detail |
|---|---|
| Parameter name | **`apiKey`**, as a **query parameter**. ✅ verified against the extension spec. |
| Both `apiKey` and `u` present | **Reject with error 43** (corrected 2026-08-16 — this said 40). The apiKeyAuth extension spec: *"When an API key is provided, the client **must not** provide a `u` parameter; passing in `u` must be treated as an **error 43**"*, and *"If multiple conflicting authentication parameters are passed in, the server must return an **error 43**, Multiple conflicting authentication mechanisms provided."* Ambiguous credentials are never resolved in the client's favour. |
| `u` + `t` + `s` present without `apiKey` (**token auth**) | **Reject with error 41** (corrected 2026-08-16 — this said 40, then 42). **UsArr *is* the narrower case the spec reserves for 41**, because it refuses token-based authentication as a matter of policy: *"If a server removes support for token-based authentication, it must return **error 41**…"*. The previous row cited that sentence and then applied 42, which inverts it. |
| `p` present without `apiKey` (**plaintext password auth**) | **Reject with error 42** — *"Provided authentication mechanism not supported"*, which is the code the same spec sentence reserves for *"any other particular authentication mechanism"*. |
| `apiKey` present but unknown, revoked or malformed | **Reject with error 44, "Invalid API key"** — introduced by the very extension UsArr implements, and **the single most common failure an api-key-only server will emit**. It appeared nowhere in this document. |
| Any of the above | **Never silently ignore** — silent ignoring lets a client believe it authenticated and then behave as if it had. |
| **`helpUrl` on every auth refusal** | **Populate it**, pointing at UsArr's own API-key page. The spec introduces the field precisely for this: *"it is recommended that the server provide a meaningful url… in the `helpUrl`"*. Omitting it is why a user sees "wrong password" instead of "this server needs an API key" — the exact confusion the hard-rejection policy exists to avoid. |
| Spec force | The spec *recommends* that servers offering API-key auth no longer support salt/token. The refusal is **UsArr's own policy**, implemented as a hard rejection, not an omission. |
| TLS | The spec does **not** require it, so the key rides in the request line of every call. Serve over TLS (tsnet certs or a proxy); warn in the UI otherwise. Redaction is mandatory (security.md §5). |
| Verification cost | `key_prefix` lookup (unique index) → HMAC-SHA256 → `subtle.ConstantTimeCompare`. **Not Argon2id** — see ARCHITECTURE §5.2 for why, and do not change it back. |
| Rate limiting | `/rest/*` and `/opds/*` are in the tighter bucket, keyed on `key_prefix` **and** peer IP, with the prefix-exists pre-check before any crypto. |

⚠️ **Two of the three OpenSubsonic clients named in `ARCHITECTURE.md` §3 cannot authenticate here.**
Verified from source: **Amperfy** has zero occurrences of `apiKey` in its entire Swift source and
emits `u` + `t`/`s` only; **Feishin**'s Subsonic controller has no `apiKey` path either. **Symfonium**
is the reference client and ⚠️ **its own `apiKeyAuthentication` support is unverified** — its
documentation does not mention API keys and it is closed-source. The policy is still right; the client
matrix must be stated rather than implied.

**Why salt/token is refused at all:** `t = md5(password + salt)` mathematically requires the server
to hold the password in recoverable form. Navidrome's own docs concede the consequence — *"Due to
limitations with the Subsonic API, Navidrome is unable to properly hash passwords and thus encrypts
them instead"* — with a key that by default ships in the source. Refusing the mechanism is what lets
Argon2id remain the only password storage in UsArr. A minority of ancient clients will not work;
document it prominently, because users will ask.

### OPDS

**HTTP Basic**, with the `client_credential` as the password and its label as the username. OPDS 1.2
readers (KOReader, Moon+ Reader, Librera) predominantly speak Basic and nothing else, and OPDS 2.0's
Authentication Document flow is not supported by the readers UsArr targets. Normative consequences:

1. The credential is sent on **every** request, base64-encoded, not hashed. Serve the OPDS surface
   over TLS or warn loudly.
2. **`Authorization` is stripped before any redirect UsArr emits.** Some download managers forward
   `Authorization` across redirects; without this rule that would send the *UsArr* credential to a
   *backend*.
3. Acquisition links point at UsArr's own `/stream/{usarr_id}` (§4), never at a backend URL.
4. ⚠️ Whether each named reader forwards credentials across redirects, and whether each follows a
   redirect at all, is **unverified**. It is a named test case in the milestone that ships OPDS.

---

## 3. The ID codec

```
usarr_id := crockford_base32_lower_unpadded(
                varint(instance_id) || kind_byte || enc_byte || native_id_bytes )
```

```go
const (
    encVerbatim = 0x00 // native_id_bytes are the backend's identifier as UTF-8
    encHex16    = 0x01 // native_id_bytes are 16 raw bytes; re-hex on decode
)

var kindByte = map[string]byte{
    "movie": 1, "series": 2, "season": 3, "episode": 4,
    "artist": 5, "album": 6, "track": 7,
    "book": 8, "comic": 9, "author": 10, "file": 11,
    "comic_issue": 12, // ADR-0030 — allocated in the same commit as "comic",
                       // before any client caches an id. See the note below.
    "person": 13,      // ADR-0033 — a creator entity reported under a name other than
                       // "author". Both 10 and 13 resolve to work.kind = 'person'.
}
```

**The map is keyed by the *remote* kind, not by `work.kind`, and the two vocabularies differ.**
`author` and `file` are remote kinds with no `work.kind` of the same name, and `work.kind = 'person'`
(ADR-0033) is reachable through either `author` (10) or `person` (13) depending on what the service
calls it. Decoding a `usarr_id` yields the remote kind, which is what `ux_sil` needs; the work kind
comes from the row it resolves to.

**`kind_byte` is load-bearing, not decoration.** The only unique index on `service_item_link` is
`(service_instance_id, remote_kind, remote_id)`. Without `remote_kind` at lookup time SQLite uses
only the leftmost column:

```
EXPLAIN QUERY PLAN
SELECT work_id, edition_id, remote_path FROM service_item_link
WHERE service_instance_id=? AND remote_id=?;
  -> SEARCH service_item_link USING INDEX ux_sil (service_instance_id=?)
```

— a range scan over every link on that instance, ~400k rows for a 2k-series Sonarr, on every
`stream`, every `getCoverArt` and every metadata call. Adding a `(service_instance_id, remote_id)`
unique index instead is **not** available: it asserts an invariant that is false for the \*Arrs,
where series 42 and episode 42 both exist.

**There is no `0x00` separator byte.** `varint` is self-delimiting, so a separator is decodable dead
weight.

**Length.** Base32 expands 8/5:

| Native id shape | Encoding | Bytes | Chars |
|---|---|---|---|
| 32-hex or UUID | `encHex16` | 1+1+1+16 = 19 | **31** |
| 26-char ULID | `encVerbatim` | 1+1+1+26 = 29 | **47** |
| `li_` + ~22 chars | `encVerbatim` | ~28 | **45** |
| 32-char non-hex | `encVerbatim` | 35 | **56** |

The ~48-character target is **met by the compaction rule for hex/UUID backends and missed for some
others**, and this document states the number rather than pretending otherwise. 🔍 The per-backend
native-ID shapes above are **inference from documentation, not verified against live instances**;
verify before freezing the codec, because **the codec is unchangeable once clients cache ids.**

**Stability invariant.** The ID is stable for a fixed `(instance, kind, native id)`. It is **not**
"a pure function of two values UsArr does not control" — `instance_id` is assigned by UsArr, and
*which* link is addressed for a unified item was a policy decision driven by an admin-editable
`priority`, so reordering two Navidromes would have changed every affected album's ID. The fix:

> Once a `service_item_link` has been addressed northbound it is **pinned**
> (`is_northbound_canonical = 1`). Priority changes do not move it. When the pinned instance is
> deleted, insert a `service_item_alias` row so old IDs still resolve to the surviving link.
> **Never silently rebind.**

**Unguessability is not a property of this scheme.** Base32 decodes in one line and \*Arr native ids
are small sequential integers, so the space is enumerable in a few thousand round trips.
"Opaque to the client" was a false requirement and is struck. Therefore:

> **Every resolution of a `usarr_id` — browse, metadata, `getCoverArt`, `stream`, OPDS acquisition —
> performs a `user_library_access` + permission check against the resolved
> `(service_instance_id, library_key, work.kind)` before any backend call.** On failure return the
> protocol-native not-found (Subsonic **70**), never 403, which would confirm existence. Resolution
> failures are rate-limited per credential and audit-logged, so enumeration is visible.

---

## 4. The stream path

**Design, and the reversal that produced it.** The earlier design defaulted to a `302` redirect
carrying "a backend-native ephemeral scoped token". That token does not exist:

- ✅ `jellyfin/jellyfin#10808` — *"Refactor 'Copy Stream URL' to not leak the user's session API
  key"* — is an **open issue proposing** per-object scoped keys, filed because Jellyfin's stream
  URLs today carry the user's full session token.
- ✅ Navidrome does not support OpenSubsonic `apiKeyAuthentication` in any release or in `master`
  (v0.63.2 latest; PRs #4022 and #5731 open, neither merged). Its advertised extensions are
  `transcodeOffset, formPost, songLyrics, indexBasedQueue, transcoding, playbackReport,
  topSongsByArtistId, sonicSimilarity` — no `apiKeyAuthentication`.

So a redirect would hand the client a **backend-user-equivalent credential**, usable outside UsArr's
authorization for its natural lifetime, defeating library visibility and parental controls. Two
further problems the redirect never solved: a minutes-lived token breaks **seek**, because most
Subsonic clients issue a new `Range` request to the *same* URL rather than re-calling `stream`; and
cookie-session backends cannot be redirected to at all.

**Therefore: UsArr proxies bytes for its own OpenSubsonic and OPDS surfaces, and links out for
video.**

```go
// Sketch. No transcoding, no format negotiation, no re-muxing — ever.
func (s *Server) streamRaw(w http.ResponseWriter, r *http.Request, t StreamTarget) error {
    req, _ := http.NewRequestWithContext(r.Context(), "GET", t.URL, nil)
    if rng := r.Header.Get("Range"); rng != "" {
        req.Header.Set("Range", rng)          // pass through verbatim
    }
    if ifr := r.Header.Get("If-Range"); ifr != "" {
        req.Header.Set("If-Range", ifr)
    }
    t.ApplyAuth(req)                          // backend credential, added here, never leaves
    resp, err := s.clientFor(t.InstanceID).Do(req)
    if err != nil { return err }
    defer resp.Body.Close()

    for _, h := range []string{"Content-Type", "Content-Length", "Content-Range",
                               "Accept-Ranges", "ETag", "Last-Modified"} {
        if v := resp.Header.Get(h); v != "" { w.Header().Set(h, v) }
    }
    w.Header().Set("Cache-Control", "private, no-transform")
    w.WriteHeader(resp.StatusCode)            // 200 or 206, whatever the backend said
    _, err = io.Copy(w, resp.Body)
    return err
}
```

**Rules:**

- Pass `Range` and `If-Range` through **verbatim**; mirror `Content-Range`, `Accept-Ranges`,
  `ETag` and the status code (200 or 206) from the backend. Do not attempt to satisfy ranges
  locally.
- **No transcoding.** If a Subsonic client sends `maxBitRate` or `format`, UsArr serves the original
  and reports the real format in the response fields. `transcodeOffset` is not advertised.
- **Never buffer the whole body.** `io.Copy` with a bounded buffer; the response is streamed.
- Backend credentials are applied inside the proxy and **never appear in anything the client sees**.
- **Video is not served here.** The northbound surfaces advertise no video stream endpoint; the SPA
  renders a deep link into the backend's own client.

**Cost, stated:** `Range` handling is a genuine source of subtle bugs, and this puts UsArr on the
byte path for audio. Mitigations: audio is ~1–5 Mb/s rather than a 60 Mb/s 4K remux; there is no
transcode; and the failure mode of a bug here is a client that cannot seek, not a leaked credential.

**Test matrix for the milestone that ships it:** seek mid-file; pause and resume after the token
TTL; a large file with repeated overlapping ranges; a client that re-requests the same URL from
scratch; a client that sends `Range: bytes=0-` on every request; and a backend that returns 200
where a 206 was requested.

### The `/stream/{usarr_id}` token

Used by SPA links and OPDS acquisition links, which have no session cookie.

```
token = base64url( nonce(8) || expiry_unix(8) ||
                   HMAC-SHA256(K, user_id ‖ client_credential_id ‖ usarr_id ‖
                                  instance_id ‖ expiry ‖ nonce)[:16] )
K = HKDF-SHA256(USARR_SECRET_KEY, info="usarr/stream-token/v1")
```

| Property | Value | Why |
|---|---|---|
| TTL | **default 120 s, maximum 600 s**, numeric and configurable | "Minutes, not hours" is not a specification |
| Nonce | 8 random bytes, cached for the TTL window | Replay defence within the window |
| Revocation | `client_credential_id` checked against `revoked_at` on **every** redemption | Otherwise "sign out my ex-roommate's Fire Stick" does not work for the surface that matters |
| Signing key | `HKDF(..., info="usarr/stream-token/v1")`, **distinct from the credential KEK** | Rotating one must not invalidate the other |
| Clock skew | ±60 s tolerated | The same skew reality as sync.md §3 |
| Headers | `Cache-Control: no-store`, `Referrer-Policy: no-referrer` | Keeps the token out of referrers and shared caches |

A short TTL does **not** break seek here, because the bytes come from UsArr: the client re-`Range`s
the same UsArr URL and UsArr re-authorizes from the session or the credential.

---

## 5. Capability negotiation and degradation

- `getOpenSubsonicExtensions` returns **UsArr's** implemented set — a fixed list determined by
  UsArr's code, not by the union of backends. `apiKeyAuthentication` always; others only where UsArr
  can honour them for every backend or degrade cleanly.
- **Advertise the union, degrade per item.** A client told "no playlists" because *one* of five
  backends lacks them is worse off than a client that tries and gets a clean error on the one album
  that cannot do it.
- Per-backend capabilities come from `Provider.Capabilities(ctx, instance)`, which **probes the live
  instance** rather than assuming from `kind` — a fork, an old version or a manifest-described
  service may differ. Cached on `service_instance.capabilities`, refreshed on health probe.
- Unsupported operation on the addressed item → the protocol's own error (70 / 50), never a 500.
- **Backend down:** browse and search still list everything, from the replica. Stream requests fail
  individually after a short deadline, never after a 30-second timeout. The circuit breaker means a
  down instance is *known* down and is skipped rather than re-probed per request.

---

## 6. Write-back (v1.0, not in the gateway milestone)

Recorded so the milestone that takes it on inherits the rules rather than rediscovering them. All of
it routes through the **durable write queue** (sync.md §5) with the server-derived idempotency key —
northbound protocols carry no idempotency field.

| Northbound call | Local effect | Southbound dispatch | Conflict rule |
|---|---|---|---|
| `star` / `unstar` | `tag_assignment` in the `user:` namespace, user-scoped | Queue → the addressed instance's favourite API | **UsArr wins.** Favourites are user data UsArr owns; the backend's copy is a mirror. |
| `scrobble(submission=false)` | Update `playback_state` | Fire-and-forget | No reconciliation; ephemeral by nature |
| `scrobble(submission=true)` | Increment play count, append `play_history` | Queue → backend scrobble | **Additive, never reconciled.** Play counts merge by union of events; taking a max from a backend that reset would silently delete history. |
| `setRating` | Local, user-scoped | Queue → backend | UsArr wins |
| Playlist create/update/reorder | Local `playlist` + `playlist_item` | Queue → backend **only if the playlist is single-backend** | **A cross-backend playlist cannot be written back.** Mark it `usarr_only` and say so in the UI — this is a real limitation of aggregation and it must be visible. |
| Playback position | Local `playback_state` | Two-way mirror | **The backend wins for its own media.** Audiobookshelf has the chapter model and cross-device session logic; UsArr mirrors and displays. |

General rule: **UsArr owns user-intent state (favourites, ratings, tags, requests); the backend owns
media-derived state (position, transcode session, file presence).**

`playlist.home_instance_id` is recomputed **on every `playlist_item` write** as *the single distinct
instance among members, else NULL*, and the UI is told when it flips to NULL. Nothing in the earlier
design computed it, which meant the "cannot be written back" rule had no input.
