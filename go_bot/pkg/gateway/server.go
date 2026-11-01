// Package gateway serves the native bridge over HTTP and WebSocket.
package gateway

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wxbot/pkg/client"
	"wxbot/pkg/config"
	"wxbot/pkg/i18n"
)

const defaultMaxRequestBodyMB = 3

//go:embed web/dist/*
var webDist embed.FS

var webFiles fs.FS

func init() {
	var err error
	webFiles, err = fs.Sub(webDist, "web/dist")
	if err != nil {
		panic(err)
	}
}

type Caller interface {
	Call(context.Context, string, any) (json.RawMessage, error)
}

type Response struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	Result    any    `json:"result"`
	Timestamp string `json:"timestamp"`
}

type Server struct {
	callerMu        sync.RWMutex
	caller          Caller
	login           LoginController
	cfgMu           sync.RWMutex
	configUpdateMu  sync.Mutex
	cfg             config.GatewayConfig
	fullConfig      *config.Config
	rootDir         string
	catalogMu       sync.RWMutex
	catalog         *i18n.Catalog
	mux             *http.ServeMux
	hub             *eventHub
	pacer           *OutboundPacer
	requests        chan struct{}
	logs            *logBuffer
	started         time.Time
	metrics         metricsCollector
	recvCount       atomic.Uint64
	sendCount       atomic.Uint64
	messageSequence atomic.Uint64
	wsClientsMu     sync.Mutex
	wsClients       map[string]*wsClient
	closeOnce       sync.Once
}

func New(caller Caller, cfg config.GatewayConfig, catalog *i18n.Catalog) *Server {
	cfg = gatewayDefaults(cfg)
	storedConfig := config.Config{Gateway: cloneGatewayConfig(cfg)}
	s := &Server{caller: caller, cfg: cfg, fullConfig: &storedConfig, catalog: catalog, mux: http.NewServeMux(), hub: newEventHub(cfg.MaxConnections), pacer: NewOutboundPacer(config.PolicyConfig{}), requests: make(chan struct{}, 128), logs: newLogBuffer(maxLogEntries), started: time.Now(), wsClients: make(map[string]*wsClient)}
	s.mux.HandleFunc("POST /{$}", s.handleOneBotHTTP)
	s.mux.HandleFunc("/api", s.handleOneBotHTTP)
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/login/status", s.handleLoginStatus)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/login/qrcode", s.handleLoginQRCode)
	s.mux.HandleFunc("/api/login/refresh", s.handleLoginAction("refresh"))
	s.mux.HandleFunc("/api/login/restart", s.handleLoginAction("restart"))
	s.mux.HandleFunc("/api/login/logout", s.handleLoginAction("logout"))
	s.mux.HandleFunc("/api/logs", s.handleLogs)
	s.mux.HandleFunc("/api/bot/info", s.handleBotInfo)
	s.mux.HandleFunc("/api/", s.handleUnknownAPI)
	s.mux.HandleFunc("/assets/", s.handleStatic)
	s.mux.HandleFunc("/", s.handleStatic)
	return s
}

func gatewayDefaults(cfg config.GatewayConfig) config.GatewayConfig {
	if cfg.MaxConnections < 1 {
		cfg.MaxConnections = 128
	}
	if cfg.RequestTimeoutSec < 1 {
		cfg.RequestTimeoutSec = 7
	}
	if cfg.MaxRequestBodyMB < 1 {
		cfg.MaxRequestBodyMB = defaultMaxRequestBodyMB
	}
	if cfg.WSQueueSize < 16 {
		cfg.WSQueueSize = 256
	}
	if cfg.WSPingIntervalSec < 5 {
		cfg.WSPingIntervalSec = 20
	}
	return cfg
}

func (s *Server) requestBodyLimit() int64 {
	mb := int64(s.currentGatewayConfig().MaxRequestBodyMB)
	const bytesPerMB = int64(1024 * 1024)
	if mb <= 0 {
		mb = defaultMaxRequestBodyMB
	}
	if mb > (1<<63-1)/bytesPerMB {
		return 1<<63 - 1
	}
	return mb * bytesPerMB
}

func (s *Server) requestTimeout() time.Duration {
	value := s.currentGatewayConfig().RequestTimeoutSec
	if value < 1 {
		value = 7
	}
	return seconds(value)
}

func (s *Server) websocketQueueSize() int {
	size := s.currentGatewayConfig().WSQueueSize
	if size < 16 {
		return 256
	}
	return size
}

func (s *Server) websocketPingInterval() time.Duration {
	value := s.currentGatewayConfig().WSPingIntervalSec
	if value < 5 {
		value = 20
	}
	return seconds(value)
}

// SetCaller installs or replaces the live IPC client after login.
func (s *Server) SetCaller(caller Caller) {
	s.callerMu.Lock()
	s.caller = caller
	s.callerMu.Unlock()
}

func (s *Server) SetLoginController(controller LoginController) { s.login = controller }

// SetConfig installs the persisted configuration and its owning root directory.
func (s *Server) SetConfig(cfg *config.Config, rootDir string) {
	if cfg == nil {
		return
	}
	copy := cloneConfig(*cfg)
	copy.RootDir = rootDir
	s.cfgMu.Lock()
	s.fullConfig = &copy
	s.cfg = cloneGatewayConfig(copy.Gateway)
	s.rootDir = rootDir
	s.cfgMu.Unlock()
	s.hub.setLimit(copy.Gateway.MaxConnections)
	s.pacer.UpdatePolicy(copy.Policy)
	s.logs.setLimit(copy.Logging.LogBufferSize)
	s.reconcileWSClients()
}

func cloneConfig(cfg config.Config) config.Config {
	cfg.Gateway = cloneGatewayConfig(cfg.Gateway)
	cfg.Policy.BlacklistWxids = append([]string(nil), cfg.Policy.BlacklistWxids...)
	cfg.Policy.WhitelistGroups = append([]string(nil), cfg.Policy.WhitelistGroups...)
	cfg.Endpoints = append([]config.EndpointConfig(nil), cfg.Endpoints...)
	return cfg
}

func cloneGatewayConfig(cfg config.GatewayConfig) config.GatewayConfig {
	cfg.AllowedOrigins = append([]string(nil), cfg.AllowedOrigins...)
	return cfg
}

func (s *Server) currentGatewayConfig() config.GatewayConfig {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return cloneGatewayConfig(s.cfg)
}

func (s *Server) currentConfig() config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if s.fullConfig == nil {
		return config.Config{Gateway: cloneGatewayConfig(s.cfg)}
	}
	return cloneConfig(*s.fullConfig)
}

func (s *Server) text(key string, args ...any) string {
	s.catalogMu.RLock()
	catalog := s.catalog
	s.catalogMu.RUnlock()
	if catalog == nil {
		return key
	}
	return catalog.Text(key, args...)
}

func (s *Server) currentCaller() Caller {
	s.callerMu.RLock()
	defer s.callerMu.RUnlock()
	return s.caller
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.respond(w, http.StatusOK, "errors.success", s.currentConfig())
	case http.MethodPost:
		s.updateConfig(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		s.respond(w, http.StatusMethodNotAllowed, "errors.method_not_allowed", nil)
	}
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	s.configUpdateMu.Lock()
	defer s.configUpdateMu.Unlock()

	r.Body = http.MaxBytesReader(w, r.Body, s.requestBodyLimit())
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	var updated *config.Config
	if err := decoder.Decode(&updated); err != nil {
		s.decodeError(w, err)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		s.decodeError(w, err)
		return
	}
	if updated == nil {
		s.respond(w, http.StatusBadRequest, "errors.invalid_param", nil)
		return
	}

	s.cfgMu.RLock()
	rootDir := s.rootDir
	oldLocale := s.cfg.Locale
	s.cfgMu.RUnlock()
	if strings.TrimSpace(rootDir) == "" {
		s.respond(w, http.StatusServiceUnavailable, "errors.ipc_error", map[string]any{"detail": "configuration root directory is unavailable"})
		return
	}
	updated.RootDir = rootDir
	if err := updated.Validate(); err != nil {
		s.respond(w, http.StatusBadRequest, "errors.invalid_param", map[string]any{"detail": err.Error()})
		return
	}

	var updatedCatalog *i18n.Catalog
	if updated.Gateway.Locale != oldLocale {
		catalog, err := i18n.Load(rootDir, updated.Gateway.Locale)
		if err != nil {
			s.respond(w, http.StatusBadRequest, "errors.invalid_param", map[string]any{"detail": err.Error()})
			return
		}
		updatedCatalog = catalog
	}
	if err := config.SaveConfig(rootDir, updated); err != nil {
		s.respond(w, http.StatusInternalServerError, "errors.ipc_error", map[string]any{"detail": err.Error()})
		return
	}

	copy := cloneConfig(*updated)
	s.cfgMu.Lock()
	s.fullConfig = &copy
	s.cfg = cloneGatewayConfig(copy.Gateway)
	s.cfgMu.Unlock()
	s.hub.setLimit(copy.Gateway.MaxConnections)
	s.pacer.UpdatePolicy(copy.Policy)
	s.logs.setLimit(copy.Logging.LogBufferSize)
	s.reconcileWSClients()
	if updatedCatalog != nil {
		s.catalogMu.Lock()
		s.catalog = updatedCatalog
		s.catalogMu.Unlock()
	}
	s.respond(w, http.StatusOK, "errors.success", copy)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if !s.authorized(r) {
		if isOneBotHTTPRequest(r) {
			s.writeOneBot(w, oneBotFailure(1401, "invalid or missing API token", nil))
		} else {
			s.respond(w, http.StatusUnauthorized, "errors.unauthorized", nil)
		}
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authorized(r *http.Request) bool {
	gatewayConfig := s.currentGatewayConfig()
	if gatewayConfig.APIToken == "" || (publicWebPath(r.URL.Path) && !isOneBotHTTPRequest(r)) {
		return true
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(token), []byte(gatewayConfig.APIToken)) == 1
}

func publicWebPath(requestPath string) bool {
	if requestPath == "/" || requestPath == "/healthz" || requestPath == "/api/status" || requestPath == "/api/logs" || requestPath == "/api/bot/info" || strings.HasPrefix(requestPath, "/assets/") || strings.HasPrefix(requestPath, "/api/login/") {
		return true
	}
	if requestPath != "/" && requestPath != "/api" && !strings.HasPrefix(requestPath, "/api/") && requestPath != "/ws" && !strings.HasPrefix(requestPath, "/ws/") && !strings.HasPrefix(requestPath, "/healthz/") {
		// Only browser navigation is public; POST requests at these paths are OneBot actions.
		return true
	}
	return false
}

func isOneBotHTTPRequest(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	requestPath := strings.Trim(r.URL.Path, "/")
	if requestPath == "" || requestPath == "api" {
		return true
	}
	return !strings.Contains(requestPath, "/")
}

func (s *Server) respond(w http.ResponseWriter, code int, key string, result any, args ...any) {
	s.writeResponse(w, s.newResponse(code, key, result, args...))
}

func (s *Server) writeResponse(w http.ResponseWriter, response Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(response.Code)
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) decodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		s.respond(w, 413, "errors.request_too_large", nil)
		return
	}
	s.respond(w, 400, "errors.invalid_param", nil)
}

func (s *Server) callError(w http.ResponseWriter, err error) {
	code, key, result := s.mapCallError(err)
	s.respond(w, code, key, result)
}

func callerConnected(caller Caller) bool {
	if caller == nil {
		return false
	}
	if state, ok := caller.(interface{ Connected() bool }); ok {
		return state.Connected()
	}
	return true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.respond(w, 405, "errors.method_not_allowed", nil)
		return
	}
	connected := false
	var dropped uint64
	caller := s.currentCaller()
	if caller != nil {
		connected = true
		if state, ok := caller.(interface{ Connected() bool }); ok {
			connected = state.Connected()
		}
		if state, ok := caller.(interface{ DroppedEvents() uint64 }); ok {
			dropped = state.DroppedEvents()
		}
	}
	if !connected {
		s.respond(w, 503, "errors.ipc_error", map[string]any{"connected": false, "dropped_events": dropped})
		return
	}
	s.respond(w, 200, "errors.success", map[string]any{"connected": true, "dropped_events": dropped})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.respond(w, http.StatusMethodNotAllowed, "errors.method_not_allowed", nil)
		return
	}
	s.respond(w, http.StatusOK, "errors.success", s.collectMetrics())
}

func (s *Server) handleLoginStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeLoginJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	status := LoginStatus{Status: "stopped"}
	if s.login != nil {
		status = s.login.Snapshot()
	}
	metrics := s.collectMetrics()
	status.CPUUsageSystem = metrics.CPUUsageSystem
	status.CPUUsageProcess = metrics.CPUUsageProcess
	status.MemTotalMB = metrics.MemTotalMB
	status.MemUsedMB = metrics.MemUsedMB
	status.MemProcessMB = metrics.MemProcessMB
	status.MemProcessSysMB = metrics.MemProcessSysMB
	status.Goroutines = metrics.Goroutines
	status.RecvCount = metrics.RecvCount
	status.SendCount = metrics.SendCount
	status.WSConnections = metrics.WSConnections
	status.DroppedEvents = metrics.DroppedEvents
	s.writeLoginJSON(w, http.StatusOK, status)
}

func (s *Server) handleLoginQRCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.login == nil {
		http.Error(w, "QR code is not ready", http.StatusServiceUnavailable)
		return
	}
	png, ok := s.login.QRCode()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(png)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(png)
	}
}

func (s *Server) handleLoginAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			s.writeLoginJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		if s.login == nil {
			s.writeLoginJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "login manager is unavailable"})
			return
		}
		var err error
		switch action {
		case "refresh":
			err = s.login.Refresh()
		case "restart":
			err = s.login.Restart()
		case "logout":
			err = s.login.Logout()
		}
		if err != nil {
			s.writeLoginJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
			return
		}
		status := s.login.Snapshot()
		s.writeLoginJSON(w, http.StatusAccepted, status)
	}
}

func (s *Server) writeLoginJSON(w http.ResponseWriter, code int, result any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	msg := http.StatusText(code)
	if code == http.StatusOK || code == http.StatusAccepted {
		msg = "success"
	}
	_ = json.NewEncoder(w).Encode(struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Result any    `json:"result"`
	}{Code: code, Msg: msg, Result: result})
}

func (s *Server) handleUnknownAPI(w http.ResponseWriter, r *http.Request) {
	s.respond(w, http.StatusNotFound, "errors.unknown_action", nil, r.URL.Path)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if isOneBotHTTPRequest(r) && r.URL.Path != "/api" {
		s.handleOneBotHTTP(w, r)
		return
	}
	requestPath := r.URL.Path
	if requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") {
		s.writeLoginJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if requestPath == "/ws" || strings.HasPrefix(requestPath, "/ws/") || strings.HasPrefix(requestPath, "/healthz/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) {
		name = "index.html"
	}
	data, err := fs.ReadFile(webFiles, name)
	if err != nil {
		if strings.HasPrefix(requestPath, "/assets/") {
			http.NotFound(w, r)
			return
		}
		// Vue Router uses history mode; unrecognized browser paths load the SPA shell.
		data, err = fs.ReadFile(webFiles, "index.html")
		name = "index.html"
	}
	if err != nil {
		http.Error(w, "WebUI assets are unavailable", http.StatusInternalServerError)
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}

// Publish is nonblocking; slow WebSocket consumers are disconnected instead of
// retaining unbounded events or stalling the native bridge's receive loop.
func (s *Server) Publish(event client.Event) {
	s.recvCount.Add(1)
	cfg := s.currentConfig()
	if !eventAllowedByPolicy(event, cfg.Policy.BlacklistWxids, cfg.Policy.WhitelistGroups) {
		return
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	s.hub.publish(event, currentSelfID(s))
	if payload, err := marshalWebsocketEvent(event, currentSelfID(s)); err == nil {
		s.publishWSClients(payload)
	}
	for _, endpoint := range cfg.Endpoints {
		if endpoint.Enabled && endpoint.Type == "httpClient" {
			go s.pushWebhook(endpoint, event)
		}
	}
}

func (s *Server) pushWebhook(endpoint config.EndpointConfig, event client.Event) {
	payload, err := marshalWebsocketEvent(event, currentSelfID(s))
	if err != nil {
		log.Printf("webhook %q: encode event: %v", endpoint.ID, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, strings.NewReader(string(payload)))
	if err != nil {
		log.Printf("webhook %q: create request: %v", endpoint.ID, err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	if endpoint.Token != "" {
		request.Header.Set("Authorization", "Bearer "+endpoint.Token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Printf("webhook %q: POST %s failed: %v", endpoint.ID, endpoint.URL, err)
		return
	}
	_ = response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		log.Printf("webhook %q: POST %s returned HTTP %d", endpoint.ID, endpoint.URL, response.StatusCode)
	}
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.hub.close()
		s.stopWSClients()
	})
}
