package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"wxbot/pkg/model"
)

const maxPacketSize = 10 * 1024 * 1024

type EventCallback func(msg *model.Message)

type Client struct {
	pipePath  string
	selfWxid  string
	conn      net.Conn
	mu        sync.Mutex
	reqID     atomic.Uint64
	pending   map[uint64]chan *ipcResponse
	pendingMu sync.Mutex
	onEvent   EventCallback
	closed    atomic.Bool
	closeChan chan struct{}
}

type ipcPacket struct {
	ID     *uint64          `json:"id,omitempty"`
	Action string           `json:"action,omitempty"`
	Event  string           `json:"event,omitempty"`
	Data   *json.RawMessage `json:"data,omitempty"`
	Status *int32           `json:"status,omitempty"`
	Error  string           `json:"error,omitempty"`
}

type ipcResponse struct {
	ID     *uint64          `json:"id,omitempty"`
	Status int32            `json:"status"`
	Error  string           `json:"error"`
	Data   *json.RawMessage `json:"data,omitempty"`
}

type recvMsgData struct {
	MsgType       uint32 `json:"msg_type"`
	Sender        string `json:"sender"`
	Receiver      string `json:"receiver"`
	Text          string `json:"text"`
	MentionSource string `json:"mention_source"`
	Sequence      uint64 `json:"sequence"`
}

// Connect creates and starts an IPC client for the platform endpoint.
func Connect(pid uint32, onEvent EventCallback) (*Client, error) {
	pipePath := ipcEndpoint(pid)

	var conn net.Conn
	var err error
	// Retry connection for up to 5 seconds
	for i := 0; i < 20; i++ {
		conn, err = dialPipe(pipePath)
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("连接 IPC 端点 %s 失败: %w", pipePath, err)
	}

	cli := &Client{
		pipePath:  pipePath,
		conn:      conn,
		pending:   make(map[uint64]chan *ipcResponse),
		onEvent:   onEvent,
		closeChan: make(chan struct{}),
	}

	go cli.readLoop()

	// Initial fetch of account wxid
	account, err := cli.GetAccount()
	if err == nil {
		cli.selfWxid = account
	}

	return cli, nil
}

func (c *Client) SetSelfWxid(wxid string) {
	c.selfWxid = wxid
}

func (c *Client) SelfWxid() string {
	return c.selfWxid
}

func (c *Client) Close() {
	if c.closed.Swap(true) {
		return
	}
	close(c.closeChan)
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) readLoop() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}

	reader := newPacketReader(conn)
	for !c.closed.Load() {
		body, err := reader.ReadPacket()
		if err != nil {
			if !c.closed.Load() {
				// Reconnect if disconnected.
				c.reconnect()
			}
			return
		}

		var pkt ipcPacket
		if err := json.Unmarshal(body, &pkt); err != nil {
			log.Printf("IPC 收到无效 JSON 数据包: %v", err)
			continue
		}

		if pkt.Data != nil {
			switch pkt.Event {
			case "recv_msg":
				var raw recvMsgData
				if err := json.Unmarshal(*pkt.Data, &raw); err == nil {
					msg := model.BuildMessage(
						raw.MsgType,
						raw.Sender,
						raw.Receiver,
						raw.Text,
						raw.MentionSource,
						raw.Sequence,
						c.selfWxid,
					)
					if c.onEvent != nil {
						go c.onEvent(msg)
					}
				}
				continue
			case "mmmojo_read":
				logMMMojoRead(*pkt.Data)
				continue
			case "mmmojo_write":
				logMMMojoWrite(*pkt.Data)
				continue
			}
		}

		// Handle Request-Response.
		if pkt.ID != nil {
			c.pendingMu.Lock()
			ch, ok := c.pending[*pkt.ID]
			c.pendingMu.Unlock()
			if ok {
				status := int32(0)
				if pkt.Status != nil {
					status = *pkt.Status
				}
				ch <- &ipcResponse{
					ID:     pkt.ID,
					Status: status,
					Error:  pkt.Error,
					Data:   pkt.Data,
				}
			}
		}
	}
}

func logMMMojoRead(data json.RawMessage) {
	var event struct {
		CallbackType    int    `json:"callback_type"`
		RequestID       uint32 `json:"request_id"`
		RequestDataSize uint32 `json:"request_data_size"`
		AttachDataSize  uint32 `json:"attach_data_size"`
		RequestPreview  string `json:"request_preview"`
		AttachPreview   string `json:"attach_preview"`
		Truncated       bool   `json:"truncated"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	// 只在携带业务数据（正文或附件 > 0）时记录，避免心跳和定时器刷屏
	if event.RequestDataSize > 0 || event.AttachDataSize > 0 {
		log.Printf("[Mojo管道-收到] 回调类型: %d, 请求ID: %d, 正文大小: %d 字节, 附件大小: %d 字节",
			event.CallbackType, event.RequestID, event.RequestDataSize, event.AttachDataSize)
	}
}

func logMMMojoWrite(data json.RawMessage) {
	var event struct {
		Method           *int    `json:"method"`
		Sync             *bool   `json:"sync"`
		RequestID        *uint32 `json:"request_id"`
		RequestDataSize  *uint64 `json:"request_data_size"`
		RequestPreview   string  `json:"request_preview"`
		PayloadAvailable bool    `json:"payload_available"`
		Truncated        bool    `json:"truncated"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	methodVal := -1
	if event.Method != nil {
		methodVal = *event.Method
	}
	syncVal := false
	if event.Sync != nil {
		syncVal = *event.Sync
	}
	reqIDVal := uint32(0)
	if event.RequestID != nil {
		reqIDVal = *event.RequestID
	}
	reqSizeVal := uint64(0)
	if event.RequestDataSize != nil {
		reqSizeVal = *event.RequestDataSize
	}
	// 只在携带有效载荷且大小 > 0 时记录，避免空心跳刷屏
	if reqSizeVal > 0 {
		log.Printf("[Mojo管道-发出] 业务方法: %d, 同步: %t, 请求ID: %d, 数据大小: %d 字节",
			methodVal, syncVal, reqIDVal, reqSizeVal)
	}
}

func (c *Client) reconnect() {
	for !c.closed.Load() {
		time.Sleep(1 * time.Second)
		conn, err := dialPipe(c.pipePath)
		if err == nil {
			c.mu.Lock()
			c.conn = conn
			c.mu.Unlock()
			go c.readLoop()
			return
		}
	}
}

func (c *Client) call(action string, data interface{}) (*ipcResponse, error) {
	id := c.reqID.Add(1)

	var rawData *json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		raw := json.RawMessage(b)
		rawData = &raw
	}

	req := ipcPacket{
		ID:     &id,
		Action: action,
		Data:   rawData,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	respChan := make(chan *ipcResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("IPC 未连接")
	}
	err = writePacket(c.conn, payload)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}

	select {
	case resp := <-respChan:
		if resp.Status != 0 {
			return nil, fmt.Errorf("底层执行失败 (代码 %d): %s", resp.Status, resp.Error)
		}
		return resp, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("请求超时 (action=%s)", action)
	case <-c.closeChan:
		return nil, fmt.Errorf("连接已关闭")
	}
}

func (c *Client) SendText(target, text string) error {
	_, err := c.call("send_text", map[string]string{
		"target": target,
		"text":   text,
	})
	return err
}

func (c *Client) SendAtText(target, text, members string) error {
	_, err := c.call("send_at_text", map[string]string{
		"target":  target,
		"text":    text,
		"members": members,
	})
	return err
}

func (c *Client) GetAccount() (string, error) {
	resp, err := c.call("get_account", nil)
	if err != nil {
		return "", err
	}
	if resp.Data == nil {
		return "", fmt.Errorf("返回数据为空")
	}
	var res struct {
		Account string `json:"account"`
	}
	if err := json.Unmarshal(*resp.Data, &res); err != nil {
		return "", err
	}
	return res.Account, nil
}

func (c *Client) GetContactName(wxid string) (string, error) {
	resp, err := c.call("get_contact_name", map[string]string{"wxid": wxid})
	if err != nil {
		return "", err
	}
	if resp.Data == nil {
		return "", fmt.Errorf("返回数据为空")
	}
	var res struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(*resp.Data, &res); err != nil {
		return "", err
	}
	return res.Name, nil
}
