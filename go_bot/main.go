package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"wxbot/pkg/client"
	"wxbot/pkg/config"
	"wxbot/pkg/injector"
	"wxbot/pkg/llm"
	"wxbot/pkg/model"
	"wxbot/pkg/plugin"
	"wxbot/plugins"
)

func initLogger(logDir string, toConsole bool) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录 %s 失败: %w", logDir, err)
	}
	today := time.Now().Format("2006-01-02")
	logPath := filepath.Join(logDir, fmt.Sprintf("bot_%s.log", today))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件 %s 失败: %w", logPath, err)
	}
	if toConsole {
		log.SetOutput(io.MultiWriter(os.Stdout, f))
	} else {
		log.SetOutput(f)
	}
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	return f, nil
}

func detectLocalAccount() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	for _, baseDir := range []string{filepath.Join(home, "xwechat_files"), "/root/xwechat_files"} {
		entries, err := os.ReadDir(baseDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() && strings.HasPrefix(e.Name(), "wxid_") {
					lastIdx := strings.LastIndex(e.Name(), "_")
					if lastIdx > 0 {
						return e.Name()[:lastIdx]
					}
					return e.Name()
				}
			}
		}
	}
	return ""
}

func main() {
	rootDirFlag := flag.String("root", "..", "项目根目录路径")
	dllPathFlag := flag.String("dll", "", "Rust bridge library path (default: platform-specific)")
	pidFlag := flag.Uint("pid", 0, "微信进程 PID (0 表示自动检测)")
	checkFlag := flag.Bool("check", false, "仅检查配置与语法，不进行实际注入")
	flag.Parse()

	absRoot, err := filepath.Abs(*rootDirFlag)
	if err != nil {
		log.Fatalf("解析根目录失败: %v", err)
	}

	cfg, err := config.LoadConfig(absRoot)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}

	logDir := cfg.Logging.Dir
	if !filepath.IsAbs(logDir) {
		logDir = filepath.Join(absRoot, logDir)
	}
	logFile, err := initLogger(logDir, cfg.Logging.ToConsole)
	if err == nil {
		defer logFile.Close()
		log.Printf("日志系统已启动，日志目录: %s", logDir)
	} else {
		log.Printf("初始化日志目录失败: %v，将输出到控制台", err)
	}

	if *checkFlag {
		fmt.Printf("配置与插件语法检查通过 [Go 运行环境]\n")
		fmt.Printf("绑定机器人账号: %s\n", cfg.Bot.WeChatAccount)
		fmt.Printf("LLM 接口就绪: %v (BaseURL=%s, Model=%s)\n",
			cfg.LLM.APIKey != "", cfg.LLM.BaseURL, cfg.LLM.Model)
		return
	}

	bridgePath := *dllPathFlag
	if bridgePath == "" {
		bridgePath = injector.DefaultBridgePath(absRoot)
	}

	if bridgePath == "" {
		log.Fatalf("未找到 Rust bridge 库，请先编译 rust_bridge。")
	}

	pid := uint32(*pidFlag)
	if pid == 0 {
		detectedPid, err := injector.FindWeChatProcess()
		if err != nil {
			log.Fatalf("检测微信进程失败: %v。请确保微信已启动并登录。", err)
		}
		pid = detectedPid
	}

	if pid == 0 {
		log.Printf("[1/3] 使用平台 IPC 端点建立连接")
	} else {
		log.Printf("[1/3] 目标微信进程已识别 (PID=%d)", pid)
	}
	log.Printf("[2/3] 正在准备 Rust 原生桥接模块: %s ...", bridgePath)

	if err := injector.InjectDLL(pid, bridgePath); err != nil {
		log.Fatalf("Rust 桥接模块加载失败: %v", err)
	}
	log.Printf("[2/3] Rust 原生桥接模块已就绪")

	log.Printf("[3/3] 正在建立 IPC 通道...")

	var mgr *plugin.Manager

	onEvent := func(msg *model.Message) {
		log.Printf("[收到消息] 来源: %s, 会话: %s, 正文: %s", msg.Sender, msg.ChatID, msg.Text)
		if mgr != nil {
			mgr.Dispatch(msg)
		}
	}

	ipcClient, err := client.Connect(pid, onEvent)
	if err != nil {
		log.Fatalf("连接 IPC 通道失败: %v", err)
	}
	defer ipcClient.Close()

	if cfg.Bot.WeChatAccount != "" {
		ipcClient.SetSelfWxid(cfg.Bot.WeChatAccount)
	}

	account, err := ipcClient.GetAccount()
	if err == nil && account != "" {
		ipcClient.SetSelfWxid(account)
		log.Printf("当前微信账号已自动识别：%s；所有系统插件与监听通道已就绪！", account)
	} else if localAcc := detectLocalAccount(); localAcc != "" {
		ipcClient.SetSelfWxid(localAcc)
		log.Printf("当前微信账号已自动识别：%s；所有系统插件与监听通道已就绪！", localAcc)
	} else {
		log.Printf("当前已连接到微信，等待微信完成登录...")
		go func() {
			for {
				time.Sleep(1500 * time.Millisecond)
				if acc := detectLocalAccount(); acc != "" {
					ipcClient.SetSelfWxid(acc)
					log.Printf("\n==========================================================")
					log.Printf("🎉 微信登录成功！当前账号：%s", acc)
					log.Printf("所有系统插件与监听通道已激活！可输入控制台指令: status, plugins, quit")
					log.Printf("==========================================================\n")
					break
				}
			}
		}()
	}

	llmClient := llm.NewClient(cfg.LLM)
	ctx := &plugin.Context{
		Client: ipcClient,
		Config: cfg,
		LLM:    llmClient,
	}

	mgr = plugin.NewManager(ctx)
	mgr.Register(plugins.NewEchoPlugin())
	mgr.Register(plugins.NewIDResponderPlugin())
	mgr.Register(plugins.NewPersonalityPlugin())
	mgr.Register(plugins.NewMemberIdentityPlugin())
	mgr.Register(plugins.NewMentionChatPlugin())

	log.Printf("机器人主服务已启动。可输入控制台指令: status, plugins, enable <name>, disable <name>, send <target> <text>, quit")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n正在关闭机器人服务...")
		ipcClient.Close()
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])

		switch cmd {
		case "status":
			fmt.Printf("微信 PID: %d\n", pid)
			acc := ipcClient.SelfWxid()
			if acc == "" {
				acc = detectLocalAccount()
			}
			if acc != "" {
				fmt.Printf("当前账号: %s (已登录)\n", acc)
			} else {
				fmt.Printf("当前账号: 未登录\n")
			}
			fmt.Printf("管道状态: 已连接\n")
		case "plugins":
			fmt.Println("已加载插件列表:")
			for _, p := range mgr.List() {
				state := "[已启用]"
				if !p.Enabled() {
					state = "[已禁用]"
				}
				fmt.Printf("  %s %-16s %s\n", state, p.Name(), p.Description())
			}
		case "enable":
			if len(parts) < 2 {
				fmt.Println("用法: enable <插件名>")
				continue
			}
			if mgr.SetEnabled(parts[1], true) {
				fmt.Printf("插件 %s 已启用\n", parts[1])
			} else {
				fmt.Printf("未找到插件: %s\n", parts[1])
			}
		case "disable":
			if len(parts) < 2 {
				fmt.Println("用法: disable <插件名>")
				continue
			}
			if mgr.SetEnabled(parts[1], false) {
				fmt.Printf("插件 %s 已禁用\n", parts[1])
			} else {
				fmt.Printf("未找到插件: %s\n", parts[1])
			}
		case "send":
			if len(parts) < 3 {
				fmt.Println("用法: send <目标ID> <文字内容>")
				continue
			}
			target := parts[1]
			text := strings.Join(parts[2:], " ")
			err := ipcClient.SendText(target, text)
			if err != nil {
				fmt.Printf("发送失败: %v\n", err)
			} else {
				fmt.Printf("发送成功 -> %s\n", target)
			}
		case "quit", "exit":
			fmt.Println("退出服务...")
			return
		default:
			fmt.Printf("未知指令: %s。支持指令: status, plugins, enable, disable, send, quit\n", cmd)
		}
	}

	// 如果标准输入关闭（例如后台挂起或 nohup），保持进程阻塞运行，直到收到退出信号
	select {}
}
