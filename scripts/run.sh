#!/bin/bash
# Run script for mcp-grafana-self-host
# Builds and runs the server using environment from .env file

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# Load .env file if it exists
if [ -f ".env" ]; then
    echo "Loading configuration from .env..."
    set -a
    source .env
    set +a
fi

# Validate required environment variables
if [ -z "$MCP_AUTH_TOKEN" ]; then
    echo "Warning: MCP_AUTH_TOKEN is not set. Authentication will be disabled."
    echo "Run ./scripts/setup.sh to generate a token."
fi

if [ -z "$GRAFANA_URL" ]; then
    echo "Warning: GRAFANA_URL is not set. Using default: http://localhost:3000"
fi

# Build
echo "Building mcp-grafana..."
go build -o mcp-grafana ./cmd/mcp-grafana

# Run
echo "Starting mcp-grafana server..."
./mcp-grafana

