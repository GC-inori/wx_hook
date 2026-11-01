use libc::{c_int, c_void};
use std::mem;
use std::ptr;
use std::sync::atomic::{AtomicPtr, AtomicUsize, Ordering};
use std::thread;
use std::time::Duration;

use super::ipc;
use super::logger::{bridge_error, bridge_info};

const RVA_ADD_MSG_VTABLE: usize = 0x0a9b4e18;
const VTABLE_SLOT_MERGE: usize = 8;
const EXPECTED_MERGE_RVA: usize = 0x7df4f40;

type MergePartialFn = unsafe extern "C" fn(*mut c_void, *mut c_void) -> bool;
static ORIGINAL_ADD_MSG_MERGE: AtomicPtr<c_void> = AtomicPtr::new(ptr::null_mut());
pub static WECHAT_BASE: AtomicUsize = AtomicUsize::new(0);

// Re-export sender functions for backward compatibility / external callers
pub use super::sender::{send_text, send_text_with_at};

pub use super::events::ReceivedMessageEvent;

/// Copies host memory through the kernel, which reports EFAULT for unreadable
/// ranges instead of dereferencing a potentially stale pointer in the hook.
fn read_memory(address: *const u8, output: &mut [u8]) -> bool {
    if address.is_null() || (address as usize) < 0x10000 {
        return false;
    }
    let local = libc::iovec {
        iov_base: output.as_mut_ptr().cast(),
        iov_len: output.len(),
    };
    let remote = libc::iovec {
        iov_base: address as *mut c_void,
        iov_len: output.len(),
    };
    unsafe {
        libc::process_vm_readv(libc::getpid(), &local, 1, &remote, 1, 0) == output.len() as isize
    }
}

pub unsafe fn is_readable_ptr(address: *const u8) -> bool {
    read_memory(address, &mut [0u8; 1])
}

pub(crate) fn read_native_usize(address: *const u8) -> Option<usize> {
    let mut bytes = [0u8; mem::size_of::<usize>()];
    read_memory(address, &mut bytes).then(|| usize::from_ne_bytes(bytes))
}

pub unsafe fn read_libcxx_string(address: *const u8) -> Option<Vec<u8>> {
    let mut header = [0u8; 24];
    if !read_memory(address, &mut header) {
        return None;
    }
    if header[0] & 1 == 0 {
        let size = (header[0] >> 1) as usize;
        if size > 22 {
            return None;
        }
        return Some(header[1..1 + size].to_vec());
    }
    let size = usize::from_ne_bytes(header[8..16].try_into().ok()?);
    let pointer = usize::from_ne_bytes(header[16..24].try_into().ok()?) as *const u8;
    if size > 1024 * 1024 {
        return None;
    }
    if size == 0 {
        return Some(Vec::new());
    }
    let mut bytes = vec![0u8; size];
    read_memory(pointer, &mut bytes).then_some(bytes)
}

unsafe fn read_builtin_string(this: *const u8, offset: usize) -> String {
    let Some(wrapper) = read_native_usize(this.wrapping_add(offset)) else {
        return String::new();
    };
    if wrapper < 0x10000 {
        return String::new();
    }
    let Some(pointer) = read_native_usize((wrapper as *const u8).wrapping_add(8)) else {
        return String::new();
    };
    read_libcxx_string(pointer as *const u8)
        .map(|bytes| String::from_utf8_lossy(&bytes).into_owned())
        .unwrap_or_default()
}

unsafe fn read_direct_string(this: *const u8, offset: usize) -> String {
    read_native_usize(this.wrapping_add(offset))
        .and_then(|pointer| read_libcxx_string(pointer as *const u8))
        .map(|bytes| String::from_utf8_lossy(&bytes).into_owned())
        .unwrap_or_default()
}

unsafe fn parse_and_broadcast_add_msg(this: *const u8) {
    let mut header = [0u8; 0x70];
    if !read_memory(this, &mut header) {
        return;
    }
    let msg_type = ptr::read_unaligned(header.as_ptr().add(0x14).cast::<u32>());
    let msg_id = ptr::read_unaligned(header.as_ptr().add(0x10).cast::<u32>());
    let new_msg_id = ptr::read_unaligned(header.as_ptr().add(0x68).cast::<i64>());
    let sequence = u64::try_from(new_msg_id)
        .ok()
        .filter(|id| *id != 0)
        .unwrap_or(u64::from(msg_id));

    let sender = read_builtin_string(this, 0x08);
    let receiver = read_builtin_string(this, 0x18);
    let text = read_builtin_string(this, 0x20);
    let msg_source = read_direct_string(this, 0x38);

    bridge_info!(
        "[RECV-MSG] type={}, id={}, new_id={}, from={}, to={}, text_len={}, source_len={}",
        msg_type,
        msg_id,
        new_msg_id,
        sender,
        receiver,
        text.len(),
        msg_source.len()
    );

    let event = ReceivedMessageEvent::new(msg_type, sequence, sender, receiver, text, msg_source);
    let (name, data) = super::events::classify_message(&event);
    ipc::broadcast_event(name, &data);
}

unsafe extern "C" fn hook_add_msg_merge(this: *mut c_void, input: *mut c_void) -> bool {
    let orig = ORIGINAL_ADD_MSG_MERGE.load(Ordering::Acquire);
    if orig.is_null() {
        return false;
    }
    let orig_fn: MergePartialFn = mem::transmute(orig);
    let success = orig_fn(this, input);
    if success && !this.is_null() {
        let _ = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            parse_and_broadcast_add_msg(this as *const u8);
        }));
    }
    success
}

// ==================== 内存补丁与 Hook 挂载 ====================

pub unsafe fn patch_memory(addr: usize, data: &[u8]) -> bool {
    let page_size = 4096;
    let page_start = addr & !(page_size - 1);
    let page_end = (addr + data.len() + page_size - 1) & !(page_size - 1);
    let len = page_end - page_start;

    let ret = libc::mprotect(
        page_start as *mut c_void,
        len,
        libc::PROT_READ | libc::PROT_WRITE | libc::PROT_EXEC,
    );
    if ret != 0 {
        bridge_error!("mprotect PROT_READ|WRITE|EXEC failed at 0x{:x}", addr);
        return false;
    }

    ptr::copy_nonoverlapping(data.as_ptr(), addr as *mut u8, data.len());

    let _ = libc::mprotect(
        page_start as *mut c_void,
        len,
        libc::PROT_READ | libc::PROT_EXEC,
    );
    true
}

fn is_wechat_main_process() -> bool {
    let Ok(exe) = std::fs::read_link("/proc/self/exe") else {
        return false;
    };
    let s = exe.to_string_lossy();
    (s.ends_with("/wechat") || s == "wechat") && !s.contains("wxplayer")
}

fn get_wechat_base() -> Option<usize> {
    if !is_wechat_main_process() {
        return None;
    }
    unsafe extern "C" fn callback(
        info: *mut libc::dl_phdr_info,
        _size: libc::size_t,
        data: *mut c_void,
    ) -> c_int {
        let name_ptr = (*info).dlpi_name;
        if !name_ptr.is_null() {
            let name = std::ffi::CStr::from_ptr(name_ptr).to_string_lossy();
            if name.is_empty() || name.ends_with("/wechat") || name == "wechat" {
                let base_ptr = data as *mut usize;
                *base_ptr = (*info).dlpi_addr as usize;
                return 1;
            }
        }
        0
    }
    let mut base: usize = 0;
    unsafe {
        libc::dl_iterate_phdr(Some(callback), &mut base as *mut usize as *mut c_void);
    }
    if base != 0 {
        Some(base)
    } else {
        None
    }
}

unsafe fn try_install_hook(base: usize) -> bool {
    let vtable_addr = base + RVA_ADD_MSG_VTABLE;
    let slot_ptr = (vtable_addr + VTABLE_SLOT_MERGE * mem::size_of::<usize>()) as *mut usize;

    if !is_readable_ptr(slot_ptr as *const u8) {
        return false;
    }

    let current_fn = *slot_ptr;
    let current_rva = current_fn.wrapping_sub(base);

    if current_rva != EXPECTED_MERGE_RVA {
        if current_fn == hook_add_msg_merge as *const () as usize {
            return true;
        }
        return false;
    }

    let page_size = 4096;
    let page_start = (slot_ptr as usize) & !(page_size - 1);
    let ret = libc::mprotect(
        page_start as *mut c_void,
        page_size,
        libc::PROT_READ | libc::PROT_WRITE,
    );
    if ret != 0 {
        bridge_error!(
            "mprotect PROT_READ|PROT_WRITE failed: {}",
            std::io::Error::last_os_error()
        );
        return false;
    }

    ORIGINAL_ADD_MSG_MERGE.store(current_fn as *mut c_void, Ordering::Release);
    *slot_ptr = hook_add_msg_merge as *const () as usize;

    // Notice: Keep page readable and writable since this page is part of the host .data/.data.rel.ro segment
    let _ = libc::mprotect(
        page_start as *mut c_void,
        page_size,
        libc::PROT_READ | libc::PROT_WRITE,
    );

    bridge_info!(
        "Hooked AddMsg::MergePartialFromCodedStream: orig=0x{:x} (RVA 0x{:x}) -> new=0x{:x}",
        current_fn,
        current_rva,
        hook_add_msg_merge as *const () as usize
    );

    // 安装发送消息 atuserlist 注入 Hook
    super::sender::install_sender_hooks(base);

    // 安装协程安全上下文双向跳板
    install_coroutine_scope_trampolines(base);

    true
}

const RVA_CO_SCOPE_ENTER_SITE: usize = 0x9ae35af;
const RVA_CO_SCOPE_EXIT_SITE: usize = 0x9ae35d4;
const RVA_CO_SCOPE_EXIT_RESUME: usize = 0x9ae35e7;

unsafe fn install_coroutine_scope_trampolines(base: usize) -> bool {
    let enter_site = base + RVA_CO_SCOPE_ENTER_SITE;
    let exit_site = base + RVA_CO_SCOPE_EXIT_SITE;
    let exit_resume = base + RVA_CO_SCOPE_EXIT_RESUME;

    let trampoline_page = match super::sender::allocate_trampoline_near(enter_site) {
        Some(ptr) => ptr,
        None => {
            bridge_error!("[CO-SCOPE] 无法分配近端跳板页面");
            return false;
        }
    };

    // Trampoline 1: Safe sub_9AE3540 handler at offset 0
    // If rax == 0, skip accessing [rax + 0x188] and [rax + 0x2c8], jump to epilogue
    // If rax != 0, check and increment coroutine counter normally
    let tramp1 = trampoline_page;
    let code1 = [
        0x48, 0x8b, 0x03, // mov rax, [rbx]
        0x48, 0x85, 0xc0, // test rax, rax
        0x74, 0x0f, // jz +15 (.done at offset 23)
        0x83, 0xb8, 0x88, 0x01, 0x00, 0x00, 0x03, // cmp dword ptr [rax+188h], 3
        0x74, 0x06, // jz +6  (.done at offset 23)
        0xff, 0x80, 0xc8, 0x02, 0x00, 0x00, // inc dword ptr [rax+2C8h]
        // .done:
        0x48, 0x83, 0xc4, 0x08, // add rsp, 8
        0x5b, // pop rbx
        0x41, 0x5e, // pop r14
        0xc3, // ret
    ];
    ptr::copy_nonoverlapping(code1.as_ptr(), tramp1, code1.len());

    // Patch enter_site (18 bytes from 0x9ae35af to 0x9ae35c1): E9 rel32 + NOPs
    let rel32_enter = (tramp1 as i64 - (enter_site as i64 + 5)) as i32;
    let mut patch_enter = [0x90u8; 18];
    patch_enter[0] = 0xe9;
    patch_enter[1..5].copy_from_slice(&rel32_enter.to_ne_bytes());
    if !patch_memory(enter_site, &patch_enter) {
        bridge_error!("[CO-SCOPE] 写入 enter_site 补丁失败");
        return false;
    }

    // Trampoline 2: Safe sub_9AE35D0 handler at offset 64
    // If rax == 0, skip accessing [rax + 0x188] and [rax + 0x2c8]
    // If rax != 0, check and decrement coroutine counter normally
    let tramp2 = trampoline_page.add(64);
    let rel32_back = (exit_resume as i64 - (tramp2 as i64 + 29)) as i32;
    let mut code2 = [
        0x48, 0x85, 0xc0, // test rax, rax
        0x74, 0x0f, // jz +15 (.done at offset 20)
        0x83, 0xb8, 0x88, 0x01, 0x00, 0x00, 0x03, // cmp dword ptr [rax+188h], 3
        0x74, 0x06, // jz +6  (.done at offset 20)
        0xff, 0x88, 0xc8, 0x02, 0x00, 0x00, // dec dword ptr [rax+2C8h]
        // .done:
        0x48, 0x8b, 0x5f, 0x08, // mov rbx, [rdi+8]
        0xe9, 0x00, 0x00, 0x00, 0x00, // jmp rel32 (to exit_resume)
    ];
    code2[25..29].copy_from_slice(&rel32_back.to_ne_bytes());
    ptr::copy_nonoverlapping(code2.as_ptr(), tramp2, code2.len());

    // Patch exit_site (15 bytes from 0x9ae35d4 to 0x9ae35e3): E9 rel32 + NOPs
    let rel32_exit = (tramp2 as i64 - (exit_site as i64 + 5)) as i32;
    let mut patch_exit = [0x90u8; 15];
    patch_exit[0] = 0xe9;
    patch_exit[1..5].copy_from_slice(&rel32_exit.to_ne_bytes());
    if !patch_memory(exit_site, &patch_exit) {
        bridge_error!("[CO-SCOPE] 写入 exit_site 补丁失败");
        return false;
    }

    bridge_info!("[CO-SCOPE] 成功安装协程上下文保护双向安全跳板 (enter & exit)");
    true
}

pub fn start_hook() {
    if let Err(error) = thread::Builder::new()
        .name("wx-native-hook".to_owned())
        .spawn(|| {
            for _ in 0..50 {
                if let Some(base) = get_wechat_base() {
                    WECHAT_BASE.store(base, Ordering::Release);
                    if unsafe { try_install_hook(base) } {
                        bridge_info!("WeChat hooks installed successfully at base 0x{:x}", base);
                        return;
                    }
                }
                thread::sleep(Duration::from_millis(200));
            }
            bridge_error!("Failed to install WeChat hooks within timeout");
        })
    {
        bridge_error!("cannot start native hook installer: {error}");
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rejects_unreadable_host_memory_without_dereferencing_it() {
        unsafe {
            assert!(!is_readable_ptr(ptr::null()));
            assert!(!is_readable_ptr(0x10000 as *const u8));
            assert!(read_libcxx_string(0x10000 as *const u8).is_none());
            parse_and_broadcast_add_msg(0x10000 as *const u8);
        }
    }

    #[test]
    fn copies_short_and_long_native_strings_into_owned_buffers() {
        for text in [
            "short",
            "a native string longer than the short-string inline capacity",
        ] {
            let mut native = super::super::sender::StdString::new(text);
            let bytes = unsafe {
                read_libcxx_string((&native as *const super::super::sender::StdString).cast())
            }
            .unwrap();
            native.dispose();
            assert_eq!(bytes, text.as_bytes());
        }
    }
}
