use crate::offsets::{
    RVA_APP_GLOBAL, RVA_APP_VTABLE, RVA_CACHE_PROVIDER_VTABLE, RVA_CONTACT_CACHE_VTABLE,
    RVA_CONTEXT_VTABLE,
};
use crate::wechat_abi::SmallString;

fn is_valid_ptr(ptr: usize) -> bool {
    (0x10000..0x800000000000).contains(&ptr)
}

pub fn get_current_account(core_base: usize) -> Option<String> {
    unsafe {
        let app_global_ptr = (core_base + RVA_APP_GLOBAL) as *const *const usize;
        if !is_valid_ptr(app_global_ptr as usize) {
            return None;
        }
        let app = *app_global_ptr;
        if !is_valid_ptr(app as usize) {
            return None;
        }

        let app_vtable = *app;
        if app_vtable != core_base + RVA_APP_VTABLE {
            return None;
        }

        let context = *(app.add(0x68 / 8) as *const *const usize);
        if !is_valid_ptr(context as usize) {
            return None;
        }

        let context_vtable = *context;
        if context_vtable != core_base + RVA_CONTEXT_VTABLE {
            return None;
        }

        let small_str = (context as *const u8).add(0x48) as *const SmallString;
        let account = (*small_str).to_string_lossy();
        if account.is_empty() || account == "filehelper" || account.ends_with("@chatroom") {
            return None;
        }
        Some(account)
    }
}

pub fn get_contact_name(core_base: usize, wxid: &str) -> Option<String> {
    unsafe {
        let app_global_ptr = (core_base + RVA_APP_GLOBAL) as *const *const usize;
        if !is_valid_ptr(app_global_ptr as usize) {
            return None;
        }
        let app = *app_global_ptr;
        if !is_valid_ptr(app as usize) {
            return None;
        }

        let context = *(app.add(0x68 / 8) as *const *const usize);
        if !is_valid_ptr(context as usize) {
            return None;
        }

        let provider = *((context as *const u8).add(0x460) as *const *const usize);
        if !is_valid_ptr(provider as usize) || *provider != core_base + RVA_CACHE_PROVIDER_VTABLE {
            return None;
        }

        let registry_head = *((provider as *const u8).add(0x90) as *const *const usize);
        let registry_size = *((provider as *const u8).add(0x98) as *const usize);
        if !(1..=512).contains(&registry_size) || !is_valid_ptr(registry_head as usize) {
            return None;
        }

        let mut node = *registry_head;
        let mut cache: *const usize = std::ptr::null();
        for _ in 0..registry_size {
            if node == registry_head as usize || node == 0 {
                break;
            }
            let value = *((node as *const u8).add(0x30) as *const *const usize);
            if is_valid_ptr(value as usize) && *value == core_base + RVA_CONTACT_CACHE_VTABLE {
                cache = value;
                break;
            }
            node = *(node as *const usize);
        }

        if cache.is_null() {
            return None;
        }

        // MSVC std::unordered_map at cache + 0xC8
        let map_layout = (cache as *const u8).add(0xC8);
        let sentinel = *((map_layout.add(8)) as *const usize);
        let count = *((map_layout.add(16)) as *const usize);
        let buckets = *((map_layout.add(24)) as *const usize);
        let mask = *((map_layout.add(48)) as *const usize);

        if !(1..=100000).contains(&count) || mask == 0 || (mask + 1) & mask != 0 {
            return None;
        }

        // FNV-1a 64-bit hash
        let mut hash_val: u64 = 0xCBF29CE484222325;
        for byte in wxid.as_bytes() {
            hash_val = (hash_val ^ (*byte as u64)).wrapping_mul(0x100000001B3);
        }

        let bucket_addr = buckets + ((hash_val & (mask as u64)) as usize) * 16;
        let bucket_node = *(bucket_addr as *const usize);
        if !is_valid_ptr(bucket_node) {
            return None;
        }

        let mut curr_node = bucket_node;
        for _ in 0..count.min(256) {
            if curr_node == sentinel || curr_node == 0 {
                return None;
            }
            let node_wxid_str = (curr_node as *const u8).add(0x10) as *const SmallString;
            if (*node_wxid_str).to_string_lossy() == wxid {
                let contact = *((curr_node as *const u8).add(0x30) as *const *const usize);
                if is_valid_ptr(contact as usize) {
                    let name_str = (contact as *const u8).add(0xD8) as *const SmallString;
                    let name = (*name_str).to_string_lossy();
                    if !name.trim().is_empty() {
                        return Some(name.trim().to_string());
                    }
                }
                return None;
            }
            curr_node = *(curr_node as *const usize);
        }

        None
    }
}
