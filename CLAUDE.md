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
- **Authentication**: Bearer token or Basic auth with constant-time comparison
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
ottoman config show      # Show current configuration
ottoman config paths     # Show config search paths
ottoman config init      # Create default config file
```

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
| `/api/layouts` | `GET` | Get all stored layouts |
| `/api/layouts/switch` | `POST` | Switch to specified layout |
| `/api/layouts/save-current` | `POST` | Save the current layout as a new layout |
| `/api/layouts/remove` | `POST` | Remove the specified layout |
| `/api/monitors` | `GET` | Get all monitors (with control backend, capabilities, brightness, visibility) |
| `/api/monitors/brightness` | `POST` | Set a monitor's brightness (DDC or TV backend) |
| `/api/monitors/power` | `POST` | Turn a monitor on/off (DDC standby or TV power) |
| `/api/monitors/settings` | `POST` | Update a monitor's registry entry (name, backend, visibility) |
| `/api/audio/sinks` | `GET` | List PipeWire output sinks |
| `/api/audio/volume` | `POST` | Set a sink's volume/mute/default |
| `/api/tv/state` | `GET` | Get TV integration state |
| `/api/tv/pair` | `POST` | Start on-screen TV pairing |
| `/api/tv/power` | `POST` | Turn the TV on (WoL) or off (SSAP) |
| `/api/tv/volume` | `POST` | Set TV volume/mute |
| `/api/tv/input` | `POST` | Switch the TV's external input |
| `/api/boot` | `POST` | Reboot into a specific OS (GRUB dual-boot) |
| `/api/shutdown` | `POST` | Shut down agent |
| `/api/trackpad` | `GET` | Open mouse / keyboard WebSocket controller |

Runtime data (layouts, monitor registry, TV pairing key) lives in the XDG data
dir (`~/.local/share/ottoman/`), separate from the config file, so redeploying
config never clobbers it. Linux backends: displays via GNOME Mutter D-Bus
(Wayland) with an xrandr fallback; input via `/dev/uinput`; audio via `wpctl`;
brightness/power via `ddcutil`. One-time host setup (uinput/i2c/grub-reboot, plus
optional GDM autologin-into-a-locked-screen so the agent runs after Wake-on-LAN)
is applied natively as root by `ottoman agent host-setup` (self-elevates via
sudo), offered interactively at the end of `agent install`. `host-setup
--greeter` additionally deploys a **login-screen layout agent**: `ottoman agent
run --greeter` runs as the `gdm` user against the GDM greeter's own Mutter
(display/layouts only — input/audio are skipped), so you can switch display
layouts on the login screen and it mirrors the user's last-used layout there. It
reads a gdm-readable copy of config + layouts under `/var/lib/ottoman/greeter`
(owned `<user>:gdm`, setgid, group-readable) that the user's agent keeps in sync.
A GNOME Quick Settings extension lives in `gnome-extension/`.

# Debug
When running you may encounter `unsupported OS: MINGW64_NT-10.0-26200` - ignore this.
It's an artifact from running in MINGW64 - the command is successful or not depending on the exit code.
