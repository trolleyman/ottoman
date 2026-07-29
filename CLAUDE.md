# ottoman

## Project Overview

Ottoman is a home automation system for controlling a Windows/Linux desktop computer remotely from a Raspberry Pi Zero 2 W. It consists of two components that run from a single binary:

- **Controller** (Raspberry Pi): Web interface, Wake-on-LAN, HTTP proxy to agent, periodic IP reporting
- **Agent** (Desktop): HTTP REST API for display switching with platform-specific implementations

## Build Commands

```bash
mage deps              # Install Go dependencies
mage build             # Build for current platform
mage buildAll          # Build for all platforms (pi, windows, linux)
mage buildPi           # Build for Raspberry Pi (linux/arm)
mage buildWindows      # Build for Windows (windows/amd64)
mage buildLinux        # Build for Linux desktop (linux/amd64)
```

## Test/Lint Commands

```bash
mage test              # Run tests
mage lint              # Run linter
mage clean             # Remove build artifacts
```

## Run Locally

```bash
mage runController     # Run controller locally
mage runAgent          # Run agent locally
mage runSimulated      # Run simulated controller + agent locally
```

## Project Structure

```
cmd/ottoman/main.go      # CLI entry point (Cobra commands)
internal/
  config/                # Unified configuration (Viper + TOML)
    config.go            # Config loading, validation, defaults
  controller/            # Raspberry Pi controller component
    controller.go        # HTTP server, routes, proxy logic
    wol.go               # Wake-on-LAN implementation
    config.go            # Server config wrapper
    deploy.go            # Systemd service installation
  agent/                 # Desktop agent component
    agent.go             # HTTP API for display control
    config.go            # Agent config wrapper
    deploy.go            # Service registration (systemd/Windows startup)
  display/               # Display management abstraction
    display.go           # Interface & layout store
    windows.go           # Windows implementation (PowerShell/WMI)
    linux.go             # Linux implementation (xrandr)
  common/                # Shared types and utilities
    types.go             # Core structs (DisplayLayout, etc.)
    api.go               # HTTP request/response types & helpers
magefiles/               # Mage build tasks (Go)
examples/                # Config file templates (TOML)
build/                   # Compiled binaries output
web/                     # React frontend
```

## Key Conventions

- **Build System**: Mage (magefile.org) - tasks defined in `magefiles/magefile.go`
- **CLI Framework**: Cobra (spf13/cobra)
- **Configuration**: Viper with TOML format - unified `ottoman.toml` for both agent and controller
- **HTTP**: Standard library `net/http`
- **Platform-specific code**: Build tags (`//go:build windows`, `//go:build linux`)
- **Config search paths** (unified `ottoman.toml`):
  - `./ottoman.toml` (current directory)
  - `/etc/ottoman/ottoman.toml` (Linux system-wide)
  - `~/.config/ottoman/ottoman.toml` (Linux user)
  - `%APPDATA%/ottoman/ottoman.toml` (Windows)
- **Authentication**: `internal/common/auth.go`. One `Authenticator` gates both
  components, wired in as ordinary `net/http` middleware around the router -
  **not** inside the generated strict handler, which never sees the
  `ResponseWriter` and so cannot set the session cookie. It accepts a Bearer
  header, Basic auth (token as the password), or the `ottoman_auth` cookie, all
  compared in constant time. `/api/*` is gated; `/health`, `/api/auth*` and the
  SPA assets are not, so the login screen can always load. Loopback peers are
  gated too unless `require_local_auth = false`; unset means gated, since the
  case where the exemption is unsafe (a TLS front-end forwarding traffic in from
  `127.0.0.1`) is invisible from inside a request. The local-peer test reads
  `RemoteAddr` only and never a forwarded header, which would be spoofable.
  `Authenticator.Permits` is the single source of truth: the middleware and
  `/api/auth/check` both call it, so the UI is never told something the gate
  disagrees with.
- **Talking to the agent**: `controller.agent.url` carries the scheme, and
  `Controller.agentURL` / `agentWebSocketURL` derive every request from it
  (`https` implies `wss`). Never hardcode `http://` at a call site. The legacy
  `ip_address` + `port` pair still loads and is folded into a URL by
  `AgentControllerConfig.resolvedURL`.
- **Error handling**: Uses `github.com/pkg/errors` for wrapping

## Deployment Commands

```bash
mage deployAgent         # Deploy agent (build + copy + register service)
mage deployController    # Deploy controller via SSH to Raspberry Pi
mage deployAll           # Deploy controller + agent
```

Deployment settings are saved to `magefiles/deploy.toml` (gitignored).

## Config Commands

```bash
ottoman config show                    # Show current configuration
ottoman config paths                   # Show config search paths
ottoman config init                    # Create a config file (interactive)
ottoman config set <key> <value>       # Set one value, validated, non-interactively
ottoman config rotate-token            # New auth token in every copy that must match
```

`config set` validates the value against the key before writing (a listen
address that isn't `host:port`, a URL with no scheme, a 5-character token are
all refused), touches only that key, and lists the settable keys in its
`--help`. It rewrites the file from its parsed contents, so comments and key
order are not preserved. `config rotate-token` generates a token and writes it
to every `auth_token` key the file already has, mirrors it to the greeter copy,
and with `--push <user@host>` sets `controller.auth_token` on the Pi over SSH -
the three copies that must agree, updated from one command. Neither is picked
up until the affected component restarts.

## API Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | `GET` | No | Health check |
| `/api/status` | `GET` | Yes | Detailed status (controller or agent) |
| `/api/status/agent` | `GET` | Agent status |
| `/api/auth` | `POST` | No | Login with token |
| `/api/auth/logout` | `POST` | No | Logout |
| `/api/auth/check` | `GET` | Yes | Auth check |
| `/api/wake` | `POST` | Wake agent (only on controller) |
| `/api/layouts` | `GET` | Get all stored layouts (controller: mirrored copy when the agent is down, plus `pending_layout`) |
| `/api/layouts/switch` | `POST` | Switch to specified layout (controller: queued when the agent is down, `queued: true`) |
| `/api/layouts/save-current` | `POST` | Save the current layout as a new layout |
| `/api/layouts/remove` | `POST` | Remove the specified layout |
| `/api/layouts/update` | `POST` | Update a layout's name, emoji, and aliases |
| `/api/monitors` | `GET` | Get all monitors (control backend, capabilities, brightness, visibility, live TV state) |
| `/api/monitors/brightness` | `POST` | Set a monitor's brightness (DDC or TV backend) |
| `/api/monitors/power` | `POST` | Turn a monitor on/off (DDC standby, or TV WoL/SSAP) |
| `/api/monitors/volume` | `POST` | Set a monitor's volume/mute (TV backend) |
| `/api/monitors/pair` | `POST` | Start on-screen pairing for a TV-backed monitor |
| `/api/monitors/input` | `POST` | Switch a TV-backed monitor's external input |
| `/api/monitors/settings` | `POST` | Update a monitor's registry entry (name, backend, visibility) |
| `/api/tv/export` | `GET` | (Agent only) TV registry entries + pairing keys, for the controller to mirror |
| `/api/audio/sinks` | `GET` | List PipeWire output sinks |
| `/api/audio/volume` | `POST` | Set a sink's volume/mute/default |
| `/api/boot` | `POST` | Reboot into a specific OS (GRUB dual-boot) |
| `/api/shutdown` | `POST` | Shut down agent |
| `/api/trackpad` | `GET` | Open mouse / keyboard WebSocket controller |

Runtime data (layouts, monitor registry, TV pairing keys) lives in the XDG data
dir (`~/.local/share/ottoman/`), separate from the config file, so redeploying
config never clobbers it. Linux backends: displays via GNOME Mutter D-Bus
(Wayland) with an xrandr fallback; input via `/dev/uinput`; audio via `wpctl`;
brightness/power via `ddcutil`. One-time host setup (uinput/i2c/grub-reboot) is
applied natively as root by `ottoman agent host-setup` (self-elevates via
sudo), offered interactively at the end of `agent install`. `host-setup
--greeter` additionally deploys a **login-screen layout agent**: `ottoman agent
run --greeter` runs as the `gdm` user against the GDM greeter's own Mutter
(display/layouts only — input/audio are skipped), so you can switch display
layouts on the login screen and it mirrors the user's last-used layout there. It
reads a gdm-readable copy of config + layouts under `/var/lib/ottoman/greeter`
(owned `<user>:gdm`, setgid, group-readable) that the user's agent keeps in sync
- `config.toml` included, on every layout change and at agent startup, so a
rotated token or a changed listen address reaches the login screen instead of
drifting from the install-time copy. `config set` / `config rotate-token` mirror
it too, for changes made while the agent isn't running.
A GNOME Quick Settings extension lives in `gnome-extension/`.

**Choosing a layout for a machine that is off** (`internal/controller/pending.go`):
the desktop can't be told anything while it's asleep, so the controller holds
the choice and applies it on the way up. `POST /api/layouts/switch` against a
down agent returns `queued: true` instead of failing, `POST /api/wake` takes an
optional `layout`, and the queued id comes back as `pending_layout` on
`/api/layouts` so the UI can badge the card ("on wake"). It is applied twice:
once to the GDM greeter (so the login screen lands on the right display) and
again to the session agent when it takes over, since Mutter restores the
session's own config on login - the agent's status carries `greeter` so the
controller can tell them apart. The choice lapses after 15 minutes. The
controller also **mirrors the agent's layouts** (`controller-layouts.json`,
polled every 30s) and serves them when the agent is down, so the list is still
there when you need to pick where the machine comes back up. This is the
reliable answer to "the monitors are off, put it on the TV": whether a
powered-off monitor drops off the bus is a property of the monitor, so intent
can't be inferred - but it can be stated.

The TV backend (`internal/tv`, LG webOS over SSAP + Wake-on-LAN) is shared: both
the agent and the controller construct a `tv.Manager`. Normally the controller
proxies `/api/monitors/*` to the agent, but it also **mirrors** the agent's TV
registry + pairing keys (polling `/api/tv/export` every 30s into its own data
dir) so that when the agent (desktop) is down it drives the TV directly — power
on (WoL) / off (SSAP), volume, mute, and OLED backlight — letting you turn the TV
off after shutting the computer down. Input switching stays agent-only. The
per-monitor fallback lives in `internal/controller/{monitors,tv}.go`.

# Debug
When running you may encounter `unsupported OS: MINGW64_NT-10.0-26200` - ignore this.
It's an artifact from running in MINGW64 - the command is successful or not depending on the exit code.
