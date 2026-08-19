#!/usr/bin/env bash
#
# bookorbit-filter-and-shape-probe.sh — is the replica a SUBSET, and is a
#                                       BookOrbit library one media kind or two?
#
# UsArr · ADR-0052 · internal/bookorbit/scope.go · companion to the two Kavita probes
#
# WHY THIS EXISTS
#   Two open questions need one trip to a live BookOrbit, and they are bundled
#   into one script so it is run once rather than twice.
#
#     QUESTION A — CONTENT FILTERS, and this is the important one.
#        `LibraryService.findAll` and `BookService.queryForLibrary` both narrow
#        their results on `user.contentFilters` for any non-superuser. A shared
#        service account carrying a filter therefore yields a SILENTLY SUBSET
#        replica: UsArr imports what it is shown, and nothing on any screen says
#        anything is missing.
#
#        Put the way the project's PM put it, because the framing is the point:
#        UsArr CANNOT TELL THE DIFFERENCE BETWEEN A BOOK YOU DO NOT HAVE AND A
#        BOOK YOUR SERVICE ACCOUNT CANNOT SEE. That is not a cosmetic honesty
#        problem. It breaks the replica thesis in the least noticeable way
#        possible — every screen renders, every count looks plausible, and the
#        library is quietly smaller than the one on disk.
#
#        If it cannot be closed, the honest response is DETECT AND SAY SO. Never
#        hope the filter is empty. This probe is the detector's field trial.
#
#     QUESTION B — MIXED LIBRARIES. Does a single BookOrbit library contain both
#        comic-format and prose books, or are they separate libraries? UsArr's
#        library record holds exactly one kind, so a mixed container means TWO
#        UsArr libraries from ONE upstream container. That is a design change,
#        not a detail, and it should be settled by a number rather than a guess.
#
#   Also captured, because it costs one more call and this is run once: whether
#   the account is a superuser, its `provisioningMethod`, and its permission
#   count and names — the same three facts `internal/bookorbit/scope.go` grades,
#   so a run can be compared against what UsArr itself will say.
#
# WHAT IT SENDS — read-only
#   GETs, plus the POST that BookOrbit REQUIRES for its own paged book query
#   (`POST /api/v1/libraries/{id}/books` is a read; the body carries the filter),
#   plus the one POST that mints a token (`POST /api/v1/auth/login`). Nothing
#   else. No write, no scan, no metadata refresh, no upload, no setting change.
#   Every book count below is obtained by asking the server to COUNT, with
#   `size: 1`, rather than by downloading pages of books.
#
# WHAT IT PRINTS — this output is meant to be safe to paste in public
#   Counts, classifications, HTTP status codes and byte sizes. It never prints
#   your base URL, your username, your password, the access token, a library
#   name, a book title, a tag or genre name, or a filesystem path. Libraries are
#   identified by SLOT ("library 2"), never by id and never by name.
#
#   That is enforced mechanically, not by care. Every response body is passed
#   through a string-aware AWK pass (`blank.awk`) BEFORE anything parses or
#   prints it. That pass deletes every JSON string VALUE outright and replaces
#   any KEY that is not plain `[A-Za-z0-9_]` with `"?"`. So a library called
#   `Dad's "Secret" ] } Comics` and a path of `/mnt/media/x]y` are gone before
#   the first `grep` runs — they cannot leak, and they cannot break the parse
#   either. It was drilled against exactly those two strings.
#
#   THREE DELIBERATE EXCEPTIONS, each printed because a count is not the answer:
#     · `provisioningMethod`, matched against BookOrbit's own four-value CHECK
#       constraint (`local`, `manual`, `oidc`, `shared`; schema/auth.ts:63).
#       Anything else is withheld and reported by length. The value names an
#       account TYPE, not an account.
#     · PERMISSION NAMES, matched against the 23-member `Permission` enum. It is
#       a fixed published vocabulary — `manage_users`, `opds_access` — which
#       names a capability and never a person or a title. "7 permissions" cannot
#       be compared against UsArr's scope grader; the names can. Anything not in
#       the enum is counted, never printed.
#     · FILE-FORMAT EXTENSIONS, matched against `DEFAULT_FORMAT_PRIORITY` plus
#       `cbx`. `epub` and `cbz` name a container format, not a book. Anything
#       else is counted as "unrecognised", never printed.
#
#   It writes no file anywhere except a private temp directory it deletes on
#   exit, and it never writes your password or the access token to any file.
#   Your PASSWORD never appears in any process's argv either: it is fed to curl
#   on STDIN (`--data-binary @-`), and the access token is fed to curl on stdin
#   too (`-H @-`). Neither is ever an argument.
#
# ENDPOINTS, VERIFIED AGAINST SOURCE AT bookorbit/bookorbit@73b7877 BEFORE THIS
# SCRIPT WAS WRITTEN — not guessed. Paths are under the global prefix `api/v1`
# (`server/src/main.ts:61`).
#
#   POST /api/v1/auth/login              auth.controller.ts:76, @Public.
#                                        Body {username,password} — login.dto.ts.
#                                        Returns {accessToken,user}. The global
#                                        ValidationPipe is `forbidNonWhitelisted`
#                                        (main.ts:65-71), so it is sent with
#                                        those two keys and no others.
#   GET  /api/v1/health                  health.controller.ts:9,16, @Public.
#   GET  /api/v1/auth/me                 auth.controller.ts:96. Body is
#                                        AuthService.buildUserResponse
#                                        (auth.service.ts:297-311): isSuperuser,
#                                        provisioningMethod, permissions — and
#                                        `["*"]` substituted for a superuser.
#                                        NOTE it does NOT carry contentFilters,
#                                        which is why the next call exists.
#   GET  /api/v1/users/me/content-filters
#                                        user.controller.ts:309. No
#                                        @RequirePermission. Returns
#                                        ContentFilterRulesWithNames —
#                                        includeTags/excludeTags/includeGenres/
#                                        excludeGenres, each an array of
#                                        {id,name} (content-filter.repository.ts
#                                        :35-63). THE NAMES ARE TAG AND GENRE
#                                        NAMES; they are blanked before the count
#                                        is taken and are never printed.
#   GET  /api/v1/libraries               library.controller.ts:31 ->
#                                        LibraryService.findAll
#                                        (library.service.ts:71-93). This is one
#                                        of the two narrowing sites: a
#                                        non-superuser is served
#                                        libraryRepo.findAllForUser(id,
#                                        contentFilters), whose `bookCount` is a
#                                        LEFT JOIN carrying the filter clauses
#                                        (library.repository.ts:30-52). So the
#                                        bookCount you see here is ALREADY
#                                        narrowed.
#   GET  /api/v1/libraries/{id}/stats    library.controller.ts:108. Guarded by
#                                        @RequireLibraryAccess('viewer') and NO
#                                        @RequirePermission, so a shared account
#                                        with an empty permission set can call
#                                        it. It calls libraryRepo.getStats(id)
#                                        (library.repository.ts:150-179) with NO
#                                        user and NO content filters at all.
#                                        THIS IS THE GROUND TRUTH: `totalBooks`
#                                        counts every book with status='present'
#                                        in that library, filtered or not, and
#                                        `formatCounts` groups the PRIMARY file's
#                                        format the same way.
#   POST /api/v1/libraries/{id}/books    library.controller.ts:42 ->
#                                        BookService.queryForLibrary
#                                        (book.service.ts:1033-1046). The other
#                                        narrowing site: `contentFilters:
#                                        this.isSuperuser(user) ? undefined :
#                                        user.contentFilters`. Same endpoint and
#                                        same body shape the UsArr adapter pages
#                                        with. Returns {items,total,page,size}
#                                        (types/book.ts:315).
#
#   The body is validated by BookQueryPipe (book/pipes/book-query.pipe.ts:11-51).
#   `pagination.size` is `.min(1).max(200)` and `pagination.page` is 0-BASED and
#   `.min(0)`. A `size` above 200 is REJECTED WITH 400, not clamped — this probe
#   fires that boundary once on purpose rather than trusting the reading, and
#   otherwise never asks for more than `size: 1`, because it wants counts and
#   not books.
#
#   The filters used below are `format includesAny [...]` and `fileAvailability
#   isPresent`, both declared in FIELD_OPERATORS (types/query.ts:101-130).
#   `format includesAny` compiles to `exists (select 1 from book_files where
#   book_id = books.id and format in (...))` (book-query-builder.service.ts:618
#   -639), so it counts a book that has AT LEAST ONE file of that kind — which is
#   the right question for B, and means a book holding both an epub and a cbz is
#   counted under both. `fileAvailability isPresent` compiles to
#   `books.status = 'present'` (ibid.:785-794), which is the SAME predicate
#   `getStats` counts on, and that is what makes the A comparison exact.
#
# HOW QUESTION A IS ACTUALLY ANSWERED, since it turned out to be answerable
#   The two counts below are taken over the identical predicate and differ in
#   exactly one thing — whether the content filter is applied:
#
#     TRUE    GET  /libraries/{id}/stats  -> totalBooks   (no filter, ever)
#     VISIBLE POST /libraries/{id}/books  -> total        (filtered for you)
#             with filter `fileAvailability isPresent`, size 1
#
#   A shortfall is therefore not an inference. It is a subtraction, and the
#   number it produces is the number of books UsArr would silently never learn
#   about. Two honest caveats, both reported inline rather than buried:
#     · Without the `isPresent` rule the two counts would NOT be comparable:
#       getStats counts `status='present'` while the book query's floor is
#       `status <> 'processing'` (book.repository.ts:530-532), which also admits
#       `missing`. The rule removes that asymmetry instead of explaining it away.
#     · `formatCounts` INNER JOINs `books.primary_file_id`, so a book with no
#       primary file is in `totalBooks` and in no format bucket. The gap is
#       reported as its own line, never silently absorbed.
#
#   WHAT THIS PROBE CANNOT ANSWER, and does not pretend to: whether a library
#   exists that this account is not shown AT ALL. LibraryAccessGuard throws the
#   same ForbiddenException for "library exists, no access row" and for "no such
#   library" (guards/library-access.guard.ts:39), so for a non-superuser both are
#   an indistinguishable 403 and no read-only scan can separate them. That is
#   reported as UNANSWERED with the reason, which is the whole point of saying it
#   here.
#
# USAGE
#   BOOKORBIT_URL=http://127.0.0.1:3000 BOOKORBIT_USERNAME=usarr \
#     ./bookorbit-filter-and-shape-probe.sh
#   BOOKORBIT_URL=... ./bookorbit-filter-and-shape-probe.sh --username usarr
#
#   Run it as the SERVICE ACCOUNT UsArr will use, not as your admin account. A
#   run as an admin answers a question nobody asked: superusers bypass content
#   filters at every site (`user.isSuperuser ? undefined : user.contentFilters`)
#   and cannot be given one at all (user.service.ts:423-425), so an admin run
#   reports a clean bill of health for a credential UsArr should not be storing.
#   The probe says so on its own when it sees `isSuperuser: true`.
#
#   The password is read from BOOKORBIT_PASSWORD, or prompted for silently if
#   that is unset. There is deliberately no --password flag: a credential on a
#   command line lands in your shell history and in the process table.
#
# REQUIREMENTS
#   bash and curl. THAT IS ALL. `jq` is NOT used anywhere in this script and is
#   not consulted for anything. The earlier Kavita volume-walk probe was crippled
#   on a box without it and came back UNANSWERED on half its questions; this one
#   was designed jq-absent from the first line, and every structure it needs is
#   read by the three AWK passes below, which are POSIX awk and nothing more.
#
# Licensed under AGPL-3.0, with the rest of UsArr.

set -euo pipefail

readonly PROBE_VERSION="1"
readonly SOURCE="bookorbit/bookorbit@73b7877"

TIMEOUT=30
MAX_LIBRARIES=50

usage() {
	cat <<'USAGE'
bookorbit-filter-and-shape-probe.sh — is the replica a SUBSET, and is a
                                      BookOrbit library one media kind or two?

  BOOKORBIT_URL=http://127.0.0.1:3000 BOOKORBIT_USERNAME=usarr \
    ./bookorbit-filter-and-shape-probe.sh
  BOOKORBIT_URL=... ./bookorbit-filter-and-shape-probe.sh --username usarr

  --username U     BookOrbit username, if you would rather not export it
  --url U          BookOrbit base URL, if you would rather not export it
  --max-libraries N  stop after N libraries (default 50)
  --timeout S      per-request timeout in seconds (default 30)
  -h, --help       this text

Run it as the SERVICE ACCOUNT UsArr will use, not as your admin account: a
superuser bypasses content filters entirely, so an admin run answers a question
nobody asked. The probe says so if it sees one.

The password comes from BOOKORBIT_PASSWORD, or a silent prompt. There is no
--password flag on purpose: a credential on a command line lands in your shell
history and in the process table.

Read-only: GETs, plus the POST BookOrbit requires for its own paged book query
and the POST that mints a token. Needs only bash and curl — no jq. Prints
counts, classifications, status codes and sizes, and never the URL, the
username, the password, the token, a library name, a book title, a tag or genre
name, or a file path. Safe to paste in public. The three deliberate exceptions
-- provisioningMethod, permission names and file-format extensions -- and why
each is safe are in the comment block at the top of this file.
USAGE
	exit "${1:-0}"
}

die() {
	printf 'probe: %s\n' "$*" >&2
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
	--username)
		[ $# -ge 2 ] || die "--username needs a value"
		BOOKORBIT_USERNAME="$2"
		shift 2
		;;
	--username=*)
		BOOKORBIT_USERNAME="${1#*=}"
		shift
		;;
	--url)
		[ $# -ge 2 ] || die "--url needs a base URL"
		BOOKORBIT_URL="$2"
		shift 2
		;;
	--url=*)
		BOOKORBIT_URL="${1#*=}"
		shift
		;;
	--max-libraries)
		[ $# -ge 2 ] || die "--max-libraries needs a number"
		MAX_LIBRARIES="$2"
		shift 2
		;;
	--max-libraries=*)
		MAX_LIBRARIES="${1#*=}"
		shift
		;;
	--timeout)
		[ $# -ge 2 ] || die "--timeout needs a number of seconds"
		TIMEOUT="$2"
		shift 2
		;;
	--timeout=*)
		TIMEOUT="${1#*=}"
		shift
		;;
	--password | --password=* | --pass | --pass=*)
		die "there is no --password flag on purpose: a credential on a command line lands in your shell history and in the process table. Use the BOOKORBIT_PASSWORD environment variable, or let the probe prompt you."
		;;
	-h | --help) usage 0 ;;
	*) die "unknown argument: $1 (try --help)" ;;
	esac
done

case "$TIMEOUT" in '' | *[!0-9]*) die "--timeout must be a positive integer" ;; esac
case "$MAX_LIBRARIES" in '' | *[!0-9]*) die "--max-libraries must be a positive integer" ;; esac
[ "$MAX_LIBRARIES" -ge 1 ] || die "--max-libraries must be at least 1"

command -v curl >/dev/null 2>&1 || die "curl is required and is not on PATH"
command -v awk >/dev/null 2>&1 || die "awk is required and is not on PATH"

BASE="${BOOKORBIT_URL:-}"
[ -n "$BASE" ] || die "set BOOKORBIT_URL (or pass --url), e.g. http://127.0.0.1:3000"
BASE="${BASE%/}"
case "$BASE" in
http://* | https://*) ;;
*) die "BOOKORBIT_URL must start with http:// or https://" ;;
esac

USERNAME="${BOOKORBIT_USERNAME:-}"
if [ -z "$USERNAME" ]; then
	[ -t 0 ] || die "BOOKORBIT_USERNAME is unset and stdin is not a terminal, so the probe cannot prompt for it"
	printf 'BookOrbit username (never printed): ' >&2
	IFS= read -r USERNAME
fi
[ -n "$USERNAME" ] || die "no username supplied"

PASSWORD="${BOOKORBIT_PASSWORD:-}"
if [ -z "$PASSWORD" ]; then
	[ -t 0 ] || die "BOOKORBIT_PASSWORD is unset and stdin is not a terminal, so the probe cannot prompt for it"
	printf 'BookOrbit password (not echoed, not stored, not printed): ' >&2
	IFS= read -rs PASSWORD
	printf '\n' >&2
fi
[ -n "$PASSWORD" ] || die "no password supplied"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/bookorbit-probe.XXXXXX")"
chmod 700 "$WORK"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------------------
# The three AWK passes. These are the whole reason this probe needs no jq, and
# the reason its output is safe by construction rather than by care.
# ---------------------------------------------------------------------------

# blank.awk — the redactor. A string-aware scan that honours backslash escapes,
# so a `\"` inside a name does not end the string early. Every string VALUE
# becomes `""`. Every KEY survives only if it is plain [A-Za-z0-9_]; anything
# else becomes `"?"`, because a key is emitted verbatim and a key carrying `{`,
# `}`, `[` or `]` would break every depth walk downstream AND could leak. After
# this pass no brace, bracket, comma or colon in the buffer is inside a string,
# which is what makes the two depth walks below correct rather than lucky.
#
# It also drops every whitespace character OUTSIDE a string, which is not
# cosmetic: it means everything downstream may assume `"key":value` with no gap,
# so one pretty-printing reverse proxy in front of BookOrbit cannot silently turn
# every count in this probe into a zero. The drill that built this script found
# exactly that failure against a `json.dumps` serializer.
cat >"$WORK/blank.awk" <<'AWK'
{ buf = buf $0 "\n" }
END {
	n = length(buf)
	i = 1
	while (i <= n) {
		c = substr(buf, i, 1)
		if (c != "\"") {
			if (c != " " && c != "\t" && c != "\r" && c != "\n") printf "%s", c
			i++
			continue
		}
		start = i
		i++
		while (i <= n) {
			ch = substr(buf, i, 1)
			if (ch == "\\") { i += 2; continue }
			i++
			if (ch == "\"") break
		}
		j = i
		while (j <= n && substr(buf, j, 1) ~ /^[ \t\r\n]$/) j++
		if (substr(buf, j, 1) != ":") { printf "\"\""; continue }
		inner = substr(buf, start + 1, i - start - 2)
		if (inner ~ /^[A-Za-z0-9_]+$/) printf "\"%s\"", inner
		else printf "\"?\""
	}
}
AWK

# depth.awk — emits only the scalar key/value pairs that sit at nesting depth
# `want`, one line per container that closes back to want-1. `want=1` reads the
# top-level scalars of an object; `want=2` reads one array element per line.
# Nested containers are dropped entirely, which is what stops a library's
# `folders[].id` from being mistaken for the library's own `id`.
cat >"$WORK/depth.awk" <<'AWK'
{ buf = buf $0 }
END {
	n = length(buf)
	depth = 0
	for (i = 1; i <= n; i++) {
		c = substr(buf, i, 1)
		if (c == "[" || c == "{") { depth++; continue }
		if (c == "]" || c == "}") { depth--; if (depth == want - 1) printf "\n"; continue }
		if (depth == want) printf "%s", c
	}
}
AWK

# member.awk — prints the body of the container held by `key`, object or array
# alike, by walking to its matching close. Used for one job only: counting the
# elements of each content-filter bucket without ever touching a tag name.
cat >"$WORK/member.awk" <<'AWK'
{ buf = buf $0 }
END {
	needle = "\"" key "\":"
	p = index(buf, needle)
	if (p == 0) exit 0
	i = p + length(needle)
	n = length(buf)
	while (i <= n && substr(buf, i, 1) != "{" && substr(buf, i, 1) != "[") i++
	if (i > n) exit 0
	depth = 0
	for (; i <= n; i++) {
		c = substr(buf, i, 1)
		if (c == "{" || c == "[") { depth++; if (depth == 1) continue }
		if (c == "}" || c == "]") { depth--; if (depth == 0) break }
		printf "%s", c
	}
	printf "\n"
}
AWK

# strings.awk — the ONE pass that reads raw string values, and the only place in
# this probe where an unredacted string is looked at. It is used for exactly one
# array, `permissions`, whose members are a closed 23-name enum, and every value
# it yields is matched against that enum by the caller before anything is
# printed. It requires the key at object depth 1 so a `permissions` key nested
# inside the free-form `settings` blob cannot be picked up by mistake.
cat >"$WORK/strings.awk" <<'AWK'
{ buf = buf $0 }
END {
	n = length(buf)
	needle = "\"" key "\""
	state = 0
	gdepth = 0
	depth = 0
	i = 1
	while (i <= n) {
		c = substr(buf, i, 1)
		if (c == "\"") {
			start = i
			i++
			while (i <= n) {
				ch = substr(buf, i, 1)
				if (ch == "\\") { i += 2; continue }
				i++
				if (ch == "\"") break
			}
			if (state == 1 && depth == 1) { print substr(buf, start + 1, i - start - 2); continue }
			if (state == 0 && gdepth == 1 && substr(buf, start, i - start) == needle) {
				j = i
				while (j <= n && substr(buf, j, 1) ~ /^[ \t\r\n]$/) j++
				if (substr(buf, j, 1) == ":") {
					j++
					while (j <= n && substr(buf, j, 1) ~ /^[ \t\r\n]$/) j++
					if (substr(buf, j, 1) == "[") { state = 1; depth = 1; i = j + 1 }
				}
			}
			continue
		}
		if (state == 1) {
			if (c == "[" || c == "{") depth++
			else if (c == "]" || c == "}") { depth--; if (depth == 0) break }
		} else {
			if (c == "[" || c == "{") gdepth++
			else if (c == "]" || c == "}") gdepth--
		}
		i++
	}
}
AWK

# ---------------------------------------------------------------------------
# HTTP. Every call goes through here, so exactly one place holds the token and
# no error path can print a URL or a header.
# ---------------------------------------------------------------------------

TOKEN=""

# No --fail-with-body anywhere below: it makes curl exit non-zero on a 4xx, which
# would throw away the very status code that tells the operator what to fix. That
# defect was found in the Kavita volume probe's drill and is not repeated here.

# bo_get <path> <body-file> <hdr-file> -> "<http_code> <seconds> <bytes>".
# The token goes to curl on STDIN via `-H @-`, so it is never in argv.
bo_get() {
	local path="$1" body="$2" hdr="$3" out
	if ! out="$(printf 'Authorization: Bearer %s\nAccept: application/json\n' "$TOKEN" |
		curl -sS -H @- -o "$body" -w '%{http_code} %{time_total} %{size_download}' \
			--max-time "$TIMEOUT" -D "$hdr" "${BASE}${path}" 2>"$WORK/curlerr")"; then
		# curl's own message can carry the URL and the body can carry library
		# data. Neither is printed; only the code and the classification are.
		printf '000 0 0\n'
		return 0
	fi
	printf '%s\n' "$out"
}

# bo_post <path> <json-body> <body-file> <hdr-file>. The JSON body here is only
# ever a filter and a page size, never a secret, so it goes through a 0700 temp
# file; the token still goes on stdin.
bo_post() {
	local path="$1" payload="$2" body="$3" hdr="$4" out
	printf '%s' "$payload" >"$WORK/post.json"
	if ! out="$(printf 'Authorization: Bearer %s\nAccept: application/json\nContent-Type: application/json\n' "$TOKEN" |
		curl -sS -H @- --data-binary "@$WORK/post.json" \
			-o "$body" -w '%{http_code} %{time_total} %{size_download}' \
			--max-time "$TIMEOUT" -D "$hdr" "${BASE}${path}" 2>"$WORK/curlerr")"; then
		printf '000 0 0\n'
		return 0
	fi
	printf '%s\n' "$out"
}

code_of() { printf '%s' "$1" | cut -d' ' -f1; }
size_of() { printf '%s' "$1" | cut -d' ' -f3; }

explain_code() {
	case "$1" in
	000) printf 'no HTTP response (connection, TLS or timeout). The probe does not print curl'"'"'s message because it carries the URL.' ;;
	400) printf 'bad request — the endpoint exists but rejected the body.' ;;
	401) printf 'unauthenticated — the credential was rejected, or the token expired mid-run (BookOrbit'"'"'s default access-token lifetime is short).' ;;
	403) printf 'forbidden — authenticated, but this account lacks the permission or the library-access row this route needs.' ;;
	404) printf 'not found — wrong base URL, a reverse-proxy path prefix, or an endpoint this release does not have.' ;;
	429) printf 'rate limited.' ;;
	500) printf 'server error — BookOrbit threw. Its own log has the detail.' ;;
	*) printf 'HTTP %s' "$1" ;;
	esac
}

# blank <raw-file> <out-file> — redact, always, before anything else touches it.
blank() { awk -f "$WORK/blank.awk" "$1" >"$2"; }

# num <file> <key> — first integer value of <key> in an already-blanked,
# already-depth-filtered file. `|| true` guards `set -o pipefail`: an absent key
# is an ANSWER here, not a failure.
num() {
	{ grep -o "\"$2\":[[:space:]]*-\{0,1\}[0-9]\{1,\}" "$1" || true; } |
		head -n1 | sed 's/.*://; s/[[:space:]]//g'
}

# ---------------------------------------------------------------------------
# Preamble — printed before the first packet leaves.
# ---------------------------------------------------------------------------

printf '=== UsArr BookOrbit content-filter and library-shape probe v%s ===\n' "$PROBE_VERSION"
printf 'READ-ONLY. It sends GETs, the POST BookOrbit requires for its own paged book\n'
printf 'query, and the POST that mints a token. It writes nothing, scans nothing,\n'
printf 'refreshes no metadata and changes no setting. Every count below is obtained by\n'
printf 'asking the server to COUNT (size: 1), not by downloading books.\n'
printf '\n'
printf 'Output is safe to paste in public. Every response body is redacted by a\n'
printf 'string-aware AWK pass BEFORE anything parses or prints it: string values are\n'
printf 'deleted and non-identifier keys become "?". No URL, no username, no password,\n'
printf 'no token, no library name, no book title, no tag or genre name, no file path.\n'
printf 'Libraries are identified by SLOT ("library 2"), never by id and never by name.\n'
printf '\n'
printf 'Three values ARE printed, on purpose, because a count does not answer the\n'
printf 'question: provisioningMethod (a four-value account TYPE), permission names (a\n'
printf 'fixed 23-member enum naming capabilities), and file-format extensions (epub,\n'
printf 'cbz). None of the three names a person, a library or a title. Anything outside\n'
printf 'those published vocabularies is withheld and reported as a count.\n'
printf '\n'
printf 'Needs only bash and curl. jq is not used anywhere in this script.\n'
printf 'Endpoints verified against %s before this script was written.\n' "$SOURCE"
printf '\n'

# ---------------------------------------------------------------------------
# 0. Reachability and login.
# ---------------------------------------------------------------------------

printf -- '--- 0. reachability and login ---\n'

r="$(bo_get "/api/v1/health" "$WORK/health" "$WORK/h0")"
hc="$(code_of "$r")"
if [ "$hc" = "200" ]; then
	printf 'GET  /api/v1/health                  : 200 OK (@Public, no credential needed)\n'
else
	printf 'GET  /api/v1/health                  : %s — %s\n' "$hc" "$(explain_code "$hc")"
fi

# The password is fed to curl on STDIN and never becomes an argument. `printf` is
# a bash builtin, so neither it nor the escaper below puts the password in any
# process's argv, and it is never written to a file.
json_escape() { printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

if printf '%s%s' "$USERNAME" "$PASSWORD" | LC_ALL=C grep -q '[[:cntrl:]]'; then
	die "the username or password contains a control character; the probe will not risk mangling it into JSON"
fi

LOGIN_BODY="{\"username\":\"$(json_escape "$USERNAME")\",\"password\":\"$(json_escape "$PASSWORD")\"}"

if ! r="$(printf '%s' "$LOGIN_BODY" |
	curl -sS -H 'Content-Type: application/json' -H 'Accept: application/json' \
		--data-binary @- -o "$WORK/login" -w '%{http_code} %{time_total} %{size_download}' \
		--max-time "$TIMEOUT" -D "$WORK/hl" "${BASE}/api/v1/auth/login" 2>"$WORK/curlerr")"; then
	r='000 0 0'
fi
LOGIN_BODY=""
PASSWORD=""
hc="$(code_of "$r")"
if [ "$hc" != "200" ]; then
	printf 'POST /api/v1/auth/login              : %s — %s\n' "$hc" "$(explain_code "$hc")"
	printf '\n=== VERDICT ===\n'
	printf 'INCONCLUSIVE — could not authenticate. Nothing below ran, and nothing below\n'
	printf 'should be read as a finding. This is not evidence about content filters.\n'
	exit 1
fi

# A JWT is base64url plus dots: it contains no quote and no backslash, so this
# is exact rather than approximate. The token is read from the RAW body -- the
# one thing that must not be redacted -- and the raw body is destroyed on the
# next line, before any other parse touches it.
TOKEN="$({ grep -o '"accessToken"[[:space:]]*:[[:space:]]*"[^"]*"' "$WORK/login" || true; } |
	head -n1 | sed 's/.*"\([^"]*\)"$/\1/')"
: >"$WORK/login"

if [ -z "$TOKEN" ]; then
	printf 'POST /api/v1/auth/login              : 200, but no accessToken in the body.\n'
	printf '\n=== VERDICT ===\n'
	printf 'INCONCLUSIVE — a 200 with no accessToken is not a BookOrbit answering. Check\n'
	printf 'the base URL is BookOrbit and not something in front of it.\n'
	exit 1
fi
printf 'POST /api/v1/auth/login              : 200 OK (accessToken received; never printed)\n'

r="$(bo_get "/api/v1/auth/me" "$WORK/me.raw" "$WORK/hm")"
hc="$(code_of "$r")"
if [ "$hc" != "200" ]; then
	printf 'GET  /api/v1/auth/me                 : %s — %s\n' "$hc" "$(explain_code "$hc")"
	printf '\n=== VERDICT ===\n'
	printf 'INCONCLUSIVE — the token minted but would not read. Nothing below ran.\n'
	exit 1
fi
printf 'GET  /api/v1/auth/me                 : 200 OK (bearer token accepted)\n'
printf '\n'

# ---------------------------------------------------------------------------
# 1. The account. Cheap, and the same three facts internal/bookorbit/scope.go
#    grades, so this run can be compared line for line against what UsArr says.
# ---------------------------------------------------------------------------

printf -- '--- 1. the account UsArr would store ---\n'

blank "$WORK/me.raw" "$WORK/me"

IS_SUPERUSER=unknown
if grep -q '"isSuperuser":[[:space:]]*true' "$WORK/me"; then
	IS_SUPERUSER=yes
elif grep -q '"isSuperuser":[[:space:]]*false' "$WORK/me"; then
	IS_SUPERUSER=no
fi
printf 'isSuperuser                          : %s\n' "$IS_SUPERUSER"

# provisioningMethod is read from the RAW body and matched against BookOrbit's
# own CHECK constraint. Anything outside those four is withheld: the constraint
# is the vocabulary, so a value outside it is not a value this probe understands
# and is not one it will print.
PROV_RAW="$({ grep -o '"provisioningMethod"[[:space:]]*:[[:space:]]*"[^"]*"' "$WORK/me.raw" || true; } |
	head -n1 | sed 's/.*"\([^"]*\)"$/\1/')"
case "$PROV_RAW" in
local | manual | oidc | shared) PROV="$PROV_RAW" ;;
'') PROV='ABSENT' ;;
*) PROV="OUTSIDE THE CHECK CONSTRAINT (withheld, ${#PROV_RAW} chars)" ;;
esac
printf 'provisioningMethod                   : %s\n' "$PROV"

# The 23 members of Permission (packages/types/src/permissions.ts@73b7877), plus
# the literal "*" that buildUserResponse substitutes for a superuser. Every value
# read off the wire is matched against this list before it is printed; anything
# unmatched is counted and never shown, so a 24th permission upstream shows up
# here as an unrecognised count rather than as unredacted text.
PERM_VOCAB=" library_download library_upload library_edit_metadata library_delete_books book_dock_access demo_restricted kobo_sync koreader_sync hardcover_sync readwise_sync storygraph_sync opds_access email_send manage_email manage_libraries manage_metadata_config manage_icons manage_app_settings manage_book_dock manage_users view_user_activity view_audit_log notification_access "

# The elevated set is UsArr's own, transcribed from internal/bookorbit/scope.go
# so a run can be compared against the Services screen without a second lookup.
PERM_ELEVATED=" library_download library_upload library_edit_metadata library_delete_books email_send manage_email manage_libraries manage_metadata_config manage_icons manage_app_settings manage_book_dock manage_users view_user_activity view_audit_log "

awk -v key=permissions -f "$WORK/strings.awk" "$WORK/me.raw" >"$WORK/perms" || true
PERM_N=0
PERM_KNOWN=0
PERM_UNKNOWN=0
PERM_ELEV=0
PERM_STAR=0
PERM_LIST=""
PERM_ELEV_LIST=""
while IFS= read -r p; do
	[ -n "$p" ] || continue
	PERM_N=$((PERM_N + 1))
	if [ "$p" = "*" ]; then
		PERM_STAR=1
		continue
	fi
	# The shape test comes FIRST and is not decoration: `$p` is interpolated into
	# a case PATTERN below, so a value off the wire carrying `*` or `?` would
	# match names it is not. Anything that is not a plain lowercase-underscore
	# token is counted as unrecognised and never printed.
	case "$p" in
	'' | *[!a-z_]*)
		PERM_UNKNOWN=$((PERM_UNKNOWN + 1))
		continue
		;;
	esac
	case "$PERM_VOCAB" in
	*" $p "*)
		PERM_KNOWN=$((PERM_KNOWN + 1))
		PERM_LIST="$PERM_LIST $p"
		case "$PERM_ELEVATED" in
		*" $p "*)
			PERM_ELEV=$((PERM_ELEV + 1))
			PERM_ELEV_LIST="$PERM_ELEV_LIST $p"
			;;
		esac
		;;
	*) PERM_UNKNOWN=$((PERM_UNKNOWN + 1)) ;;
	esac
done <"$WORK/perms"

if [ "$PERM_STAR" -eq 1 ]; then
	printf 'permissions                          : ["*"] — the superuser substitution\n'
	printf '                                       (auth.service.ts:309), not a real set.\n'
else
	printf 'permissions                          : %s total (%s in the 23-member enum, %s not)\n' \
		"$PERM_N" "$PERM_KNOWN" "$PERM_UNKNOWN"
	if [ -n "$PERM_LIST" ]; then
		printf 'permission names                     :%s\n' "$PERM_LIST"
	fi
	if [ "$PERM_ELEV" -gt 0 ]; then
		printf 'of those, ELEVATED per scope.go      :%s\n' "$PERM_ELEV_LIST"
	fi
	if [ "$PERM_UNKNOWN" -gt 0 ]; then
		printf 'unrecognised permission names        : %s (withheld — not in the enum at %s)\n' \
			"$PERM_UNKNOWN" "$SOURCE"
	fi
fi
printf '\n'

# ---------------------------------------------------------------------------
# 2. QUESTION A, part one: does this account carry a content filter at all?
# ---------------------------------------------------------------------------

printf -- '--- 2. question A, part one: content filters on this account ---\n'

CF_STATUS=""
CF_TOTAL=""
CF_IT=0
CF_ET=0
CF_IG=0
CF_EG=0

r="$(bo_get "/api/v1/users/me/content-filters" "$WORK/cf.raw" "$WORK/hc")"
CF_STATUS="$(code_of "$r")"
if [ "$CF_STATUS" = "200" ]; then
	blank "$WORK/cf.raw" "$WORK/cf"
	# Each bucket is an array of {id,name}. The names are gone by now, so
	# counting `"id":` inside the bucket is exact and cannot see a tag name.
	count_bucket() {
		awk -v key="$1" -f "$WORK/member.awk" "$WORK/cf" |
			{ grep -o '"id":' || true; } | wc -l | tr -d ' '
	}
	CF_IT="$(count_bucket includeTags)"
	CF_ET="$(count_bucket excludeTags)"
	CF_IG="$(count_bucket includeGenres)"
	CF_EG="$(count_bucket excludeGenres)"
	CF_TOTAL=$((CF_IT + CF_ET + CF_IG + CF_EG))
	printf 'GET  /api/v1/users/me/content-filters : 200 OK\n'
	printf 'includeTagIds                        : %s\n' "$CF_IT"
	printf 'excludeTagIds                        : %s\n' "$CF_ET"
	printf 'includeGenreIds                      : %s\n' "$CF_IG"
	printf 'excludeGenreIds                      : %s\n' "$CF_EG"
	printf 'total filter entries                 : %s\n' "$CF_TOTAL"
	printf '(counts only — the endpoint returns tag and genre NAMES and they were\n'
	printf ' redacted before this count was taken. Nothing here can leak one.)\n'
else
	printf 'GET  /api/v1/users/me/content-filters : %s — %s\n' "$CF_STATUS" "$(explain_code "$CF_STATUS")"
fi
printf '\n'

# ---------------------------------------------------------------------------
# 3. Libraries, and QUESTION A part two: true count against visible count.
# ---------------------------------------------------------------------------

printf -- '--- 3. question A, part two: true count vs visible count, per library ---\n'

r="$(bo_get "/api/v1/libraries" "$WORK/libs.raw" "$WORK/hL")"
LIBS_STATUS="$(code_of "$r")"
if [ "$LIBS_STATUS" != "200" ]; then
	printf 'GET  /api/v1/libraries               : %s — %s\n' "$LIBS_STATUS" "$(explain_code "$LIBS_STATUS")"
	printf '\n=== VERDICT ===\n'
	if [ "$CF_STATUS" != "200" ]; then
		printf 'A1 filters set    : UNANSWERED — the content-filter endpoint returned %s.\n' "$CF_STATUS"
	elif [ "$CF_TOTAL" -gt 0 ]; then
		printf 'A1 filters set    : PRESENT — %s filter entries. The replica WILL be a subset.\n' "$CF_TOTAL"
	else
		printf 'A1 filters set    : ABSENT TODAY — 0 filter entries in all four buckets.\n'
	fi
	printf 'A2 shortfall      : UNANSWERED — the library list would not read (%s), so no\n' "$LIBS_STATUS"
	printf '                    library could be counted. This is an absent observation, NOT\n'
	printf '                    a finding that nothing is hidden.\n'
	printf 'B  mixed libs     : UNANSWERED — same reason.\n'
	exit 1
fi

blank "$WORK/libs.raw" "$WORK/libs"
awk -v want=2 -f "$WORK/depth.awk" "$WORK/libs" | { grep '"id":' || true; } >"$WORK/libl"
NLIB="$(wc -l <"$WORK/libl" | tr -d ' ')"
printf 'GET  /api/v1/libraries               : 200 OK — %s libraries visible to this account\n' "$NLIB"

if [ "${NLIB:-0}" -eq 0 ]; then
	printf '\n=== VERDICT ===\n'
	printf 'A2 shortfall      : UNANSWERED — this account is shown no library at all, so\n'
	printf '                    there is nothing to compare. Grant it library access and\n'
	printf '                    re-run. An empty list is not a clean bill of health.\n'
	printf 'B  mixed libs     : UNANSWERED — same reason.\n'
	exit 0
fi

# The known-format vocabulary: DEFAULT_FORMAT_PRIORITY (types/library.ts:8-25)
# plus cbx, which is in COMIC_FORMATS (types/book.ts:20) but not in the priority
# list. 17 values, inside the 20-element cap the rule schema imposes on `value`.
COMIC_JSON='["cbz","cbr","cb7","cbx"]'
AUDIO_JSON='["m4b","mp3","m4a","opus","ogg","flac"]'
EBOOK_JSON='["epub","kepub","pdf","mobi","azw3","azw","fb2"]'

present_rule='{"type":"rule","field":"fileAvailability","operator":"isPresent"}'
mk_body() {
	# $1 = extra rule or empty. Always ANDed with fileAvailability isPresent so
	# every count below sits on the same `status = 'present'` predicate that
	# getStats counts on. size 1 because this wants a total, not books.
	if [ -n "$1" ]; then
		printf '{"filter":{"type":"group","join":"AND","rules":[%s,%s]},"pagination":{"page":0,"size":1}}' \
			"$present_rule" "$1"
	else
		printf '{"filter":{"type":"group","join":"AND","rules":[%s]},"pagination":{"page":0,"size":1}}' \
			"$present_rule"
	fi
}
fmt_rule() { printf '{"type":"rule","field":"format","operator":"%s","value":%s}' "$1" "$2"; }

# total_of <body-file> — the top-level `total` of a BooksPage. Read after the
# depth filter so a `total` nested inside an item could not be mistaken for it.
total_of() {
	awk -v want=1 -f "$WORK/depth.awk" "$1" >"$WORK/page.d"
	num "$WORK/page.d" total
}

: >"$WORK/rows"
SLOT=0
SHORTFALL_LIBS=0
SHORTFALL_BOOKS=0
UNCOMPARED=0
MIXED_LIBS=0
CAP_STATUS=""

while IFS= read -r line; do
	if [ "$SLOT" -ge "$MAX_LIBRARIES" ]; then
		printf '\n(stopping at --max-libraries %s; %s libraries were listed)\n' "$MAX_LIBRARIES" "$NLIB"
		break
	fi
	SLOT=$((SLOT + 1))
	printf '%s' "$line" >"$WORK/lib.one"
	LID="$(num "$WORK/lib.one" id)"
	LBC="$(num "$WORK/lib.one" bookCount)"
	[ -n "$LID" ] || continue

	printf '\nlibrary %s\n' "$SLOT"
	if [ "$IS_SUPERUSER" = "yes" ]; then
		printf '  bookCount from GET /libraries      : %s  (unfiltered — a superuser is served\n' "${LBC:-unknown}"
		printf '                                       libraryRepo.findAll, library.repository.ts:18-28)\n'
	else
		printf '  bookCount from GET /libraries      : %s  (already content-filtered — this is\n' "${LBC:-unknown}"
		printf '                                       findAllForUser, library.repository.ts:30-52)\n'
	fi

	# TRUE: unfiltered, status='present'.
	r="$(bo_get "/api/v1/libraries/${LID}/stats" "$WORK/st.raw" "$WORK/hs")"
	sc="$(code_of "$r")"
	TRUE=""
	if [ "$sc" = "200" ]; then
		blank "$WORK/st.raw" "$WORK/st"
		awk -v want=1 -f "$WORK/depth.awk" "$WORK/st" >"$WORK/st.d"
		TRUE="$(num "$WORK/st.d" totalBooks)"
		printf '  GET  /libraries/{id}/stats         : 200 OK\n'
		printf '  totalBooks (TRUE, no filter ever)  : %s\n' "${TRUE:-unknown}"
	else
		printf '  GET  /libraries/{id}/stats         : %s — %s\n' "$sc" "$(explain_code "$sc")"
	fi

	# VISIBLE: the same predicate, through the filtered path.
	r="$(bo_post "/api/v1/libraries/${LID}/books" "$(mk_body '')" "$WORK/pg.raw" "$WORK/hp")"
	pc="$(code_of "$r")"
	VIS=""
	if [ "$pc" = "200" ]; then
		blank "$WORK/pg.raw" "$WORK/pg"
		VIS="$(total_of "$WORK/pg")"
		printf '  POST /libraries/{id}/books total   : %s  (VISIBLE — content-filtered)\n' "${VIS:-unknown}"
	else
		printf '  POST /libraries/{id}/books         : %s — %s\n' "$pc" "$(explain_code "$pc")"
	fi

	if [ -n "$TRUE" ] && [ -n "$VIS" ]; then
		if [ "$VIS" -lt "$TRUE" ]; then
			SHORTFALL_LIBS=$((SHORTFALL_LIBS + 1))
			SHORTFALL_BOOKS=$((SHORTFALL_BOOKS + TRUE - VIS))
			printf '  SHORTFALL                          : %s books present in this library that\n' "$((TRUE - VIS))"
			printf '                                       this account is NOT shown.\n'
		elif [ "$VIS" -gt "$TRUE" ]; then
			UNCOMPARED=$((UNCOMPARED + 1))
			printf '  SHORTFALL                          : none, and the visible count is HIGHER\n'
			printf '                                       (%s > %s), which should not happen on\n' "$VIS" "$TRUE"
			printf '                                       one predicate. Treat this library as\n'
			printf '                                       UNCOMPARED rather than clean.\n'
		else
			printf '  SHORTFALL                          : none — the two counts agree exactly.\n'
		fi
	else
		UNCOMPARED=$((UNCOMPARED + 1))
		printf '  SHORTFALL                          : UNCOMPARED — one of the two counts did\n'
		printf '                                       not read. Not a finding either way.\n'
	fi

	# QUESTION B. Two independent readings, because they answer subtly different
	# questions and disagreeing is itself informative.
	#
	#   (a) stats.formatCounts — UNFILTERED, and keyed on the PRIMARY file only.
	#       It INNER JOINs books.primary_file_id, so a book with no primary file
	#       appears in totalBooks and in no bucket; that gap is printed, never
	#       absorbed.
	#   (b) POST .../books with `format includesAny` — the endpoint the adapter
	#       actually pages, VISIBLE-only, and counting a book that has AT LEAST
	#       ONE file of the kind. A book holding both an epub and a cbz is
	#       counted under both, which is exactly what "is this container mixed"
	#       needs to know.
	KE=0
	KC=0
	KA=0
	KU=0
	FMT_KNOWN=""
	FMT_UNREC=0
	FMT_UNREC_BOOKS=0
	if [ "$sc" = "200" ]; then
		awk -v key=formatCounts -f "$WORK/member.awk" "$WORK/st" | tr ',' '\n' >"$WORK/fc"
		while IFS= read -r pair; do
			[ -n "$pair" ] || continue
			k="$(printf '%s' "$pair" | sed 's/^"//; s/":.*//' | tr 'A-Z' 'a-z')"
			v="$(printf '%s' "$pair" | sed 's/.*://')"
			case "$v" in '' | *[!0-9]*) continue ;; esac
			# getBookMediaKind (types/book.ts:97-103): audio set, then comic set,
			# then everything else non-empty is an ebook. `?` is a key this probe
			# redacted for not being a plain identifier; it lands in the same
			# `else` branch upstream would put it in.
			case "$k" in
			m4b | mp3 | m4a | opus | ogg | flac) KA=$((KA + v)) ;;
			cbz | cbr | cb7 | cbx) KC=$((KC + v)) ;;
			*) KE=$((KE + v)) ;;
			esac
			# The name is only ever PRINTED if it is a plain lowercase-alphanumeric
			# token AND is in the published format vocabulary. The first test is
			# what stops a redacted `?` — or anything else carrying a glob
			# metacharacter — from being matched as a pattern by the second.
			known=no
			case "$k" in
			'' | *[!a-z0-9]*) ;;
			*)
				case " epub kepub pdf cbz cbr cb7 cbx mobi azw3 azw fb2 m4b mp3 m4a opus ogg flac " in
				*" $k "*) known=yes ;;
				esac
				;;
			esac
			if [ "$known" = yes ]; then
				FMT_KNOWN="$FMT_KNOWN $k=$v"
			else
				FMT_UNREC=$((FMT_UNREC + 1))
				FMT_UNREC_BOOKS=$((FMT_UNREC_BOOKS + v))
			fi
		done <"$WORK/fc"
		if [ -n "$TRUE" ]; then
			KU=$((TRUE - KE - KC - KA))
			[ "$KU" -ge 0 ] || KU=0
		fi
		printf '  formatCounts by PRIMARY file (unfiltered):\n'
		printf '    ebook %s · comic %s · audiobook %s · no primary file %s\n' "$KE" "$KC" "$KA" "$KU"
		if [ -n "$FMT_KNOWN" ]; then
			printf '    recognised extensions             :%s\n' "$FMT_KNOWN"
		fi
		if [ "$FMT_UNREC" -gt 0 ]; then
			printf '    unrecognised extensions           : %s distinct, %s books (withheld —\n' "$FMT_UNREC" "$FMT_UNREC_BOOKS"
			printf '                                        not in DEFAULT_FORMAT_PRIORITY; counted\n'
			printf '                                        as ebook, which is what getBookMediaKind\n'
			printf '                                        does with any non-audio non-comic format)\n'
		fi
	fi

	VC=""
	VA=""
	VE=""
	for kind in comic audio ebook; do
		case "$kind" in
		comic) jsonv="$COMIC_JSON" ;;
		audio) jsonv="$AUDIO_JSON" ;;
		ebook) jsonv="$EBOOK_JSON" ;;
		esac
		r="$(bo_post "/api/v1/libraries/${LID}/books" "$(mk_body "$(fmt_rule includesAny "$jsonv")")" "$WORK/pg.raw" "$WORK/hp")"
		if [ "$(code_of "$r")" = "200" ]; then
			blank "$WORK/pg.raw" "$WORK/pg"
			t="$(total_of "$WORK/pg")"
			case "$kind" in
			comic) VC="$t" ;;
			audio) VA="$t" ;;
			ebook) VE="$t" ;;
			esac
		fi
	done
	printf '  books with >=1 file of each kind (POST .../books, VISIBLE-only):\n'
	printf '    ebook %s · comic %s · audiobook %s\n' "${VE:-?}" "${VC:-?}" "${VA:-?}"

	# Mixed is judged on the unfiltered reading when there is one, because that is
	# the one that describes the container rather than this account's view of it.
	NONZERO=0
	if [ "$sc" = "200" ]; then
		# `[ x ] && y` as a bare statement would take `set -e` down when the test
		# is false, which is exactly the common case here. Written as `if`.
		if [ "$KE" -gt 0 ]; then NONZERO=$((NONZERO + 1)); fi
		if [ "$KC" -gt 0 ]; then NONZERO=$((NONZERO + 1)); fi
		if [ "$KA" -gt 0 ]; then NONZERO=$((NONZERO + 1)); fi
		if [ "$NONZERO" -ge 2 ]; then
			MIXED_LIBS=$((MIXED_LIBS + 1))
			printf '  MIXED                              : YES — %s media kinds in one container.\n' "$NONZERO"
		elif [ "$NONZERO" -eq 1 ]; then
			printf '  MIXED                              : no — one media kind.\n'
		else
			printf '  MIXED                              : UNANSWERED — the library counted zero\n'
			printf '                                       books in every kind.\n'
		fi
		printf '%s\n' "$NONZERO" >>"$WORK/rows"
	else
		printf '  MIXED                              : UNANSWERED — stats did not read.\n'
	fi

	# Fire the size cap once, deliberately, on the first library. A cap that has
	# never been triggered is indistinguishable from no cap.
	if [ -z "$CAP_STATUS" ]; then
		r="$(bo_post "/api/v1/libraries/${LID}/books" '{"pagination":{"page":0,"size":201}}' "$WORK/cap.raw" "$WORK/hcap")"
		CAP_STATUS="$(code_of "$r")"
	fi
done <"$WORK/libl"

printf '\n'

# ---------------------------------------------------------------------------
# 4. Verdict.
# ---------------------------------------------------------------------------

printf -- '=== VERDICT ===\n'

# A, part one.
if [ "$CF_STATUS" != "200" ]; then
	printf 'A1 filters set    : UNANSWERED — GET /users/me/content-filters returned %s (%s)\n' \
		"$CF_STATUS" "$(explain_code "$CF_STATUS")"
	printf '                    An endpoint that did not answer is NOT an empty filter.\n'
elif [ "$IS_SUPERUSER" = "yes" ]; then
	printf 'A1 filters set    : NOT APPLICABLE — this account is a SUPERUSER. Every narrowing\n'
	printf '                    site reads `user.isSuperuser ? undefined : user.contentFilters`,\n'
	printf '                    and setContentFilters refuses a superuser outright\n'
	printf '                    (user.service.ts:423-425). So this run proves nothing about the\n'
	printf '                    account UsArr should actually store. RE-RUN AS THE SHARED\n'
	printf '                    SERVICE ACCOUNT — and note that storing a superuser credential\n'
	printf '                    is itself the finding scope.go reports as elevated.\n'
elif [ "$CF_TOTAL" -gt 0 ]; then
	printf 'A1 filters set    : PRESENT — %s filter entries on this account (include tags %s,\n' "$CF_TOTAL" "$CF_IT"
	printf '                    exclude tags %s, include genres %s, exclude genres %s).\n' "$CF_ET" "$CF_IG" "$CF_EG"
	printf '                    The replica WILL be a subset of the real library, and nothing\n'
	printf '                    on the wire says which books are missing. Detect and say so.\n'
else
	printf 'A1 filters set    : ABSENT TODAY — 0 entries in all four buckets, so nothing is\n'
	printf '                    being hidden by a content filter at this moment. Read that\n'
	printf '                    word: TODAY. An admin can add one at any time, in BookOrbit,\n'
	printf '                    with no signal to UsArr at all. "It is empty now" is not a\n'
	printf '                    design; the detector still has to ship.\n'
fi

# A, part two.
if [ "$SHORTFALL_LIBS" -gt 0 ]; then
	printf 'A2 shortfall      : PROVEN — %s of %s libraries return fewer books to this account\n' "$SHORTFALL_LIBS" "$SLOT"
	printf '                    than they contain, %s books in total. That is not an inference:\n' "$SHORTFALL_BOOKS"
	printf '                    it is stats.totalBooks minus the books-query total over the\n'
	printf '                    identical `status = present` predicate. Those %s books are\n' "$SHORTFALL_BOOKS"
	printf '                    exactly what UsArr would silently never learn about.\n'
elif [ "$UNCOMPARED" -ge "$SLOT" ]; then
	printf 'A2 shortfall      : UNANSWERED — no library produced both counts, so nothing was\n'
	printf '                    compared. An absent observation, not a negative finding.\n'
elif [ "$UNCOMPARED" -gt 0 ]; then
	printf 'A2 shortfall      : NONE ON %s OF %s LIBRARIES — every library that produced both\n' "$((SLOT - UNCOMPARED))" "$SLOT"
	printf '                    counts returned them equal. The other %s were UNCOMPARED and\n' "$UNCOMPARED"
	printf '                    say nothing either way.\n'
else
	printf 'A2 shortfall      : NONE — all %s libraries return exactly as many books as they\n' "$SLOT"
	printf '                    contain. This account sees the whole of every library it can\n'
	printf '                    see, as of this run.\n'
fi

# The gap this probe cannot close, stated rather than left as silence.
if [ "$IS_SUPERUSER" = "yes" ]; then
	printf 'A3 hidden libs    : NOT APPLICABLE — a superuser is shown every library.\n'
else
	printf 'A3 hidden libs    : UNANSWERED BY CONSTRUCTION — whether a library exists that this\n'
	printf '                    account is never shown cannot be settled read-only.\n'
	printf '                    LibraryAccessGuard throws the same ForbiddenException for\n'
	printf '                    "exists, no access row" and for "no such library"\n'
	printf '                    (library-access.guard.ts:39), so both are an identical 403 and\n'
	printf '                    no scan can separate them. This is a SECOND silent-subset axis,\n'
	printf '                    above the book level, and it is not closed by A2 reading clean.\n'
fi

# B.
if [ ! -s "$WORK/rows" ]; then
	printf 'B  mixed libs     : UNANSWERED — no library produced a format breakdown.\n'
elif [ "$MIXED_LIBS" -gt 0 ]; then
	printf 'B  mixed libs     : YES — %s of %s libraries hold more than one media kind. One\n' "$MIXED_LIBS" "$SLOT"
	printf '                    upstream container therefore becomes MORE THAN ONE UsArr\n'
	printf '                    library, because the UsArr library record holds exactly one\n'
	printf '                    kind. That is a design change, and the numbers above say which\n'
	printf '                    libraries force it and by how much.\n'
else
	printf 'B  mixed libs     : NO — every one of the %s libraries measured holds a single media\n' "$SLOT"
	printf '                    kind. One upstream container maps to one UsArr library. Read as\n'
	printf '                    a fact about THIS instance today, not about BookOrbit: nothing\n'
	printf '                    in the schema prevents a mixed library, so the mapping should\n'
	printf '                    still fail loudly rather than assume.\n'
fi

# The cap, fired rather than assumed.
case "$CAP_STATUS" in
400) printf 'pagination cap    : CONFIRMED — size 201 was REJECTED with 400, not clamped, as\n                    book-query.pipe.ts:27 says. Page with size <= 200.\n' ;;
200) printf 'pagination cap    : CONTRADICTED — size 201 returned 200 on this build. The pipe\n                    at %s caps it at 200; this instance did not. Worth a look.\n' "$SOURCE" ;;
'') printf 'pagination cap    : UNANSWERED — the boundary was never fired.\n' ;;
*) printf 'pagination cap    : %s — %s\n' "$CAP_STATUS" "$(explain_code "$CAP_STATUS")" ;;
esac

printf '\n'
printf 'Nothing above names a library, a book, a tag, a genre or a path. The three\n'
printf 'printed vocabularies (provisioningMethod, permission names, format extensions)\n'
printf 'name account types, capabilities and container formats, never content.\n'
printf 'Paste it as it stands.\n'

# The exit status is deliberately 0 for every verdict a real run can produce,
# shortfall and clean alike: both are successful probe runs. Only a run that
# could not reach, authenticate against or list libraries on BookOrbit exits
# non-zero, because that is the only case where there is nothing to read.
exit 0
