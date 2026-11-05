use libc::c_void;
use std::collections::HashSet;
use std::mem;
use std::ptr;
use std::sync::atomic::{AtomicI32, AtomicPtr, Ordering};
use std::sync::{Arc, Condvar, Mutex, OnceLock};
use std::time::Duration;

use serde_json::{json, Value};

use super::logger::{bridge_error, bridge_info};
use super::msg_hook::{is_readable_ptr, patch_memory, WECHAT_BASE};

pub const RVA_SEND_TEXT_MESSAGE: usize = 0x6f93c80;
pub const RVA_CHAT_ID_COPY_CALL: usize = 0x6f93dc8;
pub const RVA_STD_STRING_ASSIGN: usize = 0x450fdb0;

pub const SEND_PARAMS_MAGIC: u32 = 0x41544D53; // "ATMS"

// ==================== C++ std::string (LLVM libc++) ABI ====================

#[repr(C)]
#[derive(Clone, Copy)]
pub struct StdString {
    pub words: [usize; 3], // 24 bytes
}

impl StdString {
    pub fn new(s: &str) -> Self {
        let bytes = s.as_bytes();
        let len = bytes.len();
        if len <= 22 {
            let mut words = [0usize; 3];
            let raw = words.as_mut_ptr() as *mut u8;
            unsafe {
                *raw = (len as u8) << 1;
                ptr::copy_nonoverlapping(bytes.as_ptr(), raw.add(1), len);
                *raw.add(1 + len) = 0;
            }
            StdString { words }
        } else {
            let alloc_size = len + 1;
            let p = unsafe { libc::malloc(alloc_size) as *mut u8 };
            if p.is_null() {
                return Self::new("");
            }
            unsafe {
                ptr::copy_nonoverlapping(bytes.as_ptr(), p, len);
                *p.add(len) = 0;
            }
            StdString {
                words: [alloc_size | 1, len, p as usize],
            }
        }
    }

    pub fn as_str(&self) -> Option<&str> {
        unsafe {
            if (self.words[0] & 1) == 0 {
                let len = ((self.words[0] & 0xff) >> 1) as usize;
                let bytes = (self.words.as_ptr() as *const u8).add(1);
                std::str::from_utf8(std::slice::from_raw_parts(bytes, len)).ok()
            } else {
                let len = self.words[1];
                let bytes = self.words[2] as *const u8;
                if len == 0 {
                    return Some("");
                }
                if bytes.is_null() {
                    return None;
                }
                std::str::from_utf8(std::slice::from_raw_parts(bytes, len)).ok()
            }
        }
    }

    pub fn dispose(&mut self) {
        if (self.words[0] & 1) != 0 {
            let p = self.words[2] as *mut libc::c_void;
            if !p.is_null() {
                unsafe { libc::free(p) };
            }
            self.words[0] = 0;
            self.words[1] = 0;
            self.words[2] = 0;
        }
    }
}

#[repr(C)]
struct NativeStdVector {
    start: *mut StdString,
    end: *mut StdString,
    cap: *mut StdString,
    storage: Vec<StdString>,
}

impl NativeStdVector {
    fn from_strings(strings: &[&str]) -> Self {
        let mut storage: Vec<StdString> =
            strings.iter().map(|value| StdString::new(value)).collect();
        let start = storage.as_mut_ptr();
        let len = storage.len();
        let cap = storage.capacity();
        Self {
            start,
            end: unsafe { start.add(len) },
            cap: unsafe { start.add(cap) },
            storage,
        }
    }

    fn as_ptr(&self) -> *const usize {
        let _end = self.end;
        let _cap = self.cap;
        &self.start as *const *mut StdString as *const usize
    }
}

impl Drop for NativeStdVector {
    fn drop(&mut self) {
        for value in &mut self.storage {
            value.dispose();
        }
    }
}

// Extended parameter block passing at_user_list with safety magic
#[repr(C)]
pub struct SendTextParams {
    pub chat_id: StdString,
    pub content: StdString,
    pub at_user_list: StdString,
    pub magic: u32,
    pub _pad: [u8; 4],
}

#[repr(C)]
struct CallbackWrapper {
    refcount: std::sync::atomic::AtomicI32,
    _pad: u32,
    invoke_fn: unsafe extern "C" fn(*mut CallbackWrapper, *mut c_void),
    destroy_fn: unsafe extern "C" fn(*mut CallbackWrapper),
}

unsafe extern "C" fn cb_invoke(_this: *mut CallbackWrapper, _res: *mut c_void) {
    bridge_info!("[SEND-TEXT] Callback invoke completed");
}

unsafe extern "C" fn cb_destroy(_this: *mut CallbackWrapper) {
    bridge_info!("[SEND-TEXT] Callback destroy completed");
}

type SendTextFn = unsafe extern "C" fn(
    a1: usize,
    params: *const SendTextParams,
    method: *const StdString,
    callback: *mut *mut CallbackWrapper,
);

type CopyStdStringFn = unsafe extern "C" fn(*mut c_void, *const c_void) -> *mut c_void;
static ORIGINAL_COPY_FN: AtomicPtr<c_void> = AtomicPtr::new(ptr::null_mut());

type GSourceFunc = unsafe extern "C" fn(*mut c_void) -> libc::c_int;
type GIdleAddFn = unsafe extern "C" fn(GSourceFunc, *mut c_void) -> libc::c_uint;

static G_IDLE_ADD: OnceLock<Option<GIdleAddFn>> = OnceLock::new();

fn get_g_idle_add() -> Option<GIdleAddFn> {
    *G_IDLE_ADD.get_or_init(|| unsafe {
        let sym = libc::dlsym(
            libc::RTLD_DEFAULT,
            b"g_idle_add\0".as_ptr() as *const libc::c_char,
        );
        if sym.is_null() {
            None
        } else {
            Some(mem::transmute(sym))
        }
    })
}

fn is_main_thread() -> bool {
    unsafe { libc::syscall(libc::SYS_gettid) == libc::getpid() as libc::c_long }
}

enum SendState<T = ()> {
    Pending,
    Running,
    Complete(Result<T, String>),
    Cancelled,
}

struct SendCompletion {
    state: Mutex<SendState>,
    condvar: Condvar,
}

impl SendCompletion {
    fn new() -> Self {
        Self {
            state: Mutex::new(SendState::Pending),
            condvar: Condvar::new(),
        }
    }

    fn begin(&self) -> bool {
        let mut state = self
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if !matches!(*state, SendState::Pending) {
            return false;
        }
        *state = SendState::Running;
        true
    }

    fn finish(&self, result: Result<(), String>) {
        *self
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner()) = SendState::Complete(result);
        self.condvar.notify_all();
    }

    fn wait(&self, timeout: Duration) -> Result<(), String> {
        let start = std::time::Instant::now();
        let mut state = self
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        loop {
            if let SendState::Complete(result) = &*state {
                return result.clone();
            }
            let remaining = timeout.saturating_sub(start.elapsed());
            if remaining.is_zero() {
                if matches!(*state, SendState::Pending) {
                    *state = SendState::Cancelled;
                    return Err(
                        "send timed out before main-thread execution; request cancelled".to_owned(),
                    );
                }
                return Err("send timed out during native execution; outcome unknown, do not retry automatically".to_owned());
            }
            let (next, _) = self
                .condvar
                .wait_timeout(state, remaining)
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            state = next;
        }
    }
}

struct NativeTaskState<T> {
    state: Mutex<NativeTaskStatus<T>>,
    condvar: Condvar,
}

enum NativeTaskStatus<T> {
    Pending,
    Running,
    Complete(Option<Result<T, String>>),
    Cancelled,
}

struct NativeThreadTask<T> {
    completion: NativeTaskState<T>,
    operation: Mutex<Option<Box<dyn FnOnce() -> Result<T, String> + Send>>>,
}

impl<T> NativeThreadTask<T> {
    fn new(operation: impl FnOnce() -> Result<T, String> + Send + 'static) -> Self {
        Self {
            completion: NativeTaskState {
                state: Mutex::new(NativeTaskStatus::Pending),
                condvar: Condvar::new(),
            },
            operation: Mutex::new(Some(Box::new(operation))),
        }
    }

    fn begin(&self) -> bool {
        let mut state = self
            .completion
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if !matches!(*state, NativeTaskStatus::Pending) {
            return false;
        }
        *state = NativeTaskStatus::Running;
        true
    }

    fn finish(&self, result: Result<T, String>) {
        *self
            .completion
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner()) =
            NativeTaskStatus::Complete(Some(result));
        self.completion.condvar.notify_all();
    }

    fn wait(&self, timeout: Duration) -> Result<T, String> {
        let started = std::time::Instant::now();
        let mut state = self
            .completion
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        loop {
            if let NativeTaskStatus::Complete(result) = &mut *state {
                return result.take().unwrap_or_else(|| {
                    Err("native operation result was already consumed".to_owned())
                });
            }
            let remaining = timeout.saturating_sub(started.elapsed());
            if remaining.is_zero() {
                if matches!(*state, NativeTaskStatus::Pending) {
                    *state = NativeTaskStatus::Cancelled;
                    return Err("native operation timed out before main-thread execution; request cancelled".to_owned());
                }
                return Err("native operation timed out during execution; outcome unknown, do not retry automatically".to_owned());
            }
            let (next, _) = self
                .completion
                .condvar
                .wait_timeout(state, remaining)
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            state = next;
        }
    }
}

unsafe extern "C" fn main_thread_native_dispatcher<T: Send + 'static>(
    data: *mut c_void,
) -> libc::c_int {
    let task = Arc::from_raw(data as *const NativeThreadTask<T>);
    if task.begin() {
        let operation = task
            .operation
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .take();
        let result = match operation {
            Some(operation) => std::panic::catch_unwind(std::panic::AssertUnwindSafe(operation))
                .unwrap_or_else(|_| Err("native operation panicked on the main thread".to_owned())),
            None => Err("native operation was already consumed".to_owned()),
        };
        task.finish(result);
    }
    0 // G_SOURCE_REMOVE
}

fn run_native_on_main_thread<T, F>(operation: F) -> Result<T, String>
where
    T: Send + 'static,
    F: FnOnce() -> Result<T, String> + Send + 'static,
{
    if is_main_thread() {
        return std::panic::catch_unwind(std::panic::AssertUnwindSafe(operation))
            .unwrap_or_else(|_| Err("native operation panicked on the main thread".to_owned()));
    }

    let idle_add = get_g_idle_add().ok_or_else(|| "g_idle_add 未在进程中找到".to_owned())?;
    let task = Arc::new(NativeThreadTask::new(operation));
    let raw_ptr = Arc::into_raw(task.clone()) as *mut c_void;
    if unsafe { idle_add(main_thread_native_dispatcher::<T>, raw_ptr) } == 0 {
        unsafe {
            drop(Arc::from_raw(raw_ptr as *const NativeThreadTask<T>));
        }
        return Err("g_idle_add failed to schedule native operation".to_owned());
    }
    task.wait(Duration::from_secs(5))
}

struct MainThreadSendTask {
    target: String,
    text: String,
    at_list: Option<String>,
    completion: SendCompletion,
}

unsafe extern "C" fn main_thread_send_dispatcher(data: *mut c_void) -> libc::c_int {
    let task = Arc::from_raw(data as *const MainThreadSendTask);
    if task.completion.begin() {
        let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            do_send_text_direct(&task.target, &task.text, task.at_list.as_deref())
        }))
        .unwrap_or_else(|_| Err("main-thread text dispatcher failed".to_owned()));
        task.completion.finish(outcome);
    }
    0 // G_SOURCE_REMOVE
}

// ==================== atuserlist 注入 Hook ====================

unsafe extern "C" fn hook_chat_id_copy(rdi: *mut c_void, rsi: *const c_void) -> *mut c_void {
    let orig = ORIGINAL_COPY_FN.load(Ordering::Acquire);
    if orig.is_null() {
        return rdi;
    }
    let orig_fn: CopyStdStringFn = mem::transmute(orig);

    // 1. 先执行原本的 chat_id 拷贝赋值
    let res = orig_fn(rdi, rsi);

    // 2. 判断入参是否为带有 SEND_PARAMS_MAGIC 的 SendTextParams
    if !rsi.is_null() && is_readable_ptr(rsi as *const u8) {
        let params = rsi as *const SendTextParams;
        if is_readable_ptr((rsi as usize + mem::size_of::<SendTextParams>() - 1) as *const u8) {
            if (*params).magic == SEND_PARAMS_MAGIC {
                let at_list_src = &(*params).at_user_list as *const StdString;
                let is_non_empty = ((*at_list_src).words[0] & 1 != 0
                    && (*at_list_src).words[1] > 0)
                    || ((*at_list_src).words[0] & 1 == 0 && (*at_list_src).words[0] >> 1 > 0);
                if is_non_empty {
                    // rdi 指向 obj + 0x90 (chat_id)。由此反推 obj 首地址
                    let obj = (rdi as usize).wrapping_sub(0x90) as *mut c_void;
                    // Message::atuserlist 位于 obj + 0x18
                    let at_list_dst = (obj as usize + 0x18) as *mut c_void;
                    orig_fn(at_list_dst, at_list_src as *const c_void);
                    bridge_info!(
                        "[SEND-TEXT] 成功将 atuserlist 写入底层 Message 对象 (偏移 +0x18) 地址: 0x{:x}",
                        at_list_dst as usize
                    );
                }
            }
        }
    }

    res
}

pub unsafe fn allocate_trampoline_near(target: usize) -> Option<*mut u8> {
    let page_size = 4096;
    for step in 1..=30 {
        for sign in &[1i64, -1i64] {
            let offset = sign * step * 0x4000000i64; // 64MB 跨步
            let hint = ((target as i64) + offset) as *mut c_void;
            let ptr = libc::mmap(
                hint,
                page_size,
                libc::PROT_READ | libc::PROT_WRITE | libc::PROT_EXEC,
                libc::MAP_PRIVATE | libc::MAP_ANONYMOUS,
                -1,
                0,
            );
            if ptr != libc::MAP_FAILED {
                let diff = (ptr as i64) - (target as i64);
                if diff >= -0x70000000i64 && diff <= 0x70000000i64 {
                    return Some(ptr as *mut u8);
                }
                let _ = libc::munmap(ptr, page_size);
            }
        }
    }
    None
}

pub unsafe fn install_sender_hooks(base: usize) -> bool {
    let call_site = base + RVA_CHAT_ID_COPY_CALL;
    let orig_target = base + RVA_STD_STRING_ASSIGN;
    ORIGINAL_COPY_FN.store(orig_target as *mut c_void, Ordering::Release);

    let trampoline = match allocate_trampoline_near(call_site) {
        Some(t) => t,
        None => {
            bridge_error!("[SENDER-HOOK] 无法在微信代码段 2GB 范围内分配跳板页面");
            return false;
        }
    };

    // 构建跳板指令:
    // 48 b8 xx xx xx xx xx xx xx xx : mov rax, <hook_chat_id_copy>
    // ff e0                         : jmp rax
    let mut code = [0u8; 12];
    code[0] = 0x48;
    code[1] = 0xb8;
    code[2..10].copy_from_slice(&(hook_chat_id_copy as *const () as usize).to_ne_bytes());
    code[10] = 0xff;
    code[11] = 0xe0;
    ptr::copy_nonoverlapping(code.as_ptr(), trampoline, 12);

    // 在 call_site 写入 rel32 call: E8 <rel32>
    let rel32 = (trampoline as i64 - (call_site as i64 + 5)) as i32;
    let mut patch = [0u8; 5];
    patch[0] = 0xe8;
    patch[1..5].copy_from_slice(&rel32.to_ne_bytes());

    if !patch_memory(call_site, &patch) {
        bridge_error!("[SENDER-HOOK] 写入 call_site 补丁失败: 0x{:x}", call_site);
        return false;
    }

    bridge_info!(
        "[SENDER-HOOK] 成功挂载 atuserlist 注入 Hook (call_site: 0x{:x} -> trampoline: 0x{:x} -> handler: 0x{:x})",
        call_site, trampoline as usize, hook_chat_id_copy as *const () as usize
    );
    true
}

fn extract_database_key_bytes(key_str: &StdString) -> Result<Vec<u8>, String> {
    let is_long = (key_str.words[0] & 1) != 0;
    let (len, data_ptr) = if is_long {
        (key_str.words[1], key_str.words[2] as *const u8)
    } else {
        (((key_str.words[0] & 0xff) >> 1) as usize, unsafe {
            (key_str.words.as_ptr() as *const u8).add(1)
        })
    };

    if len == 0 {
        return Err("未能从应用上下文提取数据库密钥".to_owned());
    }
    if len > 4096 {
        return Err("数据库密钥长度超过安全上限".to_owned());
    }
    if data_ptr.is_null() || !unsafe { is_readable_ptr(data_ptr) } {
        return Err("数据库密钥数据不可读".to_owned());
    }

    let mut bytes = vec![0u8; len];
    let local = libc::iovec {
        iov_base: bytes.as_mut_ptr().cast(),
        iov_len: len,
    };
    let remote = libc::iovec {
        iov_base: data_ptr as *mut c_void,
        iov_len: len,
    };
    let read = unsafe { libc::process_vm_readv(libc::getpid(), &local, 1, &remote, 1, 0) };
    if read != len as isize {
        return Err("数据库密钥数据不可完整读取".to_owned());
    }
    Ok(bytes)
}

fn database_key_hex(key_str: &StdString) -> Result<String, String> {
    let bytes = extract_database_key_bytes(key_str)?;
    Ok(bytes.iter().map(|byte| format!("{byte:02x}")).collect())
}

// ==================== 对外发送接口 ====================

fn do_get_database_info_direct() -> Result<Value, String> {
    type GetAppContextFn = unsafe extern "C" fn(*mut [usize; 2]) -> *mut [usize; 2];
    type GetDbKeyFn = unsafe extern "C" fn(*mut StdString, usize) -> *mut StdString;

    let base = ensure_native_ready("获取数据库密钥")?;
    let (self_wxid, _) =
        super::account::get_self_account().ok_or_else(|| "微信未登录或账号未就绪".to_owned())?;
    let get_app: GetAppContextFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_APP_CONTEXT_GETTER_SHARED) };
    let get_key_fn: GetDbKeyFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_DB_KEY_GETTER) };

    let mut app_ctx = NativeSharedPtr::new();
    unsafe { get_app(&mut app_ctx.0) };
    if app_ctx.0[0] == 0 {
        return Err("微信应用上下文不可用".to_owned());
    }

    let mut key_str = StdString::new("");
    unsafe { get_key_fn(&mut key_str, app_ctx.0[0]) };
    let key_result = database_key_hex(&key_str);
    key_str.dispose();
    let hex_key = key_result?;

    let home = std::env::var("HOME").unwrap_or_else(|_| "/root".to_owned());
    let db_dir = format!("{home}/.xwechat/xwechat_files/{self_wxid}/db");
    Ok(json!({
        "key": hex_key,
        "page_size": 4096,
        "cipher_version": 4,
        "wxid": self_wxid,
        "db_dir": db_dir,
    }))
}

pub fn get_database_info() -> Result<Value, String> {
    run_native_on_main_thread(do_get_database_info_direct)
}

pub fn decrypt_image_file(
    src_path: &str,
    dest_path: Option<&str>,
) -> Result<Value, String> {
    let bytes = std::fs::read(src_path).map_err(|e| format!("Failed to read source file: {e}"))?;
    if bytes.len() < 2 {
        return Err("File too small to be an encrypted image".to_owned());
    }

    let b0 = bytes[0];
    let b1 = bytes[1];
    let (key, ext) = if (b0 ^ 0xFF) == (b1 ^ 0xD8) {
        (b0 ^ 0xFF, "jpg")
    } else if (b0 ^ 0x89) == (b1 ^ 0x50) {
        (b0 ^ 0x89, "png")
    } else if (b0 ^ 0x47) == (b1 ^ 0x49) {
        (b0 ^ 0x47, "gif")
    } else if (b0 ^ 0x42) == (b1 ^ 0x4D) {
        (b0 ^ 0x42, "bmp")
    } else if b0 == 0xFF && b1 == 0xD8 {
        (0, "jpg")
    } else if b0 == 0x89 && b1 == 0x50 {
        (0, "png")
    } else if b0 == 0x47 && b1 == 0x49 {
        (0, "gif")
    } else {
        return Err("Unknown image encoding or not an encrypted .dat file".to_owned());
    };

    let decrypted: Vec<u8> = if key != 0 {
        bytes.iter().map(|b| b ^ key).collect()
    } else {
        bytes
    };
    let out_path = match dest_path.filter(|path| !path.is_empty()) {
        Some(path) => path.to_owned(),
        None if src_path.ends_with(".dat") => {
            format!("{}.{ext}", src_path.strip_suffix(".dat").unwrap_or(src_path))
        }
        None => format!("{src_path}.{ext}"),
    };
    std::fs::write(&out_path, &decrypted)
        .map_err(|e| format!("Failed to write decrypted image: {e}"))?;
    Ok(json!({
        "src_path": src_path,
        "dest_path": out_path,
        "extension": ext,
        "xor_key": key,
        "size": decrypted.len(),
    }))
}

fn do_send_text_direct(target: &str, text: &str, at_list: Option<&str>) -> Result<(), String> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return Err("微信主程序基址未初始化，暂无法发送消息".to_owned());
    }
    if super::account::get_self_account().is_none() {
        return Err("微信未登录或账号未就绪，禁止发送消息".to_owned());
    }

    let send_fn_addr = base + RVA_SEND_TEXT_MESSAGE;
    let send_fn: SendTextFn = unsafe { mem::transmute(send_fn_addr) };

    let mut chat_id_str = StdString::new(target);
    let mut content_str = StdString::new(text);
    let mut at_list_str = StdString::new(at_list.unwrap_or(""));
    let mut empty_method = StdString::new("");

    let params = SendTextParams {
        chat_id: chat_id_str,
        content: content_str,
        at_user_list: at_list_str,
        magic: SEND_PARAMS_MAGIC,
        _pad: [0; 4],
    };

    let cb_heap = unsafe {
        let p = libc::malloc(mem::size_of::<CallbackWrapper>()) as *mut CallbackWrapper;
        if p.is_null() {
            chat_id_str.dispose();
            content_str.dispose();
            at_list_str.dispose();
            empty_method.dispose();
            return Err("malloc CallbackWrapper failed".to_owned());
        }
        (*p).refcount = std::sync::atomic::AtomicI32::new(1);
        (*p).invoke_fn = cb_invoke;
        (*p).destroy_fn = cb_destroy;
        p
    };

    let mut cb_holder = cb_heap;

    bridge_info!(
        "[SEND-TEXT] 主线程开始发送消息 -> 目标: {}, @列表: '{}', 长度: {} 字节",
        target,
        at_list.unwrap_or(""),
        text.len()
    );

    let res = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        send_fn(0, &params, &empty_method, &mut cb_holder);
    }));

    chat_id_str.dispose();
    content_str.dispose();
    at_list_str.dispose();
    empty_method.dispose();

    match res {
        Ok(()) => {
            bridge_info!("[SEND-TEXT] 主线程发送调用成功");
            Ok(())
        }
        Err(_) => {
            bridge_error!("[SEND-TEXT] 主线程发送调用发生异常");
            Err("send_text failed with exception in main thread".to_owned())
        }
    }
}

pub fn send_text(target: &str, text: &str) -> Result<(), String> {
    send_text_with_at(target, text, None)
}

fn do_set_chat_read_direct(target: &str) -> Result<(), String> {
    let base = ensure_native_ready("set chat read")?;
    type SetChatReadFn = unsafe extern "C" fn(*const StdString) -> usize;
    let set_read: SetChatReadFn = unsafe {
        mem::transmute(base + crate::offsets::RVA_SET_CHAT_READ)
    };
    let mut target_str = StdString::new(target);
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        set_read(&target_str);
    }));
    target_str.dispose();
    outcome
        .map(|_| ())
        .map_err(|_| "set_chat_read failed on main thread".to_owned())
}

pub fn set_chat_read(target: &str) -> Result<(), String> {
    let target_clean = target.trim().to_owned();
    if target_clean.is_empty() {
        return Err("target is empty".to_owned());
    }
    run_native_on_main_thread(move || do_set_chat_read_direct(&target_clean))
}

pub fn send_text_with_at(target: &str, text: &str, at_list: Option<&str>) -> Result<(), String> {
    if is_main_thread() {
        return do_send_text_direct(target, text, at_list);
    }

    let idle_add = get_g_idle_add().ok_or_else(|| "g_idle_add 未在进程中找到".to_owned())?;

    let task = Arc::new(MainThreadSendTask {
        target: target.to_owned(),
        text: text.to_owned(),
        at_list: at_list.map(|s| s.to_owned()),
        completion: SendCompletion::new(),
    });

    let raw_ptr = Arc::into_raw(task.clone()) as *mut c_void;
    if unsafe { idle_add(main_thread_send_dispatcher, raw_ptr) } == 0 {
        unsafe {
            drop(Arc::from_raw(raw_ptr as *const MainThreadSendTask));
        }
        return Err("g_idle_add failed to schedule text send".to_owned());
    }
    task.completion.wait(Duration::from_secs(5))
}

// ==================== 好友申请同意 ====================

type VerifyUserFn = unsafe extern "C" fn(
    result_buf: *mut u8,
    context: usize,
    wxid: *const StdString,
    opcode: i32,
    scene: u32,
    scan_qrcode: i32,
    ticket: *const StdString,
    content: *const StdString,
    encryptusername: *const StdString,
) -> *mut u8;

struct MainThreadVerifyUserTask {
    wxid: String,
    ticket: String,
    encryptusername: String,
    scene: u32,
    remark: String,
    completion: SendCompletion,
}

fn do_agree_friend_req_direct(
    wxid: &str,
    ticket: &str,
    encryptusername: &str,
    scene: u32,
    remark: &str,
) -> Result<(), String> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return Err("微信主程序基址未初始化，暂无法处理好友申请".to_owned());
    }
    if super::account::get_self_account().is_none() {
        return Err("微信未登录或账号未就绪，无法处理好友申请".to_owned());
    }

    let verify_fn: VerifyUserFn = unsafe { mem::transmute(base + crate::offsets::RVA_VERIFY_USER) };
    let mut wxid_str = StdString::new(wxid);
    let mut ticket_str = StdString::new(ticket);
    let mut remark_str = StdString::new(remark);
    let mut encryptusername_str = StdString::new(encryptusername);
    let mut result_buf = [0u8; 128];

    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        verify_fn(
            result_buf.as_mut_ptr(),
            0,
            &wxid_str,
            2,
            scene,
            0,
            &ticket_str,
            &remark_str,
            &encryptusername_str,
        )
    }));

    wxid_str.dispose();
    ticket_str.dispose();
    remark_str.dispose();
    encryptusername_str.dispose();

    match result {
        Ok(result_ptr) => {
            let response = if result_ptr.is_null() {
                result_buf.as_mut_ptr()
            } else {
                result_ptr
            };
            if !unsafe { is_readable_ptr(response as *const u8) } {
                return Err("VerifyUser returned an unreadable result pointer".to_owned());
            }
            if unsafe { *response } != 0 {
                bridge_info!(
                    "[VERIFY-USER] 已提交好友申请同意: wxid={}, scene={}",
                    wxid,
                    scene
                );
                Ok(())
            } else {
                bridge_error!("[VERIFY-USER] 微信未确认好友申请同意: wxid={}", wxid);
                Err("VerifyUser returned an unsuccessful result".to_owned())
            }
        }
        Err(_) => {
            bridge_error!("[VERIFY-USER] 调用微信好友申请处理函数发生异常");
            Err("agree_friend_req failed with exception in main thread".to_owned())
        }
    }
}

unsafe extern "C" fn main_thread_verify_user_dispatcher(data: *mut c_void) -> libc::c_int {
    let task = Arc::from_raw(data as *const MainThreadVerifyUserTask);
    if task.completion.begin() {
        let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            do_agree_friend_req_direct(
                &task.wxid,
                &task.ticket,
                &task.encryptusername,
                task.scene,
                &task.remark,
            )
        }))
        .unwrap_or_else(|_| Err("main-thread VerifyUser dispatcher failed".to_owned()));
        task.completion.finish(outcome);
    }
    0 // G_SOURCE_REMOVE
}

pub fn agree_friend_req(
    wxid: &str,
    ticket: &str,
    encryptusername: &str,
    scene: u32,
    remark: &str,
) -> Result<(), String> {
    if wxid.trim().is_empty() {
        return Err("wxid is empty".to_owned());
    }
    if ticket.trim().is_empty() {
        return Err("ticket is empty".to_owned());
    }
    if is_main_thread() {
        return do_agree_friend_req_direct(wxid, ticket, encryptusername, scene, remark);
    }

    let idle_add = get_g_idle_add().ok_or_else(|| "g_idle_add 未在进程中找到".to_owned())?;
    let task = Arc::new(MainThreadVerifyUserTask {
        wxid: wxid.to_owned(),
        ticket: ticket.to_owned(),
        encryptusername: encryptusername.to_owned(),
        scene,
        remark: remark.to_owned(),
        completion: SendCompletion::new(),
    });
    let raw_ptr = Arc::into_raw(task.clone()) as *mut c_void;
    if unsafe { idle_add(main_thread_verify_user_dispatcher, raw_ptr) } == 0 {
        unsafe {
            drop(Arc::from_raw(raw_ptr as *const MainThreadVerifyUserTask));
        }
        return Err("g_idle_add failed to schedule VerifyUser".to_owned());
    }
    task.completion.wait(Duration::from_secs(5))
}

// ==================== 联系人删除 ====================

struct NativeSharedPtr([usize; 2]);

impl NativeSharedPtr {
    fn new() -> Self {
        Self([0; 2])
    }
}

impl Drop for NativeSharedPtr {
    fn drop(&mut self) {
        unsafe { release_native_shared_ptr(&mut self.0) };
    }
}

unsafe fn release_native_shared_ptr(shared: &mut [usize; 2]) {
    let control = shared[1] as *mut u8;
    if !control.is_null() {
        // WeChat's control block stores the shared count after its vptr; the
        // +0x10 vtable entry disposes the managed object when the count reaches 0.
        let shared_count = &*(control.add(8) as *const AtomicI32);
        if shared_count.fetch_sub(1, Ordering::Release) == 1 {
            std::sync::atomic::fence(Ordering::Acquire);
            let vtable = *(control as *const *const usize);
            if !vtable.is_null() {
                let destroy_address = *vtable.add(2);
                if destroy_address != 0 {
                    let destroy: unsafe extern "C" fn(*mut u8) = mem::transmute(destroy_address);
                    destroy(control);
                }
            }
        }
    }
    *shared = [0; 2];
}

struct MainThreadDeleteFriendTask {
    wxid: String,
    completion: SendCompletion,
}

fn do_delete_friend_direct(wxid: &str) -> Result<(), String> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return Err("微信主程序基址未初始化，暂无法删除联系人".to_owned());
    }
    if super::account::get_self_account().is_none() {
        return Err("微信未登录或账号未就绪，无法删除联系人".to_owned());
    }

    type GetAppContextFn = unsafe extern "C" fn(*mut [usize; 2]) -> *mut [usize; 2];
    type GetContactServiceFn = unsafe extern "C" fn(*mut [usize; 2], usize) -> *mut [usize; 2];
    type DeleteContactFn = unsafe extern "C" fn(usize, *const StdString);

    let get_app: GetAppContextFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_APP_CONTEXT_GETTER_SHARED) };
    let get_provider: GetContactServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CONTACT_PROVIDER_GETTER) };
    let get_service: GetContactServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CONTACT_SERVICE_GETTER) };
    let delete_contact: DeleteContactFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_DELETE_CONTACT) };

    let mut app = NativeSharedPtr::new();
    unsafe { get_app(&mut app.0) };
    if app.0[0] == 0 {
        return Err("微信应用上下文不可用".to_owned());
    }

    let mut provider = NativeSharedPtr::new();
    unsafe { get_provider(&mut provider.0, app.0[0]) };
    if provider.0[0] == 0 {
        return Err("微信联系人 Provider 不可用".to_owned());
    }

    let mut service = NativeSharedPtr::new();
    unsafe { get_service(&mut service.0, provider.0[0]) };
    if service.0[0] == 0 {
        return Err("微信联系人 Service 不可用".to_owned());
    }

    let mut target_wxid = StdString::new(wxid);
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        delete_contact(service.0[0], &target_wxid);
    }));
    target_wxid.dispose();

    match outcome {
        Ok(()) => {
            bridge_info!("[DELETE-FRIEND] 已提交删除好友: wxid={}", wxid);
            Ok(())
        }
        Err(_) => {
            bridge_error!(
                "[DELETE-FRIEND] 调用微信联系人删除函数发生异常: wxid={}",
                wxid
            );
            Err("del_friend failed with exception in main thread".to_owned())
        }
    }
}

unsafe extern "C" fn main_thread_delete_friend_dispatcher(data: *mut c_void) -> libc::c_int {
    let task = Arc::from_raw(data as *const MainThreadDeleteFriendTask);
    if task.completion.begin() {
        let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            do_delete_friend_direct(&task.wxid)
        }))
        .unwrap_or_else(|_| Err("main-thread delFriend dispatcher failed".to_owned()));
        task.completion.finish(outcome);
    }
    0 // G_SOURCE_REMOVE
}

pub fn del_friend(wxid: &str) -> Result<(), String> {
    let wxid = wxid.trim();
    if wxid.is_empty() {
        return Err("wxid is empty".to_owned());
    }
    if is_main_thread() {
        return do_delete_friend_direct(wxid);
    }

    let idle_add = get_g_idle_add().ok_or_else(|| "g_idle_add 未在进程中找到".to_owned())?;
    let task = Arc::new(MainThreadDeleteFriendTask {
        wxid: wxid.to_owned(),
        completion: SendCompletion::new(),
    });
    let raw_ptr = Arc::into_raw(task.clone()) as *mut c_void;
    if unsafe { idle_add(main_thread_delete_friend_dispatcher, raw_ptr) } == 0 {
        unsafe {
            drop(Arc::from_raw(raw_ptr as *const MainThreadDeleteFriendTask));
        }
        return Err("g_idle_add failed to schedule delFriend".to_owned());
    }
    task.completion.wait(Duration::from_secs(5))
}

fn do_revoke_msg_direct(
    to_user: &str,
    client_msg_id: &str,
    svr_msg_id: u64,
    create_time: u32,
) -> Result<(), String> {
    type GetAppContextFn = unsafe extern "C" fn(*mut [usize; 2]) -> *mut [usize; 2];
    type RevokeSendCgiFn =
        unsafe extern "C" fn(*mut u8, usize, *const u8, *const StdString, *const i32) -> *mut u8;

    let base = ensure_native_ready("撤回消息")?;
    let (self_wxid, _) = super::account::get_self_account()
        .ok_or_else(|| "微信未登录或账号未就绪，无法撤回消息".to_owned())?;
    let get_app: GetAppContextFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_APP_CONTEXT_GETTER_SHARED) };
    let mut app_ctx = NativeSharedPtr::new();
    unsafe { get_app(&mut app_ctx.0) };
    if app_ctx.0[0] == 0 {
        return Err("微信应用上下文不可用，无法撤回消息".to_owned());
    }
    let vtable = unsafe { *(app_ctx.0[0] as *const *const usize) };
    if vtable.is_null() || unsafe { *vtable.add(3) } == 0 {
        return Err("微信网络服务入口不可用".to_owned());
    }
    let get_net_svc: unsafe extern "C" fn(usize) -> usize =
        unsafe { mem::transmute(*vtable.add(3)) };
    let net_svc = unsafe { get_net_svc(app_ctx.0[0]) };
    if net_svc == 0 {
        return Err("微信网络服务不可用".to_owned());
    }

    let req_init: unsafe extern "C" fn(*mut u8) =
        unsafe { mem::transmute(base + crate::offsets::RVA_REVOKE_MSG_REQUEST_INIT) };
    let req_free: unsafe extern "C" fn(*mut u8) =
        unsafe { mem::transmute(base + crate::offsets::RVA_REVOKE_MSG_REQUEST_FREE) };
    let revoke_send: RevokeSendCgiFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_REVOKE_MSG_SEND_CGI) };

    let mut req_buf = [0u8; 72];
    unsafe { req_init(req_buf.as_mut_ptr()) };
    unsafe {
        ptr::write(
            req_buf.as_mut_ptr().add(16) as *mut StdString,
            StdString::new(client_msg_id),
        );
        let timestamp = if create_time == 0 {
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_secs() as u32
        } else {
            create_time
        };
        ptr::write(req_buf.as_mut_ptr().add(24) as *mut u32, timestamp);
        ptr::write(
            req_buf.as_mut_ptr().add(32) as *mut StdString,
            StdString::new(&self_wxid),
        );
        ptr::write(
            req_buf.as_mut_ptr().add(48) as *mut StdString,
            StdString::new(to_user),
        );
        ptr::write(req_buf.as_mut_ptr().add(56) as *mut u64, svr_msg_id);
        ptr::write(req_buf.as_mut_ptr().add(64) as *mut u32, 0);
    }

    let mut result_buf = [0u8; 64];
    let mut empty_str = StdString::new("");
    let timeouts = [0i32; 5];
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        revoke_send(
            result_buf.as_mut_ptr(),
            net_svc,
            req_buf.as_ptr(),
            &empty_str,
            timeouts.as_ptr(),
        );
    }));
    unsafe {
        empty_str.dispose();
        (&mut *(req_buf.as_mut_ptr().add(16) as *mut StdString)).dispose();
        (&mut *(req_buf.as_mut_ptr().add(32) as *mut StdString)).dispose();
        (&mut *(req_buf.as_mut_ptr().add(48) as *mut StdString)).dispose();
        req_free(req_buf.as_mut_ptr());
    }
    outcome.map_err(|_| "revoke_msg failed with exception on the main thread".to_owned())
}

pub fn revoke_msg(
    to_user: &str,
    client_msg_id: &str,
    svr_msg_id: u64,
    create_time: u32,
) -> Result<(), String> {
    let to_user = to_user.trim().to_owned();
    let client_msg_id = client_msg_id.trim().to_owned();
    if to_user.is_empty() {
        return Err("to_user is empty".to_owned());
    }
    if client_msg_id.is_empty() {
        return Err("client_msg_id is empty".to_owned());
    }
    run_native_on_main_thread(move || {
        do_revoke_msg_direct(&to_user, &client_msg_id, svr_msg_id, create_time)
    })
}

fn do_get_member_list_direct(chatroom_name: &str) -> Result<Value, String> {
    type GetChatroomInfoFn =
        unsafe extern "C" fn(*mut [usize; 2], usize, *const StdString) -> *mut [usize; 2];

    let base = ensure_native_ready("获取群成员列表")?;
    let (_app, _provider, chatroom_svc) = chatroom_service_handles(base)?;
    let get_info: GetChatroomInfoFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_GET_CHATROOM_INFO) };
    let mut room_str = StdString::new(chatroom_name);
    let mut shared_info = NativeSharedPtr::new();
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        get_info(&mut shared_info.0, chatroom_svc.0[0], &room_str);
    }));
    room_str.dispose();
    outcome.map_err(|_| "get_member_list failed with exception on the main thread".to_owned())?;
    if shared_info.0[0] == 0 {
        return Err("Chatroom not found".to_owned());
    }

    let info_ptr = shared_info.0[0] as *const u8;
    for offset in [32usize, 88, 136] {
        if !unsafe { is_readable_ptr(info_ptr.add(offset)) } {
            return Err("Chatroom information is unreadable".to_owned());
        }
    }
    let owner_wxid = unsafe { (&*(info_ptr.add(32) as *const StdString)).as_str() }
        .unwrap_or_default()
        .to_owned();
    let topic = unsafe { (&*(info_ptr.add(88) as *const StdString)).as_str() }
        .unwrap_or_default()
        .to_owned();
    let members_raw = unsafe { (&*(info_ptr.add(136) as *const StdString)).as_str() }
        .unwrap_or_default()
        .to_owned();
    let members = members_raw
        .split(|character| character == '^' || character == ';')
        .filter(|member| !member.is_empty())
        .map(|member| {
            let nickname = super::account::get_cached_nickname(member).unwrap_or_default();
            json!({
                "wxid": member,
                "nickname": nickname,
                "is_owner": member == owner_wxid,
            })
        })
        .collect::<Vec<_>>();
    Ok(json!({
        "chatroom_name": chatroom_name,
        "topic": topic,
        "owner_wxid": owner_wxid,
        "member_count": members.len(),
        "members": members,
    }))
}

pub fn get_member_list(chatroom_name: &str) -> Result<Value, String> {
    let room = chatroom_name.trim().to_owned();
    if room.is_empty() {
        return Err("chatroom_name is empty".to_owned());
    }
    run_native_on_main_thread(move || do_get_member_list_direct(&room))
}

fn do_get_contact_lists_direct() -> Result<(Value, Value), String> {
    type GetAppContextFn = unsafe extern "C" fn(*mut [usize; 2]) -> *mut [usize; 2];
    type GetServiceFn = unsafe extern "C" fn(*mut [usize; 2], usize) -> *mut [usize; 2];
    type GetAllContactsFn = unsafe extern "C" fn(*mut [usize; 5], usize) -> *mut [usize; 5];

    let base = ensure_native_ready("获取联系人列表")?;
    let get_app: GetAppContextFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_APP_CONTEXT_GETTER_SHARED) };
    let get_provider: GetServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CONTACT_PROVIDER_GETTER) };
    let get_storage: GetServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CONTACT_STORAGE_GETTER) };
    let mut app = NativeSharedPtr::new();
    unsafe { get_app(&mut app.0) };
    if app.0[0] == 0 {
        return Err("微信应用上下文不可用".to_owned());
    }
    let mut provider = NativeSharedPtr::new();
    unsafe { get_provider(&mut provider.0, app.0[0]) };
    if provider.0[0] == 0 {
        return Err("微信联系人 Provider 不可用".to_owned());
    }
    let mut storage_svc = NativeSharedPtr::new();
    unsafe { get_storage(&mut storage_svc.0, provider.0[0]) };
    if storage_svc.0[0] == 0 {
        return Err("微信联系人存储服务不可用".to_owned());
    }

    let get_all: GetAllContactsFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_GET_ALL_ROOM_CONTACTS) };
    let mut map_storage = [0usize; 5];
    let result = unsafe { get_all(&mut map_storage, storage_svc.0[0]) };
    if result.is_null() {
        return Err("联系人列表枚举失败".to_owned());
    }

    let (self_wxid, _) = super::account::get_self_account().unwrap_or_default();
    let mut groups = Vec::new();
    let mut friends = Vec::new();
    let mut current = map_storage[2];
    let mut visited = HashSet::new();
    while current != 0 {
        if visited.len() >= 100_000 {
            return Err("联系人列表节点数超过安全上限".to_owned());
        }
        if !visited.insert(current) {
            return Err("联系人列表包含循环链表".to_owned());
        }
        if !unsafe { is_readable_ptr(current as *const u8) }
            || !unsafe { is_readable_ptr((current + 16) as *const u8) }
        {
            return Err("联系人列表节点不可读".to_owned());
        }
        let node_ptr = current as *const u8;
        let next = unsafe { *(node_ptr as *const usize) };
        let username = unsafe { (&*(node_ptr.add(16) as *const StdString)).as_str() };
        if let Some(username) = username {
            if username.ends_with("@chatroom") {
                groups.push(json!({
                    "chatroom_name": username,
                    "wxid": username,
                }));
            } else if !username.starts_with("gh_")
                && username != self_wxid
                && !username.ends_with("@openim")
                && !username.ends_with("@app")
            {
                let nickname = super::account::get_cached_nickname(username).unwrap_or_default();
                friends.push(json!({ "wxid": username, "nickname": nickname }));
            }
        }
        current = next;
    }

    // The native nodes are borrowed from the contact storage. They must not be
    // individually freed by the bridge; all native pointers stay within this call.
    Ok((Value::Array(groups), Value::Array(friends)))
}

pub fn get_group_list() -> Result<Value, String> {
    run_native_on_main_thread(|| do_get_contact_lists_direct().map(|(groups, _friends)| groups))
}

pub fn get_friend_list() -> Result<Value, String> {
    run_native_on_main_thread(|| do_get_contact_lists_direct().map(|(_groups, friends)| friends))
}

fn ensure_native_ready(action: &str) -> Result<usize, String> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return Err(format!("微信主程序基址未初始化，暂无法{action}"));
    }
    if super::account::get_self_account().is_none() {
        return Err(format!("微信未登录或账号未就绪，无法{action}"));
    }
    Ok(base)
}

fn chatroom_service_handles(
    base: usize,
) -> Result<(NativeSharedPtr, NativeSharedPtr, NativeSharedPtr), String> {
    type GetAppContextFn = unsafe extern "C" fn(*mut [usize; 2]) -> *mut [usize; 2];
    type GetServiceFn = unsafe extern "C" fn(*mut [usize; 2], usize) -> *mut [usize; 2];

    let get_app: GetAppContextFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_APP_CONTEXT_GETTER_SHARED) };
    let get_provider: GetServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CONTACT_PROVIDER_GETTER) };
    let get_chatroom: GetServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CHATROOM_SERVICE_GETTER) };

    let mut app = NativeSharedPtr::new();
    unsafe { get_app(&mut app.0) };
    if app.0[0] == 0 {
        return Err("微信应用上下文不可用".to_owned());
    }

    let mut provider = NativeSharedPtr::new();
    unsafe { get_provider(&mut provider.0, app.0[0]) };
    if provider.0[0] == 0 {
        return Err("微信联系人 Provider 不可用".to_owned());
    }

    let mut service = NativeSharedPtr::new();
    unsafe { get_chatroom(&mut service.0, provider.0[0]) };
    if service.0[0] == 0 {
        return Err("微信 Chatroom Service 不可用".to_owned());
    }
    Ok((app, provider, service))
}

fn oplog_service_handles(
    base: usize,
) -> Result<(NativeSharedPtr, NativeSharedPtr, NativeSharedPtr), String> {
    type GetAppContextFn = unsafe extern "C" fn(*mut [usize; 2]) -> *mut [usize; 2];
    type GetServiceFn = unsafe extern "C" fn(*mut [usize; 2], usize) -> *mut [usize; 2];

    let get_app: GetAppContextFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_APP_CONTEXT_GETTER_SHARED) };
    let get_provider: GetServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CONTACT_PROVIDER_GETTER) };
    let get_oplog: GetServiceFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_OPLOG_SERVICE_GETTER) };

    let mut app = NativeSharedPtr::new();
    unsafe { get_app(&mut app.0) };
    if app.0[0] == 0 {
        return Err("微信应用上下文不可用".to_owned());
    }

    let mut provider = NativeSharedPtr::new();
    unsafe { get_provider(&mut provider.0, app.0[0]) };
    if provider.0[0] == 0 {
        return Err("微信联系人 Provider 不可用".to_owned());
    }

    let mut service = NativeSharedPtr::new();
    unsafe { get_oplog(&mut service.0, provider.0[0]) };
    if service.0[0] == 0 {
        return Err("微信 Oplog Service 不可用".to_owned());
    }
    Ok((app, provider, service))
}

fn normalized_members(member_wxids: &[String]) -> Result<Vec<String>, String> {
    let members = member_wxids
        .iter()
        .map(|member| member.trim().to_owned())
        .collect::<Vec<_>>();
    if members.is_empty() || members.iter().any(String::is_empty) {
        return Err("member list must contain non-empty wxids".to_owned());
    }
    Ok(members)
}

fn do_create_group_direct(member_wxids: &[String], topic: &str) -> Result<String, String> {
    type CreateChatRoomFn = unsafe extern "C" fn(
        *mut StdString,
        usize,
        *const usize,
        *const StdString,
    ) -> *mut StdString;

    let base = ensure_native_ready("创建群聊")?;
    let (_app, _provider, chatroom_svc) = chatroom_service_handles(base)?;
    let members = normalized_members(member_wxids)?;
    let member_refs = members.iter().map(String::as_str).collect::<Vec<_>>();
    let native_members = NativeStdVector::from_strings(&member_refs);
    let mut result_wxid = StdString::new("");
    let mut topic_str = StdString::new(topic);
    let create: CreateChatRoomFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CREATE_CHATROOM) };

    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        create(
            &mut result_wxid,
            chatroom_svc.0[0],
            native_members.as_ptr(),
            &topic_str,
        );
    }));
    let room_id = result_wxid.as_str().unwrap_or_default().to_owned();
    result_wxid.dispose();
    topic_str.dispose();
    drop(native_members);

    match outcome {
        Ok(()) if !room_id.is_empty() => {
            bridge_info!("[CREATE-GROUP] 已创建群聊: {}", room_id);
            Ok(room_id)
        }
        Ok(()) => Err("create_group returned an empty chatroom id".to_owned()),
        Err(_) => Err("create_group failed with exception on the main thread".to_owned()),
    }
}

pub fn create_group(member_wxids: &[String], topic: &str) -> Result<String, String> {
    let members = member_wxids.to_vec();
    let topic = topic.to_owned();
    run_native_on_main_thread(move || do_create_group_direct(&members, &topic))
}

fn do_del_members_direct(chatroom_name: &str, member_wxids: &[String]) -> Result<(), String> {
    type DelChatRoomMemberFn =
        unsafe extern "C" fn(*mut [u8; 64], usize, *const usize, *const StdString, u32) -> usize;

    let base = ensure_native_ready("移出群成员")?;
    let (_app, _provider, chatroom_svc) = chatroom_service_handles(base)?;
    let members = normalized_members(member_wxids)?;
    let member_refs = members.iter().map(String::as_str).collect::<Vec<_>>();
    let native_members = NativeStdVector::from_strings(&member_refs);
    let mut room_str = StdString::new(chatroom_name);
    let mut result_buf = [0u8; 64];
    let delete: DelChatRoomMemberFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_DEL_CHATROOM_MEMBER) };
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        delete(
            &mut result_buf,
            chatroom_svc.0[0],
            native_members.as_ptr(),
            &room_str,
            0,
        );
    }));
    room_str.dispose();
    drop(native_members);
    outcome
        .map(|_| ())
        .map_err(|_| "del_members failed with exception on the main thread".to_owned())
}

pub fn del_members(chatroom_name: &str, member_wxids: &[String]) -> Result<(), String> {
    let room = chatroom_name.trim().to_owned();
    if room.is_empty() {
        return Err("chatroom name is empty".to_owned());
    }
    let members = member_wxids.to_vec();
    run_native_on_main_thread(move || do_del_members_direct(&room, &members))
}

fn do_add_members_direct(chatroom_name: &str, member_wxids: &[String]) -> Result<(), String> {
    type AddChatRoomMemberFn = unsafe extern "C" fn(
        usize,
        *const usize,
        *const StdString,
        *const u8,
        *mut StdString,
    ) -> u8;

    let base = ensure_native_ready("邀请群成员")?;
    let (_app, _provider, chatroom_svc) = chatroom_service_handles(base)?;
    let members = normalized_members(member_wxids)?;
    let member_refs = members.iter().map(String::as_str).collect::<Vec<_>>();
    let native_members = NativeStdVector::from_strings(&member_refs);
    let mut room_str = StdString::new(chatroom_name);
    let mut result_str = StdString::new("");
    let add: AddChatRoomMemberFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_ADD_CHATROOM_MEMBER) };
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        add(
            chatroom_svc.0[0],
            native_members.as_ptr(),
            &room_str,
            ptr::null(),
            &mut result_str,
        );
    }));
    result_str.dispose();
    room_str.dispose();
    drop(native_members);
    outcome
        .map(|_| ())
        .map_err(|_| "add_members failed with exception on the main thread".to_owned())
}

pub fn add_members(chatroom_name: &str, member_wxids: &[String]) -> Result<(), String> {
    let room = chatroom_name.trim().to_owned();
    if room.is_empty() {
        return Err("chatroom name is empty".to_owned());
    }
    let members = member_wxids.to_vec();
    run_native_on_main_thread(move || do_add_members_direct(&room, &members))
}

fn do_quit_group_direct(chatroom_name: &str) -> Result<(), String> {
    type QuitChatRoomFn = unsafe extern "C" fn(usize, *const StdString) -> usize;

    let base = ensure_native_ready("退出群聊")?;
    let (_app, _provider, oplog_svc) = oplog_service_handles(base)?;
    let mut room_str = StdString::new(chatroom_name);
    let quit: QuitChatRoomFn = unsafe { mem::transmute(base + crate::offsets::RVA_QUIT_CHATROOM) };
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        quit(oplog_svc.0[0], &room_str);
    }));
    room_str.dispose();
    outcome
        .map(|_| ())
        .map_err(|_| "quit_group failed with exception on the main thread".to_owned())
}

pub fn quit_group(chatroom_name: &str) -> Result<(), String> {
    let room = chatroom_name.trim().to_owned();
    if room.is_empty() {
        return Err("chatroom name is empty".to_owned());
    }
    run_native_on_main_thread(move || do_quit_group_direct(&room))
}

fn do_edit_self_member_nick_direct(chatroom_name: &str, new_nick: &str) -> Result<(), String> {
    type ModChatRoomMemberNickFn =
        unsafe extern "C" fn(usize, *const StdString, *const StdString, *const StdString) -> usize;

    let base = ensure_native_ready("修改群昵称")?;
    let (self_wxid, _) = super::account::get_self_account()
        .ok_or_else(|| "微信未登录或账号未就绪，无法修改群昵称".to_owned())?;
    let (_app, _provider, oplog_svc) = oplog_service_handles(base)?;
    let mut room_str = StdString::new(chatroom_name);
    let mut self_str = StdString::new(&self_wxid);
    let mut nick_str = StdString::new(new_nick);
    let modify: ModChatRoomMemberNickFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_MOD_CHATROOM_MEMBER_NICK) };
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        modify(oplog_svc.0[0], &room_str, &self_str, &nick_str);
    }));
    nick_str.dispose();
    self_str.dispose();
    room_str.dispose();
    outcome
        .map(|_| ())
        .map_err(|_| "edit_self_member_nick failed with exception on the main thread".to_owned())
}

pub fn edit_self_member_nick(chatroom_name: &str, new_nick: &str) -> Result<(), String> {
    let room = chatroom_name.trim().to_owned();
    if room.is_empty() {
        return Err("chatroom name is empty".to_owned());
    }
    let nick = new_nick.to_owned();
    run_native_on_main_thread(move || do_edit_self_member_nick_direct(&room, &nick))
}

#[repr(C)]
struct EditRemarkObj {
    vtable: usize,
    wxid: StdString,
    remark: StdString,
    padding: usize,
}

fn do_edit_obj_remark_direct(wxid: &str, remark: &str) -> Result<(), String> {
    type EditRemarkFn = unsafe extern "C" fn(*const EditRemarkObj) -> usize;

    let base = ensure_native_ready("修改好友备注")?;
    let mut obj = EditRemarkObj {
        vtable: base + crate::offsets::RVA_VTABLE_EDIT_REMARK,
        wxid: StdString::new(wxid),
        remark: StdString::new(remark),
        padding: 0,
    };
    let edit: EditRemarkFn = unsafe { mem::transmute(base + crate::offsets::RVA_EDIT_OBJ_REMARK) };
    let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        edit(&obj);
    }));
    obj.remark.dispose();
    obj.wxid.dispose();
    outcome
        .map(|_| ())
        .map_err(|_| "edit_obj_remark failed with exception on the main thread".to_owned())
}

pub fn edit_obj_remark(wxid: &str, remark: &str) -> Result<(), String> {
    let wxid = wxid.trim().to_owned();
    if wxid.is_empty() {
        return Err("wxid is empty".to_owned());
    }
    let remark = remark.to_owned();
    run_native_on_main_thread(move || do_edit_obj_remark_direct(&wxid, &remark))
}

// ==================== 红包 CGI 请求 ====================

type HongBaoReqInitFn = unsafe extern "C" fn(*mut u8);
type HongBaoReqFreeFn = unsafe extern "C" fn(*mut u8);
type SkBufferInitFn = unsafe extern "C" fn(*mut u8);
type HongBaoResFreeFn = unsafe extern "C" fn(*mut u8);
type GetNetworkServiceFn = unsafe extern "C" fn(*mut c_void) -> *mut c_void;
type CoCgiSendCommonRequestFn = unsafe extern "C" fn(
    res_buf: *mut u8,
    net_svc: *mut c_void,
    req_buf: *const c_void,
    cgi_id: i32,
    cgi_url: *const StdString,
    empty_str: *const StdString,
    options: *const [u32; 2],
) -> *mut u8;

struct HongBaoReqGuard {
    storage: Box<[usize; 6]>,
    base: usize,
}

impl HongBaoReqGuard {
    fn new(base: usize) -> Self {
        let mut guard = Self {
            storage: Box::new([0; 6]),
            base,
        };
        let init: HongBaoReqInitFn =
            unsafe { mem::transmute(base + crate::offsets::RVA_HONGBAO_REQ_INIT) };
        unsafe { init(guard.storage.as_mut_ptr().cast()) };
        guard
    }

    fn as_mut_ptr(&mut self) -> *mut u8 {
        self.storage.as_mut_ptr().cast()
    }
}

impl Drop for HongBaoReqGuard {
    fn drop(&mut self) {
        let free: HongBaoReqFreeFn =
            unsafe { mem::transmute(self.base + crate::offsets::RVA_HONGBAO_REQ_FREE) };
        unsafe { free(self.storage.as_mut_ptr().cast()) };
    }
}

struct HongBaoResGuard {
    storage: Box<[usize; 8]>,
    base: usize,
}

impl HongBaoResGuard {
    fn new(base: usize) -> Self {
        Self {
            storage: Box::new([0; 8]),
            base,
        }
    }

    fn as_mut_ptr(&mut self) -> *mut u8 {
        self.storage.as_mut_ptr().cast()
    }
}

impl Drop for HongBaoResGuard {
    fn drop(&mut self) {
        let free: HongBaoResFreeFn =
            unsafe { mem::transmute(self.base + crate::offsets::RVA_HONGBAO_RES_FREE) };
        unsafe { free(self.storage.as_mut_ptr().cast()) };
    }
}

fn query_encode(value: &str) -> String {
    let mut encoded = String::with_capacity(value.len());
    for byte in value.as_bytes() {
        if byte.is_ascii_alphanumeric() || matches!(*byte, b'-' | b'.' | b'_' | b'~') {
            encoded.push(*byte as char);
        } else {
            use std::fmt::Write as _;
            let _ = write!(encoded, "%{byte:02X}");
        }
    }
    encoded
}

fn receive_hongbao_query(
    channel_id: &str,
    in_way: u32,
    native_url: &str,
    send_id: &str,
    session_wxid: &str,
) -> String {
    format!(
        "agreeDuty=0&channelId={}&inWay={}&nativeUrl={}&sendId={}&sessionUserName={}&union_source=0",
        query_encode(channel_id),
        in_way,
        query_encode(native_url),
        query_encode(send_id),
        query_encode(session_wxid),
    )
}

fn open_hongbao_query(
    channel_id: &str,
    native_url: &str,
    nick_name: &str,
    session_wxid: &str,
    timing_identifier: &str,
) -> String {
    format!(
        "channelId={}&nativeUrl={}&nickName={}&sessionUserName={}&timingIdentifier={}&union_source=0",
        query_encode(channel_id),
        query_encode(native_url),
        query_encode(nick_name),
        query_encode(session_wxid),
        query_encode(timing_identifier),
    )
}

fn resolve_network_service(base: usize) -> Result<*mut c_void, String> {
    let context_address = base + crate::offsets::RVA_GLOBAL_APP_CONTEXT;
    let context = super::msg_hook::read_native_usize(context_address as *const u8)
        .ok_or_else(|| "WeChat app context pointer is unreadable".to_owned())?;
    if context < 0x10000 {
        return Err("WeChat app context is not initialized".to_owned());
    }
    let vtable = super::msg_hook::read_native_usize(context as *const u8)
        .ok_or_else(|| "WeChat app context vtable is unreadable".to_owned())?;
    if vtable < 0x10000 {
        return Err("WeChat app context vtable is not initialized".to_owned());
    }
    let getter_address = super::msg_hook::read_native_usize((vtable + 24) as *const u8)
        .ok_or_else(|| "network service getter is unreadable".to_owned())?;
    if getter_address < 0x10000 {
        return Err("network service getter is not initialized".to_owned());
    }
    let getter: GetNetworkServiceFn = unsafe { mem::transmute(getter_address) };
    let service = unsafe { getter(context as *mut c_void) };
    if service.is_null() || (service as usize) < 0x10000 {
        return Err("WeChat network service is unavailable".to_owned());
    }
    Ok(service)
}

fn do_hongbao_cgi_direct(
    cgi_id: i32,
    req_type: u32,
    cgi_path: &str,
    query: &str,
) -> Result<String, String> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return Err("微信主程序基址未初始化，暂无法处理红包".to_owned());
    }
    if super::account::get_self_account().is_none() {
        return Err("微信未登录或账号未就绪，无法处理红包".to_owned());
    }
    let network_service = resolve_network_service(base)?;

    let mut request = HongBaoReqGuard::new(base);
    let request_ptr = request.as_mut_ptr();
    let buffer = unsafe { libc::malloc(32) as *mut u8 };
    if buffer.is_null() {
        return Err("malloc SKBuiltinBuffer_t failed".to_owned());
    }
    unsafe {
        ptr::write_bytes(buffer, 0, 32);
        let init: SkBufferInitFn = mem::transmute(base + crate::offsets::RVA_SK_BUFFER_INIT);
        init(buffer);
        ptr::write_unaligned(request_ptr.add(16).cast::<u32>(), req_type);
        ptr::write_unaligned(request_ptr.add(20).cast::<u32>(), 1);
        ptr::write_unaligned(request_ptr.add(24).cast::<*mut c_void>(), buffer.cast());
        ptr::write_unaligned(request_ptr.add(36).cast::<u32>(), 0xE);
    }

    let query_string = unsafe { libc::malloc(mem::size_of::<StdString>()) as *mut StdString };
    if query_string.is_null() {
        return Err("malloc HongBao query StdString failed".to_owned());
    }
    unsafe {
        ptr::write(query_string, StdString::new(query));
        ptr::write_unaligned(buffer.add(8).cast::<*mut StdString>(), query_string);
        ptr::write_unaligned(buffer.add(16).cast::<u32>(), query.len() as u32);
        ptr::write_unaligned(buffer.add(24).cast::<u32>(), 3);
    }

    let mut cgi_url = StdString::new(cgi_path);
    let mut empty_string = StdString::new("");
    let options = [0u32; 2];
    let mut response = HongBaoResGuard::new(base);
    let send: CoCgiSendCommonRequestFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CO_CGI_SEND_COMMON_REQUEST) };
    let _result_ptr = unsafe {
        send(
            response.as_mut_ptr(),
            network_service,
            request_ptr.cast(),
            cgi_id,
            &cgi_url,
            &empty_string,
            &options,
        )
    };
    cgi_url.dispose();
    empty_string.dispose();

    let response_ptr = response.as_mut_ptr();
    let payload_buffer = unsafe { super::msg_hook::read_native_usize(response_ptr.add(16)) }
        .filter(|pointer| *pointer >= 0x10000)
        .ok_or_else(|| "HongBaoRes response payload buffer is missing".to_owned())?;
    let string_pointer =
        unsafe { super::msg_hook::read_native_usize((payload_buffer as *const u8).add(8)) }
            .filter(|pointer| *pointer >= 0x10000)
            .ok_or_else(|| "HongBaoRes JSON StdString is missing".to_owned())?;
    let json_string = unsafe { super::msg_hook::read_libcxx_string(string_pointer as *const u8) }
        .ok_or_else(|| "HongBaoRes JSON string is unreadable".to_owned())?;
    let json_text = String::from_utf8(json_string)
        .map_err(|error| format!("HongBaoRes JSON is not UTF-8: {error}"))?;
    serde_json::from_str::<serde_json::Value>(&json_text)
        .map_err(|error| format!("HongBaoRes did not contain valid JSON: {error}"))?;
    bridge_info!(
        "[HONGBAO] CGI {} completed; response length={}",
        cgi_id,
        json_text.len()
    );
    Ok(json_text)
}

fn do_receive_hongbao_direct(
    send_id: &str,
    channel_id: &str,
    native_url: &str,
    session_wxid: &str,
    in_way: u32,
) -> Result<String, String> {
    if send_id.trim().is_empty() || native_url.trim().is_empty() || session_wxid.trim().is_empty() {
        return Err("send_id, native_url, and session_wxid are required".to_owned());
    }
    if in_way > 1 {
        return Err("in_way must be 0 (group) or 1 (private)".to_owned());
    }
    let query = receive_hongbao_query(channel_id, in_way, native_url, send_id, session_wxid);
    do_hongbao_cgi_direct(1581, 3, "/cgi-bin/mmpay-bin/receivewxhb", &query)
}

fn do_open_hongbao_direct(
    channel_id: &str,
    native_url: &str,
    session_wxid: &str,
    timing_identifier: &str,
    nick_name: &str,
) -> Result<String, String> {
    if native_url.trim().is_empty()
        || session_wxid.trim().is_empty()
        || timing_identifier.trim().is_empty()
    {
        return Err("native_url, session_wxid, and timing_identifier are required".to_owned());
    }
    let query = open_hongbao_query(
        channel_id,
        native_url,
        nick_name,
        session_wxid,
        timing_identifier,
    );
    do_hongbao_cgi_direct(1685, 4, "/cgi-bin/mmpay-bin/openwxhb", &query)
}

enum HongBaoOperation {
    Receive {
        send_id: String,
        channel_id: String,
        native_url: String,
        session_wxid: String,
        in_way: u32,
    },
    Open {
        channel_id: String,
        native_url: String,
        session_wxid: String,
        timing_identifier: String,
        nick_name: String,
    },
}

struct StringCompletion {
    state: Mutex<SendState<String>>,
    condvar: Condvar,
}

impl StringCompletion {
    fn new() -> Self {
        Self {
            state: Mutex::new(SendState::Pending),
            condvar: Condvar::new(),
        }
    }

    fn begin(&self) -> bool {
        let mut state = self
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if !matches!(*state, SendState::Pending) {
            return false;
        }
        *state = SendState::Running;
        true
    }

    fn finish(&self, result: Result<String, String>) {
        *self
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner()) = SendState::Complete(result);
        self.condvar.notify_all();
    }

    fn wait(&self, timeout: Duration) -> Result<String, String> {
        let start = std::time::Instant::now();
        let mut state = self
            .state
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        loop {
            if let SendState::Complete(result) = &*state {
                return result.clone();
            }
            let remaining = timeout.saturating_sub(start.elapsed());
            if remaining.is_zero() {
                if matches!(*state, SendState::Pending) {
                    *state = SendState::Cancelled;
                    return Err(
                        "HongBao request timed out before main-thread execution; request cancelled"
                            .to_owned(),
                    );
                }
                return Err("HongBao request timed out during native execution; outcome unknown, do not retry automatically".to_owned());
            }
            let (next, _) = self
                .condvar
                .wait_timeout(state, remaining)
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            state = next;
        }
    }
}

struct MainThreadHongBaoTask {
    operation: HongBaoOperation,
    completion: StringCompletion,
}

fn run_hongbao_operation(operation: &HongBaoOperation) -> Result<String, String> {
    match operation {
        HongBaoOperation::Receive {
            send_id,
            channel_id,
            native_url,
            session_wxid,
            in_way,
        } => do_receive_hongbao_direct(send_id, channel_id, native_url, session_wxid, *in_way),
        HongBaoOperation::Open {
            channel_id,
            native_url,
            session_wxid,
            timing_identifier,
            nick_name,
        } => do_open_hongbao_direct(
            channel_id,
            native_url,
            session_wxid,
            timing_identifier,
            nick_name,
        ),
    }
}

unsafe extern "C" fn main_thread_hongbao_dispatcher(data: *mut c_void) -> libc::c_int {
    let task = Arc::from_raw(data as *const MainThreadHongBaoTask);
    if task.completion.begin() {
        let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            run_hongbao_operation(&task.operation)
        }))
        .unwrap_or_else(|_| Err("main-thread HongBao dispatcher failed".to_owned()));
        task.completion.finish(outcome);
    }
    0
}

fn dispatch_hongbao(operation: HongBaoOperation) -> Result<String, String> {
    if is_main_thread() {
        return run_hongbao_operation(&operation);
    }
    let idle_add = get_g_idle_add().ok_or_else(|| "g_idle_add 未在进程中找到".to_owned())?;
    let task = Arc::new(MainThreadHongBaoTask {
        operation,
        completion: StringCompletion::new(),
    });
    let raw_ptr = Arc::into_raw(task.clone()) as *mut c_void;
    if unsafe { idle_add(main_thread_hongbao_dispatcher, raw_ptr) } == 0 {
        unsafe { drop(Arc::from_raw(raw_ptr as *const MainThreadHongBaoTask)) };
        return Err("g_idle_add failed to schedule HongBao request".to_owned());
    }
    task.completion.wait(Duration::from_secs(15))
}

pub fn receive_hongbao(
    send_id: &str,
    channel_id: &str,
    native_url: &str,
    session_wxid: &str,
    in_way: u32,
) -> Result<String, String> {
    dispatch_hongbao(HongBaoOperation::Receive {
        send_id: send_id.to_owned(),
        channel_id: channel_id.to_owned(),
        native_url: native_url.to_owned(),
        session_wxid: session_wxid.to_owned(),
        in_way,
    })
}

pub fn open_hongbao(
    channel_id: &str,
    native_url: &str,
    session_wxid: &str,
    timing_identifier: &str,
    nick_name: &str,
) -> Result<String, String> {
    dispatch_hongbao(HongBaoOperation::Open {
        channel_id: channel_id.to_owned(),
        native_url: native_url.to_owned(),
        session_wxid: session_wxid.to_owned(),
        timing_identifier: timing_identifier.to_owned(),
        nick_name: nick_name.to_owned(),
    })
}

// ==================== 转账确认收款 ====================

type AcceptPayTransferFn = unsafe extern "C" fn(
    result_buf: *mut u8,
    unused: usize,
    shared_ptr_transfer_info: *const [usize; 2],
    group_username: *const StdString,
) -> *mut u8;

struct MainThreadConfirmTransferTask {
    trans_id: String,
    invalid_time: u32,
    transfer_attach: String,
    completion: SendCompletion,
}

fn do_confirm_transfer_direct(
    trans_id: &str,
    invalid_time: u32,
    transfer_attach: &str,
) -> Result<(), String> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return Err("微信主程序基址未初始化，暂无法确认转账".to_owned());
    }
    if super::account::get_self_account().is_none() {
        return Err("微信未登录或账号未就绪，无法确认转账".to_owned());
    }

    const TRANSFER_INFO_SIZE: usize = 440;
    const TRANSFER_ATTACH_OFFSET: usize = 216;
    const INVALID_TIME_OFFSET: usize = 264;
    const TRANS_ID_OFFSET: usize = 408;

    // usize storage provides the 8-byte alignment required by libc++ std::string.
    let mut transfer_info = Box::new([0usize; TRANSFER_INFO_SIZE / mem::size_of::<usize>()]);
    let info_ptr = transfer_info.as_mut_ptr() as *mut u8;
    let mut result_buf = [0u8; 64];
    let shared_ptr = [info_ptr as usize, 0usize];
    let accept_fn: AcceptPayTransferFn =
        unsafe { mem::transmute(base + crate::offsets::RVA_CO_ACCEPT_PAY_TRANSFER) };

    unsafe {
        ptr::write(
            info_ptr.add(TRANSFER_ATTACH_OFFSET) as *mut StdString,
            StdString::new(transfer_attach),
        );
        ptr::write(info_ptr.add(INVALID_TIME_OFFSET) as *mut u32, invalid_time);
        ptr::write(
            info_ptr.add(TRANS_ID_OFFSET) as *mut StdString,
            StdString::new(trans_id),
        );
    }

    bridge_info!(
        "[CONFIRM-TRANSFER] Calling native transfer acceptance: trans_id={}, invalid_time={}",
        trans_id,
        invalid_time
    );

    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        accept_fn(result_buf.as_mut_ptr(), 0, &shared_ptr, ptr::null())
    }));

    unsafe {
        (&mut *(info_ptr.add(TRANSFER_ATTACH_OFFSET) as *mut StdString)).dispose();
        (&mut *(info_ptr.add(TRANS_ID_OFFSET) as *mut StdString)).dispose();
    }
    // `transfer_info` owns and releases the zeroed 440-byte TransferInfo storage.
    drop(transfer_info);

    match result {
        Ok(result_ptr) => {
            bridge_info!(
                "[CONFIRM-TRANSFER] Native acceptance call completed: trans_id={}, result_ptr=0x{:x}",
                trans_id,
                result_ptr as usize
            );
            Ok(())
        }
        Err(_) => {
            bridge_error!("[CONFIRM-TRANSFER] Native transfer acceptance raised an exception");
            Err("confirm_transfer failed with exception in main thread".to_owned())
        }
    }
}

unsafe extern "C" fn main_thread_confirm_transfer_dispatcher(data: *mut c_void) -> libc::c_int {
    let task = Arc::from_raw(data as *const MainThreadConfirmTransferTask);
    if task.completion.begin() {
        let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            do_confirm_transfer_direct(&task.trans_id, task.invalid_time, &task.transfer_attach)
        }))
        .unwrap_or_else(|_| Err("main-thread transfer confirmation dispatcher failed".to_owned()));
        task.completion.finish(outcome);
    }
    0 // G_SOURCE_REMOVE
}

pub fn confirm_transfer(
    trans_id: &str,
    invalid_time: u32,
    transfer_attach: &str,
) -> Result<(), String> {
    if trans_id.trim().is_empty() {
        return Err("trans_id is empty".to_owned());
    }
    if is_main_thread() {
        return do_confirm_transfer_direct(trans_id, invalid_time, transfer_attach);
    }

    let idle_add = get_g_idle_add().ok_or_else(|| "g_idle_add 未在进程中找到".to_owned())?;
    let task = Arc::new(MainThreadConfirmTransferTask {
        trans_id: trans_id.to_owned(),
        invalid_time,
        transfer_attach: transfer_attach.to_owned(),
        completion: SendCompletion::new(),
    });
    let raw_ptr = Arc::into_raw(task.clone()) as *mut c_void;
    if unsafe { idle_add(main_thread_confirm_transfer_dispatcher, raw_ptr) } == 0 {
        unsafe {
            drop(Arc::from_raw(
                raw_ptr as *const MainThreadConfirmTransferTask,
            ));
        }
        return Err("g_idle_add failed to schedule transfer confirmation".to_owned());
    }
    task.completion.wait(Duration::from_secs(5))
}

// ==================== 富媒体与通用消息发送 ====================

pub const RVA_SEND_MESSAGE: usize = 0x6f7d010;

#[repr(C)]
pub struct SendMessageParams {
    pub chat_id: StdString,     // offset 0
    pub message_type: i32,      // offset 24
    pub _pad1: [u8; 4],         // offset 28
    pub content_xml: StdString, // offset 32
    pub message_subtype: i32,   // offset 56
    pub _pad2: [u8; 4],         // offset 60
    pub is_base64: bool,        // offset 64
    pub _pad3: [u8; 7],         // offset 65
    pub valid: bool,            // offset 72
    pub _pad4: [u8; 7],         // offset 73
}

type SendMessageFn = unsafe extern "C" fn(
    a1: usize,
    params: *const SendMessageParams,
    method: *const StdString,
    callback: *mut *mut CallbackWrapper,
);

fn do_send_rich_direct(
    target: &str,
    msg_type: i32,
    content_xml: &str,
    is_base64: bool,
    subtype: i32,
) -> Result<(), String> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return Err("微信主程序基址未初始化，暂无法发送消息".to_owned());
    }
    if super::account::get_self_account().is_none() {
        return Err("微信未登录或账号未就绪，禁止发送消息".to_owned());
    }

    let send_fn_addr = base + RVA_SEND_MESSAGE;
    let send_fn: SendMessageFn = unsafe { mem::transmute(send_fn_addr) };

    let mut chat_id_str = StdString::new(target);
    let mut content_str = StdString::new(content_xml);
    let mut empty_method = StdString::new("");

    let params = SendMessageParams {
        chat_id: chat_id_str,
        message_type: msg_type,
        _pad1: [0; 4],
        content_xml: content_str,
        message_subtype: subtype,
        _pad2: [0; 4],
        is_base64,
        _pad3: [0; 7],
        valid: true,
        _pad4: [0; 7],
    };

    let cb_heap = unsafe {
        let p = libc::malloc(mem::size_of::<CallbackWrapper>()) as *mut CallbackWrapper;
        if p.is_null() {
            chat_id_str.dispose();
            content_str.dispose();
            empty_method.dispose();
            return Err("malloc CallbackWrapper failed".to_owned());
        }
        (*p).refcount = std::sync::atomic::AtomicI32::new(1);
        (*p).invoke_fn = cb_invoke;
        (*p).destroy_fn = cb_destroy;
        p
    };

    let mut cb_holder = cb_heap;

    bridge_info!(
        "[SEND-RICH] 主线程开始发送富媒体消息 -> 目标: {}, 类型: {}, XML/内容长度: {} 字节",
        target,
        msg_type,
        content_xml.len()
    );

    let res = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        send_fn(0, &params, &empty_method, &mut cb_holder);
    }));

    chat_id_str.dispose();
    content_str.dispose();
    empty_method.dispose();

    match res {
        Ok(()) => {
            bridge_info!("[SEND-RICH] 主线程发送富媒体调用成功");
            Ok(())
        }
        Err(_) => {
            bridge_error!("[SEND-RICH] 主线程发送富媒体调用发生异常");
            Err("send_rich_message failed with exception in main thread".to_owned())
        }
    }
}

struct MainThreadRichSendTask {
    target: String,
    msg_type: i32,
    content_xml: String,
    is_base64: bool,
    subtype: i32,
    completion: SendCompletion,
}

unsafe extern "C" fn main_thread_rich_send_dispatcher(data: *mut c_void) -> libc::c_int {
    let task = Arc::from_raw(data as *const MainThreadRichSendTask);
    if task.completion.begin() {
        let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            do_send_rich_direct(
                &task.target,
                task.msg_type,
                &task.content_xml,
                task.is_base64,
                task.subtype,
            )
        }))
        .unwrap_or_else(|_| Err("main-thread rich-message dispatcher failed".to_owned()));
        task.completion.finish(outcome);
    }
    0 // G_SOURCE_REMOVE
}

pub fn send_rich_message(
    target: &str,
    msg_type: i32,
    content_xml: &str,
    is_base64: bool,
    subtype: i32,
) -> Result<(), String> {
    if is_main_thread() {
        return do_send_rich_direct(target, msg_type, content_xml, is_base64, subtype);
    }

    let idle_add = get_g_idle_add().ok_or_else(|| "g_idle_add 未在进程中找到".to_owned())?;

    let task = Arc::new(MainThreadRichSendTask {
        target: target.to_owned(),
        msg_type,
        content_xml: content_xml.to_owned(),
        is_base64,
        subtype,
        completion: SendCompletion::new(),
    });

    let raw_ptr = Arc::into_raw(task.clone()) as *mut c_void;
    if unsafe { idle_add(main_thread_rich_send_dispatcher, raw_ptr) } == 0 {
        unsafe {
            drop(Arc::from_raw(raw_ptr as *const MainThreadRichSendTask));
        }
        return Err("g_idle_add failed to schedule rich-message send".to_owned());
    }
    task.completion.wait(Duration::from_secs(5))
}

pub fn send_image(target: &str, img_url_or_xml: &str) -> Result<(), String> {
    send_rich_message(target, 3, img_url_or_xml, false, 0)
}

pub fn send_xml(target: &str, xml: &str) -> Result<(), String> {
    send_rich_message(target, 49, xml, false, 0)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn unique_image_test_path(extension: &str) -> std::path::PathBuf {
        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir().join(format!(
            "wx_bridge_decrypt_{}_{}.{}",
            std::process::id(),
            timestamp,
            extension
        ))
    }

    #[test]
    fn decrypt_image_file_detects_xor_formats_and_writes_default_path() {
        for (magic, extension) in [
            (&[0xFF, 0xD8, 0x11][..], "jpg"),
            (&[0x89, 0x50, 0x11][..], "png"),
            (&[0x47, 0x49, 0x11][..], "gif"),
            (&[0x42, 0x4D, 0x11][..], "bmp"),
        ] {
            let source = unique_image_test_path("dat");
            let key = 0x5A;
            let encrypted: Vec<u8> = magic.iter().map(|byte| byte ^ key).collect();
            std::fs::write(&source, encrypted).unwrap();

            let result = decrypt_image_file(source.to_str().unwrap(), None).unwrap();
            let destination = result["dest_path"].as_str().unwrap();
            assert!(destination.ends_with(&format!(".{extension}")));
            assert_eq!(result["extension"], extension);
            assert_eq!(result["xor_key"], key);
            assert_eq!(result["size"], magic.len());
            assert_eq!(std::fs::read(destination).unwrap(), magic);

            std::fs::remove_file(source).unwrap();
            std::fs::remove_file(destination).unwrap();
        }
    }

    #[test]
    fn decrypt_image_file_accepts_plain_images_and_explicit_destinations() {
        let source = unique_image_test_path("dat");
        let destination = unique_image_test_path("png");
        let plain = [0x89, 0x50, 0x4E, 0x47];
        std::fs::write(&source, plain).unwrap();

        let result = decrypt_image_file(
            source.to_str().unwrap(),
            Some(destination.to_str().unwrap()),
        )
        .unwrap();
        assert_eq!(result["dest_path"], destination.to_str().unwrap());
        assert_eq!(result["xor_key"], 0);
        assert_eq!(std::fs::read(&destination).unwrap(), plain);

        std::fs::remove_file(source).unwrap();
        std::fs::remove_file(destination).unwrap();
    }

    #[test]
    fn decrypt_image_file_rejects_short_and_unknown_data() {
        let source = unique_image_test_path("dat");
        std::fs::write(&source, [0x00]).unwrap();
        assert_eq!(
            decrypt_image_file(source.to_str().unwrap(), None).unwrap_err(),
            "File too small to be an encrypted image"
        );

        std::fs::write(&source, [0x00, 0x01]).unwrap();
        assert_eq!(
            decrypt_image_file(source.to_str().unwrap(), None).unwrap_err(),
            "Unknown image encoding or not an encrypted .dat file"
        );
        std::fs::remove_file(source).unwrap();
    }

    #[test]
    fn set_chat_read_rejects_an_empty_target_before_native_dispatch() {
        assert_eq!(set_chat_read("  ").unwrap_err(), "target is empty");
    }

    #[test]
    fn std_string_and_native_vector_match_libcpp_layout() {
        assert_eq!(mem::size_of::<StdString>(), 24);
        assert_eq!(mem::size_of::<EditRemarkObj>(), 64);
        let mut short = StdString::new("wxid_short");
        assert_eq!(short.as_str(), Some("wxid_short"));
        short.dispose();

        let long_text = "wxid_".to_owned() + &"x".repeat(32);
        let mut long = StdString::new(&long_text);
        assert_eq!(long.as_str(), Some(long_text.as_str()));
        long.dispose();

        let vector = NativeStdVector::from_strings(&["wxid_a", "wxid_b"]);
        let descriptor = unsafe { std::slice::from_raw_parts(vector.as_ptr(), 3) };
        assert_eq!(descriptor[0], vector.start as usize);
        assert_eq!(descriptor[1], vector.end as usize);
        assert_eq!(descriptor[2], vector.cap as usize);
        assert_eq!(unsafe { vector.end.offset_from(vector.start) }, 2);
    }

    #[test]
    fn database_key_hex_reads_short_and_long_libcpp_strings() {
        let mut short = StdString::new("abcd");
        assert_eq!(database_key_hex(&short).unwrap(), "61626364");
        short.dispose();

        let long_value = "k".repeat(32);
        let mut long = StdString::new(&long_value);
        assert_eq!(database_key_hex(&long).unwrap(), "6b".repeat(32));
        long.dispose();
    }

    #[test]
    fn hongbao_queries_preserve_parameter_order_and_encode_nested_urls() {
        let receive = receive_hongbao_query(
            "1",
            0,
            "wxpay://hongbao?a=1&b=中",
            "send-1",
            "room@chatroom",
        );
        assert_eq!(
            receive,
            "agreeDuty=0&channelId=1&inWay=0&nativeUrl=wxpay%3A%2F%2Fhongbao%3Fa%3D1%26b%3D%E4%B8%AD&sendId=send-1&sessionUserName=room%40chatroom&union_source=0"
        );
        let open = open_hongbao_query("1", "wxpay://hongbao", "小明", "wxid_1", "token+1");
        assert_eq!(
            open,
            "channelId=1&nativeUrl=wxpay%3A%2F%2Fhongbao&nickName=%E5%B0%8F%E6%98%8E&sessionUserName=wxid_1&timingIdentifier=token%2B1&union_source=0"
        );
    }

    #[test]
    fn completion_delivers_result_without_nested_locks() {
        let completion = Arc::new(SendCompletion::new());
        let worker = completion.clone();
        let handle = std::thread::spawn(move || {
            assert!(worker.begin());
            worker.finish(Ok(()));
        });
        assert_eq!(completion.wait(Duration::from_secs(1)), Ok(()));
        assert!(handle.join().is_ok());
    }

    #[test]
    fn queued_send_is_cancelled_on_timeout() {
        let completion = SendCompletion::new();
        let error = completion.wait(Duration::ZERO).unwrap_err();
        assert!(error.contains("cancelled"));
        assert!(!completion.begin());
    }

    #[test]
    fn running_send_reports_unknown_outcome_and_can_finish_later() {
        let completion = SendCompletion::new();
        assert!(completion.begin());
        assert!(completion
            .wait(Duration::ZERO)
            .unwrap_err()
            .contains("outcome unknown"));
        completion.finish(Ok(()));
        assert_eq!(completion.wait(Duration::ZERO), Ok(()));
    }
}
