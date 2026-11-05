# 2. OneBot 11 Go 网关功能与适配矩阵 (OneBot 11 Go Gateway Matrix)

> 本文档详细梳理 `go_bot` 网关程序对 OneBot 11 标准规范的实现程度、微信特性适配转换机制、双向 ID 映射规范及多协议端点网络架构，供上层 Bot（如 `napcat-bot-go`）或第三方业务接入查阅。

---

## 一、 网络接入与端点模式

`go_bot` 支持在 `config.yaml` 的 `endpoints` 中配置多个并发接入端点：

| 端点类型 (`type`) | 角色模型 | 默认端口/路径 | 特性说明 |
| :--- | :--- | :--- | :--- |
| **`httpServer`** | HTTP 服务端 | `0.0.0.0:8080/` | 标准 OneBot 11 HTTP POST 动作接口 |
| **`wsServer`** | 正向 WebSocket 服务端 | `0.0.0.0:8080/ws` | 供外部 Bot 客户端主动连入，实时下发事件与接收动作 |
| **`wsClient`** | **反向 WebSocket 客户端** | 目标端指定 URL (如 `ws://127.0.0.1:27039/ws`) | **网关主动连接 Bot 框架**，支持断线 3 秒退避重连与 Ping/Pong 保活 |
| **`httpClient`** | HTTP Webhook 客户端 | 目标端指定 URL (如 `http://127.0.0.1:5700/webhook`) | 收到微信事件时主动以 HTTP POST 方式推送 JSON |

---

## 二、 OneBot 11 API 动作适配矩阵

### 1. 消息发送与管理
| OneBot 动作名 | 映射的底层调用 | 实现细节与降级策略 |
| :--- | :--- | :--- |
| **`send_msg`** | `sendText` / `sendText2` / 富媒体 | 根据 `message_type` 自动判断分发给好友或微信群 |
| **`send_group_msg`** | `sendText` / `sendText2` / `sendImage` | 发送微信群消息。若消息含 `at` 段，自动转为 `sendText2` 注入 `atuserlist` |
| **`send_private_msg`** | `sendText` / `sendImage` / `sendFile` | 发送好友私聊文本或图片/文件 |
| **`delete_msg`** | `revoke_msg` | 将 OneBot `message_id` 还原为原生消息标识；优先从已收到的消息上下文补齐会话、客户端消息 ID、服务端消息 ID 与创建时间，再转发撤回请求。也接受显式 `to_user` / `group_id` / `user_id` 和撤回元数据 |
| **`get_msg`** | `queryObj` | 查询历史消息详情（依赖本地 SQLite 缓存） |
| **`delete_friend`** | `delFriend` | 根据 `user_id` 删除指定好友；`del_friend` 与 `delFriend` 也可调用 |
| **`confirm_trans`** | `confirmTrans` | 按转账单号及微信要求的辅助字段提交确认 |
| **`receive_hong_bao`** | `receiveHongBao` | 查询红包状态并获取拆领所需的 `timingIdentifier` |
| **`open_hong_bao`** | `openHongBao` | 使用 `timingIdentifier` 提交红包领取请求 |

### 2. 账号与状态查询
| OneBot 动作名 | 映射的底层调用 | 实现细节与降级策略 |
| :--- | :--- | :--- |
| **`get_login_info`** | `getSelfInfo` | 返回登录微信号的 `user_id`（wxid 或映射数字）与 `nickname` |
| **`get_status`** | 内部度量 | 返回长连接在线状态 `online: true` 及收发 QPS 数据 |
| **`get_version_info`**| 静态返回 | `app_name: "wxbot-onebot"`, `protocol_version: "v11"` |

### 3. 列表枚举与群资料 (优雅降级设计)
| OneBot 动作名 | 映射的底层调用 | 实现细节与降级策略 |
| :--- | :--- | :--- |
| **`get_group_list`** | `getGroupList` | 将原生群列表转换为 OneBot `[{"group_id": 123, "group_name": "..."}]`；群 ID 映射为稳定数字 ID，底层失败时返回空列表 |
| **`get_friend_list`** | `getFriendList` | 将原生好友列表转换为 OneBot `[{"user_id": 456, "nickname": "..."}]`；用户 ID 映射为稳定数字 ID，底层失败时返回空列表 |
| **`get_group_member_list`** | `getMemberList` | 将原生成员对象展开为 OneBot 成员数组，映射群号与成员 ID，并将群主标记为 `role: "owner"`；底层失败时返回空列表 |
| **`get_group_member_info`** | `getMemberNick` | 查询群成员昵称。微信群不区分管理员，默认固定返回 `role: "member"` |
| **`get_group_root_files`** | - | 微信无 QQ 群文件系统，返回 `files: [], folders: []`，客户端安全降级 |
| **`get_group_files_by_folder`** | - | 同上，安全返回空列表 |
| **`get_group_file_url`** | - | 同上，安全返回空列表 |

### 4. 群管与好友请求
| OneBot 动作名 | 映射的底层调用 | 说明 |
| :--- | :--- | :--- |
| **`set_group_kick`** | `delMembers` | 将 OneBot `group_id` 和 `user_id` 转为群标识与单成员列表，移出指定成员 |
| **`set_group_leave`** | `quitGroup` | 将 `group_id` 转为 `chatroom_name` 并提交主动退群请求 |
| **`set_group_card`** | `editSelfMemberNick` | 使用 `group_id` 和 `card`（或 `nickname`）修改当前账号的群内昵称 |
| **`set_friend_remark`** | `editObjRemark` | 将 `user_id` 与 `remark` 转为微信好友备注原子调用 |
| **`set_friend_add_request`** | `agreeFriendReq`；备注形态转 `editObjRemark` | 带 `flag` 时沿用好友申请同意流程；仅带 `wxid`/`user_id` 与 `remark` 时作为备注兼容别名 |

收到 `friendReq` 时，网关按 ticket/flag 缓存申请信息并上报 OneBot `request.friend` 事件。网关不自动审核或发送欢迎消息；由外部客户端通过带 `flag` 的 `set_friend_add_request` 决定是否调用 `agreeFriendReq`。拒绝操作由底层桥接层暂不支持。好友添加成功时，网关会发送 OneBot 11 `notice_type: "friend_add"` 通知。

收到 `transPay` 时，网关通过 OneBot notice 透传 `transfer_id`、`invalid_time`、`transfer_attach` 和金额等字段；需要确认时，由外部客户端调用 `confirm_trans`，网关不会自动确认收款。

收到 `hongBao` 时，网关发送 `notice_type: "wx_red_packet"`，包含祝福语、红包发送 ID、原始 `native_url`、用户与群 ID。客户端可根据事件自行调用 `receive_hong_bao` 查询状态，再按需要调用 `open_hong_bao`；网关不会自动延时拆领或生成“已领取”事件。

### 5. 微信专属扩展动作
| 扩展动作名 | 说明 |
| :--- | :--- |
| **`send_xml`** | 发送自定义 XML 卡片数据包 |
| **`send_card`** | 发送个人/公众号推荐名片卡片 |
| **`send_applet`** | 发送微信小程序卡片 |
| **`set_read_status`** | 设置指定会话为已读状态 |
| **`create_group`** | 扩展动作；将 `user_list`/`member_list`/`users` 及 `topic`/`name` 转发至 `createGroup`，成功返回新群 ID |
| **`add_members`** | 扩展动作；将 `group_id` 与 `user_list`/`member_list` 转发至 `addMembers` |
| **`get_db_key`** | 扩展动作；向底层驱动请求当前登录账号的 32 字节 AES 密钥与数据库目录 |
| **`get_msg`** | 标准动作；将 `message_id` 转为底层 `queryObj` 查询 |
| **`get_history_msg`** / **`get_chat_history`** / **`get_group_msg_history`** | 扩展动作；支持会话维度历史消息拉取与数据库密钥元数据联动 |

---

## 三、 ID 双向映射系统 (`onebot_idmap`)

微信原生体系采用字符串（`wxid_xxx`、`xxx@chatroom`），而 NapCat/QQ 机器人客户端强依赖 `int64` / `int32` 数字。网关内置了 `IDMapper` 映射器：

1. **全链路映射**：群列表、好友列表和群成员列表按 OneBot 11 返回数字 ID；`delete_msg` 与 `get_group_member_list` 的输入 ID 始终支持反向解析。配置 `gateway.qq_compat: true` 时，事件与其它动作的用户、群组和消息 ID 也执行数字化转换。
2. **分配区间**：
   - 用户 ID (`user_id` / `qq`)：从 **`100000001`** 起始递增分配（`int64`）。
   - 群组 ID (`group_id`)：从 **`200000001`** 起始递增分配（`int64`）。
   - 消息 ID (`message_id`)：从 **`1000001`** 起始递增分配（`int32`）。
3. **双向索引持久化**：
   - 运行中自动在 `-root` 目录下维护并持久化 `id_mapping.json`；
   - 重启后自动载入原有双向映射表，保证相同的微信号与群号永远映射至唯一且固定的数字 ID。
4. **全链路自动换算**：
   - 事件出站时：自动将 `user_id`、`group_id`、`message_id`、`[CQ:at,qq=...]` 及消息段 `data.qq` 换算为数字；
   - 动作入站时：自动将数字 ID 逆向解析回微信原生的 `wxid_xxx` 与 `xxx@chatroom` 再交由微信底座发送；成员数组 `user_list`、`member_list`、`users` 与群标识字段 `chatroom_name` 同样参与双向转换。

---

## 四、 微信特性适配与富媒体转换

1. **群聊 @ 提及转换与前缀清洗**：
   - 微信进来的群消息若包含 `<atuserlist>`，网关会自动在 `message` 数组前注入 `{"type": "at", "data": {"qq": ...}}` 消息段，并在 `raw_message` 开头拼接 `[CQ:at,qq=...]`；
   - 自动检测并剥离正文开头的 `@昵称\u2005`（U+2005 四分之一空格）或 `@昵称 ` 前缀，保证大模型提取到的提问文本干净无杂质。
2. **富媒体与远程图片自动下载**：
   - 网关内置 `media_resolver.go`，当 Bot 传入网络图片 URL（`http://`, `https://`）或 Base64 编码时，网关在 15 秒限时内将其下载/解码至本地缓存目录 (`data/media`)，并将绝对路径交给微信发送，对上层透明。
3. **引用回复降级**：
   - 微信暂无原生引用段，网关遇到 `type: "reply"` 时采取静默跳过，确保后续纯文本正文正常发至微信群。

---

## 五、 出站节流与防风控机制 (`OutboundPacer`)

为防止机器人高频刷屏触发微信官方风控与封号，网关内置了出站步进调度器：

* **目标最小间隔 (`target_min_interval_ms`)**：同群/同好友发信间隔限制（默认 600ms）。
* **全局发信 QPS 限制 (`max_messages_per_sec`)**：全局最大允许发信速率（默认 5 条/秒）。
* **随机抖动 (`jitter_min_ms` ~ `jitter_max_ms`)**：每次发信追加 200~500ms 随机微小延迟，打散机器人发信特征。
* **排重防死循环 (`anti_loop_window_sec`)**：2 秒内若有完全重复的文本重复发送，自动抑制。
