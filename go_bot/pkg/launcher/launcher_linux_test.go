//go:build linux

package launcher

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"wxbot/pkg/config"
	"wxbot/pkg/i18n"
)

func TestLockExclusiveAndReusable(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wxbot.lock")
	first, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireLock(path); err == nil {
		_ = second.Close()
		t.Fatal("concurrent runner acquired process lock")
	}
	_ = first.Close()
	second, err := acquireLock(path)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	_ = second.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("lock file should remain to preserve its inode")
	}
}

func TestLockRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target, link := filepath.Join(root, "target"), filepath.Join(root, "lock")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if file, err := acquireLock(link); err == nil {
		_ = file.Close()
		t.Fatal("symlink lock accepted")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "keep" {
		t.Fatal("symlink target was modified")
	}
}

func TestChildDoesNotInheritLock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".wxbot.lock")
	lock, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	child, err := startChild("/bin/sleep", []string{"30"}, nil, root, filepath.Join(root, "child.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer child.stop()
	_ = lock.Close()
	next, err := acquireLock(path)
	if err != nil {
		t.Fatalf("child inherited lock: %v", err)
	}
	_ = next.Close()
}

func TestWaitSocketDetectsChildExit(t *testing.T) {
	process := &childProcess{done: make(chan struct{}), err: errors.New("test exit")}
	close(process.done)
	err := waitForSocket(context.Background(), "/nonexistent/socket", process, time.Second)
	if err == nil || !strings.Contains(err.Error(), "test exit") {
		t.Fatalf("unexpected readiness error: %v", err)
	}
}

func TestRuntimeEnvironment(t *testing.T) {
	env := replaceEnv([]string{"DISPLAY=:1", "KEEP=value", "DISPLAY=:2"}, "DISPLAY", ":99")
	if strings.Join(env, ",") != "KEEP=value,DISPLAY=:99" {
		t.Fatalf("duplicate environment values: %v", env)
	}
	cfg := &config.Config{Launcher: config.LauncherConfig{Display: ":98"}}
	env = runtimeEnv(cfg, "/bridge.so", "/socket", "/logs")
	for _, want := range []string{"DISPLAY=:98", "LD_PRELOAD=/bridge.so", "WX_BRIDGE_SOCKET=/socket", "WX_BRIDGE_LIBRARY=/bridge.so", "WX_BRIDGE_LOG_DIR=/logs", "QT_XCB_GL_INTEGRATION=none"} {
		if !strings.Contains("\n"+strings.Join(env, "\n")+"\n", "\n"+want+"\n") {
			t.Errorf("missing environment setting %s", want)
		}
	}
}

func testCatalog(t *testing.T) *i18n.Catalog {
	t.Helper()
	root := os.Getenv("WX_LAUNCHER_TEST_ROOT")
	if root == "" {
		root = filepath.Join("..", "..", "..")
	}
	catalog, err := i18n.Load(root, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func fakeBridge(t *testing.T, socket string, reply func() string) {
	t.Helper()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	group.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			group.Go(func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				if _, err := bufio.NewReader(conn).ReadString('\n'); err == nil {
					_, _ = io.WriteString(conn, reply()+"\n")
				}
			})
		}
	})
	t.Cleanup(func() { _ = listener.Close(); group.Wait() })
}

func TestEnsureRuntimeReusesBridgeAndReleasesLock(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "bridge.sock")
	t.Setenv("WX_BRIDGE_SOCKET", socket)
	cfg.Launcher.QRPort = 0
	cfg.Launcher.BridgeLib = "/missing-but-not-needed-when-reusing.so"
	fakeBridge(t, socket, func() string { return `{"id":1,"status":0,"data":{"wxid":"wxid_test"}}` })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := EnsureRuntime(ctx, cfg, testCatalog(t)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".wxbot.lock")
	if lock, err := acquireLock(path); err == nil {
		_ = lock.Close()
		t.Fatal("runtime did not retain runner lock")
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for {
		if lock, err := acquireLock(path); err == nil {
			_ = lock.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runtime did not release lock on cancellation")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestLoginTimeoutClosesQRPreview(t *testing.T) {
	cfg, err := config.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(cfg.RootDir, "bridge.sock")
	fakeBridge(t, socket, func() string { return `{"id":1,"status":0,"data":{"wxid":""}}` })
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()
	cfg.Launcher.QRBind, cfg.Launcher.QRPort = "127.0.0.1", port
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = waitForLogin(ctx, cfg, testCatalog(t), socket)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected login timeout: %v", err)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("QR preview listener leaked after timeout: %v", err)
	}
	_ = listener.Close()
}

func TestLoginSuccessClosesQRPreview(t *testing.T) {
	cfg, err := config.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // Keep login-state tests independent of desktop helpers.
	socket := filepath.Join(cfg.RootDir, "bridge.sock")
	var calls atomic.Int32
	fakeBridge(t, socket, func() string {
		if calls.Add(1) >= 3 {
			return `{"id":1,"status":0,"data":{"wxid":"wxid_logged_in"}}`
		}
		return `{"id":1,"status":0,"data":{"wxid":""}}`
	})
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()
	cfg.Launcher.QRBind, cfg.Launcher.QRPort = "127.0.0.1", port
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForLogin(ctx, cfg, testCatalog(t), socket); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("QR preview listener leaked after successful login: %v", err)
	}
	_ = listener.Close()
}

func TestFailedStartupReleasesLock(t *testing.T) {
	cfg, err := config.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	socket := filepath.Join(cfg.RootDir, "bridge.sock")
	t.Setenv("WX_BRIDGE_SOCKET", socket)
	cfg.Launcher.QRPort = 0
	fakeBridge(t, socket, func() string { return `{"id":1,"status":0,"data":{"wxid":""}}` })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := EnsureRuntime(ctx, cfg, testCatalog(t)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected startup error: %v", err)
	}
	lock, err := acquireLock(filepath.Join(cfg.RootDir, ".wxbot.lock"))
	if err != nil {
		t.Fatalf("failed startup retained the runner lock: %v", err)
	}
	_ = lock.Close()
}

func TestHelperOutputBoundAndCancellation(t *testing.T) {
	output := &boundedOutput{limit: 3}
	if _, err := output.Write([]byte("abcd")); err == nil || output.Len() != 0 {
		t.Fatal("helper output bound was not enforced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runTool(ctx, ":99", "/bin/sleep", 128, "30"); err == nil {
		t.Fatal("cancelled helper ran successfully")
	}
}
