# Ottoman

Agent startup preserves the live display arrangement and its saved boot layout,
including at the GDM login screen. Saved layouts are applied only on request;
an unreachable TV does not trigger an automatic fallback. Layouts captured on
Wayland use monitor identities to survive connector renumbering. On Xorg, stale
or missing output names are rejected before any displays are disabled: recapture
the layout in the current session. Failed switches show the server's error
message in the Layouts panel.

Home automation system for controlling a desktop computer from a Raspberry Pi Zero 2 W.

## Overview

Ottoman consists of two components that run from a single binary:

1. **Server** (runs on Raspberry Pi)
   - Web interface for control
   - Wake-on-LAN magic packets to wake desktop
   - HTTP proxy to client for display switching
   - Periodic IP reporting to external API for remote access
   - Token authentication

2. **Client** (runs on desktop - Windows/Linux)
   - HTTP REST API for display switching
   - Platform-specific display management (Windows/Linux)
   - Layout configurations stored in config file

## Quick Install

### From Source

```bash
# Clone and build
git clone https://github.com/trolleyman/ottoman.git
cd ottoman
mise install && mise run build

# Install to system location
./build/ottoman install        # Linux
.\build\ottoman.exe install    # Windows
```

This installs ottoman to:
- **Windows:** `%LOCALAPPDATA%\ottoman\ottoman.exe`
- **Linux:** `~/.local/bin/ottoman`

### One-liner (build + deploy client)

```bash
mage deployClient
```

This interactively builds, copies, and registers the client as a service.

## Client Setup

### 1. Run the client

```bash
ottoman client run
```

### 2. (Optional) Start automatically at login

```bash
ottoman client service install
```

This creates:
- **Windows:** Startup script in `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup`
- **Linux:** Systemd user service

To remove: `ottoman client service uninstall`

### 3. Save display layouts

```bash
# Save current monitor configuration as a layout
ottoman client layout add desktop "Desktop Setup" 🖥️

# List saved layouts
ottoman client layout list

# Apply a layout
ottoman client layout apply desktop

# Add an alias
ottoman client layout alias add desktop d
```

## Server Setup (Raspberry Pi)

Use the interactive deployment command:

```bash
mage deployController
```

This will:
1. Ask for SSH target and server configuration
2. Auto-detect your desktop's MAC/IP for Wake-on-LAN
3. Build for Raspberry Pi
4. Deploy binary and config via SSH
5. Install systemd service

Settings are saved to `magefiles/deploy.toml` for future deployments.

## PC remote

PC remote is the home page (also available at `/remote`). The trackpad, right-hand
scroll rail and full-width phone keyboard stay together. The top-left menu holds
the display layout, PC volume, TV volume and brightness, Settings, and desktop
management. On larger screens it becomes a collapsible sidebar. Fullscreen is
shown only where supported. This page controls the computer without streaming its screen.

When disconnected, the same trackpad and inactive keyboard remain visible.
Configured TVs can still be controlled from the menu directly through the Pi
using webOS and Wake-on-LAN. Pair the TV while the computer is online first; the
Pi persists its connection settings and pairing key.

The PC remote keyboard stays visible even when the trackpad is unfocused. Choose Ottoman or Device keyboard in menu → Settings. The shared keyboard bar
shows local word completions and swipe alternatives, or searches emoji and symbols.
Search results replace the number row; emoji
categories share one continuous list with icon shortcuts. Contractions such as
“don’t” and “isn’t” are supported in completions and swipe alternatives.
Accepted suggestions and swipe words insert a trailing space. Punctuation removes that automatic separator. Optional **Autocorrect** (off by default) corrects common one-letter errors when you enter a space or punctuation, including `keynoard` → `keyboard`. The small
× dismisses the field without deleting its text; typing restores it.
Emoji is available from the **#+=** symbols page. Enable **Show emoji key on letters** in keyboard settings to also show it beside space on the main letter page; it is off by default.

Hold **Q** for backtick in the British layout; symbol mode uses the same key positions.
**Fn** exposes function, paging and media keys. Hold an arrow key to repeat navigation; release it to stop. Tap a modifier for the next key, double-tap to lock it, or hold it across multiple keys. Meta (Super/Win/Cmd) opens the desktop launcher on a double-tap instead of locking. To send Meta+L, tap Meta then L, or hold Meta while pressing L. A separate launcher button also opens the desktop launcher.

The clipboard button beside **Fn** replaces the existing function row with three
actions: **Paste image**, **Paste from phone**, and **Copy from PC**. Choosing an
image uploads and pastes it immediately. Phone text uses browser clipboard access
when permitted, with a manual paste field as fallback. PC text opens a selectable
field with an explicit Copy action. Clipboard access requires a secure browser
context; iPhone may require an additional tap or its native paste/copy menu.
Sharing is on demand, with no background clipboard synchronization. Successful
copy or paste returns to the keyboard and deselects Clipboard.

Image paste replaces the desktop clipboard; the receiving app must support images.
Photos are converted to PNG (maximum 4096 pixels per side and 20 MB uploaded).
Linux needs `wl-copy`/`wl-paste` on Wayland or `xclip` on X11; Windows uses its
native clipboard through PowerShell. Text transfers are limited to 1 MiB.
Clipboard sharing is unavailable on the login screen.

Compatible Linux players expose now-playing information through MPRIS. The menu
shows a player and only the playback controls it supports; unavailable players
and unsupported platforms have no card.

Enable **Taller keys** in Settings for larger touch targets. Tab, comma and
period beside space are off by default and individually configurable. Emoji is
hidden on letters by default and remains available from **#+=**.
**Adaptive key sizes** uses the preceding words and repeated phrases in the current field to predict next letters, including between words and in capitals. It silently scales touch targets in both dimensions around fixed centers, capped at 20% growth. Visible keys stay the same size unless **Debug key sizes** is enabled, which reveals enlarged shapes, boundaries and actual scale. Suggestions continue with next-word choices after a space, without correction ticks or confirmation highlighting. Completions reuse vocabulary found in the current field and offer existing one-edit typo corrections when ordinary completions run out; this never enables automatic correction by itself. A compact fallback keeps next-word choices available while the full dictionary loads. Both are off by default. Keyboard options are grouped by typing, preview/navigation, layout, prediction/feedback, and gesture timing.

A stationary one-finger hold right-clicks after 500 ms by default. Settings → Trackpad lets you disable it or adjust the hold duration (250–1,000 ms) and movement tolerance (2–20 CSS px, default 6). Moving, adding another contact, opening the menu or disconnecting cancels it; continuing a tap-drag takes priority.

A tap presses the mouse button when you lift, then releases it after a 300 ms retouch window. Touch again nearby within that window to continue the same press: hold for as long as you need, then move to drag. Dragging starts immediately with movement and releases when you lift, without a second press that could maximize a window or select a whole word. Each additional quick stationary tap advances the native click count and reserves the next press for another retouch. Tap–tap–drag produces one click-drag; tap–tap–tap–drag produces a double-click-drag; tap–tap–tap–tap–drag produces a triple-click-drag, with no fixed chain limit. Stationary tap chains finish all their clicks when the retouch window closes. Ordinary pointer movement never arms a drag; cancellation, focus loss and disconnect release held input.

## Linux login and lock screens

GNOME's lock screen uses the normal session agent. Keyboard and trackpad input
use `/dev/uinput`, so they can also operate the locked session; accessible text
mirroring stays blank while locked. `mage deployAll` restarts that session agent
through `DeployAgent` as part of the normal deployment.

The GDM login screen uses a separate `gdm` agent. After deploying this version,
run the one-time setup on the desktop:

```bash
ottoman agent host-setup --greeter
```

It requests sudo, installs the greeter copy, and grants your user and `gdm`
membership in `ottoman-input`, a dedicated group for `/dev/uinput`. It does not
grant GDM read access to physical keyboard event devices. Reboot once so GDM
starts with its new group membership and the updated agent. The greeter supports
authenticated keyboard/trackpad and layout control; audio and native text
mirroring remain disabled before login. Without uinput permission, it continues
serving display/layout control and reports input as unavailable.

Future `deployAll`/`deployAgent` runs verify and atomically refresh the binary at
`/var/lib/ottoman/greeter/bin/ottoman`. A running greeter agent notices replacement
within two seconds and re-executes in the same GDM session; the phone reconnects.
Deployment reports an error if an installed greeter cannot be refreshed. Agents
from versions without this watcher need the initial reboot above. Deployment
does not restart GDM or log out your desktop session.

## Focused text on the trackpad

Enable **Mirror focused text** in menu → Settings to show an accessible desktop
field in a native text box inside the trackpad. The box has a fixed two-line
height and scrolls independently, with native selection controls. The × dismisses
it without deleting text or moving the keyboard. Protected fields stay masked;
terminal/read-only content uses the retained preview renderer.

Spacebar cursor gestures and navigation keys follow the phone text box’s visual
lines for both native editable fields and locally typed text by default; turn off
**Navigate by preview lines** to use the desktop’s layout instead. Adjust
**Space cursor sensitivity** for faster or finer movement. Both Ottoman and
Device keyboards can replace the
native selection; local desktop edits and caret moves update the preview.
Mirroring is off by default and uses the existing authenticated WebSocket.

Linux uses AT-SPI2 on the graphical user's accessibility bus. The current lock
screen check supports GNOME; sessions without a readable lock state fall back
to ordinary keyboard input. EditableText controls support native range edits.
Linux terminal emulators exposing the AT-SPI Terminal role and Text interface
also mirror their text and caret, including shells such as Fish, Bash, and Zsh.
This is a plain text view of the terminal buffer; ANSI colors and formatting
are not mirrored. Terminal and read-only rich text previews always send navigation
keys directly to the desktop. Locally typed text uses the phone text box’s
visual lines and sends horizontal caret moves to the desktop. Preview taps
can position the caret only in fields that support native edits.
Windows uses UI Automation through a helper running under the agent's user
account (Windows PowerShell and the system UI Automation assemblies). Writable
ValuePattern controls with a matching TextPattern support native edits; rich
editors without that model retain keystroke input and a read-only native preview.
If the OS requests accessibility access, grant it to the desktop agent. Elevated
or inaccessible applications may not expose a usable text model.

With **Mirror focused text** enabled, password fields show dots without reading
or mirroring their text or selection. Linux uses a character-count property when
available (capped at 4,096); when the count is unavailable, including on Windows,
the preview shows a fixed eight-dot mask that does not indicate actual length.
Password fields use opaque focus tokens so typing history clears between fields.
Locked or inaccessible sessions remain blank. Context stays in memory, is limited to 4,096 Unicode code points around the caret, and clears on
focus loss, disabling mirroring, or disconnection. Unsupported applications use
the existing typing indicator. Focus tokens and revisions reject stale edits
and replace the phone's pending state with the latest desktop context. macOS
is deferred until Ottoman has a supported macOS agent/input backend.

## Configuration

Configuration is stored in `config.toml`:
- **Windows:** `%APPDATA%\ottoman\config.toml`
- **Linux:** `~/.config/ottoman/config.toml`

View config paths: `ottoman config paths`
Show current config: `ottoman config show`
Create default config: `ottoman config init`
Change one setting: `ottoman config set agent.listen_address 127.0.0.1:17294`
Rotate the auth token: `ottoman config rotate-token --push pi@ottoman.home`

See `examples/` for sample configuration files.

## Building from Source

### Prerequisites

- [mise](https://mise.jdx.dev/) - Runtime version manager
- Go 1.21+

### Build Commands

```bash
mise install              # Install tools (Go, mage)
mise run deps             # Install Go dependencies
mise run build            # Build for current platform
mise run build:all        # Build for all platforms
mise run build:pi         # Raspberry Pi (linux/arm)
mise run build:windows    # Windows (windows/amd64)
mise run build:linux      # Linux desktop (linux/amd64)
mise run install          # Build and install
```

### Development

```bash
mise run test             # Run tests
mise run lint             # Run linter
mise run clean            # Remove build artifacts
mise run run:server       # Run server locally
mise run run:client       # Run client locally
```

## CLI Reference

```bash
# Installation
ottoman install                           # Install binary to system location

# Client commands
ottoman client run                        # Run the client
ottoman client service install            # Setup autostart
ottoman client service uninstall          # Remove autostart

# Layout management
ottoman client layout list                # List all layouts
ottoman client layout add <id> <name>     # Save current display config
ottoman client layout apply <id>          # Apply a layout
ottoman client layout alias add <id> <a>  # Add alias to layout
ottoman client layout alias remove <id> <a>

# Server commands
ottoman server run                        # Run the server
ottoman server install                    # Install systemd service

# Config
ottoman config show                       # Show current config
ottoman config paths                      # Show config search paths
ottoman config init                       # Create default config (interactive)
ottoman config set <key> <value>          # Set one value, validated (see --help for keys)
ottoman config rotate-token               # New auth token everywhere (--push for the Pi)

# Status
ottoman status                            # Check server and client status
```

## API Reference

### Server API

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Health check |
| `/api/status` | GET | No | Detailed status |
| `/api/wake` | POST | Yes | Send Wake-on-LAN |
| `/api/layouts` | GET | Yes | List display layouts (proxied) |
| `/api/layouts/switch` | POST | Yes | Switch layout (proxied) |
| `/api/clipboard/text` | GET/POST | Yes | Read/write desktop text clipboard (1 MiB) |
| `/api/media` | GET/POST | Yes | Read active player and send supported playback actions |
| `/api/clipboard/image` | POST | Yes | Forward an image to the desktop clipboard |

### Client API

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Health check |
| `/api/status` | GET | No | Detailed status |
| `/api/layouts` | GET | Yes | List available layouts |
| `/api/layouts/switch` | POST | Yes | Switch to a layout |
| `/api/monitors` | GET | Yes | List connected monitors |
| `/api/clipboard/text` | GET/POST | Yes | Read/write desktop text clipboard (1 MiB) |
| `/api/media` | GET/POST | Yes | Linux MPRIS metadata and capability-checked playback actions |
| `/api/clipboard/image` | POST | Yes | Copy a PNG/JPEG/GIF body to the clipboard (20 MB / 16 megapixel limit); returns 204 when ready |

### Authentication

`/api/*` is gated by `auth_token`; an unauthenticated caller gets `401`. Three
credential shapes are accepted:

```
Authorization: Bearer your-secret-token
Authorization: Basic <base64 of any-username:your-secret-token>
Cookie: ottoman_auth=<value issued by POST /api/auth>
```

`/health`, `/api/auth*` and the web UI's own assets stay open, so an
unauthenticated browser can still load the page and reach the login screen.

`POST /api/auth` with `{"token": "..."}` sets the `ottoman_auth` cookie
(HttpOnly, and holding a hash of the token rather than the token itself). The
browser then sends it automatically - including on the `/api/trackpad`
WebSocket, which is why the cookie exists at all: a browser cannot put an
`Authorization` header on a WebSocket handshake.

**Loopback callers are gated too, by default.** Set `require_local_auth = false`
to exempt them, so the CLI and a browser on the machine itself work with no
credentials. That exemption is only safe while nothing on the host forwards
outside traffic in: a TLS front-end (`tailscale serve`, a reverse proxy) dials
from `127.0.0.1`, so every request it carries looks local and would be waved
straight through. Nothing in a request distinguishes the two cases, which is
why the default is the strict one.

Both components check for that combination at startup: if a `tailscale serve`
mapping forwards to their listen port while `require_local_auth` is off, they
log a `SECURITY:` warning naming the tailnet address that reaches them - and
say so more sharply when Funnel is on for it, because then the open API is
facing the public internet rather than just your tailnet.

The gate never trusts `X-Forwarded-For` or similar headers when deciding whether
a peer is local - they are client-controlled, so honouring one would let any
remote caller claim the exemption.

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│  Raspberry Pi   │         │     Desktop     │
│                 │         │  (Windows/Linux) │
│  ┌───────────┐  │  HTTP   │  ┌───────────┐  │
│  │  Ottoman  │──┼────────▶│  │  Ottoman  │  │
│  │  Server   │  │         │  │  Client   │  │
│  └───────────┘  │         │  └───────────┘  │
│       │         │         │       │         │
│       │ WoL     │         │       │ Display │
│       ▼         │         │       ▼         │
│  ┌─────────┐    │         │  ┌─────────┐    │
│  │ Network │    │         │  │ xrandr/ │    │
│  │ (UDP:9) │    │         │  │ WinAPI  │    │
│  └─────────┘    │         │  └─────────┘    │
└─────────────────┘         └─────────────────┘
```

## License

MIT


# TODO
- bugs
   - when clicking and dragging the trackpad, the trackpad doesn't grab input - it should grab as soon as I click on it so that I can click and drag and the cursor will move
   - when hiding the keyboard on the trackpad on mobile, this should unfocus the trackpad.
   - triple click and drag is needed to drag on mobile
   - local IP pinging and redirect not working
   - scrolling on macOS/phone -> Windows is not precise
   - sometimes two finger scroll finishing makes mouse jump up quite far
- major features
   - add brightness settings first to CLI, then to UI
      - HDMI
      - DisplayPort
      - how does HDR fit into it?
   - streaming of monitors!! that'd be awesome
      - potentially turn on and off for each individual montior to save streaming costs
      - RDP?
   - also UDP rather than TCP for streaming mouse positions and also controlling mouse to make it more performant
   - volume slider to control volume
- minor features
   - Add HDR info at least of monitors
   - Add monitor logical size to monitor info (e.g. `XxY (X2xY2 @ Nx)` - `2680x1400 (1920x1080 @ 2x)` - dimensions are wrong here but give impression)
