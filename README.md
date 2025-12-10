# MCP Grafana Self-Hosted Server

A self-hosted [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server for Grafana, configured via environment variables and designed for Docker deployment.

This is a simplified fork of [grafana/mcp-grafana](https://github.com/grafana/mcp-grafana) that:
- Uses **only streamable-http transport** on the `/mcp` endpoint
- Configures via **.env file**
- Defaults to **port 8443**
- Supports **TLS via environment variables**

## Available Tools

| Category | Tool | Description |
|----------|------|-------------|
| **Search** | `search_dashboards` | Search for dashboards by query string |
| | `search_folders` | Search for folders by query string |
| **Datasource** | `list_datasources` | List available datasources, optionally filter by type |
| | `get_datasource_by_uid` | Get datasource details by UID |
| | `get_datasource_by_name` | Get datasource details by name |
| **Dashboard** | `get_dashboard_by_uid` | Get full dashboard JSON by UID |
| | `get_dashboard_summary` | Get compact dashboard overview (recommended) |
| | `get_dashboard_property` | Extract specific data via JSONPath |
| | `get_dashboard_panel_queries` | Get all panel queries from a dashboard |
| **Prometheus** | `query_prometheus` | Execute PromQL queries (instant or range) |
| | `list_prometheus_metric_names` | List metric names with regex filtering |
| | `list_prometheus_metric_metadata` | Get metric metadata |
| | `list_prometheus_label_names` | List label names |
| | `list_prometheus_label_values` | Get values for a specific label |
| **Loki** | `query_loki_logs` | Execute LogQL queries |
| | `query_loki_stats` | Get stream statistics for a selector |
| | `list_loki_label_names` | List available label names |
| | `list_loki_label_values` | Get values for a specific label |
| **Tempo** | `search_tempo_traces` | Search traces by service, span, tags, duration |
| | `get_tempo_trace` | Get full trace by trace ID |
| | `query_tempo_traceql` | Execute TraceQL queries |
| | `list_tempo_tag_names` | List available tag names |
| | `list_tempo_tag_values` | Get values for a specific tag |
| **Alerting** | `list_alert_rules` | List alert rules with state and labels |
| | `get_alert_rule_by_uid` | Get full alert rule configuration |
| | `list_contact_points` | List notification contact points |
| **Incident** | `list_incidents` | List incidents (active, resolved, drill) |
| | `get_incident` | Get incident details by ID |
| **OnCall** | `list_oncall_schedules` | List OnCall schedules |
| | `get_oncall_shift` | Get shift details |
| | `get_current_oncall_users` | Get users currently on-call |
| | `list_oncall_teams` | List OnCall teams |
| | `list_oncall_users` | List OnCall users |
| | `list_alert_groups` | List IRM alert groups |
| | `get_alert_group` | Get alert group details |
| **Sift** | `list_sift_investigations` | List Sift investigations |
| | `get_sift_investigation` | Get investigation by UUID |
| | `get_sift_analysis` | Get specific analysis from investigation |
| **Admin** | `list_teams` | Search for teams |
| | `list_users_by_org` | List users in current organization |
| **Navigation** | `generate_deeplink` | Generate URLs for dashboards, panels, explore |
| **Annotations** | `get_annotations` | Fetch annotations with filters |
| | `get_annotation_tags` | Get annotation tags |

## Requirements

- **Grafana version 9.0 or later** for full functionality
- Docker (recommended) or Go 1.24+
- Grafana service account token with appropriate permissions

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

### TLS Client Configuration (for connecting to Grafana)

| Variable | Description |
|----------|-------------|
| `MCP_TLS_CERT_FILE` | Path to TLS certificate file for client authentication |
| `MCP_TLS_KEY_FILE` | Path to TLS private key file for client authentication |
| `MCP_TLS_CA_FILE` | Path to TLS CA certificate file for server verification |

### TLS Server Configuration (for HTTPS server)

| Variable | Description |
|----------|-------------|
| `MCP_SERVER_TLS_CERT_FILE` | Path to TLS certificate file for HTTPS server |
| `MCP_SERVER_TLS_KEY_FILE` | Path to TLS private key file for HTTPS server |

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

## Development

### Start the Server

```bash
docker-compose up -d --build grafana-mcp-server
```

### Stop the Server

```bash
docker-compose down
```

### Restart the Server

```bash
docker-compose restart grafana-mcp-server
```

### View Logs

```bash
docker-compose logs -f grafana-mcp-server
```

## License

This project is licensed under the [Apache License, Version 2.0](LICENSE).
