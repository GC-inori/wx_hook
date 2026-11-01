# WeChat Native Hook Gateway

基于 Linux 原生微信 (v4.1.15.55) 的底座挂钩与 HTTP/WebSocket 协议网关服务，附带现代化 WebUI 控制台。

## 架构概览

本项目采用模块化分层设计：

- **Rust 桥接层 (`rust_bridge/`)**：编译为 `librust_bridge.so`，利用 Linux `LD_PRELOAD` 原生挂钩机制注入微信主进程，拦截核心收发与事件逻辑，通过 Unix Domain Socket 提供高性能 IPC 通信通道。
- **Go 网关服务 (`go_bot/`)**：负责管理 Linux Xvfb 无头桌面与微信进程生命周期，对外暴露 OneBot 11 标准协议 HTTP API、WebSocket 实时事件流与 REST 管理接口，并内嵌 WebUI 前端资产。
- **现代化 WebUI (`web_ui/`)**：基于 Vue 3 + Tailwind CSS 构建，提供运行看板、微信扫码登录、网络端点管理（OneBot 11 HTTP/WS、Webhook 上报、反向 WebSocket 客户端）、出站防风控节流调度、执行日志实时终端、接口调试工作台与系统设置中心。

---

## 快速开始 (Linux)

### 1. 部署运行

在目标 Linux 机器（推荐 Debian 12 / Ubuntu 22.04+）上：

```sh
# 解压预编译归档包
tar -xzf dist_linux.tar.gz
cd dist_linux

# 检查环境并启动服务
chmod +x wxbot librust_bridge.so
./wxbot -root .
```

### 2. 访问控制台

网关默认启动并在 `http://127.0.0.1:8080`（或配置的 `gateway.listen`）提供 Web 控制台：

- **扫码登录**：直接在 Web 控制台识别微信二维码并确认登录。
- **运行看板**：查看系统负载、微信进程状态及实时事件流水。
- **网络配置**：配置对接到 NoneBot2、Koishi 等下游机器人的 Webhook 上报端点。
- **执行日志**：实时日志终端滚屏与级别过滤。
- **接口调试**：直接在线发送文本、图片、文件、@群成员等微信 API 请求。

---

## 配置文件 (`config.yaml`)

```yaml
bot:
  wechat_account: "" # 绑定的微信账号 wxid，留空自动获取登录账号
  ipc_socket: "/tmp/wx_bridge.sock"

launcher:
  auto_start: true # 是否由网关自动拉起 Xvfb 与微信
  wechat_bin: "" # 留空自动查找 /opt/wechat/wechat
  bridge_lib: "./librust_bridge.so"
  display: ":99"
  use_xvfb: true
  xvfb_screen: "1280x1024x24"
  qr_port: 8899 # 临时二维码 HTTP 预览端口
  qr_bind: "0.0.0.0"
  login_timeout: 120

gateway:
  listen: "127.0.0.1:8080"
  locale: "zh-CN" # zh-CN 或 en-US
  api_token: "" # API 访问密钥，留空则免认证
  allowed_origins: []
  max_connections: 128

policy:
  anti_revoke: true # 开启消息防撤回监听
  auto_agree_friend: false # 自动同意好友申请
  auto_agree_group: true # 自动同意入群邀请
  filter_gh: true # 过滤公众号推广推送
  media_dir: "data/media"
  retry_count: 3

logging:
  dir: "logs"
  level: "info"
  to_console: true
```

---

## 本地编译构建指南

详细的跨平台交叉编译与智能体分工指南参见 [agent.md](agent.md)。

- **Rust 桥接库**：`cargo zigbuild --target x86_64-unknown-linux-gnu.2.36 --release`
- **Go 机器人程序**：`$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags "-s -w" -o wxbot_linux main.go`
- **前端 WebUI**：`cd web_ui && npm run build`（产物自动输出至 `go_bot/pkg/gateway/web/dist`）
