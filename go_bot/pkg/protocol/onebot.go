// Package protocol defines OneBot 11 protocol types and helpers.
package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Segment is a OneBot 11 message segment.
type Segment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// Response is the standard OneBot 11 action response envelope.
type Response struct {
	Status  string `json:"status"`
	Retcode int    `json:"retcode"`
	Data    any    `json:"data"`
	Message string `json:"message"`
	Wording string `json:"wording"`
	Echo    any    `json:"echo,omitempty"`
}

// Event is a OneBot 11 event with extensions preserved for native clients.
type Event map[string]any

// ActionRequest is a OneBot 11 action request.
type ActionRequest struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
	Echo   json.RawMessage `json:"echo,omitempty"`
}

// OneBotActions lists standard OneBot 11 actions implemented or recognized by this gateway.
var OneBotActions = []string{
	"send_msg", "send_group_msg", "send_private_msg", "delete_msg", "get_msg", "get_login_info",
	"get_friend_list", "get_group_list", "get_group_member_list", "get_group_member_info", "set_group_kick", "set_group_card",
	"get_group_root_files", "get_group_files_by_folder", "get_group_file_url",
	"set_group_leave", "set_friend_add_request", "set_friend_remark", "set_group_add_request", "delete_friend", "get_status", "get_version_info",
	"create_group", "add_members",
	"send_xml", "send_card", "send_applet", "query_obj", "set_read_status",
}

var oneBotActionSet = func() map[string]struct{} {
	result := make(map[string]struct{}, len(OneBotActions))
	for _, action := range OneBotActions {
		result[action] = struct{}{}
	}
	return result
}()

// IsOneBotAction reports whether action is a standard or declared extension action.
func IsOneBotAction(action string) bool {
	_, ok := oneBotActionSet[action]
	return ok
}

// NewResponse creates a successful or failed OneBot response.
func NewResponse(retcode int, data any, message string, echo any) Response {
	status := "ok"
	if retcode != 0 {
		status = "failed"
	}
	if data == nil {
		data = map[string]any{}
	}
	wording := ""
	if retcode != 0 {
		wording = message
	}
	return Response{Status: status, Retcode: retcode, Data: data, Message: message, Wording: wording, Echo: echo}
}

// ParseMessage accepts either a string with CQ codes or OneBot segment array.
func ParseMessage(message json.RawMessage) ([]Segment, error) {
	if len(message) == 0 || string(message) == "null" {
		return nil, fmt.Errorf("message is required")
	}
	var text string
	if err := json.Unmarshal(message, &text); err == nil {
		return ParseCQCode(text), nil
	}
	var segments []Segment
	if err := json.Unmarshal(message, &segments); err != nil {
		return nil, fmt.Errorf("message must be a string or segment array: %w", err)
	}
	for i := range segments {
		if segments[i].Data == nil {
			segments[i].Data = map[string]any{}
		}
		segments[i].Type = strings.ToLower(strings.TrimSpace(segments[i].Type))
	}
	return segments, nil
}

// ParseCQCode parses CQ codes while preserving text outside codes.
func ParseCQCode(input string) []Segment {
	segments := make([]Segment, 0)
	for len(input) > 0 {
		start := strings.Index(input, "[CQ:")
		if start < 0 {
			appendText(&segments, input)
			break
		}
		if start > 0 {
			appendText(&segments, input[:start])
		}
		rest := input[start+4:]
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			appendText(&segments, input[start:])
			break
		}
		segment, ok := parseCQBody(rest[:end])
		if ok {
			segments = append(segments, segment)
		} else {
			appendText(&segments, input[start:start+4+end+1])
		}
		input = rest[end+1:]
	}
	return segments
}

func parseCQBody(body string) (Segment, bool) {
	parts := strings.Split(body, ",")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return Segment{}, false
	}
	data := make(map[string]any)
	for _, item := range parts[1:] {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		data[strings.TrimSpace(key)] = cqUnescape(value)
	}
	return Segment{Type: strings.ToLower(strings.TrimSpace(parts[0])), Data: data}, true
}

func appendText(segments *[]Segment, text string) {
	if text == "" {
		return
	}
	if n := len(*segments); n > 0 && (*segments)[n-1].Type == "text" {
		previous := (*segments)[n-1].Data["text"].(string)
		(*segments)[n-1].Data["text"] = previous + text
		return
	}
	*segments = append(*segments, Segment{Type: "text", Data: map[string]any{"text": text}})
}

func cqUnescape(value string) string {
	value = strings.ReplaceAll(value, "&#44;", ",")
	value = strings.ReplaceAll(value, "&#91;", "[")
	value = strings.ReplaceAll(value, "&#93;", "]")
	return strings.ReplaceAll(value, "&amp;", "&")
}

func cqEscape(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "[", "&#91;")
	value = strings.ReplaceAll(value, "]", "&#93;")
	return strings.ReplaceAll(value, ",", "&#44;")
}

// FormatCQCode serializes segments as OneBot CQ-code raw_message.
func FormatCQCode(segments []Segment) string {
	var output strings.Builder
	for _, segment := range segments {
		if segment.Type == "text" {
			if text, ok := segment.Data["text"].(string); ok {
				output.WriteString(text)
			}
			continue
		}
		output.WriteString("[CQ:")
		output.WriteString(segment.Type)
		keys := make([]string, 0, len(segment.Data))
		for key := range segment.Data {
			keys = append(keys, key)
		}
		// Stable ordering makes CQ output deterministic.
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[j] < keys[i] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}
		for _, key := range keys {
			output.WriteByte(',')
			output.WriteString(key)
			output.WriteByte('=')
			output.WriteString(cqEscape(fmt.Sprint(segment.Data[key])))
		}
		output.WriteByte(']')
	}
	return output.String()
}

// FormatMessage converts plain text and CQ-coded native strings into message segments.
func FormatMessage(text string) []Segment {
	return ParseCQCode(text)
}

// EventTime extracts a Unix timestamp from either RFC3339 or Unix-form native time.
func EventTime(timestamp string) int64 {
	if timestamp == "" {
		return time.Now().Unix()
	}
	if n, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
		if n > 1_000_000_000_000 {
			return n / 1000
		}
		return n
	}
	if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
		return parsed.Unix()
	}
	return time.Now().Unix()
}
