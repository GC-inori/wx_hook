use super::MAX_CAPTURE_BYTES;
use super::logger::{bridge_error, bridge_info, bridge_warn};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::io::{self, BufRead, BufReader, Write};
use std::os::unix::fs::{FileTypeExt, PermissionsExt};
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::{self, Receiver, Sender};
use std::sync::{Arc, Mutex, OnceLock};
use std::thread;
use std::time::Duration;

const DEFAULT_SOCKET_PATH: &str = "/tmp/wx_bridge.sock";
const MAX_REQUEST_LINE: usize = 3 * 1024 * 1024;
const WRITE_TIMEOUT: Duration = Duration::from_millis(100);

#[derive(Deserialize)]
struct IpcRequest {
    id: Option<Value>,
    action: String,
    data: Option<Value>,
}

#[derive(Serialize)]
struct IpcResponse {
    id: Option<Value>,
    status: i32,
    error: String,
    data: Option<Value>,
}

struct ConnectedClient {
    writer: Mutex<UnixStream>,
}

static CONNECTED_CLIENTS: Mutex<Vec<Arc<ConnectedClient>>> = Mutex::new(Vec::new());
static EVENT_SENDER: OnceLock<Sender<String>> = OnceLock::new();
static SERVER_RUNNING: AtomicBool = AtomicBool::new(false);

/// Queues one newline-delimited JSON event for all connected socket clients.
pub fn broadcast_event<T: Serialize>(event: &str, data: &T) {
    let Ok(line) = serde_json::to_string(&json!({ "event": event, "data": data })) else {
        return;
    };
    if let Some(sender) = EVENT_SENDER.get() {
        let _ = sender.send(line);
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

    let _ = EVENT_SENDER.get_or_init(|| {
        let (sender, receiver) = mpsc::channel();
        let _ = thread::Builder::new()
            .name("wx-bridge-events".to_owned())
            .spawn(move || dispatch_events(receiver));
        sender
    });

    let listener_started = thread::Builder::new()
        .name("wx-bridge-socket".to_owned())
        .spawn(run_listener)
        .is_ok();
    if !listener_started {
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
                let _ = thread::Builder::new()
                    .name("wx-bridge-client".to_owned())
                    .spawn(move || handle_client(stream, client));
            }
            Err(error) => {
                bridge_error!("socket accept failed: {error}");
                thread::sleep(Duration::from_millis(100));
            }
        }
    }

    bridge_warn!("Socket listener exiting, removing socket: {}", path.display());
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
                let response = IpcResponse {
                    id: None,
                    status: -1,
                    error: format!("invalid or oversized request: {error}"),
                    data: None,
                };
                let _ = write_response(&client, &response);
                break;
            }
        }

        let request: IpcRequest = match serde_json::from_slice(&line) {
            Ok(request) => request,
            Err(error) => {
                let response = IpcResponse {
                    id: None,
                    status: -1,
                    error: format!("invalid JSON: {error}"),
                    data: None,
                };
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
    match request.action.as_str() {
        "ping" => IpcResponse {
            id,
            status: 0,
            error: String::new(),
            data: Some(json!({ "socket": "wx_bridge" })),
        },
        "send_raw" => match parse_send_raw(request.data.as_ref()) {
            Ok((method, sync, request_id, payload)) => {
                let result = unsafe { super::send_raw(method, sync, request_id, &payload) };
                match result {
                    Ok(sent) => IpcResponse {
                        id,
                        status: if sent { 0 } else { 2 },
                        error: if sent {
                            String::new()
                        } else {
                            "SendMMMojoWriteInfo returned false".to_owned()
                        },
                        data: Some(json!({ "sent": sent, "bytes": payload.len() })),
                    },
                    Err(error) => IpcResponse {
                        id,
                        status: 2,
                        error,
                        data: None,
                    },
                }
            }
            Err(error) => IpcResponse {
                id,
                status: 1,
                error,
                data: None,
            },
        },
        other => IpcResponse {
            id,
            status: 404,
            error: format!("unknown action: {other}"),
            data: None,
        },
    }
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
    writer.write_all(bytes)?;
    writer.write_all(b"\n")?;
    writer.flush()
}

fn lock_clients() -> std::sync::MutexGuard<'static, Vec<Arc<ConnectedClient>>> {
    CONNECTED_CLIENTS
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner())
}

#[cfg(test)]
mod tests {
    use super::*;

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
