# 项目开发规范与智能体指引 (Agent Guidelines)

本文档是 `wx_hook` 与 `napcat-bot-go` 生态协同开发的唯一指导规范。所有参与本项目的智能体与开发者必须严格遵守本规范。

---

## 一、 核心开发与构建规范

### 1. 本机编译与打包铁律 (Mandatory Local Build Policy)
- **除 `ida-mcp` 逆向分析工具外，任何语言（Rust、Go、C/C++、Python、Shell 等）的代码编写、依赖安装、交叉编译与打包构建，必须且只能在开发者本地电脑（Local Machine）完成。**
- **严禁在远程 SSH 测试机/服务器上安装编译链**（包括但不限于 `cargo`、`rustc`、`go`、`gcc`、`clang`、`llvm`、`pip` 等构建重型包），严禁在远程执行构建打包任务。远程机器性能受限，仅负责运行测试与加载最终制品。

---

### 2. 本地交叉编译与打包指南 (Local Cross-Compilation)

#### (1) Rust 桥接库 (`librust_bridge.so`)
本机已配置 `cargo-zigbuild` 与 `ziglang` 环境，直接在本地 Windows 终端执行：
```powershell
cd E:\副业\wx_hook\rust_bridge
cargo zigbuild --target x86_64-unknown-linux-gnu.2.36 --release
```
编译产物位于：
`E:\副业\wx_hook\rust_bridge\target\x86_64-unknown-linux-gnu\release\librust_bridge.so`

#### (2) Go 机器人网关程序 (`wxbot_linux`)
在本地 Windows 终端通过 Go 自带的交叉编译器执行：
```powershell
cd E:\副业\wx_hook\go_bot
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags "-s -w" -o wxbot_linux main.go
```
编译产物位于：
`E:\副业\wx_hook\go_bot\wxbot_linux`

#### (3) 完整发布包打包 (`dist_linux.tar.gz`)
每次核心二进制或配置更新后，需将产物重新归档为发布包：
```powershell
# 在根目录下通过打包脚本或临时目录将 wxbot, librust_bridge.so, config.yaml, locales 归档
tar -czf dist_linux.tar.gz -C dist_linux/ .
```

---

### 3. 上传与远程部署流程 (Upload & Run Workflow)

所有制品在本地构建并验证通过后，通过 `scp` 一键上传至远程目标机器并测试：
```powershell
# 1. 增量上传二进制与配置（快速更新）
scp E:\副业\wx_hook\go_bot\wxbot_linux root@192.168.2.100:/opt/wx/wxbot
scp E:\副业\wx_hook\config.yaml root@192.168.2.100:/opt/wx/config.yaml

# 2. 或全量上传发布包
scp E:\副业\wx_hook\dist_linux.tar.gz root@192.168.2.100:/opt/dist_linux.tar.gz

# 3. 远程仅负责权限赋予与重启服务
ssh root@192.168.2.100 "chmod +x /opt/wx/wxbot /opt/wx/librust_bridge.so; /opt/wx/wxbot -root /opt/wx"
```

---

### 4. 智能体分工与模型协作规范 (Model Assignment & Division of Labor)

- **后端代码编写 (Go / Rust)**：
  - 所有 Go (`go_bot/`, `napcat-bot-go/`) 和 Rust (`rust_bridge/`) 后端业务与底层代码的编写、改动与重构，必须交由子智能体（Subagent）执行，指定模型为 **`gpt-6-luna`**。
  - 主智能体使用 `Task` 工具（配置 `model: "gpt-6-luna"`, `subagent_type: "generalPurpose"`）派发任务并验收交付。
- **前端开发、文档编制与 IDA 逆向 (Frontend, Docs & IDA MCP)**：
  - 由主智能体直接独立编写与操作，不进行外部委派。
  - 涵盖所有 WebUI (`web_ui/`) 页面与前端组件的开发实现。
  - 涵盖所有基于 `ida-mcp` 的逆向工程分析、地址与符号提取、反编译审计及 IDA 相关脚本编写。
  - 涵盖项目整体架构设计、文档梳理与维护。

---

## 二、 项目技术文档结构规范 (Documentation Architecture)

为确保项目持续迭代时历史逆向成果不丢失、Rust 底座与 Go 网关接口对齐、上层应用解耦清晰，项目技术文档统一组织在 `docs/` 目录下，包含以下 4 份核心文档：

```
docs/
├── 00_reverse_engineering_notes.md    # 0. 逆向工程与内存布局笔记 (函数地址/交叉引用/ABI)
├── 01_rust_bridge_capabilities.md     # 1. Rust 桥接层功能与接口清单 (已实现/未实现/IPC协议)
├── 02_onebot11_go_gateway_matrix.md   # 2. OneBot 11 Go 网关功能与适配矩阵 (API/事件/ID映射)
└── 03_napcat_go_wx_universal_roadmap.md # 3. napcat-go-wx 通用版分支规划与精简指南 (解耦/精简)
```

### 文档职责划分与维护准则

1. **`00_reverse_engineering_notes.md`（逆向工程笔记）**
   - **收录内容**：微信目标版本（当前 `4.1.15.55`）的基址、RVA 偏移表、关键函数签名、虚表（VTable）结构、内存 Hook 点位置、Protobuf 结构体 tag 与交叉引用路径。
   - **维护时机**：每次通过 `ida-mcp` 挖掘出新函数、新增或校准偏移量时，必须实时更新本文档。

2. **`01_rust_bridge_capabilities.md`（Rust 底座功能清单）**
   - **收录内容**：`rust_bridge` 已支持的 IPC 请求列表、消息拦截与事件分类机制、发送通道实现状态，以及明确标记为 501 未实现的函数清单。
   - **维护时机**：每次在 `rust_bridge` 中接入新功能（如撤回、好友通过、群管理）或调整 IPC 协议格式时更新。

3. **`02_onebot11_go_gateway_matrix.md`（OneBot 11 Go 网关矩阵）**
   - **收录内容**：Go 网关提供的 OneBot 11 API 动作支持矩阵、事件分发类型、`onebot_idmap` 数字 ID 双向映射规则、正向与反向 WebSocket 运作机制及出站节流策略。
   - **维护时机**：每次网关层新增 API 适配、修改事件转换逻辑或优化网络端点调度时更新。

4. **`03_napcat_go_wx_universal_roadmap.md`（napcat-go-wx 通用版规划）**
   - **收录内容**：`napcat-bot-go` 剥离专用业务（如 L4D2 模块）的独立 `wx` 分支规划；通用 AI 对话引擎、Agent 运行时、MCP 扩展、调度任务与 RBAC 权限等通用核心模块的保留清单与微信特性定制点。
   - **维护时机**：在进行机器人业务解耦、拉取专用分支及针对微信特性进行配置微调时更新。

---

## 三、 架构分层与职责界限铁律 (Strict Separation of Concerns)

为保证整个微信机器人技术栈结构清晰、高内聚低耦合，所有开发者与智能体必须严格遵循以下三层职责界限，**严禁越界编写逻辑**：

```
┌────────────────────────────────────────────────────────┐
│  业务大脑层：napcat-bot-go (E:\Code\Go\napcat-bot-go)   │
│  · 包含所有业务策略与决策（AI 对话、指令系统、定时任务） │
│  · 条件审核好友、主动推送、单向好友检测、转账红包策略   │
└──────────────────────────┬─────────────────────────────┘
                           │ OneBot 11 协议 (WebSocket / HTTP)
┌──────────────────────────▼─────────────────────────────┐
│  协议网关层：go_bot (E:\副业\wx_hook\go_bot)           │
│  · 纯协议翻译与双向转发：IPC ↔ OneBot 11               │
│  · 无状态、无策略、无业务大脑，严禁写任何自动业务动作     │
└──────────────────────────┬─────────────────────────────┘
                           │ 进程间 IPC (Unix Domain Socket / Pipe)
┌──────────────────────────▼─────────────────────────────┐
│  底层驱动层：rust_bridge (E:\副业\wx_hook\rust_bridge)  │
│  · 微信内存 Hook、汇编级注入、C++ ABI 适配与符号寻址    │
│  · 仅向 IPC 暴露原子 API 与捕获原始事件，不包含业务判断 │
└────────────────────────────────────────────────────────┘
```

### 1. `rust_bridge`：底层驱动 (Driver Layer)
- **唯一职责**：做微信 Linux 客户端的底层内存 Hook、逆向符号寻址、C++ ABI 适配与原始数据包捕获。
- **功能范围**：
  - 向 IPC Socket 暴露底层原生原子 API（如 `sendText`、`confirmTrans`、`receiveHongBao`、`openHongBao`、`agreeFriendReq`、`delFriend` 等）；
  - 拦截并上报底层原始事件（如接收消息、撤回、好友申请、转账通知、红包消息等）。
- **红线禁令**：**严禁在 Rust 驱动层中做任何业务判断或自动闭环**（例如严禁在 Rust 层收到转账后自动去调确认收款，严禁收到红包后自动触发拆领）。

### 2. `go_bot`：协议网关 (Gateway Layer)
- **唯一职责**：**纯协议转换与网络路由，保持无状态、无策略、无业务大脑**。
- **功能范围**：
  - **事件上行**：将 Rust 上报的原始 IPC 事件，标准化封装为 OneBot 11 事件（`message`、`request.friend`、`notice.wx_trans_pay`、`notice.wx_red_packet` 等），并广播给已连接的 OneBot 客户端；
  - **动作下行**：将客户端发来的 OneBot 11 动作（`send_msg`、`confirm_trans`、`open_hong_bao`、`set_friend_add_request`、`delete_friend` 等）转换为对应的 IPC 动作，下发给 Rust 驱动层并返回执行结果；
  - 基础链路保障：断线重连、消息去重、基础出站速率平滑（Pacer 节流保护）。
- **红线禁令**：
  - **严禁在网关层编写任何自动触发的业务闭环代码**（如自动收转账 `auto_confirm_transfer`、自动拆红包 `auto_open_hongbao`、自动同意好友 `auto_agree_friend` 等业务逻辑全部严禁写在网关内）；
  - 网关绝不能替业务层自作主张执行业务行为，所有业务行为必须由上层 OneBot 客户端（`napcat-bot-go`）主动下发指令驱动。

### 3. `napcat-bot-go`：业务大脑 (Application Layer)
- **唯一职责**：**承载全部应用层业务逻辑与智能体生态**。
- **功能范围**：
  - **好友申请策略**：监听 `request.friend`，根据验证消息关键词白名单/黑名单、来源场景做业务过滤，决定是否调用 `set_friend_add_request`；
  - **消息与对话分发**：以 `/` 开头的斜杠指令、大模型 AI 对话（DeepSeek/Gemini/OpenAI 等）、多轮会话上下文维护；
  - **主动推送与自动化**：基于持久化 Cron 的定时推送任务、公众号发文/爬虫抓取等外部事件通过 Webhook 触发的主动消息发送；
  - **单向好友（拉黑/被删）检测与清理**：执行无痕建群探测算法，识别被删/拉黑好友，按需调用 `delete_friend` 清理；
  - **资金与红包自动化**：监听 `wx_trans_pay` 和 `wx_red_packet`，由业务插件决定是否自动确认收款、安全随机延时拆红包、入账记账对账等。

