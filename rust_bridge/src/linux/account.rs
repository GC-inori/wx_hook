use libc::c_void;
use std::collections::HashMap;
use std::mem;
use std::sync::atomic::Ordering;
use std::sync::Mutex;

use super::logger::bridge_info;
use super::msg_hook::{is_readable_ptr, read_libcxx_string, read_native_usize, WECHAT_BASE};
use super::sender::StdString;

pub const RVA_ACCOUNT_MANAGER_PTR: usize = 0xacf6710;
pub const RVA_GET_CURRENT_WXID: usize = 0x60464f0;
pub const RVA_GET_CURRENT_NICK: usize = 0x6046ad0;

static CONTACT_CACHE: Mutex<Option<HashMap<String, String>>> = Mutex::new(None);

pub fn update_contact_cache(wxid: &str, name: &str) {
    if wxid.is_empty() || name.is_empty() {
        return;
    }
    let mut lock = CONTACT_CACHE
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner());
    let map = lock.get_or_insert_with(HashMap::new);
    map.insert(wxid.to_owned(), name.to_owned());
}

pub fn get_contact_name(wxid: &str) -> Option<String> {
    if wxid.is_empty() {
        return None;
    }
    // 1. Check self account
    if let Some((self_wxid, self_nick)) = get_self_account() {
        if wxid == self_wxid {
            return Some(self_nick);
        }
    }
    // 2. Check dynamic contact cache
    let lock = CONTACT_CACHE
        .lock()
        .unwrap_or_else(|poisoned| poisoned.into_inner());
    if let Some(ref map) = *lock {
        if let Some(name) = map.get(wxid) {
            return Some(name.clone());
        }
    }
    None
}

/// Safely inspects current logged-in WeChat account (wxid and nickname) from native memory.
pub fn get_self_account() -> Option<(String, String)> {
    let base = WECHAT_BASE.load(Ordering::Acquire);
    if base == 0 {
        return None;
    }

    // 1. Verify global AccountManager pointer is allocated and valid
    let mgr_ptr_addr = base + RVA_ACCOUNT_MANAGER_PTR;
    if !unsafe { is_readable_ptr(mgr_ptr_addr as *const u8) } {
        return None;
    }
    let mgr_ptr = read_native_usize(mgr_ptr_addr as *const u8)? as *const c_void;
    if mgr_ptr.is_null() || !unsafe { is_readable_ptr(mgr_ptr as *const u8) } {
        return None;
    }

    // 2. Call sub_60464F0 to retrieve current user wxid
    let get_wxid_fn: unsafe extern "C" fn() -> *const u8 =
        unsafe { mem::transmute(base + RVA_GET_CURRENT_WXID) };

    let wxid = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        let p = get_wxid_fn();
        if !p.is_null() && is_readable_ptr(p) {
            read_libcxx_string(p).map(|bytes| String::from_utf8_lossy(&bytes).into_owned())
        } else {
            None
        }
    }))
    .ok()
    .flatten()
    .unwrap_or_default();

    if wxid.is_empty() {
        return None;
    }

    // 3. Call sub_6046AD0 to retrieve current user nickname
    let get_nick_fn: unsafe extern "C" fn(*mut StdString) -> *mut StdString =
        unsafe { mem::transmute(base + RVA_GET_CURRENT_NICK) };

    let mut nick_holder = StdString { words: [0; 3] };
    let nick = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| unsafe {
        get_nick_fn(&mut nick_holder);
        let raw = &nick_holder as *const StdString as *const u8;
        read_libcxx_string(raw).map(|bytes| String::from_utf8_lossy(&bytes).into_owned())
    }))
    .ok()
    .flatten()
    .unwrap_or_default();

    nick_holder.dispose();

    bridge_info!(
        "[ACCOUNT] 成功读取内存当前登录信息: wxid='{}', nick='{}'",
        wxid,
        nick
    );
    update_contact_cache(&wxid, &nick);
    Some((wxid, nick))
}
