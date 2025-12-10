# MCP Grafana Self-Hosted Server

A self-hosted [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server for Grafana, configured via environment variables and designed for Docker deployment.

This is a simplified fork of [grafana/mcp-grafana](https://github.com/grafana/mcp-grafana) that:
- Uses **only streamable-http transport** on the `/mcp` endpoint
- Configures via **.env file** (no config.yaml)
- Defaults to **port 8443**
- Supports **TLS via environment variables**
- Includes **CLI flag for organization ID**

## Requirements

- **Grafana version 9.0 or later** for full functionality
- Docker (recommended) or Go 1.24+
- Grafana service account token with appropriate permissions

## Quick Start

### 1. Setup

   ```bash
# Generate .env file with MCP auth token
./scripts/setup.sh

# Edit .env with your Grafana credentials
nano .env
```

### 2. Configure

Update `.env` with your Grafana instance details:

   ```bash
GRAFANA_URL=http://localhost:3000
GRAFANA_SERVICE_ACCOUNT_TOKEN=your-token-here
MCP_AUTH_TOKEN=generated-by-setup-script
   ```

### 3. Run

   ```bash
# Option A: Docker (recommended)
./run.sh

# Option B: Go directly
./scripts/run.sh

# Option C: Docker with options
./scripts/docker-run.sh --build --detach
```

The server will be available at `http://localhost:8443/mcp`

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `GRAFANA_URL` | URL of your Grafana instance |
| `GRAFANA_SERVICE_ACCOUNT_TOKEN` | Grafana service account token for authentication |
| `MCP_AUTH_TOKEN` | Bearer token for MCP client authentication |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MCP_SERVER_PORT` | `8443` | Port for the MCP server |
| `MCP_LOG_LEVEL` | `info` | Log level: debug, info, warn, error |
| `GRAFANA_ORG_ID` | - | Grafana organization ID for multi-org support |

### TLS Client Configuration (for connecting to Grafana)

| Variable | Description |
|----------|-------------|
| `MCP_TLS_CERT_FILE` | Path to TLS certificate file for client authentication |
| `MCP_TLS_KEY_FILE` | Path to TLS private key file for client authentication |
| `MCP_TLS_CA_FILE` | Path to TLS CA certificate file for server verification |
| `MCP_TLS_SKIP_VERIFY` | Skip TLS certificate verification (insecure, testing only) |

### TLS Server Configuration (for HTTPS server)

| Variable | Description |
|----------|-------------|
| `MCP_SERVER_TLS_CERT_FILE` | Path to TLS certificate file for HTTPS server |
| `MCP_SERVER_TLS_KEY_FILE` | Path to TLS private key file for HTTPS server |

## CLI Flags

All configuration can be overridden with CLI flags:

| Flag | Description |
|------|-------------|
| `--port` | Server port |
| `--log-level` | Log level (debug, info, warn, error) |
| `--org-id` | Grafana organization ID |
| `--debug` | Enable debug mode for Grafana transport |
| `--tls-cert-file` | TLS client certificate |
| `--tls-key-file` | TLS client private key |
| `--tls-ca-file` | TLS CA certificate |
| `--tls-skip-verify` | Skip TLS verification |
| `--server.tls-cert-file` | Server TLS certificate |
| `--server.tls-key-file` | Server TLS private key |
| `--disable-<category>` | Disable specific tool categories |

**Configuration priority:** CLI flags > Environment variables > Defaults

## Examples

### Basic Usage

```bash
# .env file
GRAFANA_URL=http://localhost:3000
GRAFANA_SERVICE_ACCOUNT_TOKEN=glsa_...
MCP_AUTH_TOKEN=abc123...
```

```bash
./run.sh
# Server: http://localhost:8443/mcp
```

### Multi-Organization Support

```bash
# .env file
GRAFANA_ORG_ID=2
```

Or via CLI:

```bash
./mcp-grafana --org-id 2
```

### TLS Client (connecting to Grafana with mTLS)

```bash
# .env file
MCP_TLS_CERT_FILE=/path/to/client.crt
MCP_TLS_KEY_FILE=/path/to/client.key
MCP_TLS_CA_FILE=/path/to/ca.crt
```

The `run.sh` script automatically mounts these certificates into Docker.

### HTTPS Server

```bash
# .env file
MCP_SERVER_TLS_CERT_FILE=/path/to/server.crt
MCP_SERVER_TLS_KEY_FILE=/path/to/server.key
```

```bash
./run.sh
# Server: https://localhost:8443/mcp
```

### Docker with Custom Port

```bash
# .env file
MCP_SERVER_PORT=9000
```

```bash
./run.sh
# Server: http://localhost:9000/mcp
```

## Authentication

MCP clients must include the `MCP_AUTH_TOKEN` in the `Authorization` header:

```
Authorization: Bearer <your-mcp-auth-token>
```

**Note:** The `/healthz` endpoint is public and does not require authentication.

## Health Check

```bash
curl http://localhost:8443/healthz
# Response: ok (200 OK)
```

## Tool Categories

Disable specific tool categories to reduce context window usage:

```bash
./mcp-grafana --disable-oncall --disable-sift
```

Available categories:
- `search`, `datasource`, `dashboard`, `folder`
- `prometheus`, `loki`, `tempo`, `mimir`, `pyroscope`
- `incident`, `sift`, `alerting`, `oncall`
- `admin`, `asserts`, `navigation`, `annotations`, `proxied`

## Development

### Building

```bash
go build -o mcp-grafana ./cmd/mcp-grafana
```

### Running Locally

```bash
# Auto-loads .env
./mcp-grafana
```

### Docker Build

```bash
docker build -t mcp-grafana-self-host .
docker run --rm -p 8443:8443 --env-file .env mcp-grafana-self-host
```

## Differences from Upstream

This self-hosted version differs from [grafana/mcp-grafana](https://github.com/grafana/mcp-grafana):

1. **Transport:** Only streamable-http (no stdio/SSE)
2. **Endpoint:** Fixed to `/mcp` (not configurable)
3. **Configuration:** Environment variables only (no config.yaml)
4. **TLS:** Supports env vars for TLS configuration
5. **Organization ID:** CLI flag `--org-id` for multi-org support
6. **Port:** Defaults to 8443 instead of 8000

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).

## Links

- [Upstream Project](https://github.com/grafana/mcp-grafana)
- [Model Context Protocol](https://modelcontextprotocol.io/)
- [Grafana Service Accounts](https://grafana.com/docs/grafana/latest/administration/service-accounts/)
