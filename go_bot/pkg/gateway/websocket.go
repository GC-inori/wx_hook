package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"wxbot/pkg/client"
	"wxbot/pkg/protocol"
)

type subscriber struct {
	queue       chan []byte
	actionQueue chan []byte
	done        chan struct{}
	once        sync.Once
}

func (p *subscriber) stop() { p.once.Do(func() { close(p.done) }) }

type eventHub struct {
	mu     sync.Mutex
	peers  map[*subscriber]struct{}
	limit  int
	closed bool
}

func newEventHub(limit int) *eventHub {
	if limit < 1 {
		limit = 128
	}
	return &eventHub{peers: make(map[*subscriber]struct{}), limit: limit}
}

func (h *eventHub) setLimit(limit int) {
	if limit < 1 {
		return
	}
	h.mu.Lock()
	h.limit = limit
	h.mu.Unlock()
}

func (h *eventHub) add(p *subscriber) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.peers) >= h.limit {
		return false
	}
	h.peers[p] = struct{}{}
	return true
}
func (h *eventHub) remove(p *subscriber) {
	h.mu.Lock()
	delete(h.peers, p)
	h.mu.Unlock()
	p.stop()
}
func (h *eventHub) publish(event client.Event, selfIDs ...string) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	payload, err := marshalWebsocketEvent(event, selfIDs...)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for peer := range h.peers {
		select {
		case peer.queue <- payload:
		default:
			peer.stop()
			delete(h.peers, peer)
		}
	}
}

func (h *eventHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.peers)
}
func (h *eventHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for peer := range h.peers {
		peer.stop()
		delete(h.peers, peer)
	}
}

func (s *Server) allowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // Native clients have no browser origin.
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	for _, allowed := range s.currentGatewayConfig().AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.respond(w, 405, "errors.method_not_allowed", nil)
		return
	}
	if !s.allowedOrigin(r) {
		s.respond(w, 403, "errors.unauthorized", nil)
		return
	}
	queueSize := s.websocketQueueSize()
	pingInterval := s.websocketPingInterval()
	peer := &subscriber{
		queue:       make(chan []byte, queueSize),
		actionQueue: make(chan []byte, queueSize),
		done:        make(chan struct{}),
	}
	if !s.hub.add(peer) {
		s.respond(w, 429, "errors.rate_limit", nil)
		return
	}
	defer s.hub.remove(peer)
	upgrader := websocket.Upgrader{
		ReadBufferSize: 1024, WriteBufferSize: 4096,
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin:      s.allowedOrigin,
		Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
			s.respond(w, status, "errors.invalid_param", nil)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(s.requestBodyLimit())
	readWait := pingInterval
	if pingInterval <= time.Duration((1<<63-1)/3) {
		readWait *= 3
	} else {
		readWait = time.Duration(1<<63 - 1)
	}
	readDeadline := time.Now().Add(readWait)
	_ = conn.SetReadDeadline(readDeadline)
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(readWait)) })
	go s.wsReadLoop(conn, peer)
	go s.wsActionLoop(r.Context(), peer)

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-peer.done:
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "stream closed; reconnect and resynchronize"), time.Now().Add(time.Second))
			return
		case data := <-peer.queue:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

func (s *Server) wsReadLoop(conn *websocket.Conn, peer *subscriber) {
	defer peer.stop()
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		select {
		case peer.actionQueue <- payload:
		case <-peer.done:
			return
		default:
			var request protocol.ActionRequest
			_ = json.Unmarshal(payload, &request)
			response := oneBotFailure(1409, "action queue is full", request.Echo)
			if !s.enqueueWSResponse(peer, response) {
				return
			}
		}
	}
}

func (s *Server) wsActionLoop(ctx context.Context, peer *subscriber) {
	for {
		select {
		case <-peer.done:
			return
		case payload := <-peer.actionQueue:
			var request protocol.ActionRequest
			if err := json.Unmarshal(payload, &request); err != nil || strings.TrimSpace(request.Action) == "" {
				response := oneBotFailure(1400, "request must contain an action", nil)
				if !s.enqueueWSResponse(peer, response) {
					return
				}
				continue
			}
			response := s.limitedOneBotActionResponse(ctx, request.Action, request.Params, request.Echo)
			if !s.enqueueWSResponse(peer, response) {
				return
			}
		}
	}
}

func (s *Server) enqueueWSResponse(peer *subscriber, response protocol.Response) bool {
	payload, err := json.Marshal(response)
	if err != nil {
		return false
	}
	select {
	case peer.queue <- payload:
		return true
	case <-peer.done:
		return false
	default:
		peer.stop()
		return false
	}
}
