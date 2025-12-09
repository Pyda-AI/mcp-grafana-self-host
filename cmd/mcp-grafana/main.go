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
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"

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

// Config represents the YAML configuration file structure
type Config struct {
	GrafanaURL                  string `yaml:"grafana_url"`
	GrafanaServiceAccountToken  string `yaml:"grafana_service_account_token"`
	MCPAuthToken                string `yaml:"mcp_auth_token"`
	ServerPort                  string `yaml:"server_port"`
	BasePath                    string `yaml:"base_path"`
	EndpointPath                string `yaml:"endpoint_path"`
	LogLevel                    string `yaml:"log_level"`
	Debug                       bool   `yaml:"debug"`
	TLSCertFile                 string `yaml:"tls_cert_file"`
	TLSKeyFile                  string `yaml:"tls_key_file"`
	TLSCAFile                   string `yaml:"tls_ca_file"`
	TLSSkipVerify               bool   `yaml:"tls_skip_verify"`
	ServerTLSCertFile           string `yaml:"server_tls_cert_file"`
	ServerTLSKeyFile            string `yaml:"server_tls_key_file"`
}

// loadConfig loads configuration from config.yaml if it exists
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Config file doesn't exist, use defaults
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// getConfigValue returns value from: 1) env var, 2) config file, 3) default
func getConfigValue(envVar string, configValue string, defaultValue string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	if configValue != "" {
		return configValue
	}
	return defaultValue
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

func newServer(transport string, dt disabledTools) (*server.MCPServer, *mcpgrafana.ToolManager) {
	sm := mcpgrafana.NewSessionManager()

	// Declare variable for ToolManager that will be initialized after server creation
	var stm *mcpgrafana.ToolManager

	// Create hooks
	hooks := &server.Hooks{
		OnRegisterSession:   []server.OnRegisterSessionHookFunc{sm.CreateSession},
		OnUnregisterSession: []server.OnUnregisterSessionHookFunc{sm.RemoveSession},
	}

	// Add proxied tools hooks if enabled and we're not running in stdio mode.
	// (stdio mode is handled by InitializeAndRegisterServerTools; per-session tools
	// are not supported).
	if transport != "stdio" && !dt.proxied {
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

func run(transport, addr, basePath, endpointPath string, logLevel slog.Level, dt disabledTools, gc mcpgrafana.GrafanaConfig, tls tlsConfig, auth mcpgrafana.AuthConfig) error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
	s, tm := newServer(transport, dt)

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

		// For stdio, close stdin to unblock the Listen call
		if transport == "stdio" {
			_ = os.Stdin.Close()
		}
	}()

	// Start the appropriate server based on transport
	switch transport {
	case "stdio":
		srv := server.NewStdioServer(s)
		cf := mcpgrafana.ComposedStdioContextFunc(gc)
		srv.SetContextFunc(cf)

		// For stdio (single-tenant), initialize proxied tools on the server directly
		if !dt.proxied {
			stdioCtx := cf(ctx)
			if err := tm.InitializeAndRegisterServerTools(stdioCtx); err != nil {
				slog.Error("failed to initialize proxied tools for stdio", "error", err)
			}
		}

		slog.Info("Starting Grafana MCP server using stdio transport", "version", mcpgrafana.Version())

		err := srv.Listen(ctx, os.Stdin, os.Stdout)
		if err != nil && err != context.Canceled {
			return fmt.Errorf("server error: %v", err)
		}
		return nil

	case "sse":
		httpSrv := &http.Server{Addr: addr}
		srv := server.NewSSEServer(s,
			server.WithSSEContextFunc(mcpgrafana.ComposedSSEContextFunc(gc)),
			server.WithStaticBasePath(basePath),
			server.WithHTTPServer(httpSrv),
		)
		mux := http.NewServeMux()
		if basePath == "" {
			basePath = "/"
		}
		mux.Handle(basePath, srv)
		mux.HandleFunc("/healthz", handleHealthz)

		// Apply Bearer token authentication middleware if configured
		if auth.Token != "" {
			httpSrv.Handler = mcpgrafana.NewBearerAuthMiddleware(auth)(mux)
		} else {
			httpSrv.Handler = mux
		}

		slog.Info("Starting Grafana MCP server using SSE transport",
			"version", mcpgrafana.Version(), "address", addr, "basePath", basePath, "auth_enabled", auth.Token != "")
		return runHTTPServer(ctx, srv, addr, "SSE")
	case "streamable-http":
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

		// Apply Bearer token authentication middleware if configured
		if auth.Token != "" {
			httpSrv.Handler = mcpgrafana.NewBearerAuthMiddleware(auth)(mux)
		} else {
			httpSrv.Handler = mux
		}

		slog.Info("Starting Grafana MCP server using StreamableHTTP transport",
			"version", mcpgrafana.Version(), "address", addr, "endpointPath", endpointPath, "auth_enabled", auth.Token != "")
		return runHTTPServer(ctx, srv, addr, "StreamableHTTP")
	default:
		return fmt.Errorf("invalid transport type: %s. Must be 'stdio', 'sse' or 'streamable-http'", transport)
	}
}

func main() {
	// Load .env file first (environment variables take precedence over .env)
	if err := loadEnvFile(".env"); err != nil {
		slog.Warn("Failed to load .env file", "error", err)
	}

	// Load config.yaml as fallback (env vars from .env or system take precedence)
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		slog.Error("Failed to load config file", "error", err)
		os.Exit(1)
	}
	if cfg == nil {
		cfg = &Config{} // Use empty config if file doesn't exist
	}

	// Define flags (can still override config/env)
	serverPort := flag.String("port", "", "The port to start the server on")
	basePath := flag.String("base-path", "", "Base path for the server")
	endpointPath := flag.String("endpoint-path", "", "Endpoint path for the streamable-http server")
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

	// Resolve configuration: CLI flag > env var > config file > default
	// Transport is always streamable-http
	resolvedTransport := "streamable-http"

	// Server port
	resolvedPort := *serverPort
	if resolvedPort == "" {
		resolvedPort = getConfigValue("MCP_SERVER_PORT", cfg.ServerPort, "8443")
	}
	resolvedAddr := "0.0.0.0:" + resolvedPort

	// Base path
	resolvedBasePath := *basePath
	if resolvedBasePath == "" {
		resolvedBasePath = getConfigValue("MCP_BASE_PATH", cfg.BasePath, "")
	}

	// Endpoint path
	resolvedEndpointPath := *endpointPath
	if resolvedEndpointPath == "" {
		resolvedEndpointPath = getConfigValue("MCP_ENDPOINT_PATH", cfg.EndpointPath, "/mcp")
	}

	// Log level
	resolvedLogLevel := *logLevel
	if resolvedLogLevel == "" {
		resolvedLogLevel = getConfigValue("MCP_LOG_LEVEL", cfg.LogLevel, "info")
	}

	// Get auth token from env var or config file (required for HTTP transports)
	authToken := getConfigValue(mcpgrafana.AuthTokenEnvVar, cfg.MCPAuthToken, "")

	// Convert local grafanaConfig to mcpgrafana.GrafanaConfig
	grafanaCfg := mcpgrafana.GrafanaConfig{Debug: gc.debug || cfg.Debug}

	// TLS config from flags or config file
	tlsCertFile := gc.tlsCertFile
	if tlsCertFile == "" {
		tlsCertFile = cfg.TLSCertFile
	}
	tlsKeyFile := gc.tlsKeyFile
	if tlsKeyFile == "" {
		tlsKeyFile = cfg.TLSKeyFile
	}
	tlsCAFile := gc.tlsCAFile
	if tlsCAFile == "" {
		tlsCAFile = cfg.TLSCAFile
	}
	tlsSkipVerify := gc.tlsSkipVerify || cfg.TLSSkipVerify

	if tlsCertFile != "" || tlsKeyFile != "" || tlsCAFile != "" || tlsSkipVerify {
		grafanaCfg.TLSConfig = &mcpgrafana.TLSConfig{
			CertFile:   tlsCertFile,
			KeyFile:    tlsKeyFile,
			CAFile:     tlsCAFile,
			SkipVerify: tlsSkipVerify,
		}
	}

	// Server TLS config
	serverTLSCertFile := tls.certFile
	if serverTLSCertFile == "" {
		serverTLSCertFile = cfg.ServerTLSCertFile
	}
	serverTLSKeyFile := tls.keyFile
	if serverTLSKeyFile == "" {
		serverTLSKeyFile = cfg.ServerTLSKeyFile
	}
	serverTLS := tlsConfig{certFile: serverTLSCertFile, keyFile: serverTLSKeyFile}

	// Build auth config with public paths that don't require authentication
	authCfg := mcpgrafana.AuthConfig{
		Token:       authToken,
		PublicPaths: []string{"/healthz"},
	}

	// Log auth status (only for HTTP transports)
	if resolvedTransport != "stdio" {
		mcpgrafana.LogAuthStatus(authToken)
	}

	if err := run(resolvedTransport, resolvedAddr, resolvedBasePath, resolvedEndpointPath, parseLevel(resolvedLogLevel), dt, grafanaCfg, serverTLS, authCfg); err != nil {
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
