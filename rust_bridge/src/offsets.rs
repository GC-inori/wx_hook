pub const WECHAT_VERSION: &str = "4.1.15.55";
pub const RVA_DB_KEY_GETTER: usize = 0x619FF60;
pub const RVA_VERIFY_USER: usize = 0x6921760;
pub const RVA_CO_ACCEPT_PAY_TRANSFER: usize = 0x7269CA0;
pub const RVA_HONGBAO_REQ_INIT: usize = 0x7CC1BD0;
pub const RVA_HONGBAO_REQ_FREE: usize = 0x7CC1DA0;
pub const RVA_SK_BUFFER_INIT: usize = 0x7BA6400;
pub const RVA_CO_CGI_SEND_COMMON_REQUEST: usize = 0x727ED70;
pub const RVA_HONGBAO_RES_FREE: usize = 0x7CC2700;
pub const RVA_GLOBAL_APP_CONTEXT: usize = 0xACF6710;
pub const CORE_SIZE: u64 = 204304944;
pub const CORE_SHA256: [u8; 32] = [
    0x6f, 0xbb, 0xba, 0x53, 0x6b, 0x04, 0xc0, 0xa2, 0x47, 0x30, 0x71, 0x0f, 0x54, 0x08, 0x87, 0xa5,
    0x3f, 0x4e, 0xaa, 0x24, 0x39, 0x8e, 0x4f, 0x94, 0x90, 0x5d, 0x9b, 0x4a, 0x36, 0x14, 0x85, 0x95,
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
pub const RVA_APP_CONTEXT_GETTER_SHARED: usize = 0x60421E0;
pub const RVA_CONTACT_PROVIDER_GETTER: usize = 0x619CE90;
pub const RVA_CONTACT_SERVICE_GETTER: usize = 0x6111260;
pub const RVA_DELETE_CONTACT: usize = 0x786F830;

// Chatroom & Oplog RVAs
pub const RVA_CHATROOM_SERVICE_GETTER: usize = 0x6095540;
pub const RVA_OPLOG_SERVICE_GETTER: usize = 0x61AE4B0;
pub const RVA_CREATE_CHATROOM: usize = 0x78C0310;
pub const RVA_ADD_CHATROOM_MEMBER: usize = 0x78BE100;
pub const RVA_DEL_CHATROOM_MEMBER: usize = 0x78C49E0;
pub const RVA_QUIT_CHATROOM: usize = 0x78FB8C0;
pub const RVA_MOD_CHATROOM_MEMBER_NICK: usize = 0x78FBCA0;
pub const RVA_EDIT_OBJ_REMARK: usize = 0x76BF030;
pub const RVA_VTABLE_EDIT_REMARK: usize = 0xA966508;

// Revoke & Enumeration RVAs
pub const RVA_REVOKE_MSG_SEND_CGI: usize = 0x6AD8DC0;
pub const RVA_REVOKE_MSG_REQUEST_INIT: usize = 0x7CF78E0;
pub const RVA_REVOKE_MSG_REQUEST_FREE: usize = 0x7CF7C00;
pub const RVA_GET_CHATROOM_INFO: usize = 0x65A10B0;
pub const RVA_CONTACT_STORAGE_GETTER: usize = 0x6383FB0;
pub const RVA_GET_ALL_ROOM_CONTACTS: usize = 0x65B3570;
pub const RVA_GET_CONTACT_BY_WXID: usize = 0x786E260;
