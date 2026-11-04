package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"wxbot/pkg/config"
)

func TestDispatchActionSharesPolicyMediaPacingAndIPC(t *testing.T) {
	mediaBytes := []byte("dispatch media")
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write(mediaBytes)
	}))
	defer mediaServer.Close()

	var calls atomic.Int32
	var gotAction string
	var gotData json.RawMessage
	s := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		calls.Add(1)
		gotAction = action
		gotData = append(json.RawMessage(nil), data.(json.RawMessage)...)
		return json.RawMessage(`{"sent":true}`), nil
	}}, "")
	root := t.TempDir()
	cfg := validConfig("en-US")
	cfg.Policy = config.PolicyConfig{
		MediaDir: "data/media", RateLimitEnabled: true, AntiLoopWindowSec: 60,
	}
	s.SetConfig(cfg, root)

	code, key, result, err := s.DispatchAction(context.Background(), "send_image", json.RawMessage(`{"wxid":"friend","img_path":"`+mediaServer.URL+`/image"}`))
	if err != nil || code != 200 || key != "errors.success" || string(result.(json.RawMessage)) != `{"sent":true}` {
		t.Fatalf("dispatch returned %d/%s, result=%v, err=%v", code, key, result, err)
	}
	if gotAction != "sendImage" {
		t.Fatalf("native action %q, want canonical sendImage", gotAction)
	}
	var fields map[string]string
	if err := json.Unmarshal(gotData, &fields); err != nil {
		t.Fatal(err)
	}
	mediaPath := fields["img_path"]
	if filepath.Ext(mediaPath) != ".gif" || !strings.HasPrefix(mediaPath, filepath.Join(root, "data", "media", "tmp")) {
		t.Fatalf("media was not resolved under configured media directory: %q", mediaPath)
	}
	gotMedia, err := os.ReadFile(mediaPath)
	if err != nil || string(gotMedia) != string(mediaBytes) {
		t.Fatalf("resolved media content %q, err=%v", gotMedia, err)
	}

	// The second identical outgoing message is rejected by the shared pacer.
	code, key, _, err = s.DispatchAction(context.Background(), "sendText", json.RawMessage(`{"wxid":"friend","msg":"repeat"}`))
	if err != nil || code != 200 {
		t.Fatalf("first paced action returned %d/%s, err=%v", code, key, err)
	}
	code, key, _, err = s.DispatchAction(context.Background(), "sendText", json.RawMessage(`{"wxid":"friend","msg":"repeat"}`))
	if err == nil || code != 429 || key != "errors.duplicate_throttled" {
		t.Fatalf("duplicate was not paced: %d/%s err=%v", code, key, err)
	}
	if calls.Load() != 2 || s.sendCount.Load() != 2 {
		t.Fatalf("unexpected native dispatch counts: calls=%d sends=%d", calls.Load(), s.sendCount.Load())
	}
}

func TestDispatchActionMapsDeleteFriendAliasAndUserID(t *testing.T) {
	var gotAction string
	var gotParams map[string]json.RawMessage
	server := testServer(t, "en-US", fakeCaller{call: func(_ context.Context, action string, data any) (json.RawMessage, error) {
		gotAction = action
		if err := json.Unmarshal(data.(json.RawMessage), &gotParams); err != nil {
			t.Fatal(err)
		}
		return json.RawMessage(`{"deleted":true}`), nil
	}}, "")

	_, _, _, err := server.DispatchAction(context.Background(), "delete_friend", json.RawMessage(`{"user_id":"wxid_target"}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotAction != "delFriend" || rawParamString(gotParams, "wxid") != "wxid_target" {
		t.Fatalf("delete_friend dispatch = %q %#v", gotAction, gotParams)
	}
}
