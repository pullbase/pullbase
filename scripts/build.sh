#!/bin/bash
set -e  # Exit on any error

# Ensure we're in the project root
cd "$(dirname "$0")/.."

# Initialize go modules if they don't exist
if [ ! -f "server/go.mod" ]; then
    echo "Initializing server go modules..."
    cd server
    go mod init github.com/pullbase/pullbase/server
    go mod tidy
    cd ..
fi

if [ ! -f "agent/go.mod" ]; then
    echo "Initializing agent go modules..."
    cd agent
    go mod init github.com/pullbase/pullbase/agent
    go mod tidy
    cd ..
fi

# Build the agent
echo "Building agent..."
cd agent
CGO_ENABLED=0 GOOS=linux go build -o pullbase-agent
cd ..

# Build the server
echo "Building server..."
cd server
CGO_ENABLED=0 GOOS=linux go build -o pullbase-server
cd ..

# Build Docker images
echo "Building Docker images..."
docker-compose build