#!/usr/bin/env bash
#
# greeter-test.sh — validate that ottoman can drive the GDM login screen's Mutter.
#
# The GDM greeter runs its own gnome-shell/Mutter instance as the `gdm` user.
# This harness drops a one-shot into the greeter's autostart directory so we can
# see, from inside that session, whether we can (a) run as gdm, (b) reach the
# greeter's org.gnome.Mutter.DisplayConfig, and (c) apply one of your saved
# layouts to it. It's the proof-of-concept behind the greeter integration; it is
# NOT part of the shipped agent.
#
# Usage:
#   scripts/greeter-test.sh setup            # read-only probe (no display change)
#   scripts/greeter-test.sh setup with-tv    # also APPLY the named layout
#   <log out and back in — the probe runs while the login screen is shown>
#   scripts/greeter-test.sh result           # print what the probe logged
#   scripts/greeter-test.sh cleanup          # remove everything, restore backup
#   scripts/greeter-test.sh recover          # restore + restart GDM (see below)
#
# Safety:
#   * `setup` with no layout changes nothing about your displays — it only reads
#     Mutter state. Pass a layout name only once the read-only run looks good.
#   * Applying a layout at the greeter is PERSISTENT (it writes the greeter's
#     monitors.xml), so this script backs that file up first. Pick a layout that
#     keeps your login monitor on — e.g. `with-tv`, NOT `tv`/`with-tv-primary`,
#     which could move the login screen onto an off display.
#   * If the login screen ever looks wrong: press Ctrl+Alt+F3 for a text console,
#     log in, and run:  sudo scripts/greeter-test.sh recover
#
set -euo pipefail

TESTDIR=/var/lib/ottoman-greeter-test
GDMHOME="$TESTDIR/home"
LAYOUTFILE="$TESTDIR/layout"
LOG="$TESTDIR/probe.log"
PROBE=/usr/local/bin/ottoman-greeter-probe.sh
BIN=/usr/local/bin/ottoman
GREETER_AUTOSTART=/usr/share/gdm/greeter/autostart
DESKTOP="$GREETER_AUTOSTART/zz-ottoman-greeter-test.desktop"

SUDO=""; [ "$(id -u)" -eq 0 ] || SUDO="sudo"

# Resolve the real (non-root) user even when invoked via sudo.
REAL_USER="${SUDO_USER:-$(id -un)}"
REAL_HOME="$(getent passwd "$REAL_USER" | cut -d: -f6)"

# Resolve the gdm account and its greeter monitors.xml.
GDM_USER=""
for u in gdm Debian-gdm; do
	if getent passwd "$u" >/dev/null 2>&1; then GDM_USER="$u"; break; fi
done
GDM_MON=""
if [ -n "$GDM_USER" ]; then
	GDM_MON="$(getent passwd "$GDM_USER" | cut -d: -f6)/.config/monitors.xml"
fi
GDM_MON_BAK="${GDM_MON}.ottoman-test-bak"

die() { echo "error: $*" >&2; exit 1; }

write_probe() {
	# The one-shot that runs inside the greeter session as gdm.
	$SUDO tee "$PROBE" >/dev/null <<'PROBE_EOF'
#!/bin/sh
TESTDIR=/var/lib/ottoman-greeter-test
LOG="$TESTDIR/probe.log"
LAYOUT="$(cat "$TESTDIR/layout" 2>/dev/null || true)"
export HOME="$TESTDIR/home"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"

state() {
	gdbus call --session --dest org.gnome.Mutter.DisplayConfig \
		--object-path /org/gnome/Mutter/DisplayConfig \
		--method org.gnome.Mutter.DisplayConfig.GetCurrentState 2>&1
}

{
	echo "=== greeter probe ran as $(id -un) (uid $(id -u)) ==="
	echo "session: type=${XDG_SESSION_TYPE:-?} wayland=${WAYLAND_DISPLAY:-?} bus=${DBUS_SESSION_BUS_ADDRESS:-none}"
	if gdbus wait --session --timeout 30 org.gnome.Mutter.DisplayConfig 2>/dev/null; then
		echo "mutter DisplayConfig: reachable"
	else
		echo "mutter DisplayConfig: NOT reachable (gdbus wait failed)"
		echo "=== done (could not reach Mutter) ==="
		exit 0
	fi
	sleep 3
	if [ -z "$LAYOUT" ]; then
		echo "--- read-only, no layout applied ---"
		echo "current serial: $(state | head -c 60)"
	else
		echo "--- layouts visible to gdm ---"
		/usr/local/bin/ottoman agent layout list 2>&1
		echo "serial BEFORE: $(state | head -c 60)"
		echo "--- applying layout: $LAYOUT ---"
		/usr/local/bin/ottoman agent layout apply "$LAYOUT" 2>&1
		echo "apply exit=$?"
		echo "serial AFTER (should differ): $(state | head -c 60)"
	fi
	echo "=== done ==="
} >> "$LOG" 2>&1
PROBE_EOF
	$SUDO chmod 0755 "$PROBE"
}

write_desktop() {
	$SUDO tee "$DESKTOP" >/dev/null <<EOF
[Desktop Entry]
Type=Application
Name=Ottoman greeter test
Exec=$PROBE
NoDisplay=true
X-GNOME-Autostart-enabled=true
EOF
}

cmd_setup() {
	local layout="${1:-}"

	command -v gdbus >/dev/null || die "gdbus not found (install glib2 tools)"
	[ -d "$GREETER_AUTOSTART" ] || die "no greeter autostart dir at $GREETER_AUTOSTART (is this GDM?)"
	[ -n "$GDM_MON" ] || die "could not locate the gdm user / its monitors.xml"

	# Locate the ottoman binary to run as gdm.
	local binsrc=""
	if [ -x "$REAL_HOME/.local/bin/ottoman" ]; then
		binsrc="$REAL_HOME/.local/bin/ottoman"
	else
		binsrc="$(command -v ottoman || true)"
	fi
	[ -n "$binsrc" ] && [ -x "$binsrc" ] || die "can't find an ottoman binary (build/install it, or put it on PATH)"

	echo "==> resetting $TESTDIR"
	$SUDO rm -rf "$TESTDIR"
	$SUDO install -d -m 0777 "$TESTDIR"
	echo -n "$layout" | $SUDO tee "$LAYOUTFILE" >/dev/null

	echo "==> backing up the greeter's monitors.xml (if any)"
	if $SUDO test -f "$GDM_MON_BAK"; then
		# Never clobber an existing backup — re-running setup must keep the
		# pristine original, not overwrite it with an already-tested state.
		echo "    backup already exists, keeping it: $GDM_MON_BAK"
	elif [ -f "$GDM_MON" ]; then
		$SUDO cp "$GDM_MON" "$GDM_MON_BAK"
		echo "    backup: $GDM_MON_BAK"
	else
		echo "    (none yet at $GDM_MON)"
	fi

	echo "==> installing ottoman binary -> $BIN  (from $binsrc)"
	$SUDO cp "$binsrc" "$BIN"
	$SUDO chmod 0755 "$BIN"

	echo "==> giving gdm a private copy of your ottoman config + layouts"
	$SUDO install -d -m 0700 "$GDMHOME/.config/ottoman" "$GDMHOME/.local/share/ottoman"
	if [ -d "$REAL_HOME/.config/ottoman" ]; then
		$SUDO cp -a "$REAL_HOME/.config/ottoman/." "$GDMHOME/.config/ottoman/"
	else
		echo "    warn: no $REAL_HOME/.config/ottoman (agent may fail to load config)"
	fi
	if [ -d "$REAL_HOME/.local/share/ottoman" ]; then
		$SUDO cp -a "$REAL_HOME/.local/share/ottoman/." "$GDMHOME/.local/share/ottoman/"
	else
		echo "    warn: no $REAL_HOME/.local/share/ottoman (no saved layouts?)"
	fi
	$SUDO chown -R "$GDM_USER" "$GDMHOME"

	echo "==> installing greeter probe + autostart entry"
	write_probe
	write_desktop

	echo
	if [ -z "$layout" ]; then
		echo "READ-ONLY probe armed (no display change)."
	else
		echo "APPLY probe armed for layout: $layout"
		echo "  (persistent: writes the greeter's monitors.xml — backed up above)"
	fi
	echo
	echo "Next:"
	echo "  1. Log out (or: $SUDO systemctl restart gdm3) to show the login screen."
	echo "  2. Wait ~10s at the login screen, then log back in."
	echo "  3. Run:  scripts/greeter-test.sh result"
	echo
	echo "If the login screen looks wrong: Ctrl+Alt+F3, log in, then"
	echo "  sudo scripts/greeter-test.sh recover"
}

cmd_result() {
	[ -f "$LOG" ] || die "no log at $LOG yet — did you log out and back in after setup?"
	cat "$LOG"
}

cmd_cleanup() {
	echo "==> removing greeter probe + autostart entry"
	$SUDO rm -f "$DESKTOP" "$PROBE" "$BIN"
	if [ -n "$GDM_MON" ] && [ -f "$GDM_MON_BAK" ]; then
		echo "==> restoring greeter monitors.xml from backup"
		$SUDO cp "$GDM_MON_BAK" "$GDM_MON"
		$SUDO rm -f "$GDM_MON_BAK"
	fi
	echo "==> removing $TESTDIR"
	$SUDO rm -rf "$TESTDIR"
	echo "Done. The greeter reverts to its old layout on the next login screen"
	echo "(or run: $SUDO systemctl restart gdm3)."
}

cmd_recover() {
	echo "==> emergency recover: restore greeter layout + restart GDM"
	$SUDO rm -f "$DESKTOP" "$PROBE"
	if [ -n "$GDM_MON" ] && [ -f "$GDM_MON_BAK" ]; then
		$SUDO cp "$GDM_MON_BAK" "$GDM_MON"
	fi
	$SUDO systemctl restart gdm3 2>/dev/null || $SUDO systemctl restart gdm
}

case "${1:-}" in
	setup)   shift; cmd_setup "${1:-}" ;;
	result)  cmd_result ;;
	cleanup) cmd_cleanup ;;
	recover) cmd_recover ;;
	*)
		grep '^#' "$0" | sed 's/^# \{0,1\}//' | sed -n '2,40p'
		exit 1
		;;
esac
