package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"wxbot/pkg/client"
	"wxbot/pkg/protocol"
)

// DispatchAction is the shared action pipeline for HTTP and WebSocket ingress.
// It validates and canonicalizes the action, applies outbound pacing,
// resolves remote media, then invokes the current native bridge caller.
func (s *Server) DispatchAction(ctx context.Context, actionType string, rawData json.RawMessage) (code int, key string, result any, err error) {
	if strings.TrimSpace(actionType) == "" {
		return 400, "errors.invalid_param", map[string]any{}, errors.New("action type is empty")
	}
	canonical, ok := protocol.Canonical(actionType)
	if !ok {
		return 404, "errors.unknown_action", map[string]any{}, fmt.Errorf("unknown action %q", actionType)
	}

	body := strings.TrimSpace(string(rawData))
	if body == "" {
		rawData = json.RawMessage(`{}`)
	} else if !strings.HasPrefix(body, "{") || !json.Valid(rawData) {
		return 400, "errors.invalid_param", map[string]any{}, errors.New("action data must be a valid JSON object")
	}

	if canonical == "delFriend" {
		var params map[string]json.RawMessage
		if json.Unmarshal(rawData, &params) == nil && params != nil && rawParamString(params, "wxid") == "" {
			if userID := rawParamString(params, "user_id"); userID != "" {
				params["wxid"], _ = json.Marshal(userID)
				if normalized, err := json.Marshal(params); err == nil {
					rawData = normalized
				}
			}
		}
	}

	caller := s.currentCaller()
	if caller == nil || !callerConnected(caller) {
		return 401, "errors.account_not_logged_in", map[string]any{}, client.ErrDisconnected
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if canonical == "sendImage" || canonical == "sendFile" {
		resolved, resolveErr := resolveMediaPayload(ctx, canonical, rawData, s.mediaDirectory())
		if resolveErr != nil {
			return 400, "errors.invalid_param", map[string]any{"detail": resolveErr.Error()}, resolveErr
		}
		rawData = resolved
	}

	callCtx, cancel := context.WithTimeout(ctx, s.requestTimeout())
	defer cancel()
	if err := s.pacer.Pace(callCtx, canonical, rawData); err != nil {
		code, key, result := s.mapCallError(err)
		return code, key, result, err
	}

	caller = s.currentCaller()
	if caller == nil || !callerConnected(caller) {
		return 401, "errors.account_not_logged_in", map[string]any{}, client.ErrDisconnected
	}
	nativeResult, callErr := caller.Call(callCtx, canonical, rawData)
	// Count actions that reached native execution, including native failures.
	s.sendCount.Add(1)
	if callErr != nil {
		code, key, result := s.mapCallError(callErr)
		return code, key, result, callErr
	}
	if len(nativeResult) == 0 || string(nativeResult) == "null" {
		return 200, "errors.success", map[string]any{}, nil
	}
	return 200, "errors.success", nativeResult, nil
}

func (s *Server) newResponse(code int, key string, result any, args ...any) Response {
	if result == nil {
		result = map[string]any{}
	}
	return Response{
		Code: code, Msg: s.text(key, args...), Result: result,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (s *Server) mapCallError(err error) (int, string, any) {
	code := 502
	key := "errors.ipc_error"
	var result any = map[string]any{"detail": err.Error()}
	var native *client.BridgeError
	switch {
	case errors.As(err, &native):
		code = native.Code
		if code < 400 || code > 599 {
			switch native.Status {
			case -1, 1:
				code = 400
			case 3:
				code = 401
			case 4, 404:
				code = 404
			case 5, 501:
				code = 501
			case 408, 504:
				code = 504
			default:
				code = 500
			}
		}
		switch code {
		case 400, 422:
			key = "errors.invalid_param"
		case 401:
			key = "errors.account_not_logged_in"
		case 404:
			key = "errors.contact_not_found"
		case 429:
			key = "errors.rate_limit"
		case 501:
			key = "errors.unsupported_action"
		case 408, 504:
			key = "errors.timeout"
		}
		if len(native.Result) != 0 && string(native.Result) != "null" {
			result = native.Result
		}
	case errors.Is(err, ErrDuplicateThrottled):
		code, key = 429, "errors.duplicate_throttled"
	case errors.Is(err, client.ErrBusy):
		code, key = 429, "errors.rate_limit"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		code, key = 504, "errors.timeout"
		result = map[string]any{"outcome": "unknown", "detail": err.Error()}
	case errors.Is(err, client.ErrDisconnected):
		result = map[string]any{"outcome": "unknown", "detail": err.Error()}
	}
	return code, key, result
}
