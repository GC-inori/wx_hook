package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	firstUserID    int64 = 100000001
	firstGroupID   int64 = 200000001
	firstMessageID int32 = 1000001
)

var cqAtIDPattern = regexp.MustCompile(`(\[CQ:at,qq=)([^,\]]+)`)

// IDMapper translates WeChat identifiers to stable numeric OneBot identifiers.
// The mapping tables are protected by one mutex so that forward and reverse
// indexes are updated and persisted as a single operation.
type IDMapper struct {
	mu sync.RWMutex

	users           map[string]int64
	usersReverse    map[int64]string
	groups          map[string]int64
	groupsReverse   map[int64]string
	messages        map[string]int32
	messagesReverse map[int32]string
	messageContexts map[string]revokeMessageContext

	nextUserID    int64
	nextGroupID   int64
	nextMessageID int32
	rootDir       string
	filePath      string
	lastErr       error
}

type revokeMessageContext struct {
	ToUser      string
	ClientMsgID string
	SvrMsgID    string
	CreateTime  string
}

type idMappingFile struct {
	Version       int              `json:"version"`
	Users         map[string]int64 `json:"users"`
	Groups        map[string]int64 `json:"groups"`
	Messages      map[string]int32 `json:"messages"`
	NextUserID    int64            `json:"next_user_id"`
	NextGroupID   int64            `json:"next_group_id"`
	NextMessageID int32            `json:"next_message_id"`
}

// NewIDMapper creates a mapper and loads rootDir/id_mapping.json when rootDir
// is non-empty. An empty root directory keeps mappings in memory only.
func NewIDMapper(rootDir string) (*IDMapper, error) {
	mapper := newIDMapper()
	if strings.TrimSpace(rootDir) != "" {
		if err := mapper.SetRootDir(rootDir); err != nil {
			return nil, err
		}
	}
	return mapper, nil
}

func newIDMapper() *IDMapper {
	return &IDMapper{
		users: make(map[string]int64), usersReverse: make(map[int64]string),
		groups: make(map[string]int64), groupsReverse: make(map[int64]string),
		messages: make(map[string]int32), messagesReverse: make(map[int32]string),
		messageContexts: make(map[string]revokeMessageContext),
		nextUserID:      firstUserID, nextGroupID: firstGroupID, nextMessageID: firstMessageID,
	}
}

// SetRootDir loads the persisted tables from rootDir/id_mapping.json. It is
// safe to call while the mapper is in use; subsequent calls for the same root
// are no-ops.
func (m *IDMapper) SetRootDir(rootDir string) error {
	if m == nil {
		return errors.New("ID mapper is nil")
	}
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil
	}
	absolute, err := filepath.Abs(rootDir)
	if err != nil {
		return fmt.Errorf("resolve ID mapping directory: %w", err)
	}
	absolute = filepath.Clean(absolute)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rootDir == absolute {
		return nil
	}
	path := filepath.Join(absolute, "id_mapping.json")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		m.lastErr = fmt.Errorf("read ID mapping file %s: %w", path, err)
		return m.lastErr
	}
	if err == nil {
		var saved idMappingFile
		if err := json.Unmarshal(data, &saved); err != nil {
			m.lastErr = fmt.Errorf("parse ID mapping file %s: %w", path, err)
			return m.lastErr
		}
		if err := m.loadFileLocked(saved); err != nil {
			m.lastErr = fmt.Errorf("load ID mapping file %s: %w", path, err)
			return m.lastErr
		}
	}
	m.rootDir = absolute
	m.filePath = path
	m.lastErr = nil
	return nil
}

// PersistenceError returns the most recent persistence error, if any.
func (m *IDMapper) PersistenceError() error {
	if m == nil {
		return errors.New("ID mapper is nil")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastErr
}

// UserWxidToNum returns the stable numeric OneBot ID for a WeChat user.
func (m *IDMapper) UserWxidToNum(wxid string) int64 {
	if m == nil || strings.TrimSpace(wxid) == "" {
		return 0
	}
	wxid = strings.TrimSpace(wxid)
	if id, err := strconv.ParseInt(wxid, 10, 64); err == nil {
		return id
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.users[wxid]; ok {
		return id
	}
	id, ok := m.allocateUserIDLocked()
	if !ok {
		return 0
	}
	m.users[wxid], m.usersReverse[id] = id, wxid
	m.persistAfterMutationLocked()
	return id
}

// NumToUserWxid returns the original WeChat user ID, or the decimal input if
// the numeric ID is not present in the mapper.
func (m *IDMapper) NumToUserWxid(id int64) string {
	if m == nil {
		return strconv.FormatInt(id, 10)
	}
	m.mu.RLock()
	wxid := m.usersReverse[id]
	m.mu.RUnlock()
	if wxid != "" {
		return wxid
	}
	return strconv.FormatInt(id, 10)
}

// UserIDToWxid reverses a numeric OneBot user ID when it is known.
func (m *IDMapper) UserIDToWxid(value string) string {
	value = strings.TrimSpace(value)
	if id, err := strconv.ParseInt(value, 10, 64); err == nil {
		return m.NumToUserWxid(id)
	}
	return value
}

// GroupChatroomToNum returns the stable numeric OneBot ID for a WeChat group.
func (m *IDMapper) GroupChatroomToNum(chatroom string) int64 {
	if m == nil || strings.TrimSpace(chatroom) == "" {
		return 0
	}
	chatroom = strings.TrimSpace(chatroom)
	if id, err := strconv.ParseInt(chatroom, 10, 64); err == nil {
		return id
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.groups[chatroom]; ok {
		return id
	}
	id, ok := m.allocateGroupIDLocked()
	if !ok {
		return 0
	}
	m.groups[chatroom], m.groupsReverse[id] = id, chatroom
	m.persistAfterMutationLocked()
	return id
}

// NumToGroupChatroom returns the original group ID, or the decimal input when
// the numeric ID is unknown.
func (m *IDMapper) NumToGroupChatroom(id int64) string {
	if m == nil {
		return strconv.FormatInt(id, 10)
	}
	m.mu.RLock()
	chatroom := m.groupsReverse[id]
	m.mu.RUnlock()
	if chatroom != "" {
		return chatroom
	}
	return strconv.FormatInt(id, 10)
}

// GroupIDToChatroom reverses a numeric OneBot group ID when it is known.
func (m *IDMapper) GroupIDToChatroom(value string) string {
	value = strings.TrimSpace(value)
	if id, err := strconv.ParseInt(value, 10, 64); err == nil {
		return m.NumToGroupChatroom(id)
	}
	return value
}

// MessageIDToNum maps a native message identifier to the signed 32-bit ID
// expected by strict OneBot clients. The original value remains reversible.
func (m *IDMapper) MessageIDToNum(value any) int32 {
	if m == nil {
		return 0
	}
	key := idText(value)
	if key == "" {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.messages[key]; ok {
		return id
	}
	id, ok := m.allocateMessageIDLocked()
	if !ok {
		return 0
	}
	m.messages[key], m.messagesReverse[id] = id, key
	m.persistAfterMutationLocked()
	return id
}

// NumToMessageID returns the original native message ID, or a decimal string
// for an unmapped numeric ID.
func (m *IDMapper) NumToMessageID(id int32) string {
	if m == nil {
		return strconv.FormatInt(int64(id), 10)
	}
	m.mu.RLock()
	messageID := m.messagesReverse[id]
	m.mu.RUnlock()
	if messageID != "" {
		return messageID
	}
	return strconv.FormatInt(int64(id), 10)
}

// MessageIDToNative reverses a numeric OneBot message ID when it is known.
func (m *IDMapper) MessageIDToNative(value string) string {
	value = strings.TrimSpace(value)
	if id, err := strconv.ParseInt(value, 10, 32); err == nil {
		return m.NumToMessageID(int32(id))
	}
	return value
}

func (m *IDMapper) rememberRevokeMessage(nativeID string, context revokeMessageContext) {
	if m == nil || strings.TrimSpace(nativeID) == "" || context.ToUser == "" {
		return
	}
	m.mu.Lock()
	m.messageContexts[nativeID] = context
	m.mu.Unlock()
}

func (m *IDMapper) revokeMessage(nativeID string) (revokeMessageContext, bool) {
	if m == nil {
		return revokeMessageContext{}, false
	}
	m.mu.RLock()
	context, ok := m.messageContexts[nativeID]
	m.mu.RUnlock()
	return context, ok
}

// TranslateEvent returns a translated copy of a formatted OneBot event.
func (m *IDMapper) TranslateEvent(event map[string]any) map[string]any {
	if m == nil || event == nil {
		return event
	}
	copy, ok := cloneJSONMap(event)
	if !ok {
		return event
	}
	translated, _ := m.translateValue(copy, translateForward, "").(map[string]any)
	return translated
}

// TranslateActionParams changes virtual numeric IDs back to native WeChat IDs.
// Invalid or non-object JSON is returned unchanged for the caller to validate.
func (m *IDMapper) TranslateActionParams(action string, raw json.RawMessage) json.RawMessage {
	if m == nil || len(raw) == 0 || !json.Valid(raw) {
		return raw
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return raw
	}
	if _, ok := value.(map[string]any); !ok {
		return raw
	}
	translated := m.translateValue(value, translateReverse, "")
	encoded, err := json.Marshal(translated)
	if err != nil {
		return raw
	}
	return encoded
}

// TranslateActionResponseData converts WeChat IDs in a OneBot action result to
// numeric IDs while retaining the response's original JSON-compatible shape.
func (m *IDMapper) TranslateActionResponseData(action string, data any) any {
	if m == nil || data == nil {
		return data
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return data
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return data
	}
	return m.translateValue(value, translateForward, "")
}

type translationDirection uint8

const (
	translateForward translationDirection = iota
	translateReverse
)

func (m *IDMapper) translateValue(value any, direction translationDirection, key string) any {
	switch typed := value.(type) {
	case map[string]any:
		segmentType, _ := typed["type"].(string)
		if segmentType == "reply" {
			if data, ok := typed["data"].(map[string]any); ok {
				if id, exists := data["id"]; exists {
					data["id"] = m.translateID(id, direction, "message_id")
				}
			}
		}
		for childKey, child := range typed {
			if segmentType == "reply" && childKey == "id" {
				typed[childKey] = m.translateID(child, direction, "message_id")
				continue
			}
			switch childKey {
			case "user_list", "member_list", "users":
				typed[childKey] = m.translateUserList(child, direction)
			default:
				typed[childKey] = m.translateValue(child, direction, childKey)
			}
		}
		return typed
	case []any:
		for i := range typed {
			typed[i] = m.translateValue(typed[i], direction, key)
		}
		return typed
	case string:
		if key == "message" || key == "raw_message" {
			return m.translateCQAt(typed, direction)
		}
		switch key {
		case "self_id", "user_id", "wxid", "user_wxid", "member_wxid", "from_wxid", "sender_wxid", "account", "group_id", "group_wxid", "chatroom_id", "chatroom_name", "to_user", "session", "message_id", "msg_id", "sequence":
			return m.translateID(typed, direction, key)
		}
		if key == "qq" && typed != "all" {
			return m.translateID(typed, direction, "user_id")
		}
		if direction == translateForward && strings.HasSuffix(typed, "@chatroom") {
			return m.GroupChatroomToNum(typed)
		}
		return typed
	case json.Number:
		return m.translateID(typed, direction, key)
	default:
		return value
	}
}

func (m *IDMapper) translateUserList(value any, direction translationDirection) any {
	switch typed := value.(type) {
	case []any:
		for i := range typed {
			typed[i] = m.translateID(typed[i], direction, "user_id")
		}
		return typed
	case string:
		parts := strings.Split(typed, ",")
		for i := range parts {
			parts[i] = fmt.Sprint(m.translateID(strings.TrimSpace(parts[i]), direction, "user_id"))
		}
		return strings.Join(parts, ",")
	default:
		return m.translateValue(value, direction, "user_id")
	}
}

func (m *IDMapper) translateID(value any, direction translationDirection, key string) any {
	text := idText(value)
	if text == "" {
		return value
	}
	switch key {
	case "to_user", "session":
		if direction == translateForward {
			if strings.HasSuffix(text, "@chatroom") {
				return m.GroupChatroomToNum(text)
			}
			return m.UserWxidToNum(text)
		}
		if id, ok := numericID(value); ok {
			m.mu.RLock()
			chatroom := m.groupsReverse[id]
			wxid := m.usersReverse[id]
			m.mu.RUnlock()
			if chatroom != "" {
				return chatroom
			}
			if wxid != "" {
				return wxid
			}
		}
	case "self_id", "user_id", "wxid", "user_wxid", "member_wxid", "from_wxid", "sender_wxid", "account", "qq":
		if direction == translateForward {
			return m.UserWxidToNum(text)
		}
		if id, ok := numericID(value); ok {
			m.mu.RLock()
			wxid, exists := m.usersReverse[id]
			m.mu.RUnlock()
			if exists {
				return wxid
			}
		}
	case "group_id", "group_wxid", "chatroom_id", "chatroom_name":
		if direction == translateForward {
			return m.GroupChatroomToNum(text)
		}
		if id, ok := numericID(value); ok {
			m.mu.RLock()
			chatroom, exists := m.groupsReverse[id]
			m.mu.RUnlock()
			if exists {
				return chatroom
			}
		}
	case "message_id", "msg_id", "sequence":
		if direction == translateForward {
			return m.MessageIDToNum(value)
		}
		if id, ok := numericMessageID(value); ok {
			m.mu.RLock()
			messageID, exists := m.messagesReverse[id]
			m.mu.RUnlock()
			if exists {
				return messageID
			}
		}
	}
	return value
}

func (m *IDMapper) translateCQAt(value string, direction translationDirection) string {
	return cqAtIDPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := cqAtIDPattern.FindStringSubmatch(match)
		if len(parts) != 3 || parts[2] == "all" {
			return match
		}
		if direction == translateForward {
			return parts[1] + strconv.FormatInt(m.UserWxidToNum(parts[2]), 10)
		}
		if id, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			m.mu.RLock()
			wxid, exists := m.usersReverse[id]
			m.mu.RUnlock()
			if exists {
				return parts[1] + wxid
			}
		}
		return match
	})
}

func (m *IDMapper) allocateUserIDLocked() (int64, bool) {
	id := m.nextUserID
	if id < firstUserID {
		id = firstUserID
	}
	for {
		if _, exists := m.usersReverse[id]; !exists {
			if id == math.MaxInt64 {
				m.nextUserID = firstUserID
			} else {
				m.nextUserID = id + 1
			}
			return id, true
		}
		if id == math.MaxInt64 {
			return 0, false
		}
		id++
	}
}

func (m *IDMapper) allocateGroupIDLocked() (int64, bool) {
	id := m.nextGroupID
	if id < firstGroupID {
		id = firstGroupID
	}
	for {
		if _, exists := m.groupsReverse[id]; !exists {
			if id == math.MaxInt64 {
				m.nextGroupID = firstGroupID
			} else {
				m.nextGroupID = id + 1
			}
			return id, true
		}
		if id == math.MaxInt64 {
			return 0, false
		}
		id++
	}
}

func (m *IDMapper) allocateMessageIDLocked() (int32, bool) {
	id := m.nextMessageID
	if id < firstMessageID || id < 0 {
		id = firstMessageID
	}
	for {
		if _, exists := m.messagesReverse[id]; !exists {
			if id == math.MaxInt32 {
				m.nextMessageID = firstMessageID
			} else {
				m.nextMessageID = id + 1
			}
			return id, true
		}
		if id == math.MaxInt32 {
			return 0, false
		}
		id++
	}
}

func (m *IDMapper) persistAfterMutationLocked() {
	if err := m.persistLocked(); err != nil {
		m.lastErr = err
		log.Printf("persist OneBot ID mappings: %v", err)
		return
	}
	m.lastErr = nil
}

func (m *IDMapper) persistLocked() error {
	if m.filePath == "" {
		return nil
	}
	state := idMappingFile{
		Version: 1, Users: m.users, Groups: m.groups, Messages: m.messages,
		NextUserID: m.nextUserID, NextGroupID: m.nextGroupID, NextMessageID: m.nextMessageID,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ID mappings: %w", err)
	}
	if err := os.MkdirAll(m.rootDir, 0750); err != nil {
		return fmt.Errorf("create ID mapping directory: %w", err)
	}
	temporary, err := os.CreateTemp(m.rootDir, ".id_mapping-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary ID mapping file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set ID mapping file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary ID mapping file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary ID mapping file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary ID mapping file: %w", err)
	}
	if err := os.Rename(temporaryPath, m.filePath); err != nil {
		return fmt.Errorf("replace ID mapping file: %w", err)
	}
	return nil
}

func (m *IDMapper) loadFileLocked(saved idMappingFile) error {
	usersReverse := make(map[int64]string, len(saved.Users))
	for wxid, id := range saved.Users {
		if wxid == "" || id <= 0 {
			return fmt.Errorf("invalid user mapping for %q", wxid)
		}
		if previous, exists := usersReverse[id]; exists && previous != wxid {
			return fmt.Errorf("duplicate user ID %d for %q and %q", id, previous, wxid)
		}
		usersReverse[id] = wxid
	}
	groupsReverse := make(map[int64]string, len(saved.Groups))
	for chatroom, id := range saved.Groups {
		if chatroom == "" || id <= 0 {
			return fmt.Errorf("invalid group mapping for %q", chatroom)
		}
		if previous, exists := groupsReverse[id]; exists && previous != chatroom {
			return fmt.Errorf("duplicate group ID %d for %q and %q", id, previous, chatroom)
		}
		groupsReverse[id] = chatroom
	}
	messagesReverse := make(map[int32]string, len(saved.Messages))
	for messageID, id := range saved.Messages {
		if messageID == "" || id <= 0 {
			return fmt.Errorf("invalid message mapping for %q", messageID)
		}
		if previous, exists := messagesReverse[id]; exists && previous != messageID {
			return fmt.Errorf("duplicate message ID %d for %q and %q", id, previous, messageID)
		}
		messagesReverse[id] = messageID
	}
	m.users, m.usersReverse = saved.Users, usersReverse
	m.groups, m.groupsReverse = saved.Groups, groupsReverse
	m.messages, m.messagesReverse = saved.Messages, messagesReverse
	if m.users == nil {
		m.users = make(map[string]int64)
	}
	if m.groups == nil {
		m.groups = make(map[string]int64)
	}
	if m.messages == nil {
		m.messages = make(map[string]int32)
	}
	m.nextUserID = nextInt64ID(saved.NextUserID, firstUserID, usersReverse)
	m.nextGroupID = nextInt64ID(saved.NextGroupID, firstGroupID, groupsReverse)
	m.nextMessageID = nextInt32ID(saved.NextMessageID, firstMessageID, messagesReverse)
	return nil
}

func nextInt64ID(saved, base int64, reverse map[int64]string) int64 {
	if saved < base {
		saved = base
	}
	for id := range reverse {
		if id >= saved && id < math.MaxInt64 {
			saved = id + 1
		}
	}
	return saved
}

func nextInt32ID(saved, base int32, reverse map[int32]string) int32 {
	if saved < base {
		saved = base
	}
	for id := range reverse {
		if id >= saved && id < math.MaxInt32 {
			saved = id + 1
		}
	}
	return saved
}

func cloneJSONMap(value map[string]any) (map[string]any, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var copy map[string]any
	if err := decoder.Decode(&copy); err != nil || copy == nil {
		return nil, false
	}
	return copy, true
}

func idText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case json.RawMessage:
		var text string
		if json.Unmarshal(typed, &text) == nil {
			return strings.TrimSpace(text)
		}
		return strings.TrimSpace(string(typed))
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		return fmt.Sprint(value)
	}
}

func numericID(value any) (int64, bool) {
	text := idText(value)
	if text == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(text, 10, 64)
	return id, err == nil
}

func numericMessageID(value any) (int32, bool) {
	id, ok := numericID(value)
	if !ok || id < math.MinInt32 || id > math.MaxInt32 {
		return 0, false
	}
	return int32(id), true
}
