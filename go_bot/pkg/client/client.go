// Package client implements bounded, concurrent request/response IPC with the native bridge.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxPacketSize         = 3 * 1024 * 1024
	eventQueueSize        = 2048
	maxPendingRequests    = 128
	defaultRequestTimeout = 7 * time.Second
	defaultReconnectWait  = 3 * time.Second
)

var (
	ErrClosed       = errors.New("IPC client is closed")
	ErrDisconnected = errors.New("IPC connection lost; request outcome may be unknown")
	ErrBusy         = errors.New("IPC request capacity exceeded")
)

// Event preserves the native event body without converting 64-bit message IDs to floats.
type Event struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	Timestamp string          `json:"timestamp"`
}

type EventCallback func(Event)

// BridgeError preserves the native status, public response code, and structured result.
type BridgeError struct {
	Status  int32
	Code    int
	Message string
	Result  json.RawMessage
}

func (e *BridgeError) Error() string { return fmt.Sprintf("bridge status %d: %s", e.Status, e.Message) }

type ipcPacket struct {
	ID     *uint64         `json:"id,omitempty"`
	Action string          `json:"action,omitempty"`
	Event  string          `json:"event,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Status *int32          `json:"status,omitempty"`
	Code   int             `json:"code,omitempty"`
	Error  string          `json:"error,omitempty"`
	Msg    string          `json:"msg,omitempty"`
}

type callResult struct {
	data json.RawMessage
	err  error
}

type Client struct {
	pipePath          string
	reconnectInterval time.Duration
	connMu            sync.RWMutex
	conn              net.Conn
	selfMu            sync.RWMutex
	selfWxid          string
	writeGate         chan struct{}
	slots             chan struct{}
	reqID             atomic.Uint64
	pendingMu         sync.Mutex
	pending           map[uint64]chan callResult
	onEvent           EventCallback
	events            chan Event
	dropped           atomic.Uint64
	closed            atomic.Bool
	closeChan         chan struct{}
}

// Connect retries the endpoint for five seconds. It never guesses the logged-in account.
func Connect(pid uint32, onEvent EventCallback, reconnectInterval ...time.Duration) (*Client, error) {
	endpoint := ipcEndpoint(pid)
	var conn net.Conn
	var err error
	for i := 0; i < 20; i++ {
		conn, err = dialPipe(endpoint)
		if err == nil {
			break
		}
		if i != 19 {
			time.Sleep(250 * time.Millisecond)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("connect IPC %s: %w", endpoint, err)
	}
	return newClient(endpoint, conn, onEvent, reconnectInterval...), nil
}

func newClient(endpoint string, conn net.Conn, callback EventCallback, reconnectInterval ...time.Duration) *Client {
	interval := defaultReconnectWait
	if len(reconnectInterval) > 0 && reconnectInterval[0] > 0 {
		interval = reconnectInterval[0]
	}
	c := &Client{
		pipePath: endpoint, conn: conn, onEvent: callback, reconnectInterval: interval,
		writeGate: make(chan struct{}, 1), slots: make(chan struct{}, maxPendingRequests),
		pending: make(map[uint64]chan callResult), events: make(chan Event, eventQueueSize),
		closeChan: make(chan struct{}),
	}
	go c.eventLoop()
	go c.readLoop(conn)
	return c
}

func (c *Client) eventLoop() {
	for {
		select {
		case <-c.closeChan:
			return
		case event := <-c.events:
			if c.onEvent != nil {
				func() {
					defer func() {
						if recovered := recover(); recovered != nil {
							log.Printf("IPC event callback panic: %v", recovered)
						}
					}()
					c.onEvent(event)
				}()
			}
		}
	}
}

func (c *Client) DroppedEvents() uint64   { return c.dropped.Load() }
func (c *Client) SetSelfWxid(wxid string) { c.selfMu.Lock(); c.selfWxid = wxid; c.selfMu.Unlock() }
func (c *Client) SelfWxid() string        { c.selfMu.RLock(); defer c.selfMu.RUnlock(); return c.selfWxid }
func (c *Client) Connected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn != nil && !c.closed.Load()
}

func (c *Client) Close() {
	if c.closed.Swap(true) {
		return
	}
	close(c.closeChan)
	c.connMu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()
	c.failPending(ErrClosed)
}

func (c *Client) failPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- callResult{err: err}:
		default:
		}
		delete(c.pending, id)
	}
}

func (c *Client) readLoop(conn net.Conn) {
	for {
		reader := newPacketReader(conn)
		for {
			body, err := reader.ReadPacket()
			if err != nil {
				break
			}
			var pkt ipcPacket
			if err := json.Unmarshal(body, &pkt); err != nil {
				log.Printf("invalid IPC packet: %v", err)
				continue
			}
			if pkt.Event != "" {
				name := canonicalEvent(pkt.Event)
				event := Event{Event: name, Data: pkt.Data, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
				select {
				case c.events <- event:
				default:
					c.dropped.Add(1)
				}
				continue
			}
			if pkt.ID == nil {
				continue
			}
			result := callResult{data: pkt.Result}
			if len(result.data) == 0 {
				result.data = pkt.Data
			}
			if pkt.Status == nil {
				result.err = errors.New("IPC response missing status")
			} else if *pkt.Status != 0 {
				message := pkt.Error
				if message == "" {
					message = pkt.Msg
				}
				result.err = &BridgeError{Status: *pkt.Status, Code: pkt.Code, Message: message, Result: result.data}
			}
			c.pendingMu.Lock()
			ch := c.pending[*pkt.ID]
			if ch != nil {
				select {
				case ch <- result:
				default:
				}
			}
			c.pendingMu.Unlock()
		}
		c.connMu.Lock()
		_ = conn.Close()
		c.conn = nil
		// Fail outstanding calls before reconnecting; side-effecting requests must never be replayed.
		c.failPending(ErrDisconnected)
		c.connMu.Unlock()
		for {
			select {
			case <-c.closeChan:
				return
			case <-time.After(c.reconnectInterval):
			}
			next, err := dialPipe(c.pipePath)
			if err != nil {
				continue
			}
			c.connMu.Lock()
			if c.closed.Load() {
				_ = next.Close()
				c.connMu.Unlock()
				return
			}
			c.conn = next
			conn = next
			c.connMu.Unlock()
			break
		}
	}
}

func canonicalEvent(name string) string {
	switch name {
	case "recv_msg":
		return "recvMsg"
	case "revoke_msg":
		return "revokeMsg"
	case "friend_req":
		return "friendReq"
	case "trans_pay":
		return "transPay"
	case "qr_pay":
		return "qrPay"
	case "group_member_changes":
		return "groupMemberChanges"
	default:
		return name
	}
}

// Call executes any bridge action. Cancellation never replays the operation; a timeout
// after submission means that its final outcome must be reconciled by the caller.
func (c *Client) Call(ctx context.Context, action string, data any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, defaultRequestTimeout)
	}
	defer cancel()
	if c.closed.Load() {
		return nil, ErrClosed
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	default:
		return nil, ErrBusy
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	id := c.reqID.Add(1)
	payload, err := json.Marshal(ipcPacket{ID: &id, Action: action, Data: raw})
	if err != nil {
		return nil, err
	}
	if len(payload) >= maxPacketSize {
		return nil, errors.New("IPC request exceeds size limit")
	}
	response := make(chan callResult, 1)
	select {
	case c.writeGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closeChan:
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		<-c.writeGate
		return nil, err
	}
	// Register and write against the same connection generation as readLoop's failure handling.
	c.connMu.RLock()
	conn := c.conn
	if conn == nil || c.closed.Load() {
		c.connMu.RUnlock()
		<-c.writeGate
		if c.closed.Load() {
			return nil, ErrClosed
		}
		return nil, ErrDisconnected
	}
	c.pendingMu.Lock()
	c.pending[id] = response
	c.pendingMu.Unlock()
	defer func() { c.pendingMu.Lock(); delete(c.pending, id); c.pendingMu.Unlock() }()
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	err = conn.SetWriteDeadline(deadline)
	cancelFinished := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		_ = conn.SetWriteDeadline(time.Now())
		close(cancelFinished)
	})
	if err == nil {
		err = writePacket(conn, payload)
	}
	// Ensure a cancellation callback cannot change the next request's deadline.
	if !stopCancel() {
		<-cancelFinished
	}
	_ = conn.SetWriteDeadline(time.Time{})
	// An incomplete frame cannot be reused safely.
	if err != nil {
		_ = conn.Close()
	}
	c.connMu.RUnlock()
	<-c.writeGate
	if err != nil {
		if contextError := ctx.Err(); contextError != nil {
			return nil, contextError
		}
		return nil, fmt.Errorf("write IPC request (outcome may be unknown): %w", err)
	}
	select {
	case result := <-response:
		return result.data, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closeChan:
		return nil, ErrClosed
	}
}

func (c *Client) SendText(target, text string) error {
	_, err := c.Call(context.Background(), "sendText", map[string]any{"target": target, "text": text})
	return err
}
func (c *Client) SendAtText(target, text string, members ...string) error {
	_, err := c.Call(context.Background(), "sendText2", map[string]any{"target": target, "text": text, "members": members})
	return err
}
func (c *Client) SendImage(target, image string) error {
	_, err := c.Call(context.Background(), "sendImage", map[string]any{"target": target, "image": image})
	return err
}
func (c *Client) SendXml(target, xml string) error {
	_, err := c.Call(context.Background(), "sendXml", map[string]any{"target": target, "xml": xml})
	return err
}
func (c *Client) SendRichMessage(target string, msgType int, content string, isBase64 bool, subtype int) error {
	_, err := c.Call(context.Background(), "send_message", map[string]any{"target": target, "msg_type": msgType, "content": content, "is_base64": isBase64, "subtype": subtype})
	return err
}
func (c *Client) GetAccount() (string, error) {
	wxid, _, err := c.GetSelfInfo()
	return wxid, err
}
func (c *Client) GetSelfInfo() (string, string, error) {
	data, err := c.Call(context.Background(), "getSelfInfo", map[string]any{})
	if err != nil {
		return "", "", err
	}
	var info struct {
		Wxid     string `json:"wxid"`
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", "", err
	}
	c.SetSelfWxid(info.Wxid)
	return info.Wxid, info.Nickname, nil
}
func (c *Client) GetContactName(wxid string) (string, error) {
	data, err := c.Call(context.Background(), "getMemberNick", map[string]string{"wxid": wxid})
	if err != nil {
		return "", err
	}
	var info struct {
		Name     string `json:"name"`
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", err
	}
	if info.Nickname != "" {
		return info.Nickname, nil
	}
	return info.Name, nil
}
