use std::ffi::c_void;
use std::sync::atomic::AtomicI32;
use crate::offsets::{RVA_ALLOCATE, RVA_FREE, RVA_APP_FUNC, RVA_SCHEDULER_FUNC, RVA_SCHEDULE_FUNC};

#[repr(C)]
pub struct SmallString {
    pub data: [u8; 16],
    pub length: usize,
    pub capacity: usize,
}

impl SmallString {
    pub fn new() -> Self {
        Self {
            data: [0; 16],
            length: 0,
            capacity: 15,
        }
    }

    pub fn set_native_string(&mut self, core_base: usize, text: &str) -> bool {
        let bytes = text.as_bytes();
        let len = bytes.len();
        if len > 1023 {
            return false;
        }

        if len < 16 {
            self.data[..len].copy_from_slice(bytes);
            self.data[len] = 0;
            self.length = len;
            self.capacity = 15;
            return true;
        }

        unsafe {
            type AllocateFn = extern "system" fn(usize) -> *mut u8;
            let allocate: AllocateFn = std::mem::transmute(core_base + RVA_ALLOCATE);
            let buffer = allocate(len + 1);
            if buffer.is_null() {
                return false;
            }
            std::ptr::copy_nonoverlapping(bytes.as_ptr(), buffer, len);
            *buffer.add(len) = 0;

            *(self.data.as_mut_ptr() as *mut *mut u8) = buffer;
            self.length = len;
            self.capacity = len;
        }
        true
    }

    pub fn free_native_string(&mut self, core_base: usize) {
        if self.capacity > 15 {
            unsafe {
                let allocation = *(self.data.as_ptr() as *const *mut c_void);
                if !allocation.is_null() {
                    type FreeFn = extern "system" fn(*mut c_void, usize);
                    let free: FreeFn = std::mem::transmute(core_base + RVA_FREE);
                    free(allocation, self.capacity + 1);
                }
            }
        }
        self.data = [0; 16];
        self.length = 0;
        self.capacity = 15;
    }

    pub fn to_string_lossy(&self) -> String {
        if self.length == 0 {
            return String::new();
        }
        unsafe {
            let slice = if self.capacity <= 15 {
                &self.data[..self.length.min(15)]
            } else {
                let ptr = *(self.data.as_ptr() as *const *const u8);
                if ptr.is_null() {
                    return String::new();
                }
                std::slice::from_raw_parts(ptr, self.length)
            };
            String::from_utf8_lossy(slice).to_string()
        }
    }
}

impl Drop for SmallString {
    fn drop(&mut self) {
        // Handled via free_native_string(core_base)
    }
}

#[repr(C)]
pub struct TextRequest {
    pub receiver: SmallString,
    pub body: SmallString,
}

#[repr(C)]
#[derive(Default, Copy, Clone)]
pub struct SharedMessage {
    pub object: *mut c_void,
    pub control: *mut c_void,
}

#[repr(C)]
pub struct TextCompletion {
    pub references: AtomicI32,
    pub padding: u32,
    pub invoke: extern "system" fn(*mut TextCompletion, *const u8),
    pub destroy: extern "system" fn(*mut TextCompletion),
    pub user_callback: Option<Box<dyn FnOnce(bool) + Send>>,
}

extern "system" fn completion_invoke(self_ptr: *mut TextCompletion, result: *const u8) {
    if self_ptr.is_null() {
        return;
    }
    unsafe {
        let completion = &mut *self_ptr;
        let success = !result.is_null() && *result == 1;
        if let Some(cb) = completion.user_callback.take() {
            cb(success);
        }
    }
}

extern "system" fn completion_destroy(self_ptr: *mut TextCompletion) {
    if self_ptr.is_null() {
        return;
    }
    unsafe {
        let _ = Box::from_raw(self_ptr);
    }
}

impl TextCompletion {
    pub fn new(cb: Option<Box<dyn FnOnce(bool) + Send>>) -> *mut Self {
        let comp = Box::new(Self {
            references: AtomicI32::new(1),
            padding: 0,
            invoke: completion_invoke,
            destroy: completion_destroy,
            user_callback: cb,
        });
        Box::into_raw(comp)
    }
}

extern "C" {
    fn schedule_wechat_task(
        core_base: usize,
        app_func_rva: usize,
        scheduler_func_rva: usize,
        schedule_func_rva: usize,
        callback: Option<extern "C" fn(*mut c_void)>,
        user_data: *mut c_void,
    ) -> u32;
}

pub fn schedule_task<F: FnOnce() + Send + 'static>(core_base: usize, task: F) -> u32 {
    let boxed: Box<Box<dyn FnOnce() + Send>> = Box::new(Box::new(task));
    let raw = Box::into_raw(boxed) as *mut c_void;

    extern "C" fn runner(arg: *mut c_void) {
        if !arg.is_null() {
            let boxed: Box<Box<dyn FnOnce() + Send>> = unsafe { Box::from_raw(arg as *mut _) };
            boxed();
        }
    }

    unsafe {
        schedule_wechat_task(
            core_base,
            RVA_APP_FUNC,
            RVA_SCHEDULER_FUNC,
            RVA_SCHEDULE_FUNC,
            Some(runner),
            raw,
        )
    }
}
