package launcher

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestQueryLogin(t *testing.T) {
	for _, tc := range []struct {
		name, reply, wxid string
		wantErr           bool
	}{
		{"data", `{"id":1,"status":0,"data":{"wxid":"wxid_a"}}`, "wxid_a", false},
		{"result", `{"id":1,"status":0,"result":{"wxid":"wxid_b"}}`, "wxid_b", false},
		{"event before reply", "{\"event\":\"recv_msg\",\"data\":{}}\n" + `{"id":1,"status":0,"data":{"wxid":"wxid_a"}}`, "wxid_a", false},
		{"null result", `{"id":1,"status":0,"result":null,"data":{"wxid":"wxid_a"}}`, "wxid_a", false},
		{"not logged in", `{"id":1,"status":0,"data":{"wxid":""}}`, "", false},
		{"empty account", `{"id":1,"status":0,"data":{"wxid":"  "}}`, "", false},
		{"native failure", `{"id":1,"status":-1,"data":{"wxid":"wxid_fake"}}`, "", true},
		{"missing status", `{"id":1,"data":{"wxid":"wxid_fake"}}`, "", true},
		{"malformed", "broken JSON", "", true},
		{"wrong data type", `{"id":1,"status":0,"data":[]}`, "", true},
		{"unrelated responses", strings.Repeat("{\"id\":2,\"status\":0}\n", 128), "", true},
		{"oversized", strings.Repeat("x", maxIPCBytes+1), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, peer := net.Pipe()
			defer conn.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer peer.Close()
				line, err := bufio.NewReader(peer).ReadString('\n')
				if err == nil && !strings.Contains(line, `"get_self_info"`) {
					t.Error("unexpected login action")
				}
				_, _ = io.WriteString(peer, tc.reply+"\n")
			}()
			wxid, err := queryLogin(context.Background(), conn)
			_ = conn.Close()
			<-done
			if wxid != tc.wxid || (err != nil) != tc.wantErr {
				t.Fatalf("wxid=%q err=%v; want wxid=%q error=%v", wxid, err, tc.wxid, tc.wantErr)
			}
		})
	}
}

func TestQueryLoginCancellation(t *testing.T) {
	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := queryLogin(ctx, conn); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled probe succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("login probe did not respond to cancellation")
	}
}

func TestQRPreviewRoutesAndLifecycle(t *testing.T) {
	p, err := startQRPreview("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodGet, "/", 404},
		{http.MethodGet, "/config.yaml", 404},
		{http.MethodGet, "/../config.yaml", 404},
		{http.MethodPost, "/qrcode.png", 405},
		{http.MethodGet, "/qrcode.png", 503},
	} {
		w := httptest.NewRecorder()
		p.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s %s: %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
	p.image.Store(&qrImage{data: []byte("png-content")})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		p.ServeHTTP(w, httptest.NewRequest(method, "/qrcode.png", nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("invalid PNG response: %v", w.Result())
		}
		if method == http.MethodHead && w.Body.Len() != 0 {
			t.Fatal("HEAD wrote an image body")
		}
	}
	response, err := (&http.Client{Timeout: time.Second}).Get("http://" + p.addr + "/qrcode.png")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(body) != "png-content" {
		t.Fatalf("unexpected network response: %q %v", body, err)
	}
}

func TestQRPreviewConcurrentRefresh(t *testing.T) {
	p := &qrPreview{}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Go(func() {
			for j := 0; j < 100; j++ {
				p.image.Store(&qrImage{data: []byte(fmt.Sprintf("image-%d", j))})
				p.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/qrcode.png", nil))
			}
		})
	}
	group.Wait()
}

func TestQRPreviewClosesListener(t *testing.T) {
	p, err := startQRPreview("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	p.image.Store(&qrImage{data: []byte("image")})
	p.close()
	if p.image.Load() != nil {
		t.Fatal("image retained after shutdown")
	}
	conn, err := net.DialTimeout("tcp", p.addr, 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatal("preview listener remained open")
	}
}

func TestQRListenerBoundsConnections(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	limited := &qrListener{Listener: base, slots: make(chan struct{}, 1), done: make(chan struct{})}
	defer limited.Close()
	client, err := net.Dial("tcp", base.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	accepted, err := limited.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	done := make(chan error, 1)
	go func() { _, err := limited.Accept(); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("connection limit failed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	_ = limited.Close()
	select {
	case err := <-done:
		if err != net.ErrClosed {
			t.Fatalf("expected closed listener, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("closing a full listener did not unblock Accept")
	}
	_ = accepted.Close() // A repeated Close must not release the capacity twice.
}

func TestWaitIntervalCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitInterval(ctx, time.Hour); err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
