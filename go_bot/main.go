// wxbot is the HTTP/WebSocket gateway for the native WeChat hook bridge.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"wxbot/pkg/client"
	"wxbot/pkg/config"
	"wxbot/pkg/gateway"
	"wxbot/pkg/i18n"
	"wxbot/pkg/launcher"
	"wxbot/pkg/protocol"
)

type logTee struct {
	mu      sync.Mutex
	output  io.Writer
	addLog  func(level, msg string)
	pending []byte
}

func (t *logTee) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n, err := t.output.Write(p)
	if n > 0 && t.addLog != nil {
		t.pending = append(t.pending, p[:n]...)
		for {
			newline := bytes.IndexByte(t.pending, '\n')
			if newline < 0 {
				break
			}
			line := strings.TrimSuffix(string(t.pending[:newline]), "\r")
			t.pending = append(t.pending[:0], t.pending[newline+1:]...)
			t.addLog(classifyLogLevel(line), line)
		}
	}
	return n, err
}

func classifyLogLevel(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
		return "ERROR"
	}
	if strings.Contains(lower, "warning") || strings.Contains(lower, "warn") {
		return "WARN"
	}
	return "INFO"
}

func initLogger(logDir string, logging config.LoggingConfig, api *gateway.Server) (io.WriteCloser, error) {
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return nil, err
	}
	file := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "gateway.log"),
		MaxSize:    logging.MaxSizeMB,
		MaxBackups: logging.MaxBackups,
		Compress:   logging.Compress,
	}
	writers := []io.Writer{file}
	if logging.ToConsole {
		writers = append(writers, os.Stdout)
	}
	tee := &logTee{output: io.MultiWriter(writers...)}
	if api != nil {
		tee.addLog = api.AddLog
	}
	log.SetOutput(tee)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	return file, nil
}

func runBackgroundRuntime(ctx context.Context, cfg *config.Config, catalog *i18n.Catalog, manager *launcher.LoginManager, api *gateway.Server, pid uint32) {
	for ctx.Err() == nil {
		runtimeCtx, cancelRuntime := context.WithCancel(ctx)
		runtimeDone := make(chan error, 1)
		go func() {
			runtimeDone <- launcher.EnsureRuntime(runtimeCtx, cfg, catalog, manager)
		}()

		select {
		case <-ctx.Done():
			cancelRuntime()
			<-runtimeDone
			api.SetCaller(nil)
			manager.StopOwned()
			manager.Update("stopped", "", 0)
			return
		case <-manager.RestartRequests():
			cancelRuntime()
			<-runtimeDone
			api.SetCaller(nil)
			manager.StopOwned()
			manager.Update("launching", "", 0)
			continue
		case <-manager.LogoutRequests():
			cancelRuntime()
			<-runtimeDone
			api.SetCaller(nil)
			manager.StopOwned()
			manager.Update("stopped", "", 0)
			if !waitForRestart(ctx, manager) {
				return
			}
			manager.Update("launching", "", 0)
			continue
		case err := <-runtimeDone:
			cancelRuntime()
			if err != nil {
				if ctx.Err() != nil {
					manager.Update("stopped", "", 0)
					manager.StopOwned()
					return
				}
				log.Printf("launcher runtime: %v", err)
				if manager.Snapshot().Status != "expired" {
					manager.Update("waiting_scan", "", 0)
				}
				if !pauseRuntimeRetry(ctx, configuredReconnectInterval(cfg.Bot)) {
					manager.Update("stopped", "", 0)
					manager.StopOwned()
					return
				}
				continue
			}
		}

		ipc, err := client.Connect(pid, func(event client.Event) { api.Publish(event) }, configuredReconnectInterval(cfg.Bot))
		if err != nil {
			log.Printf("connect to WeChat bridge: %v", err)
			manager.Update("waiting_scan", "", 0)
			if !pauseRuntimeRetry(ctx, configuredReconnectInterval(cfg.Bot)) {
				manager.Update("stopped", "", 0)
				manager.StopOwned()
				return
			}
			continue
		}
		wxid := manager.Snapshot().Wxid
		if cfg.Bot.WeChatAccount != "" {
			wxid = cfg.Bot.WeChatAccount
		}
		ipc.SetSelfWxid(wxid)
		api.SetCaller(ipc)
		manager.Update("logged_in", wxid, 0)
		log.Printf("WeChat bridge connected (wxid=%s)", wxid)

		disconnected := false
		for !disconnected {
			select {
			case <-ctx.Done():
				api.SetCaller(nil)
				ipc.Close()
				manager.StopOwned()
				manager.Update("stopped", "", 0)
				return
			case <-manager.RestartRequests():
				api.SetCaller(nil)
				ipc.Close()
				manager.StopOwned()
				manager.Update("launching", "", 0)
				disconnected = true
			case <-manager.LogoutRequests():
				api.SetCaller(nil)
				ipc.Close()
				manager.StopOwned()
				manager.Update("stopped", "", 0)
				if !waitForRestart(ctx, manager) {
					return
				}
				manager.Update("launching", "", 0)
				disconnected = true
			case <-time.After(configuredReconnectInterval(cfg.Bot)):
				if !ipc.Connected() {
					api.SetCaller(nil)
					ipc.Close()
					manager.Update("launching", "", 0)
					disconnected = true
				}
			}
		}
	}
	api.SetCaller(nil)
	manager.StopOwned()
	manager.Update("stopped", "", 0)
}

func configuredReconnectInterval(bot config.BotConfig) time.Duration {
	seconds := int64(bot.ReconnectIntervalSec)
	if seconds <= 0 {
		return 3 * time.Second
	}
	const maxSeconds = int64((1<<63 - 1) / int64(time.Second))
	if seconds > maxSeconds {
		return time.Duration(1<<63 - 1)
	}
	return time.Duration(seconds) * time.Second
}

func pauseRuntimeRetry(ctx context.Context, interval time.Duration) bool {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func waitForRestart(ctx context.Context, manager *launcher.LoginManager) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-manager.RestartRequests():
			return true
		case <-manager.LogoutRequests():
			// Repeated logout requests are coalesced while stopped.
		}
	}
}

func run() error {
	root := flag.String("root", ".", "Project/configuration root containing config.yaml and locales/")
	pid := flag.Uint("pid", 0, "WeChat PID (Linux Unix socket does not require a PID)")
	library := flag.String("dll", "", "Preloaded bridge path (compatibility flag; Linux uses LD_PRELOAD)")
	check := flag.Bool("check", false, "Validate configuration and locale files without connecting")
	listen := flag.String("listen", "", "Override HTTP listen address")
	locale := flag.String("locale", "", "Override response language: zh-CN or en-US")
	flag.Parse()
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		return err
	}
	cfg, err := config.LoadConfig(absRoot)
	if err != nil {
		return err
	}
	if *listen != "" {
		cfg.Gateway.Listen = *listen
	}
	if *locale != "" {
		cfg.Gateway.Locale = *locale
	}
	if *library != "" {
		cfg.Launcher.BridgeLib = *library
	}
	if err := cfg.Launcher.Validate(); err != nil {
		return err
	}
	catalog, err := i18n.Load(absRoot, cfg.Gateway.Locale)
	if err != nil {
		return err
	}
	if _, _, err := net.SplitHostPort(cfg.Gateway.Listen); err != nil {
		return fmt.Errorf("invalid gateway.listen: %w", err)
	}
	host, _, _ := net.SplitHostPort(cfg.Gateway.Listen)
	ip := net.ParseIP(host)
	if *pid > uint(^uint32(0)) {
		return errors.New("pid exceeds 32-bit range")
	}
	if *check {
		fmt.Printf("Configuration and locales valid: listen=%s locale=%s actions=%d\n", cfg.Gateway.Listen, cfg.Gateway.Locale, len(protocol.Actions))
		return nil
	}
	logDir := cfg.Logging.Dir
	if !filepath.IsAbs(logDir) {
		logDir = filepath.Join(absRoot, logDir)
	}
	manager := launcher.NewLoginManager(cfg.Launcher.Display, launcher.SocketPath(cfg), cfg.Gateway.Listen)
	api := gateway.New(nil, cfg.Gateway, catalog)
	api.SetConfig(cfg, absRoot)
	api.SetLoginController(manager)
	logfile, err := initLogger(logDir, cfg.Logging, api)
	if err != nil {
		return fmt.Errorf("initialize logging: %w", err)
	}
	defer logfile.Close()
	if cfg.Gateway.APIToken == "" && host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		log.Print("warning: gateway binding non-loopback address without API token; set gateway.api_token to restrict access")
	}
	if cfg.Bot.IpcSocket != "" && os.Getenv("WX_BRIDGE_SOCKET") == "" {
		if err := os.Setenv("WX_BRIDGE_SOCKET", cfg.Bot.IpcSocket); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr: cfg.Gateway.Listen, Handler: api,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024,
	}
	listener, err := net.Listen("tcp", cfg.Gateway.Listen)
	if err != nil {
		return err
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	log.Printf("%s HTTP=%s /api WebSocket=/ws", catalog.Text("app.ready"), listener.Addr())

	runtimeCtx, stopRuntime := context.WithCancel(ctx)
	runtimeDone := make(chan struct{})
	go func() {
		defer close(runtimeDone)
		runBackgroundRuntime(runtimeCtx, cfg, catalog, manager, api, uint32(*pid))
	}()

	var runErr error
	select {
	case err := <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	case <-ctx.Done():
		log.Print(catalog.Text("app.shutdown"))
	}
	stopRuntime()
	api.Close() // http.Server.Shutdown does not close upgraded WebSocket connections.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		if runErr == nil {
			runErr = err
		}
	}
	select {
	case <-runtimeDone:
	case <-shutdownCtx.Done():
		if runErr == nil {
			runErr = shutdownCtx.Err()
		}
	}
	return runErr
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
