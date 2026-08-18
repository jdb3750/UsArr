#!/usr/bin/env bash
#
# kavita-volume-walk-probe.sh — does Kavita's Series/volumes read carry its files?
#
# UsArr, ADR-0041 · docs/reference/sync.md §2 · docs/REVIEW-LOG.md LS-200
#
# WHY THIS EXISTS
#   A spec says a field exists. It does not say which code path fills it.
#   `GET /api/Series/volumes` declares `VolumeDto.chapters[].files[]`, and if the
#   handler actually populates it, UsArr's volume-and-chapter walk costs ONE
#   request per series. If it comes back null or empty, the same walk costs one
#   request per CHAPTER, which is a different design. Nothing in this repo can
#   answer that; only a live instance can.
#
# WHAT IT SENDS
#   Read-only GETs and one read-only POST, to the Kavita base URL you supply, and
#   to nothing else. No writes, no scans, no metadata refresh. Your Auth Key
#   travels in the `x-api-key` request header — the one scheme Kavita's OpenAPI
#   document declares (AuthKey, in: header) — and goes nowhere but that host.
#
# WHAT IT PRINTS — this output is meant to be safe to paste into a chat
#   Counts, distributions and classifications ONLY. It never prints your Auth
#   Key, your base URL, a library name, a series title, or a file path. Where the
#   SHAPE of a path matters it prints a classification ("absolute", "relative",
#   "mount-like first segment: 9 of 9") and never the string itself. Series are
#   identified by sample slot ("sample 3"), never by id or name.
#
#   It writes no file anywhere except a private temp directory it deletes on
#   exit, and it never writes your Auth Key to any file at all.
#
# USAGE
#   KAVITA_URL=http://127.0.0.1:5000 KAVITA_API_KEY=... ./kavita-volume-walk-probe.sh
#   KAVITA_URL=... ./kavita-volume-walk-probe.sh --sample 8
#
#   The key is read from KAVITA_API_KEY, or prompted for silently if that is
#   unset. There is deliberately no --api-key flag: a credential on a command
#   line lands in your shell history and in the process table.
#
# REQUIREMENTS
#   bash, curl. jq is optional: without it the probe still answers question 1 —
#   the load-bearing one — from a byte count of the response, and says plainly
#   which questions it could not answer.
#
# Licensed under AGPL-3.0, with the rest of UsArr.

set -euo pipefail

readonly PROBE_VERSION="1"
readonly SPEC="api/specs/kavita-v0.9.0.2.json"

SAMPLE=5
TIMEOUT=30

usage() {
	cat <<'USAGE'
kavita-volume-walk-probe.sh — does Kavita's Series/volumes read carry its files?

  KAVITA_URL=http://127.0.0.1:5000 KAVITA_API_KEY=... ./kavita-volume-walk-probe.sh
  KAVITA_URL=... ./kavita-volume-walk-probe.sh --sample 8

  --sample N    how many series to sample, spread across the library (default 5)
  --timeout S   per-request timeout in seconds (default 30)
  --url U       Kavita base URL, if you would rather not export KAVITA_URL
  -h, --help    this text

The Auth Key comes from KAVITA_API_KEY, or a silent prompt. There is no
--api-key flag on purpose: a credential on a command line lands in your shell
history and in the process table.

Read-only. Prints counts, distributions and classifications only — never the
key, the URL, a library name, a series title or a file path. Safe to paste.
The full rationale is in the comment block at the top of this file.
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

HAVE_JQ=0
if command -v jq >/dev/null 2>&1; then HAVE_JQ=1; fi

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

WORK="$(mktemp -d "${TMPDIR:-/tmp}/kavita-probe.XXXXXX")"
chmod 700 "$WORK"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------------------
# HTTP. Every call goes through here so that exactly one place holds the key,
# and so that no error path can print a URL or a header.
# ---------------------------------------------------------------------------

# kget <path-with-query> <body-file> — GET. Echoes "<http_code> <seconds>".
kget() {
	local path="$1" body="$2" out
	# No --fail-with-body: it makes curl exit non-zero on 4xx, which would throw
	# away the very status code that tells the operator what to fix.
	if ! out="$(curl -sS -o "$body" \
		-w '%{http_code} %{time_total}' \
		--max-time "$TIMEOUT" \
		-H "x-api-key: ${KEY}" \
		-H 'Accept: application/json, text/plain;q=0.9, */*;q=0.1' \
		-D "$WORK/hdr" \
		"${BASE}${path}" 2>"$WORK/curlerr")"; then
		# curl's own message can carry the URL; the body can carry library data.
		# Neither is printed. Only the code and the classification are.
		printf '000 0\n'
		return 0
	fi
	printf '%s\n' "$out"
}

# kpost <path-with-query> <json-body> <body-file> — POST. Same contract.
kpost() {
	local path="$1" payload="$2" body="$3" out
	if ! out="$(curl -sS -o "$body" \
		-w '%{http_code} %{time_total}' \
		--max-time "$TIMEOUT" \
		-H "x-api-key: ${KEY}" \
		-H 'Content-Type: application/json' \
		-H 'Accept: application/json, text/plain;q=0.9, */*;q=0.1' \
		-D "$WORK/hdr" \
		--data-binary "$payload" \
		"${BASE}${path}" 2>"$WORK/curlerr")"; then
		printf '000 0\n'
		return 0
	fi
	printf '%s\n' "$out"
}

# countmatches <extended-basic-regex> <file> — how many times the pattern occurs.
#
# `|| true` is inside the braces on purpose: `set -o pipefail` is on, and grep
# exits 1 when it matches nothing, which here is a real answer ("no chapter
# carried files[]") rather than a failure.
countmatches() {
	{ grep -o -- "$1" "$2" || true; } | wc -l | tr -d ' '
}

code_of() { printf '%s' "${1%% *}"; }
secs_of() { printf '%s' "${1##* }"; }

explain_code() {
	case "$1" in
	000) printf 'no HTTP response (connection, TLS or timeout). The probe does not print curl'"'"'s message because it carries the URL.' ;;
	401) printf 'unauthenticated — the Auth Key was rejected. Kavita User Settings -> Manage Auth Keys.' ;;
	403) printf 'forbidden — the key authenticated but the account lacks rights for this endpoint.' ;;
	404) printf 'not found — wrong base URL, a reverse-proxy path prefix, or an endpoint this release does not have.' ;;
	*) printf 'HTTP %s' "$1" ;;
	esac
}

# ---------------------------------------------------------------------------
# Preamble — printed before the first packet leaves.
# ---------------------------------------------------------------------------

printf '=== UsArr Kavita volume-walk probe v%s ===\n' "$PROBE_VERSION"
printf 'Read-only. Sends GET/POST to your Kavita only. Auth Key travels in the\n'
printf 'x-api-key header and is never printed, never logged, never written to a file.\n'
printf 'Output is counts, distributions and classifications only: no URL, no library\n'
printf 'name, no series title, no file path. Safe to paste.\n'
printf 'Endpoints verified against %s before this script was written.\n' "$SPEC"
if [ "$HAVE_JQ" -eq 0 ]; then
	printf '\n!! jq is not installed. Question 1 is still answered (from response byte\n'
	printf '!! counts); questions 2, 3, 4 and 5 are reported UNANSWERED. `apt-get install jq`\n'
	printf '!! and re-run for the full set.\n'
fi
printf '\n'

# ---------------------------------------------------------------------------
# 0. Reachability and auth.
# ---------------------------------------------------------------------------

printf -- '--- 0. reachability ---\n'
r="$(kget "/api/Health" "$WORK/health")"
hc="$(code_of "$r")"
if [ "$hc" = "200" ]; then
	printf 'GET /api/Health              : 200 OK\n'
else
	printf 'GET /api/Health              : %s — %s\n' "$hc" "$(explain_code "$hc")"
fi

FILTER_BODY='{"id":0,"name":null,"statements":[],"combination":1,"sortOptions":{"sortField":1,"isAscending":true},"entityType":0,"limitTo":0}'

r="$(kpost "/api/Series/all-v2?PageNumber=1&PageSize=1&context=1" "$FILTER_BODY" "$WORK/probe1")"
hc="$(code_of "$r")"
if [ "$hc" != "200" ]; then
	printf 'POST /api/Series/all-v2      : %s — %s\n' "$hc" "$(explain_code "$hc")"
	printf '\nVERDICT: INCONCLUSIVE — could not authenticate a series read. Nothing below ran.\n'
	exit 1
fi
printf 'POST /api/Series/all-v2      : 200 OK (authenticated)\n'

# The `Pagination` response header is middleware, not in the OpenAPI document, so
# it is read when present and never required.
TOTAL_SERIES=""
if [ -f "$WORK/hdr" ]; then
	pag="$(tr -d '\r' <"$WORK/hdr" | sed -n 's/^[Pp]agination: *//p' | tail -n1)"
	if [ -n "$pag" ]; then
		TOTAL_SERIES="$(printf '%s' "$pag" | sed -n 's/.*"totalItems" *: *\([0-9]\{1,\}\).*/\1/p')"
	fi
fi
if [ -n "$TOTAL_SERIES" ]; then
	printf 'series in library (Pagination header): %s\n' "$TOTAL_SERIES"
else
	printf 'series in library            : UNKNOWN (no Pagination header — a reverse proxy may strip it)\n'
fi
printf '\n'

# ---------------------------------------------------------------------------
# 1. Pick the sample — spread across the library, not the first N.
# ---------------------------------------------------------------------------

printf -- '--- 1. sample selection ---\n'
IDS=""
NSEL=0

pick_by_page() {
	# One PageSize=1 request per sample slot, at evenly spread page numbers.
	local total="$1" k page r hc id
	k=1
	while [ "$k" -le "$SAMPLE" ]; do
		page=$(((2 * k - 1) * total / (2 * SAMPLE) + 1))
		[ "$page" -ge 1 ] || page=1
		[ "$page" -le "$total" ] || page="$total"
		r="$(kpost "/api/Series/all-v2?PageNumber=${page}&PageSize=1&context=1" "$FILTER_BODY" "$WORK/pick")"
		hc="$(code_of "$r")"
		if [ "$hc" = "200" ]; then
			if [ "$HAVE_JQ" -eq 1 ]; then
				id="$(jq -r '.[0].id // empty' <"$WORK/pick")"
			else
				id="$(tr -d ' \n' <"$WORK/pick" | sed -n 's/^\[{"id":\([0-9]\{1,\}\).*/\1/p')"
			fi
			if [ -n "$id" ]; then
				IDS="$IDS $id"
				NSEL=$((NSEL + 1))
			fi
		fi
		k=$((k + 1))
	done
}

pick_from_one_page() {
	# Fallback when the total is unknown: one page, ids taken at spread indices.
	local r hc n i idx id
	r="$(kpost "/api/Series/all-v2?PageNumber=1&PageSize=200&context=1" "$FILTER_BODY" "$WORK/pick")"
	hc="$(code_of "$r")"
	[ "$hc" = "200" ] || return 0
	if [ "$HAVE_JQ" -eq 1 ]; then
		n="$(jq 'length' <"$WORK/pick")"
	else
		n="$(jq_free_ids_count "$WORK/pick")"
	fi
	[ "${n:-0}" -gt 0 ] || return 0
	i=1
	while [ "$i" -le "$SAMPLE" ]; do
		idx=$(((2 * i - 1) * n / (2 * SAMPLE)))
		[ "$idx" -lt "$n" ] || idx=$((n - 1))
		if [ "$HAVE_JQ" -eq 1 ]; then
			id="$(jq -r --argjson i "$idx" '.[$i].id // empty' <"$WORK/pick")"
		else
			id="$({ grep -o '"id":[0-9]\{1,\}' "$WORK/pick" || true; } | sed -n "$((idx + 1))p" | cut -d: -f2)"
		fi
		if [ -n "$id" ]; then
			IDS="$IDS $id"
			NSEL=$((NSEL + 1))
		fi
		i=$((i + 1))
	done
}

jq_free_ids_count() { countmatches '"id":[0-9]\{1,\}' "$1"; }

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
printf '\n'

# ---------------------------------------------------------------------------
# 2. The walk.
# ---------------------------------------------------------------------------

: >"$WORK/summaries.ndjson"
: >"$WORK/vol_times"
: >"$WORK/meta_times"
: >"$WORK/nojq"

SUM_VOL=0
SUM_CHAP=0
SUM_NONEMPTY=0
SUM_NULL=0
SUM_EMPTY=0
SUM_FILES=0

printf -- '--- 2. GET /api/Series/volumes?seriesId=N, one call per sampled series ---\n'
printf '%-9s %8s %9s %14s %12s %8s\n' "slot" "volumes" "chapters" "chapters with" "files" "ms"
printf '%-9s %8s %9s %14s %12s %8s\n' "" "" "" "files[] set" "total" ""

slot=0
for id in $IDS; do
	slot=$((slot + 1))
	r="$(kget "/api/Series/volumes?seriesId=${id}" "$WORK/vol")"
	hc="$(code_of "$r")"
	ms="$(awk -v s="$(secs_of "$r")" 'BEGIN{printf "%.0f", s*1000}')"
	if [ "$hc" != "200" ]; then
		printf '%-9s %8s %9s %14s %12s %8s   <- %s\n' "sample $slot" "-" "-" "-" "-" "-" "$(explain_code "$hc")"
		continue
	fi
	printf '%s\n' "$ms" >>"$WORK/vol_times"

	if [ "$HAVE_JQ" -eq 1 ]; then
		jq -c '
			def cls($p):
				if $p == null then "null"
				elif ($p | test("^[A-Za-z]:[\\\\/]")) then "windows-absolute"
				elif ($p | startswith("/")) then "absolute"
				elif ($p | startswith("\\\\")) then "unc"
				else "relative" end;
			def mountish($p):
				($p | ltrimstr("/") | split("/") | .[0] // "" | ascii_downcase) as $s
				| ["mnt","media","srv","data","storage","share","shares","tank","pool",
				   "nas","mount","export","exports","volumes","vol1","volume1","volume2",
				   "volume3","array1","docker","mergerfs"]
				| index($s) != null;
			[ .[] ] as $vols
			| [ $vols[] | (.chapters // [])[] ] as $chs
			| [ $chs[] | (.files // [])[] ] as $fs
			| [ $fs[] | .filePath ] as $ps
			| {
				volumes:          ($vols | length),
				chapters:         ($chs | length),
				files_null:       ([ $chs[] | select(.files == null) ] | length),
				files_empty:      ([ $chs[] | select(.files != null and (.files | length) == 0) ] | length),
				files_nonempty:   ([ $chs[] | select(.files != null and (.files | length) > 0) ] | length),
				files:            ($fs | length),
				path_null:        ([ $ps[] | select(. == null) ] | length),
				cls:              ([ $ps[] | cls(.) ]),
				mountish:         ([ $ps[] | select(. != null) | select(mountish(.)) ] | length),
				lens:             ([ $ps[] | select(. != null) | length ]),
				segs:             ([ $ps[] | select(. != null) | (ltrimstr("/") | split("/") | length) ])
			  }' <"$WORK/vol" >>"$WORK/summaries.ndjson" || {
			printf '%-9s   <- response was not the expected JSON array\n' "sample $slot"
			continue
		}
		line="$(tail -n1 "$WORK/summaries.ndjson")"
		v="$(printf '%s' "$line" | jq -r '.volumes')"
		c="$(printf '%s' "$line" | jq -r '.chapters')"
		ne="$(printf '%s' "$line" | jq -r '.files_nonempty')"
		f="$(printf '%s' "$line" | jq -r '.files')"
		nnull="$(printf '%s' "$line" | jq -r '.files_null')"
		nempty="$(printf '%s' "$line" | jq -r '.files_empty')"
		SUM_NULL=$((SUM_NULL + nnull))
		SUM_EMPTY=$((SUM_EMPTY + nempty))
	else
		# No jq. These three counts are exact regardless of formatting, because
		# each key occurs in exactly one DTO in this payload: `chapters` only in
		# VolumeDto, `files` only in ChapterDto, `filePath` only in MangaFileDto.
		v="$(countmatches '"chapters"' "$WORK/vol")"
		c="$(countmatches '"files"' "$WORK/vol")"
		f="$(countmatches '"filePath"' "$WORK/vol")"
		ne="$(countmatches '"files":[[:space:]]*\[[[:space:]]*{' "$WORK/vol")"
		nnull="$(countmatches '"files":[[:space:]]*null' "$WORK/vol")"
		nempty="$(countmatches '"files":[[:space:]]*\[[[:space:]]*\]' "$WORK/vol")"
		SUM_NULL=$((SUM_NULL + nnull))
		SUM_EMPTY=$((SUM_EMPTY + nempty))
		printf '%s %s %s\n' "$c" "$ne" "$f" >>"$WORK/nojq"
	fi

	SUM_VOL=$((SUM_VOL + v))
	SUM_CHAP=$((SUM_CHAP + c))
	SUM_NONEMPTY=$((SUM_NONEMPTY + ne))
	SUM_FILES=$((SUM_FILES + f))
	printf '%-9s %8s %9s %14s %12s %8s\n' "sample $slot" "$v" "$c" "$ne" "$f" "$ms"
done
printf '%-9s %8s %9s %14s %12s\n' "TOTAL" "$SUM_VOL" "$SUM_CHAP" "$SUM_NONEMPTY" "$SUM_FILES"
printf 'chapters whose files[] was JSON null: %s · [] empty array: %s\n' "$SUM_NULL" "$SUM_EMPTY"
printf '\n'

# ---------------------------------------------------------------------------
# 3. Path shape — classification only, never a path.
# ---------------------------------------------------------------------------

printf -- '--- 3. path shape (classification only; no path is printed) ---\n'
if [ "$HAVE_JQ" -eq 0 ]; then
	printf 'UNANSWERED — needs jq.\n\n'
elif [ "$SUM_FILES" -eq 0 ]; then
	printf 'UNANSWERED — no files[] entries were returned, so there were no paths to classify.\n\n'
else
	jq -s -r '
		def med: sort | if length == 0 then "-"
			elif (length % 2) == 1 then (.[(length-1)/2] | tostring)
			else (((.[length/2 - 1] + .[length/2]) / 2) | tostring) end;
		def mn: if length == 0 then "-" else (min | tostring) end;
		def mx: if length == 0 then "-" else (max | tostring) end;
		([ .[].cls[] ]) as $c
		| ([ .[].lens[] ]) as $l
		| ([ .[].segs[] ]) as $s
		| ([ .[].mountish ] | add // 0) as $m
		| ([ .[].path_null ] | add // 0) as $pn
		| "filePath values seen        : \($c | length)"
		, "  absolute (starts with /)  : \([$c[] | select(. == "absolute")] | length)"
		, "  windows-absolute (C:\\)    : \([$c[] | select(. == "windows-absolute")] | length)"
		, "  UNC (\\\\host\\share)         : \([$c[] | select(. == "unc")] | length)"
		, "  relative                  : \([$c[] | select(. == "relative")] | length)"
		, "  JSON null                 : \($pn)"
		, "mount-like first segment    : \($m) of \([$c[] | select(. != "null")] | length)"
		, "  (first path segment is one of mnt media srv data storage share shares tank"
		, "   pool nas mount export exports volumes vol1 volume1 volume2 volume3 array1"
		, "   docker mergerfs — the classification, never the segment, is printed)"
		, "path length in chars        : min \($l | mn) · median \($l | med) · max \($l | mx)"
		, "path depth in segments      : min \($s | mn) · median \($s | med) · max \($s | mx)"
	' <"$WORK/summaries.ndjson"
	printf '\n'
fi

# ---------------------------------------------------------------------------
# 4. Row volume.
# ---------------------------------------------------------------------------

printf -- '--- 4. row volume (media_file rows a full walk would write) ---\n'
if [ "$HAVE_JQ" -eq 1 ] && [ -s "$WORK/summaries.ndjson" ]; then
	jq -s -r '
		def med: sort | if length == 0 then "-"
			elif (length % 2) == 1 then (.[(length-1)/2] | tostring)
			else (((.[length/2 - 1] + .[length/2]) / 2) | tostring) end;
		([ .[].files ]) as $f
		| ([ .[].chapters ]) as $c
		| "files per series            : min \($f | min) · median \($f | med) · max \($f | max)"
		, "chapters per series         : min \($c | min) · median \($c | med) · max \($c | max)"
		, "sampled series              : \(length)"
	' <"$WORK/summaries.ndjson"
elif [ "$HAVE_JQ" -eq 0 ] && [ -s "$WORK/nojq" ]; then
	sort -n -k3 "$WORK/nojq" | awk '
		{ f[NR]=$3; c[NR]=$1 }
		END {
			if (NR == 0) { print "UNANSWERED"; exit }
			printf "files per series            : min %d · median %d · max %d\n", f[1], f[int((NR+1)/2)], f[NR]
			printf "sampled series              : %d\n", NR
		}'
else
	printf 'UNANSWERED.\n'
fi
printf '\n'

# ---------------------------------------------------------------------------
# 5. Free-tier metadata fields.
# ---------------------------------------------------------------------------

printf -- '--- 5. GET /api/Series/metadata?seriesId=N — is an honest "43 of 60" possible? ---\n'
META_STATUS_SET=0
META_TOTAL_SET=0
META_GATE=0
META_N=0
if [ "$HAVE_JQ" -eq 0 ]; then
	printf 'UNANSWERED — needs jq.\n\n'
else
	slot=0
	for id in $IDS; do
		slot=$((slot + 1))
		r="$(kget "/api/Series/metadata?seriesId=${id}" "$WORK/meta")"
		hc="$(code_of "$r")"
		[ "$hc" = "200" ] || {
			printf 'sample %-3s : %s\n' "$slot" "$(explain_code "$hc")"
			continue
		}
		awk -v s="$(secs_of "$r")" 'BEGIN{printf "%.0f\n", s*1000}' >>"$WORK/meta_times"
		ps="$(jq -r '.publicationStatus // 0' <"$WORK/meta")"
		tc="$(jq -r '.totalCount // 0' <"$WORK/meta")"
		mc="$(jq -r '.maxCount // 0' <"$WORK/meta")"
		case "$ps" in
		0) psn="Ongoing" ;;
		1) psn="Hiatus" ;;
		2) psn="Completed" ;;
		3) psn="Cancelled" ;;
		4) psn="Ended" ;;
		*) psn="unknown($ps)" ;;
		esac
		META_N=$((META_N + 1))
		# publicationStatus is a non-nullable int and 0 is a real member (Ongoing),
		# so "populated" cannot be read off the value. What CAN be read is whether
		# the library uses more than the default, so the distribution is printed.
		if [ "$ps" != "0" ]; then META_STATUS_SET=$((META_STATUS_SET + 1)); fi
		if [ "$tc" != "0" ]; then META_TOTAL_SET=$((META_TOTAL_SET + 1)); fi
		if [ "$tc" != "0" ] && { [ "$ps" = "2" ] || [ "$ps" = "3" ] || [ "$ps" = "4" ]; }; then
			META_GATE=$((META_GATE + 1))
		fi
		printf 'sample %-3s : publicationStatus=%-9s totalCount=%-5s maxCount=%s\n' "$slot" "$psn" "$tc" "$mc"
	done
	printf '\n'
	printf 'series read                 : %s\n' "$META_N"
	printf 'publicationStatus non-default (not Ongoing=0): %s\n' "$META_STATUS_SET"
	printf 'totalCount > 0              : %s\n' "$META_TOTAL_SET"
	printf 'both, per ARCHITECTURE.md §6.1 (status in {Completed,Cancelled,Ended} AND a total): %s of %s\n' "$META_GATE" "$META_N"
	printf '\n'
fi

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
	if [ -s "$1" ]; then awk 'NR==1||$1<m{m=$1} END{printf "%d", m}' "$1"; else printf -- '-'; fi
}
max_file() {
	if [ -s "$1" ]; then awk '$1>m{m=$1} END{printf "%d", m}' "$1"; else printf -- '-'; fi
}

VOL_MED="$(med_file "$WORK/vol_times")"
META_MED="$(med_file "$WORK/meta_times")"

printf -- '--- 6. request cost (wall clock, this network, this moment) ---\n'
printf 'GET /Series/volumes  ms     : min %s · median %s · max %s (n=%s)\n' \
	"$(min_file "$WORK/vol_times")" "$VOL_MED" "$(max_file "$WORK/vol_times")" "$(wc -l <"$WORK/vol_times" | tr -d ' ')"
printf 'GET /Series/metadata ms     : min %s · median %s · max %s (n=%s)\n' \
	"$(min_file "$WORK/meta_times")" "$META_MED" "$(max_file "$WORK/meta_times")" "$(wc -l <"$WORK/meta_times" | tr -d ' ')"
printf '\n'

# ---------------------------------------------------------------------------
# 7. Verdict.
# ---------------------------------------------------------------------------

N_SERIES="${TOTAL_SERIES:-}"
printf -- '=== VERDICT ===\n'

# Q1
if [ "$SUM_CHAP" -eq 0 ]; then
	printf 'Q1 walk cost      : INCONCLUSIVE — the sampled series returned no chapters at all.\n'
	Q1=inconclusive
elif [ "$SUM_NONEMPTY" -eq "$SUM_CHAP" ]; then
	printf 'Q1 walk cost      : PASS — every one of the %s sampled chapters carried a non-empty\n' "$SUM_CHAP"
	printf '                    files[]. The walk is ONE request per SERIES.\n'
	Q1=pass
elif [ "$SUM_NONEMPTY" -eq 0 ]; then
	printf 'Q1 walk cost      : FAIL — none of the %s sampled chapters carried files[] (%s null,\n' "$SUM_CHAP" "$SUM_NULL"
	printf '                    %s empty). The walk costs ONE request per CHAPTER.\n' "$SUM_EMPTY"
	Q1=fail
else
	printf 'Q1 walk cost      : MIXED — %s of %s sampled chapters carried files[]. Do not design\n' "$SUM_NONEMPTY" "$SUM_CHAP"
	printf '                    for either shape until the split is understood; re-run with a\n'
	printf '                    larger --sample first.\n'
	Q1=mixed
fi

# Q2
if [ "$HAVE_JQ" -eq 0 ]; then
	printf 'Q2 path exposure  : UNANSWERED — needs jq.\n'
elif [ "$SUM_FILES" -eq 0 ]; then
	printf 'Q2 path exposure  : UNANSWERED — no paths were returned.\n'
else
	abs="$(jq -s -r '[ .[].cls[] | select(. == "absolute" or . == "windows-absolute" or . == "unc") ] | length' <"$WORK/summaries.ndjson")"
	tot="$(jq -s -r '[ .[].cls[] | select(. != "null") ] | length' <"$WORK/summaries.ndjson")"
	if [ "$abs" = "$tot" ] && [ "$tot" != "0" ]; then
		printf 'Q2 path exposure  : HOST-REVEALING — all %s paths are absolute. Storing them verbatim\n' "$tot"
		printf '                    puts the host filesystem layout in UsArr'"'"'s database and in\n'
		printf '                    anything that renders a media_file row.\n'
	elif [ "$abs" = "0" ]; then
		printf 'Q2 path exposure  : LOW — none of the %s paths is absolute; they are library-relative.\n' "$tot"
	else
		printf 'Q2 path exposure  : MIXED — %s of %s paths are absolute.\n' "$abs" "$tot"
	fi
fi

# Q3
if [ "$NSEL" -gt 0 ] && [ "$SUM_FILES" -gt 0 ]; then
	if [ -n "$N_SERIES" ]; then
		est=$((SUM_FILES * N_SERIES / NSEL))
		printf 'Q3 row volume     : ~%s media_file rows for %s series (mean %s files per sampled\n' "$est" "$N_SERIES" "$((SUM_FILES / NSEL))"
		printf '                    series x the real series count). A sample of %s, so treat it as\n' "$NSEL"
		printf '                    an order of magnitude, not a number.\n'
	else
		printf 'Q3 row volume     : mean %s files per sampled series; the library total is unknown\n' "$((SUM_FILES / NSEL))"
		printf '                    because no Pagination header came back.\n'
	fi
else
	printf 'Q3 row volume     : UNANSWERED — no files were returned.\n'
fi

# Q4
if [ "$HAVE_JQ" -eq 1 ] && [ "$META_N" -gt 0 ]; then
	if [ "$META_GATE" -eq 0 ]; then
		printf 'Q4 "43 of 60"     : NOT POSSIBLE on this sample — 0 of %s series met the gate, so the\n' "$META_N"
		printf '                    UI shows a bare count ("43 issues") for all of them.\n'
	elif [ "$META_GATE" -eq "$META_N" ]; then
		printf 'Q4 "43 of 60"     : POSSIBLE for all %s sampled series.\n' "$META_N"
	else
		printf 'Q4 "43 of 60"     : POSSIBLE for %s of %s sampled series; the other %s get a bare count.\n' \
			"$META_GATE" "$META_N" "$((META_N - META_GATE))"
	fi
else
	printf 'Q4 "43 of 60"     : UNANSWERED.\n'
fi

# Q5
if [ "$VOL_MED" != "-" ] && [ -n "$N_SERIES" ]; then
	serial="$(awk -v m="$VOL_MED" -v n="$N_SERIES" 'BEGIN{printf "%.1f", m*n/1000}')"
	conc="$(awk -v m="$VOL_MED" -v n="$N_SERIES" 'BEGIN{printf "%.1f", m*n/4000}')"
	printf 'Q5 walk wall-clock: median %s ms per volumes call -> ~%ss serially over %s series,\n' "$VOL_MED" "$serial" "$N_SERIES"
	printf '                    ~%ss at 4 concurrent (sync.md §2'"'"'s bound). Excludes the\n' "$conc"
	if [ "$META_MED" = "-" ]; then
		printf '                    per-series metadata call, which was not measured on this run.\n'
	else
		printf '                    per-series metadata call, which measured a median of %s ms.\n' "$META_MED"
	fi
elif [ "$VOL_MED" != "-" ]; then
	printf 'Q5 walk wall-clock: median %s ms per volumes call; series count unknown, so no total.\n' "$VOL_MED"
else
	printf 'Q5 walk wall-clock: UNANSWERED.\n'
fi

printf '\n'
printf 'Nothing above names a library, a series or a path. Paste it as it stands.\n'

# The exit status is deliberately 0 for PASS, FAIL and MIXED alike: all three are
# successful probe runs. Only a run that could not reach or authenticate Kavita
# exits non-zero, because that is the only case where there is nothing to read.
exit 0
