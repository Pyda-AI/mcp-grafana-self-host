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

const (
	// DefaultTempoTraceLimit is the default number of traces to return
	DefaultTempoTraceLimit = 20

	// MaxTempoTraceLimit is the maximum number of traces that can be requested
	MaxTempoTraceLimit = 100
)

// tempoClient represents a client for interacting with Tempo via Grafana's datasource proxy
type tempoClient struct {
	httpClient *http.Client
	baseURL    string
}

func newTempoClient(ctx context.Context, uid string) (*tempoClient, error) {
	// First check if the datasource exists
	ds, err := getDatasourceByUID(ctx, GetDatasourceByUIDParams{UID: uid})
	if err != nil {
		return nil, err
	}

	// Verify it's a Tempo datasource
	if !strings.Contains(strings.ToLower(ds.Type), "tempo") {
		return nil, fmt.Errorf("datasource %s (type: %s) is not a Tempo datasource", uid, ds.Type)
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

	return &tempoClient{
		httpClient: client,
		baseURL:    baseURL,
	}, nil
}

// makeRequest is a helper method to make HTTP requests to the Tempo API
func (c *tempoClient) makeRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
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
		return nil, fmt.Errorf("tempo API returned status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body with a limit
	body := io.LimitReader(resp.Body, 1024*1024*48) // 48MB limit
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	return bodyBytes, nil
}

// TraceSearchResult represents a single trace in search results
type TraceSearchResult struct {
	TraceID           string            `json:"traceID"`
	RootServiceName   string            `json:"rootServiceName"`
	RootTraceName     string            `json:"rootTraceName"`
	StartTimeUnixNano string            `json:"startTimeUnixNano"`
	DurationMs        int               `json:"durationMs"`
	SpanSets          []SpanSet         `json:"spanSets,omitempty"`
	ServiceStats      map[string]int    `json:"serviceStats,omitempty"`
}

// SpanSet represents a set of spans matching search criteria
type SpanSet struct {
	Spans   []SpanInfo `json:"spans"`
	Matched int        `json:"matched"`
}

// SpanInfo represents basic span information
type SpanInfo struct {
	SpanID            string            `json:"spanID"`
	StartTimeUnixNano string            `json:"startTimeUnixNano"`
	DurationNanos     string            `json:"durationNanos"`
	Attributes        []SpanAttribute   `json:"attributes,omitempty"`
}

// SpanAttribute represents a span attribute
type SpanAttribute struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// SearchTracesParams defines the parameters for searching traces
type SearchTracesParams struct {
	DatasourceUID string   `json:"datasourceUid" jsonschema:"required,description=The UID of the Tempo datasource to query"`
	ServiceName   string   `json:"serviceName,omitempty" jsonschema:"description=Filter by service name"`
	SpanName      string   `json:"spanName,omitempty" jsonschema:"description=Filter by span/operation name"`
	Tags          string   `json:"tags,omitempty" jsonschema:"description=Filter by tags in key=value format\\, comma separated (e.g. 'http.status_code=200\\,http.method=GET')"`
	MinDuration   string   `json:"minDuration,omitempty" jsonschema:"description=Minimum trace duration (e.g. '100ms'\\, '1s')"`
	MaxDuration   string   `json:"maxDuration,omitempty" jsonschema:"description=Maximum trace duration (e.g. '5s'\\, '10s')"`
	Limit         int      `json:"limit,omitempty" jsonschema:"default=20,description=Maximum number of traces to return (max 100)"`
	StartRFC3339  string   `json:"startRfc3339,omitempty" jsonschema:"description=Start time in RFC3339 format (defaults to 1 hour ago)"`
	EndRFC3339    string   `json:"endRfc3339,omitempty" jsonschema:"description=End time in RFC3339 format (defaults to now)"`
}

func searchTraces(ctx context.Context, args SearchTracesParams) ([]TraceSearchResult, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	// Build query parameters
	params := url.Values{}
	
	// Build TraceQL query if service or span name provided
	var queryParts []string
	if args.ServiceName != "" {
		queryParts = append(queryParts, fmt.Sprintf(`resource.service.name="%s"`, args.ServiceName))
	}
	if args.SpanName != "" {
		queryParts = append(queryParts, fmt.Sprintf(`name="%s"`, args.SpanName))
	}
	if args.Tags != "" {
		// Parse and add tags
		for _, tag := range strings.Split(args.Tags, ",") {
			parts := strings.SplitN(strings.TrimSpace(tag), "=", 2)
			if len(parts) == 2 {
				queryParts = append(queryParts, fmt.Sprintf(`%s="%s"`, parts[0], parts[1]))
			}
		}
	}
	
	if len(queryParts) > 0 {
		query := "{" + strings.Join(queryParts, " && ") + "}"
		params.Set("q", query)
	}

	if args.MinDuration != "" {
		params.Set("minDuration", args.MinDuration)
	}
	if args.MaxDuration != "" {
		params.Set("maxDuration", args.MaxDuration)
	}

	// Set limit
	limit := args.Limit
	if limit <= 0 {
		limit = DefaultTempoTraceLimit
	}
	if limit > MaxTempoTraceLimit {
		limit = MaxTempoTraceLimit
	}
	params.Set("limit", fmt.Sprintf("%d", limit))

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

	params.Set("start", fmt.Sprintf("%d", startTime.Unix()))
	params.Set("end", fmt.Sprintf("%d", endTime.Unix()))

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/search", params)
	if err != nil {
		return nil, fmt.Errorf("searching traces: %w", err)
	}

	var response struct {
		Traces []TraceSearchResult `json:"traces"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if response.Traces == nil {
		return []TraceSearchResult{}, nil
	}

	return response.Traces, nil
}

var SearchTraces = mcpgrafana.MustTool(
	"search_tempo_traces",
	"Search for traces in a Tempo datasource. Supports filtering by service name, span/operation name, tags, and duration. Returns a list of matching traces with their IDs, root service, duration, and timing information. Default time range is the last hour.",
	searchTraces,
	mcp.WithTitleAnnotation("Search Tempo traces"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// GetTraceParams defines the parameters for getting a specific trace
type GetTraceParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Tempo datasource to query"`
	TraceID       string `json:"traceId" jsonschema:"required,description=The trace ID to retrieve"`
}

// TraceResponse represents the response from getting a trace
type TraceResponse struct {
	Batches []ResourceSpans `json:"batches"`
}

// ResourceSpans represents a batch of spans from a resource
type ResourceSpans struct {
	Resource   Resource     `json:"resource"`
	ScopeSpans []ScopeSpans `json:"scopeSpans"`
}

// Resource represents a resource (service) that produced spans
type Resource struct {
	Attributes []SpanAttribute `json:"attributes"`
}

// ScopeSpans represents spans within a scope
type ScopeSpans struct {
	Scope Scope  `json:"scope"`
	Spans []Span `json:"spans"`
}

// Scope represents the instrumentation scope
type Scope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Span represents a single span in a trace
type Span struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId,omitempty"`
	Name              string          `json:"name"`
	Kind              int             `json:"kind"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	EndTimeUnixNano   string          `json:"endTimeUnixNano"`
	Attributes        []SpanAttribute `json:"attributes,omitempty"`
	Status            SpanStatus      `json:"status,omitempty"`
	Events            []SpanEvent     `json:"events,omitempty"`
}

// SpanStatus represents the status of a span
type SpanStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// SpanEvent represents an event that occurred during a span
type SpanEvent struct {
	TimeUnixNano string          `json:"timeUnixNano"`
	Name         string          `json:"name"`
	Attributes   []SpanAttribute `json:"attributes,omitempty"`
}

func getTrace(ctx context.Context, args GetTraceParams) (*TraceResponse, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", fmt.Sprintf("/api/traces/%s", args.TraceID), nil)
	if err != nil {
		return nil, fmt.Errorf("getting trace %s: %w", args.TraceID, err)
	}

	var response TraceResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return &response, nil
}

var GetTrace = mcpgrafana.MustTool(
	"get_tempo_trace",
	"Retrieve a specific trace by its trace ID from a Tempo datasource. Returns the full trace data including all spans, their attributes, timing, and relationships.",
	getTrace,
	mcp.WithTitleAnnotation("Get Tempo trace"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ListTempoTagNamesParams defines the parameters for listing tag names
type ListTempoTagNamesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Tempo datasource to query"`
	Scope         string `json:"scope,omitempty" jsonschema:"description=Scope to filter tags: 'resource'\\, 'span'\\, or 'intrinsic'. If not provided\\, returns all tags."`
}

func listTempoTagNames(ctx context.Context, args ListTempoTagNamesParams) ([]string, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	params := url.Values{}
	if args.Scope != "" {
		params.Set("scope", args.Scope)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/v2/search/tags", params)
	if err != nil {
		return nil, fmt.Errorf("listing tag names: %w", err)
	}

	var response struct {
		Scopes []struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		} `json:"scopes"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		// Try the v1 API format
		var v1Response struct {
			TagNames []string `json:"tagNames"`
		}
		if err := json.Unmarshal(bodyBytes, &v1Response); err != nil {
			return nil, fmt.Errorf("unmarshalling response: %w", err)
		}
		return v1Response.TagNames, nil
	}

	// Flatten all tags from all scopes
	var allTags []string
	for _, scope := range response.Scopes {
		allTags = append(allTags, scope.Tags...)
	}

	return allTags, nil
}

var ListTempoTagNames = mcpgrafana.MustTool(
	"list_tempo_tag_names",
	"List all available tag names (keys) in a Tempo datasource. Optionally filter by scope ('resource', 'span', or 'intrinsic'). Useful for discovering available attributes to filter traces by.",
	listTempoTagNames,
	mcp.WithTitleAnnotation("List Tempo tag names"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ListTempoTagValuesParams defines the parameters for listing tag values
type ListTempoTagValuesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Tempo datasource to query"`
	TagName       string `json:"tagName" jsonschema:"required,description=The tag name to get values for"`
}

func listTempoTagValues(ctx context.Context, args ListTempoTagValuesParams) ([]string, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", fmt.Sprintf("/api/v2/search/tag/%s/values", url.PathEscape(args.TagName)), nil)
	if err != nil {
		return nil, fmt.Errorf("listing tag values for %s: %w", args.TagName, err)
	}

	var response struct {
		TagValues []string `json:"tagValues"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if response.TagValues == nil {
		return []string{}, nil
	}

	return response.TagValues, nil
}

var ListTempoTagValues = mcpgrafana.MustTool(
	"list_tempo_tag_values",
	"List all values for a specific tag name in a Tempo datasource. Useful for discovering what values are available to filter traces by.",
	listTempoTagValues,
	mcp.WithTitleAnnotation("List Tempo tag values"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// QueryTraceQLParams defines the parameters for running a TraceQL query
type QueryTraceQLParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Tempo datasource to query"`
	Query         string `json:"query" jsonschema:"required,description=The TraceQL query to execute (e.g. '{resource.service.name=\"frontend\" && status=error}')"`
	Limit         int    `json:"limit,omitempty" jsonschema:"default=20,description=Maximum number of traces to return (max 100)"`
	StartRFC3339  string `json:"startRfc3339,omitempty" jsonschema:"description=Start time in RFC3339 format (defaults to 1 hour ago)"`
	EndRFC3339    string `json:"endRfc3339,omitempty" jsonschema:"description=End time in RFC3339 format (defaults to now)"`
}

func queryTraceQL(ctx context.Context, args QueryTraceQLParams) ([]TraceSearchResult, error) {
	client, err := newTempoClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating Tempo client: %w", err)
	}

	params := url.Values{}
	params.Set("q", args.Query)

	// Set limit
	limit := args.Limit
	if limit <= 0 {
		limit = DefaultTempoTraceLimit
	}
	if limit > MaxTempoTraceLimit {
		limit = MaxTempoTraceLimit
	}
	params.Set("limit", fmt.Sprintf("%d", limit))

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

	params.Set("start", fmt.Sprintf("%d", startTime.Unix()))
	params.Set("end", fmt.Sprintf("%d", endTime.Unix()))

	bodyBytes, err := client.makeRequest(ctx, "GET", "/api/search", params)
	if err != nil {
		return nil, fmt.Errorf("executing TraceQL query: %w", err)
	}

	var response struct {
		Traces []TraceSearchResult `json:"traces"`
	}
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if response.Traces == nil {
		return []TraceSearchResult{}, nil
	}

	return response.Traces, nil
}

var QueryTraceQL = mcpgrafana.MustTool(
	"query_tempo_traceql",
	"Execute a TraceQL query against a Tempo datasource. TraceQL is Tempo's query language for searching traces. Example queries: '{resource.service.name=\"frontend\"}', '{status=error}', '{duration>1s}'. Returns matching traces.",
	queryTraceQL,
	mcp.WithTitleAnnotation("Query Tempo with TraceQL"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// AddTempoTools registers all Tempo tools with the MCP server
func AddTempoTools(mcp *server.MCPServer) {
	SearchTraces.Register(mcp)
	GetTrace.Register(mcp)
	ListTempoTagNames.Register(mcp)
	ListTempoTagValues.Register(mcp)
	QueryTraceQL.Register(mcp)
}

