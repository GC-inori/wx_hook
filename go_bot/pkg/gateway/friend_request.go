package gateway

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"wxbot/pkg/client"
)

const (
	friendRequestTTL = 24 * time.Hour
	friendRequestMax = 1024
	friendAddedTTL   = time.Hour
)

type cachedFriendRequest struct {
	Wxid            string
	Ticket          string
	EncryptUsername string
	Scene           uint32
	CachedAt        time.Time
}

func (s *Server) cacheFriendRequest(event client.Event) (cachedFriendRequest, bool) {
	fields := rawObject(event.Data)
	applicant := rawObject(fields["applicant"])
	ticket := firstRawString(applicant, "ticket", "flag")
	flag := firstRawString(fields, "flag", "ticket")
	if ticket == "" {
		ticket = flag
	}
	if flag == "" {
		flag = ticket
	}
	wxid := firstRawString(applicant, "wxid", "user_id")
	if wxid == "" {
		wxid = firstRawString(fields, "final_from_wxid", "sender", "user_id", "wxid")
	}
	encryptusername := firstRawString(applicant, "encryptusername", "encrypt_username")
	if encryptusername == "" {
		encryptusername = firstRawString(fields, "encryptusername", "encrypt_username")
	}
	request := cachedFriendRequest{
		Wxid:            wxid,
		Ticket:          ticket,
		EncryptUsername: encryptusername,
		Scene:           sceneFromRaw(applicant["scene"], sceneFromRaw(fields["scene"], 6)),
		CachedAt:        time.Now(),
	}
	if request.Wxid == "" || request.Ticket == "" {
		return request, false
	}

	keys := []string{ticket}
	if flag != "" && flag != ticket {
		keys = append(keys, flag)
	}
	s.friendRequestMu.Lock()
	defer s.friendRequestMu.Unlock()
	if s.friendRequests == nil {
		s.friendRequests = make(map[string]cachedFriendRequest)
	}
	for key, item := range s.friendRequests {
		if time.Since(item.CachedAt) > friendRequestTTL {
			delete(s.friendRequests, key)
		}
	}
	for _, key := range keys {
		s.friendRequests[key] = request
	}
	for len(s.friendRequests) > friendRequestMax {
		oldestKey := ""
		oldestAt := time.Now()
		for key, item := range s.friendRequests {
			if item.CachedAt.Before(oldestAt) {
				oldestKey, oldestAt = key, item.CachedAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(s.friendRequests, oldestKey)
	}
	return request, true
}

func (s *Server) cachedFriendRequest(flag string) (cachedFriendRequest, bool) {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return cachedFriendRequest{}, false
	}
	s.friendRequestMu.Lock()
	defer s.friendRequestMu.Unlock()
	request, ok := s.friendRequests[flag]
	if !ok {
		return cachedFriendRequest{}, false
	}
	if time.Since(request.CachedAt) > friendRequestTTL {
		delete(s.friendRequests, flag)
		return cachedFriendRequest{}, false
	}
	return request, true
}

func rawParamScene(raw json.RawMessage) (uint32, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	scene, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err == nil {
		return uint32(scene), true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		scene, err = strconv.ParseUint(strings.TrimSpace(text), 10, 32)
		if err == nil {
			return uint32(scene), true
		}
	}
	return 0, false
}

func sceneFromRaw(raw json.RawMessage, fallback uint32) uint32 {
	if scene, ok := rawParamScene(raw); ok {
		return scene
	}
	return fallback
}

func (s *Server) autoAgreeFriend(request cachedFriendRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), s.requestTimeout())
	defer cancel()
	_, _, _, err := s.DispatchAction(ctx, "agreeFriendReq", marshalParams(map[string]any{
		"wxid":            request.Wxid,
		"ticket":          request.Ticket,
		"encryptusername": request.EncryptUsername,
		"scene":           request.Scene,
		"remark":          "",
	}))
	if err != nil {
		log.Printf("automatically agree to friend request from %q: %v", request.Wxid, err)
		return
	}
	s.publishFriendAdded(request.Wxid)
}

func friendAddedSystemMessage(event client.Event, selfID string) (string, bool) {
	if event.Event != "systemMsg" && event.Event != "recvMsg" {
		return "", false
	}
	fields := rawObject(event.Data)
	if firstRawString(fields, "msg_type") != "10000" {
		return "", false
	}
	message := firstRawString(fields, "notification", "text", "msg")
	if !strings.Contains(message, "你已添加了") && !strings.Contains(message, "现在可以开始聊天了") {
		return "", false
	}
	for _, key := range []string{"final_from_wxid", "sender", "sender_wxid", "sender_id", "wxid", "from_wxid", "receiver"} {
		wxid := firstRawString(fields, key)
		if wxid == "" || wxid == selfID || strings.HasSuffix(wxid, "@chatroom") {
			continue
		}
		return wxid, true
	}
	return "", true
}

func (s *Server) publishFriendAdded(wxid string) {
	wxid = strings.TrimSpace(wxid)
	if wxid != "" {
		now := time.Now()
		s.friendRequestMu.Lock()
		if s.friendAddedAt == nil {
			s.friendAddedAt = make(map[string]time.Time)
		}
		for id, addedAt := range s.friendAddedAt {
			if now.Sub(addedAt) > friendAddedTTL {
				delete(s.friendAddedAt, id)
			}
		}
		if addedAt, exists := s.friendAddedAt[wxid]; exists && now.Sub(addedAt) <= friendAddedTTL {
			s.friendRequestMu.Unlock()
			return
		}
		s.friendAddedAt[wxid] = now
		s.friendRequestMu.Unlock()
	}

	data, _ := json.Marshal(map[string]string{"wxid": wxid})
	s.Publish(client.Event{Event: "friendAdded", Data: data})

	cfg := s.currentConfig().Policy
	message := cfg.FriendWelcomeMessage
	if !cfg.FriendWelcomeEnabled || wxid == "" || strings.TrimSpace(message) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.requestTimeout())
		defer cancel()
		_, _, _, err := s.DispatchAction(ctx, "sendText", marshalParams(map[string]string{
			"target": wxid,
			"text":   message,
		}))
		if err != nil {
			log.Printf("send friend welcome message to %q: %v", wxid, err)
		}
	}()
}
