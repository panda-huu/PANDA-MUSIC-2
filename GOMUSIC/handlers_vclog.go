package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

var (
	vcOnce   sync.Map
	vcActive sync.Map
	reVCUser = regexp.MustCompile(`\b(\d{6,})\b`)
)

func registerVoiceChatHandlers() {
	if Bot == nil {
		return
	}
	Bot.On(telegram.OnAction, func(m *telegram.NewMessage) error {
		handleVoiceChatService(m)
		return nil
	})
	Bot.AddRawHandler(&telegram.UpdateGroupCall{}, handleRawGroupCall)
}

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
			log.Println("vc notice sent", id)
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
	sendGroupNotice(chatID, "<blockquote><b>"+smallcaps("voice chat started")+" 🥀</b></blockquote>")
}

func announceVCEnded(chatID int64) {
	chatID = canonChatID(chatID)
	if chatID == 0 {
		return
	}
	if vcDedupe(fmt.Sprintf("end:%d", chatID)) {
		return
	}
	sendGroupNotice(chatID, "<blockquote><b>"+smallcaps("voice chat ended.")+"🥀</b></blockquote>")
}

func userMentionByID(uid int64) string {
	if uid == 0 {
		return richEsc("User")
	}
	name, _ := userFromCache(uid, "User", "-")
	if Bot != nil {
		if u, err := Bot.GetUser(uid); err == nil && u != nil {
			if n, _, _ := userBits(u); n != "" && n != "User" {
				name = n
			}
		}
	}
	return mentionHTML(uid, name)
}

func announceVCInvite(chatID, byID int64, toIDs []int64) {
	chatID = canonChatID(chatID)
	if chatID == 0 || len(toIDs) == 0 {
		return
	}
	if vcDedupe(fmt.Sprintf("inv:%d:%d:%v", chatID, byID, toIDs)) {
		return
	}
	var invited []string
	for _, id := range toIDs {
		if id != 0 && id != byID {
			invited = append(invited, userMentionByID(id))
		}
	}
	if len(invited) == 0 {
		return
	}
	by := userMentionByID(byID)
	if byID == 0 {
		by = richEsc("User")
	}
	sendGroupNotice(chatID, "<blockquote>"+
		smallcaps("invited by")+": "+by+" 🥀\n"+
		smallcaps("invited to")+" "+strings.Join(invited, ", ")+" 🥀"+
		"</blockquote>")
}

func idsFromAction(v any, skip int64) []int64 {
	if v == nil {
		return nil
	}
	seen := map[int64]bool{}
	var out []int64
	for _, m := range reVCUser.FindAllStringSubmatch(fmt.Sprintf("%v", v), -1) {
		if len(m) < 2 {
			continue
		}
		var id int64
		fmt.Sscanf(m[1], "%d", &id)
		if id < 100000 || id == skip || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func handleVoiceChatService(m *telegram.NewMessage) {
	if m == nil || m.IsPrivate() || m.Action == nil {
		return
	}
	chatID := m.ChatID()
	if id := m.ChannelID(); id != 0 {
		chatID = id
	}
	kind := strings.ToLower(fmt.Sprintf("%T %v", m.Action, m.Action))
	log.Println("vc service", kind)
	switch a := m.Action.(type) {
	case *telegram.MessageActionGroupCall:
		if a.Duration > 0 {
			announceVCEnded(chatID)
		} else {
			announceVCStarted(chatID)
		}
		return
	case *telegram.MessageActionInviteToGroupCall:
		ids := a.Users
		if len(ids) == 0 {
			ids = idsFromAction(a, userIDOf(m))
		}
		announceVCInvite(chatID, userIDOf(m), ids)
		return
	}
	if strings.Contains(kind, "invitetogroupcall") || (strings.Contains(kind, "invite") && strings.Contains(kind, "call")) {
		announceVCInvite(chatID, userIDOf(m), idsFromAction(m.Action, userIDOf(m)))
		return
	}
	if strings.Contains(kind, "groupcall") && !strings.Contains(kind, "scheduled") {
		if strings.Contains(kind, "duration") && !strings.Contains(kind, "duration:0") && !strings.Contains(kind, "duration 0") {
			announceVCEnded(chatID)
		} else {
			announceVCStarted(chatID)
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
	log.Println("vc raw call", fmt.Sprintf("%T", upd.Call), "chat", chatID)
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
	kind := strings.ToLower(fmt.Sprintf("%T", upd.Call))
	if strings.Contains(kind, "discard") {
		announceVCEnded(chatID)
		return nil
	}
	if chatID != 0 {
		if _, exists := vcActive.Load(chatID); exists {
			return nil
		}
		vcActive.Store(chatID, chatID)
	}
	announceVCStarted(chatID)
	return nil
}
