use serde::Serialize;
use std::ffi::c_void;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};

use crate::ipc::broadcast_event;
use crate::minhook;
use crate::offsets::{RVA_BUILDER, RVA_MENTION_FACTORY, RVA_RECEIVE_HOOK, RVA_VTABLE_MESSAGE};
use crate::wechat_abi::{schedule_task, SharedMessage, SmallString, TextCompletion, TextRequest};

#[derive(Serialize, Clone, Debug)]
pub struct ReceivedMessageEvent {
    pub msg_type: u32,
    pub sender: String,
    pub receiver: String,
    pub text: String,
    pub mention_source: String,
    pub sequence: u64,
}

static SEQUENCE: AtomicU64 = AtomicU64::new(1);
pub static CORE_BASE: std::sync::atomic::AtomicUsize = std::sync::atomic::AtomicUsize::new(0);

type ReceiveFn = extern "system" fn(*mut c_void, *const usize, u8, u8) -> usize;
static mut ORIGINAL_RECEIVE: Option<ReceiveFn> = None;

type FactoryFn = extern "system" fn(*mut SharedMessage) -> *mut SharedMessage;
static mut ORIGINAL_FACTORY: Option<FactoryFn> = None;

static HOOKS_INSTALLED: AtomicBool = AtomicBool::new(false);

thread_local! {
    static MENTION_MEMBERS: std::cell::RefCell<Option<String>> = const { std::cell::RefCell::new(None) };
}

pub struct MentionGuard;
impl MentionGuard {
    pub fn enter(members: String) -> Self {
        MENTION_MEMBERS.with(|m| *m.borrow_mut() = Some(members));
        MentionGuard
    }
}
impl Drop for MentionGuard {
    fn drop(&mut self) {
        MENTION_MEMBERS.with(|m| *m.borrow_mut() = None);
    }
}

extern "system" fn hooked_mention_factory(output: *mut SharedMessage) -> *mut SharedMessage {
    unsafe {
        let original = match ORIGINAL_FACTORY {
            Some(f) => f,
            None => return output,
        };
        let result = original(output);

        MENTION_MEMBERS.with(|members_opt| {
            if let Some(members) = &*members_opt.borrow() {
                if !output.is_null() && !(*output).object.is_null() {
                    let base = CORE_BASE.load(Ordering::Relaxed);
                    let obj = (*output).object as *mut u8;
                    let vtable = *(obj as *const usize);
                    if vtable == base + RVA_VTABLE_MESSAGE {
                        let members_slot = (obj.add(0x778)) as *mut SmallString;
                        if (*members_slot).length == 0 {
                            (*members_slot).set_native_string(base, members);
                        }
                    }
                }
            }
        });

        result
    }
}

unsafe fn read_pb_string(message: *const u8, offset: usize) -> Option<String> {
    let wrapper_ptr = *(message.add(offset) as *const *const u8);
    if wrapper_ptr.is_null() {
        return None;
    }
    let string_ptr = *(wrapper_ptr.add(8) as *const *const SmallString);
    if string_ptr.is_null() {
        return None;
    }
    Some((*string_ptr).to_string_lossy())
}

unsafe fn read_source_string(message: *const u8, offset: usize) -> Option<String> {
    let string_ptr = *(message.add(offset) as *const *const SmallString);
    if string_ptr.is_null() {
        return None;
    }
    let s = (*string_ptr).to_string_lossy();
    if s.contains("<msgsource") || s.contains("<atuserlist") {
        Some(s)
    } else {
        None
    }
}

unsafe fn sample_messages(batch: *const usize) {
    if batch.is_null() {
        return;
    }
    let start = *batch;
    let end = *batch.add(1);
    if end <= start || (end - start) % 0x78 != 0 || (end - start) / 0x78 > 512 {
        return;
    }

    let mut addr = start;
    while addr < end {
        let msg = addr as *const u8;
        let msg_type = *(msg.add(0x14) as *const u32);
        if msg_type == 1 {
            if let (Some(sender), Some(receiver), Some(text)) = (
                read_pb_string(msg, 8),
                read_pb_string(msg, 0x18),
                read_pb_string(msg, 0x20),
            ) {
                let source = read_source_string(msg, 0x38).unwrap_or_default();
                let seq = SEQUENCE.fetch_add(1, Ordering::Relaxed);
                let event = ReceivedMessageEvent {
                    msg_type,
                    sender,
                    receiver,
                    text,
                    mention_source: source,
                    sequence: seq,
                };
                broadcast_event("recv_msg", &event);
            }
        }
        addr += 0x78;
    }
}

extern "system" fn hooked_receive(
    manager: *mut c_void,
    batch: *const usize,
    flag1: u8,
    flag2: u8,
) -> usize {
    unsafe {
        sample_messages(batch);
        if let Some(original) = ORIGINAL_RECEIVE {
            original(manager, batch, flag1, flag2)
        } else {
            0
        }
    }
}

pub fn install_hooks(core_base: usize) -> bool {
    if HOOKS_INSTALLED.load(Ordering::SeqCst) {
        return true;
    }
    CORE_BASE.store(core_base, Ordering::SeqCst);

    minhook::initialize();

    unsafe {
        // 1. Hook receive function
        let recv_target = (core_base + RVA_RECEIVE_HOOK) as *mut c_void;
        let mut original_recv_ptr: *mut c_void = std::ptr::null_mut();
        if minhook::create_hook(
            recv_target,
            hooked_receive as *mut c_void,
            &mut original_recv_ptr,
        ) == minhook::MhStatus::Ok
        {
            ORIGINAL_RECEIVE = Some(std::mem::transmute(original_recv_ptr));
            minhook::enable_hook(recv_target);
        } else {
            return false;
        }

        // 2. Hook mention factory
        let factory_target = (core_base + RVA_MENTION_FACTORY) as *mut c_void;
        let mut original_factory_ptr: *mut c_void = std::ptr::null_mut();
        if minhook::create_hook(
            factory_target,
            hooked_mention_factory as *mut c_void,
            &mut original_factory_ptr,
        ) == minhook::MhStatus::Ok
        {
            ORIGINAL_FACTORY = Some(std::mem::transmute(original_factory_ptr));
            minhook::enable_hook(factory_target);
        } else {
            return false;
        }
    }

    HOOKS_INSTALLED.store(true, Ordering::SeqCst);
    true
}

pub fn uninstall_hooks() {
    if !HOOKS_INSTALLED.load(Ordering::SeqCst) {
        return;
    }
    let base = CORE_BASE.load(Ordering::SeqCst);
    unsafe {
        minhook::disable_hook((base + RVA_RECEIVE_HOOK) as *mut c_void);
        minhook::remove_hook((base + RVA_RECEIVE_HOOK) as *mut c_void);
        minhook::disable_hook((base + RVA_MENTION_FACTORY) as *mut c_void);
        minhook::remove_hook((base + RVA_MENTION_FACTORY) as *mut c_void);
        minhook::uninitialize();
    }
    HOOKS_INSTALLED.store(false, Ordering::SeqCst);
}

pub fn send_text(target: &str, text: &str, at_members: Option<String>) -> Result<(), String> {
    let base = CORE_BASE.load(Ordering::SeqCst);
    if base == 0 {
        return Err("Weixin.dll base not initialized".to_string());
    }

    let target_owned = target.to_string();
    let text_owned = text.to_string();

    let task = move || {
        let _guard = at_members.map(MentionGuard::enter);

        let mut request = TextRequest {
            receiver: SmallString::new(),
            body: SmallString::new(),
        };

        if !request.receiver.set_native_string(base, &target_owned)
            || !request.body.set_native_string(base, &text_owned)
        {
            request.receiver.free_native_string(base);
            request.body.free_native_string(base);
            return;
        }

        let mut description = SmallString::new();
        description.set_native_string(base, "wxbot.rust");

        let mut completion = TextCompletion::new(None);

        unsafe {
            type BuilderFn = extern "system" fn(
                *mut c_void,
                *mut TextRequest,
                *mut SmallString,
                *mut *mut TextCompletion,
            );
            let builder: BuilderFn = std::mem::transmute(base + RVA_BUILDER);
            builder(
                std::ptr::null_mut(),
                &mut request,
                &mut description,
                &mut completion,
            );
        }

        if !completion.is_null() {
            unsafe {
                let _ = Box::from_raw(completion);
            }
        }

        request.receiver.free_native_string(base);
        request.body.free_native_string(base);
        description.free_native_string(base);
    };

    let ret = schedule_task(base, task);
    if ret != 0 {
        return Err(format!("schedule_task failed with code {}", ret));
    }
    Ok(())
}
