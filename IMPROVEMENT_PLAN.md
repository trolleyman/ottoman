# Ottoman Improvement Plan

Context: controller runs on `ottoman.home` (Raspberry Pi Zero 2 W); the agent runs on this
desktop — Zorin OS, **GNOME 46 on Wayland**, MSI **B650M GAMING PLUS WIFI** (AM5), NVIDIA dGPU +
AMD iGPU, Realtek onboard 2.5GbE, PipeWire audio. Two displays connected: one DisplayPort
monitor and one HDMI TV (OLED).

Scope: Linux agent first; Windows parity later.

---

## 1. Wake-on-LAN is broken

The Go code (`internal/controller/wol.go`) builds and broadcasts a correct magic packet, so the
problem is almost certainly machine configuration, not ottoman. WoL needs **all** of these layers
working:

### 1a. BIOS (MSI B650M GAMING PLUS WIFI)

Enter BIOS (Del at boot) → press F7 for Advanced mode:

1. **Settings → Advanced → Wake Up Event Setup → `Resume By PCI-E Device` → Enabled.**
   This is the setting that covers the onboard Realtek LAN (there is no separate
   "Wake on LAN" entry on this board).
2. **Settings → Advanced → Power Management Setup → `ErP Ready` → Disabled.**
   ErP cuts standby power to the NIC when the machine is off — with it enabled, WoL from
   powered-off (S5) can never work. If present, also leave `Deep Sleep` off.

### 1b. Linux NIC configuration

`nmcli` reports the active ethernet connection has `802-3-ethernet.wake-on-lan = default`,
which on Realtek/r8169 usually resolves to *disabled* — NetworkManager re-applies this on every
boot, so a one-off `ethtool -s ... wol g` doesn't stick. Persistent fix:

```bash
# Check current state (run on the host, not in a sandbox):
sudo ethtool enp6s0 | grep -i wake     # want: "Supports Wake-on: ...g...", "Wake-on: g"

# Make it persistent via NetworkManager:
nmcli connection show                   # find the wired connection name
nmcli connection modify "<name>" 802-3-ethernet.wake-on-lan magic
nmcli connection up "<name>"
```

> Note: the interface name/MAC observed inside the dev sandbox is namespaced; verify the real
> interface name on the host first (`ip -br link`).

### 1c. Verifying end-to-end

- From the Pi: `tcpdump -i eth0 udp port 9` on another LAN machine while pressing Wake, to
  confirm the packet leaves the Pi and arrives on the LAN.
- Test from suspend first (easier), then from full shutdown (needs 1a).
- If the machine dual-boots Windows: Windows Fast Startup leaves the NIC in a state where WoL
  often fails — disable Fast Startup and enable "Wake on Magic Packet" in Device Manager.

### 1d. Code improvements (small)

- `/api/wake` currently fire-and-forgets. Add a response detailing which interface(s)/broadcast
  addresses the packet was sent on, and optionally poll the agent's `/health` afterwards so the
  UI can show "sent → booting → online".
- Config sanity check: warn at startup if the configured MAC doesn't parse or the broadcast
  address isn't on a local subnet.

**Effort:** BIOS/nmcli fixes are configuration only (minutes once at the machine). Code
diagnostics ~half a day.

---

## 2. Wayland support (monitors, mouse/keyboard)

Both Linux backends are X11-only, which is why nothing works properly on this machine:

- `internal/display/linux.go` shells out to **xrandr** — under Wayland it talks to XWayland and
  sees one fake virtual screen; it cannot reconfigure real outputs.
- `internal/input/linux.go` shells out to **xdotool** — under Wayland it can only inject events
  into XWayland windows, not the real session.

### 2a. Displays: mutter D-Bus backend

GNOME Wayland exposes full display control via the session D-Bus API
`org.gnome.Mutter.DisplayConfig` (`GetCurrentState` / `ApplyMonitorsConfig`). Plan:

- Add a `MutterManager` implementing the existing `display.Manager` interface using
  `github.com/godbus/dbus/v5` (no CGo, no extra tools needed).
- Backend selection at startup: if `XDG_SESSION_TYPE=wayland` (or `WAYLAND_DISPLAY` set) and the
  mutter D-Bus name is present → mutter backend; else fall back to xrandr. Keeps X11 support.
- Bonus over xrandr: GetCurrentState returns vendor/product/serial per connector, so
  `Monitor.Name`/`Manufacturer` (currently empty on Linux) get real values, matching Windows.
- Note: GNOME 46 has no `gdctl` CLI (that arrived in GNOME 47), so D-Bus is the right layer, and
  it also survives a GNOME upgrade.

**Effort:** ~2–4 days including mapping layouts to `ApplyMonitorsConfig`'s
(monitors, logical-monitors, properties) structure and testing persistent vs temporary apply.

### 2b. Input: uinput backend

Replace xdotool with a virtual input device via `/dev/uinput` (e.g. `bendahl/uinput` Go lib):

- Works on Wayland, X11, and even the console — no compositor cooperation needed; supports
  relative mouse moves, clicks, scroll (incl. high-res wheel for the pixel-precise trackpad
  path), and keyboard.
- One-time host setup (needs sudo): load module + grant access, e.g.
  `/etc/udev/rules.d/99-ottoman-uinput.rules` with
  `KERNEL=="uinput", GROUP="input", MODE="0660"`, add the user to the `input` group, and
  `uinput` in `/etc/modules-load.d/`. The agent's `deploy` step can print/install this.
- Caveats: `GetPosition()` isn't knowable via uinput — the trackpad protocol is
  relative-move based, so implement absolute `MoveTo` only if actually needed (or via the
  RemoteDesktop portal later). Keyboard input is evdev scancode based, so non-QWERTY layouts
  need a keysym→scancode map for text typing.
- Alternative considered: XDG RemoteDesktop portal (`org.freedesktop.portal.RemoteDesktop`) —
  compositor-blessed and sandbox-friendly, but requires an interactive permission dialog and
  restore-token management; uinput is simpler and headless-safe for a trusted local agent.

**Effort:** ~1–2 days plus deploy-script/udev polish.

---

## 3. Brightness + display power on/off

Two very different device classes here:

### 3a. Desktop monitors → DDC/CI (not CEC)

Monitors don't speak CEC — the standard control channel for external monitors is **DDC/CI**,
which works over both HDMI and **DisplayPort** (DP AUX i2c), so the "sometimes I use
DisplayPort" case is covered.

- Brightness: VCP code `0x10` (0–100). Contrast `0x12`, input source `0x60` come for free.
- Power: VCP code `0xD6` (4 = standby/off, 1 = on) — real monitor standby, better than DPMS.
- Implementation: shell out to `ddcutil` (`detect`, `getvcp`, `setvcp`) the same way the code
  currently wraps xrandr; a pure-Go i2c implementation is possible later but not worth it now.
- Host setup (needs sudo, one-time): install `ddcutil`, load `i2c-dev`
  (`/etc/modules-load.d/i2c-dev.conf` — currently not loaded, there are no `/dev/i2c-*` nodes),
  add user to the `i2c` group. NVIDIA proprietary drivers occasionally need
  `ddcutil detect --verbose` sanity-checking, but modern drivers generally work.
- Mapping ddcutil displays (i2c bus / EDID) to ottoman's `Monitor` entries: match on EDID,
  which the mutter backend (2a) gives us — a nice reason to do 2a first.

### 3b. OLED TV → network API (preferred) or CEC via the Pi

Important constraint: **desktop GPUs (NVIDIA/AMD) do not wire up HDMI-CEC**, so the desktop
cannot send CEC at all. Options:

1. **TV network API (recommended if it's an LG/Samsung/Sony smart TV).** For LG webOS this
   gives everything in one integration: power on (Wake-on-LAN to the TV) / off, **OLED
   backlight** ("OLED Light" — the setting that actually matters for OLED, distinct from the
   `brightness` picture setting), volume/mute, and input switching. One-time pairing key stored
   in `ottoman.toml`. Runs happily from either the Pi or the desktop agent.
2. **CEC via the Pi.** The Pi Zero 2 W's HDMI port does speak CEC — if the Pi is (or can be)
   plugged into a spare HDMI input on the TV, `cec-client`/`cec-ctl` on the controller gives
   power on/off, volume up/down/mute. CEC has **no brightness control**, so this covers
   power+volume only.
3. Pulse-Eight USB-CEC adapter on the desktop (~£40) — only worth it if 1 and 2 both fail.

**Recommendation:** DDC/CI for the monitors + TV network API for the TV; CEC-via-Pi as the
fallback for non-smart TVs.

### 3c. API surface

New agent endpoints (proxied by the controller like everything else):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/monitors/brightness` | `GET`/`POST` | Get/set brightness per monitor (DDC or TV backend) |
| `/api/monitors/power` | `POST` | Turn a specific display on/off |
| `/api/tv/...` | — | TV-specific extras if needed (input, OLED light) |

Config grows a `[tv]` section (`type = "webos"`, `host`, `mac`, `pairing_key`) and the display
layer gets a per-monitor "control backend" (ddc | tv | none). Web UI: brightness slider +
power toggle per monitor card.

**Effort:** DDC/CI backend ~1–2 days. LG webOS (or equivalent) integration ~2–3 days including
pairing flow. CEC-on-Pi variant ~1–2 days.

---

## 4. TV volume control

"TV volume" is two different knobs — worth exposing both:

1. **PC output volume into the TV** (the PipeWire sink for the HDMI audio device). Easy and
   works today with no new host deps: wrap `wpctl` (`get-volume`, `set-volume`, `set-mute`,
   `set-default`) in an audio backend, expose `/api/audio/sinks` + `/api/audio/volume`.
   Also handy for switching default output between TV and speakers/headphones. ~1 day.
2. **The TV's own hardware volume** — rides on whichever TV integration lands in 3b
   (webOS API or CEC volume keys). No extra effort beyond 3b.

Web UI: a volume slider (per sink) + mute on the main page; TV hardware volume grouped with the
TV card.

---

## 5. Taskbar quick-controls (tray applet)

A tray icon on the desktop for one-click access to: layout switching, per-monitor brightness,
monitor power on/off, and volume — without opening the web UI.

### 5a. Prerequisite: a monitor registry + capability model

Right now monitors only exist as whatever `ListMonitors` returns at that moment. Quick controls
(and per-monitor settings) need persistent identity:

- **Registry** (persisted next to layouts): known monitors keyed by **EDID** (stable across
  ports/reboots; the mutter backend in 2a provides it), each with a friendly name
  (e.g. "LG OLED TV", "Dell 27\"") and its control backend (`ddc` | `tv` | `none`).
- **Capabilities** per monitor, discovered once and cached: `brightness?`, `power?`,
  `volume?` (TV), `inputs?`. Exposed via `GET /api/monitors` so *any* frontend (web UI, tray)
  renders the right controls per device.
- **Visibility config**: per-monitor, per-control `show/hide` overrides in the registry
  (e.g. hide brightness on a monitor whose DDC is flaky, hide power on the primary), editable
  from the web UI settings page and stored server-side so the tray and web UI stay consistent.

This registry is worth doing anyway — 3a/3b need the EDID↔backend mapping regardless.

### 5b. GNOME Quick Settings extension (the bottom-right panel)

Target UX: controls living in the native GNOME **Quick Settings** menu (the bottom-right
panel on Zorin, alongside volume/network/power) — not a legacy tray icon.

- **Implementation:** a GNOME Shell extension (JavaScript/GJS, GNOME 46's
  `QuickSettings.SystemIndicator` / `QuickSlider` / `QuickMenuToggle` API) that talks to the
  **local agent's HTTP REST API** (works even if the Pi is down). Ships in-repo under
  `gnome-extension/`, installed by `mage deployAgent` into
  `~/.local/share/gnome-shell/extensions/ottoman@trolleyman/`.
- **Controls**, driven entirely by the registry + capabilities from 5a (the extension has no
  device knowledge of its own):
  - a **QuickSlider per monitor** that reports `brightness` (and one for TV volume) — real
    native sliders, same look as the built-in volume slider;
  - a **QuickMenuToggle** per monitor for power on/off, and one for layouts with a submenu of
    saved layouts (radio-style);
  - hidden controls (per 5a visibility config) simply don't render.
- Caveats: extension code is GNOME-version-coupled (target 46, small shims on upgrade) and
  restarts with the shell — it should degrade gracefully (grey out) when the agent is
  unreachable.
- **Windows later:** no Quick Settings equivalent — plan a small tray icon + popup flyout
  window there instead; both frontends drive the same REST API, so it's UI-only work.
- Since the panel talks to the same REST API as the web UI, every feature above (2–4) lands in
  it for free once its endpoint exists.

**Effort:** registry + capabilities ~1–2 days (partly shared with 3a/3b); Quick Settings
extension ~2–3 days including deploy/install wiring.

---

## 6. Deploy overwrites the layout DB

Confirmed bug. Layouts aren't a separate database — they live inside the live config file as
`agent.layouts` (`~/.config/ottoman/config.toml`):

- Saving/removing a layout at runtime (`/api/layouts/save-current` etc.) rewrites the whole
  config file via `config.SaveAgent` (`internal/agent/agent.go:726`).
- `mage deployAgent` (`magefiles/magefile.go:626-630`) then unconditionally copies the
  `magefiles/deploy_agent.toml` template over that same file on **every** deploy — clobbering
  every layout saved since the template was generated (and any other config edited on the
  machine, e.g. a rotated auth token).

A second, related bug: `config.SaveAgent` only writes `listen_address`, `auth_token`, and
`layouts` (`internal/config/config.go:202-240`) — so a runtime layout save *also* silently
drops any other keys you had in the config (trackpad settings, etc.). Data and settings being
in one file hurts in both directions.

### Fix

1. **Split runtime data out of the config.** Move layouts to their own store, e.g.
   `~/.local/share/ottoman/layouts.json` (XDG data dir; `%LOCALAPPDATA%` on Windows), written
   atomically (temp file + rename). Config file becomes read-only from the agent's point of
   view; `SaveAgent`'s lossy rewrite path disappears. Migration: on first run, if the config
   contains `agent.layouts`, import them into the new store (leave config untouched).
   The monitor registry (5a) should live in the same data dir.
2. **Make deploy non-destructive about config too**: copy the template only if no config
   exists on the target; otherwise leave it alone (print a diff/hint if the template has new
   keys). Same for the controller deploy (`scp` at `magefiles/magefile.go:768-771`).

**Effort:** ~1 day including migration + doing the same for the controller path.

---

## Suggested order

0. **Layout store split + non-destructive deploy (6)** — small, and stops active data loss.
1. **WoL config fixes** (BIOS + nmcli) — no code, unblocks the core use case.
2. **Wayland display backend (2a)** — biggest functional gap on this machine.
3. **Wayland input via uinput (2b)** — restores the trackpad.
4. **PipeWire volume (4.1)** — small, high value.
5. **DDC/CI brightness+power (3a)** + the monitor registry (5a) alongside it.
6. **TV integration (3b + 4.2)** — pick approach once TV model is known.
7. **GNOME Quick Settings extension (5b)** — once the endpoints above exist.
8. Windows parity for brightness/audio later (WMI `WmiMonitorBrightnessMethods` only covers
   laptop panels; Windows will also want a DDC path via the physical-monitor Win32 API), plus
   a tray+flyout equivalent of the quick-settings panel.

## Open questions / info needed from you

- **TV brand/model?** (EDID wasn't readable from the sandbox.) If it's an LG OLED, option 3b.1
  is a clear win. Also: is the Pi physically near/connected to the TV?
- On the host (not the sandbox), the output of:
  - `sudo ethtool <iface> | grep -i wake` — confirms what the NIC supports/has enabled
  - `wpctl status` — confirms sink names for the audio backend
- Does the desktop dual-boot Windows? (Affects WoL via Fast Startup.)
