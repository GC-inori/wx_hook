use libc::{c_char, c_int, c_void};
use serde::Serialize;
use std::collections::HashMap;
use std::mem;
use std::ptr;
use std::sync::atomic::{AtomicPtr, Ordering};
use std::sync::{Mutex, OnceLock};

pub mod account;
pub mod events;
pub mod ipc;
pub mod logger;
pub mod msg_hook;
pub mod sender;

use logger::{bridge_error, bridge_info};

/// The most recently observed active MMMojo environment.
#[allow(non_upper_case_globals)]
pub static g_environment: AtomicPtr<c_void> = AtomicPtr::new(ptr::null_mut());

const MAX_CAPTURE_BYTES: usize = 1024 * 1024;
const MAX_HEX_BYTES: usize = 64 * 1024;
const MAX_PREVIEW_BYTES: usize = 256;
const MAX_TRACKED_WRITES: usize = 4096;

pub type ReadCallback = unsafe extern "C" fn(u32, *const c_void, *mut c_void);
type SetCallbacksFn = unsafe extern "C" fn(*mut c_void, c_int, *mut c_void);
type ReadInfoGetter = unsafe extern "C" fn(*const c_void, *mut u32) -> *const c_void;
type CreateWriteInfoFn = unsafe extern "C" fn(c_int, bool, u32) -> *mut c_void;
type GetWriteRequestFn = unsafe extern "C" fn(*mut c_void, usize) -> *mut c_void;
type SendWriteInfoFn = unsafe extern "C" fn(*mut c_void, *mut c_void) -> bool;

#[derive(Serialize)]
struct ReadPacketEvent {
    callback_type: c_int,
    request_id: u32,
    request_data_size: u32,
    attach_data_size: u32,
    request_hex: String,
    attach_hex: String,
    request_preview: String,
    attach_preview: String,
    truncated: bool,
}

#[derive(Serialize)]
struct WritePacketEvent {
    method: Option<c_int>,
    sync: Option<bool>,
    request_id: Option<u32>,
    request_data_size: Option<usize>,
    request_hex: String,
    request_preview: String,
    payload_available: bool,
    truncated: bool,
}

static ORIGINAL_READ_PUSH: AtomicPtr<c_void> = AtomicPtr::new(ptr::null_mut());
static ORIGINAL_READ_PULL: AtomicPtr<c_void> = AtomicPtr::new(ptr::null_mut());
static ORIGINAL_USER_DATA: AtomicPtr<c_void> = AtomicPtr::new(ptr::null_mut());

#[used]
#[link_section = ".init_array"]
static INIT_ARRAY: unsafe extern "C" fn() = {
    unsafe extern "C" fn init() {
        let exe = std::fs::read_link("/proc/self/exe").unwrap_or_default();
        let exe_str = exe.to_string_lossy();
        if !exe_str.ends_with("/wechat") && exe_str != "wechat" {
            return;
        }
        bridge_info!(
            "Linux preload bridge mapped into host PID: {}",
            libc::getpid()
        );
        ipc::start_unix_server();
        msg_hook::start_hook();
    }
    init
};

#[derive(Clone, Copy)]
struct WriteMetadata {
    method: c_int,
    sync: bool,
    request_id: u32,
    request_buffer: usize,
    request_size: Option<usize>,
}

static SET_CALLBACKS: OnceLock<Option<SetCallbacksFn>> = OnceLock::new();
static GET_READ_REQUEST: OnceLock<Option<ReadInfoGetter>> = OnceLock::new();
static GET_READ_ATTACH: OnceLock<Option<ReadInfoGetter>> = OnceLock::new();
static CREATE_WRITE_INFO: OnceLock<Option<CreateWriteInfoFn>> = OnceLock::new();
static GET_WRITE_REQUEST: OnceLock<Option<GetWriteRequestFn>> = OnceLock::new();
static SEND_WRITE_INFO: OnceLock<Option<SendWriteInfoFn>> = OnceLock::new();
static WRITE_METADATA: OnceLock<Mutex<HashMap<usize, WriteMetadata>>> = OnceLock::new();

unsafe fn resolve_symbol<T: Copy>(symbol: &'static [u8]) -> Option<T> {
    if mem::size_of::<T>() != mem::size_of::<*mut c_void>() || symbol.last() != Some(&0) {
        return None;
    }
    let address = libc::dlsym(libc::RTLD_NEXT, symbol.as_ptr() as *const c_char);
    if address.is_null() {
        None
    } else {
        // All resolved types in this module are C function pointers, which have
        // the same representation and size as a data pointer on supported Linux.
        Some(mem::transmute_copy::<*mut c_void, T>(&address))
    }
}

fn set_callbacks_fn() -> Option<SetCallbacksFn> {
    *SET_CALLBACKS.get_or_init(|| unsafe { resolve_symbol(b"SetMMMojoEnvironmentCallbacks\0") })
}

fn get_read_request_fn() -> Option<ReadInfoGetter> {
    *GET_READ_REQUEST.get_or_init(|| unsafe { resolve_symbol(b"GetMMMojoReadInfoRequest\0") })
}

fn get_read_attach_fn() -> Option<ReadInfoGetter> {
    *GET_READ_ATTACH.get_or_init(|| unsafe { resolve_symbol(b"GetMMMojoReadInfoAttach\0") })
}

fn create_write_info_fn() -> Option<CreateWriteInfoFn> {
    *CREATE_WRITE_INFO.get_or_init(|| unsafe { resolve_symbol(b"CreateMMMojoWriteInfo\0") })
}

fn get_write_request_fn() -> Option<GetWriteRequestFn> {
    *GET_WRITE_REQUEST.get_or_init(|| unsafe { resolve_symbol(b"GetMMMojoWriteInfoRequest\0") })
}

fn send_write_info_fn() -> Option<SendWriteInfoFn> {
    *SEND_WRITE_INFO.get_or_init(|| unsafe { resolve_symbol(b"SendMMMojoWriteInfo\0") })
}

unsafe fn capture_bytes(pointer: *const c_void, size: u32) -> (String, String, bool) {
    if pointer.is_null() || size == 0 {
        return (String::new(), String::new(), false);
    }

    let available = (size as usize).min(MAX_CAPTURE_BYTES);
    let bytes = std::slice::from_raw_parts(pointer as *const u8, available);
    let hex_length = bytes.len().min(MAX_HEX_BYTES);
    let mut hex = String::with_capacity(hex_length.saturating_mul(2));
    for byte in &bytes[..hex_length] {
        use std::fmt::Write as _;
        let _ = write!(hex, "{byte:02x}");
    }

    let preview_length = bytes.len().min(MAX_PREVIEW_BYTES);
    let preview = String::from_utf8_lossy(&bytes[..preview_length]).into_owned();
    let truncated =
        size as usize > available || bytes.len() > hex_length || bytes.len() > preview_length;
    (hex, preview, truncated)
}

unsafe fn capture_read_field(
    read_info: *const c_void,
    getter: Option<ReadInfoGetter>,
) -> (u32, String, String, bool) {
    let Some(getter) = getter else {
        return (0, String::new(), String::new(), false);
    };
    let mut size = 0_u32;
    let data = getter(read_info, &mut size);
    let (hex, preview, truncated) = capture_bytes(data, size);
    (size, hex, preview, truncated)
}

fn is_debug_packets_enabled() -> bool {
    static ENABLED: OnceLock<bool> = OnceLock::new();
    *ENABLED.get_or_init(|| std::env::var_os("WX_BRIDGE_CAPTURE_PACKETS").is_some())
}

unsafe fn emit_read_event(callback_type: c_int, request_id: u32, read_info: *const c_void) {
    if !is_debug_packets_enabled() {
        return;
    }
    let (request_data_size, request_hex, request_preview, request_truncated) =
        capture_read_field(read_info, get_read_request_fn());
    let (attach_data_size, attach_hex, attach_preview, attach_truncated) =
        capture_read_field(read_info, get_read_attach_fn());

    let event = ReadPacketEvent {
        callback_type,
        request_id,
        request_data_size,
        attach_data_size,
        request_hex,
        attach_hex,
        request_preview,
        attach_preview,
        truncated: request_truncated || attach_truncated,
    };
    bridge_info!(
        "Captured inbound read packet: callback_type={}, req_id={}, req_size={}, attach_size={}",
        callback_type,
        request_id,
        request_data_size,
        attach_data_size
    );
    ipc::broadcast_event("mmmojo_read", &event);
}

unsafe extern "C" fn read_push_trampoline(
    request_id: u32,
    read_info: *const c_void,
    user_data: *mut c_void,
) {
    let _ = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        emit_read_event(1, request_id, read_info);
    }));
    let orig = ORIGINAL_READ_PUSH.load(Ordering::Acquire);
    if !orig.is_null() {
        let func: ReadCallback = mem::transmute(orig);
        func(request_id, read_info, user_data);
    }
}

unsafe extern "C" fn read_pull_trampoline(
    request_id: u32,
    read_info: *const c_void,
    user_data: *mut c_void,
) {
    let _ = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        emit_read_event(2, request_id, read_info);
    }));
    let orig = ORIGINAL_READ_PULL.load(Ordering::Acquire);
    if !orig.is_null() {
        let func: ReadCallback = mem::transmute(orig);
        func(request_id, read_info, user_data);
    }
}

/// Intercepts callback registration while preserving the host's actual callback pointers.
/// Each callback_type takes exactly one argument from the variadic list.
#[no_mangle]
#[allow(non_snake_case)]
pub unsafe extern "C" fn SetMMMojoEnvironmentCallbacks(
    environment: *mut c_void,
    callback_type: c_int,
    arg: *mut c_void,
) {
    g_environment.store(environment, Ordering::Release);
    bridge_info!(
        "SetMMMojoEnvironmentCallbacks called: env={:p}, type={}, arg={:p}",
        environment,
        callback_type,
        arg
    );
    ipc::start_unix_server();

    let Some(real_set_callbacks) = set_callbacks_fn() else {
        bridge_error!("Failed to resolve real SetMMMojoEnvironmentCallbacks");
        return;
    };

    let mut forwarded_arg = arg;
    match callback_type {
        0 => {
            ORIGINAL_USER_DATA.store(arg, Ordering::Release);
        }
        1 => {
            ORIGINAL_READ_PUSH.store(arg, Ordering::Release);
            forwarded_arg = read_push_trampoline as *mut c_void;
        }
        2 => {
            ORIGINAL_READ_PULL.store(arg, Ordering::Release);
            forwarded_arg = read_pull_trampoline as *mut c_void;
        }
        _ => {}
    }

    real_set_callbacks(environment, callback_type, forwarded_arg);
}

/// Passthrough proxy for libmmmojo read request data.
#[no_mangle]
#[allow(non_snake_case)]
pub unsafe extern "C" fn GetMMMojoReadInfoRequest(
    read_info: *const c_void,
    request_data_size: *mut u32,
) -> *const c_void {
    match get_read_request_fn() {
        Some(real) => real(read_info, request_data_size),
        None => ptr::null(),
    }
}

/// Passthrough proxy for libmmmojo read attachment data.
#[no_mangle]
#[allow(non_snake_case)]
pub unsafe extern "C" fn GetMMMojoReadInfoAttach(
    read_info: *const c_void,
    attach_data_size: *mut u32,
) -> *const c_void {
    match get_read_attach_fn() {
        Some(real) => real(read_info, attach_data_size),
        None => ptr::null(),
    }
}

/// Passthrough proxy for creating a libmmmojo write request.
#[no_mangle]
#[allow(non_snake_case)]
pub unsafe extern "C" fn CreateMMMojoWriteInfo(
    method: c_int,
    sync: bool,
    request_id: u32,
) -> *mut c_void {
    let Some(real) = create_write_info_fn() else {
        return ptr::null_mut();
    };
    let write_info = real(method, sync, request_id);
    if !write_info.is_null() {
        let metadata = WriteMetadata {
            method,
            sync,
            request_id,
            request_buffer: 0,
            request_size: None,
        };
        let mut writes = write_metadata()
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if writes.len() >= MAX_TRACKED_WRITES {
            if let Some(oldest_key) = writes.keys().next().copied() {
                writes.remove(&oldest_key);
            }
        }
        writes.insert(write_info as usize, metadata);
    }
    write_info
}

/// Passthrough proxy for accessing a libmmmojo write request buffer.
#[no_mangle]
#[allow(non_snake_case)]
pub unsafe extern "C" fn GetMMMojoWriteInfoRequest(
    write_info: *mut c_void,
    size: usize,
) -> *mut c_void {
    let Some(real) = get_write_request_fn() else {
        return ptr::null_mut();
    };
    let request = real(write_info, size);
    if !write_info.is_null() {
        if let Some(metadata) = write_metadata()
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .get_mut(&(write_info as usize))
        {
            metadata.request_buffer = request as usize;
            metadata.request_size = Some(size);
        }
    }
    request
}

/// Records outgoing write data and then forwards the original send call.
#[no_mangle]
#[allow(non_snake_case)]
pub unsafe extern "C" fn SendMMMojoWriteInfo(
    environment: *mut c_void,
    write_info: *mut c_void,
) -> bool {
    let metadata = if write_info.is_null() {
        None
    } else {
        write_metadata()
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .remove(&(write_info as usize))
    };
    let _ = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        emit_write_event(metadata);
    }));
    bridge_info!(
        "SendMMMojoWriteInfo: env={:p}, write_info={:p}",
        environment,
        write_info
    );
    match send_write_info_fn() {
        Some(real) => real(environment, write_info),
        None => false,
    }
}

/// Sends a binary request through the currently active environment.
pub(crate) unsafe fn send_raw(
    method: c_int,
    sync: bool,
    request_id: u32,
    payload: &[u8],
) -> Result<bool, String> {
    let environment = g_environment.load(Ordering::Acquire);
    if environment.is_null() {
        return Err("MMMojo environment is not active".to_owned());
    }
    if payload.len() > MAX_CAPTURE_BYTES {
        return Err(format!("request exceeds {MAX_CAPTURE_BYTES} byte limit"));
    }

    if create_write_info_fn().is_none() {
        return Err("CreateMMMojoWriteInfo is unavailable".to_owned());
    }
    if get_write_request_fn().is_none() {
        return Err("GetMMMojoWriteInfoRequest is unavailable".to_owned());
    }
    if send_write_info_fn().is_none() {
        return Err("SendMMMojoWriteInfo is unavailable".to_owned());
    }

    let write_info = CreateMMMojoWriteInfo(method, sync, request_id);
    if write_info.is_null() {
        return Err("CreateMMMojoWriteInfo returned null".to_owned());
    }
    let request_buffer = GetMMMojoWriteInfoRequest(write_info, payload.len());
    if request_buffer.is_null() && !payload.is_empty() {
        return Err("GetMMMojoWriteInfoRequest returned null".to_owned());
    }
    if !payload.is_empty() {
        ptr::copy_nonoverlapping(payload.as_ptr(), request_buffer as *mut u8, payload.len());
    }

    Ok(SendMMMojoWriteInfo(environment, write_info))
}

fn write_metadata() -> &'static Mutex<HashMap<usize, WriteMetadata>> {
    WRITE_METADATA.get_or_init(|| Mutex::new(HashMap::new()))
}

fn emit_write_event(metadata: Option<WriteMetadata>) {
    if !is_debug_packets_enabled() {
        return;
    }
    let (
        method,
        sync,
        request_id,
        request_data_size,
        request_hex,
        request_preview,
        payload_available,
        truncated,
    ) = match metadata {
        Some(metadata) => match metadata.request_size {
            Some(size) if size == 0 || metadata.request_buffer != 0 => {
                let buffer = metadata.request_buffer as *const c_void;
                let capture_size = size.min(u32::MAX as usize) as u32;
                let (hex, preview, capture_truncated) =
                    unsafe { capture_bytes(buffer, capture_size) };
                (
                    Some(metadata.method),
                    Some(metadata.sync),
                    Some(metadata.request_id),
                    Some(size),
                    hex,
                    preview,
                    true,
                    capture_truncated || size > u32::MAX as usize,
                )
            }
            _ => (
                Some(metadata.method),
                Some(metadata.sync),
                Some(metadata.request_id),
                metadata.request_size,
                String::new(),
                String::new(),
                false,
                false,
            ),
        },
        None => (
            None,
            None,
            None,
            None,
            String::new(),
            String::new(),
            false,
            false,
        ),
    };

    let event = WritePacketEvent {
        method,
        sync,
        request_id,
        request_data_size,
        request_hex,
        request_preview,
        payload_available,
        truncated,
    };
    ipc::broadcast_event("mmmojo_write", &event);
}

#[no_mangle]
#[allow(non_snake_case)]
pub extern "C" fn WxStartBridge() -> i32 {
    ipc::start_unix_server();
    0
}
