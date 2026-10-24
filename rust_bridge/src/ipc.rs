use std::ffi::c_void;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

type HANDLE = *mut c_void;
type BOOL = i32;

const INVALID_HANDLE_VALUE: HANDLE = -1isize as *mut c_void;
const PIPE_ACCESS_DUPLEX: u32 = 0x00000003;
const PIPE_TYPE_BYTE: u32 = 0x00000000;
const PIPE_READMODE_BYTE: u32 = 0x00000000;
const PIPE_WAIT: u32 = 0x00000000;
const PIPE_UNLIMITED_INSTANCES: u32 = 255;

extern "system" {
    fn CreateNamedPipeW(
        lpName: *const u16,
        dwOpenMode: u32,
        dwPipeMode: u32,
        nMaxInstances: u32,
        nOutBufferSize: u32,
        nInBufferSize: u32,
        nDefaultTimeOut: u32,
        lpSecurityAttributes: *mut c_void,
    ) -> HANDLE;
    fn ConnectNamedPipe(hNamedPipe: HANDLE, lpOverlapped: *mut c_void) -> BOOL;
    fn DisconnectNamedPipe(hNamedPipe: HANDLE) -> BOOL;
    fn ReadFile(
        hFile: HANDLE,
        lpBuffer: *mut c_void,
        nNumberOfBytesToRead: u32,
        lpNumberOfBytesRead: *mut u32,
        lpOverlapped: *mut c_void,
    ) -> BOOL;
    fn WriteFile(
        hFile: HANDLE,
        lpBuffer: *const c_void,
        nNumberOfBytesToWrite: u32,
        lpNumberOfBytesWritten: *mut u32,
        lpOverlapped: *mut c_void,
    ) -> BOOL;
    fn CloseHandle(hObject: HANDLE) -> BOOL;
    fn GetLastError() -> u32;
}

use crate::account;
use crate::hooks;

#[derive(Serialize, Deserialize, Debug)]
pub struct IpcRequest {
    pub id: Option<u64>,
    pub action: String,
    pub data: Option<Value>,
}

#[derive(Serialize, Debug)]
pub struct IpcResponse {
    pub id: Option<u64>,
    pub status: i32,
    pub error: String,
    pub data: Option<Value>,
}

struct ClientHandle {
    handle: HANDLE,
}

unsafe impl Send for ClientHandle {}

static CONNECTED_CLIENTS: Mutex<Vec<Arc<Mutex<ClientHandle>>>> = Mutex::new(Vec::new());
static RUNNING: AtomicBool = AtomicBool::new(false);

pub fn broadcast_event<T: Serialize>(event: &str, data: &T) {
    let payload = json!({
        "event": event,
        "data": data,
    });
    let json_bytes = match serde_json::to_vec(&payload) {
        Ok(b) => b,
        Err(_) => return,
    };

    let len_prefix = (json_bytes.len() as u32).to_le_bytes();

    let mut packet = Vec::with_capacity(4 + json_bytes.len());
    packet.extend_from_slice(&len_prefix);
    packet.extend_from_slice(&json_bytes);

    let clients = {
        let guard = CONNECTED_CLIENTS.lock().unwrap();
        guard.clone()
    };

    for client in clients {
        let guard = client.lock().unwrap();
        let mut written: u32 = 0;
        unsafe {
            WriteFile(
                guard.handle,
                packet.as_ptr() as *const c_void,
                packet.len() as u32,
                &mut written,
                std::ptr::null_mut(),
            );
        }
    }
}

fn handle_client(client_arc: Arc<Mutex<ClientHandle>>) {
    let handle = {
        let guard = client_arc.lock().unwrap();
        guard.handle
    };

    loop {
        // Read 4-byte length prefix
        let mut len_buf = [0u8; 4];
        let mut bytes_read: u32 = 0;
        let ok = unsafe {
            ReadFile(
                handle,
                len_buf.as_mut_ptr() as *mut c_void,
                4,
                &mut bytes_read,
                std::ptr::null_mut(),
            )
        };
        if ok == 0 || bytes_read < 4 {
            break; // Client disconnected
        }

        let msg_len = u32::from_le_bytes(len_buf) as usize;
        if msg_len == 0 || msg_len > 10 * 1024 * 1024 {
            break; // Invalid frame
        }

        let mut body = vec![0u8; msg_len];
        let mut total_read = 0;
        let mut read_failed = false;

        while total_read < msg_len {
            let mut chunk: u32 = 0;
            let ok = unsafe {
                ReadFile(
                    handle,
                    body.as_mut_ptr().add(total_read) as *mut c_void,
                    (msg_len - total_read) as u32,
                    &mut chunk,
                    std::ptr::null_mut(),
                )
            };
            if ok == 0 || chunk == 0 {
                read_failed = true;
                break;
            }
            total_read += chunk as usize;
        }

        if read_failed {
            break;
        }

        // Process request
        let req: IpcRequest = match serde_json::from_slice(&body) {
            Ok(r) => r,
            Err(e) => {
                let err_resp = IpcResponse {
                    id: None,
                    status: -1,
                    error: format!("Invalid JSON: {}", e),
                    data: None,
                };
                send_response(handle, &err_resp);
                continue;
            }
        };

        let resp = process_request(&req);
        send_response(handle, &resp);
    }

    // Remove client
    let mut clients = CONNECTED_CLIENTS.lock().unwrap();
    clients.retain(|c| !Arc::ptr_eq(c, &client_arc));
    unsafe {
        DisconnectNamedPipe(handle);
        CloseHandle(handle);
    }
}

fn process_request(req: &IpcRequest) -> IpcResponse {
    let core_base = hooks::CORE_BASE.load(Ordering::Relaxed);

    match req.action.as_str() {
        "send_text" => {
            let target = req.data.as_ref()
                .and_then(|d| d.get("target"))
                .and_then(|t| t.as_str())
                .unwrap_or_default();
            let text = req.data.as_ref()
                .and_then(|d| d.get("text"))
                .and_then(|t| t.as_str())
                .unwrap_or_default();

            if target.is_empty() || text.is_empty() {
                return IpcResponse {
                    id: req.id,
                    status: 1,
                    error: "target or text is empty".to_string(),
                    data: None,
                };
            }

            match hooks::send_text(target, text, None) {
                Ok(_) => IpcResponse {
                    id: req.id,
                    status: 0,
                    error: String::new(),
                    data: None,
                },
                Err(e) => IpcResponse {
                    id: req.id,
                    status: 2,
                    error: e,
                    data: None,
                },
            }
        }
        "send_at_text" => {
            let target = req.data.as_ref()
                .and_then(|d| d.get("target"))
                .and_then(|t| t.as_str())
                .unwrap_or_default();
            let text = req.data.as_ref()
                .and_then(|d| d.get("text"))
                .and_then(|t| t.as_str())
                .unwrap_or_default();
            let members = req.data.as_ref()
                .and_then(|d| d.get("members"))
                .and_then(|t| t.as_str())
                .unwrap_or_default();

            if target.is_empty() || text.is_empty() || members.is_empty() {
                return IpcResponse {
                    id: req.id,
                    status: 1,
                    error: "target, text or members is empty".to_string(),
                    data: None,
                };
            }

            match hooks::send_text(target, text, Some(members.to_string())) {
                Ok(_) => IpcResponse {
                    id: req.id,
                    status: 0,
                    error: String::new(),
                    data: None,
                },
                Err(e) => IpcResponse {
                    id: req.id,
                    status: 2,
                    error: e,
                    data: None,
                },
            }
        }
        "get_account" => {
            match account::get_current_account(core_base) {
                Some(acc) => IpcResponse {
                    id: req.id,
                    status: 0,
                    error: String::new(),
                    data: Some(json!({ "account": acc })),
                },
                None => IpcResponse {
                    id: req.id,
                    status: 3,
                    error: "Account not ready or not logged in".to_string(),
                    data: None,
                },
            }
        }
        "get_contact_name" => {
            let wxid = req.data.as_ref()
                .and_then(|d| d.get("wxid"))
                .and_then(|t| t.as_str())
                .unwrap_or_default();

            if wxid.is_empty() {
                return IpcResponse {
                    id: req.id,
                    status: 1,
                    error: "wxid is empty".to_string(),
                    data: None,
                };
            }

            match account::get_contact_name(core_base, wxid) {
                Some(name) => IpcResponse {
                    id: req.id,
                    status: 0,
                    error: String::new(),
                    data: Some(json!({ "name": name })),
                },
                None => IpcResponse {
                    id: req.id,
                    status: 4,
                    error: "Contact not found in cache".to_string(),
                    data: None,
                },
            }
        }
        other => IpcResponse {
            id: req.id,
            status: 404,
            error: format!("Unknown action: {}", other),
            data: None,
        },
    }
}

fn send_response(handle: HANDLE, resp: &IpcResponse) {
    if let Ok(bytes) = serde_json::to_vec(resp) {
        let len_prefix = (bytes.len() as u32).to_le_bytes();
        let mut packet = Vec::with_capacity(4 + bytes.len());
        packet.extend_from_slice(&len_prefix);
        packet.extend_from_slice(&bytes);

        let mut written: u32 = 0;
        unsafe {
            WriteFile(
                handle,
                packet.as_ptr() as *const c_void,
                packet.len() as u32,
                &mut written,
                std::ptr::null_mut(),
            );
        }
    }
}

pub fn start_pipe_server() {
    if RUNNING.swap(true, Ordering::SeqCst) {
        return;
    }

    thread::spawn(|| {
        let pid = std::process::id();
        let pipe_name = format!("\\\\.\\pipe\\wxbot_ipc_{}\0", pid);
        let pipe_name_w: Vec<u16> = pipe_name.encode_utf16().collect();

        while RUNNING.load(Ordering::SeqCst) {
            let handle = unsafe {
                CreateNamedPipeW(
                    pipe_name_w.as_ptr(),
                    PIPE_ACCESS_DUPLEX,
                    PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT,
                    PIPE_UNLIMITED_INSTANCES,
                    65536,
                    65536,
                    0,
                    std::ptr::null_mut(),
                )
            };

            if handle == INVALID_HANDLE_VALUE {
                thread::sleep(std::time::Duration::from_millis(500));
                continue;
            }

            let connected = unsafe { ConnectNamedPipe(handle, std::ptr::null_mut()) };
            let err = unsafe { GetLastError() };
            // 0 means success or ERROR_PIPE_CONNECTED (535)
            if connected != 0 || err == 535 {
                let client_arc = Arc::new(Mutex::new(ClientHandle { handle }));
                {
                    let mut guard = CONNECTED_CLIENTS.lock().unwrap();
                    guard.push(client_arc.clone());
                }
                let worker_arc = client_arc.clone();
                thread::spawn(move || {
                    handle_client(worker_arc);
                });
            } else {
                unsafe {
                    CloseHandle(handle);
                }
            }
        }
    });
}
