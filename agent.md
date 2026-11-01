# 项目开发规范与智能体指引 (Agent Guidelines)

## 核心开发与构建规范

### 1. 本机编译与打包铁律 (Mandatory Local Build Policy)
- **除 `ida-mcp` 逆向分析工具外，任何语言（Rust、Go、C/C++、Python、Shell 等）的代码编写、依赖安装、交叉编译与打包构建，必须且只能在开发者本地电脑（Local Machine）完成。**
- **严禁在远程 SSH 测试机/服务器上安装编译链**（包括但不限于 `cargo`、`rustc`、`go`、`gcc`、`clang`、`llvm`、`pip` 等构建重型包），严禁在远程执行构建打包任务。远程机器性能受限，仅负责运行测试与加载最终二进制制品。

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

#### (2) Go 机器人程序 (`wxbot_linux`)
在本地 Windows 终端通过 Go 自带的交叉编译器执行：
```powershell
cd E:\副业\wx_hook\go_bot
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags "-s -w" -o wxbot_linux main.go
```
编译产物位于：
`E:\副业\wx_hook\go_bot\wxbot_linux`

---

### 3. 上传与远程部署流程 (Upload & Run Workflow)

所有产物在本地构建完成后，通过 `scp` 一键上传至远程目标机器并测试：
```powershell
# 1. 上传 Rust 动态库
scp E:\副业\wx_hook\rust_bridge\target\x86_64-unknown-linux-gnu\release\librust_bridge.so root@192.168.2.100:/opt/wx/librust_bridge.so

# 2. 上传 Go 机器人可执行文件
scp E:\副业\wx_hook\go_bot\wxbot_linux root@192.168.2.100:/opt/wx/wxbot

# 3. 远程仅负责重启与测试日志
ssh root@192.168.2.100 "chmod +x /opt/wx/wxbot /opt/wx/librust_bridge.so; /opt/wx/wxbot -root /opt/wx"
```

---

### 4. 智能体分工与模型协作规范 (Model Assignment & Division of Labor)

- **后端开发 (Go / Rust)**：
  - 所有 Go (`go_bot/`) 和 Rust (`rust_bridge/`) 后端代码的编写与实现，交由子智能体（Subagent）执行，指定模型为 **`gpt-6-luna`**。
  - 遇到 Go / Rust 编码任务时，主智能体使用 `Task` 工具（配置 `model: "gpt-6-luna"`, `subagent_type: "generalPurpose"`）派发任务并跟进。
- **前端与 IDA MCP 分析 (Frontend & IDA MCP)**：
  - 由主智能体直接独立编写与操作，不进行外部委派。
  - 涵盖所有前端页面与组件的开发实现。
  - 涵盖所有基于 `ida-mcp` 的逆向工程分析、地址与符号提取、反编译审计及 IDA 相关脚本编写。

