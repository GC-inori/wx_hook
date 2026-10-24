# Linux 原生微信 (4.1.13.26) 逆向与动态库分析笔记

> 本文档用于记录 Linux 微信各 ELF 二进制程序及 `.so` 共享库的逆向分析结论、内存布局与 ABI 规范，供后续开发、Hook 拦截和维护查阅，避免重复检索。

---

## 一、 基本信息与文件资产

* **分析版本**：微信 Linux 原生通用版 `4.1.13.26` (amd64 / x86_64)
* **安装包**：`WeChatLinux_x86_64.deb`
* **解压路径**：`linux_extracted/opt/wechat/`（或本地分析路径 `E:\Code\Hacker\Project\Weixin\linux_extracted\opt\wechat\`）
* **主程序**：`opt/wechat/wechat`（172.8 MB，ELF 64-bit LSB PIE）

---

## 二、 模块全景与优先级矩阵

| 模块名称 | 体积 | 职责分类 | 分析状态 | 优先级与跟进建议 |
| :--- | :--- | :--- | :--- | :--- |
| **`libmmmojo.so`** | 2.0 MB | Chromium Mojo 跨进程 IPC 通信库 | **已深入分析** | **极高**（前端 UI 与核心进程 IPC 总线，符号全导出，首选 Hook 点） |
| **`wechat`** (主程序) | 172.8 MB | 业务逻辑、UI 框架、本地数据库核心 | **已确定依赖关系** | **高**（导入了 `libmmmojo.so` 的 21 个导出符号，负责注册回调与发包） |
| **`libandromeda.so`** | 5.8 MB | 网络层封装 (Cronet / gQUIC) | 已初步筛查 | 中低（网络底层，主要为 HTTP/QUIC 栈，无需深究聊天逻辑） |
| **`libconfService.so`** | 1.4 MB | 配置同步服务 | 已初步筛查 | 中低（含 `CmdMsg`、`OnSyncMsgTimer`，可作同步信令参考） |
| **`libroam_migration.so`** | 8.0 MB | 聊天记录漫游与迁移 | 暂未分析 | 低（仅在涉及聊天记录备份/还原时分析） |
| **`libilink2.so`** / `libilink_stream.so` | 13.4 MB | 腾讯私有二进制加密传输协议 | 已确认职责 | 忽略（处于加密链路末端，明文消息处理不在此层） |
| **`libvoip*`** (3个库) | 30 MB+ | 音视频通话协议与编解码 | 已确认职责 | **直接排除**（音视频专用） |
| **`libvlc*`** (2个库) | 1.3 MB | 视频播放器内核 | 已确认职责 | **直接排除**（媒体播放专用） |
| **`libwxocr.so`** / `libwxtrans.so` | 56 MB | 图片 OCR 文字识别与翻译 | 已确认职责 | **直接排除**（离线工具插件） |
| **`libcrashpad_client.so`** | 3.3 MB | Google Crashpad 崩溃收集上报 | 已确认职责 | **直接排除**（崩溃监控） |

---

## 三、 `libmmmojo.so` 核心分析笔记

* **IDA 数据库**：`libmmmojo.so.i64` (MD5: `4632d25f4ffd71a83d16a6cf78d1e67c`，基址 `0x0`)
* **函数总数**：6323 个函数，导出 23 个以 `MMMojo` 命名的公开 C 接口函数。

### 1. 回调函数注册表 (`SetMMMojoEnvironmentCallbacks`)
* **入口地址**：`0x3DEA0`（实际转至内部实现 `sub_B1470` / `0xB1470`）
* **参数模式**：2 个固定参数 + 1 个可变参数（`c_int type, ...`）。
* **回调类型枚举 (`MMMojoCallbackType`)**：

| 枚举值 | 常量名 | 回调函数原型 / 槽位数据 | 触发时机与用途 |
| :---: | :--- | :--- | :--- |
| `0` | `kMMUserData` | `void *user_data`（非函数，存入 `env + 8`） | 关联给回调函数的用户上下文指针 |
| `1` | `kMMReadPush` | `void (*)(uint32_t req_id, const void *read_info, void *user_data)` | **接收 Push 消息推送（被动收消息核心）** |
| `2` | `kMMReadPull` | 同上 | 接收 Pull 请求的响应消息 |
| `3` | `kMMReadShared` | 同上 | 接收共享内存数据 |
| `4` | `kMMRemoteConnect` | `void (*)(bool connected, void *user_data)` | IPC 远端管道连接建立通知 |
| `5` | `kMMRemoteDisconnect`| `void (*)(void *user_data)` | IPC 远端断开通知 |
| `6` | `kMMRemoteProcessLaunched` | `void (*)(void *user_data)` | 子进程启动完成通知 |
| `7` | `kMMRemoteProcessLaunchFailed` | `void (*)(int error_code, void *user_data)` | 子进程拉起失败 |
| `8` | `kMMRemoteMojoError` | `void (*)(const void *err_buf, int err_sz, void *user_data)`| Mojo 内部通信错误 |

> **关键认知**：
> `CreateMMMojoEnvironment`（`0x3DE00`）在堆上分配 **272 字节** 的环境对象（`Environment`）。回调函数指针依次存放在该对象的 `+16` ~ `+72` 偏移中。必须通过原函数创建，严禁外部伪造。

---

### 2. 接收结构体 `ReadInfo` 内存布局
* **读取函数**：
  * `GetMMMojoReadInfoRequest` (`0x3E0B0`)
  * `GetMMMojoReadInfoAttach` (`0x3E160`)
  * `GetMMMojoReadInfoMethod` (`0x3E1B0`)
  * `GetMMMojoReadInfoSync` (`0x3E1C0`)
* **结构体内部大小**：152 字节。
* **字段关键偏移**：
  * `+0` (`uint32_t`)：消息类型方法号（Method ID）。
  * `+4` (`uint8_t`)：是否为同步调用（Sync flag）。
  * `+8` (`uint8_t`)：普通 Request 数据有效标志（Valid flag）。
  * `+16` (`void *`)：普通数据**起始指针 (Begin)**。
  * `+24` (`void *`)：普通数据**结束指针 (End)**。
    * **数据长度计算**：`(uintptr_t)*(a1 + 24) - (uintptr_t)*(a1 + 16)`。注意：`+24` 是 End 指针，并非直接存长度。
  * `+32` (`void *`)：缓冲区容量指针 (Capacity)。
  * `+72` / `+80`：Shared 数据句柄标志及句柄。
  * `+88` / `+96`：Shared 内存映射有效标志及已映射指针。
  * `+104` / `+112`：Shared 内存长度标志及实际长度（`uint32_t`）。
  * `+120` (`uint8_t`)：Attach 附加数据有效标志。
  * `+128` (`void *`)：Attach 数据起始指针。
  * `+136` (`void *`)：Attach 数据结束指针（长度为 `+136 - +128`）。

> **生命周期注意**：
> Getter 返回的是借用指针，使用完毕后需由微信原调用流触发 `RemoveMMMojoReadInfo`（`0x3E1E0`），Hook 层不可提前释放。

---

### 3. 发送结构体 `WriteInfo` 与分发路径
* **创建函数**：`CreateMMMojoWriteInfo(int method, bool sync, uint32_t request_id)` (`0x3E2C0`)
  * 分配 **184 字节** 内部对象并初始化。
* **分发函数**：`SendMMMojoWriteInfo(void *env, void *write_info)` (`0x3E940` -> `sub_B3A00`)
* **消息类型枚举 (`MMMojoInfoMethod`)**：
  * `1` (`kMMPush`)：单向推送消息。
  * `2` (`kMMPullReq`)：请求消息（期望对端回执）。
  * `3` (`kMMPullResp`)：对端 Pull 的响应消息。
  * `4` (`kMMShared`)：共享内存大块数据。
* **写入流程**：
  1. 调用 `CreateMMMojoWriteInfo(method, sync, req_id)` 获取句柄。
  2. 调用 `GetMMMojoWriteInfoRequest(write_info, payload_size)` 获取可写缓冲区指针（内部存入 `+24`）。
  3. 将明文/Protobuf 序列化数据拷贝入该缓冲区。
  4. 如有附加数据，调用 `GetMMMojoWriteInfoAttach(write_info, attach_size)` 写入。
  5. 调用 `SendMMMojoWriteInfo(env, write_info)` 完成投递。

> **生命周期注意**：
> `SendMMMojoWriteInfo` 内部会消费并释放 `write_info` 对象（调用 `sub_3E9B0` 和释放堆内存）。发送后指针失效，严禁二次释放。

---

### 4. C / Rust 兼容 FFI 签名

```rust
use core::ffi::{c_int, c_void};

// 回调函数类型定义
pub type MMReadCallback = unsafe extern "C" fn(
    request_id: u32,
    read_info: *const c_void,
    user_data: *mut c_void,
);

unsafe extern "C" {
    pub fn CreateMMMojoEnvironment() -> *mut c_void;
    pub fn SetMMMojoEnvironmentCallbacks(env: *mut c_void, callback_type: c_int, ...);
    pub fn StartMMMojoEnvironment(env: *mut c_void);
    pub fn StopMMMojoEnvironment(env: *mut c_void);

    pub fn GetMMMojoReadInfoRequest(read_info: *const c_void, request_data_size: *mut u32) -> *const c_void;
    pub fn GetMMMojoReadInfoAttach(read_info: *const c_void, attach_data_size: *mut u32) -> *const c_void;
    pub fn GetMMMojoReadInfoMethod(read_info: *const c_void) -> c_int;
    pub fn GetMMMojoReadInfoSync(read_info: *const c_void) -> bool;
    pub fn RemoveMMMojoReadInfo(read_info: *const c_void);

    pub fn CreateMMMojoWriteInfo(method: c_int, sync: bool, request_id: u32) -> *mut c_void;
    pub fn GetMMMojoWriteInfoRequest(write_info: *mut c_void, request_data_size: usize) -> *mut c_void;
    pub fn GetMMMojoWriteInfoAttach(write_info: *mut c_void, attach_data_size: usize) -> *mut c_void;
    pub fn SendMMMojoWriteInfo(env: *mut c_void, write_info: *mut c_void) -> bool;
    pub fn RemoveMMMojoWriteInfo(write_info: *mut c_void);
}
```

---

## 四、 后续分析备忘录

1. **主程序 `wechat` 调用链**：
   * `wechat` 导入了 `libmmmojo.so` 的全部 21 个读写/环境函数。
   * 下一步通过在 Debian 运行抓包（`LD_PRELOAD=librust_bridge.so`），观察微信登录、接收文本、发送文本时，`method` 的具体数值与 payload 的 Protobuf 结构体 tag。
2. **新模块分析规范**：
   * 每次逆向新的 `.so` 或子模块，必须将符号表、虚表结构、字段偏移和关键调用栈记录到本文档对应的章节下。

---

## 五、 主程序 `wechat` 内嵌 Protobuf 协议与消息映射

通过对 `wechat` 主程序常量段（`.rodata`）字符串交叉检索，已确认其内部全套采用腾讯标准的 `micromsg.*` Protobuf 协议（共包含 1658 种类型），核心收发协议如下：

* **发消息请求 (Outbound Request)**：
  * 类型名：`micromsg.SendMsgRequestNew`（新版文本消息发送请求对象）
* **收消息/同步 (Inbound Sync / Push)**：
  * 类型名：`micromsg.AddMsg`（标准消息追加推送，包含 MsgType、FromUserName、ToUserName、Content 等）
  * 现代跨端协议：`micromsg.BypMsgText`、`micromsg.BypMsgPack`（BYP 漫游消息）
* **结论**：Mojo 管道内传输的二进制 Payload 就是标准序列化后的 `micromsg.SendMsgRequestNew` 与 `micromsg.AddMsg`，在 Rust 侧可直接用开源的标准 Protobuf 定义反序列化。

---

## 六、 统一配置与全链路日志规范

为了便于无桌面环境下的故障排查与运行审计，已建立统一的 `config.yaml` 配置与分层落盘日志规范：

### 1. 配置规范 (`config.yaml`)
* 支持单文件集中管理机器人核心参数（`bot`）、大模型调用（`llm`）、群聊白名单（`members`）及日志输出（`logging`）。
* 同时保留对旧版 `bot_config.json` 等独立文件的自动回退兼容。

### 2. 统一日志目录 (`logs/`)
所有组件默认在程序执行目录下统一创建 `logs/` 文件夹：
* **`logs/bot_YYYY-MM-DD.log`**：Go 机器人主程序日志（同时镜像输出至控制台），记录连接握手、插件分发与业务处理。
* **`logs/bridge.log`**：Rust 注入层专用日志，记录 ELF 初始化、Mojo 符号劫持、套接字绑定与二进制数据包捕获。
* **`logs/wechat.log`**：微信后台进程的标准输出与标准错误（由 `start_bot.sh` 捕获）。


