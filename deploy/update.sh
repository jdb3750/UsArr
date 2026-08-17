#!/usr/bin/env bash
# deploy/update.sh — update a UsArr install from its checkout, or fail loudly.
#
# The failure this exists to prevent: a host that quietly keeps running an old
# build. Every step of pull → build → install → restart can succeed while
# leaving the live process on the previous binary — a diverged checkout that
# merges instead of fast-forwarding, a `go build` that ships no SPA, an
# `install` that replaces the inode under a process nobody restarted. So every
# step here is checked, and the last thing the script does is prove the RUNNING
# process is the commit the checkout is on. Anything less is a rumour.
#
# No sudo anywhere: the documented target (docs/DEVELOPMENT.md §12) is a Debian
# LXC administered as root, where the sudo binary is not installed at all.
#
# Environment:
#   USARR_CHECKOUT      git checkout to update           (default /opt/UsArr)
#   USARR_INSTALL_PATH  where the binary is installed    (default /usr/local/bin/usarr)
#   USARR_SERVICE       systemd unit name                (default usarr)
#   USARR_BRANCH        branch that must be checked out  (default main)
#   USARR_ALLOW_DIRTY   set to 1 to build with modified TRACKED files (default unset).
#                       Untracked files never block an update.
#
# Usage: deploy/update.sh [--check]
#   --check  report whether an update is available and exit; change nothing.

set -euo pipefail

CHECKOUT="${USARR_CHECKOUT:-/opt/UsArr}"
INSTALL_PATH="${USARR_INSTALL_PATH:-/usr/local/bin/usarr}"
SERVICE="${USARR_SERVICE:-usarr}"
BRANCH="${USARR_BRANCH:-main}"
ALLOW_DIRTY="${USARR_ALLOW_DIRTY:-}"

CHECK_ONLY=0
for arg in "$@"; do
	case "$arg" in
	--check) CHECK_ONLY=1 ;;
	-h | --help)
		# The header comment IS the help text, printed by rule rather than by
		# line number so editing the header cannot silently truncate --help.
		awk 'NR>1 && /^#/ { sub(/^#( |$)/, ""); print; next } NR>1 { exit }' "$0"
		exit 0
		;;
	*)
		printf 'update: unknown argument %s (try --check or --help)\n' "$arg" >&2
		exit 2
		;;
	esac
done

step() { printf '\n==> %s\n' "$*"; }
info() { printf '    %s\n' "$*"; }
die() {
	printf '\nupdate: FAILED: %s\n' "$1" >&2
	shift
	for line in "$@"; do printf '  %s\n' "$line" >&2; done
	exit 1
}

# ── 1. the checkout ──────────────────────────────────────────────────────────
# Refusing to guess is the point: run from the wrong directory and every later
# step would still "succeed", against the wrong tree.
[ -d "$CHECKOUT" ] || die "no such directory: $CHECKOUT" \
	"Set USARR_CHECKOUT to the UsArr git checkout on this host."
cd "$CHECKOUT"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 ||
	die "$CHECKOUT is not a git checkout" \
		"Set USARR_CHECKOUT to the UsArr git checkout on this host."
# A git repo is not necessarily THIS repo, and `make build` in the wrong one
# fails in a way that reads like a toolchain problem.
if [ ! -f "$CHECKOUT/go.mod" ] || [ ! -f "$CHECKOUT/Makefile" ]; then
	die "$CHECKOUT is a git checkout but not UsArr (no go.mod + Makefile)"
fi

step "checkout: $CHECKOUT"

# ── 2. clean tree, right branch ──────────────────────────────────────────────
# TRACKED and UNTRACKED are two different questions and only one of them is a
# reason to stop. A modified tracked file can make the fast-forward below fail
# or lose work; an untracked scratch file cannot do either. Servers accumulate
# untracked cruft — a probe script left in the checkout root is the normal case,
# not a fault — and aborting on it would train the operator to pass
# USARR_ALLOW_DIRTY=1 every time, which would disable the check that matters.
TRACKED_DIRTY="$(git status --porcelain --untracked-files=no)"
UNTRACKED="$(git ls-files --others --exclude-standard)"

if [ -n "$TRACKED_DIRTY" ]; then
	if [ "$ALLOW_DIRTY" = "1" ]; then
		info "tracked files are modified; USARR_ALLOW_DIRTY=1, continuing:"
		printf '%s\n' "$TRACKED_DIRTY" | sed 's/^/      /'
	else
		printf '\nupdate: FAILED: tracked files are modified in %s\n' "$CHECKOUT" >&2
		printf '%s\n' "$TRACKED_DIRTY" | sed 's/^/  /' >&2
		printf '  Commit, stash or discard these, or set USARR_ALLOW_DIRTY=1.\n' >&2
		printf '  (Untracked files are ignored and never block an update.)\n' >&2
		exit 1
	fi
fi

if [ -n "$UNTRACKED" ]; then
	info "$(printf '%s\n' "$UNTRACKED" | wc -l | tr -d ' ') untracked file(s) present; ignored"
	# If the build output itself is showing up as untracked, .gitignore is
	# wrong and every future run would report cruft that is only the last
	# build. Worth one line, because the fix is one line.
	if printf '%s\n' "$UNTRACKED" | grep -qx 'usarr'; then
		info "NOTE: ./usarr is NOT gitignored — add '/usarr' to .gitignore"
	fi
fi

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
[ "$CURRENT_BRANCH" = "$BRANCH" ] ||
	die "HEAD is on '$CURRENT_BRANCH', not '$BRANCH'" \
		"Run: git -C $CHECKOUT switch $BRANCH" \
		"Or set USARR_BRANCH=$CURRENT_BRANCH if that is deliberate."

# ── 3. fetch, then fast-forward only ─────────────────────────────────────────
# --ff-only, never a plain pull: a diverged checkout must be a loud stop, not a
# merge commit invented on a production host.
step "fetching origin/$BRANCH"
git fetch --quiet origin "$BRANCH" || die "git fetch failed (network? credentials?)"

OLD_HEAD="$(git rev-parse HEAD)"
OLD_SHORT="$(git rev-parse --short HEAD)"
REMOTE_HEAD="$(git rev-parse FETCH_HEAD)"
BEHIND="$(git rev-list --count "HEAD..$REMOTE_HEAD")"
AHEAD="$(git rev-list --count "$REMOTE_HEAD..HEAD")"

if [ "$AHEAD" != "0" ] && [ "$BEHIND" != "0" ]; then
	die "checkout has diverged from origin/$BRANCH ($AHEAD local, $BEHIND remote)" \
		"This host has commits origin does not. Resolve it by hand:" \
		"  git -C $CHECKOUT log --oneline $REMOTE_HEAD..HEAD"
fi

if [ "$BEHIND" = "0" ]; then
	info "already up to date at $OLD_SHORT"
	if [ "$CHECK_ONLY" = "1" ]; then
		printf '\nupdate: nothing to pull. Run deploy/status.sh to check the installed binary.\n'
		exit 0
	fi
	# Deliberately NOT an exit: the checkout being current says nothing about
	# whether the installed binary or the running process match it. Rebuilding
	# and re-verifying is the whole point of the run.
	info "continuing anyway — the binary or the running process may still be stale"
else
	if [ "$CHECK_ONLY" = "1" ]; then
		printf '\nupdate: %s commit(s) available (%s -> %s). Re-run without --check to apply.\n' \
			"$BEHIND" "$OLD_SHORT" "$(git rev-parse --short "$REMOTE_HEAD")"
		exit 0
	fi
	step "fast-forwarding $BEHIND commit(s)"
	git merge --ff-only "$REMOTE_HEAD" ||
		die "fast-forward failed" "The checkout cannot advance cleanly; resolve it by hand."
fi

NEW_HEAD="$(git rev-parse HEAD)"
NEW_SHORT="$(git rev-parse --short HEAD)"
if [ "$OLD_HEAD" = "$NEW_HEAD" ]; then
	info "HEAD unchanged: $NEW_SHORT"
else
	info "HEAD $OLD_SHORT -> $NEW_SHORT ($BEHIND commit(s))"
	git --no-pager log --oneline "$OLD_HEAD..$NEW_HEAD" | sed 's/^/      /'
fi

# ── 4. build ─────────────────────────────────────────────────────────────────
# `make build`, never `go build`: the SPA is embedded by the web-build
# prerequisite, and a bare `go build` silently produces a binary that 404s on /.
step "building (make build)"
make build ||
	die "make build failed" \
		"The installed binary and the running service are UNTOUCHED." \
		"Fix the build, then re-run this script."

BUILT="$CHECKOUT/usarr"
[ -x "$BUILT" ] || die "make build succeeded but $BUILT is missing or not executable"

# ── 5. prove the build is actually this commit ───────────────────────────────
# The Makefile stamps `git rev-parse --short HEAD` into main.commit. If the two
# disagree, something served a cached or stale artifact and installing it would
# reintroduce exactly the bug this script exists to prevent.
step "verifying the freshly built binary"
BUILT_VERSION="$("$BUILT" --version)" ||
	die "$BUILT --version failed" \
		"A binary that cannot report its own identity is too old to verify." \
		"Check that the checkout really is at $NEW_SHORT and rebuild."
BUILT_COMMIT="$(printf '%s\n' "$BUILT_VERSION" | sed -n 's/^commit:[[:space:]]*//p')"
printf '%s\n' "$BUILT_VERSION" | sed 's/^/      /'

[ "$BUILT_COMMIT" = "$NEW_SHORT" ] ||
	die "the built binary reports commit '$BUILT_COMMIT', checkout is at '$NEW_SHORT'" \
		"That is a stale build. Nothing was installed." \
		"Try: make -C $CHECKOUT clean && $0"

# ── 6. install ───────────────────────────────────────────────────────────────
step "installing to $INSTALL_PATH"
install -m755 "$BUILT" "$INSTALL_PATH" ||
	die "install to $INSTALL_PATH failed (are you root?)"
info "installed $(sha256sum "$INSTALL_PATH" | cut -c1-12)…"

# ── 7. restart ───────────────────────────────────────────────────────────────
# A skipped restart is the classic silent failure, so an absent systemctl or an
# unknown unit stops the script rather than letting it report success.
command -v systemctl >/dev/null 2>&1 ||
	die "systemctl not found, so the service was NOT restarted" \
		"The NEW binary is installed at $INSTALL_PATH but the OLD one is still running." \
		"Restart UsArr however this host runs it, then verify with: deploy/status.sh"

systemctl cat "$SERVICE" >/dev/null 2>&1 ||
	die "systemd knows no unit named '$SERVICE', so the service was NOT restarted" \
		"The NEW binary is installed at $INSTALL_PATH but the OLD one is still running." \
		"Set USARR_SERVICE to the real unit name, or restart it by hand, then run deploy/status.sh"

# Recorded before the restart so the journal query below cannot pick up lines
# from the previous run.
RESTART_MARK="$(date '+%Y-%m-%d %H:%M:%S')"
step "restarting $SERVICE"
systemctl restart "$SERVICE" || die "systemctl restart $SERVICE failed" \
	"Look at: systemctl status $SERVICE ; journalctl -u $SERVICE -n 50"

# ── 8. prove the RUNNING process is the new build ────────────────────────────
step "verifying the running service"
# The unit can report active before the process has logged its startup line.
for _ in 1 2 3 4 5 6 7 8 9 10; do
	if [ "$(systemctl is-active "$SERVICE" || true)" = "active" ]; then
		break
	fi
	sleep 1
done

ACTIVE="$(systemctl is-active "$SERVICE" || true)"
[ "$ACTIVE" = "active" ] ||
	die "$SERVICE is '$ACTIVE' after the restart" \
		"Look at: systemctl status $SERVICE ; journalctl -u $SERVICE -n 50"

MAIN_PID="$(systemctl show -p MainPID --value "$SERVICE")"
if [ -z "$MAIN_PID" ] || [ "$MAIN_PID" = "0" ]; then
	die "$SERVICE is active but reports no MainPID"
fi
info "MainPID $MAIN_PID"

# A trailing "(deleted)" means this process holds an inode that has since been
# replaced on disk — the exact signature of "rebuilt and installed, never
# restarted". After a restart it must not appear.
EXE="$(readlink "/proc/$MAIN_PID/exe" 2>/dev/null || true)"
if [ -z "$EXE" ]; then
	info "cannot read /proc/$MAIN_PID/exe (permissions?); relying on the journal below"
else
	info "exe $EXE"
	case "$EXE" in
	*"(deleted)")
		die "the running process is on a DELETED inode: $EXE" \
			"It is still executing the previous binary. Restart $SERVICE again."
		;;
	esac
fi

# The startup log line carries the commit (cmd/usarr/main.go, "starting UsArr").
# This is the only check that reads the identity out of the process itself
# rather than off the disk, so it is the one that actually settles the question.
LIVE_COMMIT=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
	LOGLINE="$(journalctl -u "$SERVICE" --since "$RESTART_MARK" --no-pager 2>/dev/null |
		grep -F 'starting UsArr' | tail -n 1 || true)"
	if [ -n "$LOGLINE" ]; then
		# Matches both handlers: text (commit=abc1234) and JSON ("commit":"abc1234").
		LIVE_COMMIT="$(printf '%s\n' "$LOGLINE" |
			sed -n 's/.*commit["=:]*[ "]*\([0-9a-f][0-9a-f]*\).*/\1/p')"
		break
	fi
	sleep 1
done

if [ -z "$LIVE_COMMIT" ]; then
	die "could not read a 'starting UsArr' line from the journal since $RESTART_MARK" \
		"The service is active, but its build identity is unverified." \
		"Look at: journalctl -u $SERVICE --since '$RESTART_MARK' -n 50"
fi

[ "$LIVE_COMMIT" = "$NEW_SHORT" ] ||
	die "the RUNNING process reports commit '$LIVE_COMMIT', expected '$NEW_SHORT'" \
		"The update did not take effect. Look at: journalctl -u $SERVICE -n 50"

printf '\nupdate: OK — %s is running %s (%s)\n' "$SERVICE" "$NEW_SHORT" "$INSTALL_PATH"
