//! Pure message classification, separated from native memory access for testing.
use quick_xml::events::Event;
use quick_xml::Reader;
use serde::Serialize;
use serde_json::{json, Value};
use std::collections::HashMap;

#[derive(Serialize, Clone, Debug)]
pub struct ReceivedMessageEvent {
    pub msg_type: u32,
    pub sender: String,
    pub receiver: String,
    pub text: String,
    pub mention_source: String,
    pub sequence: u64,
    pub from_wxid: String,
    pub final_from_wxid: String,
    pub msg: String,
    pub msg_id: u64,
}

impl ReceivedMessageEvent {
    pub fn new(
        msg_type: u32,
        msg_id: u64,
        sender: String,
        receiver: String,
        text: String,
        mention_source: String,
    ) -> Self {
        let from_wxid = if receiver.ends_with("@chatroom") {
            receiver.clone()
        } else {
            sender.clone()
        };
        let final_from_wxid = if sender.ends_with("@chatroom") {
            text.split_once(":\n")
                .map(|(member, _)| member)
                .filter(|member| {
                    !member.is_empty()
                        && member.len() <= 256
                        && !member.chars().any(char::is_whitespace)
                })
                .unwrap_or(&sender)
                .to_owned()
        } else {
            sender.clone()
        };
        Self {
            msg_type,
            msg_id,
            sequence: msg_id,
            from_wxid,
            final_from_wxid,
            msg: text.clone(),
            sender,
            receiver,
            text,
            mention_source,
        }
    }
}

#[derive(Default)]
struct XmlFields {
    text: HashMap<String, String>,
    attributes: HashMap<String, String>,
}

impl XmlFields {
    fn value(&self, suffix: &str) -> String {
        self.text
            .iter()
            .find(|(path, _)| {
                path.as_str() == suffix
                    || path
                        .strip_suffix(suffix)
                        .is_some_and(|prefix| prefix.ends_with('/'))
            })
            .map(|(_, value)| value.clone())
            .unwrap_or_default()
    }

    fn attribute(&self, path: &str, name: &str) -> String {
        self.attributes
            .get(&format!("{path}@{name}"))
            .cloned()
            .unwrap_or_default()
    }
}

// The parser never resolves external entities or DTDs. Bound nesting/node counts
// in addition to the native string size limit to keep work on the host bounded.
fn parse_xml(xml: &str) -> Option<XmlFields> {
    if xml.len() > 1024 * 1024 {
        return None;
    }
    let mut reader = Reader::from_str(xml);
    let mut fields = XmlFields::default();
    let mut stack = Vec::new();
    let mut nodes = 0usize;
    loop {
        let event = reader.read_event().ok()?;
        let empty = matches!(&event, Event::Empty(_));
        match event {
            Event::Start(element) | Event::Empty(element) => {
                nodes += 1;
                if nodes > 4096 || stack.len() >= 64 {
                    return None;
                }
                let name = String::from_utf8_lossy(element.name().as_ref()).into_owned();
                let mut path = stack.join("/");
                if !path.is_empty() {
                    path.push('/');
                }
                path.push_str(&name);
                for attribute in element.attributes() {
                    let attribute = attribute.ok()?;
                    let key = String::from_utf8_lossy(attribute.key.as_ref());
                    let value = attribute.decode_and_unescape_value(reader.decoder()).ok()?;
                    fields
                        .attributes
                        .insert(format!("{path}@{key}"), value.into_owned());
                }
                // Empty elements end in '/>' and do not alter the parent path.
                if !empty {
                    stack.push(name);
                }
            }
            Event::End(_) => {
                stack.pop()?;
            }
            Event::Text(text) => {
                let decoded = text.decode().ok()?;
                let decoded = quick_xml::escape::unescape(&decoded).ok()?;
                fields
                    .text
                    .entry(stack.join("/"))
                    .or_default()
                    .push_str(&decoded);
            }
            Event::CData(text) => {
                fields
                    .text
                    .entry(stack.join("/"))
                    .or_default()
                    .push_str(&text.decode().ok()?);
            }
            Event::GeneralRef(reference) => {
                let name = reference.decode().ok()?;
                let entity = format!("&{name};");
                fields
                    .text
                    .entry(stack.join("/"))
                    .or_default()
                    .push_str(&quick_xml::escape::unescape(&entity).ok()?);
            }
            Event::DocType(_) => return None,
            Event::Eof => return if stack.is_empty() { Some(fields) } else { None },
            _ => {}
        }
    }
}

fn percent_decode_query_value(value: &str) -> String {
    let bytes = value.as_bytes();
    let mut decoded = Vec::with_capacity(bytes.len());
    let mut index = 0;
    while index < bytes.len() {
        match bytes[index] {
            b'%' if index + 2 < bytes.len() => {
                let hex = std::str::from_utf8(&bytes[index + 1..index + 3]).unwrap_or_default();
                if let Ok(byte) = u8::from_str_radix(hex, 16) {
                    decoded.push(byte);
                    index += 3;
                    continue;
                }
                decoded.push(bytes[index]);
            }
            b'+' => decoded.push(b' '),
            byte => decoded.push(byte),
        }
        index += 1;
    }
    String::from_utf8_lossy(&decoded).into_owned()
}

fn url_query_value(url: &str, wanted: &str) -> Option<String> {
    let query = url.split_once('?').map_or(url, |(_, query)| query);
    let query = query.split('#').next().unwrap_or(query);
    query
        .split(['&', ';'])
        .filter_map(|pair| pair.split_once('='))
        .find(|(key, _)| key.eq_ignore_ascii_case(wanted))
        .map(|(_, value)| percent_decode_query_value(value))
}

/// Return exactly one public event, retaining the original message fields even
/// when optional XML is malformed or a system-message subtype is not recognized.
pub fn classify_message(event: &ReceivedMessageEvent) -> (&'static str, Value) {
    let mut data = json!(event);
    let xml_text = if event.sender.ends_with("@chatroom") {
        event
            .text
            .split_once(":\n")
            .map_or(event.text.as_str(), |(_, body)| body)
    } else {
        &event.text
    };
    let xml = if matches!(event.msg_type, 37 | 49 | 10000 | 10002) {
        parse_xml(xml_text).unwrap_or_default()
    } else {
        XmlFields::default()
    };
    let name = match event.msg_type {
        10002 => {
            let revoked_id = xml.value("revokemsg/newmsgid");
            let revoked_id = if revoked_id.is_empty() {
                xml.value("revokemsg/msgid")
            } else {
                revoked_id
            };
            // Keep the original decimal representation; server IDs can exceed
            // JavaScript's exact-integer range. The envelope msg_id stays intact.
            data["revoked_msg_id"] = json!(revoked_id);
            data["revoked_content"] = json!(xml.value("revokemsg/replacemsg"));
            data["session"] = json!(xml.value("revokemsg/session"));
            "revokeMsg"
        }
        37 => {
            let wxid = xml.attribute("msg", "fromusername");
            let nickname = xml.attribute("msg", "fromnickname");
            data["applicant"] = json!({
                "wxid": if wxid.is_empty() { event.final_from_wxid.clone() } else { wxid },
                "nickname": nickname,
                "alias": xml.attribute("msg", "alias"),
                "content": xml.attribute("msg", "content"),
                "ticket": xml.attribute("msg", "ticket"),
                "encryptusername": xml.attribute("msg", "encryptusername"),
                "scene": xml.attribute("msg", "scene"),
            });
            "friendReq"
        }
        49 => match xml.value("appmsg/type").trim() {
            "2000" => {
                data["transfer_id"] = json!(xml.value("wcpayinfo/transferid"));
                data["transaction_id"] = json!(xml.value("wcpayinfo/transcationid"));
                data["amount"] = json!(xml.value("wcpayinfo/feedesc"));
                data["pay_subtype"] = json!(xml.value("wcpayinfo/paysubtype"));
                data["invalid_time"] = json!(xml.value("wcpayinfo/invalidtime"));
                data["transfer_attach"] = json!(xml.value("wcpayinfo/transferattach"));
                "transPay"
            }
            "2001" => {
                let native_url = xml.value("wcpayinfo/nativeurl");
                let sender_title = {
                    let value = xml.value("wcpayinfo/sendertitle");
                    if value.is_empty() {
                        xml.value("wcpayinfo/sender_title")
                    } else {
                        value
                    }
                };
                let receiver_title = {
                    let value = xml.value("wcpayinfo/receivertitle");
                    if value.is_empty() {
                        xml.value("wcpayinfo/receiver_title")
                    } else {
                        value
                    }
                };
                let is_red_packet = !receiver_title.contains("收款")
                    && (native_url.to_ascii_lowercase().contains("hongbao")
                        || receiver_title.contains("红包")
                        || (!sender_title.is_empty() && !sender_title.contains("收款")));
                if is_red_packet {
                    data["native_url"] = json!(native_url);
                    data["title"] = json!(sender_title);
                    data["pay_msg_id"] = json!(xml.value("wcpayinfo/paymsgid"));
                    data["send_id"] =
                        json!(url_query_value(&native_url, "sendid").unwrap_or_default());
                    data["channel_id"] =
                        json!(url_query_value(&native_url, "channelid")
                            .unwrap_or_else(|| "1".to_owned()));
                    "hongBao"
                } else if receiver_title.contains("收款")
                    || !xml.value("wcpayinfo/paymsgid").is_empty()
                    || !xml.value("wcpayinfo/outtradeno").is_empty()
                {
                    data["payment_id"] = json!(xml.value("wcpayinfo/paymsgid"));
                    data["amount"] = json!(xml.value("wcpayinfo/feedesc"));
                    "qrPay"
                } else {
                    "recvMsg"
                }
            }
            _ => "recvMsg",
        },
        10000 => {
            let system_type = xml.attribute("sysmsg", "type");
            let member_change = matches!(
                system_type.as_str(),
                "delchatroommember" | "addchatroommember"
            ) || [
                "邀请",
                "加入了群聊",
                "移出了群聊",
                "退出了群聊",
                "invited",
                "joined the group chat",
                "removed from the group chat",
                "left the group chat",
            ]
            .iter()
            .any(|marker| event.text.contains(marker));
            data["system_type"] = json!(system_type);
            data["notification"] = json!(event.text);
            if member_change {
                data["group_wxid"] = json!(event.from_wxid);
                "groupMemberChanges"
            } else {
                "systemMsg"
            }
        }
        _ => "recvMsg",
    };
    (name, data)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn event(kind: u32, text: &str) -> ReceivedMessageEvent {
        ReceivedMessageEvent::new(
            kind,
            9007199254740993,
            "wxid_sender".into(),
            "wxid_receiver".into(),
            text.into(),
            String::new(),
        )
    }

    #[test]
    fn ordinary_message_preserves_required_fields_and_id() {
        let (name, data) = classify_message(&event(1, "hello"));
        assert_eq!(name, "recvMsg");
        for key in [
            "from_wxid",
            "final_from_wxid",
            "msg",
            "msg_id",
            "sender",
            "receiver",
            "text",
            "sequence",
        ] {
            assert!(data.get(key).is_some(), "{key}");
        }
        assert_eq!(data["msg_id"].as_u64(), Some(9007199254740993));
    }

    #[test]
    fn group_sender_is_extracted_only_from_inbound_prefix() {
        let mut message = event(1, "wxid_member:\nhello");
        message = ReceivedMessageEvent::new(
            1,
            42,
            "room@chatroom".into(),
            message.receiver,
            message.text,
            String::new(),
        );
        assert_eq!(message.from_wxid, "room@chatroom");
        assert_eq!(message.final_from_wxid, "wxid_member");
        let outgoing = ReceivedMessageEvent::new(
            1,
            43,
            "self".into(),
            "room@chatroom".into(),
            "literal:\ntext".into(),
            String::new(),
        );
        assert_eq!(outgoing.final_from_wxid, "self");
    }

    #[test]
    fn revocation_decodes_cdata_and_keeps_revoked_id_as_string() {
        let (name, data) = classify_message(&event(10002, "<sysmsg><revokemsg><session>room</session><newmsgid>18446744073709551615</newmsgid><replacemsg><![CDATA[A recalled <message>]]></replacemsg></revokemsg></sysmsg>"));
        assert_eq!(name, "revokeMsg");
        assert_eq!(data["revoked_msg_id"], "18446744073709551615");
        assert_eq!(data["revoked_content"], "A recalled <message>");
    }

    #[test]
    fn friend_request_extracts_attributes_and_escapes() {
        let (name, data) = classify_message(&event(37, "<msg fromusername='wxid_applicant' fromnickname='A &amp; B' ticket='ticket' content='hello'/>"));
        assert_eq!(name, "friendReq");
        assert_eq!(data["applicant"]["wxid"], "wxid_applicant");
        assert_eq!(data["applicant"]["nickname"], "A & B");
    }

    #[test]
    fn red_packets_are_structured_without_misclassifying_qr_payments() {
        let red_packet = "<msg><appmsg><type>2001</type><wcpayinfo><nativeurl>wxpay://c2c/hongbao?sendid=send-123&amp;channelid=7</nativeurl><sendertitle>新年快乐</sendertitle><paymsgid>pay-9</paymsgid></wcpayinfo></appmsg></msg>";
        let (name, data) = classify_message(&event(49, red_packet));
        assert_eq!(name, "hongBao");
        assert_eq!(
            data["native_url"],
            "wxpay://c2c/hongbao?sendid=send-123&channelid=7"
        );
        assert_eq!(data["title"], "新年快乐");
        assert_eq!(data["pay_msg_id"], "pay-9");
        assert_eq!(data["send_id"], "send-123");
        assert_eq!(data["channel_id"], "7");

        let defaults = "<msg><appmsg><type>2001</type><wcpayinfo><nativeurl>wxpay://hongbao?sendid=private</nativeurl><receivertitle>领取红包</receivertitle></wcpayinfo></appmsg></msg>";
        let (name, data) = classify_message(&event(49, defaults));
        assert_eq!(name, "hongBao");
        assert_eq!(data["send_id"], "private");
        assert_eq!(data["channel_id"], "1");

        let qr = "<msg><appmsg><type>2001</type><wcpayinfo><paysubtype>1</paysubtype><paymsgid>pay-1</paymsgid><sendertitle>收款</sendertitle></wcpayinfo></appmsg></msg>";
        assert_eq!(classify_message(&event(49, qr)).0, "qrPay");
        let qr_receiver_title = "<msg><appmsg><type>2001</type><wcpayinfo><receivertitle>收款</receivertitle><sendertitle>扫码付款</sendertitle><paymsgid>pay-2</paymsgid></wcpayinfo></appmsg></msg>";
        assert_eq!(classify_message(&event(49, qr_receiver_title)).0, "qrPay");
    }

    #[test]
    fn transfer_and_qr_payments_are_distinguished_from_links_and_red_packets() {
        let transfer = "<msg><appmsg><type>2000</type><wcpayinfo><transferid>tr-1</transferid><transcationid>tx-1</transcationid><feedesc>￥1.00</feedesc><paysubtype>1</paysubtype><invalidtime>1730000000</invalidtime><transferattach>attach-data</transferattach></wcpayinfo></appmsg></msg>";
        let (name, data) = classify_message(&event(49, transfer));
        assert_eq!(name, "transPay");
        assert_eq!(data["transfer_id"], "tr-1");
        assert_eq!(data["transaction_id"], "tx-1");
        assert_eq!(data["amount"], "￥1.00");
        assert_eq!(data["pay_subtype"], "1");
        assert_eq!(data["invalid_time"], "1730000000");
        assert_eq!(data["transfer_attach"], "attach-data");
        let qr = "<msg><appmsg><type>2001</type><wcpayinfo><paysubtype>1</paysubtype><paymsgid>pay-1</paymsgid><sendertitle>收款</sendertitle></wcpayinfo></appmsg></msg>";
        assert_eq!(classify_message(&event(49, qr)).0, "qrPay");
        assert_eq!(
            classify_message(&event(
                49,
                "<msg><appmsg><type>5</type><title>transferid</title></appmsg></msg>"
            ))
            .0,
            "recvMsg"
        );
    }

    #[test]
    fn system_messages_and_group_changes_have_distinct_names() {
        assert_eq!(
            classify_message(&event(10000, "Alice邀请Bob加入了群聊")).0,
            "groupMemberChanges"
        );
        assert_eq!(
            classify_message(&event(10000, "You changed the chat background")).0,
            "systemMsg"
        );
    }

    #[test]
    fn malformed_and_hostile_xml_never_panics_or_resolves_entities() {
        for xml in [
            "<msg><appmsg>",
            "<!DOCTYPE msg [<!ENTITY x SYSTEM 'file:///etc/passwd'>]><msg>&x;</msg>",
            "<msg><appmsg><type>5</type></msg>",
        ] {
            assert_eq!(classify_message(&event(49, xml)).0, "recvMsg");
        }
        assert!(parse_xml(&format!("{}{}", "<x>".repeat(65), "</x>".repeat(65))).is_none());
    }
}
