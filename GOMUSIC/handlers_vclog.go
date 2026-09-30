package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

var (
	vcOnce   sync.Map
	vcActive sync.Map
)

func vcDedupe(key string) bool {
	if v, ok := vcOnce.Load(key); ok {
		if t, ok := v.(time.Time); ok && time.Since(t) < 8*time.Second {
			return true
		}
	}
	vcOnce.Store(key, time.Now())
	return false
}

func sendGroupNotice(chatID int64, html string) {
	if chatID == 0 || !acceptUpdates.Load() {
		return
	}
	for _, id := range welcomeLookupIDs(chatID) {
		if _, err := sendHTML(Bot, id, html, nil); err == nil {
			return
		} else {
			log.Println("vc notice fail", id, err)
		}
	}
}

func announceVCStarted(chatID int64) {
	chatID = canonChatID(chatID)
	key := fmt.Sprintf("start:%d", chatID)
	if vcDedupe(key) {
		return
	}
	body := "<blockquote><b>" + smallcaps("voice chat started") + " 🍃</b></blockquote>"
	sendGroupNotice(chatID, body)
}

func announceVCEnded(chatID int64) {
	chatID = canonChatID(chatID)
	key := fmt.Sprintf("end:%d", chatID)
	if vcDedupe(key) {
		return
	}
	body := "<blockquote><b>" + smallcaps("voice chat ended.") + "🍃</b></blockquote>"
	sendGroupNotice(chatID, body)
}

func userMentionByID(uid int64) string {
	if uid == 0 {
		return richEsc("User")
	}
	name, _ := userFromCache(uid, "User", "-")
	if Bot != nil {
		if u, err := Bot.GetUser(uid); err == nil && u != nil {
			n, _, _ := userBits(u)
			if n != "" {
				name = n
			}
		}
	}
	return mentionHTML(uid, name)
}

func announceVCInvite(chatID, byID int64, toIDs []int64) {
	chatID = canonChatID(chatID)
	if byID == 0 || len(toIDs) == 0 {
		return
	}
	key := fmt.Sprintf("inv:%d:%d:%v", chatID, byID, toIDs)
	if vcDedupe(key) {
		return
	}
	var invited []string
	for _, id := range toIDs {
		if id == 0 {
			continue
		}
		invited = append(invited, userMentionByID(id))
	}
	if len(invited) == 0 {
		return
	}
	body := "<blockquote>" +
		smallcaps("invited by") + ": " + userMentionByID(byID) + " 🍃\n" +
		smallcaps("invited to") + " " + strings.Join(invited, ", ") + " 🍃" +
		"</blockquote>"
	sendGroupNotice(chatID, body)
}

func handleVoiceChatService(m *telegram.NewMessage) {
	if m == nil || m.IsPrivate() || m.Action == nil {
		return
	}
	chatID := m.ChatID()
	if id := m.ChannelID(); id != 0 {
		chatID = id
	}
	switch a := m.Action.(type) {
	case *telegram.MessageActionGroupCall:
		if a.Duration > 0 {
			announceVCEnded(chatID)
		} else {
			announceVCStarted(chatID)
		}
	case *telegram.MessageActionInviteToGroupCall:
		announceVCInvite(chatID, userIDOf(m), a.Users)
	default:
		kind := strings.ToLower(fmt.Sprintf("%T", m.Action))
		if strings.Contains(kind, "invitetogroupcall") {
			announceVCInvite(chatID, userIDOf(m), nil)
		} else if strings.Contains(kind, "groupcall") && !strings.Contains(kind, "scheduled") {
			s := strings.ToLower(fmt.Sprintf("%v", m.Action))
			if strings.Contains(s, "duration") && !strings.Contains(s, "duration:0") {
				announceVCEnded(chatID)
			} else {
				announceVCStarted(chatID)
			}
		}
	}
}

func peerToChatID(p telegram.Peer) int64 {
	if p == nil {
		return 0
	}
	switch x := p.(type) {
	case *telegram.PeerChannel:
		return canonChatID(-1000000000000 - x.ChannelID)
	case *telegram.PeerChat:
		return -x.ChatID
	}
	return 0
}

func handleRawGroupCall(u telegram.Update, c *telegram.Client) error {
	upd, ok := u.(*telegram.UpdateGroupCall)
	if !ok || upd == nil {
		return nil
	}
	chatID := peerToChatID(upd.Peer)
	switch call := upd.Call.(type) {
	case *telegram.GroupCallDiscarded:
		if chatID == 0 {
			if v, ok := vcActive.Load(call.ID); ok {
				if id, ok := v.(int64); ok {
					chatID = id
				}
			}
		}
		vcActive.Delete(call.ID)
		announceVCEnded(chatID)
	default:
		id := int64(0)
		if gc, ok := upd.Call.(*telegram.GroupCall);
		ok && gc != nil {
			id = gc.ID
		}
		if id != 0 && chatID != 0 {
			if _, exists := vcActive.Load(id); exists {
				return nil
			}
			vcActive.Store(id, chatID)
		}
		announceVCStarted(chatID)
	}
	return nil
}
