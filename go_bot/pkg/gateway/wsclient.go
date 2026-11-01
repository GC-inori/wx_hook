package gateway

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"wxbot/pkg/config"
	"wxbot/pkg/protocol"
)

const wsClientReconnectDelay = 3 * time.Second

type wsClient struct {
	endpoint config.EndpointConfig
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	session  *wsClientSession
}

type wsClientSession struct {
	conn     *websocket.Conn
	outbound chan []byte
	close    sync.Once
}

func (c *wsClient) connection() bool {
	c.mu.RLock()
	connected := c.session != nil
	c.mu.RUnlock()
	return connected
}

func (c *wsClient) setSession(session *wsClientSession) {
	c.mu.Lock()
	c.session = session
	c.mu.Unlock()
}

func (c *wsClient) clearSession(session *wsClientSession) {
	c.mu.Lock()
	if c.session == session {
		c.session = nil
	}
	c.mu.Unlock()
}

func (c *wsClient) stop() {
	c.cancel()
	c.mu.Lock()
	session := c.session
	c.session = nil
	c.mu.Unlock()
	if session != nil {
		session.shutdown()
	}
}

func (c *wsClient) enqueue(payload []byte) bool {
	c.mu.RLock()
	session := c.session
	if session == nil {
		c.mu.RUnlock()
		return false
	}
	select {
	case session.outbound <- payload:
		c.mu.RUnlock()
		return true
	default:
		c.mu.RUnlock()
		// A client that cannot keep up is reconnected so it cannot retain an
		// unbounded event backlog or block the native event receiver.
		session.shutdown()
		return false
	}
}

func (s *wsClientSession) shutdown() {
	s.close.Do(func() { _ = s.conn.Close() })
}

func wsClientKey(endpoint config.EndpointConfig) string {
	if endpoint.ID != "" {
		return endpoint.ID
	}
	return endpoint.Type + ":" + endpoint.URL
}

func (s *Server) reconcileWSClients() {
	configSnapshot := s.currentConfig()
	desired := make(map[string]config.EndpointConfig)
	for _, endpoint := range configSnapshot.Endpoints {
		if endpoint.Enabled && endpoint.Type == "wsClient" {
			desired[wsClientKey(endpoint)] = endpoint
		}
	}

	s.wsClientsMu.Lock()
	var stopped []*wsClient
	for key, current := range s.wsClients {
		endpoint, keep := desired[key]
		if keep && endpoint == current.endpoint {
			delete(desired, key)
			continue
		}
		delete(s.wsClients, key)
		stopped = append(stopped, current)
	}
	var started []*wsClient
	for key, endpoint := range desired {
		ctx, cancel := context.WithCancel(context.Background())
		current := &wsClient{endpoint: endpoint, ctx: ctx, cancel: cancel}
		s.wsClients[key] = current
		started = append(started, current)
	}
	s.wsClientsMu.Unlock()

	for _, current := range stopped {
		current.stop()
	}
	for _, current := range started {
		go s.runWSClient(current.ctx, current)
	}
}

func (s *Server) stopWSClients() {
	s.wsClientsMu.Lock()
	clients := make([]*wsClient, 0, len(s.wsClients))
	for key, current := range s.wsClients {
		clients = append(clients, current)
		delete(s.wsClients, key)
	}
	s.wsClientsMu.Unlock()
	for _, current := range clients {
		current.stop()
	}
}

func (s *Server) publishWSClients(eventPayload []byte) {
	s.wsClientsMu.Lock()
	clients := make([]*wsClient, 0, len(s.wsClients))
	for _, current := range s.wsClients {
		clients = append(clients, current)
	}
	s.wsClientsMu.Unlock()
	for _, current := range clients {
		current.enqueue(eventPayload)
	}
}

func (s *Server) runWSClient(ctx context.Context, client *wsClient) {
	for {
		if ctx.Err() != nil {
			return
		}
		header := make(http.Header)
		if client.endpoint.Token != "" {
			header.Set("Authorization", "Bearer "+client.endpoint.Token)
		}
		conn, response, err := websocket.DefaultDialer.DialContext(ctx, client.endpoint.URL, header)
		if err != nil {
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			if ctx.Err() == nil {
				log.Printf("WebSocket client %q: connect %s failed: %v", client.endpoint.ID, client.endpoint.URL, err)
			}
			if !waitContext(ctx, wsClientReconnectDelay) {
				return
			}
			continue
		}
		log.Printf("WebSocket client %q: connected to %s", client.endpoint.ID, client.endpoint.URL)
		if !s.runWSClientSession(ctx, client, conn) {
			return
		}
		if !waitContext(ctx, wsClientReconnectDelay) {
			return
		}
	}
}

func (s *Server) runWSClientSession(parent context.Context, client *wsClient, conn *websocket.Conn) bool {
	ctx, cancel := context.WithCancel(parent)
	session := &wsClientSession{conn: conn, outbound: make(chan []byte, s.websocketQueueSize())}
	client.setSession(session)
	conn.SetReadLimit(s.requestBodyLimit())
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	failures := make(chan error, 2)
	go s.wsClientWriteLoop(ctx, client.endpoint, session, failures)
	go s.wsClientReadLoop(ctx, session, failures)

	select {
	case <-parent.Done():
	case <-failures:
	}
	cancel()
	session.shutdown()
	client.clearSession(session)
	return parent.Err() == nil
}

func (s *Server) wsClientWriteLoop(ctx context.Context, endpoint config.EndpointConfig, session *wsClientSession, failures chan<- error) {
	var ticker *time.Ticker
	var heartbeat <-chan time.Time
	if endpoint.Heartbeat > 0 {
		interval := time.Duration(endpoint.Heartbeat) * time.Millisecond
		if interval > 0 {
			ticker = time.NewTicker(interval)
			heartbeat = ticker.C
			defer ticker.Stop()
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-session.outbound:
			_ = session.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := session.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				select {
				case failures <- err:
				default:
				}
				return
			}
		case <-heartbeat:
			if err := session.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				select {
				case failures <- err:
				default:
				}
				return
			}
		}
	}
}

func (s *Server) wsClientReadLoop(ctx context.Context, session *wsClientSession, failures chan<- error) {
	for {
		_, payload, err := session.conn.ReadMessage()
		if err != nil {
			select {
			case failures <- err:
			default:
			}
			return
		}
		var request protocol.ActionRequest
		var response protocol.Response
		if err := json.Unmarshal(payload, &request); err != nil || strings.TrimSpace(request.Action) == "" {
			response = oneBotFailure(1400, "request must contain an action", nil)
		} else {
			response = s.limitedOneBotActionResponse(ctx, request.Action, request.Params, request.Echo)
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			continue
		}
		select {
		case session.outbound <- encoded:
		case <-ctx.Done():
			return
		default:
			select {
			case failures <- errWSQueueFull:
			default:
			}
			return
		}
	}
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var errWSQueueFull = &wsQueueError{}

type wsQueueError struct{}

func (*wsQueueError) Error() string { return "WebSocket outgoing queue is full" }
