use std::ffi::c_void;

#[repr(i32)]
#[derive(Debug, PartialEq, Eq, Clone, Copy)]
pub enum MhStatus {
    Ok = 0,
    Unknown = -1,
    AlreadyInitialized = 1,
    NotInitialized = 2,
    AlreadyCreated = 3,
    NotCreated = 4,
    Enabled = 5,
    Disabled = 6,
    NotExecutable = 7,
    UnsupportedFunction = 8,
    MemoryAlloc = 9,
    MemoryProtect = 10,
    ModuleNotFound = 11,
    FunctionNotFound = 12,
}

extern "C" {
    fn MH_Initialize() -> i32;
    fn MH_Uninitialize() -> i32;
    fn MH_CreateHook(
        p_target: *mut c_void,
        p_detour: *mut c_void,
        pp_original: *mut *mut c_void,
    ) -> i32;
    fn MH_EnableHook(p_target: *mut c_void) -> i32;
    fn MH_DisableHook(p_target: *mut c_void) -> i32;
    fn MH_RemoveHook(p_target: *mut c_void) -> i32;
}

pub fn initialize() -> MhStatus {
    match unsafe { MH_Initialize() } {
        0 => MhStatus::Ok,
        1 => MhStatus::AlreadyInitialized,
        _ => MhStatus::Unknown,
    }
}

pub fn uninitialize() -> MhStatus {
    match unsafe { MH_Uninitialize() } {
        0 => MhStatus::Ok,
        _ => MhStatus::Unknown,
    }
}

pub unsafe fn create_hook(
    target: *mut c_void,
    detour: *mut c_void,
    original: *mut *mut c_void,
) -> MhStatus {
    match MH_CreateHook(target, detour, original) {
        0 => MhStatus::Ok,
        3 => MhStatus::AlreadyCreated,
        _ => MhStatus::Unknown,
    }
}

pub unsafe fn enable_hook(target: *mut c_void) -> MhStatus {
    match MH_EnableHook(target) {
        0 => MhStatus::Ok,
        _ => MhStatus::Unknown,
    }
}

pub unsafe fn disable_hook(target: *mut c_void) -> MhStatus {
    match MH_DisableHook(target) {
        0 => MhStatus::Ok,
        _ => MhStatus::Unknown,
    }
}

pub unsafe fn remove_hook(target: *mut c_void) -> MhStatus {
    match MH_RemoveHook(target) {
        0 => MhStatus::Ok,
        _ => MhStatus::Unknown,
    }
}
