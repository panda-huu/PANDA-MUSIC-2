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

func welcomeFromService(m *telegram.NewMessage) {
	if m == nil || m.IsPrivate() || m.Action == nil || isVCService(m) {
		return
	}
	chatID := m.ChatID()
	if id := m.ChannelID(); id != 0 {
		chatID = id
	}
	kind := strings.ToLower(fmt.Sprintf("%T %v", m.Action, m.Action))
	if strings.Contains(kind, "invite") || strings.Contains(kind, "groupcall") {
		return
	}
	added := strings.Contains(kind, "adduser") || strings.Contains(kind, "joined")
	if !added {
		return
	}
	switch a := m.Action.(type) {
	case *telegram.MessageActionChatAddUser:
		for _, id := range a.Users {
			name, uname := userFromCache(id, "User", "-")
			welcomeUserJoin(chatID, id, name, uname)
		}
		return
	case *telegram.MessageActionChatJoinedByLink:
		welcomeUserJoin(chatID, userIDOf(m), senderFullName(m), senderUsername(m))
		return
	}
	if userIDOf(m) != 0 && userIDOf(m) != botSelfID() {
		welcomeUserJoin(chatID, userIDOf(m), senderFullName(m), senderUsername(m))
	}
}
