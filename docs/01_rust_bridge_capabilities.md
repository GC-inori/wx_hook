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

---

## 四、 事件捕获与分发清单 (Captured Inbound Events)

通过 `sample_messages` 批处理内存拦截，捕获并主动向 Go 网关推送以下事件：

| 事件名称 (`event`) | 底层微信 `msg_type` | 事件说明与核心字段 |
| :--- | :---: | :--- |
| **`recv_msg` / `recvMsg`** | `1`, `49`, 其他 | 收到好友私聊、群聊发言、系统推文。提取 `from_wxid`、`final_from_wxid`、`text` 以及 `<atuserlist>` 元数据 |
| **`revokeMsg`** | `10002` | 消息防撤回通知。提取被撤回的消息 ID (`revoked_msg_id`) 与会话 (`session`) |
| **`friendReq`** | `37` | 好友申请。解析申请人 `wxid`、昵称、打招呼内容 (`content`)、`ticket` 与 `encryptusername` |
| **`groupMemberChanges`**| `10000` (含特征标记) | 群成员变动通知（进群、退群、移出群聊）。提取群号 `group_wxid` 与通知内容 |
| **`transPay`** | `49` (AppType 2000) | 微信转账通知。提取转账金额 `amount` 与交易单号 |
| **`qrPay`** | `49` (AppType 2001) | 二维码收款通知。提取收款金额 `amount` 与单号 |
| **`systemMsg`** | `10000` | 其它微信系统通知消息 |

---

## 五、 未实现 / 占位接口清单 (501 Not Implemented)

目前以下功能在 `rust_bridge/src/linux/ipc.rs` 中预留了分支，调用时均返回 `retcode: 501 ("Not implemented / In progress")`：

1. **消息撤回**：
   - `revokeMyMsg` / `revoke_msg`：撤回自己在 2 分钟内发出的消息。
2. **列表枚举**：
   - `getGroupList` / `get_group_list`：获取加入的微信群列表。
   - `getMemberList` / `get_member_list`：获取指定群的完整成员列表。
   - `getPublicList` / `get_public_list`：获取已关注的微信公众号列表。
   - `getLabelList` / `get_label_list`：获取联系人标签列表。
3. **关系链与群管理**：
   - `agreeFriendReq` / `agree_friend_req`：程序自动同意好友申请。
   - `delFriend` / `del_friend`：删除指定好友。
   - `createGroup` / `create_group`：拉人建群。
   - `addMembers` / `inviteMembers`：拉好友进群 / 邀请进群。
   - `delMembers`：将群成员移出群聊。
   - `quitGroup`：主动退出微信群。
   - `editSelfMemberNick`：修改群内自己的群昵称。
   - `editObjRemark`：修改好友备注。
