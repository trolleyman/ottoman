# Ottoman

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

### Client API

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Health check |
| `/api/status` | GET | No | Detailed status |
| `/api/layouts` | GET | Yes | List available layouts |
| `/api/layouts/switch` | POST | Yes | Switch to a layout |
| `/api/monitors` | GET | Yes | List connected monitors |

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

**Loopback callers are exempt by default**, so the CLI and a browser on the
machine itself work with no credentials. Set `require_local_auth = true` to
withdraw that exemption. You want it on as soon as a TLS front-end
(`tailscale serve`, a reverse proxy) forwards outside traffic in: those dial
from `127.0.0.1`, so with the default exemption every proxied request looks
local and is waved straight through.

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
