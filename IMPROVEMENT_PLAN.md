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

### 1b. Linux NIC configuration — ✅ verified OK

Checked on the host (2026-07-04): `enp6s0` reports `Supports Wake-on: pumbg`, `Wake-on: g` —
the NIC is already armed for magic packets under Linux. The connection profile still has
`802-3-ethernet.wake-on-lan = default` (the driver default happens to be `g`); make it explicit
so a NetworkManager/driver update can't silently turn it off:

```bash
nmcli -f NAME,TYPE,DEVICE connection show --active   # find the wired profile name
nmcli connection modify "<name>" 802-3-ethernet.wake-on-lan magic
nmcli connection up "<name>"
```

**Since the Linux side is fine, the actual breakage is almost certainly 1a (BIOS) and/or 1c
(Windows dual-boot state).**

### 1c. Windows dual-boot (confirmed applicable)

The machine dual-boots Windows. The NIC's power-off state is set by whichever OS shut the
machine down, so WoL must be configured on **both** sides:

- Windows Device Manager → Realtek NIC → Advanced: enable "Wake on Magic Packet";
  Power Management tab: "Allow this device to wake the computer" + "Only allow a magic packet".
- Disable **Fast Startup** (Control Panel → Power Options → "Choose what the power buttons do").
  Fast-Startup shutdown is a hibernate hybrid that frequently leaves the Realtek NIC unable to
  wake.

### 1d. Verifying end-to-end

- From another LAN machine: `sudo tcpdump -ni <iface> udp port 9` while pressing Wake in the
  ottoman UI, to confirm the packet leaves the Pi and arrives on the LAN.
- Test matrix: wake from suspend (easiest) → from Linux shutdown (tests BIOS) → from Windows
  shutdown (tests Fast Startup/driver).

### 1e. Code improvements (small)

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

### 3b. OLED TV → LG webOS network API (decided)

The TV is an **LG OLED65A16LA** (2021 A1 series, webOS 6.0) — the LG webOS network API is the
clear choice. One integration gives everything we want:

- **Power on**: Wake-on-LAN magic packet to the TV's MAC — `8C:19:B5:72:FE:62` (reuses
  `internal/controller/wol.go`!). The TV is on Wi-Fi, which is fine: LG TVs support wake over
  their own Wi-Fi (WoWLAN), no ethernet run needed. The enabling toggle on this set lives under
  **Settings → Devices → TV Management → "TV On With Mobile"** (verified on the actual TV).
  Give the TV a DHCP reservation so its IP is stable for the SSAP
  connection. If broadcast magic packets prove unreliable through the Wi-Fi AP, send the
  packet as a subnet-directed/unicast datagram to the TV's IP as well — worth supporting both
  in the wake code anyway.
- **Power off**, **volume/mute**, **input switching**: SSAP websocket API
  (`wss://<tv>:3001`, `ssap://audio/setVolume`, `ssap://system/turnOff`, ...). One-time
  on-screen pairing prompt yields a client key stored in `ottoman.toml`.
- **OLED Light** (`backlight` in the picture-settings Luna service) — the setting that actually
  matters for OLED panel brightness, distinct from the `brightness` picture control. Proven
  workable on webOS 6 by projects like Home Assistant's `webostv` and `bscpylgtv`.
- Implementation in Go: the SSAP protocol is plain JSON over websocket, and the repo already
  uses a websocket library for the trackpad — a small `internal/tv/webos` package rather than
  a third-party dependency.
- Can run from either the desktop agent or the Pi; agent is the natural home (same box, same
  registry as DDC monitors).

Important constraint that rules out alternatives: **desktop GPUs (NVIDIA/AMD) do not wire up
HDMI-CEC**, so the desktop cannot send CEC at all. Fallbacks if webOS misbehaves: the Pi sits
next to the TV and its HDMI port speaks CEC (`cec-ctl` → power + volume, but no brightness),
or a Pulse-Eight USB-CEC adapter (~£40) on the desktop.

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
   Confirmed on the host: the default sink is the NVIDIA HDMI output feeding the TV
   (`alsa_output.pci-0000_01_00.1.hdmi-stereo`, id 55), with the Logi Z407 speakers as the
   second sink — so default-sink switching between TV and speakers is a real use case here,
   not hypothetical. Match sinks by node name (ids are not stable across reboots). ~1 day.
   Note HDMI sink volume is software attenuation — fine for trim, but the TV's own volume
   (below) is the better master control.
2. **The TV's own hardware volume** — rides on the webOS integration in 3b
   (`ssap://audio/setVolume` / `getVolume` / mute). No extra effort beyond 3b.

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

## 7. Remote OS selection on wake (GRUB dual-boot)

Goal: "Wake into Linux" / "Wake into Windows" buttons, without breaking unattended remote
wake. Two constraints shape the design:

- GRUB runs before any network stack, so the magic packet can't carry an OS choice — the
  machine always boots the *default* entry when woken remotely.
- **Do not remove the GRUB timeout.** A menu that waits forever means a remotely-woken machine
  sits at GRUB doing nothing until someone touches a keyboard. Keep the 5 s auto-boot.

### Design

1. **Flip the GRUB default to Linux** (one-time, on the desktop):
   ```bash
   # /etc/default/grub:
   #   GRUB_DEFAULT=saved
   #   GRUB_TIMEOUT=5          (keep it)
   sudo update-grub
   sudo grub-set-default "<linux entry name>"   # names: grep menuentry /boot/grub/grub.cfg
   ```
   Rationale: the ottoman agent lives on Linux (for now), so the OS that ottoman can *talk to*
   should be the one a plain wake lands in. Local boots are unaffected — the menu still shows
   for 5 s and you can pick Windows at the keyboard.
2. **Wake → Linux**: plain WoL, nothing else needed.
3. **Wake → Windows**: WoL → controller polls the agent's `/health` → once the Linux agent is
   up, controller calls a new agent endpoint `POST /api/boot {"target": "windows"}` → agent
   runs `grub-reboot "<windows entry>" && systemctl reboot`. `grub-reboot` sets a *one-shot*
   next-boot (requires `GRUB_DEFAULT=saved`), so the machine reboots into Windows exactly once
   and the default stays Linux. Costs one extra boot cycle (~30–60 s), but needs zero
   Windows-side changes and is completely reliable.
   - Same endpoint also serves "switch a running Linux box to Windows" (without WoL), which is
     handy on its own.
   - `grub-reboot` needs root: ship a narrow sudoers rule
     (`callum ALL=(root) NOPASSWD: /usr/sbin/grub-reboot *`) installed by the deploy step, or
     have GRUB read its env file from a path the agent user can write.
4. **Windows → Linux later** (when the Windows agent grows the same endpoint): from Windows the
   GRUB env file isn't reachable (ext4), but the UEFI boot-order route works:
   `bcdedit /set {fwbootmgr} bootsequence <GRUB entry GUID>` + `shutdown /r /t 0` (needs
   admin). Until then, switching Windows→Linux means a manual reboot or the TV-side keyboard.

UI: turn the Wake button into a split-button (Wake → Linux / Windows), and likewise split the
Shutdown button into shutdown / reboot, with the reboot half carrying a dropdown for
"Reboot into Windows" (plain reboot lands back in the Linux default). All driven by a
`[boot]` config section listing the GRUB entry names.

**Effort:** GRUB config is minutes; `/api/boot` endpoint + controller orchestration + UI
~1 day. Windows-side BootNext support later, alongside the other Windows parity work.

---

## Suggested order

0. **Layout store split + non-destructive deploy (6)** — small, and stops active data loss.
1. **WoL config fixes** (BIOS + nmcli) — no code, unblocks the core use case.
2. **Wayland display backend (2a)** — biggest functional gap on this machine.
3. **Wayland input via uinput (2b)** — restores the trackpad.
4. **PipeWire volume (4.1)** — small, high value.
5. **DDC/CI brightness+power (3a)** + the monitor registry (5a) alongside it.
6. **TV integration (3b + 4.2)** — pick approach once TV model is known.
7. **Boot-target selection (7)** — GRUB default flip + `/api/boot` endpoint.
8. **GNOME Quick Settings extension (5b)** — once the endpoints above exist.
9. Windows parity later: brightness/audio (WMI `WmiMonitorBrightnessMethods` only covers
   laptop panels; Windows will also want a DDC path via the physical-monitor Win32 API),
   BootNext via `bcdedit` for Windows→Linux switching, plus a tray+flyout equivalent of the
   quick-settings panel.

## Answered questions (host facts, checked 2026-07-04)

- **TV**: LG OLED65A16LA (A1 series 2021, webOS 6.0) → webOS network API chosen (3b).
  The Pi sits near the TV, so CEC-via-Pi remains available as a fallback.
- **NIC**: `enp6s0` — `Supports Wake-on: pumbg`, `Wake-on: g` → Linux side of WoL already
  works; remaining suspects are BIOS and Windows Fast Startup.
- **Dual-boot**: yes, Windows is present → 1c applies.
- **Audio**: PipeWire 1.0.5; default sink `alsa_output.pci-0000_01_00.1.hdmi-stereo`
  ("HDA NVidia Digital Stereo (HDMI)" → the TV), plus "Logi Z407 Analogue Stereo".

- **TV MAC**: `8C:19:B5:72:FE:62` (Wi-Fi — fine, LG wakes over WoWLAN; no ethernet run
  needed).

## Remaining open questions

- Current BIOS state of `Resume By PCI-E Device` / `ErP Ready` (needs a reboot to check).
- Enable the TV's wake toggle: **Settings → Devices → TV Management → "TV On With Mobile"** —
  then verify with a test magic packet to the TV's MAC.
