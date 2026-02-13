# CLAUDE.md

## Project Overview

Ottoman is a home automation system for controlling a Windows/Linux desktop computer remotely from a Raspberry Pi Zero 2 W. It consists of two components that run from a single binary:

- **Server** (Raspberry Pi): Web interface, Wake-on-LAN, HTTP proxy to client, periodic IP reporting
- **Client** (Desktop): HTTP REST API for display switching with platform-specific implementations

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
mage runServer         # Run server locally
mage runClient         # Run client locally
```

## Project Structure

```
cmd/ottoman/main.go      # CLI entry point (Cobra commands)
internal/
  config/                # Unified configuration (Viper + TOML)
    config.go            # Config loading, validation, defaults
  server/                # Raspberry Pi server component
    server.go            # HTTP server, routes, proxy logic
    wol.go               # Wake-on-LAN implementation
    config.go            # Server config wrapper
    deploy.go            # Systemd service installation
  client/                # Desktop client component
    client.go            # HTTP API for display control
    config.go            # Client config wrapper
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
- **Configuration**: Viper with TOML format - unified `ottoman.toml` for both server and client
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
mage deployClient        # Deploy client (build + copy + register service)
mage deployServer        # Deploy server via SSH to Raspberry Pi
mage deployAll           # Deploy client + server
```

Deployment settings are saved to `magefiles/deploy.toml` (gitignored).

## Config Commands

```bash
ottoman config show      # Show current configuration
ottoman config paths     # Show config search paths
ottoman config init      # Create default config file
```

## API Endpoints

**Server** (port 8080):
- `GET /health` - Health check
- `GET /api/status` - Status with uptime
- `POST /api/wake` - Send WoL packet (auth required)
- `GET /api/layouts` - Proxy to client layouts
- `POST /api/layouts/switch` - Proxy layout switch

**Client** (port 8081):
- `GET /health` - Health check
- `GET /api/status` - Status with uptime
- `GET /api/layouts` - List available layouts (auth required)
- `POST /api/layouts/switch` - Switch display layout (auth required)
- `GET /api/monitors` - List connected monitors (auth required)

# Debug
When running you may encounter `unsupported OS: MINGW64_NT-10.0-26200` - ignore this.
It's an artifact from running in MINGW64 - the command is successful or not depending on the exit code.
