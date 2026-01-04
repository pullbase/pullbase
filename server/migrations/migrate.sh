#!/bin/bash


if [ -n "$PULLBASE_DB_HOST" ]; then
    DB_HOST="$PULLBASE_DB_HOST"
    DB_USER="$PULLBASE_DB_USER"
    DB_PASSWORD="$PULLBASE_DB_PASSWORD"
    DB_NAME="$PULLBASE_DB_NAME"
# Default values if not in Docker
else
    DB_HOST=${DB_HOST:-"localhost"}
    DB_PORT=${DB_PORT:-"5432"}
    DB_USER=${DB_USER:-"pullbaseuser"}
    DB_PASSWORD=${DB_PASSWORD:-"pullbasepass"}
    DB_NAME=${DB_NAME:-"pullbasedb"}
    DB_SSLMODE=${DB_SSLMODE:-"disable"}
fi

# Build database URL
POSTGRESQL_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT:-5432}/${DB_NAME}?sslmode=${DB_SSLMODE:-disable}"

# Migrations directory path
MIGRATIONS_PATH="migrations"

# Check if we're in the server directory or project root
if [ -d "server/migrations" ]; then
    MIGRATIONS_PATH="server/migrations"
fi

# Display help
function show_help {
    echo "PullBase Migration Helper"
    echo "========================="
    echo ""
    echo "Usage: ./migrate.sh [options] [command]"
    echo ""
    echo "Commands:"
    echo "  up [N]       Apply all or N up migrations"
    echo "  down [N]     Apply all or N down migrations"
    echo "  version      Show current migration version"
    echo "  force V      Force migration to version V"
    echo "  create NAME  Create a new migration named NAME"
    echo ""
    echo "Options:"
    echo "  -h, --help   Show this help message"
    echo ""
    echo "Environment variables:"
    echo "  DB_HOST      Database host (default: localhost)"
    echo "  DB_PORT      Database port (default: 5432)"
    echo "  DB_USER      Database user (default: postgres)"
    echo "  DB_PASSWORD  Database password (default: postgres)"
    echo "  DB_NAME      Database name (default: pullbase)"
    echo "  DB_SSLMODE   Database SSL mode (default: disable)"
    echo ""
    echo "Example:"
    echo "  ./migrate.sh up"
    echo "  DB_USER=myuser DB_PASSWORD=mypass ./migrate.sh down 1"
}

# Show help if no arguments or help flag
if [ "$1" == "" ] || [ "$1" == "-h" ] || [ "$1" == "--help" ]; then
    show_help
    exit 0
fi

# Handle commands
case "$1" in
    up)
        if [ "$2" == "" ]; then
            migrate -path $MIGRATIONS_PATH -database "$POSTGRESQL_URL" up
        else
            migrate -path $MIGRATIONS_PATH -database "$POSTGRESQL_URL" up $2
        fi
        ;;
    down)
        if [ "$2" == "" ]; then
            migrate -path $MIGRATIONS_PATH -database "$POSTGRESQL_URL" down
        else
            migrate -path $MIGRATIONS_PATH -database "$POSTGRESQL_URL" down $2
        fi
        ;;
    version)
        migrate -path $MIGRATIONS_PATH -database "$POSTGRESQL_URL" version
        ;;
    force)
        if [ "$2" == "" ]; then
            echo "Error: Version number required for force command"
            exit 1
        else
            migrate -path $MIGRATIONS_PATH -database "$POSTGRESQL_URL" force $2
        fi
        ;;
    create)
        if [ "$2" == "" ]; then
            echo "Error: Migration name required for create command"
            exit 1
        else
            migrate create -ext sql -dir $MIGRATIONS_PATH -seq $2
        fi
        ;;
    *)
        echo "Error: Unknown command '$1'"
        show_help
        exit 1
        ;;
esac 