package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/amarnathcjd/gogram/telegram"
)

type welcomeCfg struct {
	On    bool
	Text  string
	Photo string
}

var welcomeStore sync.Map

func defaultWelcomeText() string {
	return "<blockquote><b>" + smallcaps("welcome") + "</b> {mention}</blockquote>\n" +
		"<blockquote expandable>" + smallcaps("glad you joined") + " {chat}\n" +
		smallcaps("id") + " : <code>{id}</code></blockquote>"
}

func welcomeOf(chatID int64) welcomeCfg {
	chatID = canonChatID(chatID)
	if v, ok := welcomeStore.Load(chatID); ok {
		if c, ok := v.(welcomeCfg); ok {
			return c
		}
	}
	return welcomeCfg{On: false, Text: defaultWelcomeText()}
}

func saveWelcome(chatID int64, cfg welcomeCfg) {
	welcomeStore.Store(canonChatID(chatID), cfg)
}

func fillWelcome(tpl string, uid int64, name, uname, chat string, count int) string {
	if strings.TrimSpace(tpl) == "" {
		tpl = defaultWelcomeText()
	}
	mention := mentionHTML(uid, name)
	repl := map[string]string{
		"{mention}":  mention,
		"{name}":     richEsc(name),
		"{username}": richEsc(uname),
		"{id}":       fmt.Sprintf("%d", uid),
		"{chat}":     richEsc(chat),
		"{count}":    strconv.Itoa(count),
	}
	out := tpl
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func welcomeCount(chatID int64) int {
	if Bot == nil {
		return 0
	}
	n, err := Bot.GetChatMembersCount(chatID)
	if err != nil {
		n, err = Bot.GetChatMembersCount(canonChatID(chatID))
	}
	if err != nil {
		return 0
	}
	return int(n)
}

func sendWelcome(chatID int64, uid int64, name, uname string) {
	cfg := welcomeOf(chatID)
	if !cfg.On {
		return
	}
	if uid == 0 || uid == botSelfID() {
		return
	}
	chat := chatTitleOf(chatID)
	body := fillWelcome(cfg.Text, uid, name, uname, chat, welcomeCount(chatID))
	if cfg.Photo != "" {
		body = richImg(cfg.Photo) + body
	}
	_, _ = sendHTML(Bot, canonChatID(chatID), body, nil)
}

func handleWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() {
		return nil
	}
	if !isAuthorized(m) && !isOwner(userIDOf(m)) {
		return nil
	}
	arg := strings.ToLower(strings.TrimSpace(cmdArgs(m)))
	chatID := canonChatID(m.ChatID())
	cfg := welcomeOf(chatID)
	switch arg {
	case "on", "enable", "true", "1":
		cfg.On = true
		if strings.TrimSpace(cfg.Text) == "" {
			cfg.Text = defaultWelcomeText()
		}
		saveWelcome(chatID, cfg)
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("welcome enabled")), nil)
	case "off", "disable", "false", "0":
		cfg.On = false
		saveWelcome(chatID, cfg)
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("welcome disabled")), nil)
	default:
		st := smallcaps("off")
		if cfg.On {
			st = smallcaps("on")
		}
		body := "<blockquote><b>" + smallcaps("welcome") + " : " + st + "</b></blockquote>\n" +
			"<blockquote expandable>" + smallcaps("use") + " <code>/welcome on</code> / <code>/welcome off</code>\n" +
			smallcaps("set text with") + " <code>/setwelcome</code>\n" +
			smallcaps("reset with") + " <code>/resetwelcome</code>\n\n" +
			smallcaps("placeholders") + " : {mention} {name} {username} {id} {chat} {count}</blockquote>"
		_, _ = sendHTML(Bot, chatID, body, nil)
	}
	_, _ = m.Delete()
	return nil
}

func handleSetWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() {
		return nil
	}
	if !isAuthorized(m) && !isOwner(userIDOf(m)) {
		return nil
	}
	chatID := canonChatID(m.ChatID())
	text := strings.TrimSpace(cmdArgs(m))
	if text == "" {
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("usage")+"\n<code>/setwelcome welcome {mention} to {chat}</code>"), nil)
		_, _ = m.Delete()
		return nil
	}
	cfg := welcomeOf(chatID)
	cfg.Text = text
	cfg.On = true
	saveWelcome(chatID, cfg)
	_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("custom welcome saved")), nil)
	_, _ = m.Delete()
	return nil
}

func handleResetWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() {
		return nil
	}
	if !isAuthorized(m) && !isOwner(userIDOf(m)) {
		return nil
	}
	chatID := canonChatID(m.ChatID())	saveWelcome(chatID, welcomeCfg{On: welcomeOf(chatID).On, Text: defaultWelcomeText()})
	_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("welcome reset to default")), nil)
	_, _ = m.Delete()
	return nil
}

func handleWelcomeParticipant(p *telegram.ParticipantUpdate) error {
	if p == nil || p.User == nil {
		return nil
	}
	if !(p.IsAdded() || p.IsJoined()) {
		return nil
	}
	if p.User.ID == botSelfID() {
		return nil
	}
	chatID := canonChatID(p.ChannelID())
	name, uname, uid := userBits(p.User)
	sendWelcome(chatID, uid, name, uname)
	return nil
}
