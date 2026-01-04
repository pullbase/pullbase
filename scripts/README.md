# PullBase Build Scripts

This directory contains scripts to help with building and deploying PullBase.

## Scripts Overview

### `build-with-ui.sh`
Builds PullBase with embedded Web UI for production deployment.

**What it does:**
- Builds the Vite React web UI (`web/`)
- Copies built assets to the Go server embed directory (`server/pkg/server/ui/`)
- Builds the Go server binary with embedded UI

**Usage:**
```bash
./scripts/build-with-ui.sh
# or
make build-ui
```

**Output:** `bin/pullbase-server` - A standalone binary with embedded web UI

### `build-ui-for-docker.sh`
Builds the UI and starts Docker Compose with the latest UI changes.

**What it does:**
- Builds the Vite React web UI
- Copies built assets to the Go server embed directory
- Rebuilds and starts Docker Compose services

**Usage:**
```bash
./scripts/build-ui-for-docker.sh
# or
make docker-up
```

## Development Workflows

### Local Development (Hot Reload)
For active UI development with hot reload:

```bash
make dev
# This starts:
# - Vite dev server on http://localhost:5173
# - Go server on https://127.0.0.1:8080
```

### Docker Development
When you want to test with Docker:

```bash
make docker-up
# This will:
# 1. Build the latest UI
# 2. Copy it to the embed directory  
# 3. Start Docker Compose with rebuilt containers
```

### Production Build
For standalone deployment:

```bash
make build-ui
# Creates: bin/pullbase-server (with embedded UI)
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make dev` | Start development servers (Vite + Go) |
| `make docker-up` | Build UI and start Docker Compose |
| `make docker-down` | Stop Docker Compose |
| `make build-ui` | Build standalone binary with embedded UI |
| `make up` | Start Docker Compose (without UI rebuild) |
| `make down` | Stop Docker Compose |

## UI Technology Stack

- **Frontend:** React + TypeScript + Vite
- **Styling:** Tailwind CSS
- **Routing:** React Router
- **Build:** Vite (replaced Next.js)
- **Embed:** Go embed for static assets

## Directory Structure

```
web/                     # Vite React application
├── src/                 # React source code
├── dist/                # Vite build output
└── package.json         # Frontend dependencies

server/pkg/server/ui/    # Embedded UI assets (auto-generated)
├── index.html           # Main HTML file
├── assets/              # JS, CSS, fonts
└── *.svg, *.ico         # Static assets

scripts/                 # Build automation
├── build-with-ui.sh     # Production build script
└── build-ui-for-docker.sh # Docker build script
```

## Notes

- The `server/pkg/server/ui/` directory is auto-generated - don't edit manually
- Always use the build scripts to update embedded UI assets
- The Vite build creates optimized, production-ready assets
- Docker builds automatically include the latest UI when using `docker-up` target 