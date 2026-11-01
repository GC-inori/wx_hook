package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQQCompatActionsReverseIDsAndIgnoreReplies(t *testing.T) {
	type nativeCall struct {
		action string
		params json.RawMessage
	}
	calls := make(chan nativeCall, 1)
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		calls <- nativeCall{action: action, params: data.(json.RawMessage)}
		return json.RawMessage(`{"sequence":123}`), nil
	}}, "")
	cfg := server.currentConfig()
	cfg.Gateway.QQCompat = true
	server.SetConfig(&cfg, t.TempDir())
	userID := server.idMapper.UserWxidToNum("wxid_destination")
	messageID := server.idMapper.MessageIDToNum("native-reply-id")
	if userID != firstUserID || messageID != firstMessageID {
		t.Fatalf("unexpected initial IDs: user=%d message=%d", userID, messageID)
	}

	status, response := request(t, server, http.MethodPost, `{"action":"send_msg","params":{"user_id":100000001,"message":[{"type":"reply","data":{"id":1000001}},{"type":"text","data":{"text":"hello"}}]}}`)
	if status != http.StatusOK || response.Retcode != 0 {
		t.Fatalf("reply segment caused send failure: status=%d response=%+v", status, response)
	}
	call := <-calls
	var params map[string]json.RawMessage
	if err := json.Unmarshal(call.params, &params); err != nil {
		t.Fatal(err)
	}
	var target, text string
	_ = json.Unmarshal(params["target"], &target)
	_ = json.Unmarshal(params["text"], &text)
	if call.action != "sendText" || target != "wxid_destination" || text != "hello" {
		t.Fatalf("QQ-compatible action was not reversed correctly: action=%s params=%s", call.action, call.params)
	}
	if response.Data.(map[string]any)["message_id"] != float64(firstMessageID+1) {
		t.Fatalf("send_msg response message ID was not mapped: %#v", response.Data)
	}

	for _, action := range []string{"get_group_root_files", "get_group_files_by_folder", "get_group_file_url"} {
		status, response = request(t, server, http.MethodPost, `{"action":"`+action+`","params":{}}`)
		data, ok := response.Data.(map[string]any)
		files, filesOK := data["files"].([]any)
		folders, foldersOK := data["folders"].([]any)
		if status != http.StatusOK || response.Retcode != 0 || !ok || !filesOK || !foldersOK || len(files) != 0 || len(folders) != 0 {
			t.Fatalf("unexpected %s response: status=%d response=%+v", action, status, response)
		}
	}
}

func TestOneBotAPIPathAndVersionInfo(t *testing.T) {
	s := testServer(t, "en-US", fakeCaller{}, "")
	for _, path := range []string{"/api", "/get_version_info"} {
		body := `{"action":"get_version_info","params":{},"echo":"version"}`
		if path != "/api" {
			body = `{}`
		}
		recorder := httptest.NewRecorder()
		s.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		var response struct {
			Status  string         `json:"status"`
			Retcode int            `json:"retcode"`
			Echo    string         `json:"echo"`
			Data    map[string]any `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || response.Status != "ok" || response.Retcode != 0 || response.Data["app_name"] != "wxbot-onebot" || response.Data["protocol_version"] != "v11" {
			t.Fatalf("unexpected response at %s: %s", path, recorder.Body.String())
		}
		if path == "/api" && response.Echo != "version" {
			t.Fatalf("echo was not preserved: %s", recorder.Body.String())
		}
	}
}
