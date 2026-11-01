//go:build linux

package launcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"wxbot/pkg/config"
	"wxbot/pkg/i18n"
)

// EnsureRuntime holds the process lock until ctx is cancelled. Any Xvfb/WeChat
// processes created here are registered with the login manager after successful
// login and stopped by it during restart, logout, or gateway shutdown. On startup
// failure only processes created by this call are stopped; reused processes remain untouched.
func EnsureRuntime(ctx context.Context, cfg *config.Config, catalog *i18n.Catalog, managers ...*LoginManager) (err error) {
	var manager *LoginManager
	if len(managers) != 0 {
		manager = managers[0]
	}
	if manager != nil {
		manager.Update("launching", "", 0)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cfg.Launcher.Validate(); err != nil {
		return err
	}
	lock, err := acquireLock(filepath.Join(cfg.RootDir, ".wxbot.lock"))
	if err != nil {
		return fmt.Errorf("%s: %w", catalog.Text("launcher.lock_failed"), err)
	}
	success := false
	var owned []*childProcess
	defer func() {
		if !success {
			for i := len(owned) - 1; i >= 0; i-- {
				owned[i].stop()
			}
			_ = lock.Close()
		}
	}()
	if cfg.Launcher.AutoStart {
		socket := socketPath(cfg)
		if !socketAlive(ctx, socket) {
			if err := rejectExistingWeChat(); err != nil {
				return err
			}
			library := rootPath(cfg.RootDir, cfg.Launcher.BridgeLib)
			if strings.ContainsAny(library, " \t\r\n:") {
				return errors.New("resolved launcher.bridge_lib contains an LD_PRELOAD separator")
			}
			info, statErr := os.Stat(library)
			if statErr != nil {
				return fmt.Errorf("bridge library %s: %w", library, statErr)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("bridge library %s is not a regular file", library)
			}
			wechat, findErr := findWeChat(cfg)
			if findErr != nil {
				return findErr
			}
			logDir := rootPath(cfg.RootDir, cfg.Logging.Dir)
			if err := os.MkdirAll(logDir, 0750); err != nil {
				return err
			}
			display, _ := cfg.Launcher.DisplayNumber()
			xSocket := fmt.Sprintf("/tmp/.X11-unix/X%d", display)
			if !socketAlive(ctx, xSocket) {
				if !cfg.Launcher.UseXvfb {
					return fmt.Errorf("display %s unavailable and launcher.use_xvfb is false", cfg.Launcher.Display)
				}
				log.Print(catalog.Text("launcher.start_xvfb", cfg.Launcher.Display))
				// Xvfb takes a display number, not the optional screen suffix.
				xvfb, startErr := startChild("Xvfb", []string{fmt.Sprintf(":%d", display), "-screen", "0", cfg.Launcher.XvfbScreen, "-nolisten", "tcp"}, nil, cfg.RootDir, filepath.Join(logDir, "xvfb.log"))
				if startErr != nil {
					return fmt.Errorf("start Xvfb: %w", startErr)
				}
				owned = append(owned, xvfb)
				if err := waitForSocket(ctx, xSocket, xvfb, durationSeconds(cfg.Launcher.XvfbWaitTimeout)); err != nil {
					return fmt.Errorf("wait for Xvfb (see %s): %w", filepath.Join(logDir, "xvfb.log"), err)
				}
			}
			env := runtimeEnv(cfg, library, socket, logDir)
			log.Print(catalog.Text("launcher.start_wechat", wechat))
			process, startErr := startChild(wechat, nil, env, cfg.RootDir, filepath.Join(logDir, "wechat.log"))
			if startErr != nil {
				return fmt.Errorf("start WeChat: %w", startErr)
			}
			owned = append(owned, process)
			if manager != nil {
				manager.Update("launching", "", process.cmd.Process.Pid)
			}
			if err := waitForSocket(ctx, socket, process, durationSeconds(cfg.Launcher.BridgeWaitTimeout)); err != nil {
				return fmt.Errorf("wait for WeChat bridge (see %s): %w", filepath.Join(logDir, "wechat.log"), err)
			}
		} else {
			log.Print(catalog.Text("launcher.reuse"))
		}
		if err := waitForLogin(ctx, cfg, catalog, socket, manager); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	success = true
	if manager != nil && len(owned) != 0 {
		processes := append([]*childProcess(nil), owned...)
		manager.setOwnedStop(func() {
			for i := len(processes) - 1; i >= 0; i-- {
				processes[i].stop()
			}
		})
	}
	// Cancellation, rather than GC or process inheritance, defines lock lifetime.
	go func() { <-ctx.Done(); _ = lock.Close() }()
	return nil
}

func acquireLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("lock path must be a regular file: %s", path)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, err
	}
	// Never unlink the lock file: doing so permits separate locked inodes.
	return file, nil
}

func rootPath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, path)
}

func socketPath(cfg *config.Config) string { return SocketPath(cfg) }

func runtimeEnv(cfg *config.Config, library, socket, logs string) []string {
	env := replaceEnv(os.Environ(), "DISPLAY", cfg.Launcher.Display)
	env = replaceEnv(env, "LD_PRELOAD", library)
	env = replaceEnv(env, "QT_XCB_GL_INTEGRATION", "none")
	env = replaceEnv(env, "WX_BRIDGE_SOCKET", socket)
	env = replaceEnv(env, "WX_BRIDGE_LIBRARY", library)
	return replaceEnv(env, "WX_BRIDGE_LOG_DIR", logs)
}

func replaceEnv(env []string, name, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, item := range env {
		if !strings.HasPrefix(item, name+"=") {
			result = append(result, item)
		}
	}
	return append(result, name+"="+value)
}

func findWeChat(cfg *config.Config) (string, error) {
	candidates := []string{cfg.Launcher.WeChatBin}
	if candidates[0] == "" {
		candidates = []string{"/opt/wechat/wechat", "/usr/bin/wechat", "/opt/wechat-beta/wechat"}
	}
	for _, path := range candidates {
		path = rootPath(cfg.RootDir, path)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("WeChat executable not found; set launcher.wechat_bin (checked %v)", candidates)
}

func rejectExistingWeChat() error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Errorf("inspect existing WeChat processes: %w", err)
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		name, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err == nil && strings.EqualFold(strings.TrimSpace(string(name)), "wechat") {
			return errors.New("WeChat is already running without a reachable bridge; stop it before starting with LD_PRELOAD")
		}
	}
	return nil
}

type childProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error // Written before closing done; readers must first observe done.
}

func startChild(name string, args, env []string, root, logPath string) (*childProcess, error) {
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = root, env, file, file
	// Separate process groups survive Ctrl+C to the foreground gateway. exec
	// closes all non-standard descriptors, including the runner's flock.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	process := &childProcess{cmd: cmd, done: make(chan struct{})}
	go func() { process.err = cmd.Wait(); close(process.done) }()
	return process, nil
}

func (p *childProcess) stop() {
	_ = unix.Kill(-p.cmd.Process.Pid, unix.SIGTERM)
	select {
	case <-p.done:
		return
	default:
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
		_ = unix.Kill(-p.cmd.Process.Pid, unix.SIGKILL)
		<-p.done
	}
}

func socketAlive(ctx context.Context, path string) bool {
	probe, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probe, "unix", path)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func durationSeconds(value int) time.Duration {
	if value <= 0 {
		return 0
	}
	const maxSeconds = int64((1<<63 - 1) / int64(time.Second))
	seconds := int64(value)
	if seconds > maxSeconds {
		seconds = maxSeconds
	}
	return time.Duration(seconds) * time.Second
}

func waitForSocket(ctx context.Context, path string, process *childProcess, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-process.done:
			return fmt.Errorf("process exited before readiness: %v", process.err)
		default:
		}
		if socketAlive(ctx, path) {
			return nil
		}
		if err := waitInterval(ctx, 100*time.Millisecond); err != nil {
			return err
		}
	}
}

func waitForLogin(ctx context.Context, cfg *config.Config, catalog *i18n.Catalog, socket string, managers ...*LoginManager) error {
	var manager *LoginManager
	if len(managers) != 0 {
		manager = managers[0]
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Launcher.LoginTimeout)*time.Second)
	defer cancel()
	if wxid, err := probeLogin(ctx, socket); err == nil && wxid != "" {
		if manager != nil {
			manager.Update("logged_in", wxid, 0)
		}
		log.Print(catalog.Text("launcher.logged_in", wxid))
		return nil
	}
	if manager != nil {
		manager.Update("waiting_scan", "", 0)
	}
	log.Print(catalog.Text("launcher.wait_login", cfg.Launcher.LoginTimeout))
	var preview *qrPreview
	if cfg.Launcher.QRPort != 0 {
		var err error
		preview, err = startQRPreview(cfg.Launcher.QRBind, cfg.Launcher.QRPort)
		if err != nil {
			return fmt.Errorf("start QR preview: %w", err)
		}
		defer preview.close()
		log.Print(catalog.Text("launcher.qr_preview", preview.addr))
	}
	qr, err := newQRCapture(cfg.Launcher.Display, catalog)
	if err != nil {
		return err
	}
	defer qr.close()
	started := time.Now()
	var refreshRequests <-chan struct{}
	if manager != nil {
		refreshRequests = manager.RefreshRequests()
	}
	for {
		select {
		case <-refreshRequests:
			qr.last = ""
			qr.visible = false
		default:
		}
		if err := ctx.Err(); err != nil {
			if manager != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					manager.Update("expired", "", 0)
				} else {
					manager.Update("stopped", "", 0)
				}
			}
			return fmt.Errorf("%s: %w", catalog.Text("launcher.login_timeout"), err)
		}
		wxid, err := probeLogin(ctx, socket)
		if err == nil && wxid != "" {
			if manager != nil {
				manager.Update("logged_in", wxid, 0)
			}
			log.Print(catalog.Text("launcher.logged_in", wxid))
			return nil
		}
		qr.refresh(ctx, preview, time.Since(started), manager)
		if err := waitInterval(ctx, time.Second); err != nil {
			if manager != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					manager.Update("expired", "", 0)
				} else {
					manager.Update("stopped", "", 0)
				}
			}
			return fmt.Errorf("%s: %w", catalog.Text("launcher.login_timeout"), err)
		}
	}
}

// Commands have bounded output and deadlines. No shell expansion or inheritance
// of the bridge's LD_PRELOAD into screenshot/QR helpers is permitted.
type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.Len() {
		return 0, errors.New("helper output exceeds size limit")
	}
	return b.Buffer.Write(data)
}

func runTool(ctx context.Context, display, name string, limit int, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = replaceEnv(replaceEnv(os.Environ(), "DISPLAY", display), "LD_PRELOAD", "")
	output := &boundedOutput{limit: limit}
	cmd.Stdout, cmd.Stderr = output, io.Discard
	cmd.WaitDelay = 250 * time.Millisecond
	err := cmd.Run()
	return output.Bytes(), err
}

type qrCapture struct {
	display string
	dir     string
	last    string
	visible bool
	tools   bool
	returns int
	catalog *i18n.Catalog
}

func newQRCapture(display string, catalog *i18n.Catalog) (*qrCapture, error) {
	dir, err := os.MkdirTemp("", "wx-login-")
	if err != nil {
		return nil, err
	}
	qr := &qrCapture{display: display, dir: dir, tools: true, catalog: catalog}
	for _, tool := range []string{"scrot", "zbarimg", "qrencode"} {
		if _, err := exec.LookPath(tool); err != nil {
			qr.tools = false
			log.Print(catalog.Text("launcher.qr_missing_tool", tool))
		}
	}
	return qr, nil
}

func (q *qrCapture) close() { _ = os.RemoveAll(q.dir) }

func (q *qrCapture) refresh(ctx context.Context, preview *qrPreview, elapsed time.Duration, manager *LoginManager) {
	// Limit automatic Return to two early attempts on windows explicitly named
	// as login dialogs; never send a global keypress into an arbitrary desktop.
	if q.last == "" && elapsed >= 2*time.Second && elapsed < 8*time.Second && q.returns < 2 {
		q.returns++
		ids, err := runTool(ctx, q.display, "xdotool", 4096, "search", "--onlyvisible", "--name", "登录|Log ?[Ii]n")
		if err == nil {
			if windows := strings.Fields(string(ids)); len(windows) > 0 {
				if _, err := strconv.ParseUint(windows[0], 10, 64); err == nil {
					_, _ = runTool(ctx, q.display, "xdotool", 4096, "key", "--window", windows[0], "Return")
				}
			}
		}
	}
	if !q.tools || ctx.Err() != nil {
		return
	}
	capturePath := filepath.Join(q.dir, "capture.png")
	if _, err := runTool(ctx, q.display, "scrot", 4096, "-o", capturePath); err != nil {
		q.markQRDisappeared(manager)
		return
	}
	if info, err := os.Stat(capturePath); err != nil || info.Size() > maxQRBytes {
		q.markQRDisappeared(manager)
		return
	}
	decoded, err := runTool(ctx, q.display, "zbarimg", 16384, "-q", "--raw", capturePath)
	if err != nil {
		q.markQRDisappeared(manager)
		return
	}
	url := strings.TrimSpace(strings.SplitN(string(decoded), "\n", 2)[0])
	if url == "" || len(url) > 8192 {
		q.markQRDisappeared(manager)
		return
	}
	q.visible = true
	if url == q.last {
		q.visible = true
		if manager != nil {
			manager.Update("waiting_scan", "", 0)
		}
		return
	}
	// Publish a regenerated QR, never the desktop screenshot (which might
	// contain account data when login completes concurrently with capture).
	png, err := runTool(ctx, q.display, "qrencode", maxQRBytes, "-t", "PNG", "-o", "-", "--", url)
	if err != nil || len(png) < 8 || !bytes.Equal(png[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return
	}
	if preview != nil {
		preview.image.Store(&qrImage{data: png})
	}
	if manager != nil {
		manager.SetQRCode(png)
		manager.Update("waiting_scan", "", 0)
	}
	terminal, err := runTool(ctx, q.display, "qrencode", 128*1024, "-t", "UTF8", "--", url)
	if err == nil {
		log.Printf("%s\n%s", q.catalog.Text("launcher.qr_ready"), terminal)
	}
	q.last = url
}

func (q *qrCapture) markQRDisappeared(manager *LoginManager) {
	if q.last != "" && q.visible {
		q.visible = false
		if manager != nil {
			manager.Update("scanned", "", 0)
		}
	}
}
