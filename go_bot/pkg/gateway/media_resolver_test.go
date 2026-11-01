package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveMediaPayloadDownloadsHTTPFile(t *testing.T) {
	want := []byte("mock image bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(want)
	}))
	defer server.Close()

	mediaDir := t.TempDir()
	resolved, err := resolveMediaPayload(context.Background(), "sendImage", json.RawMessage(`{"img_path":"`+server.URL+`/image"}`), mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(resolved, &fields); err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(fields["img_path"]) != ".png" || !strings.HasPrefix(fields["img_path"], filepath.Join(mediaDir, "tmp")) {
		t.Fatalf("unexpected resolved path %q", fields["img_path"])
	}
	got, err := os.ReadFile(fields["img_path"])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("downloaded bytes %q, want %q", got, want)
	}
}

func TestResolveMediaPayloadDecodesBase64(t *testing.T) {
	want := []byte("base64 image bytes")
	encoded := base64.StdEncoding.EncodeToString(want)
	for _, tc := range []struct {
		name, value, extension string
	}{
		{name: "base64 scheme", value: "base64://" + encoded, extension: ".txt"},
		{name: "data URI", value: "data:image/jpeg;base64," + encoded, extension: ".jpg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mediaDir := t.TempDir()
			input, _ := json.Marshal(map[string]string{"file_path": tc.value})
			resolved, err := resolveMediaPayload(context.Background(), "send_file", input, mediaDir)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]string
			if err := json.Unmarshal(resolved, &fields); err != nil {
				t.Fatal(err)
			}
			if filepath.Ext(fields["file_path"]) != tc.extension {
				t.Fatalf("extension %q, want %q", filepath.Ext(fields["file_path"]), tc.extension)
			}
			got, err := os.ReadFile(fields["file_path"])
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("decoded bytes %q, want %q", got, want)
			}
		})
	}
}

func TestResolveMediaPayloadErrorsAndLocalPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	for _, value := range []string{server.URL + "/missing", "base64://%%%"} {
		if _, err := resolveMediaPayload(context.Background(), "sendImage", json.RawMessage(`{"image":"`+value+`"}`), t.TempDir()); err == nil {
			t.Errorf("expected resolver error for %q", value)
		}
	}
	local := filepath.Join(t.TempDir(), "existing.png")
	input, err := json.Marshal(map[string]string{"path": local})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveMediaPayload(context.Background(), "sendImage", input, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if string(resolved) != string(input) {
		t.Fatalf("local path payload changed: %s", resolved)
	}
}
