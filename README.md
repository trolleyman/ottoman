# Ottoman

Home automation system for controlling a desktop computer (hades) from a Raspberry Pi Zero 2 W.

## Overview

Ottoman consists of two components that run from a single binary:

1. **Server** (runs on Raspberry Pi)
   - Web interface for control
   - Wake-on-LAN magic packets to wake hades
   - HTTP proxy to client for display switching
   - Periodic IP reporting to external API for remote access
   - Token or username/password authentication

2. **Client** (runs on hades - Windows/Linux dual boot)
   - HTTP REST API for display switching
   - Platform-specific display management (Windows/Linux)
   - Pre-defined layout configurations

## Installation

### Prerequisites

- [mise](https://mise.jdx.dev/) - Runtime version manager
- Go 1.21+
- Bun (for build scripts)

### Building

```bash
# Install dependencies
mise install
mise run deps

# Build for current platform
mise run build

# Build for all platforms
mise run build:all

# Build for specific targets
mise run build:pi       # Raspberry Pi (linux/arm)
mise run build:windows  # Windows (windows/amd64)
mise run build:linux    # Linux desktop (linux/amd64)
```

### Development Tasks

```bash
mise run test           # Run tests
mise run lint           # Run linter
mise run clean          # Remove build artifacts
mise run run:server     # Run server locally
mise run run:client     # Run client locally
mise run deploy:pi      # Deploy to Raspberry Pi
mise run deploy:client  # Deploy client locally
```

## Usage

### CLI Commands

```bash
# Server commands (run on Raspberry Pi)
ottoman server run              # Run the server
ottoman server install          # Install systemd service
ottoman server deploy -t user@pi # Deploy to Pi via SSH

# Client commands (run on desktop)
ottoman client run              # Run the client agent
ottoman client install          # Install as service (systemd/Windows)
ottoman client deploy           # Deploy locally

# Deployment
ottoman deploy --server-target user@pi  # Deploy both components

# Status
ottoman status --server localhost:8080 --client localhost:8081
```

### Configuration

Configuration files are searched in order:

**Server:**
1. `./ottoman-server.json`
2. `/etc/ottoman/server.json`
3. `~/.config/ottoman/server.json`

**Client:**
1. `./ottoman-client.json`
2. `/etc/ottoman/client.json` (Linux)
3. `~/.config/ottoman/client.json` (Linux)
4. `%APPDATA%/ottoman/client.json` (Windows)

See `examples/` for sample configuration files.

## API Reference

### Server API

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Health check |
| `/api/status` | GET | No | Detailed status |
| `/api/wake` | POST | Yes | Send Wake-on-LAN |
| `/api/wake/targets` | GET | Yes | List wake targets |
| `/api/layouts` | GET | Yes | List display layouts (proxied) |
| `/api/layouts/switch` | POST | Yes | Switch layout (proxied) |
| `/api/layouts/current` | GET | Yes | Get current layout (proxied) |
| `/api/client/status` | GET | Yes | Check client status |

### Client API

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Health check |
| `/api/status` | GET | No | Detailed status |
| `/api/layouts` | GET | Yes | List available layouts |
| `/api/layouts/switch` | POST | Yes | Switch to a layout |
| `/api/layouts/current` | GET | Yes | Get current layout |
| `/api/monitors` | GET | Yes | List connected monitors |

### Authentication

Use Bearer token in Authorization header:
```
Authorization: Bearer your-secret-token
```

Or Basic auth:
```
Authorization: Basic base64(username:password)
```

### Example Requests

```bash
# Wake hades
curl -X POST http://pi:8080/api/wake \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{"target": "hades"}'

# Switch to TV layout
curl -X POST http://pi:8080/api/layouts/switch \
  -H "Authorization: Bearer token" \
  -H "Content-Type: application/json" \
  -d '{"layout": "TV Only"}'

# List layouts
curl http://pi:8080/api/layouts \
  -H "Authorization: Bearer token"
```

## Display Layouts

Layouts are defined in `layouts.json`:

```json
[
  {
    "name": "Normal",
    "monitors": [
      {
        "name": "DP-1",
        "width": 2560,
        "height": 1440,
        "refresh_rate": 144,
        "position_x": 0,
        "position_y": 0,
        "primary": true,
        "enabled": true
      },
      {
        "name": "HDMI-1",
        "width": 1920,
        "height": 1080,
        "refresh_rate": 60,
        "position_x": 2560,
        "position_y": 180,
        "primary": false,
        "enabled": true
      }
    ]
  }
]
```

### Platform Notes

**Linux:** Uses `xrandr` for display configuration.

**Windows:** Uses PowerShell and Windows Display Settings. Complex multi-monitor layouts may require additional tools.

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│  Raspberry Pi   │         │     Hades       │
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
        │
        │ HTTPS
        ▼
┌─────────────────┐
│   Vercel API    │
│ (trolleyman.org)│
└─────────────────┘
```

## Development

### Project Structure

```
ottoman/
├── cmd/
│   └── ottoman/
│       └── main.go           # CLI entry point
├── internal/
│   ├── server/               # Pi server logic
│   │   ├── config.go
│   │   ├── server.go
│   │   ├── wol.go
│   │   └── deploy.go
│   ├── client/               # Desktop client logic
│   │   ├── config.go
│   │   ├── client.go
│   │   └── deploy.go
│   ├── display/              # Display switching
│   │   ├── display.go        # Interface
│   │   ├── windows.go        # Windows implementation
│   │   └── linux.go          # Linux implementation
│   └── common/               # Shared types
│       ├── types.go
│       └── api.go
├── web/                      # React frontend (TODO)
├── examples/                 # Sample configs
├── scripts/                  # Build scripts (TypeScript/Bun)
│   ├── build.ts
│   ├── clean.ts
│   ├── test.ts
│   ├── lint.ts
│   ├── run.ts
│   ├── deploy.ts
│   └── deps.ts
├── mise.toml                 # Task runner configuration
├── go.mod
└── README.md
```

### Adding a New Layout

1. Edit `layouts.json` (or use the API when implemented)
2. Define monitor configurations
3. Test with `ottoman client run`

## License

MIT
