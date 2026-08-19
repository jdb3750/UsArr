#!/usr/bin/env bash
#
# kavita-cover-probe.sh — can UsArr fetch Kavita cover art with a header key,
#                          and is the free tier's colour good enough to skip it?
#
# UsArr · docs/REVIEW-LOG.md LS-260 · companion to kavita-volume-walk-probe.sh
#
# WHY THIS EXISTS
#   Four questions gate the cover-art work, and every one of them is a fact about
#   the owner's running Kavita rather than about a schema. Nothing in this repo
#   can settle any of them.
#
#     Q1 THE GATE. Does `GET /api/Image/series-cover` accept the Auth Key in the
#        `x-api-key` HEADER? The spec declares one global security scheme and it
#        is that header — but this repo has already recorded a Kavita controller
#        (`GET /api/Plugin/version`) that is `[AllowAnonymous]` and validates the
#        key out of the QUERY STRING itself, so the document's global `security`
#        block is not evidence about any particular controller. If the header
#        does not work, every cover fetch has to carry the Auth Key in a URL,
#        which changes the design and the logging rules with it. So the probe
#        tests three ways — header only, `?apiKey=` only, and neither — and
#        reports the status of each rather than one verdict.
#
#     Q2 Does a FREE Kavita populate `SeriesDto.primaryColor` / `secondaryColor`?
#        An entire zero-fetch first slice rests on it: if the colours are there,
#        UsArr can render a tinted placeholder for every series without fetching
#        a single image. ADR-0035 §1 established that free Kavita's *identifier*
#        fields come back null; the colour fields have never been checked.
#
#     Q3 What `Content-Type` does a cover actually come back as, and how big?
#        WebP would mean decoding needs a dependency this project does not have
#        and does not want. JPEG or PNG it already has in the standard library.
#        Read alongside it: does the response carry `ETag` or `Last-Modified` —
#        and, separately, does the server HONOUR them with a 304? A validator
#        header that is present but ignored is not conditional revalidation.
#
#     Q4 What does a series with NO cover return — 404, or 200 with a
#        placeholder? That decides whether a failed fetch is retried later or
#        cached as permanently absent.
#
# WHAT IT SENDS
#   Read-only GETs, plus the same read-only `POST /api/Series/all-v2` the volume
#   probe uses to page the library, to the Kavita base URL you supply and to
#   nothing else. No writes, no scans, no metadata refresh, no cover upload.
#   Every image question is a GET.
#
# WHAT IT PRINTS — this output is meant to be safe to paste into a chat
#   Counts, classifications, HTTP status codes, header NAMES, byte sizes and
#   timings. It never prints your Auth Key, your base URL, a library name, a
#   series title, a file path or a `coverImage` value. Series are identified by
#   sample slot ("sample 3"), never by id or name.
#
#   ONE DELIBERATE EXCEPTION, and it is deliberate: the probe DOES print the
#   `primaryColor` / `secondaryColor` VALUES it finds. Those are hex tints Kavita
#   derives from the cover art — `#a3624f` and the like. They name nothing: not a
#   series, not a library, not a path. They are printed because "5 of 5 series
#   carry a colour" is not the answer to Q2. A field can be populated and useless
#   — every series the same tint, or every one `#000000` — and only the values
#   themselves show that. Any value that is NOT a plain hex colour is withheld
#   and reported by length, so a Kavita that put something else in that field
#   cannot leak it through this probe.
#
#   `ETag` and `Last-Modified` are reported as PRESENT or ABSENT, and by whether
#   a conditional re-request earns a 304. Their values are not printed: an ETag
#   can be derived from a file's identity and a Last-Modified is a file mtime.
#
#   It writes no file anywhere except a private temp directory it deletes on
#   exit, and it never writes your Auth Key to any file at all.
#
# USAGE
#   KAVITA_URL=http://127.0.0.1:5000 KAVITA_API_KEY=... ./kavita-cover-probe.sh
#   KAVITA_URL=... ./kavita-cover-probe.sh --sample 8
#
#   The key is read from KAVITA_API_KEY, or prompted for silently if that is
#   unset. There is deliberately no --api-key flag: a credential on a command
#   line lands in your shell history and in the process table.
#
# REQUIREMENTS
#   bash and curl. THAT IS ALL. `jq` is NOT used anywhere in this script and is
#   not consulted for anything — the previous probe degraded to four-of-five
#   UNANSWERED on a box without it, so this one was designed jq-absent from the
#   first line. `od` is used for one optional cross-check and its absence costs
#   only that line.
#
# Licensed under AGPL-3.0, with the rest of UsArr.

set -euo pipefail

readonly PROBE_VERSION="1"
readonly SPEC="api/specs/kavita-v0.9.0.2.json"

SAMPLE=5
TIMEOUT=30

usage() {
	cat <<'USAGE'
kavita-cover-probe.sh — can UsArr fetch Kavita covers with a header key, and is
                        the free tier's colour good enough to skip fetching?

  KAVITA_URL=http://127.0.0.1:5000 KAVITA_API_KEY=... ./kavita-cover-probe.sh
  KAVITA_URL=... ./kavita-cover-probe.sh --sample 8

  --sample N    how many series to sample, spread across the library (default 5)
  --timeout S   per-request timeout in seconds (default 30)
  --url U       Kavita base URL, if you would rather not export KAVITA_URL
  -h, --help    this text

The Auth Key comes from KAVITA_API_KEY, or a silent prompt. There is no
--api-key flag on purpose: a credential on a command line lands in your shell
history and in the process table.

Read-only. Needs only bash and curl — no jq. Prints counts, classifications,
status codes, header names, sizes and timings, and never the key, the URL, a
library name, a series title or a file path. It DOES print the primaryColor /
secondaryColor hex values, on purpose: see the comment block at the top of this
file for why those specific values are in the output. Safe to paste.
USAGE
	exit "${1:-0}"
}

die() {
	printf 'probe: %s\n' "$*" >&2
	exit 2
}

while [ $# -gt 0 ]; do
	case "$1" in
	--sample)
		[ $# -ge 2 ] || die "--sample needs a number"
		SAMPLE="$2"
		shift 2
		;;
	--sample=*)
		SAMPLE="${1#*=}"
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
	--url)
		[ $# -ge 2 ] || die "--url needs a base URL"
		KAVITA_URL="$2"
		shift 2
		;;
	--url=*)
		KAVITA_URL="${1#*=}"
		shift
		;;
	--api-key | --api-key=* | --key | --key=*)
		die "there is no --api-key flag on purpose: a credential on a command line lands in your shell history and in the process table. Use the KAVITA_API_KEY environment variable, or let the probe prompt you."
		;;
	-h | --help) usage 0 ;;
	*) die "unknown argument: $1 (try --help)" ;;
	esac
done

case "$SAMPLE" in '' | *[!0-9]*) die "--sample must be a positive integer" ;; esac
case "$TIMEOUT" in '' | *[!0-9]*) die "--timeout must be a positive integer" ;; esac
[ "$SAMPLE" -ge 1 ] || die "--sample must be at least 1"

command -v curl >/dev/null 2>&1 || die "curl is required and is not on PATH"

HAVE_OD=0
if command -v od >/dev/null 2>&1; then HAVE_OD=1; fi

BASE="${KAVITA_URL:-}"
[ -n "$BASE" ] || die "set KAVITA_URL (or pass --url), e.g. http://127.0.0.1:5000"
BASE="${BASE%/}"
case "$BASE" in
http://* | https://*) ;;
*) die "KAVITA_URL must start with http:// or https://" ;;
esac

KEY="${KAVITA_API_KEY:-}"
if [ -z "$KEY" ]; then
	[ -t 0 ] || die "KAVITA_API_KEY is unset and stdin is not a terminal, so the probe cannot prompt for it"
	printf 'Kavita Auth Key (not echoed, not stored, not printed): ' >&2
	IFS= read -rs KEY
	printf '\n' >&2
fi
[ -n "$KEY" ] || die "no Auth Key supplied"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/kavita-cover-probe.XXXXXX")"
chmod 700 "$WORK"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------------------
# HTTP. Every call goes through here so that exactly one place holds the key,
# and so that no error path can print a URL or a header.
# ---------------------------------------------------------------------------

# kget <path-with-query> <body-file> <hdr-file> <auth: header|query|none> [curl-args...]
# Echoes "<http_code> <seconds> <bytes>". Never fails the script on a 4xx.
kget() {
	local path="$1" body="$2" hdr="$3" auth="$4" url out
	shift 4
	local extra=("$@")
	url="${BASE}${path}"
	# No --fail-with-body: it makes curl exit non-zero on 4xx, which would throw
	# away the very status code that tells the operator what to fix. That defect
	# was found in the volume probe's drill and is not repeated here.
	local args=(-sS -o "$body" -w '%{http_code} %{time_total} %{size_download}'
		--max-time "$TIMEOUT" -D "$hdr")
	case "$auth" in
	header) args+=(-H "x-api-key: ${KEY}") ;;
	query)
		# The Auth Key is in this URL. The URL is never echoed, never logged and
		# never written to a file — but it is visible in this machine's process
		# table for the life of the request, exactly as the header variant's is.
		# That is a known and accepted cost of asking the question at all.
		case "$path" in
		*\?*) url="${url}&apiKey=${KEY}" ;;
		*) url="${url}?apiKey=${KEY}" ;;
		esac
		;;
	none) ;;
	*) die "internal: bad auth mode" ;;
	esac
	[ "${#extra[@]}" -eq 0 ] || args+=("${extra[@]}")
	if ! out="$(curl "${args[@]}" "$url" 2>"$WORK/curlerr")"; then
		# curl's own message can carry the URL; the body can carry library data.
		# Neither is printed. Only the code and the classification are.
		printf '000 0 0\n'
		return 0
	fi
	printf '%s\n' "$out"
}

# kpost <path-with-query> <json-body> <body-file> <hdr-file>. Header auth only.
kpost() {
	local path="$1" payload="$2" body="$3" hdr="$4" out
	if ! out="$(curl -sS -o "$body" -w '%{http_code} %{time_total} %{size_download}' \
		--max-time "$TIMEOUT" -D "$hdr" \
		-H "x-api-key: ${KEY}" \
		-H 'Content-Type: application/json' \
		-H 'Accept: application/json, text/plain;q=0.9, */*;q=0.1' \
		--data-binary "$payload" \
		"${BASE}${path}" 2>"$WORK/curlerr")"; then
		printf '000 0 0\n'
		return 0
	fi
	printf '%s\n' "$out"
}

code_of() { printf '%s' "$(printf '%s' "$1" | cut -d' ' -f1)"; }
secs_of() { printf '%s' "$(printf '%s' "$1" | cut -d' ' -f2)"; }
size_of() { printf '%s' "$(printf '%s' "$1" | cut -d' ' -f3)"; }
ms_of() { awk -v s="$(secs_of "$1")" 'BEGIN{printf "%.0f", s*1000}'; }

# hval <hdr-file> <lowercase-header-name> — last value of a response header.
# `|| true` guards `set -o pipefail`: a header that is absent is an ANSWER here.
hval() {
	{ tr -d '\r' <"$2" 2>/dev/null || true; } |
		{ grep -i "^$1:" || true; } | tail -n1 | sed "s/^[^:]*:[[:space:]]*//"
}

explain_code() {
	case "$1" in
	000) printf 'no HTTP response (connection, TLS or timeout). The probe does not print curl'"'"'s message because it carries the URL.' ;;
	400) printf 'bad request — the endpoint exists but rejected the parameters.' ;;
	401) printf 'unauthenticated — the Auth Key was rejected or not seen. Kavita User Settings -> Manage Auth Keys.' ;;
	403) printf 'forbidden — the key authenticated but the account lacks rights for this endpoint.' ;;
	404) printf 'not found — wrong base URL, a reverse-proxy path prefix, or an endpoint this release does not have.' ;;
	500) printf 'server error — Kavita threw. Its own log has the detail.' ;;
	*) printf 'HTTP %s' "$1" ;;
	esac
}

# ---------------------------------------------------------------------------
# Preamble — printed before the first packet leaves.
# ---------------------------------------------------------------------------

printf '=== UsArr Kavita cover-art probe v%s ===\n' "$PROBE_VERSION"
printf 'Read-only. Sends GET, plus the same read-only POST /api/Series/all-v2 the\n'
printf 'volume probe uses to page the library, to your Kavita only. Every image\n'
printf 'question is a GET. Your Auth Key is never printed, never logged, never\n'
printf 'written to a file.\n'
printf '\n'
printf 'Output is safe to paste: counts, classifications, status codes, header NAMES,\n'
printf 'byte sizes and timings. No URL, no library name, no series title, no file path.\n'
printf '\n'
printf 'ONE EXCEPTION, on purpose: the primaryColor / secondaryColor VALUES are printed.\n'
printf 'They are hex tints Kavita derives from cover art (#a3624f and the like) and they\n'
printf 'name nothing — not a series, not a library, not a path. They are printed because\n'
printf '"5 of 5 carry a colour" is not the answer: a field can be populated and useless\n'
printf 'if every series is the same tint or every one is #000000, and only the values\n'
printf 'show that. Anything in those fields that is not a plain hex colour is WITHHELD\n'
printf 'and reported by length instead.\n'
printf '\n'
printf 'ETag and Last-Modified are reported as present/absent and by whether a\n'
printf 'conditional re-request earns a 304. Their values are not printed.\n'
printf '\n'
printf 'Needs only bash and curl. jq is not used anywhere in this script.\n'
printf 'Endpoints verified against %s before this script was written.\n' "$SPEC"
printf '\n'

# ---------------------------------------------------------------------------
# 0. Reachability and auth.
# ---------------------------------------------------------------------------

printf -- '--- 0. reachability ---\n'
r="$(kget "/api/Health" "$WORK/health" "$WORK/h0" none)"
hc="$(code_of "$r")"
if [ "$hc" = "200" ]; then
	printf 'GET /api/Health              : 200 OK ([AllowAnonymous], sent with no key)\n'
else
	printf 'GET /api/Health              : %s — %s\n' "$hc" "$(explain_code "$hc")"
fi

FILTER_BODY='{"id":0,"name":null,"statements":[],"combination":1,"sortOptions":{"sortField":1,"isAscending":true},"entityType":0,"limitTo":0}'

r="$(kpost "/api/Series/all-v2?PageNumber=1&PageSize=1&context=1" "$FILTER_BODY" "$WORK/probe1" "$WORK/h1")"
hc="$(code_of "$r")"
if [ "$hc" != "200" ]; then
	printf 'POST /api/Series/all-v2      : %s — %s\n' "$hc" "$(explain_code "$hc")"
	printf '\nVERDICT: INCONCLUSIVE — could not authenticate a series read. Nothing below ran.\n'
	exit 1
fi
printf 'POST /api/Series/all-v2      : 200 OK (authenticated, x-api-key header)\n'

# The `Pagination` response header is middleware, not in the OpenAPI document, so
# it is read when present and never required.
TOTAL_SERIES=""
pag="$(hval 'pagination' "$WORK/h1")"
if [ -n "$pag" ]; then
	TOTAL_SERIES="$(printf '%s' "$pag" | sed -n 's/.*"totalItems" *: *\([0-9]\{1,\}\).*/\1/p')"
fi
if [ -n "$TOTAL_SERIES" ]; then
	printf 'series in library (Pagination header): %s\n' "$TOTAL_SERIES"
else
	printf 'series in library            : UNKNOWN (no Pagination header — a reverse proxy may strip it)\n'
fi
printf '\n'

# ---------------------------------------------------------------------------
# 1. Sample selection. Each single-series page body is kept, because it already
#    carries primaryColor, secondaryColor and coverImage — Q2 costs no extra
#    request.
# ---------------------------------------------------------------------------

printf -- '--- 1. sample selection ---\n'
IDS=""
NSEL=0

: >"$WORK/colors"

# first_id <file> — the id of the first object in a SeriesDto array. SeriesDto
# has no nested objects at all, so on a one-element page `"id":` occurs exactly
# once and this needs no JSON parser.
first_id() {
	{ grep -o '"id":[[:space:]]*[0-9]\{1,\}' "$1" || true; } | head -n1 |
		sed 's/.*://; s/[[:space:]]//g'
}

# field_raw <file> <key> — the first value of a top-level SeriesDto string field,
# still quoted, or the literal `null`. Same reasoning: one occurrence per series.
field_raw() {
	{ grep -o "\"$2\":[[:space:]]*\(\"[^\"]*\"\|null\)" "$1" || true; } | head -n1 |
		sed "s/^\"$2\":[[:space:]]*//"
}

# safe_color <raw> — a hex colour is printed; anything else is withheld. This is
# what keeps the one deliberate exception to the redaction rule from becoming a
# hole: if Kavita ever puts a non-colour in that field, it does not reach stdout.
safe_color() {
	local raw="$1" v
	if [ -z "$raw" ]; then
		printf 'ABSENT'
		return 0
	fi
	if [ "$raw" = "null" ]; then
		printf 'null'
		return 0
	fi
	v="${raw#\"}"
	v="${v%\"}"
	if [ -z "$v" ]; then
		printf 'empty-string'
		return 0
	fi
	case "$v" in
	'#'[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f] | \
		'#'[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f] | \
		'#'[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f])
		printf '%s' "$v"
		;;
	*) printf 'NON-HEX (withheld, %s chars)' "${#v}" ;;
	esac
}

pick_by_page() {
	# One PageSize=1 request per sample slot, at evenly spread page numbers.
	local total="$1" k page rr cc id
	k=1
	while [ "$k" -le "$SAMPLE" ]; do
		page=$(((2 * k - 1) * total / (2 * SAMPLE) + 1))
		[ "$page" -ge 1 ] || page=1
		[ "$page" -le "$total" ] || page="$total"
		rr="$(kpost "/api/Series/all-v2?PageNumber=${page}&PageSize=1&context=1" "$FILTER_BODY" "$WORK/pick" "$WORK/hp")"
		cc="$(code_of "$rr")"
		if [ "$cc" = "200" ]; then
			id="$(first_id "$WORK/pick")"
			if [ -n "$id" ]; then
				IDS="$IDS $id"
				NSEL=$((NSEL + 1))
				record_colors "$WORK/pick"
			fi
		fi
		k=$((k + 1))
	done
}

pick_from_one_page() {
	# Fallback when the total is unknown: one page, split one object per line.
	local rr cc n i idx id
	rr="$(kpost "/api/Series/all-v2?PageNumber=1&PageSize=200&context=1" "$FILTER_BODY" "$WORK/bigpage" "$WORK/hp")"
	cc="$(code_of "$rr")"
	[ "$cc" = "200" ] || return 0
	split_objects "$WORK/bigpage" "$WORK/objs"
	n="$(wc -l <"$WORK/objs" | tr -d ' ')"
	[ "${n:-0}" -gt 0 ] || return 0
	i=1
	while [ "$i" -le "$SAMPLE" ]; do
		idx=$(((2 * i - 1) * n / (2 * SAMPLE) + 1))
		[ "$idx" -ge 1 ] || idx=1
		[ "$idx" -le "$n" ] || idx="$n"
		sed -n "${idx}p" "$WORK/objs" >"$WORK/one"
		id="$(first_id "$WORK/one")"
		if [ -n "$id" ]; then
			IDS="$IDS $id"
			NSEL=$((NSEL + 1))
			record_colors "$WORK/one"
		fi
		i=$((i + 1))
	done
}

# split_objects <array-file> <out-file> — one JSON object per line. Safe here
# because SeriesDto contains no nested object, so `},{` at the top level is the
# only place that sequence can legitimately appear; a `},{` inside a series NAME
# would mis-split, which is why this is used only for counting and never for
# anything the probe prints as a fact about a specific series.
split_objects() {
	tr -d '\n' <"$1" | sed 's/},[[:space:]]*{/}\
{/g' >"$2.raw"
	# Filtering through grep is not decoration: `sed` leaves the final object
	# WITHOUT a trailing newline, so `wc -l` on its output undercounts by one and
	# the last series silently vanishes from every count downstream. grep
	# newline-terminates what it emits. This was found by driving the probe
	# against a stub with a known series count and reading 39 for 40.
	{ grep '"id":' "$2.raw" || true; } >"$2"
}

record_colors() {
	local pc sc ci
	pc="$(field_raw "$1" primaryColor)"
	sc="$(field_raw "$1" secondaryColor)"
	ci="$(field_raw "$1" coverImage)"
	# coverImage is a STORAGE PATH. Only its null-ness is recorded, never its value.
	if [ "$ci" = "null" ] || [ -z "$ci" ] || [ "$ci" = '""' ]; then ci=absent; else ci=present; fi
	printf '%s\t%s\t%s\n' "$(safe_color "$pc")" "$(safe_color "$sc")" "$ci" >>"$WORK/colors"
}

if [ -n "$TOTAL_SERIES" ] && [ "$TOTAL_SERIES" -gt 0 ]; then
	[ "$SAMPLE" -le "$TOTAL_SERIES" ] || SAMPLE="$TOTAL_SERIES"
	pick_by_page "$TOTAL_SERIES"
	printf 'sampled %d series, spread evenly across the sort order (ids withheld)\n' "$NSEL"
else
	pick_from_one_page
	printf 'sampled %d series from the first page only (no total available; ids withheld)\n' "$NSEL"
fi

if [ "$NSEL" -eq 0 ]; then
	printf '\nVERDICT: INCONCLUSIVE — no series could be sampled.\n'
	exit 1
fi
FIRST_ID="$(printf '%s' "$IDS" | tr ' ' '\n' | grep -v '^$' | head -n1)"
printf '\n'

# ---------------------------------------------------------------------------
# 2. Q1 — THE GATE.
# ---------------------------------------------------------------------------

printf -- '--- 2. Q1 THE GATE: GET /api/Image/series-cover, three ways ---\n'
printf 'One series (sample 1). The spec declares seriesId and apiKey as query\n'
printf 'parameters and one global header scheme; which of them the controller\n'
printf 'actually honours is what is being measured.\n\n'

# Kavita Auth Keys are GUIDs, so the query variant can splice the key into a URL
# unencoded. If a key ever contains something that is not URL-safe, the query
# test would be measuring a mangled URL rather than the controller, so say so
# instead of reporting its status as if it meant something.
KEY_URLSAFE=1
case "$KEY" in *[!A-Za-z0-9._~-]*) KEY_URLSAFE=0 ;; esac

r="$(kget "/api/Image/series-cover?seriesId=${FIRST_ID}" "$WORK/covh" "$WORK/hh" header)"
Q1_HEADER="$(code_of "$r")"
Q1_HEADER_BYTES="$(size_of "$r")"
r="$(kget "/api/Image/series-cover?seriesId=${FIRST_ID}" "$WORK/covq" "$WORK/hq" query)"
Q1_QUERY="$(code_of "$r")"
r="$(kget "/api/Image/series-cover?seriesId=${FIRST_ID}" "$WORK/covn" "$WORK/hn" none)"
Q1_NONE="$(code_of "$r")"

printf '%-34s %s\n' "x-api-key header only" "$Q1_HEADER"
if [ "$KEY_URLSAFE" -eq 1 ]; then
	printf '%-34s %s\n' "?apiKey= query only (no header)" "$Q1_QUERY"
else
	printf '%-34s %s  (your key contains characters this probe does not\n' "?apiKey= query only (no header)" "$Q1_QUERY"
	printf '%-34s     URL-encode, so read this row as unreliable)\n' ""
fi
printf '%-34s %s\n' "neither (no credential at all)" "$Q1_NONE"
printf '\n'
for c in "$Q1_HEADER" "$Q1_QUERY" "$Q1_NONE"; do
	case "$c" in 200) ;; *) printf '  %s: %s\n' "$c" "$(explain_code "$c")" ;; esac
done
printf '\n'

# The rest of the run must not go blind because the ANSWER to Q1 was "not the
# header". A probe that reports FAIL on Q1 and then UNANSWERED on Q3 and Q4 —
# because it kept sending the auth it just proved does not work — has thrown away
# three of its four questions to make a point about the first. Whichever variant
# returned 200 is what every image call below uses.
if [ "$Q1_HEADER" = "200" ]; then
	AUTH_MODE=header
elif [ "$Q1_QUERY" = "200" ]; then
	AUTH_MODE=query
elif [ "$Q1_NONE" = "200" ]; then
	AUTH_MODE=none
else
	AUTH_MODE=header
fi
if [ "$AUTH_MODE" = "header" ]; then
	printf 'image calls below use   : the x-api-key header\n'
else
	printf 'image calls below use   : %s — the header did not work, so the remaining\n' "$AUTH_MODE"
	printf '                          questions are answered with the variant that did,\n'
	printf '                          rather than left UNANSWERED to make a point.\n'
fi
printf '\n'

# ---------------------------------------------------------------------------
# 3. Q2 — colour.
# ---------------------------------------------------------------------------

printf -- '--- 3. Q2: SeriesDto.primaryColor / secondaryColor on a free instance ---\n'
printf '%-9s %-30s %-30s %s\n' "slot" "primaryColor" "secondaryColor" "coverImage"
slot=0
while IFS="$(printf '\t')" read -r pc sc ci; do
	slot=$((slot + 1))
	printf '%-9s %-30s %-30s %s\n' "sample $slot" "$pc" "$sc" "$ci"
done <"$WORK/colors"
printf '\n'

PC_SET="$({ cut -f1 "$WORK/colors" | grep -c '^#' || true; })"
SC_SET="$({ cut -f2 "$WORK/colors" | grep -c '^#' || true; })"
PC_DISTINCT="$({ cut -f1 "$WORK/colors" | grep '^#' || true; } | sort -u | wc -l | tr -d ' ')"
SC_DISTINCT="$({ cut -f2 "$WORK/colors" | grep '^#' || true; } | sort -u | wc -l | tr -d ' ')"
PC_BLACK="$({ cut -f1 "$WORK/colors" | grep -ci '^#0\{3,8\}$' || true; })"
COVER_ABSENT="$({ cut -f3 "$WORK/colors" | grep -c '^absent$' || true; })"

printf 'primaryColor is a hex colour  : %s of %s\n' "$PC_SET" "$NSEL"
printf 'secondaryColor is a hex colour: %s of %s\n' "$SC_SET" "$NSEL"
printf 'distinct primaryColor values  : %s (1 across a sample of %s means the field is\n' "$PC_DISTINCT" "$NSEL"
printf '                                populated but carries no information)\n'
printf 'distinct secondaryColor values: %s\n' "$SC_DISTINCT"
printf 'primaryColor all-zero (#000...): %s\n' "$PC_BLACK"
printf 'coverImage field absent/null  : %s of %s\n' "$COVER_ABSENT" "$NSEL"
printf '\n'

# ---------------------------------------------------------------------------
# 4. Q3 — what a cover actually is on the wire.
# ---------------------------------------------------------------------------

printf -- '--- 4. Q3: cover Content-Type, size, and validator headers ---\n'
: >"$WORK/cov_times"
: >"$WORK/cov_sizes"
: >"$WORK/cov_ct"
N_COVER_OK=0
N_COVER_404=0
N_COVER_OTHER=0
ETAG_SEEN=0
LM_SEEN=0
ETAG_VALUE=""
ETAG_PATH=""
LM_VALUE=""
LM_PATH=""
CC_VALUE=""

printf '%-9s %6s %10s %-28s %-6s %-14s %8s\n' \
	"slot" "status" "bytes" "content-type" "ETag" "Last-Modified" "ms"
slot=0
for id in $IDS; do
	slot=$((slot + 1))
	r="$(kget "/api/Image/series-cover?seriesId=${id}" "$WORK/cov" "$WORK/hc" "$AUTH_MODE")"
	hc="$(code_of "$r")"
	ms="$(ms_of "$r")"
	sz="$(size_of "$r")"
	case "$hc" in
	200) N_COVER_OK=$((N_COVER_OK + 1)) ;;
	404) N_COVER_404=$((N_COVER_404 + 1)) ;;
	*) N_COVER_OTHER=$((N_COVER_OTHER + 1)) ;;
	esac
	if [ "$hc" != "200" ]; then
		printf '%-9s %6s %10s %-28s %-6s %-14s %8s   <- %s\n' \
			"sample $slot" "$hc" "-" "-" "-" "-" "-" "$(explain_code "$hc")"
		continue
	fi
	ct="$(hval 'content-type' "$WORK/hc")"
	et="$(hval 'etag' "$WORK/hc")"
	lm="$(hval 'last-modified' "$WORK/hc")"
	cc="$(hval 'cache-control' "$WORK/hc")"
	[ -n "$CC_VALUE" ] || CC_VALUE="$cc"
	if [ -n "$et" ]; then
		ETAG_SEEN=$((ETAG_SEEN + 1))
		if [ -z "$ETAG_VALUE" ]; then
			ETAG_VALUE="$et"
			ETAG_PATH="/api/Image/series-cover?seriesId=${id}"
		fi
	fi
	if [ -n "$lm" ]; then
		LM_SEEN=$((LM_SEEN + 1))
		if [ -z "$LM_VALUE" ]; then
			LM_VALUE="$lm"
			LM_PATH="/api/Image/series-cover?seriesId=${id}"
		fi
	fi
	printf '%s\n' "$ms" >>"$WORK/cov_times"
	printf '%s\n' "$sz" >>"$WORK/cov_sizes"
	printf '%s\n' "${ct:-<none>}" >>"$WORK/cov_ct"
	# Keep the last body that was actually a 200: the magic-byte cross-check below
	# must not read an error page left behind by a later failing sample.
	cp "$WORK/cov" "$WORK/lastok"
	printf '%-9s %6s %10s %-28s %-6s %-14s %8s\n' \
		"sample $slot" "$hc" "$sz" "${ct:-<none>}" \
		"$([ -n "$et" ] && printf yes || printf no)" \
		"$([ -n "$lm" ] && printf yes || printf no)" "$ms"
done
printf '\n'

printf 'distinct Content-Type values  :\n'
{ sort "$WORK/cov_ct" 2>/dev/null || true; } | uniq -c | sed 's/^/  /'
if [ ! -s "$WORK/cov_ct" ]; then printf '  (none — no cover returned 200)\n'; fi

# Magic-byte cross-check: a Content-Type header is a claim, the first bytes are
# the fact. `$WORK/cov` still holds the last 200 body.
MAGIC="unchecked"
if [ "$HAVE_OD" -eq 1 ] && [ -s "$WORK/lastok" ]; then
	m="$(od -An -tx1 -N16 "$WORK/lastok" | tr -d ' \n')"
	case "$m" in
	ffd8ff*) MAGIC="JPEG" ;;
	89504e47*) MAGIC="PNG" ;;
	52494646????????57454250*) MAGIC="WebP" ;;
	474946383*) MAGIC="GIF" ;;
	????????6674797061766966*) MAGIC="AVIF" ;;
	????????6674797068656963*) MAGIC="HEIC" ;;
	3c*) MAGIC="not an image — the body starts with '<' (HTML or XML)" ;;
	7b*) MAGIC="not an image — the body starts with '{' (JSON)" ;;
	*) MAGIC="unrecognised magic bytes" ;;
	esac
fi
printf 'first bytes of the last 200 body: %s\n' "$MAGIC"
printf 'Cache-Control (first seen)    : %s\n' "${CC_VALUE:-ABSENT}"
printf 'ETag present                  : %s of %s covers\n' "$ETAG_SEEN" "$N_COVER_OK"
printf 'Last-Modified present         : %s of %s covers\n' "$LM_SEEN" "$N_COVER_OK"
printf '  (validator VALUES are not printed: an ETag can be derived from a file'"'"'s\n'
printf '   identity and a Last-Modified is a file mtime)\n'
printf '\n'

# A validator header that is present but ignored is not conditional
# revalidation. Fire it rather than trusting it.
printf -- '--- 4a. is the validator HONOURED? one conditional re-request ---\n'
REVAL="none"
if [ -n "$ETAG_VALUE" ]; then
	r="$(kget "$ETAG_PATH" "$WORK/cond" "$WORK/hcond" "$AUTH_MODE" -H "If-None-Match: ${ETAG_VALUE}")"
	cc2="$(code_of "$r")"
	printf 'If-None-Match (ETag)          : %s (%s bytes)\n' "$cc2" "$(size_of "$r")"
	if [ "$cc2" = "304" ]; then REVAL="etag"; fi
else
	printf 'If-None-Match (ETag)          : not attempted — no ETag came back\n'
fi
if [ "$REVAL" = "none" ] && [ -n "$LM_VALUE" ]; then
	r="$(kget "$LM_PATH" "$WORK/cond" "$WORK/hcond" "$AUTH_MODE" -H "If-Modified-Since: ${LM_VALUE}")"
	cc2="$(code_of "$r")"
	printf 'If-Modified-Since             : %s (%s bytes)\n' "$cc2" "$(size_of "$r")"
	if [ "$cc2" = "304" ]; then REVAL="last-modified"; fi
elif [ -z "$LM_VALUE" ]; then
	printf 'If-Modified-Since             : not attempted — no Last-Modified came back\n'
else
	printf 'If-Modified-Since             : not attempted — the ETag already earned a 304\n'
fi
printf '\n'

# ---------------------------------------------------------------------------
# 5. Q4 — what a series with no cover returns.
# ---------------------------------------------------------------------------

printf -- '--- 5. Q4: what does a series with NO cover return? ---\n'
NOCOVER_STATUS=""
NOCOVER_BYTES=""
NOCOVER_CT=""
SCANNED=0
SCANNED_NULL=0

r="$(kpost "/api/Series/all-v2?PageNumber=1&PageSize=200&context=1" "$FILTER_BODY" "$WORK/bigpage" "$WORK/hbig")"
if [ "$(code_of "$r")" = "200" ]; then
	split_objects "$WORK/bigpage" "$WORK/objs"
	SCANNED="$(wc -l <"$WORK/objs" | tr -d ' ')"
	{ grep '"coverImage":[[:space:]]*\(null\|""\)' "$WORK/objs" || true; } >"$WORK/nullcov"
	SCANNED_NULL="$(wc -l <"$WORK/nullcov" | tr -d ' ')"
	printf 'series scanned for a null/empty coverImage (one page): %s\n' "$SCANNED"
	printf 'of those, coverImage null or empty                   : %s\n' "$SCANNED_NULL"
	if [ "$SCANNED_NULL" -gt 0 ]; then
		head -n1 "$WORK/nullcov" >"$WORK/one"
		nid="$(first_id "$WORK/one")"
		if [ -n "$nid" ]; then
			r="$(kget "/api/Image/series-cover?seriesId=${nid}" "$WORK/nc" "$WORK/hnc" "$AUTH_MODE")"
			NOCOVER_STATUS="$(code_of "$r")"
			NOCOVER_BYTES="$(size_of "$r")"
			NOCOVER_CT="$(hval 'content-type' "$WORK/hnc")"
			printf 'GET series-cover for one such series: %s, %s bytes, content-type %s\n' \
				"$NOCOVER_STATUS" "$NOCOVER_BYTES" "${NOCOVER_CT:-<none>}"
		fi
	fi
else
	printf 'the wide page read failed, so no coverless series could be looked for\n'
fi

# A series that does not exist is a DIFFERENT question from a series with no
# cover, and is reported separately so the two are never conflated.
MAXID=0
for id in $IDS; do [ "$id" -le "$MAXID" ] || MAXID="$id"; done
GHOST=$((MAXID + 1000000))
r="$(kget "/api/Image/series-cover?seriesId=${GHOST}" "$WORK/ghost" "$WORK/hg" "$AUTH_MODE")"
GHOST_STATUS="$(code_of "$r")"
GHOST_BYTES="$(size_of "$r")"
printf 'separately, a series id that does NOT exist         : %s, %s bytes\n' "$GHOST_STATUS" "$GHOST_BYTES"
printf '  (a different question from "exists but has no cover" — not conflated with it)\n'
printf '\n'

# ---------------------------------------------------------------------------
# 6. Request cost.
# ---------------------------------------------------------------------------

med_file() {
	[ -s "$1" ] || {
		printf -- '-'
		return 0
	}
	sort -n "$1" | awk '{a[NR]=$1} END{ printf "%d", a[int((NR+1)/2)] }'
}
min_file() {
	if [ -s "$1" ]; then sort -n "$1" | head -n1 | tr -d '\n'; else printf -- '-'; fi
}
max_file() {
	if [ -s "$1" ]; then sort -n "$1" | tail -n1 | tr -d '\n'; else printf -- '-'; fi
}

COV_MED="$(med_file "$WORK/cov_times")"
SZ_MED="$(med_file "$WORK/cov_sizes")"

printf -- '--- 6. request cost (wall clock, this network, this moment) ---\n'
printf 'GET /Image/series-cover ms    : min %s · median %s · max %s (n=%s)\n' \
	"$(min_file "$WORK/cov_times")" "$COV_MED" "$(max_file "$WORK/cov_times")" \
	"$(wc -l <"$WORK/cov_times" | tr -d ' ')"
printf 'cover size bytes              : min %s · median %s · max %s\n' \
	"$(min_file "$WORK/cov_sizes")" "$SZ_MED" "$(max_file "$WORK/cov_sizes")"
printf '\n'

# ---------------------------------------------------------------------------
# 7. Verdict.
# ---------------------------------------------------------------------------

printf -- '=== VERDICT ===\n'

# Q1
if [ "$Q1_HEADER" = "200" ] && [ "$Q1_NONE" = "200" ]; then
	printf 'Q1 header auth    : PASS, with a caveat — the header works (200), but so does\n'
	printf '                    sending NO credential at all (200). This controller is\n'
	printf '                    anonymous. UsArr can use the header, and should note that\n'
	printf '                    anyone who can reach this Kavita can read its cover art.\n'
elif [ "$Q1_HEADER" = "200" ]; then
	printf 'Q1 header auth    : PASS — x-api-key in the HEADER returns 200 (%s bytes), and an\n' "$Q1_HEADER_BYTES"
	printf '                    unauthenticated request returns %s. The Auth Key never has to\n' "$Q1_NONE"
	printf '                    go in a cover URL. Design for the header.\n'
elif [ "$Q1_QUERY" = "200" ]; then
	printf 'Q1 header auth    : FAIL, WITH CONSEQUENCES — the header returned %s and only\n' "$Q1_HEADER"
	printf '                    ?apiKey= in the QUERY returned 200. Every cover fetch must\n'
	printf '                    then carry a full-admin credential in a URL, which means: it\n'
	printf '                    lands in Kavita'"'"'s access log and any reverse proxy'"'"'s, it must\n'
	printf '                    be scrubbed from every error string UsArr logs or shows, and\n'
	printf '                    a recorded go-vcr fixture must never keep it. That is a\n'
	printf '                    different design, not a smaller one.\n'
else
	printf 'Q1 header auth    : INCONCLUSIVE — header %s, query %s, neither %s. No variant\n' \
		"$Q1_HEADER" "$Q1_QUERY" "$Q1_NONE"
	printf '                    returned 200, so this says nothing about which auth the\n'
	printf '                    controller honours. Check the sampled series exists and that\n'
	printf '                    the Auth Key is current, then re-run.\n'
fi

# Q2
if [ "$PC_SET" -eq 0 ]; then
	printf 'Q2 colour today   : NOT USABLE — none of the %s sampled series carried a hex\n' "$NSEL"
	printf '                    primaryColor. A zero-fetch tinted placeholder has nothing to\n'
	printf '                    tint with; that slice needs the image.\n'
elif [ "$PC_SET" -lt "$NSEL" ]; then
	printf 'Q2 colour today   : PARTIAL — %s of %s sampled series carried a hex primaryColor\n' "$PC_SET" "$NSEL"
	printf '                    (secondaryColor: %s of %s). A colour-only slice would have to\n' "$SC_SET" "$NSEL"
	printf '                    fall back for the rest, so it is not zero-fetch.\n'
elif [ "$PC_DISTINCT" -le 1 ] && [ "$NSEL" -gt 1 ]; then
	printf 'Q2 colour today   : POPULATED BUT USELESS — all %s sampled series carry the SAME\n' "$NSEL"
	printf '                    primaryColor. The field is filled and carries no information.\n'
elif [ "$PC_BLACK" -eq "$PC_SET" ]; then
	printf 'Q2 colour today   : POPULATED BUT USELESS — every primaryColor is all-zero.\n'
else
	printf 'Q2 colour today   : USABLE — %s of %s sampled series carry a hex primaryColor and\n' "$PC_SET" "$NSEL"
	printf '                    %s of those values are distinct (secondaryColor: %s of %s).\n' "$PC_DISTINCT" "$SC_SET" "$NSEL"
	printf '                    A tinted placeholder can render with no image fetched at all.\n'
fi

# Q3
if [ "$N_COVER_OK" -eq 0 ]; then
	printf 'Q3 format & size  : UNANSWERED — no cover returned 200.\n'
else
	CT_N="$({ sort -u "$WORK/cov_ct" | wc -l || true; } | tr -d ' ')"
	CT_ONE="$(sort "$WORK/cov_ct" | uniq -c | sort -rn | head -n1 | sed 's/^ *[0-9]* *//')"
	KIB="$(awk -v b="$SZ_MED" 'BEGIN{printf "%.0f", b/1024}')"
	if [ "$CT_N" = "1" ]; then
		printf 'Q3 format & size  : %s, median %s bytes (~%s KiB) over %s covers.\n' \
			"$CT_ONE" "$SZ_MED" "$KIB" "$N_COVER_OK"
		if [ "$MAGIC" != "unchecked" ]; then
			printf '                    The first bytes of the last 200 body say: %s.\n' "$MAGIC"
		fi
	else
		printf 'Q3 format & size  : MIXED — %s distinct Content-Type values across %s covers,\n' "$CT_N" "$N_COVER_OK"
		printf '                    most common %s. Median %s bytes (~%s KiB).\n' "$CT_ONE" "$SZ_MED" "$KIB"
	fi
fi

# revalidation
case "$REVAL" in
etag) printf 'Q3 revalidation   : AVAILABLE — an ETag came back and a conditional re-request\n                    earned a 304. UsArr can store the validator and re-check cheaply.\n' ;;
last-modified) printf 'Q3 revalidation   : AVAILABLE via Last-Modified — If-Modified-Since earned a 304.\n                    No usable ETag, so store the timestamp.\n' ;;
*)
	if [ "$ETAG_SEEN" -gt 0 ] || [ "$LM_SEEN" -gt 0 ]; then
		printf 'Q3 revalidation   : NOT AVAILABLE IN PRACTICE — a validator header is present\n'
		printf '                    (ETag on %s, Last-Modified on %s of %s covers) but the\n' "$ETAG_SEEN" "$LM_SEEN" "$N_COVER_OK"
		printf '                    conditional request did NOT earn a 304. Do not build a\n'
		printf '                    revalidation path on a header the server ignores.\n'
	else
		printf 'Q3 revalidation   : ABSENT — no ETag and no Last-Modified. Every refetch is a\n'
		printf '                    full body; caching must be time-based, not validator-based.\n'
	fi
	;;
esac

# Q4
if [ -n "$NOCOVER_STATUS" ]; then
	if [ "$NOCOVER_STATUS" = "404" ]; then
		printf 'Q4 missing cover  : 404 — a coverless series is a clean negative. Cache it as\n'
		printf '                    absent rather than retrying it on every sync.\n'
	elif [ "$NOCOVER_STATUS" = "200" ]; then
		printf 'Q4 missing cover  : 200 with a %s-byte body (%s) — Kavita serves a PLACEHOLDER,\n' \
			"$NOCOVER_BYTES" "${NOCOVER_CT:-no content-type}"
		printf '                    not an error. UsArr cannot tell "has a cover" from "has\n'
		printf '                    none" by status alone; it needs the coverImage field, or a\n'
		printf '                    size/hash check against the placeholder.\n'
	else
		printf 'Q4 missing cover  : %s — %s\n' "$NOCOVER_STATUS" "$(explain_code "$NOCOVER_STATUS")"
	fi
elif [ "$SCANNED" != "0" ] && [ "$SCANNED_NULL" = "0" ]; then
	printf 'Q4 missing cover  : UNDETERMINED — all %s series on the scanned page have a\n' "$SCANNED"
	printf '                    coverImage, so there was no coverless series to ask about.\n'
	printf '                    Not guessed. (A nonexistent series id returned %s, which is a\n' "$GHOST_STATUS"
	printf '                    different question and is not the answer to this one.)\n'
else
	printf 'Q4 missing cover  : UNDETERMINED — no coverless series could be identified.\n'
fi

# cold start
if [ "$COV_MED" != "-" ]; then
	N_EST="${TOTAL_SERIES:-}"
	if [ -n "$N_EST" ]; then
		serial="$(awk -v m="$COV_MED" -v n="$N_EST" 'BEGIN{printf "%.1f", m*n/1000}')"
		conc="$(awk -v m="$COV_MED" -v n="$N_EST" 'BEGIN{printf "%.1f", m*n/4000}')"
		bytes="$(awk -v s="$SZ_MED" -v n="$N_EST" 'BEGIN{printf "%.1f", s*n/1048576}')"
		printf 'cold start        : median %s ms per cover -> ~%ss serially over %s series,\n' "$COV_MED" "$serial" "$N_EST"
		printf '                    ~%ss at 4 concurrent, ~%s MiB of image bytes.\n' "$conc" "$bytes"
	else
		printf 'cold start        : median %s ms per cover; the series count is unknown (no\n' "$COV_MED"
		printf '                    Pagination header), so no total is given.\n'
	fi
else
	printf 'cold start        : UNANSWERED — no cover fetch was timed.\n'
fi

printf '\n'
printf 'Nothing above names a library, a series or a path. The hex colours are the one\n'
printf 'deliberate exception and they name nothing. Paste it as it stands.\n'

# The exit status is deliberately 0 for every verdict a real run can produce,
# PASS and FAIL alike: both are successful probe runs. Only a run that could not
# reach or authenticate Kavita exits non-zero, because that is the only case
# where there is nothing to read.
exit 0
