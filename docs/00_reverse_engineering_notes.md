# 0. Linux 微信逆向工程与内存布局笔记 (Reverse Engineering Notes)

> 本文档用于集中记录微信 Linux 原生客户端（当前基准版本 `4.1.15.55`）的逆向工程结论、关键函数 RVA 地址、内存对象布局、虚表结构以及交叉引用路径，供后续版本升级、函数寻址与 Hook 注入快速查阅。

---

## 一、 目标二进制基本信息

* **微信版本**：`4.1.15.55` (amd64 / x86_64)
* **核心二进制**：`/opt/wechat/wechat`（ELF 64-bit LSB PIE）
* **二进制体积**：`204,304,944` 字节 (约 195 MB)
* **SHA256 校验和**：
  ```
  6fbbba536b04c0a24730710f540887a53f4eaa24398e4f94905d9b4a36148595
  ```
* **动态依赖库重点**：
  * `libmmmojo.so`：Chromium Mojo 跨进程 IPC 通信库（负责 UI 与核心服务解耦）。
  * `libc.so.6` / `libpthread.so.0` / `libglib-2.0.so.0`。

---

## 二、 核心符号与函数 RVA 偏移表 (`offsets.rs`)

以下 RVA（Relative Virtual Address）均相对于 `wechat` 进程加载基址（Base Address）：

| 符号 / 常量标识 | RVA 偏移地址 | 对应功能与用途 | 交叉引用 / 定位特征 |
| :--- | :--- | :--- | :--- |
| **`RVA_RECEIVE_HOOK`** | `0x17B9020` | 消息接收主入口 Hook 点 (`hooked_receive`) | 调用链处于底层消息批处理分发层 |
| **`RVA_MENTION_FACTORY`** | `0x709D40` | @ 提及数据对象构造工厂函数 | 交叉引用至群消息构造逻辑 |
| **`RVA_VTABLE_MESSAGE`** | `0x9026B78` | 消息基础对象虚表 (`Message::vftable`) | 发送消息时绑定的消息体虚表指针 |
| **`RVA_BUILDER`** | `0x55C9BE0` | 消息 Builder 构造器函数 | 初始化发送数据包的核心建造者 |
| **`RVA_APP_FUNC`** | `0x45D10` | 微信全局 App 管理器实例获取函数 | 用于获取顶层管理对象 |
| **`RVA_SCHEDULER_FUNC`** | `0x47310` | 任务调度器获取函数 | 异步发送任务排队调度 |
| **`RVA_SCHEDULE_FUNC`** | `0x30F9F0` | 任务推入执行函数 | 将发送 Action 提交到底层线程池 |
| **`RVA_ALLOCATE`** | `0x754C41C` | 微信内部内存分配器 (`malloc` 封装) | 用于构造底层 C++ 字符串与对象 |
| **`RVA_FREE`** | `0x754C458` | 微信内部内存释放器 (`free` 封装) | 释放堆上构建的临时消息载荷 |
| **`RVA_VERIFY_USER`** | `0x6921760` | 构造 `micromsg.VerifyUserRequest` 并提交 CGI 137 (`/cgi-bin/micromsg-bin/verifyuser`) | `opcode=2` 同意好友申请；调用在微信主线程执行 |
| **`RVA_HONGBAO_REQ_INIT` / `RVA_HONGBAO_REQ_FREE`** | `0x7CC1BD0` / `0x7CC1DA0` | `micromsg.HongBaoReq` 构造与析构 | 请求类型 3/4 分别用于 receive/open |
| **`RVA_SK_BUFFER_INIT`** | `0x7BA6400` | 初始化 32 字节 `SKBuiltinBuffer_t` | Query `StdString*` 位于对象 `+8` |
| **`RVA_CO_CGI_SEND_COMMON_REQUEST`** | `0x727ED70` | 通用 CGI 请求发送函数 | CGI 1581 (`receivewxhb`) 与 1685 (`openwxhb`) |
| **`RVA_HONGBAO_RES_FREE`** | `0x7CC2700` | `micromsg.HongBaoRes` 析构函数 | 清理 CGI 返回对象 |
| **`RVA_GLOBAL_APP_CONTEXT`** | `0xACF6710` | 全局 App Context 指针存储地址 | `*(context vtable + 24)` 获取网络服务实例 |

### 好友申请同意函数（VerifyUser）

`sub_6921760` 位于 RVA `0x6921760`，负责构造 `micromsg.VerifyUserRequest` 并请求 CGI 137 (`/cgi-bin/micromsg-bin/verifyuser`)。System V AMD64 ABI 参数依次为：`result_buf`, `context`, `wxid`, `opcode`, `scene`, `scan_qrcode`, `ticket`, `content`, `encryptusername`；`opcode=2` 表示同意好友申请。调用需在微信主线程执行，桥接层用 `g_idle_add` 调度，使用完毕后释放四个 `std::string` 参数。

### 红包 CGI 两阶段调用（HongBaoReq）

红包请求和响应均使用 Linux 微信 4.1.15.55 的 `HongBaoReq`、`SKBuiltinBuffer_t` 与 `HongBaoRes` 对象。请求对象大小至少 48 字节；`+16` 为请求类型（3=receive、4=open），`+20` 为 1，`+24` 指向 32 字节缓冲对象，`+36` 为 `0xE`。缓冲对象 `+8` 指向堆上的 `StdString` Query，`+16` 为 Query 字节长度，`+24` 为标志 3。对象经各自构造/析构函数管理。

`RVA_GLOBAL_APP_CONTEXT` 处的全局指针指向 App Context；调用其虚表 `+24` 处函数可取得网络服务。`CoCgiSendCommonRequest` 使用 System V AMD64 ABI，参数顺序为 `res_buf`, `net_svc`, `req_buf`, `cgi_id`, `cgi_url`, `empty_str`, `options`；返回对象中的 `res_buf + 16` 指向缓冲，缓冲 `+8` 指向 `StdString` JSON 响应。

- 第一步：CGI 1581，URL `/cgi-bin/mmpay-bin/receivewxhb`，请求类型 3；Query 顺序为 `agreeDuty=0&channelId=...&inWay=...&nativeUrl=...&sendId=...&sessionUserName=...&union_source=0`。群聊 `inWay=0`，私聊 `inWay=1`；响应中的 `hbStatus=0` 与 `timingIdentifier` 用于判断并发起下一步。
- 第二步：CGI 1685，URL `/cgi-bin/mmpay-bin/openwxhb`，请求类型 4；Query 顺序为 `channelId=...&nativeUrl=...&nickName=...&sessionUserName=...&timingIdentifier=...&union_source=0`。响应 JSON 包含到账金额（分）、`receiveId` 等字段。

### 账户与联系人缓存虚表（Account & Contact Cache RVAs）

| 符号 / 常量标识 | RVA 偏移地址 | 对应功能与用途 |
| :--- | :--- | :--- |
| **`RVA_APP_GLOBAL`** | `0xBB92FC0` | 全局 App 对象指针存储地址 |
| **`RVA_APP_VTABLE`** | `0x8F514E8` | App 全局虚表地址 |
| **`RVA_CONTEXT_VTABLE`** | `0x8FD1498` | 运行上下文虚表地址 |
| **`RVA_APP_CONTEXT_GETTER`** | `0x469A0` | App 上下文获取函数 |
| **`RVA_IDENTITY_GETTER`** | `0x3855C0` | 当前登录用户身份 (Identity) 获取函数 |
| **`RVA_CACHE_PROVIDER_GETTER`** | `0x382440` | 联系人缓存提供者 (Cache Provider) 获取函数 |
| **`RVA_CACHE_PROVIDER_VTABLE`** | `0x8FD19A8` | 缓存提供者虚表地址 |
| **`RVA_CONTACT_CACHE_VTABLE`** | `0x916B1F8` | 联系人缓存虚表地址 |
| **`RVA_APP_CONTEXT_GETTER_SHARED`** | `0x60421E0` | 通过共享指针获取 App Context |
| **`RVA_CONTACT_PROVIDER_GETTER`** | `0x619CE90` | 根据 App Context 获取联系人 Provider 共享指针 |
| **`RVA_CONTACT_SERVICE_GETTER`** | `0x6111260` | 根据 Provider 获取联系人 Service 共享指针 |
| **`RVA_DELETE_CONTACT`** | `0x786F830` | 使用联系人 Service 与 wxid 删除联系人 |
| **`RVA_CHATROOM_SERVICE_GETTER`** | `0x6095540` | 根据 Provider 获取群聊 Service 共享指针 (`sub_6095540(&shared_ptr, provider)`) |
| **`RVA_OPLOG_SERVICE_GETTER`** | `0x61AE4B0` | 根据 Provider 获取 Oplog Service 共享指针 (`sub_61AE4B0(&shared_ptr, provider)`) |
| **`RVA_CREATE_CHATROOM`** | `0x78C0310` | 拉人建群核心接口 (`sub_78C0310(&res_id, chatroom_svc, &members_vec, &topic)`) |
| **`RVA_ADD_CHATROOM_MEMBER`** | `0x78BE100` | 群内拉人/邀请核心接口 (`sub_78BE100(chatroom_svc, &members_vec, &room_id, empty_proto, &res)`) |
| **`RVA_DEL_CHATROOM_MEMBER`** | `0x78C49E0` | 移出群成员核心接口 (`sub_78C49E0(&res_buf, chatroom_svc, &members_vec, &room_id, scene)`) |
| **`RVA_QUIT_CHATROOM`** | `0x78FB8C0` | 主动退出微信群 (`sub_78FB8C0(oplog_svc, &room_id)`)，提交 Oplog 16 |
| **`RVA_MOD_CHATROOM_MEMBER_NICK`** | `0x78FBCA0` | 修改自己在群内的群昵称 (`sub_78FBCA0(oplog_svc, &room_id, &self_wxid, &nick)`)，提交 Oplog 48 |
| **`RVA_EDIT_OBJ_REMARK`** | `0x76BF030` | 修改好友备注 (`sub_76BF030(&edit_obj)`)，提交 Oplog 2 (`ModContact`) |
| **`RVA_VTABLE_EDIT_REMARK`** | `0xA966508` | `EditRemarkObj` 结构体虚表指针 |
| **`RVA_REVOKE_MSG_SEND_CGI`** | `0x6AD8DC0` | 消息撤回通用网络发送函数 (`sub_6AD8DC0`) |
| **`RVA_REVOKE_MSG_REQUEST_INIT`** | `0x7CF78E0` | `micromsg.RevokeMsgRequest` 结构体构造函数 |
| **`RVA_REVOKE_MSG_REQUEST_FREE`** | `0x7CF7C00` | `micromsg.RevokeMsgRequest` 结构体析构函数 |
| **`RVA_GET_CHATROOM_INFO`** | `0x65A10B0` | 获取群聊元数据结构体 `ChatRoomInfo` 共享指针 (`sub_65A10B0(&shared_ptr, chatroom_svc, &room_id)`) |
| **`RVA_CONTACT_STORAGE_GETTER`** | `0x6383FB0` | 获取联系人与群聊本地存储服务共享指针 (`sub_6383FB0(&shared_ptr, prov)`) |
| **`RVA_GET_ALL_ROOM_CONTACTS`** | `0x65B3570` | 枚举本地存储中所有已加入群与联系人条目 (`sub_65B3570(&map, storage_svc)`) |
| **`RVA_GET_CONTACT_BY_WXID`** | `0x786E260` | 从联系人缓存中按 wxid 检索单个联系人对象 (`sub_786E260(&shared_ptr, contact_svc, &wxid)`) |
| **`RVA_DB_KEY_GETTER`** | `0x619FF60` | 本地 WCDB 数据库 32 字节 AES 密钥提取与 SIMD 解密函数 (`sub_619FF60(&key_str, app_ctx)`) |
| **`RVA_WCDB_SET_CIPHER_KEY`** | `0x87B3CF0` | WCDB 数据库加密密钥与配置挂载 (`sub_87B3CF0(db, &data, 4096, 4)`) |
| **`RVA_GET_MSG_LIST_ANCHOR`** | `0x661EE00` | 内部消息锚点分页查询接口 (`sub_661EE00(storage, &result_vec, &session, anchor, limit, direction)`) |
| **`RVA_GET_ALL_SESSION_MAX_ID`** | `0x6644240` | 获取所有会话最大 LocalId 映射表 (`sub_6644240(&map, storage)`) |

### 好友删除调用（delFriend）

Linux 微信 `4.1.15.55` 的联系人删除流程依次调用 `sub_60421E0(&app_ctx)`、`sub_619CE90(&provider, app_ctx)`、`sub_6111260(&service, provider)`，最后以 `sub_786F830(service_raw_ptr, &wxid_std_string)` 发起删除。三个 getter 按 System V AMD64 ABI 接收两字共享指针输出对象；删除操作需在微信主线程执行，桥接层通过 `g_idle_add` 调度。完成后释放 `StdString`，并递减每个共享指针控制块位于 `+8` 的引用计数；计数归零时调用控制块虚表 `+16` 的销毁函数。

### 群管理与关系链接口（Chatroom & Oplog APIs）

1. **拉人建群 (`createGroup`)**:
   - 调用流程：获取 `ChatroomService` (`0x6095540`)，构造 `std::vector<StdString>` 包含初始成员 wxid，构造 `StdString` 群主题，调用 `sub_78C0310(&res_wxid, chatroom_svc, &members_vec, &topic)`；
   - 返回：新群的真实 ID（如 `xxx@chatroom`）。

2. **拉好友进群 / 邀请进群 (`addMembers` / `inviteMembers`)**:
   - 调用流程：获取 `ChatroomService` (`0x6095540`)，构造 `std::vector<StdString>` 成员列表与 `StdString` 群 ID，调用 `sub_78BE100(chatroom_svc, &members_vec, &room_id, null, &res)`。

3. **移出群成员 (`delMembers`)**:
   - 调用流程：获取 `ChatroomService` (`0x6095540`)，构造 `std::vector<StdString>` 待踢成员列表与 `StdString` 群 ID，调用 `sub_78C49E0(&res_buf, chatroom_svc, &members_vec, &room_id, 0)`。

4. **主动退群 (`quitGroup`)**:
   - 调用流程：获取 `OplogService` (`0x61AE4B0`)，构造 `StdString` 群 ID，调用 `sub_78FB8C0(oplog_svc, &room_id)`；
   - 内部机制：微信内部构造 `micromsg.QuitChatRoom` Protobuf，填充 `UserName` 与 `ChatRoomName`，提交 `Oplog 16` 并持久化退出状态。

5. **修改群内个人昵称 (`editSelfMemberNick`)**:
   - 调用流程：获取 `OplogService` (`0x61AE4B0`)，传入群 ID、自己的 wxid 与新昵称，调用 `sub_78FBCA0(oplog_svc, &room_id, &self_wxid, &new_nick)`；
   - 内部机制：提交 `Oplog 48` (`micromsg.ModChatRoomMemberDisplayName`)。

6. **修改好友备注 (`editObjRemark`)**:
   - 调用流程：构造 64 字节结构体 `EditRemarkObj`（`+0` 为虚表 `0xA966508`，`+8` 为 `wxid`，`+32` 为 `remark`，`+56` 为 0），调用 `sub_76BF030(&obj)`；
   - 内部机制：自动查询联系人缓存，更新备注字段并写本地 SQLite 库，同时提交 `Oplog 2` (`ModContact`) 到服务器完成双向同步。

### 消息撤回与枚举接口（Revoke & Enumeration APIs）

1. **消息撤回 (`revokeMyMsg`)**:
   - 核心调用点：`sub_6AD8DC0` (`RVA_REVOKE_MSG_SEND_CGI = 0x6AD8DC0`)；
   - 载荷结构：`micromsg.RevokeMsgRequest`，使用 `sub_7CF78E0` 初始化；
   - 结构体偏移：`+16` 为 `client_msg_id` (`StdString`)，`+24` 为 `create_time` (`u32`)，`+32` 为 `from_user` (`StdString`)，`+48` 为 `to_user` (`StdString`)，`+56` 为 `svr_msg_id` (`u64`)；
   - 发送调用：通过全局网络服务发起 CGI 536 (`/cgi-bin/micromsg-bin/revokemsg`) 请求；执行后调用 `sub_7CF7C00` 析构请求对象并释放 `StdString`。

2. **群成员列表获取 (`getMemberList`)**:
   - 核心函数：`sub_65A10B0` (`RVA_GET_CHATROOM_INFO = 0x65A10B0`)；
   - 调用流程：获取 `ChatroomService` (`0x6095540`)，传入目标群号 `xxx@chatroom` 调用 `sub_65A10B0(&mut chatroom_info, chatroom_svc, &room_id)`；
   - 数据解析：返回的 `ChatRoomInfo` 对象中，偏移 `+8` 为群 ID，`+32` 为群主 wxid，`+88` 为群名称，`+136` 处直接存储完整的全群成员列表字符串（以 `^` 分隔），`+184` 为群公告；
   - 成员信息补齐：遍历拆分各成员 wxid 后，调用 `sub_786E260` (`RVA_GET_CONTACT_BY_WXID = 0x786E260`) 查联系人缓存，直接从内存中回填各群员昵称与备注。

3. **已加入群列表与联系人枚举 (`getGroupList` / `getFriendList`)**:
   - 核心函数：`sub_65B3570` (`RVA_GET_ALL_ROOM_CONTACTS = 0x65B3570`)；
   - 调用流程：通过 `sub_6383FB0` (`RVA_CONTACT_STORAGE_GETTER = 0x6383FB0`) 获取存储服务指针，调用 `sub_65B3570(&mut map, storage_svc)` 枚举内存联系人表；
   - 遍历方式：读取哈希桶链表首节点 `*(_QWORD **)(map + 16)`，沿 `*node` 指针遍历，`node + 16` 处为联系人/群聊唯一标识 `StdString`；以 `@chatroom` 结尾的归入群列表，其余常规 `wxid` 归入联系人列表。

### 本地数据库解密与历史消息架构（WCDB & SQLCipher Key）

Linux 微信客户端底层采用腾讯自研开源数据库组件 **WCDB**，其底层基于 **SQLCipher 4** 构建：

1. **核心加密参数与数据库清单**：
   - **页面大小 (Page Size)**：`4096` 字节 (`0x1000`)；
   - **加密标准 (Cipher Version)**：SQLCipher `Version 4` (`AES-256-CBC`，默认 KDF 迭代 256000 次)；
   - **本地存储路径**：`$HOME/.xwechat/xwechat_files/<wxid>/db/`；
   - **核心数据库文件**：
     - `session/session.db`：会话列表、置顶状态与未读消息映射；
     - `contact/contact.db`：联系人详尽资料、备注与群聊缓存；
     - `message/message_0.db`、`message/message_1.db` ...：分库消息记录，包含文本、XML、语音与转账结构；
     - `message/message_fts.db`：消息全文检索引擎。

2. **32 字节 AES 密钥提取与 SIMD 解密算法**：
   - **关键函数**：`sub_619FF60` (`RVA_DB_KEY_GETTER = 0x619FF60`)；
   - **调用机制**：
     1. 调用 `sub_60421E0(&app_ctx)` 获取全局 `AppContext` 智能指针；
     2. 传入 `&mut key_str` 和 `app_ctx.0[0]` 调用 `sub_619FF60(&mut key_str, app_ctx.0[0])`；
     3. 内部从 `app_ctx + 600` 读取被混淆的原始密钥 `std::string`，并通过两组 128 位 SIMD Mask 执行 XOR 反混淆：
        ```text
        mask_low  = 0xD8E794D9E8E97BELL, 0x2026CD93325E16D4LL
        mask_high = 0x3071C16C37B6DF62LL, 0xBB547E5E74E1F8B6LL
        ```
     4. 输出完整的 32 字节 raw AES 密钥。转为 64 位十六进制格式后，可直接通过 `PRAGMA key = "x'<hex_key>'"; PRAGMA cipher_page_size = 4096;` 对任意本地数据库进行即时解密。

3. **内部历史消息锚点查询函数 (`GetMsgListWithAnchor`)**：
   - 核心符号：`sub_661EE00` (`RVA_GET_MSG_LIST_ANCHOR = 0x661EE00`)；
   - 参数列表：`storage_ptr`, `vector<MsgWrap>* out_vec`, `StdString* session_id`, `u64 anchor_local_id`, `u32 limit`, `u32 direction`；
   - 机制：支持以指定消息 `local_id` 为基准向前或向后快速分页拉取历史消息列表。



---

## 三、 消息接收拦截结构与字段偏移

### 1. Hook 点与调用约定
- **Hook 函数**：`hooked_receive(manager: *mut c_void, batch: *const usize, flag1: u8, flag2: u8) -> usize`
- **挂载位置**：`base + RVA_RECEIVE_HOOK (0x17B9020)`
- **数据结构**：参数 `batch` 指向一个连续的数组结构，包含消息的内存起始与结束指针：
  - `start = *(batch as *const usize)`
  - `end = *(batch.add(1) as *const usize)`
  - 单条消息步长：`0x78` 字节 (`addr += 0x78`)

### 2. 单条接收消息内存偏移
以单条消息起始指针 `msg` 为基准：
- `*(msg.add(0x14) as *const u32)`：**`msg_type`**（消息类型：1=纯文本/引用, 3=图片, 37=好友申请, 47=表情, 49=AppMsg/转账/推文, 10000=系统通知, 10002=撤回）
- `read_pb_string(msg, 0x08)`：**`sender`**（发送者 wxid）
- `read_pb_string(msg, 0x18)`：**`receiver`**（接收者 wxid，群聊时为 `xxx@chatroom`）
- `read_pb_string(msg, 0x20)`：**`text`**（消息文本内容；群聊中格式为 `member_wxid:\n正文`）
- `read_source_string(msg, 0x38)`：**`mention_source`**（消息来源 XML 元数据，包含 `<atuserlist>`）

---

## 四、 消息发送与 atuserlist 注入机制

### 1. 发送消息底层对象构建
- 发送文本消息时，底层通过 `RVA_BUILDER (0x55C9BE0)` 构建消息对象 `Message`。
- 构建出的对象绑定虚表 `RVA_VTABLE_MESSAGE (0x9026B78)`。

### 2. `@群成员` 注入原理 (`msg_hook.rs` & `sender.rs`)
- 在微信底层 `Message` 对象中，**`atuserlist`** 成员位于对象偏移 **`+0x18`**。
- `rust_bridge` 安装了 `atuserlist` 注入 Hook：在底层发送函数执行前，拦截 `obj` 指针，并将待 @ 成员列表（逗号分隔的 wxid 字符串，若 @所有人 则填 `notify@all`）转换为底层 C++ 字符串写入 `obj + 0x18`，实现原生高亮 @ 提醒。

---

## 五、 Mojo 跨进程通信与 Protobuf 映射 (备忘)

* **Chromium Mojo 接口 (`libmmmojo.so`)**：
  * `CreateMMMojoEnvironment (0x3DE00)`：分配 272 字节的 Environment 实例。
  * `SetMMMojoEnvironmentCallbacks (0x3DEA0)`：注册 `kMMReadPush` (类型 1)、`kMMReadPull` (类型 2) 等 IPC 回调。
  * `CreateMMMojoWriteInfo (0x3E2C0)` / `SendMMMojoWriteInfo (0x3E940)`：用于向前端 UI 发送渲染推送。
* **内嵌 Protobuf 类型**：
  * 发送请求：`micromsg.SendMsgRequestNew`
  * 接收追加：`micromsg.AddMsg`
  * 漫游漫步：`micromsg.BypMsgText` / `micromsg.BypMsgPack`

---

## 六、 版本升级追踪指引 (How to Update)

微信版本更新时，请按以下步骤重新提取 RVA：
1. 使用 `ida-mcp` 打开新的 `wechat` 二进制并运行分析；
2. 搜索特征字符串：
   - 搜 `"delchatroommember"` 定位群成员变动与系统消息处理函数；
   - 搜 `"<atuserlist>"` 定位 `mention_source` 解析引用；
   - 搜 `RVA_VTABLE_MESSAGE` 原虚表签名比对新虚表地址；
3. 更新 `rust_bridge/src/offsets.rs` 中的常量，并在本地重新执行交叉编译与测试。
