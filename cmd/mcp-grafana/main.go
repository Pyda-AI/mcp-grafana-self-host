package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcpgrafana "github.com/grafana/mcp-grafana"
	"github.com/grafana/mcp-grafana/tools"
)

// loadEnvFile loads environment variables from a .env file if it exists.
// Environment variables already set take precedence (won't be overwritten).
func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env file doesn't exist, that's fine
		}
		return fmt.Errorf("failed to open .env file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		
		// Remove surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		
		// Only set if not already set (env vars take precedence)
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	
	return scanner.Err()
}

func maybeAddTools(s *server.MCPServer, tf func(*server.MCPServer), enabledTools []string, disable bool, category string) {
	if !slices.Contains(enabledTools, category) {
		slog.Debug("Not enabling tools", "category", category)
		return
	}
	if disable {
		slog.Info("Disabling tools", "category", category)
		return
	}
	slog.Debug("Enabling tools", "category", category)
	tf(s)
}

// disabledTools indicates whether each category of tools should be disabled.
type disabledTools struct {
	enabledTools string

	search, datasource, incident,
	prometheus, loki, alerting,
	dashboard, folder, oncall, asserts, sift, admin,
	pyroscope, navigation, proxied, annotations, tempo, mimir bool
}

// Configuration for the Grafana client.
type grafanaConfig struct {
	// Whether to enable debug mode for the Grafana transport.
	debug bool

	// Organization ID for multi-org support
	orgID string

	// TLS configuration
	tlsCertFile   string
	tlsKeyFile    string
	tlsCAFile     string
	tlsSkipVerify bool
}


func (dt *disabledTools) addFlags() {
	flag.StringVar(&dt.enabledTools, "enabled-tools", "search,datasource,incident,prometheus,loki,alerting,dashboard,folder,oncall,asserts,sift,admin,pyroscope,navigation,proxied,annotations,tempo,mimir", "A comma separated list of tools enabled for this server. Can be overwritten entirely or by disabling specific components, e.g. --disable-search.")
	flag.BoolVar(&dt.search, "disable-search", false, "Disable search tools")
	flag.BoolVar(&dt.datasource, "disable-datasource", false, "Disable datasource tools")
	flag.BoolVar(&dt.incident, "disable-incident", false, "Disable incident tools")
	flag.BoolVar(&dt.prometheus, "disable-prometheus", false, "Disable prometheus tools")
	flag.BoolVar(&dt.loki, "disable-loki", false, "Disable loki tools")
	flag.BoolVar(&dt.alerting, "disable-alerting", false, "Disable alerting tools")
	flag.BoolVar(&dt.dashboard, "disable-dashboard", false, "Disable dashboard tools")
	flag.BoolVar(&dt.folder, "disable-folder", false, "Disable folder tools")
	flag.BoolVar(&dt.oncall, "disable-oncall", false, "Disable oncall tools")
	flag.BoolVar(&dt.asserts, "disable-asserts", false, "Disable asserts tools")
	flag.BoolVar(&dt.sift, "disable-sift", false, "Disable sift tools")
	flag.BoolVar(&dt.admin, "disable-admin", false, "Disable admin tools")
	flag.BoolVar(&dt.pyroscope, "disable-pyroscope", false, "Disable pyroscope tools")
	flag.BoolVar(&dt.navigation, "disable-navigation", false, "Disable navigation tools")
	flag.BoolVar(&dt.proxied, "disable-proxied", false, "Disable proxied tools (tools from external MCP servers)")
	flag.BoolVar(&dt.annotations, "disable-annotations", false, "Disable annotation tools")
	flag.BoolVar(&dt.tempo, "disable-tempo", false, "Disable tempo tools")
	flag.BoolVar(&dt.mimir, "disable-mimir", false, "Disable mimir tools")
}

func (gc *grafanaConfig) addFlags() {
	flag.BoolVar(&gc.debug, "debug", false, "Enable debug mode for the Grafana transport")
	flag.StringVar(&gc.orgID, "org-id", "", "Grafana organization ID for multi-org support")

	// TLS configuration flags
	flag.StringVar(&gc.tlsCertFile, "tls-cert-file", "", "Path to TLS certificate file for client authentication")
	flag.StringVar(&gc.tlsKeyFile, "tls-key-file", "", "Path to TLS private key file for client authentication")
	flag.StringVar(&gc.tlsCAFile, "tls-ca-file", "", "Path to TLS CA certificate file for server verification")
	flag.BoolVar(&gc.tlsSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification (insecure)")
}

func (dt *disabledTools) addTools(s *server.MCPServer) {
	enabledTools := strings.Split(dt.enabledTools, ",")
	maybeAddTools(s, tools.AddSearchTools, enabledTools, dt.search, "search")
	maybeAddTools(s, tools.AddDatasourceTools, enabledTools, dt.datasource, "datasource")
	maybeAddTools(s, tools.AddIncidentTools, enabledTools, dt.incident, "incident")
	maybeAddTools(s, tools.AddPrometheusTools, enabledTools, dt.prometheus, "prometheus")
	maybeAddTools(s, tools.AddLokiTools, enabledTools, dt.loki, "loki")
	maybeAddTools(s, tools.AddAlertingTools, enabledTools, dt.alerting, "alerting")
	maybeAddTools(s, tools.AddDashboardTools, enabledTools, dt.dashboard, "dashboard")
	maybeAddTools(s, tools.AddFolderTools, enabledTools, dt.folder, "folder")
	maybeAddTools(s, tools.AddOnCallTools, enabledTools, dt.oncall, "oncall")
	maybeAddTools(s, tools.AddAssertsTools, enabledTools, dt.asserts, "asserts")
	maybeAddTools(s, tools.AddSiftTools, enabledTools, dt.sift, "sift")
	maybeAddTools(s, tools.AddAdminTools, enabledTools, dt.admin, "admin")
	maybeAddTools(s, tools.AddPyroscopeTools, enabledTools, dt.pyroscope, "pyroscope")
	maybeAddTools(s, tools.AddNavigationTools, enabledTools, dt.navigation, "navigation")
	maybeAddTools(s, tools.AddAnnotationTools, enabledTools, dt.annotations, "annotations")
	maybeAddTools(s, tools.AddTempoTools, enabledTools, dt.tempo, "tempo")
	maybeAddTools(s, tools.AddMimirTools, enabledTools, dt.mimir, "mimir")
}

func newServer(dt disabledTools) (*server.MCPServer, *mcpgrafana.ToolManager) {
	sm := mcpgrafana.NewSessionManager()

	// Declare variable for ToolManager that will be initialized after server creation
	var stm *mcpgrafana.ToolManager

	// Create hooks
	hooks := &server.Hooks{
		OnRegisterSession:   []server.OnRegisterSessionHookFunc{sm.CreateSession},
		OnUnregisterSession: []server.OnUnregisterSessionHookFunc{sm.RemoveSession},
	}

	// Add proxied tools hooks if enabled (always using streamable-http)
	if !dt.proxied {
		// OnBeforeListTools: Discover, connect, and register tools
		hooks.OnBeforeListTools = []server.OnBeforeListToolsFunc{
			func(ctx context.Context, id any, request *mcp.ListToolsRequest) {
				if stm != nil {
					if session := server.ClientSessionFromContext(ctx); session != nil {
						stm.InitializeAndRegisterProxiedTools(ctx, session)
					}
				}
			},
		}

		// OnBeforeCallTool: Fallback in case client calls tool without listing first
		hooks.OnBeforeCallTool = []server.OnBeforeCallToolFunc{
			func(ctx context.Context, id any, request *mcp.CallToolRequest) {
				if stm != nil {
					if session := server.ClientSessionFromContext(ctx); session != nil {
						stm.InitializeAndRegisterProxiedTools(ctx, session)
					}
				}
			},
		}
	}
	s := server.NewMCPServer("mcp-grafana", mcpgrafana.Version(),
		server.WithInstructions(`
This server provides read-only access to your Grafana instance and the surrounding ecosystem.

Available Capabilities:
- Dashboards: Search and retrieve dashboards. Extract panel queries and datasource information.
- Datasources: List and fetch details for datasources.
- Prometheus & Mimir: Run PromQL queries, retrieve metric metadata, and explore label names/values.
- Loki: Run LogQL queries, retrieve log metadata, and explore label names/values.
- Tempo: Search traces, retrieve trace details, and explore trace tags and metadata.
- Incidents: Search and view incidents in Grafana Incident.
- Sift Investigations: View existing Sift investigations and analyses.
- Alerting: List and fetch alert rules and notification contact points.
- OnCall: View on-call schedules, shifts, teams, and users.
- Admin: List teams and users.
- Pyroscope: Fetch profiling data and explore profile types.
- Navigation: Generate deeplink URLs for Grafana resources like dashboards, panels, and Explore queries.
- Proxied Tools: Access tools from external MCP servers through dynamic discovery.

Note: This server operates in read-only mode. Write operations are not available.
Some capabilities may be disabled. Do not try to use features that are not available via tools.
`),
		server.WithHooks(hooks),
	)

	// Initialize ToolManager now that server is created
	stm = mcpgrafana.NewToolManager(sm, s, mcpgrafana.WithProxiedTools(!dt.proxied))

	dt.addTools(s)
	return s, stm
}

type tlsConfig struct {
	certFile, keyFile string
}

func (tc *tlsConfig) addFlags() {
	flag.StringVar(&tc.certFile, "server.tls-cert-file", "", "Path to TLS certificate file for server HTTPS (required for TLS)")
	flag.StringVar(&tc.keyFile, "server.tls-key-file", "", "Path to TLS private key file for server HTTPS (required for TLS)")
}

// httpServer represents a server with Start and Shutdown methods
type httpServer interface {
	Start(addr string) error
	Shutdown(ctx context.Context) error
}

// runHTTPServer handles the common logic for running HTTP-based servers
func runHTTPServer(ctx context.Context, srv httpServer, addr, transportName string) error {
	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(addr); err != nil {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for either server error or shutdown signal
	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Info(fmt.Sprintf("%s server shutting down...", transportName))

		// Create a timeout context for shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %v", err)
		}
		slog.Debug("Shutdown called, waiting for connections to close...")

		// Wait for server to finish
		select {
		case err := <-serverErr:
			// http.ErrServerClosed is expected when shutting down
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("server error during shutdown: %v", err)
			}
		case <-shutdownCtx.Done():
			slog.Warn(fmt.Sprintf("%s server did not stop gracefully within timeout", transportName))
		}
	}

	return nil
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func handleTest(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func run(addr string, logLevel slog.Level, dt disabledTools, gc mcpgrafana.GrafanaConfig, tls tlsConfig, auth mcpgrafana.AuthConfig) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
	s, _ := newServer(dt)

	// Create a context that will be cancelled on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Handle shutdown signals
	go func() {
		<-sigChan
		slog.Info("Received shutdown signal")
		cancel()
	}()

	// Start streamable-http server with hardcoded /mcp endpoint
	endpointPath := "/mcp"
	httpSrv := &http.Server{Addr: addr}
	opts := []server.StreamableHTTPOption{
		server.WithHTTPContextFunc(mcpgrafana.ComposedHTTPContextFunc(gc)),
		server.WithStateLess(dt.proxied), // Stateful when proxied tools enabled (requires sessions)
		server.WithEndpointPath(endpointPath),
		server.WithStreamableHTTPServer(httpSrv),
	}
	if tls.certFile != "" || tls.keyFile != "" {
		opts = append(opts, server.WithTLSCert(tls.certFile, tls.keyFile))
	}
	srv := server.NewStreamableHTTPServer(s, opts...)
	mux := http.NewServeMux()
	mux.Handle(endpointPath, srv)
	mux.HandleFunc("/healthz", handleHealthz)
	mux.HandleFunc("/test", handleTest)

	// Apply Bearer token authentication middleware if configured
	if auth.Token != "" {
		httpSrv.Handler = mcpgrafana.NewBearerAuthMiddleware(auth)(mux)
	} else {
		httpSrv.Handler = mux
	}

	slog.Info("Starting Grafana MCP server using StreamableHTTP transport",
		"version", mcpgrafana.Version(), "address", addr, "endpointPath", endpointPath, "auth_enabled", auth.Token != "")
	return runHTTPServer(ctx, srv, addr, "StreamableHTTP")
}

func main() {
	// Load .env file first (environment variables take precedence over .env)
	if err := loadEnvFile(".env"); err != nil {
		slog.Warn("Failed to load .env file", "error", err)
	}

	// Define flags (can override env vars)
	serverPort := flag.String("port", "", "The port to start the server on")
	logLevel := flag.String("log-level", "", "Log level (debug, info, warn, error)")
	showVersion := flag.Bool("version", false, "Print the version and exit")
	var dt disabledTools
	dt.addFlags()
	var gc grafanaConfig
	gc.addFlags()
	var tls tlsConfig
	tls.addFlags()
	flag.Parse()

	if *showVersion {
		fmt.Println(mcpgrafana.Version())
		os.Exit(0)
	}

	// Helper function to get env var with default
	getEnv := func(key, defaultValue string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return defaultValue
	}

	// Helper function to get bool env var
	getEnvBool := func(key string) bool {
		v := os.Getenv(key)
		return v == "true" || v == "1" || v == "yes"
	}

	// Resolve configuration: CLI flag > env var > default
	// Server port
	resolvedPort := *serverPort
	if resolvedPort == "" {
		resolvedPort = getEnv("MCP_SERVER_PORT", "8443")
	}
	resolvedAddr := "0.0.0.0:" + resolvedPort

	// Log level
	resolvedLogLevel := *logLevel
	if resolvedLogLevel == "" {
		resolvedLogLevel = getEnv("MCP_LOG_LEVEL", "info")
	}

	// Get auth token from env var (required for HTTP transports)
	authToken := os.Getenv(mcpgrafana.AuthTokenEnvVar)

	// Build GrafanaConfig
	grafanaCfg := mcpgrafana.GrafanaConfig{Debug: gc.debug}

	// Organization ID from flag or env var
	orgIDStr := gc.orgID
	if orgIDStr == "" {
		orgIDStr = os.Getenv("GRAFANA_ORG_ID")
	}
	if orgIDStr != "" {
		orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
		if err != nil {
			slog.Warn("Invalid organization ID, ignoring", "value", orgIDStr, "error", err)
		} else {
			grafanaCfg.OrgID = orgID
		}
	}

	// TLS config from flags or env vars
	tlsCertFile := gc.tlsCertFile
	if tlsCertFile == "" {
		tlsCertFile = getEnv("MCP_TLS_CERT_FILE", "")
	}
	tlsKeyFile := gc.tlsKeyFile
	if tlsKeyFile == "" {
		tlsKeyFile = getEnv("MCP_TLS_KEY_FILE", "")
	}
	tlsCAFile := gc.tlsCAFile
	if tlsCAFile == "" {
		tlsCAFile = getEnv("MCP_TLS_CA_FILE", "")
	}
	tlsSkipVerify := gc.tlsSkipVerify
	if !tlsSkipVerify {
		tlsSkipVerify = getEnvBool("MCP_TLS_SKIP_VERIFY")
	}

	if tlsCertFile != "" || tlsKeyFile != "" || tlsCAFile != "" || tlsSkipVerify {
		grafanaCfg.TLSConfig = &mcpgrafana.TLSConfig{
			CertFile:   tlsCertFile,
			KeyFile:    tlsKeyFile,
			CAFile:     tlsCAFile,
			SkipVerify: tlsSkipVerify,
		}
	}

	// Server TLS config from flags or env vars
	serverTLSCertFile := tls.certFile
	if serverTLSCertFile == "" {
		serverTLSCertFile = getEnv("MCP_SERVER_TLS_CERT_FILE", "")
	}
	serverTLSKeyFile := tls.keyFile
	if serverTLSKeyFile == "" {
		serverTLSKeyFile = getEnv("MCP_SERVER_TLS_KEY_FILE", "")
	}
	serverTLS := tlsConfig{certFile: serverTLSCertFile, keyFile: serverTLSKeyFile}

	// Build auth config with public paths that don't require authentication
	authCfg := mcpgrafana.AuthConfig{
		Token:       authToken,
		PublicPaths: []string{"/healthz"},
	}

	// Log auth status
	mcpgrafana.LogAuthStatus(authToken)

	if err := run(resolvedAddr, parseLevel(resolvedLogLevel), dt, grafanaCfg, serverTLS, authCfg); err != nil {
		panic(err)
	}
}

func parseLevel(level string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return slog.LevelInfo
	}
	return l
}
