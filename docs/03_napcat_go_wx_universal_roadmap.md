# 3. napcat-go-wx 通用版分支规划与精简指南 (napcat-go-wx Universal Roadmap)

> 本文档规划了从当前的 `napcat-bot-go` 中独立剥离通用核心代码、建立专门面向微信生态的 `wx` 分支（即 **`napcat-go-wx` 通用版**）的实施蓝图与架构解耦规范。

---

## 一、 背景与解耦动机

当前 `napcat-bot-go` 仓库将**通用 OneBot 11 AI 机器人底座**与**特定游戏业务（求生之路 L4D2 服务器管理系统）**深度混合：
1. 包含大量特定游戏指令（`/服`、RCON 远程执行、VPK 扩展仓库等）；
2. 包含针对 QQ 群特有功能的强假设（如群文件目录树深度解析、QQ 群头衔同步等）。

**目标**：在 `napcat-bot-go` 代码库中单独切出 `wx` 分支，彻底移除与游戏相关的专有代码，打造成一个**轻量、纯粹、高内聚的通用微信 OneBot 11 智能体框架**，供微信群问答、私聊助理、通用 MCP 扩展与自动化任务使用。

---

## 二、 模块保留与剔除清单

### 1. 【保留】通用核心组件 (Core Engine)

| 目录/模块 | 保留的核心价值与职责 |
| :--- | :--- |
| **`ai/`** | 通用大模型引擎：支持 Gemini、DeepSeek、OpenAI 兼容接口；多阶段推理（工具决策/富文本总结）；多轮会话管理器 (`SessionManager`)；意图路由与安全分类 (`decision.go`, `ai/policy/`) |
| **`agent/`** | 现代智能体管道：Agent 运行时、管道抽象、附件桥接与上下文属性注入 |
| **`protocol/onebot/`** | OneBot 11 协议通信层：支持正向/反向 WebSocket 客户端与服务端，CQ 码解析器与消息段模型 |
| **`component/tool/`** | 通用工具层：工具注册中心 (`registry.go`)、执行调度器 (`executor.go`)、Tavily 联网搜索 (`tools_search.go`)、定时任务调度工具 (`tools_schedule.go`) |
| **`schedule/`** | 持久化定时调度引擎：SQLite 驱动的 Goroutine Timer，支持断电重启恢复与 Cron 周期调度 |
| **`auth/`** | RBAC 权限与身份服务：Owner、Admin、Member 权限模型与指令鉴权 |
| **`adminapi/`** & **`web/`** | 管理控制台：Vue 3 + Tailwind CSS 控制台与对应的管理 RESTful API |
| **`config/`** | 配置中心：YAML 加载、结构体校验与环境覆盖 |

### 2. 【剔除】专有游戏与冗余模块 (To Be Removed)

在 `wx` 分支中彻底删除以下业务目录，解除所有引用：

- **`projects/l4d2/`**（战役地图管理、RCON 密码校验、玩家查询服务、游戏语言包）
- **`cmd/l4d2-commands/`**（游戏独立命令服务）
- **`cmd/l4d2-import/`**（游戏数据导入程序）
- **`cmd/l4d2-plugin/`**（游戏插件接入程序）
- **`cmd/mcp-l4d2-servers/`**（游戏 MCP 服务）

---

## 三、 微信生态下的核心适配改动 (`wx` 分支定制)

当作为 `napcat-go-wx` 运行时，必须针对微信生态的特征做以下定制与默认优化：

### 1. 默认网络拓扑：反向 WebSocket 优先
- 微信网关通常部署于 Linux 服务器或特定宿主机，机器人在本地或同一内网运行时，推荐默认采用**反向 WebSocket 监听**：
  ```yaml
  onebot:
    mode: "reverse"
    reverse_listen: "0.0.0.0:27039"
    reverse_path: "/ws"
  ```
- `wx_hook` 网关作为客户端主动连入，无须为机器人配置额外的内网穿透。

### 2. 关闭思考占位符 (`show_waiting: false`)
- 微信底层撤回机制有限且发送回执目前无真实序号；
- 将 `ai.show_waiting` 默认值设为 `false`，大模型直接在回答生成完成后推送回复，彻底杜绝群内废弃占位符残留。

### 3. 解除群聊硬编码，打通私聊 AI 双通道
- 目前 `bot_service.go` 中仅允许 `event.MessageType == "group"` 触发大模型；
- 在 `wx` 分支中放开私聊门禁：
  ```go
  isGroupAt := event.MessageType == "group" && onebot.IsAtSelf(event)
  isPrivate := event.MessageType == "private"
  if (isGroupAt || isPrivate) && s.cfg.AI.Enabled && s.aiClient != nil {
      // 统一进入 handleAIChat
  }
  ```
- 队列键根据会话类型分配：群聊为 `g:<群号>:u:<用户号>`，私聊为 `p:<用户号>`；
- 发送响应时，群聊走 `SendGroupReplyMsgContext`，私聊自动分发至 `SendPrivateMsg`。

### 4. 好友与群聊欢迎语联动
- 监听 `notice_type: "friend_add"`（微信新添加好友）与 `group_increase`（新人进群）；
- 结合 `WelcomeConfig` 提供开箱即用的自动私聊打招呼与入群欢迎。

---

## 四、 分支建立与交付现状 (Completed)

1. **分支创建与检出**：
   - 已从 `generic-dev` 正式检出独立的 **`wx`** 分支（Commit `0ab10cc`）。
2. **专有游戏模块物理删除**：
   - 彻底移除了 `projects/l4d2/`、`cmd/l4d2-*`、`cmd/mcp-l4d2-*`、`l4d2plugin/` 与 `l4d2bot.service`（删除近 6000 行专有耦合代码）。
3. **微信双通道 AI 调度与欢迎语接入**：
   - 放开私聊限制，私聊直接触发大模型意图路由，回复走 `SendPrivateMsg`；群聊保留 `@机器人` 触发；
   - 监听 `notice_type: "friend_add"`，实现添加好友自动发送欢迎语。
4. **反向 WebSocket 默认配置**：
   - 默认反向 WebSocket 监听 `0.0.0.0:27039/ws`；
   - 默认禁用 `show_waiting` 思考占位符以杜绝废弃消息残留。
5. **单元测试验证**：
   - `go test ./...` 100% PASS。
