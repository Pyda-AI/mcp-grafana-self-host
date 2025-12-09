#!/bin/bash
# Run the MCP Grafana server using Docker
# Reads configuration from .env file (if exists) or environment variables
# Transport: streamable-http (default)

set -e

IMAGE_NAME="mcp-grafana-self-host"

# Get port from environment, .env file, or default
get_port() {
    # First check if MCP_SERVER_PORT is already set in environment
    if [ -n "$MCP_SERVER_PORT" ]; then
        echo "$MCP_SERVER_PORT"
        return
    fi
    
    # Try to read from .env file
    if [ -f ".env" ]; then
        local port=$(grep -E "^MCP_SERVER_PORT=" .env 2>/dev/null | cut -d'=' -f2 | tr -d '"' | tr -d "'" | tr -d ' ')
        if [ -n "$port" ]; then
            echo "$port"
            return
        fi
    fi
    
    # Default port
    echo "8443"
}

PORT=$(get_port)

# Build if image doesn't exist or --build flag passed
if [ "$1" = "--build" ] || [ -z "$(docker images -q $IMAGE_NAME 2>/dev/null)" ]; then
    echo "Building Docker image..."
    docker build -t $IMAGE_NAME .
fi

# Prepare docker run command
DOCKER_ARGS="-d --rm -p ${PORT}:${PORT} --name mcp-grafana-server"

# Add .env file if it exists
if [ -f ".env" ]; then
    DOCKER_ARGS="$DOCKER_ARGS --env-file .env"
fi

echo "Starting MCP Grafana server..."
echo "  Transport: streamable-http"
echo "  Port: $PORT"
echo "  Endpoint: http://localhost:${PORT}/mcp"
echo ""

docker run $DOCKER_ARGS $IMAGE_NAME

echo ""
echo "Server started! Container ID:"
docker ps --filter "name=mcp-grafana-server" --format "{{.ID}}"
echo ""
echo "To view logs: docker logs -f mcp-grafana-server"
echo "To stop: docker stop mcp-grafana-server"

