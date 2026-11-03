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

### 好友申请同意函数（VerifyUser）

`sub_6921760` 位于 RVA `0x6921760`，负责构造 `micromsg.VerifyUserRequest` 并请求 CGI 137 (`/cgi-bin/micromsg-bin/verifyuser`)。System V AMD64 ABI 参数依次为：`result_buf`, `context`, `wxid`, `opcode`, `scene`, `scan_qrcode`, `ticket`, `content`, `encryptusername`；`opcode=2` 表示同意好友申请。调用需在微信主线程执行，桥接层用 `g_idle_add` 调度，使用完毕后释放四个 `std::string` 参数。

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
