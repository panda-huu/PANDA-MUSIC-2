package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func isVCService(m *telegram.NewMessage) bool {
	if m == nil || m.Action == nil {
		return false
	}
	kind := strings.ToLower(fmt.Sprintf("%T", m.Action))
	return strings.Contains(kind, "groupcall") || strings.Contains(kind, "invitetogroupcall")
}

func handleServiceMessage(m *telegram.NewMessage) error {
	if m == nil || m.IsPrivate() {
		return nil
	}
	handleVoiceChatService(m)
	if !isVCService(m) {
		welcomeFromService(m)
	}
	if !m.IsService() || m.Action == nil {
		return nil
	}
	self := botSelfID()
	if self == 0 {
		return nil
	}
	kind := strings.ToLower(fmt.Sprintf("%T %v", m.Action, m.Action))
	hasBot := strings.Contains(kind, fmt.Sprintf("%d", self))
	added := strings.Contains(kind, "adduser") || strings.Contains(kind, "joined")
	kicked := strings.Contains(kind, "deleteuser") || strings.Contains(kind, "kick")
	if !hasBot && userIDOf(m) != self {
		return nil
	}
	if !added && !kicked {
		return nil
	}
	log.Println("service membership", kind)
	chatID := canonChatID(m.ChatID())
	if m.Channel != nil && strings.TrimSpace(m.Channel.Title) != "" {
		rememberGroup(chatID, m.Channel.Title, m.Channel.Username, "")
	} else if m.Chat != nil && strings.TrimSpace(m.Chat.Title) != "" {
		rememberGroup(chatID, m.Chat.Title, "", "")
	}
	if added {
		addServedChat(chatID)
		addBroadcastChat(chatID, "group")
		go refreshGroupMeta(chatID)
	}
	go logBotMembership(chatID, userIDOf(m), senderFullName(m), senderUsername(m), kicked)
	return nil
}
