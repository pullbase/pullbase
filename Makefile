.PHONY: up down migrate migrate-up migrate-down migrate-force migrate-version migrate-create build-ui build-ui-docker docker-up docker-up-bg docker-up-no-cache docker-down dev

# Docker Compose commands with UI building
docker-up: build-ui-docker
	@echo "✅ PullBase started with latest UI!"

docker-up-bg:
	./scripts/build-ui-for-docker.sh

docker-up-no-cache:
	./scripts/build-ui-for-docker.sh --no-cache

docker-down:
	docker-compose down

# Standard Docker Compose commands (without UI rebuild)
up:
	docker-compose up

down:
	docker-compose down

build:
	docker-compose build

build-no-cache:
	docker-compose build --no-cache

# Local development commands

# Build with embedded Web UI (for production)
build-ui:
	./scripts/build-with-ui.sh

# Build UI for Docker (copies to embed directory and runs docker-compose)
build-ui-docker:
	./scripts/build-ui-for-docker.sh

# Run in development mode (separate Vite dev server)
dev:
	@echo "🚀 Starting development servers..."
	@echo "📦 Starting Vite dev server on http://localhost:5173"
	@cd web && npm run dev &
	@echo "🏗️  Starting Go server on https://127.0.0.1:8080"
	@cd server && go run ./cmd/server

# Migration commands

# Run migrations
migrate-up:
	docker run -it --rm --network pullbase_pullbase-network -v $(PWD):/app -w /app/server golang:1.24-alpine sh -c "apk add --no-cache bash curl && curl -L https://github.com/golang-migrate/migrate/releases/download/v4.18.3/migrate.linux-amd64.tar.gz | tar xvz && mv migrate /usr/local/bin/migrate && chmod +x ./migrations/migrate.sh && PULLBASE_DB_HOST=db PULLBASE_DB_USER=pullbaseuser PULLBASE_DB_PASSWORD=pullbasepass PULLBASE_DB_NAME=pullbasedb ./migrations/migrate.sh up"

# Rollback migrations
migrate-down:
	docker run -it --rm --network pullbase_pullbase-network -v $(PWD):/app -w /app/server golang:1.24-alpine sh -c "apk add --no-cache bash curl && curl -L https://github.com/golang-migrate/migrate/releases/download/v4.18.3/migrate.linux-amd64.tar.gz | tar xvz && mv migrate /usr/local/bin/migrate && chmod +x ./migrations/migrate.sh && PULLBASE_DB_HOST=db PULLBASE_DB_USER=pullbaseuser PULLBASE_DB_PASSWORD=pullbasepass PULLBASE_DB_NAME=pullbasedb ./migrations/migrate.sh down"

# Force migration to specific version
migrate-force:
	@read -p "Version to force: " version; \
	docker run -it --rm --network pullbase_pullbase-network -v $(PWD):/app -w /app/server golang:1.24-alpine sh -c "apk add --no-cache bash curl && curl -L https://github.com/golang-migrate/migrate/releases/download/v4.18.3/migrate.linux-amd64.tar.gz | tar xvz && mv migrate /usr/local/bin/migrate && chmod +x ./migrations/migrate.sh && PULLBASE_DB_HOST=db PULLBASE_DB_USER=pullbaseuser PULLBASE_DB_PASSWORD=pullbasepass PULLBASE_DB_NAME=pullbasedb ./migrations/migrate.sh force $$version"

# Check migration version
migrate-version:
	docker run -it --rm --network pullbase_pullbase-network -v $(PWD):/app -w /app/server golang:1.24-alpine sh -c "apk add --no-cache bash curl && curl -L https://github.com/golang-migrate/migrate/releases/download/v4.18.3/migrate.linux-amd64.tar.gz | tar xvz && mv migrate /usr/local/bin/migrate && chmod +x ./migrations/migrate.sh && PULLBASE_DB_HOST=db PULLBASE_DB_USER=pullbaseuser PULLBASE_DB_PASSWORD=pullbasepass PULLBASE_DB_NAME=pullbasedb ./migrations/migrate.sh version"

# Create a new migration
migrate-create:
	@read -p "Migration name: " name; \
	docker run -it --rm --network pullbase_pullbase-network -v $(PWD):/app -w /app/server golang:1.24-alpine sh -c "apk add --no-cache bash curl && curl -L https://github.com/golang-migrate/migrate/releases/download/v4.18.3/migrate.linux-amd64.tar.gz | tar xvz && mv migrate /usr/local/bin/migrate && chmod +x ./migrations/migrate.sh && PULLBASE_DB_HOST=db PULLBASE_DB_USER=pullbaseuser PULLBASE_DB_PASSWORD=pullbasepass PULLBASE_DB_NAME=pullbasedb ./migrations/migrate.sh create $$name"

# Run all migrations
migrate: migrate-up 
