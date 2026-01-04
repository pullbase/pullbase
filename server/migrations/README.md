# PullBase Database Migrations

This directory contains database migrations for PullBase. We use [golang-migrate](https://github.com/golang-migrate/migrate) to manage database migrations.

## Migration Files

Migrations follow the naming pattern: `000001_description.up.sql` and `000001_description.down.sql`. 

- The numeric prefix determines the order of migrations
- Each migration has an "up" and "down" SQL file
- "Up" migrations are applied during upgrades
- "Down" migrations are applied during rollbacks

## Current Migrations

1. **000001_create_initial_schema** - Creates the initial database schema with users, servers, and other core tables
2. **000002_add_auto_reconcile** - Adds the auto_reconcile column to the servers table
3. **000003_add_soft_delete** - Implements soft delete functionality for servers

## Using the Migration CLI

The official golang-migrate CLI tool provides commands to manage migrations:

### Install the CLI

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### Apply all pending migrations

```bash
migrate -path server/migrations -database "postgres://user:password@localhost:5432/pullbase?sslmode=disable" up
```

### Apply a specific number of migrations

```bash
migrate -path server/migrations -database "postgres://user:password@localhost:5432/pullbase?sslmode=disable" up 1
```

### Rollback all migrations

```bash
migrate -path server/migrations -database "postgres://user:password@localhost:5432/pullbase?sslmode=disable" down
```

### Rollback a specific number of migrations

```bash
migrate -path server/migrations -database "postgres://user:password@localhost:5432/pullbase?sslmode=disable" down 1
```

### Check current migration version

```bash
migrate -path server/migrations -database "postgres://user:password@localhost:5432/pullbase?sslmode=disable" version
```

### Force migration to a specific version

```bash
migrate -path server/migrations -database "postgres://user:password@localhost:5432/pullbase?sslmode=disable" force 2
```

### Create a new migration

```bash
migrate create -ext sql -dir server/migrations -seq description_of_migration
```

This will create two new files:
1. `000004_description_of_migration.up.sql` - SQL commands to apply the change
2. `000004_description_of_migration.down.sql` - SQL commands to revert the change

## Using Environment Variables

For convenience, you can set the database URL as an environment variable:

```bash
export POSTGRESQL_URL="postgres://user:password@localhost:5432/pullbase?sslmode=disable"
migrate -path server/migrations -database "${POSTGRESQL_URL}" up
```

## Helper Scripts

### Shell Script

For easier use, there is a shell script (`migrate.sh`) that handles the common database parameters and simplifies the commands:

```bash
# Apply all migrations
./migrate.sh up

# Apply 2 migrations
./migrate.sh up 2

# Rollback 1 migration
./migrate.sh down 1

# Check current version
./migrate.sh version

# Create a new migration
./migrate.sh create add_user_status

# Override database credentials
DB_USER=myuser DB_PASSWORD=mypass ./migrate.sh up
```

Run `./migrate.sh -h` for more information.

### Make Commands

The PullBase Makefile includes commands for running migrations with Docker. These commands ensure the migration tool is correctly installed and configured:

```bash
# Apply migrations
make migrate-up

# Rollback migrations
make migrate-down

# Check migration version
make migrate-version

# Force migration to specific version
make migrate-force

# Create a new migration
make migrate-create
```

These commands work with Docker, so you don't need to install any tools locally.

## Best Practices

1. Keep migrations small and focused on a single change
2. Always provide a "down" migration that properly reverts changes
3. Use transactions for complex migrations
4. Test migrations thoroughly before applying to production
5. When modifying tables, consider data preservation 