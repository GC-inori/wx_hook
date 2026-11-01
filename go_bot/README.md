# Linux 原生 Hook 的 Go API 网关

此服务只提供低层协议网关，不再加载游戏、大模型、人格、聊天总结或插件管理器。HTTP 请求直接经进程间通信（IPC）转交 Rust 桥接，事件通过 WebSocket 推送。

## 启动

在配置根目录放置 `config.yaml`、`librust_bridge.so` 和 `locales/zh-CN.yaml`、`locales/en-US.yaml`。Go 程序默认直接管理 Xvfb、以 `LD_PRELOAD` 启动微信、等待桥接和手机登录，然后提供网关；无需启动脚本，也不依赖 Python 或外部 `flock` 命令。已连接的桥接会自动复用；正在运行但未加载桥接的微信会明确报错，不会强制结束或二次注入。

```sh
./wxbot -root /opt/wx -check
./wxbot -root /opt/wx
```

默认监听 `127.0.0.1:8080`，只允许本机访问。通过 `gateway.listen` 或 `-listen` 指定其他地址；非回环地址必须同时配置 `gateway.api_token` 或环境变量 `WX_GATEWAY_TOKEN`。建议在启用 TLS 的反向代理后提供外部服务。

```yaml
bot:
  ipc_socket: /tmp/wx_bridge.sock
launcher:
  auto_start: true
  wechat_bin: "" # 留空自动查找；也可填写 /opt/wechat/wechat。
  bridge_lib: ./librust_bridge.so
  display: ":99"
  use_xvfb: true
  xvfb_screen: 1280x1024x24
  qr_port: 8899 # 0 关闭临时二维码 HTTP 预览。
  qr_bind: 0.0.0.0
  login_timeout: 120
gateway:
  listen: 127.0.0.1:8080
  locale: zh-CN
  api_token: ""
  allowed_origins: []
  max_connections: 128
logging:
  dir: logs
  to_console: true
```

`launcher.auto_start: false` 只连接外部已启动的微信，不启动桌面、不等待登录。所有相对文件路径以 `-root` 为基准；`wechat_bin` 留空时依次查找 `/opt/wechat/wechat`、`/usr/bin/wechat`、`/opt/wechat-beta/wechat`。`-dll` 可覆盖 `launcher.bridge_lib`。Xvfb 仅在本地 X11 显示不可连接时启动；桥接和 Xvfb 的就绪等待分别由 `bridge_wait_timeout` 与 `xvfb_wait_timeout` 控制，登录上限由 `login_timeout` 控制。

二维码流程仍可调用系统运行工具 `scrot`、`zbarimg`、`qrencode`，缺失时会提示并继续等待手机确认。可选 `xdotool` 只对标题明确匹配登录对话框的窗口执行最多两次 Return。二维码 URL 变化时会刷新终端和 HTTP 图片。临时 HTTP 服务只开放 `/qrcode.png`，只发布重新生成的二维码，不发布桌面截图或配置文件；登录成功、超时、取消都会关闭。默认 `0.0.0.0:8899` 在等待登录期间免认证，建议配置 `127.0.0.1` 并通过 `ssh -L 8899:127.0.0.1:8899 root@服务器` 转发访问。

进程锁为配置根目录的 `.wxbot.lock`。启动失败会清理本次创建的 WeChat/Xvfb，已有进程不受影响；启动成功后，停止网关会保留微信会话和虚拟桌面，便于重启复用。`logs/wechat.log`、`logs/xvfb.log` 用于排查启动错误。旧启动脚本仅保留为调用同一可执行文件的兼容入口，没有独立启动逻辑；旧的脚本环境变量应迁移到 `launcher` 配置。

`WX_BRIDGE_SOCKET` 优先于配置中的套接字路径。`gateway.locale` 或 `-locale` 可选择 `zh-CN`、`en-US`；启动时会检查两份语言文件，缺失或损坏会明确报错。旧版应用插件的配置字段可以保留，但不再加载或执行。

## HTTP API (OneBot 11 标准规范)

网关遵循 OneBot 11 标准规范。支持根端点 Action 请求与路径直接调用：

1. **根端点调用 (`POST /` 或 `POST /api`)**：
```sh
curl -X POST http://127.0.0.1:8080/ \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer YOUR_TOKEN' \
  -d '{
    "action": "send_group_msg",
    "params": {
      "group_id": "12345678@chatroom",
      "message": "Hello from OneBot 11! [CQ:image,file=https://example.com/pic.jpg]"
    },
    "echo": "req-1"
  }'
```

2. **路径直接调用 (`POST /<action>`)**：
```sh
curl -X POST http://127.0.0.1:8080/send_msg \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer YOUR_TOKEN' \
  -d '{"user_id":"filehelper","message":"Hello OneBot 11"}'
```

未设置令牌时可省略 `Authorization`。令牌同时保护 HTTP、健康检查和 WebSocket 握手；不接受 URL 查询参数中的令牌。

响应统一遵循 OneBot 11 标准信封结构（HTTP 状态码固定为 200）：

```json
{
  "status": "ok",
  "retcode": 0,
  "data": {"message_id": 123456},
  "message": "",
  "wording": "",
  "echo": "req-1"
}
```

支持的标准 OneBot 11 动作包括：

| 类别 | 动作 |
| --- | --- |
| 消息收发 | `send_msg`, `send_group_msg`, `send_private_msg`, `delete_msg`, `get_msg` |
| 信息查询 | `get_login_info`, `get_friend_list`, `get_group_list`, `get_group_member_list`, `get_group_member_info`, `get_status`, `get_version_info` |
| 群组管理 | `set_group_kick`, `set_group_leave`, `set_friend_add_request`, `set_group_add_request` |
| 微信扩展 | `send_xml`, `send_card`, `send_applet`, `query_obj`, `set_read_status` |

消息字段（`message`）同时兼容 **CQ 码文本**（如 `[CQ:at,qq=...]`、`[CQ:image,file=...]`）与 **OneBot 消息段数组**（Message Segment Array）。媒体图片与文件支持本地绝对路径、网络 `http(s)://` URL（自动下载）及 `base64://` 编码。

`GET /healthz` 检查 IPC 连接状态，并报告网关 IPC 事件队列的累计丢弃数量 `dropped_events`。此检查不代表微信已登录或原生接口均已可用。

## WebSocket (OneBot 11 事件流与全双工调用)

连接 `/ws` 即可接入标准的 OneBot 11 实时事件流：

```json
{
  "time": 1730000000,
  "self_id": "wxid_self",
  "post_type": "message",
  "message_type": "group",
  "sub_type": "normal",
  "message_id": 123456,
  "group_id": "12345678@chatroom",
  "user_id": "wxid_sender",
  "message": [{"type": "text", "data": {"text": "Hello"}}],
  "raw_message": "Hello",
  "font": 0,
  "sender": {"user_id": "wxid_sender", "nickname": "SenderName", "card": ""}
}
```

- **全双工 Action 交互**：在 `/ws` 连接中直接发送 `{"action": "...", "params": {...}, "echo": "..."}` 即可调用接口，并在相同连接上原路接收对应 `echo` 的响应。
- **反向 WebSocket 客户端**：在 `endpoints` 中配置类型为 `wsClient` 的端点，网关会自动向外部 OneBot 服务端（NoneBot2、AstrBot、Koishi 等）发起长连接并自动重连。

默认只允许相同主机的浏览器来源，额外来源必须写入 `gateway.allowed_origins` 精确白名单。每个 WebSocket 连接默认最多排队 256 个事件和动作，可通过 `gateway.ws_queue_size` 调整；慢连接会被关闭。IPC 事件队列上限为 2048，满时丢弃并计数，不产生额外 goroutine。此实时流不提供持久化、重放或恰好一次投递保证；重连后调用方应主动核对状态。

## 超时与并发

- HTTP 和 IPC 各限制最多 128 个进行中的请求；超出返回 429。
- 单个 IPC 调用最多 7 秒，覆盖原生主线程 5 秒调度保护及传输开销；写入超时最多 2 秒，支持请求取消。
- 默认请求正文上限为 3 MiB，可通过 `gateway.max_request_body_mb` 调整；超限返回 413。
- HTTP/WebSocket 桥接动作超时默认 7 秒，可通过 `gateway.request_timeout_sec` 调整。
- IPC 断开时立即使待处理请求失败并在后台重新连接，**不会自动重试有副作用的操作**。
- 调用超时、取消或连接断开后，操作可能已经执行；响应中的 `outcome: unknown` 表示需要人工或上层查询核对，避免重复发送和支付操作。
- SIGINT/SIGTERM 触发平滑退出，主动关闭升级后的 WebSocket，再等待 HTTP 请求结束。

## 本地验证和构建

以下命令均在开发者 Windows 电脑上执行：

```powershell
cd E:\副业\wx_hook\go_bot
go test -race ./...
go vet ./...
go run main.go -root E:\副业\wx_hook -check
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -ldflags "-s -w" -o wxbot_linux main.go
go build -ldflags "-s -w" -o E:\副业\wx_hook\dist_linux\wxbot main.go
```

Linux 换行 JSON 编解码器独立于平台，Windows 本地测试同样覆盖其分帧、连续包、大小限制和不完整输入。测试还覆盖 33 个动作转发、64 位 ID 保真、乱序并发回执、错误透传、取消、断线、队列限制、中英文提示、HTTP 校验、认证、WebSocket 事件、来源限制和退出。远程机器仅接收已构建制品用于运行测试。
