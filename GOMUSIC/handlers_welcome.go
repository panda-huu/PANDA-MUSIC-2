package main

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/amarnathcjd/gogram/telegram"
)

type welcomeBtn struct {
	Text   string
	URL    string
	Same   bool
	Colour string
}

type welcomeCfg struct {
	On      bool
	Text    string
	Photo   string
	Buttons []welcomeBtn
}

type welSession struct {
	ChatID int64
	MsgID  int
	Mode   string
}

var (
	welcomeStore sync.Map
	welSess     sync.Map
	reWelcomeBtn = regexp.MustCompile(`(?i)\[([^\]]+)\]\(buttonurl://([^)]+)\)`)
)

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

func welcomePanelCaption(chatID int64) string {
	cfg := welcomeOf(chatID)
	st := smallcaps("off")
	if cfg.On {
		st = smallcaps("on")
	}
	n := len(cfg.Buttons)
	return "<blockquote><b>" + smallcaps("welcome settings") + "</b></blockquote>\n" +
		"<blockquote expandable>" + smallcaps("status") + " : <b>" + st + "</b>\n" +
			smallcaps("buttons") + " : <code>" + fmt.Sprintf("%d", n) + "</code>\n\n" +
			smallcaps("placeholders") + " : {mention} {name} {username} {id} {chat}\n" +
			smallcaps("or use") + " [rules](buttonurl://https://t.me/xxx)</blockquote>"
}

func welcomePanelMarkup(chatID int64) [][]InlineBtn {
	_ = chatID
	return [][]InlineBtn{
		{
			styleBtn(smallcaps("enable"), "wel_on", ColourGreen),
			styleBtn(smallcaps("disable"), "wel_off", ColourRed),
		},
		{
			styleBtn(smallcaps("set text"), "wel_set", ColourBlue),
			styleBtn(smallcaps("add btn"), "wel_btn", ColourGreen),
		},
		{
			styleBtn(smallcaps("reset"), "wel_reset", ColourRed),
			styleBtn(smallcaps("test"), "wel_test", ColourBlue),
		},
		{styleBtn(smallcaps("close"), "wel_close", ColourBlue)},
	}
}

func welcomeJoinMarkup(cfg welcomeCfg) [][]InlineBtn {
	var rows [][]InlineBtn
	var row []InlineBtn
	colors := []string{ColourRed, ColourBlue, ColourGreen}
	for i, b := range cfg.Buttons {
		if b.Text == "" || b.URL == "" {
			continue
		}
		col := b.Colour
		if col == "" {
			col = colors[i%3]
		}
		btn := styleURLBtn(b.Text, b.URL, col)
		if b.Same && len(row) > 0 {
			row = append(row, btn)
		} else {
			if len(row) > 0 {
				rows = append(rows, row)
			}
			row = []InlineBtn{btn}
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func parseWelcomeButtons(text string) (string, []welcomeBtn) {
	var btns []welcomeBtn
	colors := []string{ColourRed, ColourBlue, ColourGreen}
	clean := reWelcomeBtn.ReplaceAllStringFunc(text, func(m string) string {
		parts := reWelcomeBtn.FindStringSubmatch(m)
		if len(parts) < 3 {
			return ""
		}
		url := strings.TrimSpace(parts[2])
		same := false
		if strings.HasSuffix(strings.ToLower(url), ":same") {
			same = true
			url = strings.TrimSuffix(url, ":same")
			url = strings.TrimSuffix(url, ":SAME")
		}
		btns = append(btns, welcomeBtn{
			Text:   strings.TrimSpace(parts[1]),
			URL:    strings.TrimSpace(url),
			Same:   same,
			Colour: colors[len(btns)%3],
		})
		return ""
	})
	return strings.TrimSpace(clean), btns
}

func fillWelcome(tpl string, uid int64, name, uname, chat string) string {
	if strings.TrimSpace(tpl) == "" {
		tpl = defaultWelcomeText()
	}
	repl := map[string]string{
		"{mention}":  mentionHTML(uid, name),
		"{name}":     richEsc(name),
		"{username}": richEsc(uname),
		"{id}":       fmt.Sprintf("%d", uid),
		"{chat}":     richEsc(chat),
	}
	out := tpl
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func sendWelcome(chatID int64, uid int64, name, uname string) {
	cfg := welcomeOf(chatID)
	if !cfg.On || uid == 0 || uid == botSelfID() {
		return
	}
	body := fillWelcome(cfg.Text, uid, name, uname, chatTitleOf(chatID))
	rows := welcomeJoinMarkup(cfg)
	if cfg.Photo != "" {
		body = richImg(cfg.Photo) + body
	}
	_, _ = sendHTML(Bot, canonChatID(chatID), body, gogramMarkup(rows))
}

func showWelcomePanel(chatID int64) {
	_, _ = sendQuotedPhoto(chatID, welcomePanelCaption(chatID), welcomePanelMarkup(chatID))
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
		saveWelcome(chatID, cfg)
	case "off", "disable", "false", "0":
		cfg.On = false
		saveWelcome(chatID, cfg)
	}
	_, _ = m.Delete()
	showWelcomePanel(chatID)
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
	_, _ = m.Delete()
	if text == "" {
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("send")+" /setwelcome welcome {mention}\n[rules](buttonurl://https://t.me/xxx)"), nil)
		return nil
	}
	clean, btns := parseWelcomeButtons(text)
	cfg := welcomeOf(chatID)
	cfg.Text = clean
	if len(btns) > 0 {
		cfg.Buttons = btns
	}
	cfg.On = true
	saveWelcome(chatID, cfg)
	showWelcomePanel(chatID)
	return nil
}

func handleResetWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() {
		return nil
	}
	if !isAuthorized(m) && !isOwner(userIDOf(m)) {
		return nil
	}
	chatID := canonChatID(m.ChatID())
	saveWelcome(chatID, welcomeCfg{On: welcomeOf(chatID).On, Text: defaultWelcomeText()})
	_, _ = m.Delete()
	showWelcomePanel(chatID)
	return nil
}

func handleWelcomeListen(m *telegram.NewMessage) error {
	if m == nil || blocked(m) || m.IsPrivate() {
		return nil
	}
	text := strings.TrimSpace(m.Text())
	if text == "" || strings.HasPrefix(text, "/") {
		return nil
	}
	uid := userIDOf(m)
	v, ok := welSess.Load(uid)
	if !ok {
		return nil
	}
	sess, _ := v.(*welSession)
	if sess == nil || !samePlaylistChat(sess.ChatID, m.ChatID()) {
		return nil
	}
	_, _ = m.Delete()
	cfg := welcomeOf(sess.ChatID)
	switch sess.Mode {
	case "set":
		clean, btns := parseWelcomeButtons(text)
		cfg.Text = clean
		if len(btns) > 0 {
			cfg.Buttons = append(cfg.Buttons, btns...)
		}
		cfg.On = true
	case "btn":
		parts := strings.SplitN(text, "|", 3)
		if len(parts) < 2 {
			editPlaylistPanel(sess.ChatID, sess.MsgID, wrapBQ(smallcaps("use")+" Name | https://t.me/xxx"), welcomePanelMarkup(sess.ChatID))
			return nil
		}
		label := strings.TrimSpace(parts[0])
		url := strings.TrimSpace(parts[1])
		same := len(parts) > 2 && strings.Contains(strings.ToLower(parts[2]), "same")
		colors := []string{ColourRed, ColourBlue, ColourGreen}
		cfg.Buttons = append(cfg.Buttons, welcomeBtn{
			Text: label, URL: url, Same: same, Colour: colors[len(cfg.Buttons)%3],
		})
	}
	saveWelcome(sess.ChatID, cfg)
	welSess.Delete(uid)
	editPlaylistPanel(sess.ChatID, sess.MsgID, welcomePanelCaption(sess.ChatID), welcomePanelMarkup(sess.ChatID))
	return nil
}

func handleWelcomeCallback(cb *telegram.CallbackQuery, data string) bool {
	if cb == nil || cb.Sender == nil || !strings.HasPrefix(data, "wel_") {
		return false
	}
	chatID := playlistChatID(cb)
	alert := &telegram.CallbackOptions{Alert: true}
	if !isOwner(cb.Sender.ID) {
		member, err := Bot.GetChatMember(chatID, cb.Sender.ID)
		ok := false
		if err == nil && member != nil {
			st := strings.ToLower(fmt.Sprint(member.Status))
			ok = strings.Contains(st, "admin") || strings.Contains(st, "creator") || strings.Contains(st, "owner")
		}
		if !ok {
			_, _ = cb.Answer(smallcaps("admins only"), alert)
			return true
		}
	}
	cfg := welcomeOf(chatID)
	switch data {
	case "wel_on":
		cfg.On = true
		saveWelcome(chatID, cfg)
		_, _ = cb.Answer(smallcaps("enabled"))
	case "wel_off":
		cfg.On = false
		saveWelcome(chatID, cfg)
		_, _ = cb.Answer(smallcaps("disabled"))
	case "wel_reset":
		saveWelcome(chatID, welcomeCfg{On: cfg.On, Text: defaultWelcomeText()})
		_, _ = cb.Answer(smallcaps("reset"))
	case "wel_test":
		_, _ = cb.Answer(smallcaps("preview"))
		name := sanitizeDisplayName(cb.Sender.FirstName)
		uname := "-"
		if cb.Sender.Username != "" {
			uname = "@" + cb.Sender.Username
		}
		sendWelcome(chatID, cb.Sender.ID, name, uname)
		return true
	case "wel_set":
		welSess.Store(cb.Sender.ID, &welSession{ChatID: chatID, MsgID: int(cb.MessageID), Mode: "set"})
		_, _ = cb.Answer("")
		_ = editQuotedCaption(cb, wrapBQ(smallcaps("send welcome text now")), welcomePanelMarkup(chatID))
		return true
	case "wel_btn":
		welSess.Store(cb.Sender.ID, &welSession{ChatID: chatID, MsgID: int(cb.MessageID), Mode: "btn"})
		_, _ = cb.Answer("")
		_ = editQuotedCaption(cb, wrapBQ(smallcaps("send")+" Name | https://t.me/link\n"+smallcaps("same row")+" Name | url | same"), welcomePanelMarkup(chatID))
		return true
	case "wel_close":
		_, _ = cb.Answer(smallcaps("closed"))
		if msg, _ := cb.GetMessage(); msg != nil {
			_, _ = msg.Delete()
		}
		return true
	default:
		return false
	}
	_ = editQuotedCaption(cb, welcomePanelCaption(chatID), welcomePanelMarkup(chatID))
	return true
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
	name, uname, uid := userBits(p.User)
	sendWelcome(canonChatID(p.ChannelID()), uid, name, uname)
	return nil
}
