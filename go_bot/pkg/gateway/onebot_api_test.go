package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
