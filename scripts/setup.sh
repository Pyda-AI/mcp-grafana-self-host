#!/bin/bash
# Setup script for mcp-grafana-self-host
# This script helps generate tokens and create configuration

set -e

echo "=== MCP Grafana Self-Hosted Setup ==="
echo ""

# Check for openssl
if ! command -v openssl &> /dev/null; then
    echo "Error: openssl is required but not installed."
    exit 1
fi

# Generate MCP auth token
echo "Generating MCP auth token (256-bit)..."
MCP_TOKEN=$(openssl rand -hex 32)
echo "✓ Generated token: $MCP_TOKEN"
echo ""

# Create or update .env file
ENV_FILE=".env"
if [ -f "$ENV_FILE" ]; then
    echo "Found existing $ENV_FILE, adding missing variables..."
    
    # Function to add variable if it doesn't exist
    add_if_missing() {
        local var_name="$1"
        local var_value="$2"
        local comment="$3"
        if ! grep -q "^${var_name}=" "$ENV_FILE"; then
            if [ -n "$comment" ]; then
                echo "" >> "$ENV_FILE"
                echo "# $comment" >> "$ENV_FILE"
            fi
            echo "${var_name}=${var_value}" >> "$ENV_FILE"
            echo "  ✓ Added $var_name"
        else
            echo "  - $var_name already exists, skipping"
        fi
    }
    
    add_if_missing "GRAFANA_URL" "http://localhost:3000" "Grafana connection"
    add_if_missing "GRAFANA_SERVICE_ACCOUNT_TOKEN" "your-grafana-service-account-token" ""
    add_if_missing "MCP_AUTH_TOKEN" "$MCP_TOKEN" "MCP Server Authentication"
    add_if_missing "MCP_SERVER_PORT" "8443" "Server settings"
    add_if_missing "MCP_LOG_LEVEL" "info" "Optional: Log level (debug, info, warn, error)"
    
    echo "✓ Updated $ENV_FILE"
else
    echo "Creating $ENV_FILE..."
    cat > "$ENV_FILE" << EOF
# MCP Grafana Self-Hosted Configuration
# Generated on $(date)

# Grafana connection (update these with your values)
GRAFANA_URL=http://localhost:3000
GRAFANA_SERVICE_ACCOUNT_TOKEN=your-grafana-service-account-token

# MCP Server Authentication
MCP_AUTH_TOKEN=$MCP_TOKEN

# Server settings
MCP_SERVER_PORT=8443

# Optional: Log level (debug, info, warn, error)
MCP_LOG_LEVEL=info
EOF
    echo "✓ Created $ENV_FILE"
fi
echo ""

# Create config.yaml from example if it doesn't exist
if [ ! -f "config.yaml" ] && [ -f "config.yaml.example" ]; then
    echo "Creating config.yaml from example..."
    cp config.yaml.example config.yaml
    # Update the token in config.yaml
    sed -i.bak "s/^mcp_auth_token:.*/mcp_auth_token: \"$MCP_TOKEN\"/" config.yaml 2>/dev/null || \
    sed -i '' "s/^mcp_auth_token:.*/mcp_auth_token: \"$MCP_TOKEN\"/" config.yaml
    rm -f config.yaml.bak
    echo "✓ Created config.yaml"
fi

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "1. Edit $ENV_FILE and set your GRAFANA_URL and GRAFANA_SERVICE_ACCOUNT_TOKEN"
echo "2. Build and run the server:"
echo ""
echo "   # Option A: Go directly (auto-loads .env)"
echo "   go build -o mcp-grafana ./cmd/mcp-grafana && ./mcp-grafana"
echo ""
echo "   # Option B: Docker Compose (recommended)"
echo "   docker compose up -d mcp-grafana"
echo ""
echo "   # Option C: Docker manually"
echo "   docker build -t mcp-grafana-self-host ."
echo "   docker run --rm -p 8443:8443 --env-file .env mcp-grafana-self-host"
echo ""
echo "3. Save your MCP_AUTH_TOKEN for use in the frontend:"
echo "   $MCP_TOKEN"
echo ""
echo "Configuration priority: ENV vars > .env file > config.yaml > defaults"
echo ""

