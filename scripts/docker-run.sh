#!/bin/bash
# Docker run script for mcp-grafana-self-host
# Builds Docker image and runs with .env configuration

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

IMAGE_NAME="mcp-grafana-self-host"
CONTAINER_NAME="mcp-grafana"

# Check for .env file
if [ ! -f ".env" ]; then
    echo "Error: .env file not found."
    echo "Run ./scripts/setup.sh first to create configuration."
    exit 1
fi

# Parse arguments
BUILD=false
NETWORK=""
DETACH=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --build|-b)
            BUILD=true
            shift
            ;;
        --network|-n)
            NETWORK="$2"
            shift 2
            ;;
        --detach|-d)
            DETACH=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --build, -b      Force rebuild Docker image"
            echo "  --network, -n    Docker network to connect to"
            echo "  --detach, -d     Run in detached mode"
            echo "  --help, -h       Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Build image if needed or requested
if [ "$BUILD" = true ] || [ -z "$(docker images -q $IMAGE_NAME 2>/dev/null)" ]; then
    echo "Building Docker image..."
    docker build -t "$IMAGE_NAME" .
fi

# Stop existing container if running
if [ "$(docker ps -q -f name=$CONTAINER_NAME)" ]; then
    echo "Stopping existing container..."
    docker stop "$CONTAINER_NAME"
fi

# Remove existing container if exists
if [ "$(docker ps -aq -f name=$CONTAINER_NAME)" ]; then
    docker rm "$CONTAINER_NAME"
fi

# Build docker run command
DOCKER_CMD="docker run --rm"

if [ "$DETACH" = true ]; then
    DOCKER_CMD="$DOCKER_CMD -d"
fi

DOCKER_CMD="$DOCKER_CMD --name $CONTAINER_NAME"

# Get port from .env or default
PORT=$(grep -E '^MCP_SERVER_PORT=' .env | cut -d'=' -f2 | tr -d '"' || echo "8443")
DOCKER_CMD="$DOCKER_CMD -p $PORT:$PORT"

if [ -n "$NETWORK" ]; then
    DOCKER_CMD="$DOCKER_CMD --network $NETWORK"
fi

DOCKER_CMD="$DOCKER_CMD --env-file .env $IMAGE_NAME"

echo "Running: $DOCKER_CMD"
eval "$DOCKER_CMD"

