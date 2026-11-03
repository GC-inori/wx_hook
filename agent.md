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
