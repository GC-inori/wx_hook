pub const WECHAT_VERSION: &str = "4.1.15.55";
pub const CORE_SIZE: u64 = 204304944;
pub const CORE_SHA256: [u8; 32] = [
    0x6f, 0xbb, 0xba, 0x53, 0x6b, 0x04, 0xc0, 0xa2, 0x47, 0x30, 0x71, 0x0f, 0x54, 0x08, 0x87,
    0xa5, 0x3f, 0x4e, 0xaa, 0x24, 0x39, 0x8e, 0x4f, 0x94, 0x90, 0x5d, 0x9b, 0x4a, 0x36, 0x14,
    0x85, 0x95,
];

pub const RVA_RECEIVE_HOOK: usize = 0x17B9020;
pub const RVA_MENTION_FACTORY: usize = 0x709D40;
pub const RVA_VTABLE_MESSAGE: usize = 0x9026B78;
pub const RVA_BUILDER: usize = 0x55C9BE0;
pub const RVA_APP_FUNC: usize = 0x45D10;
pub const RVA_SCHEDULER_FUNC: usize = 0x47310;
pub const RVA_SCHEDULE_FUNC: usize = 0x30F9F0;
pub const RVA_ALLOCATE: usize = 0x754C41C;
pub const RVA_FREE: usize = 0x754C458;

// Account & Contact cache RVAs
pub const RVA_APP_GLOBAL: usize = 0xBB92FC0;
pub const RVA_APP_VTABLE: usize = 0x8F514E8;
pub const RVA_CONTEXT_VTABLE: usize = 0x8FD1498;
pub const RVA_APP_CONTEXT_GETTER: usize = 0x469A0;
pub const RVA_IDENTITY_GETTER: usize = 0x3855C0;
pub const RVA_CACHE_PROVIDER_GETTER: usize = 0x382440;
pub const RVA_CACHE_PROVIDER_VTABLE: usize = 0x8FD19A8;
pub const RVA_CONTACT_CACHE_VTABLE: usize = 0x916B1F8;
