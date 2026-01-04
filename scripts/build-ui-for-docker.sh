#!/bin/bash

set -e

# Optional flag handling
NO_CACHE=false
if [[ "$1" == "--no-cache" ]]; then
  NO_CACHE=true
  shift
fi

echo "🔧 Building PullBase UI for Docker..."

# Navigate to project root
cd "$(dirname "$0")/.."

# Clean previous builds
echo "🧹 Cleaning previous UI builds..."
rm -rf web/dist
rm -rf server/pkg/server/ui

# Build Vite application
echo "📦 Building Vite React web UI..."
cd web
npm ci
npm run build
cd ..

# Copy built UI files to Go embed directory
echo "📋 Copying UI files to Go server embed directory..."
mkdir -p server/pkg/server/ui
cp -r web/dist/* server/pkg/server/ui/

echo "✅ UI build complete! Files copied to server/pkg/server/ui/"
echo ""
echo "🐳 Starting Docker Compose..."
if [[ "$NO_CACHE" == true ]]; then
  echo "♻️  Building Docker images without cache..."
  docker-compose build --no-cache
  docker-compose up
else
  docker-compose up --build
fi

echo ""
echo "🚀 PullBase is starting with embedded UI!"
echo "🌐 Web UI will be available at: https://localhost:8080/ui/"
echo "📡 API will be available at: https://localhost:8080/api/v1/"
echo ""
echo "📋 To view logs: docker-compose logs -f"
echo "🛑 To stop: docker-compose down"
