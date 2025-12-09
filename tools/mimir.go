package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	mcpgrafana "github.com/grafana/mcp-grafana"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// mimirClient represents a client for interacting with Mimir via Grafana's datasource proxy
type mimirClient struct {
	httpClient *http.Client
	baseURL    string
}

func newMimirClient(ctx context.Context, uid string) (*mimirClient, error) {
	// First check if the datasource exists
	ds, err := getDatasourceByUID(ctx, GetDatasourceByUIDParams{UID: uid})
	if err != nil {
		return nil, err
	}

	// Verify it's a Mimir/Prometheus datasource
	dsType := strings.ToLower(ds.Type)
	if !strings.Contains(dsType, "mimir") && !strings.Contains(dsType, "prometheus") {
		return nil, fmt.Errorf("datasource %s (type: %s) is not a Mimir or Prometheus datasource", uid, ds.Type)
	}

	cfg := mcpgrafana.GrafanaConfigFromContext(ctx)
	baseURL := fmt.Sprintf("%s/api/datasources/proxy/uid/%s", strings.TrimRight(cfg.URL, "/"), uid)

	// Create custom transport with TLS configuration if available
	var transport = http.DefaultTransport
	if tlsConfig := cfg.TLSConfig; tlsConfig != nil {
		var err error
		transport, err = tlsConfig.HTTPTransport(transport.(*http.Transport))
		if err != nil {
			return nil, fmt.Errorf("failed to create custom transport: %w", err)
		}
	}

	transport = NewAuthRoundTripper(transport, cfg.AccessToken, cfg.IDToken, cfg.APIKey, cfg.BasicAuth)
	transport = mcpgrafana.NewOrgIDRoundTripper(transport, cfg.OrgID)

	client := &http.Client{
		Transport: mcpgrafana.NewUserAgentTransport(transport),
	}

	return &mimirClient{
		httpClient: client,
		baseURL:    baseURL,
	}, nil
}

// makeRequest is a helper method to make HTTP requests to the Mimir API
func (c *mimirClient) makeRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	fullURL := c.baseURL + path
	if params != nil && len(params) > 0 {
		fullURL = fullURL + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mimir API returned status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body with a limit
	body := io.LimitReader(resp.Body, 1024*1024*48) // 48MB limit
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return bodyBytes, nil
}

// GetMimirBuildInfoParams defines the parameters for getting build info
type GetMimirBuildInfoParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Mimir datasource to query"`
}

// MimirBuildInfo represents Mimir build information
type MimirBuildInfo struct {
	Status string `json:"status"`
	Data   struct {
		Version   string `json:"version"`
		Revision  string `json:"revision"`
		Branch    string `json:"branch"`
		BuildUser string `json:"buildUser"`
		BuildDate string `json:"buildDate"`
		GoVersion string `json:"goVersion"`
	} `json:"data"`
}

func getMimirBuildInfo(ctx context.Context, args GetMimirBuildInfoParams) (*MimirBuildInfo, error) {
	client, err := newMimirClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/v1/status/buildinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("getting build info: %w", err)
	}

	var response MimirBuildInfo
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return &response, nil
}

var GetMimirBuildInfo = mcpgrafana.MustTool(
	"get_mimir_build_info",
	"Get build information from a Mimir datasource including version, revision, and build date. Useful for verifying connectivity and checking the Mimir version.",
	getMimirBuildInfo,
	mcp.WithTitleAnnotation("Get Mimir build info"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// GetMimirRuntimeConfigParams defines the parameters for getting runtime config
type GetMimirRuntimeConfigParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Mimir datasource to query"`
}

func getMimirRuntimeConfig(ctx context.Context, args GetMimirRuntimeConfigParams) (map[string]interface{}, error) {
	client, err := newMimirClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/v1/status/runtimeinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("getting runtime config: %w", err)
	}

	var response struct {
		Status string                 `json:"status"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return response.Data, nil
}

var GetMimirRuntimeConfig = mcpgrafana.MustTool(
	"get_mimir_runtime_config",
	"Get runtime configuration information from a Mimir datasource. Returns information about the current runtime settings and configuration.",
	getMimirRuntimeConfig,
	mcp.WithTitleAnnotation("Get Mimir runtime config"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// QueryMimirSeriesParams defines the parameters for querying series
type QueryMimirSeriesParams struct {
	DatasourceUID string   `json:"datasourceUid" jsonschema:"required,description=The UID of the Mimir datasource to query"`
	Match         []string `json:"match" jsonschema:"required,description=Series selector arguments (e.g. ['up'\\, '{job=\"prometheus\"}'])"`
	StartRFC3339  string   `json:"startRfc3339,omitempty" jsonschema:"description=Start time in RFC3339 format (defaults to 1 hour ago)"`
	EndRFC3339    string   `json:"endRfc3339,omitempty" jsonschema:"description=End time in RFC3339 format (defaults to now)"`
	Limit         int      `json:"limit,omitempty" jsonschema:"default=100,description=Maximum number of series to return"`
}

// SeriesResult represents a series result
type SeriesResult struct {
	Labels map[string]string `json:"labels"`
}

func queryMimirSeries(ctx context.Context, args QueryMimirSeriesParams) ([]map[string]string, error) {
	client, err := newMimirClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	params := url.Values{}
	for _, m := range args.Match {
		params.Add("match[]", m)
	}

	// Set time range
	var startTime, endTime time.Time
	if args.StartRFC3339 != "" {
		startTime, err = time.Parse(time.RFC3339, args.StartRFC3339)
		if err != nil {
			return nil, fmt.Errorf("parsing start time: %w", err)
		}
	} else {
		startTime = time.Now().Add(-1 * time.Hour)
	}
	
	if args.EndRFC3339 != "" {
		endTime, err = time.Parse(time.RFC3339, args.EndRFC3339)
		if err != nil {
			return nil, fmt.Errorf("parsing end time: %w", err)
		}
	} else {
		endTime = time.Now()
	}

	params.Set("start", startTime.Format(time.RFC3339))
	params.Set("end", endTime.Format(time.RFC3339))

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/v1/series", params)
	if err != nil {
		return nil, fmt.Errorf("querying series: %w", err)
	}

	var response struct {
		Status string              `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Apply limit
	limit := args.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(response.Data) > limit {
		response.Data = response.Data[:limit]
	}

	return response.Data, nil
}

var QueryMimirSeries = mcpgrafana.MustTool(
	"query_mimir_series",
	"Query series metadata from a Mimir datasource. Returns the list of time series that match a given selector. Useful for discovering what metrics and label combinations exist.",
	queryMimirSeries,
	mcp.WithTitleAnnotation("Query Mimir series"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ListMimirRulesParams defines the parameters for listing rules
type ListMimirRulesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Mimir datasource to query"`
	RuleType      string `json:"ruleType,omitempty" jsonschema:"description=Filter by rule type: 'alert' or 'record'. If not provided\\, returns all rules."`
}

// RuleGroup represents a group of rules
type RuleGroup struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Rules    []Rule `json:"rules"`
	Interval int    `json:"interval"`
}

// Rule represents a recording or alerting rule
type Rule struct {
	Name        string            `json:"name"`
	Query       string            `json:"query"`
	Duration    float64           `json:"duration,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Health      string            `json:"health"`
	State       string            `json:"state,omitempty"`
	Type        string            `json:"type"`
}

func listMimirRules(ctx context.Context, args ListMimirRulesParams) ([]RuleGroup, error) {
	client, err := newMimirClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	params := url.Values{}
	if args.RuleType != "" {
		params.Set("type", args.RuleType)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/v1/rules", params)
	if err != nil {
		return nil, fmt.Errorf("listing rules: %w", err)
	}

	var response struct {
		Status string `json:"status"`
		Data   struct {
			Groups []RuleGroup `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return response.Data.Groups, nil
}

var ListMimirRules = mcpgrafana.MustTool(
	"list_mimir_rules",
	"List recording and alerting rules from a Mimir datasource. Can filter by rule type ('alert' or 'record'). Returns rule groups with their rules, queries, and status.",
	listMimirRules,
	mcp.WithTitleAnnotation("List Mimir rules"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ListMimirAlertsParams defines the parameters for listing alerts
type ListMimirAlertsParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Mimir datasource to query"`
}

// Alert represents an active alert
type Alert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    string            `json:"activeAt,omitempty"`
	Value       string            `json:"value,omitempty"`
}

func listMimirAlerts(ctx context.Context, args ListMimirAlertsParams) ([]Alert, error) {
	client, err := newMimirClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/v1/alerts", nil)
	if err != nil {
		return nil, fmt.Errorf("listing alerts: %w", err)
	}

	var response struct {
		Status string `json:"status"`
		Data   struct {
			Alerts []Alert `json:"alerts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if response.Data.Alerts == nil {
		return []Alert{}, nil
	}

	return response.Data.Alerts, nil
}

var ListMimirAlerts = mcpgrafana.MustTool(
	"list_mimir_alerts",
	"List active alerts from a Mimir datasource. Returns all currently firing and pending alerts with their labels, annotations, state, and values.",
	listMimirAlerts,
	mcp.WithTitleAnnotation("List Mimir alerts"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// GetMimirTargetsParams defines the parameters for getting targets
type GetMimirTargetsParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Mimir datasource to query"`
	State         string `json:"state,omitempty" jsonschema:"description=Filter by target state: 'active'\\, 'dropped'\\, or 'any' (default)"`
}

// Target represents a scrape target
type Target struct {
	DiscoveredLabels map[string]string `json:"discoveredLabels"`
	Labels           map[string]string `json:"labels"`
	ScrapePool       string            `json:"scrapePool"`
	ScrapeURL        string            `json:"scrapeUrl"`
	GlobalURL        string            `json:"globalUrl,omitempty"`
	LastError        string            `json:"lastError"`
	LastScrape       string            `json:"lastScrape"`
	LastScrapeDuration float64         `json:"lastScrapeDuration"`
	Health           string            `json:"health"`
}

// TargetsResult represents the targets response
type TargetsResult struct {
	ActiveTargets  []Target `json:"activeTargets"`
	DroppedTargets []Target `json:"droppedTargets"`
}

func getMimirTargets(ctx context.Context, args GetMimirTargetsParams) (*TargetsResult, error) {
	client, err := newMimirClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	params := url.Values{}
	if args.State != "" {
		params.Set("state", args.State)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/v1/targets", params)
	if err != nil {
		return nil, fmt.Errorf("getting targets: %w", err)
	}

	var response struct {
		Status string        `json:"status"`
		Data   TargetsResult `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return &response.Data, nil
}

var GetMimirTargets = mcpgrafana.MustTool(
	"get_mimir_targets",
	"Get scrape targets from a Mimir datasource. Returns active and dropped targets with their labels, scrape URLs, health status, and last scrape information. Can filter by state.",
	getMimirTargets,
	mcp.WithTitleAnnotation("Get Mimir targets"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// AddMimirTools registers all Mimir tools with the MCP server
func AddMimirTools(mcp *server.MCPServer) {
	GetMimirBuildInfo.Register(mcp)
	GetMimirRuntimeConfig.Register(mcp)
	QueryMimirSeries.Register(mcp)
	ListMimirRules.Register(mcp)
	ListMimirAlerts.Register(mcp)
	GetMimirTargets.Register(mcp)
}

