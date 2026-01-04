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
4. Ensure tests pass: `go test ./...`
5. Submit a PR with a clear description

## Development Setup

### Prerequisites

- Go 1.22+
- Node.js 20+ (for web UI)
- Docker (for local testing)
- PostgreSQL 16+ (or use SQLite for development)

### Building

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/pullbase.git
cd pullbase

# Build server with embedded UI
./scripts/build-with-ui.sh

# Build agent
cd agent && go build -o pullbase-agent

# Run tests
go test ./...
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
