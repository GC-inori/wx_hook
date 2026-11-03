package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wxbot/pkg/protocol"
)

const oneBotAppVersion = "4.1.15.55"

type oneBotCallResult struct {
	retcode int
	data    any
	message string
}

func oneBotFailure(code int, message string, echo json.RawMessage) protocol.Response {
	return protocol.NewResponse(code, map[string]any{}, message, echo)
}

func oneBotRetcode(code int) int {
	switch code {
	case 0, 200:
		return 0
	case 400, 422:
		return 1400
	case 401:
		return 1401
	case 403:
		return 1403
	case 404:
		return 1404
	case 429:
		return 1409
	case 501:
		return 1405
	default:
		return 1500
	}
}

func (s *Server) handleOneBotHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.writeOneBot(w, oneBotFailure(1400, "OneBot actions require POST", nil))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.requestBodyLimit())
	defer r.Body.Close()
	payload, err := json.Marshal(map[string]any{})
	if err != nil {
		s.writeOneBot(w, oneBotFailure(1500, "failed to initialize action parameters", nil))
		return
	}
	action := strings.Trim(strings.TrimSpace(r.URL.Path), "/")
	var echo json.RawMessage
	if action == "" || action == "api" {
		body, readErr := readOneBotBody(r)
		if readErr != nil {
			s.writeOneBot(w, oneBotFailure(1400, readErr.Error(), nil))
			return
		}
		var request protocol.ActionRequest
		if err := json.Unmarshal(body, &request); err != nil || strings.TrimSpace(request.Action) == "" {
			s.writeOneBot(w, oneBotFailure(1400, "request must contain an action", nil))
			return
		}
		action, echo = request.Action, request.Echo
		if len(request.Params) != 0 {
			payload = request.Params
		}
	} else {
		body, readErr := readOneBotBody(r)
		if readErr != nil {
			s.writeOneBot(w, oneBotFailure(1400, readErr.Error(), nil))
			return
		}
		if len(body) != 0 {
			payload = body
		}
	}
	s.writeOneBot(w, s.limitedOneBotActionResponse(r.Context(), action, payload, echo))
}

func readOneBotBody(r *http.Request) ([]byte, error) {
	body, err := json.Marshal(map[string]any{})
	if err != nil {
		return nil, err
	}
	if r.Body == nil {
		return body, nil
	}
	read, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}
	if len(strings.TrimSpace(string(read))) == 0 {
		return body, nil
	}
	if !json.Valid(read) {
		return nil, errors.New("request body is not valid JSON")
	}
	return read, nil
}

func (s *Server) writeOneBot(w http.ResponseWriter, response protocol.Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) limitedOneBotActionResponse(ctx context.Context, action string, params, echo json.RawMessage) protocol.Response {
	select {
	case s.requests <- struct{}{}:
		defer func() { <-s.requests }()
		return s.oneBotActionResponse(ctx, action, params, echo)
	default:
		return oneBotFailure(1409, "action queue is full", echo)
	}
}

func (s *Server) oneBotActionResponse(ctx context.Context, action string, params, echo json.RawMessage) protocol.Response {
	result := s.dispatchOneBotAction(ctx, action, params)
	data := result.data
	if cfg := s.currentGatewayConfig(); cfg.QQCompat && s.idMapper != nil {
		data = s.idMapper.TranslateActionResponseData(action, data)
	}
	return protocol.NewResponse(result.retcode, data, result.message, echo)
}

func (s *Server) dispatchOneBotAction(ctx context.Context, action string, raw json.RawMessage) oneBotCallResult {
	if cfg := s.currentGatewayConfig(); cfg.QQCompat && s.idMapper != nil {
		raw = s.idMapper.TranslateActionParams(action, raw)
	}
	if strings.TrimSpace(rawString(raw)) == "" {
		raw = json.RawMessage(`{}`)
	}
	var params map[string]json.RawMessage
	if !json.Valid(raw) || json.Unmarshal(raw, &params) != nil || params == nil {
		return oneBotCallResult{retcode: 1400, data: map[string]any{}, message: "params must be a JSON object"}
	}
	if !protocol.IsOneBotAction(action) && !isNativeExtendedAction(action) {
		return oneBotCallResult{retcode: 1404, data: map[string]any{}, message: fmt.Sprintf("unknown action: %s", action)}
	}

	switch action {
	case "get_status":
		metrics := s.collectMetrics()
		return oneBotCallResult{data: map[string]any{"online": callerConnected(s.currentCaller()), "good": callerConnected(s.currentCaller()), "stat": metrics}}
	case "get_version_info":
		return oneBotCallResult{data: map[string]any{"app_name": "wxbot-onebot", "app_version": oneBotAppVersion, "protocol_version": "v11"}}
	case "send_msg", "send_group_msg", "send_private_msg":
		return s.oneBotSendMessage(ctx, action, params)
	case "get_login_info":
		result := s.callNative(ctx, "getSelfInfo", json.RawMessage(`{}`))
		if result.retcode != 0 {
			return result
		}
		fields := rawObject(result.data)
		userID := firstRawString(fields, "wxid", "account", "user_id")
		nickname := firstRawString(fields, "nickname", "name")
		return oneBotCallResult{data: map[string]any{"user_id": userID, "nickname": nickname}}
	case "get_friend_list":
		result := s.callNative(ctx, "getFriendList", json.RawMessage(`{}`))
		if result.retcode != 0 {
			return oneBotCallResult{data: []any{}}
		}
		return result
	case "get_group_list":
		result := s.callNative(ctx, "getGroupList", json.RawMessage(`{}`))
		if result.retcode != 0 {
			return oneBotCallResult{data: []any{}}
		}
		return result
	case "get_group_root_files", "get_group_files_by_folder", "get_group_file_url":
		return oneBotCallResult{data: map[string]any{"files": []any{}, "folders": []any{}}}
	case "get_group_member_list":
		group := rawParamString(params, "group_id")
		if group == "" {
			return invalidOneBotParam("group_id is required")
		}
		result := s.callNative(ctx, "getMemberList", marshalParams(map[string]any{"wxid": group, "group_id": group}))
		if result.retcode != 0 {
			return oneBotCallResult{data: []any{}}
		}
		return result
	case "get_group_member_info":
		group, user := rawParamString(params, "group_id"), rawParamString(params, "user_id")
		if group == "" || user == "" {
			return invalidOneBotParam("group_id and user_id are required")
		}
		result := s.callNative(ctx, "getMemberNick", marshalParams(map[string]any{"wxid": user, "member_wxid": user, "group_id": group}))
		if result.retcode != 0 {
			return result
		}
		fields := rawObject(result.data)
		nickname := firstRawString(fields, "nickname", "name")
		return oneBotCallResult{data: map[string]any{"group_id": group, "user_id": user, "nickname": nickname, "card": nickname, "role": "member", "sex": "unknown", "age": 0, "area": "", "join_time": 0, "last_sent_time": 0, "level": "", "unfriendly": false, "title": "", "title_expire_time": 0, "card_changeable": false}}
	case "delete_msg":
		messageID := params["message_id"]
		if len(messageID) == 0 || string(messageID) == "null" {
			return invalidOneBotParam("message_id is required")
		}
		return s.callNative(ctx, "revokeMyMsg", marshalParams(map[string]any{"msg_id": json.RawMessage(messageID), "message_id": json.RawMessage(messageID)}))
	case "get_msg":
		messageID := params["message_id"]
		if len(messageID) == 0 || string(messageID) == "null" {
			return invalidOneBotParam("message_id is required")
		}
		return s.callNative(ctx, "queryObj", marshalParams(map[string]any{"msg_id": json.RawMessage(messageID)}))
	case "set_group_kick":
		group, user := rawParamString(params, "group_id"), rawParamString(params, "user_id")
		if group == "" || user == "" {
			return invalidOneBotParam("group_id and user_id are required")
		}
		return s.callNative(ctx, "delMembers", marshalParams(map[string]any{"wxid": group, "group_wxid": group, "member_wxid": user, "user_id": user, "reject_add_request": rawParamBool(params, "reject_add_request")}))
	case "set_group_leave":
		group := rawParamString(params, "group_id")
		if group == "" {
			return invalidOneBotParam("group_id is required")
		}
		return s.callNative(ctx, "quitGroup", marshalParams(map[string]any{"wxid": group, "group_wxid": group}))
	case "set_friend_add_request":
		flag := rawParamString(params, "flag")
		if flag == "" {
			return invalidOneBotParam("flag is required")
		}
		return s.callNative(ctx, "agreeFriendReq", marshalParams(map[string]any{"flag": flag, "ticket": flag, "approve": rawParamBoolDefault(params, "approve", true), "remark": rawParamString(params, "remark")}))
	case "set_group_add_request":
		return oneBotCallResult{retcode: 1405, data: map[string]any{}, message: "group add requests are not supported by the native bridge"}
	case "send_xml", "send_card", "send_applet", "query_obj", "set_read_status":
		return s.dispatchExtendedOneBot(ctx, action, params)
	default:
		return s.dispatchExtendedOneBot(ctx, action, params)
	}
}

func (s *Server) oneBotSendMessage(ctx context.Context, action string, params map[string]json.RawMessage) oneBotCallResult {
	groupID := rawParamString(params, "group_id")
	userID := rawParamString(params, "user_id")
	messageType := rawParamString(params, "message_type")
	target := ""
	if groupID != "" || messageType == "group" || action == "send_group_msg" {
		target = groupID
	} else {
		target = userID
	}
	if target == "" {
		return invalidOneBotParam("group_id or user_id is required")
	}
	segments, err := protocol.ParseMessage(params["message"])
	if err != nil {
		return invalidOneBotParam(err.Error())
	}
	var text strings.Builder
	mentions := make([]string, 0)
	var lastSequence any
	flushText := func() *oneBotCallResult {
		if text.Len() == 0 {
			return nil
		}
		nativeAction := "sendText"
		args := map[string]any{"target": target, "text": text.String()}
		if len(mentions) != 0 {
			nativeAction = "sendText2"
			memberList := append([]string(nil), mentions...)
			args["members"] = memberList
			args["at_list"] = memberList
			args["at_user_list"] = memberList
		}
		result := s.callNative(ctx, nativeAction, marshalParams(args))
		text.Reset()
		mentions = mentions[:0]
		if result.retcode != 0 {
			return &result
		}
		lastSequence = messageSequenceFromResult(result.data)
		return nil
	}
	for _, segment := range segments {
		switch segment.Type {
		case "text":
			text.WriteString(segmentString(segment.Data["text"]))
		case "at":
			qq := segmentString(segment.Data["qq"])
			if qq == "" {
				return invalidOneBotParam("at segment requires data.qq")
			}
			mentions = append(mentions, qq)
			if qq == "all" {
				text.WriteString("@所有人\u2005")
			} else {
				text.WriteByte('@')
				text.WriteString(qq)
				text.WriteRune('\u2005')
			}
		case "reply":
			// Native WeChat message sending has no reply-reference primitive; ignore
			// the reference segment while sending any following text or media.
			continue
		case "image", "file":
			if failed := flushText(); failed != nil {
				return *failed
			}
			file := segmentString(segment.Data["file"])
			if file == "" {
				return invalidOneBotParam(segment.Type + " segment requires data.file")
			}
			nativeAction := "sendImage"
			fields := map[string]any{"target": target, "image": file}
			if segment.Type == "file" {
				nativeAction = "sendFile"
				fields = map[string]any{"target": target, "file": file}
			}
			result := s.callNative(ctx, nativeAction, marshalParams(fields))
			if result.retcode != 0 {
				return result
			}
			lastSequence = messageSequenceFromResult(result.data)
		default:
			return invalidOneBotParam(fmt.Sprintf("unsupported message segment type %q", segment.Type))
		}
	}
	if failed := flushText(); failed != nil {
		return *failed
	}
	if lastSequence == nil {
		lastSequence = s.nextMessageSequence()
	}
	return oneBotCallResult{data: map[string]any{"message_id": lastSequence}}
}

func (s *Server) dispatchExtendedOneBot(ctx context.Context, action string, params map[string]json.RawMessage) oneBotCallResult {
	nativeAction := action
	switch action {
	case "send_xml":
		nativeAction = "sendXml"
	case "send_card":
		nativeAction = "sendCard"
	case "send_applet":
		nativeAction = "sendApplet"
	case "query_obj":
		nativeAction = "queryObj"
	case "set_read_status":
		nativeAction = "setReadStatus"
	}
	canonical, ok := protocol.Canonical(nativeAction)
	if !ok {
		return oneBotCallResult{retcode: 1404, data: map[string]any{}, message: "unknown action: " + action}
	}
	translated := make(map[string]any, len(params)+2)
	for key, value := range params {
		translated[key] = json.RawMessage(value)
	}
	target := rawParamString(params, "group_id")
	if target == "" {
		target = rawParamString(params, "user_id")
	}
	if target != "" {
		translated["target"] = target
	}
	if len(translated) == 0 {
		return s.callNative(ctx, canonical, json.RawMessage(`{}`))
	}
	return s.callNative(ctx, canonical, marshalParams(translated))
}

func (s *Server) callNative(ctx context.Context, action string, params json.RawMessage) oneBotCallResult {
	code, key, result, err := s.DispatchAction(ctx, action, params)
	if err != nil {
		message := s.text(key)
		if message == "" || message == key {
			message = err.Error()
		}
		if message == "" {
			message = "native bridge action failed"
		}
		return oneBotCallResult{retcode: oneBotRetcode(code), data: result, message: message}
	}
	return oneBotCallResult{data: result}
}

func invalidOneBotParam(message string) oneBotCallResult {
	return oneBotCallResult{retcode: 1400, data: map[string]any{}, message: message}
}

func isNativeExtendedAction(action string) bool {
	_, ok := protocol.Canonical(action)
	return ok
}

func rawString(value json.RawMessage) string {
	return string(value)
}

func rawObject(value any) map[string]json.RawMessage {
	var raw []byte
	switch v := value.(type) {
	case json.RawMessage:
		raw = v
	default:
		raw, _ = json.Marshal(value)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	return fields
}

func rawParamString(params map[string]json.RawMessage, key string) string {
	var value string
	if json.Unmarshal(params[key], &value) == nil {
		return strings.TrimSpace(value)
	}
	var number json.Number
	decoder := json.NewDecoder(strings.NewReader(string(params[key])))
	decoder.UseNumber()
	if decoder.Decode(&number) == nil {
		return number.String()
	}
	return ""
}

func firstRawString(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var value string
		if json.Unmarshal(fields[key], &value) == nil && value != "" {
			return value
		}
		var number json.Number
		decoder := json.NewDecoder(strings.NewReader(string(fields[key])))
		decoder.UseNumber()
		if decoder.Decode(&number) == nil {
			return number.String()
		}
	}
	return ""
}

func rawParamBool(params map[string]json.RawMessage, key string) bool {
	var value bool
	_ = json.Unmarshal(params[key], &value)
	return value
}

func rawParamBoolDefault(params map[string]json.RawMessage, key string, fallback bool) bool {
	if len(params[key]) == 0 {
		return fallback
	}
	return rawParamBool(params, key)
}

func marshalParams(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func segmentString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func messageSequenceFromResult(value any) any {
	fields := rawObject(value)
	for _, key := range []string{"message_id", "msg_id", "sequence"} {
		if raw := fields[key]; len(raw) != 0 && string(raw) != "null" {
			var number json.Number
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			if decoder.Decode(&number) == nil {
				return number
			}
			var text string
			if json.Unmarshal(raw, &text) == nil && text != "" {
				if parsed, err := json.Number(text).Int64(); err == nil {
					return parsed
				}
				return text
			}
		}
	}
	return nil
}

func (s *Server) nextMessageSequence() uint64 {
	if s.messageSequence.Load() == 0 {
		s.messageSequence.CompareAndSwap(0, uint64(time.Now().UnixNano()))
	}
	return s.messageSequence.Add(1)
}
