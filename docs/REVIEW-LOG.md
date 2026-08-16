# Review log

Findings from the three adversarial reviews of 2026-08-16 (`review-correctness.md` C-…,
`review-security.md` S-…, `review-scope.md` P-…) that were **not** applied as written, each with the
reasoning and the evidence. Applied findings are not listed here; they are visible in the documents
themselves.

---

## Config-surface rebuttals

Covers `docs/CONFIGURATION.md`, `docs/DEVELOPMENT.md`, `docs/SETUP-CHECKLIST.md`, `.env.example`,
`Makefile`, `.gitignore`.

### 1. C-36's "empty string is treated as absent" — rejected, inverted to fail-closed

**Finding:** C-36 required *"Specify that an **empty-string** value is treated as absent, not as a
key."*

**Not applied.** An empty `USARR_SECRET_KEY` is now a **fatal startup error** naming the variable
(`CONFIGURATION.md` §3.2).

C-36 reached this from the right observation — the README's `USARR_SECRET_KEY=${USARR_SECRET_KEY}`
resolves to an empty string when the host variable is unset, so the documented "absent" branch never
fires — but drew the wrong conclusion from it. Treating empty as absent makes that compose file
*silently* generate a key on every fresh volume. The operator believes they are supplying a key from
their environment, UsArr generates a different one, and nobody discovers the divergence until a
restore. Fail-closed converts a silent misconfiguration into a startup message naming the variable,
which is the same class of fix C-36 itself asks for elsewhere (`USARR_FORWARD_AUTH_ENABLED` with an
empty allowlist being a startup error). The genuinely-absent case is unaffected: *unset* still
auto-generates, and `.env.example` ships the line commented out precisely so "unset" is the default
experience.

### 2. S-15's "keep `USARR_ALLOW_INSECURE_TLS_HOSTS` as a documented escape hatch" — rejected

**Finding:** S-15 required fix 1 said to replace blanket `verify_tls=0` with TOFU SPKI pinning and
*"keep `USARR_ALLOW_INSECURE_TLS_HOSTS` only as a documented escape hatch that logs a warning on
every use."*

**Not applied; the variable is deleted** (`CONFIGURATION.md` §7.1). C-37 is right that two mechanisms
for one property with no stated precedence is the defect, and S-15's own analysis explains why
keeping the global one is the wrong half to keep: scoping limits blast radius, it does not
authenticate anything. A per-instance `verify_tls` column **is already the escape hatch** — it is
per-connection, it is visible in the UI next to the instance it affects, and it can carry the
per-request warning S-15 wants. A second, environment-level mechanism adds nothing except a setting
that survives every UI cleanup, applies to hosts the admin has forgotten about, and cannot be
narrowed to one instance behind a shared hostname. Everything S-15 asks for (TOFU pinning, per-use
warning, the CA-import and plain-HTTP-over-a-trusted-network alternatives) is applied to the
per-instance mechanism instead.

### 3. P-15's keep-list retains `PUID`/`PGID`/`UMASK`; S-17 makes them impossible — resolved for S-17

**Findings in conflict:** P-15's v0.1 keep-list is *"…`PUID`/`PGID`/`TZ`/`UMASK`. That is 13."*
S-17 states that `distroless/static` has no shell and no `chown`, so a privilege-dropping entrypoint
requires starting as **root** in the process holding the credential vault, and offers option (a):
distroless, non-root `USER 65532`, **no** PUID/PGID, pre-chowned volume.

**Resolved for S-17 option (a).** `PUID`, `PGID` and `UMASK` are deleted; `TZ` is kept (distroless
static ships `tzdata`). P-15 was counting variables, not evaluating the base image, and S-17's
analysis is the one with a concrete failure attached. The cost is real and is documented rather than
hidden: users must pre-create and `chown 65532:65532` their volumes, or set `user:` in compose
(`CONFIGURATION.md` §2.4, and a copy-pasteable command in §8.1). The variable count lands at
**fourteen**, inside P-15's target either way.

Consequence worth recording for the architecture agent: this also settles S-17's healthcheck item.
The `HEALTHCHECK` targets the ordinary listener because v0.1 binds a real port; the loopback
health-only listener is a `tsnet`-milestone concern and is documented as **health-only, never admin**
(`CONFIGURATION.md` §9 rule 6).

### 4. S-14's key-relocation fix is necessary but not sufficient — applied, with the residual named

**Finding:** S-14 required moving the key to `$USARR_CONFIG_DIR/keys/secret.key` and excluding
`keys/` from the nightly job and the API backup.

**Applied, and stated as insufficient on its own.** The key is still *inside* the directory users
instinctively `tar`, so relocation alone converts a certainty into a near-certainty. The additional
change is to the **procedure**, which is what users actually follow: `CONFIGURATION.md` §6.1 leads
with `tar -czf backup.tgz --exclude='keys' /config` as the copy-pasteable command, marks
`tar /config` as a compromise rather than a backup in a 🚩 block, and §6.4 splits "moving to a new
host" into two deliberately separate steps instead of one `cp -r`.

**Rejected alternative:** a `USARR_KEY_DIR` variable defaulting outside `CONFIG_DIR`. There is no
correct default — anywhere outside the config volume is not persisted in a single-volume Docker
install, which is the most common deployment — so the variable's default value would be the thing
every user must remember to change. That renames the failure mode rather than removing it, and costs
a fifteenth variable to do it.

### 5. P-21's stated mechanism for the memory concern is factually stale — conclusion kept, reasoning replaced

**Finding:** P-21 argues the `< 80 MB` idle-RSS budget rests on Navidrome evidence that does not
transfer, because *"`ncruces/go-sqlite3` runs SQLite compiled to WASM inside wazero, with its own
linear-memory arena, its own page cache, and a JIT."* C-52 repeats the sandbox framing.

**Conclusion applied, premise corrected.** The driver has not run on wazero since March 2026.
Verified against `github.com/ncruces/go-sqlite3@v0.35.3`'s `go.mod` (fetched 2026-08-16): its
requires are `github.com/ncruces/go-sqlite3-wasm/v3`, `ncruces/julianday`, `ncruces/sort`,
`ncruces/wbt` and `golang.org/x/sys` — **no `tetratelabs/wazero` at any version**. There is no
sandbox, no linear-memory arena and no JIT; the SQLite C source is compiled to Wasm and then
translated to Go.

P-21's *recommendation* survives intact and is stronger for the correction: the memory profile of a
wasm2go-translated SQLite is still a different profile from cgo SQLite, so Navidrome's ~50 MB is
still not transferable evidence and the spike must still be run before the schema work.
`DEVELOPMENT.md` §1 now states the correct mechanism and the three consequences.

Related: C-06 credits the old `DEVELOPMENT.md` with *"already carr[ying] the correct, current
position."* It did not. It said the move was signalled *"in a future release"*, which was stale by
five months. The finding's substance is right; the compliment is not.

### 6. S-16.4's `pnpm audit` placement — moved from `lint-web` to `vuln`

**Finding:** *"Add `pnpm audit --audit-level=high` to `lint-web`."*

**Applied to the `vuln` target instead.** `pnpm audit` queries the registry. Putting it in `lint`
would make linting require network, breaking the invariant that everything except one govulncheck
call is hermetic (`DEVELOPMENT.md` §8). Both scans now live in `vuln`, which is gating, is part of
`check`, and is the single documented network step. `make check-offline` exists for the case where
there is no network at all.

Noted for completeness: applying S-16.2 (`vuln` gating and inside `check`) does mean `make check` is
no longer strictly offline. That trade is taken deliberately and is written down at the top of the
`Makefile` and in `DEVELOPMENT.md` §4 and §8, rather than left for someone to discover.

### 7. S-21.3's fake-seed-key mitigation — replaced with something stronger

**Finding:** *"Make the seed keys structurally non-real … and document that regenerating them from a
live stack is forbidden."*

**Applied only in part; the underlying practice is removed.** Committed seed volumes are now
gitignored entirely (`deploy/compose/seed/`), and `DEVELOPMENT.md` §7.3 replaces them with a
`make seed` script that writes the constant at run time. A documented prohibition does not survive
contact with the person who needs a working fixture at 23:00, and S-21's own argument is why: nothing
mechanically distinguishes a fake 32-hex key from a real one, so the failure is undetectable by both
the reviewer and the scanner. Removing the artifact removes the class. The structurally-obvious
constant S-21 asks for (`0000000000000000000000000000dead`) is kept — it is now generated by the
script rather than committed, which is where it does its work.

### 8. S-25's session-timeout defaults — applied as constants, not as variables

**Finding:** *"Consider defaulting idle to 72h"*, alongside adding a sudo-mode re-authentication
window.

**Both applied; neither is configurable.** `USARR_SESSION_IDLE_TIMEOUT` and
`USARR_SESSION_ABSOLUTE_TIMEOUT` are deleted per P-15, and the values are fixed at 72 h / 720 h with
sudo mode required for vault operations (`CONFIGURATION.md` §1, "What is deliberately *not*
configurable"). A tunable that exists so one user can lengthen their session is a support surface for
a setting whose only interesting direction is shorter, and shorter is already the default.
