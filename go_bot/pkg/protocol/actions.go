// Package protocol defines OneBot 11 types and native bridge action names.
// Raw JSON parameters are preserved to avoid message-ID precision loss.
package protocol

var Actions = []string{
	"checkWeChat",
	"sendText", "sendText2", "sendXml", "sendImage", "sendFile", "sendGif", "sendCard", "sendShareUrl", "sendApplet", "sendReferText", "revokeMyMsg", "revoke_msg",
	"getSelfInfo", "getMemberNick", "getFriendList", "getGroupList", "getMemberList", "getPublicList", "getLabelList", "queryObj", "queryGroup",
	"createGroup", "addMembers", "inviteMembers", "delMembers", "quitGroup", "editSelfMemberNick", "agreeFriendReq", "delFriend", "editObjRemark",
	"confirmTrans", "returnTrans", "setReadStatus", "receiveHongBao", "openHongBao",
}

var aliases = map[string]string{
	"ping": "checkWeChat", "check_wechat": "checkWeChat",
	"send_text": "sendText", "send_at_text": "sendText2", "send_xml": "sendXml", "send_image": "sendImage",
	"send_file": "sendFile", "send_gif": "sendGif", "send_card": "sendCard", "send_share_url": "sendShareUrl",
	"send_applet": "sendApplet", "send_refer_text": "sendReferText", "revoke_my_msg": "revokeMyMsg", "revoke_msg": "revoke_msg",
	"get_account": "getSelfInfo", "get_self_info": "getSelfInfo", "get_member_nick": "getMemberNick", "get_contact_name": "getMemberNick",
	"get_friend_list": "getFriendList", "get_group_list": "getGroupList", "get_member_list": "getMemberList",
	"get_public_list": "getPublicList", "get_label_list": "getLabelList", "query_obj": "queryObj", "query_group": "queryGroup",
	"create_group": "createGroup", "add_members": "addMembers", "invite_members": "inviteMembers", "del_members": "delMembers",
	"quit_group": "quitGroup", "set_group_leave": "quitGroup", "set_group_kick": "delMembers",
	"set_group_card": "editSelfMemberNick", "edit_self_member_nick": "editSelfMemberNick",
	"agree_friend_req": "agreeFriendReq", "del_friend": "delFriend", "delete_friend": "delFriend",
	"edit_obj_remark": "editObjRemark", "edit_remark": "editObjRemark", "set_friend_remark": "editObjRemark", "set_friend_add_request": "editObjRemark",
	"confirm_trans": "confirmTrans", "return_trans": "returnTrans", "set_read_status": "setReadStatus",
	"receive_hong_bao": "receiveHongBao", "open_hong_bao": "openHongBao",
}

// Canonical accepts documented names plus legacy snake_case action aliases.
func Canonical(action string) (string, bool) {
	if name, ok := aliases[action]; ok {
		return name, true
	}
	for _, name := range Actions {
		if name == action {
			return name, true
		}
	}
	if IsOneBotAction(action) {
		return action, true
	}
	return "", false
}
