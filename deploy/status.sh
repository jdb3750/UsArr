#!/usr/bin/env bash
# deploy/status.sh — "am I running the latest main?", answered in one command.
#
# Read-only. It installs nothing, restarts nothing and never touches the working
# tree; the single side effect is `git fetch`, which updates remote-tracking refs
# so "how far behind?" can be answered at all. Set USARR_NO_FETCH=1 to skip even
# that and compare against whatever origin ref is already on disk.
#
# There are THREE places a UsArr deployment can be stale, and they fail
# independently, which is why they are reported separately: the checkout can be
# behind origin, the installed binary can be older than the checkout, and the
# running process can be older than the installed binary (install replaces the
# inode; a process that was never restarted keeps the old one open).
#
# Environment:
#   USARR_CHECKOUT      git checkout                    (default /opt/UsArr)
#   USARR_INSTALL_PATH  installed binary                (default /usr/local/bin/usarr)
#   USARR_SERVICE       systemd unit name               (default usarr)
#   USARR_BRANCH        branch to compare against       (default main)
#   USARR_NO_FETCH      set to 1 to skip `git fetch`    (default unset)

set -euo pipefail

CHECKOUT="${USARR_CHECKOUT:-/opt/UsArr}"
INSTALL_PATH="${USARR_INSTALL_PATH:-/usr/local/bin/usarr}"
SERVICE="${USARR_SERVICE:-usarr}"
BRANCH="${USARR_BRANCH:-main}"

field() { printf '  %-22s %s\n' "$1" "$2"; }

# Unknown values stay as this literal rather than an empty string, so a missing
# reading is visible in the output instead of looking like agreement.
UNKNOWN="(unknown)"

# ── the checkout ─────────────────────────────────────────────────────────────
printf 'checkout\n'
CHECKOUT_HEAD="$UNKNOWN"
BEHIND="$UNKNOWN"
TRACKED_DIRTY_COUNT=0
if [ -d "$CHECKOUT" ] && git -C "$CHECKOUT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	field "path" "$CHECKOUT"
	field "branch" "$(git -C "$CHECKOUT" rev-parse --abbrev-ref HEAD)"
	CHECKOUT_HEAD="$(git -C "$CHECKOUT" rev-parse --short HEAD)"
	field "HEAD" "$CHECKOUT_HEAD"

	if [ "${USARR_NO_FETCH:-}" = "1" ]; then
		field "fetch" "skipped (USARR_NO_FETCH=1)"
	elif ! git -C "$CHECKOUT" fetch --quiet origin "$BRANCH" 2>/dev/null; then
		field "fetch" "FAILED — the remote comparison below may be stale"
	fi

	if REMOTE="$(git -C "$CHECKOUT" rev-parse --short "origin/$BRANCH" 2>/dev/null)"; then
		field "origin/$BRANCH" "$REMOTE"
		BEHIND="$(git -C "$CHECKOUT" rev-list --count "HEAD..origin/$BRANCH" 2>/dev/null || echo "$UNKNOWN")"
		AHEAD="$(git -C "$CHECKOUT" rev-list --count "origin/$BRANCH..HEAD" 2>/dev/null || echo 0)"
		field "commits behind" "$BEHIND"
		[ "$AHEAD" = "0" ] || field "commits ahead" "$AHEAD (this host has local commits)"
	else
		field "origin/$BRANCH" "$UNKNOWN (no such remote ref)"
	fi

	# Two counts, never one. An untracked scratch file on a server says nothing
	# about whether the install is current, and folding it into a single "dirty"
	# number would make a normal checkout look broken. Only the tracked count
	# reaches the verdict below.
	TRACKED_DIRTY_COUNT="$(git -C "$CHECKOUT" status --porcelain --untracked-files=no | wc -l | tr -d ' ')"
	UNTRACKED_COUNT="$(git -C "$CHECKOUT" ls-files --others --exclude-standard | wc -l | tr -d ' ')"
	field "modified (tracked)" "$TRACKED_DIRTY_COUNT"
	field "untracked files" "$UNTRACKED_COUNT (normal on a server; not a staleness signal)"
else
	field "path" "$CHECKOUT — NOT a git checkout"
fi

# ── the installed binary ─────────────────────────────────────────────────────
printf '\ninstalled binary\n'
BINARY_COMMIT="$UNKNOWN"
if [ -f "$INSTALL_PATH" ]; then
	field "path" "$INSTALL_PATH"
	field "mtime" "$(date -r "$INSTALL_PATH" '+%Y-%m-%d %H:%M:%S %Z' 2>/dev/null || echo "$UNKNOWN")"
	# A binary predating the --version flag exits non-zero here; that is a
	# reading in itself ("older than this feature"), not an error to abort on.
	if VERSION_OUT="$("$INSTALL_PATH" --version 2>/dev/null)"; then
		printf '%s\n' "$VERSION_OUT" | sed 's/^/      /'
		BINARY_COMMIT="$(printf '%s\n' "$VERSION_OUT" |
			sed -n 's/^commit:[[:space:]]*//p')"
		[ -n "$BINARY_COMMIT" ] || BINARY_COMMIT="$UNKNOWN"
	else
		field "--version" "not supported — this binary predates the flag, so it is OLD"
	fi
else
	field "path" "$INSTALL_PATH — NOT PRESENT"
fi

# ── the running process ──────────────────────────────────────────────────────
printf '\nrunning service\n'
LIVE_COMMIT="$UNKNOWN"
EXE_DELETED=0
UNIT_ACTIVE="$UNKNOWN"
if ! command -v systemctl >/dev/null 2>&1; then
	field "systemctl" "not found — cannot inspect a service on this host"
elif ! systemctl cat "$SERVICE" >/dev/null 2>&1; then
	field "unit" "$SERVICE — no such unit (set USARR_SERVICE)"
else
	field "unit" "$SERVICE"
	UNIT_ACTIVE="$(systemctl is-active "$SERVICE" 2>/dev/null || true)"
	[ -n "$UNIT_ACTIVE" ] || UNIT_ACTIVE=inactive
	field "active" "$UNIT_ACTIVE"
	MAIN_PID="$(systemctl show -p MainPID --value "$SERVICE" 2>/dev/null || echo 0)"
	field "MainPID" "${MAIN_PID:-0}"

	if [ -n "${MAIN_PID:-}" ] && [ "${MAIN_PID:-0}" != "0" ]; then
		EXE="$(readlink "/proc/$MAIN_PID/exe" 2>/dev/null || echo "$UNKNOWN")"
		field "exe" "$EXE"
		case "$EXE" in
		*"(deleted)")
			EXE_DELETED=1
			field "" "^ the binary on disk was REPLACED under this process"
			;;
		esac

		# The startup line is the only source that reports what the process
		# itself was built from, rather than what is sitting on disk.
		#
		# Selected by _PID and read FORWARDS, with no line window at all.
		#
		# Any "last N lines" window is wrong in the direction that matters. The
		# line is emitted exactly once, at start, so on a host that has since
		# logged past N it falls out of range and a perfectly current
		# deployment reports UNVERIFIED — the steady state on a long-running
		# box, not an edge case. Measured: an -n 2000 window did precisely
		# that. Raising N only moves the threshold.
		#
		# _PID pins the query to the process actually running, and the startup
		# line is among the first things that process logs, so `grep -m1` stops
		# after a handful of lines however long ago it started. journalctl
		# takes SIGPIPE when grep exits; `|| true` absorbs it under pipefail.
		LOGLINE="$(journalctl _PID="$MAIN_PID" --no-pager 2>/dev/null |
			grep -m1 -F 'starting UsArr' || true)"
		if [ -z "$LOGLINE" ]; then
			# Fallback for a unit whose logging PID is not MainPID (a wrapper
			# script, Type=forking). Windowed from the END on purpose: without
			# a PID to pin it, reading forwards would find the OLDEST startup
			# line in the unit's history rather than the current one.
			LOGLINE="$(journalctl -u "$SERVICE" --no-pager -n 5000 2>/dev/null |
				grep -F 'starting UsArr' | tail -n 1 || true)"
		fi
		if [ -n "$LOGLINE" ]; then
			LIVE_COMMIT="$(printf '%s\n' "$LOGLINE" |
				sed -n 's/.*commit["=:]*[ "]*\([0-9a-f][0-9a-f]*\).*/\1/p')"
			[ -n "$LIVE_COMMIT" ] || LIVE_COMMIT="$UNKNOWN"
		fi
		field "commit (journal)" "$LIVE_COMMIT"
	fi
fi

# ── the verdict ──────────────────────────────────────────────────────────────
# One line, and it names the FIRST stale link in the chain, because fixing an
# outer link without the inner one changes nothing an operator can observe.
printf '\n'
# `if` rather than `test && array+=`: under `set -e` a failing left-hand test in
# an && list is only exempt by a rule subtle enough not to rely on here.
STALE=()
# A unit that is not running is the loudest answer to "am I current?" there is,
# and it must not fall through to UNVERIFIED just because a dead process leaves
# no MainPID and therefore no journal reading. Reporting "a reading could not be
# taken" for a service that is simply down would bury the actual problem.
if [ "$UNIT_ACTIVE" != "$UNKNOWN" ] && [ "$UNIT_ACTIVE" != "active" ]; then
	STALE+=("$SERVICE is '$UNIT_ACTIVE' — nothing is serving, whatever is installed")
fi
if [ "$BEHIND" != "$UNKNOWN" ] && [ "$BEHIND" != "0" ]; then
	STALE+=("checkout is $BEHIND commit(s) behind origin/$BRANCH")
fi
if [ "$BINARY_COMMIT" != "$UNKNOWN" ] && [ "$CHECKOUT_HEAD" != "$UNKNOWN" ] &&
	[ "$BINARY_COMMIT" != "$CHECKOUT_HEAD" ]; then
	STALE+=("installed binary is $BINARY_COMMIT, checkout is $CHECKOUT_HEAD")
fi
if [ "$LIVE_COMMIT" != "$UNKNOWN" ] && [ "$BINARY_COMMIT" != "$UNKNOWN" ] &&
	[ "$LIVE_COMMIT" != "$BINARY_COMMIT" ]; then
	STALE+=("running process is $LIVE_COMMIT, installed binary is $BINARY_COMMIT")
fi
if [ "$EXE_DELETED" = "1" ]; then
	STALE+=("running process holds a deleted inode — it was never restarted")
fi
# Tracked modifications only. They mean the built binary cannot be reproduced
# from origin, which is a real deviation worth naming. Untracked files are
# deliberately absent from this list.
if [ "$TRACKED_DIRTY_COUNT" != "0" ]; then
	STALE+=("$TRACKED_DIRTY_COUNT tracked file(s) modified — this build does not match origin/$BRANCH")
fi

if [ "${#STALE[@]}" -gt 0 ]; then
	printf 'VERDICT: STALE\n'
	for s in "${STALE[@]}"; do printf '  - %s\n' "$s"; done
	printf '  Fix: deploy/update.sh\n'
	exit 1
fi

if [ "$LIVE_COMMIT" = "$UNKNOWN" ] || [ "$BINARY_COMMIT" = "$UNKNOWN" ] ||
	[ "$CHECKOUT_HEAD" = "$UNKNOWN" ] || [ "$BEHIND" = "$UNKNOWN" ]; then
	# Not a pass. A missing reading cannot confirm anything, and reporting
	# "up to date" off two readings out of four is exactly the false green this
	# script exists to remove.
	printf 'VERDICT: UNVERIFIED — one or more readings above are %s\n' "$UNKNOWN"
	exit 2
fi

printf 'VERDICT: up to date — checkout, binary and running process are all at %s\n' "$CHECKOUT_HEAD"
