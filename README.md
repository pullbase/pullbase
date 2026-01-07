# Pullbase: GitOps for Linux Servers

**GitOps for Linux Servers** — Manage packages, services, and configuration files using Git as your source of truth.

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Pullbase brings Argo CD-style GitOps workflows to traditional servers (VMs, bare-metal). Define your desired server state in YAML, commit to Git, and let Pullbase agents keep your fleet in sync.

## Why Pullbase?

Managing configurations across fleets of non-Kubernetes servers often means manual SSH sessions, complex push-based tools, or fragile scripts. This leads to configuration drift, inconsistency, and operational overhead.

Pullbase solves this with:

- **Git as Source of Truth** — Define packages, services, and files in YAML. Git history is your audit trail.
- **Pull-Based Model** — Lightweight agents fetch and apply configurations. No inbound SSH required.
- **Automatic Drift Detection** — Agents continuously compare actual vs. desired state and auto-reconcile.
- **One-Click Rollbacks** — Point to any previous commit and agents revert automatically.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Git Repository                         │
│                    (config.yaml files)                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Pullbase Server                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Dashboard  │  │     API     │  │   Git Monitor       │  │
│  │  (Web UI)   │  │             │  │   (Webhooks/Poll)   │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        ┌──────────┐   ┌──────────┐   ┌──────────┐
        │  Agent   │   │  Agent   │   │  Agent   │
        │ (web-01) │   │ (web-02) │   │ (db-01)  │
        └──────────┘   └──────────┘   └──────────┘
```

- **Server**: Manages environments, monitors Git, serves the dashboard, coordinates agents
- **Agents**: Run on managed servers, pull configurations, apply desired state, report status

## Quick Start

Get Pullbase running in under 5 minutes.

### Prerequisites

- Docker 24.0+ with Compose plugin
- Git

### 1. Start Pullbase

```bash
# Create a docker-compose.yml
cat > docker-compose.yml << 'EOF'
services:
  db:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: pullbaseuser
      POSTGRES_PASSWORD: changeme
      POSTGRES_DB: pullbasedb
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U pullbaseuser -d pullbasedb"]
      interval: 10s
      timeout: 5s
      retries: 5

  pullbase:
    image: pullbaseio/pullbase:latest
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    ports:
      - "8080:8080"
    environment:
      PULLBASE_DB_TYPE: postgres
      PULLBASE_DB_HOST: db
      PULLBASE_DB_PORT: 5432
      PULLBASE_DB_USER: pullbaseuser
      PULLBASE_DB_PASSWORD: changeme
      PULLBASE_DB_NAME: pullbasedb
      PULLBASE_JWT_SECRET: generate-a-secure-random-string-here
      PULLBASE_WEBHOOK_SECRET_KEY: generate-another-secure-string
      PULLBASE_BOOTSTRAP_SECRET_FILE: /app/secrets/bootstrap.secret
    volumes:
      - pullbase_secrets:/app/secrets

volumes:
  postgres_data:
  pullbase_secrets:
EOF

docker compose up -d
```

### 2. Bootstrap the Admin

```bash
# Get the one-time bootstrap secret
docker compose exec pullbase cat /app/secrets/bootstrap.secret

# Create your admin account
docker compose exec pullbase pullbasectl auth bootstrap-admin \
  --server-url http://localhost:8080 \
  --bootstrap-secret "YOUR_SECRET" \
  --username admin \
  --password 'YourSecurePassword!'
```

### 3. Access the Dashboard

Open **http://localhost:8080** and sign in with your admin credentials.

### 4. Add Your First Server

1. Create an **Environment** (links to your Git config repo)
2. Create a **Server** and generate an agent token
3. Run the install script on your target server:

```bash
curl -fsSL "https://your-pullbase-host/api/v1/servers/web-01/install-script?token=pbt_xxx" | sudo bash
```

The agent installs as a systemd service and starts reporting status immediately.

## Configuration Repository

Your Git repository defines the desired state. Create a `config.yaml`:

```yaml
serverMetadata:
  name: web-01
  environment: production

packages:
  - name: nginx
    state: latest
  - name: curl
    state: present

services:
  - name: nginx
    enabled: true
    state: running

files:
  - path: /etc/nginx/nginx.conf
    content: |
      user nginx;
      worker_processes auto;
      events { worker_connections 1024; }
      http {
        server {
          listen 80;
          location / { return 200 'Hello from Pullbase'; }
        }
      }
    mode: "0644"
    reloadService: nginx
```

Commit this to your repo, create an environment pointing to it, and agents will apply the configuration.

## Supported Platforms

**Package Managers:** apt, yum/dnf, apk

**Service Managers:** systemd, supervisor, OpenRC

**Distributions:** Ubuntu, Debian, RHEL, Rocky Linux, Fedora, Alpine Linux

## Documentation

Full documentation is available at **[docs.pullbase.io](https://docs.pullbase.io)**:

- [Installation Guide](https://docs.pullbase.io/installation)
- [Quickstart](https://docs.pullbase.io/quickstart)
- [Configuration Repository Guide](https://docs.pullbase.io/configuration-repository)
- [Agent Operations](https://docs.pullbase.io/agent-operations)
- [GitHub App Integration](https://docs.pullbase.io/github-app-integration) (private repos)
- [Security & Hardening](https://docs.pullbase.io/security-hardening)
- [CLI Reference](https://docs.pullbase.io/reference/cli)


## Building from Source

```bash
# Prerequisites: Go 1.22+, Node.js 20+

git clone https://github.com/pullbase/pullbase.git
cd pullbase

# Build server with embedded UI
./scripts/build-with-ui.sh

# Build agent
cd agent && go build -o pullbase-agent
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

Pullbase is licensed under the [Apache License 2.0](LICENSE).

## Links

- **Website**: [pullbase.io](https://pullbase.io)
- **Documentation**: [docs.pullbase.io](https://docs.pullbase.io)
- **GitHub**: [github.com/pullbase/pullbase](https://github.com/pullbase/pullbase)
