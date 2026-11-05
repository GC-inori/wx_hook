# 1. Rust 桥接层功能与接口清单 (Rust Bridge Capabilities)

> 本文档详细列出 `rust_bridge` (`librust_bridge.so`) 模块当前已实现、部分实现及未实现的功能接口与 IPC 通信协议，供后续扩展、维护和与 Go 网关对接查阅。

---

## 一、 模块架构概述

* **定位**：运行于微信 Linux 原生进程空间内部的注入动态库。
* **编译目标**：`x86_64-unknown-linux-gnu.2.36` (由 `cargo-zigbuild` 编译)。
* **通信机制**：
  * 对外监听 Unix Domain Socket（默认路径 `/tmp/wx_bridge.sock`）；
  * 与 Go 守护进程 (`wxbot`) 采用双工 JSON-RPC 风格协议交互。

---

## 二、 IPC 请求响应协议规范

### 1. 通信格式
- **请求包**：
  ```json
  {
    "id": 1001,
    "action": "sendText",
    "data": {
      "target": "wxid_xxxx",
      "text": "Hello WeChat"
    }
  }
  ```
- **响应包**：
  ```json
  {
    "id": 1001,
    "retcode": 0,
    "message": "",
    "data": {
      "sent": true,
      "target": "wxid_xxxx"
    }
  }
  ```
- **状态码定义 (`retcode`)**：
  - `0`：成功 (Success)
  - `1`：参数错误 / 解析失败 (Invalid Argument)
  - `2`：底层调用执行异常 (Execution Error)
  - `3`：账号未就绪 / 未登录 (Not Ready)
  - `4`：缓存或目标未找到 (Not Found)
  - `501`：未实现 / 占位阶段 (Not Implemented)

---

## 三、 已完整实现功能清单 (Implemented Actions)

| 动作标识 (`action`) | 兼容别名 | 职责与实现方式 | 输入参数 | 返回数据 |
| :--- | :--- | :--- | :--- | :--- |
| **`checkWeChat`** | `ping` | 进程与套接字保活检测 | 无 | `{"socket": "wx_bridge"}` |
| **`getSelfInfo`** | `get_account`, `get_self_info` | 获取当前登录微信账号基本信息 | 无 | `{"account": "wxid", "name": "昵称"}` |
| **`getMemberNick`** | `getContactName` | 根据 wxid 查询联系人或群成员昵称缓存 | `member_wxid` 或 `wxid` | `{"name": "昵称", "nickname": "昵称"}` |
| **`sendText`** | `send_text` | 发送单人好友或群聊普通纯文本消息 | `target`, `text` | `{"sent": true, "target": "..."}` |
| **`sendText2`** | `send_at_text` | 发送群聊 @ 提及消息（注入 `atuserlist`） | `target`, `text`, `members` 或 `at_list` | `{"sent": true, "members": [...]}` |
| **`sendImage`** | `send_image` | 发送图片文件（msg_type: 3） | `target`, `image` (本地路径) | `{"sent": true}` |
| **`sendFile`** | `send_file` | 发送普通文件（msg_type: 6） | `target`, `file` (本地路径) | `{"sent": true}` |
| **`sendGif`** | `send_gif` | 发送表情动图（msg_type: 47） | `target`, `gif` (本地路径) | `{"sent": true}` |
| **`sendCard`** | `send_card` | 发送名片推荐卡片（msg_type: 42） | `target`, `card` | `{"sent": true}` |
| **`sendShareUrl`**| `send_share_url`| 发送网页分享链接卡片（msg_type: 5） | `target`, `url`, `title`, `desc` | `{"sent": true}` |
| **`sendApplet`** | `send_applet` | 发送微信小程序卡片（msg_type: 33） | `target`, `applet` | `{"sent": true}` |
| **`sendReferText`**| `send_refer_text`| 发送带引用的回复文本（msg_type: 57） | `target`, `text`, `refer` | `{"sent": true}` |
| **`sendXml`** | `send_xml` | 发送自定义 XML 卡片原生载荷 | `target`, `xml` | `{"sent": true}` |
| **`sendMessage`** | `send_message` | 通用富媒体发送入口（按 msg_type 分发） | `target`, `msg_type`, `content` | `{"sent": true}` |
| **`send_raw`** | - | Mojo 底层原始字节流投递 | `method`, `sync`, `request_id`, `payload` | `{"sent": true, "bytes": N}` |
| **`agreeFriendReq`** | `agree_friend_req` | 使用微信 `VerifyUser` 函数提交同意好友申请请求（opcode 2）；在微信主线程执行 | `wxid`/`user_id`, `ticket`/`flag`, `encryptusername`, `scene`（默认 6）, `remark` | `{"agreed": true, "wxid": "..."}` |
| **`delFriend`** | `delete_friend`, `del_friend` | 获取联系人 Service 并调用微信联系人删除函数；共享指针与 C++ 字符串在调用后释放，微信主线程执行 | `wxid` 或 `user_id` | `{"deleted": true, "wxid": "..."}` |
| **`createGroup`** | `create_group` | 通过 Chatroom Service 与 libc++ vector ABI 拉人建群；主线程调用并释放服务共享指针、成员字符串向量和结果字符串 | `user_list` / `member_list` / `users`, `topic` / `name` | `{"created": true, "chatroom_name": "...@chatroom"}` |
| **`addMembers`** | `add_members`, `invite_members` | 调用 Chatroom Service 邀请联系人加入群聊；主线程执行并释放输入向量及结果字符串 | `chatroom_name` / `group_id`, `user_list` / `member_list` | `{"added": true, "chatroom_name": "..."}` |
| **`delMembers`** | `del_members` | 调用 Chatroom Service 移出指定群成员；主线程执行并释放成员向量、结果缓冲与服务共享指针 | `chatroom_name` / `group_id`, `user_list` / `member_list` | `{"deleted": true, "chatroom_name": "..."}` |
| **`quitGroup`** | `quit_group` | 通过 Oplog Service 提交主动退群请求；主线程执行并释放共享指针和 C++ 字符串 | `chatroom_name` / `group_id` | `{"quit": true, "chatroom_name": "..."}` |
| **`editSelfMemberNick`** | `edit_self_member_nick` | 通过 Oplog Service 修改当前账号的群内昵称；主线程执行 | `chatroom_name` / `group_id`, `nick` / `nickname` | `{"updated": true, "chatroom_name": "...", "nick": "..."}` |
| **`editObjRemark`** | `edit_obj_remark`, `edit_remark` | 构造 64 字节备注对象并调用微信好友备注函数；主线程执行并释放两条 C++ 字符串 | `wxid` / `user_id`, `remark` | `{"updated": true, "wxid": "...", "remark": "..."}` |
| **`revoke_msg`** | `revokeMyMsg`, `revoke_my_msg` | 构造 72 字节 `RevokeMsgRequest` 并通过网络服务提交撤回 CGI；在微信主线程执行并释放请求字符串 | `to_user` / `session` / `group_id` / `wxid`, `client_msg_id` / `msg_id`, `svr_msg_id`, `create_time` | `{"revoked": true, "to_user": "...", "client_msg_id": "..."}` |
| **`getGroupList`** | `get_group_list` | 获取联系人存储服务并枚举本地群聊；只返回群 ID 标识，不读取业务策略 | 无 | 群列表数组，成员含 `chatroom_name` / `wxid` |
| **`getFriendList`** | `get_friend_list` | 从联系人存储枚举普通联系人并补充本地昵称缓存 | 无 | 好友列表数组，成员含 `wxid` / `nickname` |
| **`getMemberList`** | `get_member_list` | 获取群元数据与成员 wxid 列表；昵称使用本地缓存；查询与遍历在微信主线程进行 | `chatroom_name` / `group_id` | 群对象，含 `owner_wxid`、`member_count` 与 `members` |
| **`confirmTrans`** | `confirm_trans` | 使用微信 `CO_AcceptPayTransfer` 确认收取转账；在微信主线程执行 | `trans_id`/`transfer_id`, `invalid_time`, `transfer_attach`（可选） | `{"confirmed": true, "trans_id": "..."}` |
| **`receiveHongBao`** | `receive_hong_bao` | 调用 CGI 1581 查询并预备拆红包，返回微信 JSON 字符串解析后的对象 | `send_id`, `channel_id`, `native_url`, `session_wxid`, `in_way` | 包含 `retcode`, `hbStatus`, `timingIdentifier` 等 |
| **`openHongBao`** | `open_hong_bao` | 携带第一阶段 `timingIdentifier` 调用 CGI 1685 拆红包 | `channel_id`, `native_url`, `session_wxid`, `timing_identifier`, `nick_name` | 包含 `retcode`, `amount`（分）, `receiveId` 等 |
| **`get_db_key`** | `getDbKey` | 提取当前登录账号 WCDB 本地数据库 32 字节 AES 密钥与路径信息；主线程执行 | 无 | `{"key": "64位十六进制", "page_size": 4096, "cipher_version": 4, "wxid": "...", "db_dir": "..."}` |
| **`queryObj`** | `query_obj` | 查询本地对象/消息元数据与数据库绑定状态 | `msg_id` (可选), `type` (可选) | 包含数据库密钥与元数据或查询路由结果 |

---

## 四、 事件捕获与分发清单 (Captured Inbound Events)

通过 `sample_messages` 批处理内存拦截，捕获并主动向 Go 网关推送以下事件：

| 事件名称 (`event`) | 底层微信 `msg_type` | 事件说明与核心字段 |
| :--- | :---: | :--- |
| **`recv_msg` / `recvMsg`** | `1`, `49`, 其他 | 收到好友私聊、群聊发言、系统推文。提取 `from_wxid`、`final_from_wxid`、`text` 以及 `<atuserlist>` 元数据 |
| **`revokeMsg`** | `10002` | 消息防撤回通知。提取被撤回的消息 ID (`revoked_msg_id`) 与会话 (`session`) |
| **`friendReq`** | `37` | 好友申请。解析申请人 `wxid`、昵称、打招呼内容 (`content`)、`ticket` 与 `encryptusername` |
| **`groupMemberChanges`**| `10000` (含特征标记) | 群成员变动通知（进群、退群、移出群聊）。提取群号 `group_wxid` 与通知内容 |
| **`transPay`** | `49` (AppType 2000) | 微信转账通知。提取转账金额、交易单号、失效时间 `invalid_time` 与附加字段 `transfer_attach` |
| **`qrPay`** | `49` (AppType 2001) | 二维码收款通知。提取收款金额 `amount` 与单号 |
| **`hongBao`** | `49` (AppType 2001) | 红包通知。提取 `native_url`、祝福语 `title`、`pay_msg_id`、`send_id` 与 `channel_id`；普通收款仍归类为 `qrPay` |
| **`systemMsg`** | `10000` | 其它微信系统通知消息 |

---

## 五、未实现 / 占位接口清单 (501 Not Implemented)

以下接口仍返回 `retcode: 501 (Not implemented / In progress)`：

1. **联系人扩展列表**：`getPublicList` / `get_public_list`、`getLabelList` / `get_label_list`。
2. **查询与工具接口**：`queryGroup`、`returnTrans`、`setReadStatus`、图像解密、登录状态与二维码、浏览器控制及云函数等。

消息撤回、群列表、好友列表、群成员列表及本地数据库密钥提取 (`get_db_key` / `queryObj`) 已接入 Rust 原子接口，不再属于 501 占位接口。
