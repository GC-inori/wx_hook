use libc::c_void;
use std::mem;
use std::ptr;
use std::sync::atomic::{AtomicPtr, Ordering};
use std::sync::{Arc, Condvar, Mutex, OnceLock};
use std::time::Duration;

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

enum SendState {
    Pending,
    Running,
    Complete(Result<(), String>),
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

// ==================== 对外发送接口 ====================

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
