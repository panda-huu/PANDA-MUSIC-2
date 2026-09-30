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
		}
		log.Println("vc notice fail", id, err)
	}
}

func announceVCStarted(chatID int64) {
	chatID = canonChatID(chatID)
	if chatID == 0 {
		return
	}
	if vcDedupe(fmt.Sprintf("start:%d", chatID)) {
		return
	}
	sendGroupNotice(chatID, "<blockquote><b>"+smallcaps("voice chat started")+" 🍃</b></blockquote>")
}

func announceVCEnded(chatID int64) {
	chatID = canonChatID(chatID)
	if chatID == 0 {
		return
	}
	if vcDedupe(fmt.Sprintf("end:%d", chatID)) {
		return
	}
	sendGroupNotice(chatID, "<blockquote><b>"+smallcaps("voice chat ended.")+"🍃</b></blockquote>")
}

func userMentionByID(uid int64) string {
	if uid == 0 {
		return richEsc("User")
	}
	name, _ := userFromCache(uid, "User", "-")
	if Bot != nil {
		if u, err := Bot.GetUser(uid); err == nil && u != nil {
			if n, _, _ := userBits(u); n != "" {
				name = n
			}
		}
	}
	return mentionHTML(uid, name)
}

func announceVCInvite(chatID, byID int64, toIDs []int64) {
	chatID = canonChatID(chatID)
	if chatID == 0 || byID == 0 || len(toIDs) == 0 {
		return
	}
	if vcDedupe(fmt.Sprintf("inv:%d:%d:%v", chatID, byID, toIDs)) {
		return
	}
	var invited []string
	for _, id := range toIDs {
		if id != 0 {
			invited = append(invited, userMentionByID(id))
		}
	}
	if len(invited) == 0 {
		return
	}
	sendGroupNotice(chatID, "<blockquote>"+
		smallcaps("invited by")+": "+userMentionByID(byID)+" 🍃\n"+
		smallcaps("invited to")+" "+strings.Join(invited, ", ")+" 🍃"+
		"</blockquote>")
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
		kind := strings.ToLower(fmt.Sprintf("%T %v", m.Action, m.Action))
		if strings.Contains(kind, "invitetogroupcall") {
			return
		}
		if strings.Contains(kind, "groupcall") && !strings.Contains(kind, "scheduled") {
			if strings.Contains(kind, "duration") && !strings.Contains(kind, "duration:0") {
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
	if discarded, ok := upd.Call.(*telegram.GroupCallDiscarded); ok && discarded != nil {
		if chatID == 0 {
			if v, ok := vcActive.Load(discarded.ID); ok {
				if id, ok := v.(int64); ok {
					chatID = id
				}
			}
		}
		vcActive.Delete(discarded.ID)
		announceVCEnded(chatID)
		return nil
	}
	s := fmt.Sprintf("%v", upd.Call)
	id := int64(0)
	fmt.Sscanf(s, "%d", &id)
	if strings.Contains(strings.ToLower(fmt.Sprintf("%T", upd.Call)), "discard") {
		announceVCEnded(chatID)
		return nil
	}
	if id != 0 && chatID != 0 {
		if _, exists := vcActive.Load(id); exists {
			return nil
		}
		vcActive.Store(id, chatID)
	}
	announceVCStarted(chatID)
	return nil
}
