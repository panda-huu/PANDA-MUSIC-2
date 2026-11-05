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

var (
	cbOnce   sync.Map
	cbOnceMu sync.Mutex
)

func handleCallback(m *telegram.NewMessage) error { return nil }

func callbackChatID(cb *telegram.CallbackQuery, msg *telegram.NewMessage) int64 {
	// CallbackQuery.ChatID is the peer attached to the inline-button update.
	// Prefer it over fields reconstructed from the message object: for
	// channel-backed supergroups, msg.ChannelID() can be a raw channel ID
	// while the player/ntgcalls state is keyed by the canonical -100 chat ID.
	if cb != nil && cb.ChatID != 0 {
		return canonChatID(cb.ChatID)
	}
	if msg != nil {
		if id := msgBotChatID(msg); id != 0 {
			return canonChatID(id)
		}
		if id := msg.ChatID(); id != 0 {
			return canonChatID(id)
		}
	}
	return 0
}

func alreadyHandledCallback(cb *telegram.CallbackQuery) bool {
	if cb == nil {
		return true
	}
	key := fmt.Sprintf("%d:%d:%d:%s", cb.QueryID, cb.ChatID, cb.MessageID, string(cb.Data))
	cbOnceMu.Lock()
	defer cbOnceMu.Unlock()
	if t, ok := cbOnce.Load(key); ok {
		if time.Since(t.(time.Time)) < 2*time.Second {
			return true
		}
	}
	cbOnce.Store(key, time.Now())
	return false
}

func handleCallbackQuery(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	if !acceptUpdates.Load() {
		_, _ = cb.Answer("")
		return nil
	}
	if alreadyHandledCallback(cb) {
		return nil
	}
	if cb.Sender != nil && isUserBlockedDB(cb.Sender.ID) {
		_, _ = cb.Answer("")
		return nil
	}
	data := string(cb.Data)
	if handlePlaylistCallback(cb, data) {
		return nil
	}
	if handleWelcomeCallback(cb, data) {
		return nil
	}
	msg, _ := cb.GetMessage()
	chatID := callbackChatID(cb, msg)
	log.Printf("player callback data=%q chat_id=%d message_chat_id=%d", data, chatID, func() int64 {
		if msg == nil { return 0 }
		return msg.ChatID()
	}())
	alert := &telegram.CallbackOptions{Alert: true}
	if chatID == 0 && (data == "pause" || data == "resume" || data == "replay" || data == "skip" || data == "seek_back" || data == "seek_fwd") {
		_, _ = cb.Answer("could not identify this group's player", alert)
		return nil
	}
	switch {
	case data == "pause":
		if Calls == nil {
			_, _ = cb.Answer("player is not ready", alert)
			return nil
		}
		if _, err := Calls.Pause(chatID); err != nil {
			log.Printf("pause callback failed chat=%d: %v", chatID, err)
			_, _ = cb.Answer("failed to pause: "+shortTitle(err.Error(), 120), alert)
			return nil
		}
		_, _ = cb.Answer("paused")
	case data == "resume":
		if Calls == nil {
			_, _ = cb.Answer("player is not ready", alert)
			return nil
		}
		if _, err := Calls.Resume(chatID); err != nil {
			log.Printf("resume callback failed chat=%d: %v", chatID, err)
			_, _ = cb.Answer("failed to resume: "+shortTitle(err.Error(), 120), alert)
			return nil
		}
		_, _ = cb.Answer("resumed")
	case data == "replay":
		cur := peekCurrent(chatID)
		if cur == nil {
			_, _ = cb.Answer("nothing playing", alert)
			return nil
		}
		if err := playSongOpt(chatID, nil, *cur, true); err != nil {
			_, _ = cb.Answer("replay failed", alert)
			return nil
		}
		_, _ = cb.Answer("replaying")
	case data == "skip" || strings.HasPrefix(data, "skip:"):
		if strings.HasPrefix(data, "skip:") {
			if id, err := strconv.ParseInt(strings.TrimPrefix(data, "skip:"), 10, 64); err == nil {
				chatID = id
			}
		}
		if err := RoomChangeStream(chatID); err != nil {
			_, _ = cb.Answer("queue empty", alert)
			return nil
		}
		_, _ = cb.Answer("skipped")
	case strings.HasPrefix(data, "queue_now:"):
		parts := strings.Split(strings.TrimPrefix(data, "queue_now:"), ":")
		var idx int
		var err error
		if len(parts) >= 2 {
			if id, e := strconv.ParseInt(parts[0], 10, 64); e == nil {
				chatID = id
			}
			idx, err = strconv.Atoi(parts[1])
		} else {
			idx, err = strconv.Atoi(parts[0])
		}
		if err != nil {
			_, _ = cb.Answer("song not in queue", alert)
			return nil
		}
		if err := RoomPlayNow(chatID, idx); err != nil {
			_, _ = cb.Answer(err.Error(), alert)
			return nil
		}
		_, _ = cb.Answer("playing now")
		if msg != nil {
			_, _ = msg.Delete()
		}
	case data == "stop":
		stopPlaybackEverywhere(chatID)
		_, _ = cb.Answer("stopped")
		if msg != nil {
			_, _ = msg.Delete()
		}
	case data == "close" || data == "close_panel" || data == "close_help":
		_, _ = cb.Answer("")
		if msg != nil {
			_, _ = msg.Delete()
		}
	case data == "progress":
		cur := peekCurrent(chatID)
		title := "nothing playing"
		if cur != nil {
			title = shortTitle(cur.Title, 28)
		}
		_, _ = cb.Answer(title, alert)
	case data == "seek_back", data == "seek_fwd":
		delta := 15 * time.Second
		if data == "seek_back" {
			delta = -15 * time.Second
		}
		if Calls == nil {
			_, _ = cb.Answer("player is not ready", alert)
			return nil
		}
		if err := Calls.SeekBy(chatID, delta.Milliseconds()); err != nil {
			log.Printf("seek callback failed chat=%d action=%s: %v", chatID, data, err)
			_, _ = cb.Answer("seek failed: "+shortTitle(err.Error(), 150), alert)
			return nil
		}
		if data == "seek_back" {
			_, _ = cb.Answer("-15s")
		} else {
			_, _ = cb.Answer("+15s")
		}
	case data == "clear":
		clearQueue(chatID)
		_, _ = cb.Answer("queue cleared")
	case data == "noop":
		_, _ = cb.Answer("")
	case data == "dev_open":
		_, _ = cb.Answer("")
	case data == "about_menu":
		_, _ = cb.Answer("")
		if !editQuotedCaption(cb, aboutInner(), GetAboutMarkup()) {
			showQuotedMenu(cb, aboutInner(), GetAboutMarkup())
		}
	case data == "start" || data == "go_back":
		_, _ = cb.Answer("")
		if !editQuotedCaption(cb, startInner(cb.Sender.ID, sanitizeDisplayName(cb.Sender.FirstName)), GetStartMarkup()) {
			showQuotedMenu(cb, startInner(cb.Sender.ID, sanitizeDisplayName(cb.Sender.FirstName)), GetStartMarkup())
		}
	case data == "help_cb" || data == "show_help" || data == "help:main" || data == "help_main":
		_, _ = cb.Answer("")
		openHelpPage(cb, "main")
	default:
		if key := helpKeyFromData(data); key != "" {
			_, _ = cb.Answer("")
			openHelpPage(cb, key)
		}
	}
	return nil
}

func notifyOwner(me *telegram.UserObj, asst string) {
	if LoggerID == 0 || me == nil {
		return
	}
	content := wrapBQ(smallcaps("bot started") + "\n\n" +
		smallcaps("bot") + " : @" + richEsc(me.Username) + "\n" +
		smallcaps("assistant") + " : @" + richEsc(asst))
	if _, err := sendHTML(Bot, LoggerID, content, nil); err != nil {
		log.Println("Logger Notification Error :", err)
	}
}
