# Contributing to Pullbase

Thank you for your interest in contributing to Pullbase! This document provides guidelines and instructions for contributing.

## Code of Conduct

Please be respectful and constructive in all interactions. We're building something together.

## How to Contribute

### Reporting Bugs

1. Check existing [GitHub Issues](https://github.com/pullbase/pullbase/issues) to avoid duplicates
2. Create a new issue with:
   - Clear title describing the bug
   - Steps to reproduce
   - Expected vs actual behavior
   - Pullbase version and environment details

### Suggesting Features

1. Open a GitHub Issue with the `enhancement` label
2. Describe the use case and proposed solution
3. Be open to discussion and alternative approaches

### Pull Requests

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Ensure tests pass (see Testing section below)
5. Update documentation if the change alters behavior or adds features (docs live at https://github.com/pullbase/docs)
6. Submit a PR with a clear description

## Development Setup

### Prerequisites

- Go 1.22+
- Node.js 20+ (for web UI)
- Docker (for local testing)
- PostgreSQL 16+ (or use SQLite 3.35.0+ for development)

> **Note:** SQLite 3.35.0+ is required because down migrations use `ALTER TABLE ... DROP COLUMN`, which was added in that version.

### Building

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/pullbase.git
cd pullbase

# Build server with embedded UI
./scripts/build-with-ui.sh

# Build agent
cd agent && go build -o pullbase-agent
```

### Testing

Quick sanity (uses stubs to avoid UI embed):

```bash
go test -tags=test ./...
```

Full tests (requires built UI assets for server without stubs):

```bash
./scripts/build-with-ui.sh   # ensures embedded UI is present
go test ./...
```

Integration tests (require Docker/Postgres, slower):

```bash
go test -tags=integration ./...
```

### Documentation & Swagger

- Update relevant docs in https://github.com/pullbase/docs when features or behavior change.
- Regenerate Swagger artifacts after API changes:
  ```bash
  swag init --parseDependency --parseInternal -g server/cmd/server/main.go -o server/docs
  ```

### Running Locally

```bash
# Copy example files
cp docker-compose.example.yml docker-compose.yml
cp .env.example .env

# Edit .env with your settings
# Start the stack
docker compose up -d
```

## Code Style

- Follow standard Go conventions
- Run `gofmt` before committing
- Keep functions focused and testable
- Add tests for new functionality

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
