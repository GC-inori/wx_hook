package protocol

// MessageSegment is an alias for a OneBot 11 message segment.
type MessageSegment = Segment

// Sender contains the standard sender fields used by message events.
type Sender struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
	Sex      string `json:"sex,omitempty"`
	Age      int    `json:"age,omitempty"`
}

// MessageEvent is the OneBot 11 message event envelope.
type MessageEvent struct {
	Time        int64     `json:"time"`
	SelfID      string    `json:"self_id"`
	PostType    string    `json:"post_type"`
	MessageType string    `json:"message_type"`
	SubType     string    `json:"sub_type"`
	MessageID   any       `json:"message_id"`
	GroupID     string    `json:"group_id,omitempty"`
	UserID      string    `json:"user_id"`
	Message     []Segment `json:"message"`
	RawMessage  string    `json:"raw_message"`
	Font        int       `json:"font"`
	Sender      Sender    `json:"sender"`
}

// NoticeEvent is a minimal OneBot 11 notice event envelope.
type NoticeEvent struct {
	Time       int64  `json:"time"`
	SelfID     string `json:"self_id"`
	PostType   string `json:"post_type"`
	NoticeType string `json:"notice_type"`
	SubType    string `json:"sub_type,omitempty"`
	MessageID  any    `json:"message_id,omitempty"`
	GroupID    string `json:"group_id,omitempty"`
	UserID     string `json:"user_id,omitempty"`
}

// RequestEvent is a minimal OneBot 11 friend/group request event envelope.
type RequestEvent struct {
	Time        int64  `json:"time"`
	SelfID      string `json:"self_id"`
	PostType    string `json:"post_type"`
	RequestType string `json:"request_type"`
	SubType     string `json:"sub_type,omitempty"`
	UserID      string `json:"user_id"`
	GroupID     string `json:"group_id,omitempty"`
	Comment     string `json:"comment"`
	Flag        string `json:"flag"`
}
