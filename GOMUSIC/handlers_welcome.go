package main

import (
	"fmt"
	"log"
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

func defaultWelcomeButtons() []welcomeBtn {
	var out []welcomeBtn
	if strings.TrimSpace(BotLink) != "" {
		out = append(out, welcomeBtn{Text: smallcaps("add me"), URL: BotLink, Colour: ColourGreen})
	}
	if strings.TrimSpace(SupportGroup) != "" {
		out = append(out, welcomeBtn{Text: smallcaps("support"), URL: SupportGroup, Same: true, Colour: ColourBlue})
	}
	if strings.TrimSpace(UpdatesChannel) != "" {
		out = append(out, welcomeBtn{Text: smallcaps("updates"), URL: UpdatesChannel, Colour: ColourRed})
	}
	return out
}

func defaultWelcomeText() string {
	return "<blockquote><b>" + smallcaps("welcome") + " {mention}</b></blockquote>\n" +
		"<blockquote expandable>" + smallcaps("you just joined") + " <b>{chat}</b>\n" +
		smallcaps("name") + " : {name}\n" +
		smallcaps("username") + " : {username}\n" +
		smallcaps("id") + " : <code>{id}</code></blockquote>"
}

func defaultWelcomeCfg() welcomeCfg {
	return welcomeCfg{On: true, Text: defaultWelcomeText(), Buttons: defaultWelcomeButtons()}
}

func welcomeLookupIDs(chatID int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, id := range append([]int64{canonChatID(chatID), chatID}, chatIDForms(chatID)...) {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func welcomeOf(chatID int64) welcomeCfg {
	for _, id := range welcomeLookupIDs(chatID) {
		if v, ok := welcomeStore.Load(id); ok {
			if c, ok := v.(welcomeCfg); ok {
				if strings.TrimSpace(c.Text) == "" {
					c.Text = defaultWelcomeText()
				}
				return c
			}
		}
	}
	return defaultWelcomeCfg()
}

func saveWelcome(chatID int64, cfg welcomeCfg) {
	if strings.TrimSpace(cfg.Text) == "" {
		cfg.Text = defaultWelcomeText()
	}
	for _, id := range welcomeLookupIDs(chatID) {
		welcomeStore.Store(id, cfg)
	}
}

func welcomePanelCaption(chatID int64) string {
	cfg := welcomeOf(chatID)
	st := smallcaps("off")
	if cfg.On {
		st = smallcaps("on")
	}
	return "<blockquote><b>" + smallcaps("welcome settings") + "</b></blockquote>\n" +
		"<blockquote expandable>" + smallcaps("status") + " : <b>" + st + "</b>\n" +
		smallcaps("buttons") + " : <code>" + fmt.Sprintf("%d", len(cfg.Buttons)) + "</code>\n\n" +
		smallcaps("placeholders") + " : {mention} {name} {username} {id} {chat}\n" +
		smallcaps("button format") + " : Name | https://t.me/xxx</blockquote>"
}

func welcomePanelMarkup(chatID int64) [][]InlineBtn {
	_ = chatID
	return [][]InlineBtn{
		{styleBtn(smallcaps("enable"), "wel_on", ColourGreen), styleBtn(smallcaps("disable"), "wel_off", ColourRed)},
		{styleBtn(smallcaps("set text"), "wel_set", ColourBlue), styleBtn(smallcaps("add btn"), "wel_btn", ColourGreen)},
		{styleBtn(smallcaps("reset"), "wel_reset", ColourRed), styleBtn(smallcaps("test"), "wel_test", ColourBlue)},
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
			continue
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
		row = []InlineBtn{btn}
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
		same := strings.HasSuffix(strings.ToLower(url), ":same")
		url = strings.TrimSuffix(strings.TrimSuffix(url, ":same"), ":SAME")
		btns = append(btns, welcomeBtn{Text: strings.TrimSpace(parts[1]), URL: strings.TrimSpace(url), Same: same, Colour: colors[len(btns)%3]})
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
	if uid == 0 || uid == botSelfID() {
		return
	}
	cfg := welcomeOf(chatID)
	if !cfg.On {
		log.Println("welcome skip disabled chat", chatID, "user", uid)
		return
	}
	name, uname = userFromCache(uid, name, uname)
	body := fillWelcome(cfg.Text, uid, name, uname, chatTitleOf(chatID))
	rows := welcomeJoinMarkup(cfg)
	if cfg.Photo != "" {
		body = richImg(cfg.Photo) + body
	}
	mk := gogramMarkup(rows)
	for _, id := range welcomeLookupIDs(chatID) {
		if _, err := sendHTML(Bot, id, body, mk); err == nil {
			log.Println("welcome sent chat", id, "user", uid)
			return
		} else {
			log.Println("welcome send fail", id, err)
		}
	}
}

func welcomeAdminOK(chatID, uid int64) bool {
	if isOwner(uid) {
		return true
	}
	for _, id := range welcomeLookupIDs(chatID) {
		member, err := Bot.GetChatMember(id, uid)
		if err != nil || member == nil {
			continue
		}
		st := strings.ToLower(fmt.Sprint(member.Status))
		if strings.Contains(st, "admin") || strings.Contains(st, "creator") || strings.Contains(st, "owner") {
			return true
		}
	}
	return false
}

func showWelcomePanel(chatID int64) {
	for _, id := range welcomeLookupIDs(chatID) {
		if _, err := sendQuotedPhoto(id, welcomePanelCaption(chatID), welcomePanelMarkup(chatID)); err == nil {
			return
		}
	}
}

func handleWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() {
		return nil
	}
	if !welcomeAdminOK(m.ChatID(), userIDOf(m)) {
		return nil
	}
	arg := strings.ToLower(strings.TrimSpace(cmdArgs(m)))
	chatID := canonChatID(m.ChatID())
	if id := m.ChannelID(); id != 0 {
		chatID = canonChatID(id)
	}
	cfg := welcomeOf(chatID)
	switch arg {
	case "on", "enable", "true", "1":
		cfg.On = true
		saveWelcome(chatID, cfg)
	case "off", "disable", "false", "0":
		cfg.On = false
		saveWelcome(chatID, cfg)
	default:
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
	if !welcomeAdminOK(m.ChatID(), userIDOf(m)) {
		return nil
	}
	chatID := canonChatID(m.ChatID())
	if id := m.ChannelID(); id != 0 {
		chatID = canonChatID(id)
	}
	text := strings.TrimSpace(cmdArgs(m))
	_, _ = m.Delete()
	if text == "" {
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("send")+" /setwelcome welcome {mention}"), nil)
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
	if !welcomeAdminOK(m.ChatID(), userIDOf(m)) {
		return nil
	}
	chatID := canonChatID(m.ChatID())
	saveWelcome(chatID, defaultWelcomeCfg())
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
	inChat := m.ChatID()
	if id := m.ChannelID(); id != 0 {
		inChat = id
	}
	if sess == nil || !samePlaylistChat(sess.ChatID, inChat) {
		return nil
	}
	_, _ = m.Delete()
	cfg := welcomeOf(sess.ChatID)
	switch sess.Mode {
	case "set":
		clean, btns := parseWelcomeButtons(text)
		cfg.Text = clean
		if len(btns) > 0 {
			cfg.Buttons = btns
		}
		cfg.On = true
	case "btn":
		parts := strings.SplitN(text, "|", 3)
		if len(parts) < 2 {
			editPlaylistPanel(sess.ChatID, sess.MsgID, wrapBQ(smallcaps("use")+" Name | https://t.me/xxx"), welcomePanelMarkup(sess.ChatID))
			return nil
		}
		colors := []string{ColourRed, ColourBlue, ColourGreen}
		cfg.Buttons = append(cfg.Buttons, welcomeBtn{
			Text: strings.TrimSpace(parts[0]), URL: strings.TrimSpace(parts[1]),
			Same: len(parts) > 2 && strings.Contains(strings.ToLower(parts[2]), "same"),
			Colour: colors[len(cfg.Buttons)%3],
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
	if !welcomeAdminOK(chatID, cb.Sender.ID) {
		_, _ = cb.Answer(smallcaps("admins only"), alert)
		return true
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
		saveWelcome(chatID, defaultWelcomeCfg())
		_, _ = cb.Answer(smallcaps("reset"))
	case "wel_test":
		_, _ = cb.Answer(smallcaps("preview"))
		name := sanitizeDisplayName(cb.Sender.FirstName)
		uname := "-"
		if cb.Sender.Username != "" {
			uname = "@" + cb.Sender.Username
		}
		sendWelcome(chatID, cb.Sender.ID, name, uname)
		_ = editQuotedCaption(cb, welcomePanelCaption(chatID), welcomePanelMarkup(chatID))
		return true
	case "wel_set":
		welSess.Store(cb.Sender.ID, &welSession{ChatID: chatID, MsgID: int(cb.MessageID), Mode: "set"})
		_, _ = cb.Answer("")
		_ = editQuotedCaption(cb, wrapBQ(smallcaps("send welcome text now")), welcomePanelMarkup(chatID))
		return true
	case "wel_btn":
		welSess.Store(cb.Sender.ID, &welSession{ChatID: chatID, MsgID: int(cb.MessageID), Mode: "btn"})
		_, _ = cb.Answer("")
		_ = editQuotedCaption(cb, wrapBQ(smallcaps("send")+" Name | https://t.me/link"), welcomePanelMarkup(chatID))
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

func welcomeUserJoin(chatID, uid int64, name, uname string) {
	if uid == 0 || uid == botSelfID() {
		return
	}
	go sendWelcome(chatID, uid, name, uname)
}

func handleWelcomeParticipant(p *telegram.ParticipantUpdate) error {
	if p == nil || p.User == nil {
		return nil
	}
	if !(p.IsAdded() || p.IsJoined()) {
		return nil
	}
	chatID := p.ChannelID()
	if chatID == 0 && p.Channel != nil {
		chatID = p.Channel.ID
	}
	name, uname, uid := userBits(p.User)
	welcomeUserJoin(chatID, uid, name, uname)
	return nil
}

func welcomeFromService(m *telegram.NewMessage) {
	if m == nil || m.IsPrivate() || m.Action == nil {
		return
	}
	chatID := m.ChatID()
	if id := m.ChannelID(); id != 0 {
		chatID = id
	}
	kind := strings.ToLower(fmt.Sprintf("%T %v", m.Action, m.Action))
	added := strings.Contains(kind, "adduser") || strings.Contains(kind, "joined") || strings.Contains(kind, "invite")
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

func welcomeFromRaw(upd *telegram.UpdateChannelParticipant) {
	if upd == nil || upd.UserID == 0 || upd.UserID == botSelfID() {
		return
	}
	newName := strings.ToLower(fmt.Sprintf("%T", upd.NewParticipant))
	if strings.Contains(newName, "banned") || strings.Contains(newName, "left") || upd.NewParticipant == nil {
		return
	}
	chatID := canonChatID(-1000000000000 - upd.ChannelID)
	name, uname := userFromCache(upd.UserID, "User", "-")
	welcomeUserJoin(chatID, upd.UserID, name, uname)
}
