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

## Suggested order

1. **WoL config fixes** (BIOS + nmcli) — no code, unblocks the core use case.
2. **Wayland display backend (2a)** — biggest functional gap on this machine.
3. **Wayland input via uinput (2b)** — restores the trackpad.
4. **PipeWire volume (4.1)** — small, high value.
5. **DDC/CI brightness+power (3a)**.
6. **TV integration (3b + 4.2)** — pick approach once TV model is known.
7. Windows parity for brightness/audio later (WMI `WmiMonitorBrightnessMethods` only covers
   laptop panels; Windows will also want a DDC path via the physical-monitor Win32 API).

## Open questions / info needed from you

- **TV brand/model?** (EDID wasn't readable from the sandbox.) If it's an LG OLED, option 3b.1
  is a clear win. Also: is the Pi physically near/connected to the TV?
- On the host (not the sandbox), the output of:
  - `sudo ethtool <iface> | grep -i wake` — confirms what the NIC supports/has enabled
  - `wpctl status` — confirms sink names for the audio backend
- Does the desktop dual-boot Windows? (Affects WoL via Fast Startup.)
