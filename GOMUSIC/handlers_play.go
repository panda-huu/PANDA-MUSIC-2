package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func cmdRoot(m *telegram.NewMessage) string {
	if m == nil {
		return ""
	}
	t := strings.TrimSpace(m.Text())
	if t == "" || !strings.HasPrefix(t, "/") {
		return ""
	}
	first := strings.Fields(t)[0]
	first = strings.TrimPrefix(first, "/")
	if i := strings.IndexByte(first, '@'); i >= 0 {
		first = first[:i]
	}
	return strings.ToLower(first)
}

func handlePlay(m *telegram.NewMessage) error  { return processPlayCommand(m, false) }
func handleVPlay(m *telegram.NewMessage) error { return processPlayCommand(m, true) }

func processPlayCommand(m *telegram.NewMessage, video bool) error {
	root := cmdRoot(m)
	if video && root != "vplay" {
		return nil
	}
	if !video && root != "play" {
		return nil
	}
	if blocked(m) || m.IsPrivate() {
		return nil
	}
	chatID := m.ChatID()
	addServedChat(chatID)
	addServedUser(userIDOf(m))
	query := cmdArgs(m)
	_, _ = m.Delete()
	if last, ok := lastCmd[chatID]; ok && time.Since(last) < time.Duration(Cooldown)*time.Second {
		return nil
	}
	lastCmd[chatID] = time.Now()
	if query == "" {
		_, _ = sendHTML(Bot, chatID, richHeading("usage", 3)+richNote("<code>/play song name</code>"), nil)
		return nil
	}
	return processPlay(m, query, video)
}
