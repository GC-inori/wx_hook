use super::logger::{bridge_error, bridge_info, bridge_warn};
use super::MAX_CAPTURE_BYTES;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::io::{self, BufRead, BufReader, Write};
use std::os::unix::fs::{FileTypeExt, PermissionsExt};
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::{self, Receiver, SyncSender};
use std::sync::{Arc, Mutex, OnceLock};
use std::thread;
use std::time::Duration;

const DEFAULT_SOCKET_PATH: &str = "/tmp/wx_bridge.sock";
const MAX_REQUEST_LINE: usize = 3 * 1024 * 1024;
const WRITE_TIMEOUT: Duration = Duration::from_millis(100);

#[derive(Deserialize)]
struct IpcRequest {
    id: Option<Value>,
    #[serde(default)]
    action: Option<String>,
    #[serde(default, rename = "type")]
    type_name: Option<String>,
    data: Option<Value>,
}

#[derive(Serialize)]
struct IpcResponse {
    #[serde(skip_serializing_if = "Option::is_none")]
    id: Option<Value>,
    status: i32,
    code: i32,
    msg: String,
    error: String,
    data: Option<Value>,
    result: Option<Value>,
}

fn make_response(id: Option<Value>, status: i32, error: &str, data: Option<Value>) -> IpcResponse {
    let (code, msg) = match status {
        0 => (200, "操作成功"),
        -1 | 1 => (400, error),
        3 => (401, "Account not ready or not logged in"),
        4 | 404 => (404, "Not found"),
        501 => (501, "Not implemented / In progress"),
        _ => (500, error),
    };
    IpcResponse {
        id,
        status,
        code,
        msg: msg.to_owned(),
        error: if status == 0 {
            String::new()
        } else {
            error.to_owned()
        },
        result: data.clone(),
        data,
    }
}

struct ConnectedClient {
    writer: Mutex<UnixStream>,
}

static CONNECTED_CLIENTS: Mutex<Vec<Arc<ConnectedClient>>> = Mutex::new(Vec::new());
static EVENT_SENDER: OnceLock<SyncSender<String>> = OnceLock::new();
static SERVER_RUNNING: AtomicBool = AtomicBool::new(false);

/// Queues one newline-delimited JSON event for all connected socket clients.
pub fn broadcast_event<T: Serialize>(event: &str, data: &T) {
    let Ok(line) = serde_json::to_string(&json!({ "event": event, "data": data })) else {
        return;
    };
    if let Some(sender) = EVENT_SENDER.get() {
        // Non-blocking try_send so slow socket consumers never stall host threads/coroutines
        let _ = sender.try_send(line);
    }
}

/// Starts the Unix-domain-socket listener once per loaded bridge instance.
pub fn start_unix_server() {
    if SERVER_RUNNING
        .compare_exchange(false, true, Ordering::AcqRel, Ordering::Acquire)
        .is_err()
    {
        return;
    }

    if EVENT_SENDER.get().is_none() {
        let (sender, receiver) = mpsc::sync_channel(2048);
        if let Err(error) = thread::Builder::new()
            .name("wx-bridge-events".to_owned())
            .spawn(move || dispatch_events(receiver))
        {
            bridge_error!("cannot start event dispatcher: {error}");
            SERVER_RUNNING.store(false, Ordering::Release);
            return;
        }
        let _ = EVENT_SENDER.set(sender);
    }

    if let Err(error) = thread::Builder::new()
        .name("wx-bridge-socket".to_owned())
        .spawn(run_listener)
    {
        bridge_error!("cannot start socket listener: {error}");
        SERVER_RUNNING.store(false, Ordering::Release);
    }
}

fn socket_path() -> PathBuf {
    std::env::var_os("WX_BRIDGE_SOCKET")
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from(DEFAULT_SOCKET_PATH))
}

fn prepare_socket_path(path: &Path) -> io::Result<()> {
    match std::fs::symlink_metadata(path) {
        Ok(metadata) if metadata.file_type().is_socket() => match UnixStream::connect(path) {
            Ok(_) => {
                return Err(io::Error::new(
                    io::ErrorKind::AddrInUse,
                    format!("another bridge is listening at {}", path.display()),
                ));
            }
            Err(error) if error.kind() == io::ErrorKind::ConnectionRefused => {
                std::fs::remove_file(path)?;
            }
            Err(error) if error.kind() == io::ErrorKind::NotFound => {}
            Err(error) => return Err(error),
        },
        Ok(_) => {
            return Err(io::Error::new(
                io::ErrorKind::AlreadyExists,
                format!("refusing to replace non-socket path {}", path.display()),
            ));
        }
        Err(error) if error.kind() == io::ErrorKind::NotFound => {}
        Err(error) => return Err(error),
    }
    Ok(())
}

fn run_listener() {
    let path = socket_path();
    if let Err(error) = prepare_socket_path(&path) {
        bridge_error!("cannot prepare socket {}: {error}", path.display());
        SERVER_RUNNING.store(false, Ordering::Release);
        return;
    }

    let listener = match UnixListener::bind(&path) {
        Ok(listener) => {
            bridge_info!("Unix socket successfully bound at {}", path.display());
            listener
        }
        Err(error) => {
            bridge_error!("cannot bind socket {}: {error}", path.display());
            SERVER_RUNNING.store(false, Ordering::Release);
            return;
        }
    };
    if let Err(error) = std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o600)) {
        bridge_error!("cannot set socket permissions: {error}");
        let _ = std::fs::remove_file(&path);
        SERVER_RUNNING.store(false, Ordering::Release);
        return;
    }

    for incoming in listener.incoming() {
        if !SERVER_RUNNING.load(Ordering::Acquire) {
            break;
        }
        match incoming {
            Ok(stream) => {
                if lock_clients().len() >= 128 {
                    bridge_warn!("client limit reached; rejecting socket connection");
                    continue;
                }
                bridge_info!("New client connected to bridge socket");
                if let Err(error) = stream.set_write_timeout(Some(WRITE_TIMEOUT)) {
                    bridge_error!("cannot configure client socket: {error}");
                    continue;
                }
                let writer = match stream.try_clone() {
                    Ok(writer) => writer,
                    Err(error) => {
                        bridge_error!("cannot clone client socket: {error}");
                        continue;
                    }
                };
                let client = Arc::new(ConnectedClient {
                    writer: Mutex::new(writer),
                });
                lock_clients().push(client.clone());
                let client_for_thread = client.clone();
                if let Err(error) = thread::Builder::new()
                    .name("wx-bridge-client".to_owned())
                    .spawn(move || handle_client(stream, client_for_thread))
                {
                    bridge_error!("cannot start client reader: {error}");
                    lock_clients().retain(|connected| !Arc::ptr_eq(connected, &client));
                }
            }
            Err(error) => {
                bridge_error!("socket accept failed: {error}");
                thread::sleep(Duration::from_millis(100));
            }
        }
    }

    bridge_warn!(
        "Socket listener exiting, removing socket: {}",
        path.display()
    );
    let _ = std::fs::remove_file(path);
    SERVER_RUNNING.store(false, Ordering::Release);
}

fn dispatch_events(receiver: Receiver<String>) {
    for line in receiver {
        let clients = lock_clients().clone();
        let mut dead_clients = Vec::new();
        for client in clients {
            if write_json_line(&client, line.as_bytes()).is_err() {
                dead_clients.push(client);
            }
        }
        if !dead_clients.is_empty() {
            let mut clients = lock_clients();
            clients.retain(|client| !dead_clients.iter().any(|dead| Arc::ptr_eq(client, dead)));
        }
    }
}

fn handle_client(stream: UnixStream, client: Arc<ConnectedClient>) {
    let mut reader = BufReader::new(stream);
    loop {
        let mut line = Vec::new();
        match read_bounded_line(&mut reader, &mut line) {
            Ok(0) => break,
            Ok(_) => {}
            Err(error) => {
                let response = make_response(
                    None,
                    -1,
                    &format!("invalid or oversized request: {error}"),
                    None,
                );
                let _ = write_response(&client, &response);
                break;
            }
        }

        let request: IpcRequest = match serde_json::from_slice(&line) {
            Ok(request) => request,
            Err(error) => {
                let response = make_response(None, -1, &format!("invalid JSON: {error}"), None);
                if write_response(&client, &response).is_err() {
                    break;
                }
                continue;
            }
        };

        let response = process_request(request);
        if write_response(&client, &response).is_err() {
            break;
        }
    }

    lock_clients().retain(|connected| !Arc::ptr_eq(connected, &client));
}

fn read_bounded_line<R: BufRead>(reader: &mut R, output: &mut Vec<u8>) -> io::Result<usize> {
    output.clear();
    loop {
        let available = reader.fill_buf()?;
        if available.is_empty() {
            return Ok(output.len());
        }
        let count = available
            .iter()
            .position(|byte| *byte == b'\n')
            .map_or(available.len(), |index| index + 1);
        if output.len().saturating_add(count) > MAX_REQUEST_LINE {
            reader.consume(count);
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                "request line exceeds configured limit",
            ));
        }
        output.extend_from_slice(&available[..count]);
        let finished = available[count - 1] == b'\n';
        reader.consume(count);
        if finished {
            return Ok(output.len());
        }
    }
}

fn process_request(request: IpcRequest) -> IpcResponse {
    let id = request.id;
    let action_str = request
        .action
        .as_deref()
        .or(request.type_name.as_deref())
        .unwrap_or_default();

    match action_str {
        "ping" | "checkWeChat" => make_response(id, 0, "", Some(json!({ "socket": "wx_bridge" }))),
        "get_account" | "get_self_info" | "getSelfInfo" => match super::account::get_self_account()
        {
            Some((wxid, name)) => make_response(
                id,
                0,
                "",
                Some(json!({
                    "account": wxid,
                    "wxid": wxid,
                    "name": name,
                    "nickname": name,
                })),
            ),
            None => make_response(id, 3, "Account not ready or not logged in", None),
        },
        "get_contact_name" | "getContactName" | "get_member_nick" | "getMemberNick" => {
            let wxid = request
                .data
                .as_ref()
                .and_then(|d| d.get("member_wxid").or_else(|| d.get("wxid")))
                .and_then(Value::as_str)
                .unwrap_or_default();

            if wxid.is_empty() {
                make_response(id, 1, "wxid is empty", None)
            } else {
                match super::account::get_contact_name(wxid) {
                    Some(name) => {
                        make_response(id, 0, "", Some(json!({ "name": name, "nickname": name })))
                    }
                    None => make_response(id, 4, "Contact not found in cache", None),
                }
            }
        }
        "send_text" | "sendText" => match parse_send_text(request.data.as_ref()) {
            Ok((target, text, at_list)) => {
                match super::sender::send_text_with_at(&target, &text, at_list.as_deref()) {
                    Ok(()) => {
                        make_response(id, 0, "", Some(json!({ "sent": true, "target": target })))
                    }
                    Err(error) => make_response(id, 1, &error, None),
                }
            }
            Err(error) => make_response(id, 1, &error, None),
        },
        "send_at_text" | "sendText2" => match parse_send_at_text(request.data.as_ref()) {
            Ok((target, text, members)) => {
                match super::sender::send_text_with_at(&target, &text, Some(&members)) {
                    Ok(()) => make_response(
                        id,
                        0,
                        "",
                        Some(json!({ "sent": true, "target": target, "members": members })),
                    ),
                    Err(error) => make_response(id, 1, &error, None),
                }
            }
            Err(error) => make_response(id, 1, &error, None),
        },
        action if rich_message_type(action).is_some() => {
            match parse_named_rich_message(action, request.data.as_ref()) {
                Ok((target, msg_type, content, is_base64, subtype)) => {
                    match super::sender::send_rich_message(
                        &target, msg_type, &content, is_base64, subtype,
                    ) {
                        Ok(()) => make_response(
                            id,
                            0,
                            "",
                            Some(json!({ "sent": true, "target": target, "msg_type": msg_type })),
                        ),
                        Err(error) => make_response(id, 2, &error, None),
                    }
                }
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "send_xml" | "sendXml" => match parse_send_xml(request.data.as_ref()) {
            Ok((target, xml)) => match super::sender::send_xml(&target, &xml) {
                Ok(()) => make_response(id, 0, "", Some(json!({ "sent": true, "target": target }))),
                Err(error) => make_response(id, 1, &error, None),
            },
            Err(error) => make_response(id, 1, &error, None),
        },
        "send_message" | "sendMessage" => match parse_send_message(request.data.as_ref()) {
            Ok((target, msg_type, content, is_base64, subtype)) => {
                match super::sender::send_rich_message(
                    &target, msg_type, &content, is_base64, subtype,
                ) {
                    Ok(()) => make_response(
                        id,
                        0,
                        "",
                        Some(json!({ "sent": true, "target": target, "msg_type": msg_type })),
                    ),
                    Err(error) => make_response(id, 1, &error, None),
                }
            }
            Err(error) => make_response(id, 1, &error, None),
        },
        "send_raw" => match parse_send_raw(request.data.as_ref()) {
            Ok((method, sync, request_id, payload)) => {
                let result = unsafe { super::send_raw(method, sync, request_id, &payload) };
                match result {
                    Ok(sent) => make_response(
                        id,
                        if sent { 0 } else { 2 },
                        if sent {
                            ""
                        } else {
                            "SendMMMojoWriteInfo returned false"
                        },
                        Some(json!({ "sent": sent, "bytes": payload.len() })),
                    ),
                    Err(error) => make_response(id, 2, &error, None),
                }
            }
            Err(error) => make_response(id, 1, &error, None),
        },
        "agree_friend_req" | "agreeFriendReq" => {
            match parse_agree_friend_req(request.data.as_ref()) {
                Ok((wxid, ticket, encryptusername, scene, remark)) => {
                    match super::sender::agree_friend_req(
                        &wxid,
                        &ticket,
                        &encryptusername,
                        scene,
                        &remark,
                    ) {
                        Ok(()) => {
                            make_response(id, 0, "", Some(json!({ "agreed": true, "wxid": wxid })))
                        }
                        Err(error) => make_response(id, 1, &error, None),
                    }
                }
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "confirm_trans" | "confirmTrans" => match parse_confirm_transfer(request.data.as_ref()) {
            Ok((trans_id, invalid_time, transfer_attach)) => {
                match super::sender::confirm_transfer(&trans_id, invalid_time, &transfer_attach) {
                    Ok(()) => make_response(
                        id,
                        0,
                        "",
                        Some(json!({ "confirmed": true, "trans_id": trans_id })),
                    ),
                    Err(error) => make_response(id, 1, &error, None),
                }
            }
            Err(error) => make_response(id, 1, &error, None),
        },
        "receiveHongBao" | "receive_hong_bao" => {
            match parse_receive_hongbao(request.data.as_ref()) {
                Ok((send_id, channel_id, native_url, session_wxid, in_way)) => {
                    match super::sender::receive_hongbao(
                        &send_id,
                        &channel_id,
                        &native_url,
                        &session_wxid,
                        in_way,
                    ) {
                        Ok(response) => match serde_json::from_str::<Value>(&response) {
                            Ok(value) => make_response(id, 0, "", Some(value)),
                            Err(error) => make_response(
                                id,
                                1,
                                &format!("invalid HongBaoRes JSON: {error}"),
                                None,
                            ),
                        },
                        Err(error) => make_response(id, 1, &error, None),
                    }
                }
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "openHongBao" | "open_hong_bao" => match parse_open_hongbao(request.data.as_ref()) {
            Ok((channel_id, native_url, session_wxid, timing_identifier, nick_name)) => {
                match super::sender::open_hongbao(
                    &channel_id,
                    &native_url,
                    &session_wxid,
                    &timing_identifier,
                    &nick_name,
                ) {
                    Ok(response) => match serde_json::from_str::<Value>(&response) {
                        Ok(value) => make_response(id, 0, "", Some(value)),
                        Err(error) => {
                            make_response(id, 1, &format!("invalid HongBaoRes JSON: {error}"), None)
                        }
                    },
                    Err(error) => make_response(id, 1, &error, None),
                }
            }
            Err(error) => make_response(id, 1, &error, None),
        },
        "delete_friend" | "del_friend" | "delFriend" => {
            match parse_delete_friend(request.data.as_ref()) {
                Ok(wxid) => match super::sender::del_friend(&wxid) {
                    Ok(()) => make_response(
                        id,
                        0,
                        "success",
                        Some(json!({ "deleted": true, "wxid": wxid })),
                    ),
                    Err(error) => make_response(id, 1, &error, None),
                },
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "revoke_my_msg" | "revokeMyMsg" | "revoke_msg" => {
            match parse_revoke_msg(request.data.as_ref()) {
                Ok((to_user, client_msg_id, svr_msg_id, create_time)) => {
                    match super::sender::revoke_msg(
                        &to_user,
                        &client_msg_id,
                        svr_msg_id,
                        create_time,
                    ) {
                        Ok(()) => make_response(
                            id,
                            0,
                            "success",
                            Some(json!({
                                "revoked": true,
                                "to_user": to_user,
                                "client_msg_id": client_msg_id,
                                "svr_msg_id": svr_msg_id,
                            })),
                        ),
                        Err(error) => make_response(id, 1, &error, None),
                    }
                }
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "get_member_list" | "getMemberList" => match parse_chatroom_name(request.data.as_ref()) {
            Ok(room) => match super::sender::get_member_list(&room) {
                Ok(members) => make_response(id, 0, "success", Some(members)),
                Err(error) => make_response(id, 1, &error, None),
            },
            Err(error) => make_response(id, 1, &error, None),
        },
        "get_group_list" | "getGroupList" => match super::sender::get_group_list() {
            Ok(groups) => make_response(id, 0, "success", Some(groups)),
            Err(error) => make_response(id, 1, &error, None),
        },
        "get_friend_list" | "getFriendList" => match super::sender::get_friend_list() {
            Ok(friends) => make_response(id, 0, "success", Some(friends)),
            Err(error) => make_response(id, 1, &error, None),
        },
        "create_group" | "createGroup" => match parse_create_group(request.data.as_ref()) {
            Ok((members, topic)) => match super::sender::create_group(&members, &topic) {
                Ok(room_id) => make_response(
                    id,
                    0,
                    "success",
                    Some(json!({ "created": true, "chatroom_name": room_id })),
                ),
                Err(error) => make_response(id, 1, &error, None),
            },
            Err(error) => make_response(id, 1, &error, None),
        },
        "add_members" | "addMembers" | "invite_members" | "inviteMembers" => {
            match parse_chatroom_members(request.data.as_ref()) {
                Ok((room, members)) => match super::sender::add_members(&room, &members) {
                    Ok(()) => make_response(
                        id,
                        0,
                        "success",
                        Some(json!({ "added": true, "chatroom_name": room })),
                    ),
                    Err(error) => make_response(id, 1, &error, None),
                },
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "del_members" | "delMembers" => match parse_chatroom_members(request.data.as_ref()) {
            Ok((room, members)) => match super::sender::del_members(&room, &members) {
                Ok(()) => make_response(
                    id,
                    0,
                    "success",
                    Some(json!({ "deleted": true, "chatroom_name": room })),
                ),
                Err(error) => make_response(id, 1, &error, None),
            },
            Err(error) => make_response(id, 1, &error, None),
        },
        "quit_group" | "quitGroup" => match parse_chatroom_name(request.data.as_ref()) {
            Ok(room) => match super::sender::quit_group(&room) {
                Ok(()) => make_response(
                    id,
                    0,
                    "success",
                    Some(json!({ "quit": true, "chatroom_name": room })),
                ),
                Err(error) => make_response(id, 1, &error, None),
            },
            Err(error) => make_response(id, 1, &error, None),
        },
        "edit_self_member_nick" | "editSelfMemberNick" => {
            match parse_edit_self_member_nick(request.data.as_ref()) {
                Ok((room, nick)) => match super::sender::edit_self_member_nick(&room, &nick) {
                    Ok(()) => make_response(
                        id,
                        0,
                        "success",
                        Some(json!({ "updated": true, "chatroom_name": room, "nick": nick })),
                    ),
                    Err(error) => make_response(id, 1, &error, None),
                },
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "edit_obj_remark" | "editObjRemark" | "edit_remark" => {
            match parse_edit_obj_remark(request.data.as_ref()) {
                Ok((wxid, remark)) => match super::sender::edit_obj_remark(&wxid, &remark) {
                    Ok(()) => make_response(
                        id,
                        0,
                        "success",
                        Some(json!({ "updated": true, "wxid": wxid, "remark": remark })),
                    ),
                    Err(error) => make_response(id, 1, &error, None),
                },
                Err(error) => make_response(id, 1, &error, None),
            }
        }
        "get_public_list" | "getPublicList" | "get_label_list" | "getLabelList" | "query_obj"
        | "queryObj" | "query_group" | "queryGroup" | "return_trans" | "returnTrans"
        | "set_read_status" | "setReadStatus" | "decrypt_image" | "decryptImage"
        | "kill_wechat" | "killWeChat" | "edit_version" | "editVersion" | "get_login_status"
        | "getLoginStatus" | "get_login_qr_code" | "getLoginQrCode" | "set_download_image"
        | "setDownloadImage" | "get_auth_info" | "getAuthInfo" | "open_browser" | "openBrowser"
        | "run_cloud_function" | "runCloudFunction" | "auth_cami" | "authCami" => make_response(
            id,
            501,
            "Not implemented / In progress",
            Some(json!({ "action": action_str, "implemented": false })),
        ),
        other => make_response(id, 4, &format!("unknown action: {other}"), None),
    }
}

fn parse_agree_friend_req(
    data: Option<&Value>,
) -> Result<(String, String, String, u32, String), String> {
    let data = data.ok_or_else(|| "data is required".to_owned())?;
    let wxid = data
        .get("wxid")
        .or_else(|| data.get("user_id"))
        .and_then(Value::as_str)
        .unwrap_or_default()
        .trim()
        .to_owned();
    if wxid.is_empty() {
        return Err("data.wxid / data.user_id is required".to_owned());
    }
    let ticket = data
        .get("ticket")
        .or_else(|| data.get("flag"))
        .and_then(Value::as_str)
        .unwrap_or_default()
        .trim()
        .to_owned();
    if ticket.is_empty() {
        return Err("data.ticket / data.flag is required".to_owned());
    }
    let encryptusername = data
        .get("encryptusername")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned();
    let scene = match data.get("scene") {
        None | Some(Value::Null) => 6,
        Some(Value::Number(value)) => value
            .as_u64()
            .and_then(|value| u32::try_from(value).ok())
            .ok_or_else(|| "data.scene must be a non-negative 32-bit integer".to_owned())?,
        Some(Value::String(value)) if value.trim().is_empty() => 6,
        Some(Value::String(value)) => value
            .trim()
            .parse::<u32>()
            .map_err(|_| "data.scene must be a non-negative 32-bit integer".to_owned())?,
        Some(_) => return Err("data.scene must be a non-negative 32-bit integer".to_owned()),
    };
    let remark = data
        .get("remark")
        .or_else(|| data.get("content"))
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned();
    Ok((wxid, ticket, encryptusername, scene, remark))
}

fn parse_revoke_msg(data: Option<&Value>) -> Result<(String, String, u64, u32), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "revoke_msg requires a data object".to_owned())?;
    let to_user = ["to_user", "session", "group_id", "wxid", "user_id"]
        .iter()
        .find_map(|key| {
            data.get(*key)
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
        })
        .ok_or_else(|| "data.to_user / session / group_id / wxid is required".to_owned())?
        .to_owned();
    let client_msg_id = ["client_msg_id", "msg_id", "message_id", "clientMsgId"]
        .iter()
        .find_map(|key| match data.get(*key) {
            Some(Value::String(value)) if !value.trim().is_empty() => Some(value.trim().to_owned()),
            Some(Value::Number(value)) => Some(value.to_string()),
            _ => None,
        })
        .ok_or_else(|| "data.client_msg_id / msg_id is required".to_owned())?;
    let svr_msg_id = parse_optional_u64(
        data.get("svr_msg_id").or_else(|| data.get("svrMsgId")),
        "svr_msg_id",
    )?;
    let create_time = parse_optional_u64(
        data.get("create_time").or_else(|| data.get("createTime")),
        "create_time",
    )?;
    let create_time = u32::try_from(create_time)
        .map_err(|_| "data.create_time must be a 32-bit unsigned integer".to_owned())?;
    Ok((to_user, client_msg_id, svr_msg_id, create_time))
}

fn parse_optional_u64(value: Option<&Value>, name: &str) -> Result<u64, String> {
    match value {
        None | Some(Value::Null) => Ok(0),
        Some(Value::Number(number)) => number
            .as_u64()
            .ok_or_else(|| format!("data.{name} must be a non-negative integer")),
        Some(Value::String(text)) => text
            .trim()
            .parse::<u64>()
            .map_err(|_| format!("data.{name} must be a non-negative integer")),
        Some(_) => Err(format!("data.{name} must be a non-negative integer")),
    }
}

fn parse_delete_friend(data: Option<&Value>) -> Result<String, String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "delete_friend requires a data object".to_owned())?;
    let wxid = ["wxid", "user_id"]
        .iter()
        .find_map(|key| {
            data.get(*key)
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
        })
        .ok_or_else(|| "data.wxid / data.user_id is required".to_owned())?
        .to_owned();
    Ok(wxid)
}

fn parse_chatroom_name(data: Option<&Value>) -> Result<String, String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "chatroom action requires a data object".to_owned())?;
    Ok(required_string(data, &["chatroom_name", "group_id"])?
        .trim()
        .to_owned())
}

fn parse_native_string_list(data: &Value, keys: &[&str]) -> Result<Vec<String>, String> {
    let (field, value) = keys
        .iter()
        .find_map(|key| data.get(*key).map(|value| (*key, value)))
        .ok_or_else(|| format!("data requires one of: {}", keys.join(" / ")))?;
    let values = if let Some(value) = value.as_str() {
        value.split(',').map(str::to_owned).collect::<Vec<_>>()
    } else if let Some(values) = value.as_array() {
        values
            .iter()
            .map(|value| {
                value
                    .as_str()
                    .map(str::to_owned)
                    .ok_or_else(|| format!("data.{field} must contain only strings"))
            })
            .collect::<Result<Vec<_>, _>>()?
    } else {
        return Err(format!("data.{field} must be a string or string array"));
    };
    let values = values
        .into_iter()
        .map(|value| value.trim().to_owned())
        .collect::<Vec<_>>();
    if values.is_empty() || values.iter().any(String::is_empty) {
        return Err(format!("data.{field} must contain non-empty strings"));
    }
    Ok(values)
}

fn parse_chatroom_members(data: Option<&Value>) -> Result<(String, Vec<String>), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "chatroom member action requires a data object".to_owned())?;
    let room = required_string(data, &["chatroom_name", "group_id"])?
        .trim()
        .to_owned();
    let members = parse_native_string_list(data, &["user_list", "member_list"])?;
    Ok((room, members))
}

fn parse_create_group(data: Option<&Value>) -> Result<(Vec<String>, String), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "create_group requires a data object".to_owned())?;
    let members = parse_native_string_list(data, &["user_list", "member_list", "users"])?;
    let topic = optional_string(data, &["topic", "name"])
        .unwrap_or_default()
        .to_owned();
    Ok((members, topic))
}

fn parse_edit_self_member_nick(data: Option<&Value>) -> Result<(String, String), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "edit_self_member_nick requires a data object".to_owned())?;
    let room = required_string(data, &["chatroom_name", "group_id"])?
        .trim()
        .to_owned();
    let nick = required_string(data, &["nick", "nickname"])?.to_owned();
    Ok((room, nick))
}

fn parse_edit_obj_remark(data: Option<&Value>) -> Result<(String, String), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "edit_obj_remark requires a data object".to_owned())?;
    let wxid = required_string(data, &["wxid", "user_id"])?
        .trim()
        .to_owned();
    let remark = data
        .get("remark")
        .and_then(Value::as_str)
        .ok_or_else(|| "data.remark must be a string".to_owned())?
        .to_owned();
    Ok((wxid, remark))
}

fn parse_receive_hongbao(
    data: Option<&Value>,
) -> Result<(String, String, String, String, u32), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "receiveHongBao requires a data object".to_owned())?;
    let send_id = required_string(data, &["send_id", "sendId"])?.to_owned();
    let channel_id = optional_string(data, &["channel_id", "channelId"])
        .unwrap_or("1")
        .to_owned();
    let native_url = required_string(data, &["native_url", "nativeUrl"])?.to_owned();
    let session_wxid =
        required_string(data, &["session_wxid", "sessionUserName", "sessionWxid"])?.to_owned();
    let in_way = parse_u32_value(data.get("in_way").or_else(|| data.get("inWay")), "in_way")?;
    if in_way > 1 {
        return Err("data.in_way must be 0 (group) or 1 (private)".to_owned());
    }
    Ok((send_id, channel_id, native_url, session_wxid, in_way))
}

fn parse_open_hongbao(
    data: Option<&Value>,
) -> Result<(String, String, String, String, String), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "openHongBao requires a data object".to_owned())?;
    let channel_id = optional_string(data, &["channel_id", "channelId"])
        .unwrap_or("1")
        .to_owned();
    let native_url = required_string(data, &["native_url", "nativeUrl"])?.to_owned();
    let session_wxid =
        required_string(data, &["session_wxid", "sessionUserName", "sessionWxid"])?.to_owned();
    let timing_identifier =
        required_string(data, &["timing_identifier", "timingIdentifier"])?.to_owned();
    let nick_name = optional_string(data, &["nick_name", "nickName"])
        .unwrap_or_default()
        .to_owned();
    Ok((
        channel_id,
        native_url,
        session_wxid,
        timing_identifier,
        nick_name,
    ))
}

fn optional_string<'a>(data: &'a Value, keys: &[&str]) -> Option<&'a str> {
    keys.iter().find_map(|key| {
        data.get(*key)
            .and_then(Value::as_str)
            .filter(|value| !value.trim().is_empty())
    })
}

fn parse_u32_value(value: Option<&Value>, name: &str) -> Result<u32, String> {
    match value {
        Some(Value::Number(number)) => number
            .as_u64()
            .and_then(|number| u32::try_from(number).ok())
            .ok_or_else(|| format!("data.{name} must be a non-negative 32-bit integer")),
        Some(Value::String(text)) => text
            .trim()
            .parse::<u32>()
            .map_err(|_| format!("data.{name} must be a non-negative 32-bit integer")),
        _ => Err(format!("data.{name} must be a non-negative 32-bit integer")),
    }
}

fn parse_confirm_transfer(data: Option<&Value>) -> Result<(String, u32, String), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "confirm_trans requires a data object".to_owned())?;
    let trans_id = required_string(data, &["trans_id", "transfer_id"])?.to_owned();
    let invalid_time = match data.get("invalid_time") {
        Some(Value::Number(value)) => value
            .as_u64()
            .and_then(|value| u32::try_from(value).ok())
            .ok_or_else(|| "data.invalid_time must be a non-negative 32-bit integer".to_owned())?,
        Some(Value::String(value)) => value
            .trim()
            .parse::<u32>()
            .map_err(|_| "data.invalid_time must be a non-negative 32-bit integer".to_owned())?,
        _ => return Err("data.invalid_time must be a non-negative 32-bit integer".to_owned()),
    };
    let transfer_attach = match data.get("transfer_attach") {
        None | Some(Value::Null) => String::new(),
        Some(Value::String(value)) => value.clone(),
        Some(_) => return Err("data.transfer_attach must be a string".to_owned()),
    };
    Ok((trans_id, invalid_time, transfer_attach))
}

fn parse_at_list(data: &Value) -> Result<Option<String>, String> {
    let Some(value) = data.get("at_list").or_else(|| data.get("members")) else {
        return Ok(None);
    };
    let list = if let Some(value) = value.as_str() {
        value
            .split(',')
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(str::to_owned)
            .collect::<Vec<_>>()
    } else if let Some(values) = value.as_array() {
        values
            .iter()
            .map(|value| {
                value
                    .as_str()
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .map(str::to_owned)
                    .ok_or_else(|| "data.members must contain non-empty strings".to_owned())
            })
            .collect::<Result<Vec<_>, _>>()?
    } else {
        return Err("data.at_list / data.members must be a string or string array".to_owned());
    };
    let list = list
        .iter()
        .map(|member| {
            if member == "all" {
                "notify@all"
            } else {
                member.as_str()
            }
        })
        .collect::<Vec<_>>()
        .join(",");
    Ok(if list.is_empty() { None } else { Some(list) })
}

fn parse_inline_at_tags(text: &str) -> (String, Option<String>) {
    if !text.contains("[@,") {
        return (text.to_owned(), None);
    }
    let mut out = String::new();
    let mut ats = Vec::new();
    let mut remaining = text;

    while let Some(start) = remaining.find("[@,") {
        out.push_str(&remaining[..start]);
        let tag_rest = &remaining[start + 3..];
        if let Some(end) = tag_rest.find(']') {
            let tag_content = &tag_rest[..end];
            let mut wxid = "";
            let mut nick = "";
            for part in tag_content.split(',') {
                let kv: Vec<&str> = part.splitn(2, '=').collect();
                if kv.len() == 2 {
                    let k = kv[0].trim();
                    let v = kv[1].trim();
                    if k == "wxid" {
                        wxid = v;
                    } else if k == "nick" {
                        nick = v;
                    }
                }
            }

            if !wxid.is_empty() {
                if wxid == "all" {
                    ats.push("notify@all".to_owned());
                    out.push_str("@所有人\u{2005}");
                } else {
                    ats.push(wxid.to_owned());
                    let display = if !nick.is_empty() { nick } else { wxid };
                    out.push('@');
                    out.push_str(display);
                    out.push('\u{2005}');
                }
            }

            remaining = &tag_rest[end + 1..];
        } else {
            out.push_str(&remaining[start..start + 3]);
            remaining = tag_rest;
        }
    }
    out.push_str(remaining);

    let at_list = if !ats.is_empty() {
        Some(ats.join(","))
    } else {
        None
    };
    (out, at_list)
}

fn parse_send_text(data: Option<&Value>) -> Result<(String, String, Option<String>), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "send_text requires a data object".to_owned())?;
    let target = required_string(data, &["target", "wxid"])?.to_owned();
    let raw_text = required_string(data, &["text", "msg"])?;

    let (cleaned_text, inline_ats) = parse_inline_at_tags(raw_text);
    let explicit_ats = parse_at_list(data)?;

    let at_list = match (inline_ats, explicit_ats) {
        (Some(a), Some(b)) => Some(format!("{a},{b}")),
        (Some(a), None) => Some(a),
        (None, Some(b)) => Some(b),
        (None, None) => None,
    };

    Ok((target, cleaned_text, at_list))
}

fn parse_send_at_text(data: Option<&Value>) -> Result<(String, String, String), String> {
    let (target, text, members) = parse_send_text(data)?;
    let members = members
        .ok_or_else(|| "send_at_text requires at_list, members, or inline @ tags".to_owned())?;
    Ok((target, text, members))
}

fn rich_message_type(action: &str) -> Option<i32> {
    match action {
        "send_image" | "sendImage" => Some(3),
        "send_file" | "sendFile" => Some(6),
        "send_gif" | "sendGif" => Some(47),
        "send_card" | "sendCard" => Some(42),
        "send_share_url" | "sendShareUrl" => Some(5),
        "send_applet" | "sendApplet" => Some(33),
        "send_refer_text" | "sendReferText" => Some(57),
        _ => None,
    }
}

fn required_string<'a>(data: &'a Value, keys: &[&str]) -> Result<&'a str, String> {
    for key in keys {
        if let Some(value) = data.get(key) {
            let value = value
                .as_str()
                .ok_or_else(|| format!("data.{key} must be a string"))?;
            if value.trim().is_empty() {
                return Err(format!("data.{key} must not be empty"));
            }
            return Ok(value);
        }
    }
    Err(format!(
        "data requires a non-empty string field: {}",
        keys.join(" / ")
    ))
}

fn optional_i32(data: &Value, keys: &[&str], default: i32) -> Result<i32, String> {
    for key in keys {
        if let Some(value) = data.get(key) {
            return value
                .as_i64()
                .and_then(|n| i32::try_from(n).ok())
                .ok_or_else(|| format!("data.{key} must be a 32-bit integer"));
        }
    }
    Ok(default)
}

fn optional_base64(data: &Value) -> Result<bool, String> {
    match data.get("is_base64") {
        None => Ok(false),
        Some(value) => value
            .as_bool()
            .ok_or_else(|| "data.is_base64 must be a boolean".to_owned()),
    }
}

fn parse_named_rich_message(
    action: &str,
    data: Option<&Value>,
) -> Result<(String, i32, String, bool, i32), String> {
    let msg_type =
        rich_message_type(action).ok_or_else(|| "unknown rich-message action".to_owned())?;
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| format!("{action} requires a data object"))?;
    let target = required_string(data, &["target", "wxid"])?.to_owned();
    // Structured cards/applets/references require the caller's native XML. Media
    // actions also accept the path/URL/base64 string understood by the native sender.
    let keys: &[&str] = match msg_type {
        3 => &["xml", "content", "image", "img", "url", "path", "msg"],
        6 => &["xml", "content", "file", "file_path", "path", "url", "msg"],
        47 => &["xml", "content", "gif", "image", "url", "path", "msg"],
        _ => &["xml", "content", "msg"],
    };
    let content = required_string(data, keys)?.to_owned();
    let is_base64 = optional_base64(data)?;
    let subtype = optional_i32(data, &["subtype"], 0)?;
    Ok((target, msg_type, content, is_base64, subtype))
}

fn parse_send_xml(data: Option<&Value>) -> Result<(String, String), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "send_xml requires a data object".to_owned())?;
    let target = required_string(data, &["target", "wxid"])?.to_owned();
    let xml = required_string(data, &["xml", "content", "msg"])?.to_owned();
    Ok((target, xml))
}

fn parse_send_message(data: Option<&Value>) -> Result<(String, i32, String, bool, i32), String> {
    let data = data
        .filter(|value| value.is_object())
        .ok_or_else(|| "send_message requires a data object".to_owned())?;
    let target = required_string(data, &["target", "wxid"])?.to_owned();
    let msg_type = optional_i32(data, &["msg_type", "type"], 1)?;
    let content = required_string(data, &["content", "xml", "msg"])?.to_owned();
    let is_base64 = optional_base64(data)?;
    let subtype = optional_i32(data, &["subtype"], 0)?;
    Ok((target, msg_type, content, is_base64, subtype))
}

fn parse_send_raw(data: Option<&Value>) -> Result<(i32, bool, u32, Vec<u8>), String> {
    let data = data.ok_or_else(|| "send_raw requires a data object".to_owned())?;
    let method = data
        .get("method")
        .and_then(Value::as_i64)
        .ok_or_else(|| "data.method must be an integer".to_owned())?;
    let method = i32::try_from(method).map_err(|_| "data.method is out of range".to_owned())?;
    let sync = data
        .get("sync")
        .and_then(Value::as_bool)
        .ok_or_else(|| "data.sync must be a boolean".to_owned())?;
    let request_id = data
        .get("request_id")
        .and_then(Value::as_u64)
        .ok_or_else(|| "data.request_id must be an unsigned integer".to_owned())?;
    let request_id =
        u32::try_from(request_id).map_err(|_| "data.request_id is out of range".to_owned())?;
    let encoded = data
        .get("hex")
        .and_then(Value::as_str)
        .ok_or_else(|| "data.hex must be a hexadecimal string".to_owned())?;
    let payload = decode_hex(encoded)?;
    if payload.len() > MAX_CAPTURE_BYTES {
        return Err(format!("payload exceeds {MAX_CAPTURE_BYTES} byte limit"));
    }
    Ok((method, sync, request_id, payload))
}

fn decode_hex(encoded: &str) -> Result<Vec<u8>, String> {
    let mut digits = encoded
        .bytes()
        .filter(|byte| !byte.is_ascii_whitespace())
        .collect::<Vec<_>>();
    if digits.starts_with(b"0x") || digits.starts_with(b"0X") {
        digits.drain(..2);
    }
    if digits.len() % 2 != 0 {
        return Err("data.hex must contain an even number of hexadecimal digits".to_owned());
    }
    let mut output = Vec::with_capacity(digits.len() / 2);
    for pair in digits.chunks_exact(2) {
        let high =
            hex_digit(pair[0]).ok_or_else(|| "data.hex contains a non-hex digit".to_owned())?;
        let low =
            hex_digit(pair[1]).ok_or_else(|| "data.hex contains a non-hex digit".to_owned())?;
        output.push((high << 4) | low);
    }
    Ok(output)
}

fn hex_digit(byte: u8) -> Option<u8> {
    match byte {
        b'0'..=b'9' => Some(byte - b'0'),
        b'a'..=b'f' => Some(byte - b'a' + 10),
        b'A'..=b'F' => Some(byte - b'A' + 10),
        _ => None,
    }
}

fn write_response(client: &Arc<ConnectedClient>, response: &IpcResponse) -> io::Result<()> {
    let bytes = serde_json::to_vec(response).map_err(io::Error::other)?;
    write_json_line(client, &bytes)
}

fn write_json_line(client: &Arc<ConnectedClient>, bytes: &[u8]) -> io::Result<()> {
    let mut writer = client
        .writer
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner());
    let result = writer
        .write_all(bytes)
        .and_then(|()| writer.write_all(b"\n"))
        .and_then(|()| writer.flush());
    if result.is_err() {
        // A partially written frame must never be reused for another response.
        let _ = writer.shutdown(std::net::Shutdown::Both);
    }
    result
}

fn lock_clients() -> std::sync::MutexGuard<'static, Vec<Arc<ConnectedClient>>> {
    CONNECTED_CLIENTS
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn call(action: &str, data: Option<Value>) -> IpcResponse {
        process_request(IpcRequest {
            id: Some(json!(42)),
            action: Some(action.to_owned()),
            type_name: None,
            data,
        })
    }

    #[test]
    fn delete_friend_parser_accepts_wxid_and_user_id() {
        assert_eq!(
            parse_delete_friend(Some(&json!({"wxid": "wxid_a"}))).unwrap(),
            "wxid_a"
        );
        assert_eq!(
            parse_delete_friend(Some(&json!({"wxid": "", "user_id": "wxid_b"}))).unwrap(),
            "wxid_b"
        );
        assert!(parse_delete_friend(Some(&json!({"wxid": " "}))).is_err());
        assert!(parse_delete_friend(Some(&json!([]))).is_err());
    }

    #[test]
    fn revoke_parser_accepts_native_aliases_and_validates_numeric_fields() {
        assert_eq!(
            parse_revoke_msg(Some(&json!({
                "session": "room@chatroom",
                "msg_id": 314,
                "svr_msg_id": "9223372036854775807",
                "create_time": "1730000000"
            })))
            .unwrap(),
            (
                "room@chatroom".to_owned(),
                "314".to_owned(),
                9_223_372_036_854_775_807,
                1_730_000_000
            )
        );
        assert_eq!(
            parse_revoke_msg(Some(&json!({
                "to_user": "wxid_a",
                "client_msg_id": "client-1",
                "svr_msg_id": 42
            })))
            .unwrap(),
            ("wxid_a".to_owned(), "client-1".to_owned(), 42, 0)
        );
        for invalid in [
            json!({"msg_id": "client-1"}),
            json!({"wxid": "wxid_a", "msg_id": "client-1", "svr_msg_id": -1}),
            json!({"wxid": "wxid_a", "msg_id": "client-1", "create_time": 4294967296u64}),
        ] {
            assert!(parse_revoke_msg(Some(&invalid)).is_err(), "{invalid}");
        }
        for action in [
            "revoke_msg",
            "revoke_my_msg",
            "revokeMyMsg",
            "get_member_list",
            "getMemberList",
            "get_group_list",
            "getGroupList",
            "get_friend_list",
            "getFriendList",
        ] {
            assert_ne!(call(action, None).code, 501, "{action}");
        }
    }

    #[test]
    fn responses_include_all_contract_fields_and_status_mapping() {
        for (status, code, msg) in [
            (0, 200, "操作成功"),
            (3, 401, "Account not ready or not logged in"),
            (4, 404, "Not found"),
            (501, 501, "Not implemented / In progress"),
        ] {
            let response =
                serde_json::to_value(make_response(Some(json!(42)), status, "detail", None))
                    .unwrap();
            assert_eq!(response["code"], code);
            assert_eq!(response["status"], status);
            assert_eq!(response["msg"], msg);
            assert_eq!(response["id"], 42);
            for field in ["code", "msg", "status", "error", "result", "data"] {
                assert!(response.get(field).is_some(), "{field}");
            }
            assert!(response["data"].is_null());
            assert!(response["result"].is_null());
        }
        let response = make_response(None, 0, "", Some(json!({"value": 1})));
        assert_eq!(response.data, response.result);
    }

    #[test]
    fn parses_hongbao_parameters_and_aliases() {
        let (send_id, channel_id, native_url, session, in_way) = parse_receive_hongbao(Some(
            &json!({"sendId":"send-1", "nativeUrl":"wxpay://hongbao", "sessionUserName":"room@chatroom", "inWay":"0"}),
        )).unwrap();
        assert_eq!(
            (
                send_id.as_str(),
                channel_id.as_str(),
                native_url.as_str(),
                session.as_str(),
                in_way
            ),
            ("send-1", "1", "wxpay://hongbao", "room@chatroom", 0)
        );

        let (channel, native_url, session, timing, nickname) = parse_open_hongbao(Some(
            &json!({"channel_id":"8", "native_url":"wxpay://hongbao", "session_wxid":"wxid_1", "timing_identifier":"token", "nick_name":"Bot"}),
        )).unwrap();
        assert_eq!(
            (
                channel.as_str(),
                native_url.as_str(),
                session.as_str(),
                timing.as_str(),
                nickname.as_str()
            ),
            ("8", "wxpay://hongbao", "wxid_1", "token", "Bot")
        );

        for data in [
            json!({"send_id":"send-1", "native_url":"u", "session_wxid":"wxid_1", "in_way":2}),
            json!({"send_id":"send-1", "native_url":"u", "session_wxid":"wxid_1", "in_way":"bad"}),
        ] {
            assert!(parse_receive_hongbao(Some(&data)).is_err(), "{data}");
        }
        assert!(parse_open_hongbao(Some(&json!({"native_url":"u"}))).is_err());
    }

    #[test]
    fn parses_confirm_transfer_ids_and_invalid_times() {
        let (trans_id, invalid_time, transfer_attach) = parse_confirm_transfer(Some(
            &json!({"trans_id": "tr-1", "invalid_time": "1730000000", "transfer_attach": "opaque"}),
        ))
        .unwrap();
        assert_eq!(trans_id, "tr-1");
        assert_eq!(invalid_time, 1_730_000_000);
        assert_eq!(transfer_attach, "opaque");

        let (transfer_id, invalid_time, transfer_attach) =
            parse_confirm_transfer(Some(&json!({"transfer_id": "tr-2", "invalid_time": 42})))
                .unwrap();
        assert_eq!(transfer_id, "tr-2");
        assert_eq!(invalid_time, 42);
        assert!(transfer_attach.is_empty());

        for data in [
            json!({"trans_id": "tr-1"}),
            json!({"trans_id": "", "invalid_time": 1}),
            json!({"trans_id": "tr-1", "invalid_time": -1}),
            json!({"trans_id": "tr-1", "invalid_time": 4294967296u64}),
            json!({"trans_id": "tr-1", "invalid_time": "not-a-time"}),
            json!({"trans_id": "tr-1", "invalid_time": 1, "transfer_attach": 1}),
        ] {
            assert!(parse_confirm_transfer(Some(&data)).is_err(), "{data}");
        }
    }

    #[test]
    fn all_in_progress_actions_and_aliases_return_clean_501() {
        let pairs = [
            ("get_public_list", "getPublicList"),
            ("get_label_list", "getLabelList"),
            ("query_obj", "queryObj"),
            ("query_group", "queryGroup"),
            ("return_trans", "returnTrans"),
            ("set_read_status", "setReadStatus"),
            ("decrypt_image", "decryptImage"),
            ("kill_wechat", "killWeChat"),
            ("edit_version", "editVersion"),
            ("get_login_status", "getLoginStatus"),
            ("get_login_qr_code", "getLoginQrCode"),
            ("set_download_image", "setDownloadImage"),
            ("get_auth_info", "getAuthInfo"),
            ("open_browser", "openBrowser"),
            ("run_cloud_function", "runCloudFunction"),
            ("auth_cami", "authCami"),
        ];
        for (snake, camel) in pairs {
            for action in [snake, camel] {
                let response = call(action, None);
                assert_eq!((response.status, response.code), (501, 501), "{action}");
                assert_eq!(response.msg, "Not implemented / In progress");
                assert_eq!(response.id, Some(json!(42)));
            }
        }
    }

    #[test]
    fn implemented_action_names_dispatch_to_validation() {
        for action in [
            "send_text",
            "sendText",
            "send_at_text",
            "sendText2",
            "send_xml",
            "sendXml",
            "send_image",
            "sendImage",
            "send_file",
            "sendFile",
            "send_gif",
            "sendGif",
            "send_card",
            "sendCard",
            "send_share_url",
            "sendShareUrl",
            "send_applet",
            "sendApplet",
            "send_refer_text",
            "sendReferText",
            "get_member_nick",
            "getMemberNick",
            "get_contact_name",
            "agree_friend_req",
            "agreeFriendReq",
            "confirm_trans",
            "confirmTrans",
            "receive_hong_bao",
            "receiveHongBao",
            "open_hong_bao",
            "openHongBao",
            "create_group",
            "createGroup",
            "add_members",
            "addMembers",
            "invite_members",
            "inviteMembers",
            "del_members",
            "delMembers",
            "quit_group",
            "quitGroup",
            "edit_self_member_nick",
            "editSelfMemberNick",
            "edit_obj_remark",
            "editObjRemark",
            "edit_remark",
        ] {
            assert_eq!(call(action, None).code, 400, "{action}");
        }
        for action in ["get_account", "get_self_info", "getSelfInfo"] {
            assert_eq!(call(action, None).code, 401, "{action}");
        }
        for action in ["ping", "checkWeChat"] {
            assert_eq!(call(action, None).code, 200, "{action}");
        }
        assert_eq!(call("not_an_action", None).code, 404);
        let response = process_request(IpcRequest {
            id: None,
            action: None,
            type_name: Some("checkWeChat".into()),
            data: None,
        });
        assert_eq!(response.code, 200);
    }

    #[test]
    fn parses_group_management_aliases_and_rejects_invalid_member_lists() {
        let (members, topic) = parse_create_group(Some(&json!({
            "users": ["wxid_a", "wxid_b"],
            "name": "team"
        })))
        .unwrap();
        assert_eq!(members, ["wxid_a", "wxid_b"]);
        assert_eq!(topic, "team");

        let (room, members) = parse_chatroom_members(Some(&json!({
            "group_id": "room@chatroom",
            "member_list": "wxid_a, wxid_b"
        })))
        .unwrap();
        assert_eq!(room, "room@chatroom");
        assert_eq!(members, ["wxid_a", "wxid_b"]);
        assert_eq!(
            parse_edit_self_member_nick(Some(&json!({
                "chatroom_name": "room@chatroom",
                "nickname": "new name"
            })))
            .unwrap(),
            ("room@chatroom".to_owned(), "new name".to_owned())
        );
        assert_eq!(
            parse_edit_obj_remark(Some(&json!({ "user_id": "wxid_a", "remark": "friend" })))
                .unwrap(),
            ("wxid_a".to_owned(), "friend".to_owned())
        );

        for invalid in [
            json!({"group_id": "room@chatroom", "member_list": []}),
            json!({"group_id": "room@chatroom", "member_list": ["wxid_a", 7]}),
            json!({"group_id": "room@chatroom", "member_list": [""]}),
            json!({"group_id": "room@chatroom", "member_list": ""}),
        ] {
            assert!(parse_chatroom_members(Some(&invalid)).is_err(), "{invalid}");
        }
    }

    #[test]
    fn rich_actions_have_fixed_native_types_and_preserve_flags() {
        for (snake, camel, expected_type) in [
            ("send_image", "sendImage", 3),
            ("send_file", "sendFile", 6),
            ("send_gif", "sendGif", 47),
            ("send_card", "sendCard", 42),
            ("send_share_url", "sendShareUrl", 5),
            ("send_applet", "sendApplet", 33),
            ("send_refer_text", "sendReferText", 57),
        ] {
            for action in [snake, camel] {
                let data = json!({"wxid": "target", "xml": "<msg/>", "is_base64": true, "subtype": 9, "msg_type": 999});
                let (target, kind, content, base64, subtype) =
                    parse_named_rich_message(action, Some(&data)).unwrap();
                assert_eq!(
                    (target.as_str(), kind, content.as_str(), base64, subtype),
                    ("target", expected_type, "<msg/>", true, 9)
                );
            }
        }
    }

    #[test]
    fn media_aliases_and_strict_validation() {
        for (action, field) in [
            ("sendImage", "image"),
            ("sendFile", "file"),
            ("sendGif", "gif"),
        ] {
            let data = json!({"target": "wxid", field: "/tmp/media"});
            assert!(parse_named_rich_message(action, Some(&data)).is_ok());
        }
        for data in [
            json!({"target": "", "content": "x"}),
            json!({"target": "x", "content": ""}),
            json!({"target": "x", "content": "x", "is_base64": "yes"}),
            json!({"target": "x", "content": "x", "subtype": 4294967296u64}),
            json!([]),
        ] {
            assert!(parse_named_rich_message("sendFile", Some(&data)).is_err());
        }
        let mention = json!({"wxid": "room@chatroom", "msg": "Hi [@,wxid=wxid_a,nick=Alice]", "members": ["all"]});
        let (_, text, members) = parse_send_at_text(Some(&mention)).unwrap();
        assert!(text.contains("@Alice"));
        assert_eq!(members, "wxid_a,notify@all");
        assert!(
            parse_send_at_text(Some(&json!({"target":"x", "text":"hi", "members":[1]}))).is_err()
        );
        assert!(parse_send_message(Some(
            &json!({"target":"x", "content":"hi", "msg_type":4294967296u64})
        ))
        .is_err());
    }

    #[test]
    fn bounds_request_lines() {
        let mut bytes = Vec::new();
        assert_eq!(
            read_bounded_line(&mut &b"{}\nrest"[..], &mut bytes).unwrap(),
            3
        );
        let oversized = vec![b'a'; MAX_REQUEST_LINE + 1];
        assert!(read_bounded_line(&mut oversized.as_slice(), &mut bytes).is_err());
    }

    #[test]
    fn decodes_hex_with_prefix_and_whitespace() {
        assert_eq!(decode_hex("0x01 02\nA0").unwrap(), [1, 2, 0xa0]);
    }

    #[test]
    fn rejects_malformed_hex() {
        assert!(decode_hex("abc").is_err());
        assert!(decode_hex("01xz").is_err());
    }

    #[test]
    fn parses_raw_send_request() {
        let data = json!({
            "method": 7,
            "sync": false,
            "request_id": 42,
            "hex": "010203a0"
        });
        let (method, sync, request_id, payload) = parse_send_raw(Some(&data)).unwrap();
        assert_eq!(method, 7);
        assert!(!sync);
        assert_eq!(request_id, 42);
        assert_eq!(payload, [1, 2, 3, 0xa0]);
    }
}
