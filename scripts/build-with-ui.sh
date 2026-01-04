#!/bin/bash

set -e

echo "🔧 Building PullBase with embedded Web UI..."

# Navigate to project root
cd "$(dirname "$0")/.."

# Clean previous builds
echo "🧹 Cleaning previous builds..."
rm -rf web/dist
rm -rf server/pkg/server/ui

# Build Vite application
echo "📦 Building Vite React web UI..."
cd web
npm ci
npm run build
cd ..

# Copy built UI files to Go embed directory
echo "📋 Copying UI files to Go server..."
mkdir -p server/pkg/server/ui
cp -r web/dist/* server/pkg/server/ui/

# Build Go application with embedded UI
echo "🏗️  Building Go server with embedded UI..."
cd server
go mod tidy
go build -o ../bin/pullbase-server ./cmd/server

echo "✅ Build complete! Binary available at: bin/pullbase-server"
echo ""
echo "🚀 To start PullBase:"
echo "   cd server && ../bin/pullbase-server"
echo ""
echo "🌐 Web UI will be available at: https://127.0.0.1:8080/ui/"
echo "📡 API will be available at: https://127.0.0.1:8080/api/v1/"
echo ""
echo "🔒 Security: Web UI is only accessible from localhost (127.0.0.1)" 