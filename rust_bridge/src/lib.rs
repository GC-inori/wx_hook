#[cfg(windows)]
pub mod account;
#[cfg(windows)]
pub mod hooks;
#[cfg(windows)]
pub mod ipc;
#[cfg(windows)]
pub mod minhook;
#[cfg(windows)]
pub mod offsets;
#[cfg(windows)]
pub mod wechat_abi;

#[cfg(target_os = "linux")]
pub mod linux;

#[cfg(windows)]
mod windows_entry {
    use std::ffi::c_void;
    use std::thread;
    use windows_sys::Win32::Foundation::{BOOL, HMODULE, TRUE};
    use windows_sys::Win32::System::LibraryLoader::{DisableThreadLibraryCalls, GetModuleHandleW};
    use windows_sys::Win32::System::SystemServices::DLL_PROCESS_ATTACH;

    pub(super) fn get_weixin_module() -> Option<usize> {
        let module_name: Vec<u16> = "Weixin.dll\0".encode_utf16().collect();
        unsafe {
            let handle = GetModuleHandleW(module_name.as_ptr());
            if handle.is_null() {
                None
            } else {
                Some(handle as usize)
            }
        }
    }

    fn init_runtime() {
        thread::spawn(|| {
            for _ in 0..20 {
                if let Some(base) = get_weixin_module() {
                    if crate::hooks::install_hooks(base) {
                        crate::ipc::start_pipe_server();
                        break;
                    }
                }
                thread::sleep(std::time::Duration::from_millis(500));
            }
        });
    }

    #[no_mangle]
    #[allow(non_snake_case)]
    pub unsafe extern "system" fn DllMain(
        module: HMODULE,
        reason: u32,
        _reserved: *mut c_void,
    ) -> BOOL {
        if reason == DLL_PROCESS_ATTACH {
            DisableThreadLibraryCalls(module);
            init_runtime();
        }
        TRUE
    }
}

#[cfg(windows)]
pub use windows_entry::DllMain;

#[cfg(windows)]
mod legacy_exports {
    use std::ffi::c_void;

    #[repr(C)]
    pub struct LegacyConfig {
        pub size: u32,
        pub pid: u32,
        pub manager: u64,
        pub log_path: [u16; 260],
        pub text: [u8; 1024],
    }

    #[repr(C)]
    pub struct LegacyTextConfig {
        pub base: LegacyConfig,
        pub target: [u8; 128],
    }

    #[repr(C)]
    pub struct LegacyAtTextConfig {
        pub base: LegacyTextConfig,
        pub members: [u8; 1024],
    }

    #[no_mangle]
    #[allow(non_snake_case)]
    pub unsafe extern "system" fn WxStartTextReceiver(parameter: *mut c_void) -> u32 {
        if parameter.is_null() {
            return 1;
        }
        if let Some(base) = super::get_weixin_module() {
            if crate::hooks::install_hooks(base) {
                crate::ipc::start_pipe_server();
                return 0;
            }
        }
        4
    }

    #[no_mangle]
    #[allow(non_snake_case)]
    pub unsafe extern "system" fn WxSendText(parameter: *mut c_void) -> u32 {
        if parameter.is_null() {
            return 1;
        }
        let config = &*(parameter as *const LegacyTextConfig);
        let target = match std::ffi::CStr::from_ptr(config.target.as_ptr() as *const i8).to_str() {
            Ok(s) => s,
            Err(_) => return 13,
        };
        let text = match std::ffi::CStr::from_ptr(config.base.text.as_ptr() as *const i8).to_str() {
            Ok(s) => s,
            Err(_) => return 13,
        };
        match crate::hooks::send_text(target, text, None) {
            Ok(_) => 0,
            Err(_) => 5,
        }
    }

    #[no_mangle]
    #[allow(non_snake_case)]
    pub unsafe extern "system" fn WxSendAtText(parameter: *mut c_void) -> u32 {
        if parameter.is_null() {
            return 1;
        }
        let config = &*(parameter as *const LegacyAtTextConfig);
        let target =
            match std::ffi::CStr::from_ptr(config.base.target.as_ptr() as *const i8).to_str() {
                Ok(s) => s,
                Err(_) => return 13,
            };
        let text =
            match std::ffi::CStr::from_ptr(config.base.base.text.as_ptr() as *const i8).to_str() {
                Ok(s) => s,
                Err(_) => return 13,
            };
        let members = match std::ffi::CStr::from_ptr(config.members.as_ptr() as *const i8).to_str()
        {
            Ok(s) => s,
            Err(_) => return 13,
        };
        match crate::hooks::send_text(target, text, Some(members.to_string())) {
            Ok(_) => 0,
            Err(_) => 5,
        }
    }

    #[no_mangle]
    #[allow(non_snake_case)]
    pub unsafe extern "system" fn WxRemoveSelfTest(_: *mut c_void) -> u32 {
        crate::hooks::uninstall_hooks();
        0
    }

    #[no_mangle]
    #[allow(non_snake_case)]
    pub unsafe extern "system" fn WxWaitForTasks(_: *mut c_void) -> u32 {
        0
    }
}

#[cfg(windows)]
use windows_entry::get_weixin_module;
