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

type welcomeBtn struct {
	Text   string
	URL    string
	Same   bool
	Colour string
}

type welcomeCfg struct {
	On        bool
	Text      string
	Photo     string
	MediaKind string
	Custom    bool
	Buttons   []welcomeBtn
}

type welSession struct {
	ChatID int64
	MsgID  int
	Mode   string
}

var (
	welcomeStore sync.Map
	welcomeMedia sync.Map
	welSess      sync.Map
	welOnce      sync.Map
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
	return welcomeCfg{On: true, Custom: false, Text: defaultWelcomeText(), Buttons: defaultWelcomeButtons()}
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
				if strings.TrimSpace(c.Text) == "" && c.Photo == "" && c.MediaKind == "" {
					c.Text = defaultWelcomeText()
				}
				if !c.Custom && len(c.Buttons) == 0 {
					c.Buttons = defaultWelcomeButtons()
				}
				return c
			}
		}
	}
	return defaultWelcomeCfg()
}

func saveWelcome(chatID int64, cfg welcomeCfg) {
	if strings.TrimSpace(cfg.Text) == "" && cfg.Photo == "" && cfg.MediaKind == "" {
		cfg.Text = defaultWelcomeText()
	}
	for _, id := range welcomeLookupIDs(chatID) {
		welcomeStore.Store(id, cfg)
	}
}

func storeWelcomeMedia(chatID int64, media any) {
	if media == nil {
		for _, id := range welcomeLookupIDs(chatID) {
			welcomeMedia.Delete(id)
		}
		return
	}
	for _, id := range welcomeLookupIDs(chatID) {
		welcomeMedia.Store(id, media)
	}
}

func loadWelcomeMedia(chatID int64) any {
	for _, id := range welcomeLookupIDs(chatID) {
		if v, ok := welcomeMedia.Load(id); ok && v != nil {
			return v
		}
	}
	return nil
}

func welcomeMediaFromMsg(m *telegram.NewMessage) (kind string, media any) {
	if m == nil || !m.IsMedia() {
		return "", nil
	}
	kind = strings.ToLower(strings.TrimSpace(m.MediaType()))
	switch kind {
	case "photo", "video", "animation", "gif", "document":
	default:
		kind = "photo"
	}
	return kind, m.Media()
}

func welcomeCaptionOf(m *telegram.NewMessage) string {
	if m == nil {
		return ""
	}
	if t := strings.TrimSpace(m.Text()); t != "" {
		return t
	}
	return ""
}

func welcomePanelCaption(chatID int64) string {
	cfg := welcomeOf(chatID)
	st := smallcaps("off")
	if cfg.On {
		st = smallcaps("on")
	}
	media := smallcaps("none")
	switch {
	case cfg.MediaKind != "":
		media = smallcaps(cfg.MediaKind)
	case cfg.Photo != "":
		media = smallcaps("image")
	}
	style := smallcaps("default")
	if cfg.Custom {
		style = smallcaps("custom")
	}
	return "<blockquote><b>" + smallcaps("welcome settings") + "</b></blockquote>\n" +
		"<blockquote expandable>" + smallcaps("status") + " : <b>" + st + "</b>\n" +
		smallcaps("style") + " : <b>" + style + "</b>\n" +
		smallcaps("media") + " : <b>" + media + "</b>\n" +
		smallcaps("buttons") + " : <code>" + fmt.Sprintf("%d", len(cfg.Buttons)) + "</code>\n\n" +
		smallcaps("placeholders") + " : {mention} {name} {username} {id} {chat}\n" +
		smallcaps("set text") + " : " + smallcaps("text or photo video with caption") + "</blockquote>"
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
		tpl = "{mention}"
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
		return
	}
	name, uname = userFromCache(uid, name, uname)
	body := fillWelcome(cfg.Text, uid, name, uname, chatTitleOf(chatID))
	rows := welcomeJoinMarkup(cfg)
	mk := gogramMarkup(rows)
	ents, plain := captionEntities(telegramHTML(body))
	opts := &telegram.MediaOptions{Caption: plain, ParseMode: "", Entities: ents, ReplyMarkup: mk}
	media := loadWelcomeMedia(chatID)
	for _, id := range welcomeLookupIDs(chatID) {
		if media != nil {
			if _, err := Bot.SendMedia(id, media, opts); err == nil {
				return
			}
		}
		sendBody := body
		if cfg.Photo != "" {
			sendBody = richImg(cfg.Photo) + body
		}
		if _, err := sendHTML(Bot, id, sendBody, mk); err == nil {
			return
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

func applyCustomWelcome(chatID int64, text string, kind string, media any) {
	clean, btns := parseWelcomeButtons(text)
	cfg := welcomeOf(chatID)
	cfg.Custom = true
	cfg.On = true
	cfg.Buttons = btns
	if clean != "" {
		cfg.Text = clean
	} else if kind != "" {
		cfg.Text = "{mention}"
	}
	if kind != "" && media != nil {
		cfg.MediaKind = kind
		cfg.Photo = ""
		storeWelcomeMedia(chatID, media)
	}
	saveWelcome(chatID, cfg)
}

func handleWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() || !welcomeAdminOK(m.ChatID(), userIDOf(m)) {
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
	case "off", "disable", "false", "0":
		cfg.On = false
	}
	saveWelcome(chatID, cfg)
	_, _ = m.Delete()
	showWelcomePanel(chatID)
	return nil
}

func handleSetWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() || !welcomeAdminOK(m.ChatID(), userIDOf(m)) {
		return nil
	}
	chatID := canonChatID(m.ChatID())
	if id := m.ChannelID(); id != 0 {
		chatID = canonChatID(id)
	}
	text := strings.TrimSpace(cmdArgs(m))
	kind, media := welcomeMediaFromMsg(m)
	_, _ = m.Delete()
	if text == "" && kind == "" {
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("send text or photo video with caption")), nil)
		return nil
	}
	applyCustomWelcome(chatID, text, kind, media)
	showWelcomePanel(chatID)
	return nil
}

func handleResetWelcome(m *telegram.NewMessage) error {
	if blocked(m) || m.IsPrivate() || !welcomeAdminOK(m.ChatID(), userIDOf(m)) {
		return nil
	}
	chatID := canonChatID(m.ChatID())
	storeWelcomeMedia(chatID, nil)
	saveWelcome(chatID, defaultWelcomeCfg())
	_, _ = m.Delete()
	showWelcomePanel(chatID)
	return nil
}

func handleWelcomeListen(m *telegram.NewMessage) error {
	if m == nil || blocked(m) || m.IsPrivate() {
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
	text := welcomeCaptionOf(m)
	if strings.HasPrefix(text, "/") {
		return nil
	}
	kind, media := welcomeMediaFromMsg(m)
	if text == "" && kind == "" {
		return nil
	}
	_, _ = m.Delete()
	switch sess.Mode {
	case "set":
		applyCustomWelcome(sess.ChatID, text, kind, media)
	case "btn":
		parts := strings.SplitN(text, "|", 3)
		if len(parts) < 2 {
			editPlaylistPanel(sess.ChatID, sess.MsgID, wrapBQ(smallcaps("use")+" Name | https://t.me/xxx"), welcomePanelMarkup(sess.ChatID))
			return nil
		}
		cfg := welcomeOf(sess.ChatID)
		cfg.Custom = true
		colors := []string{ColourRed, ColourBlue, ColourGreen}
		cfg.Buttons = append(cfg.Buttons, welcomeBtn{
			Text: strings.TrimSpace(parts[0]), URL: strings.TrimSpace(parts[1]),
			Same: len(parts) > 2 && strings.Contains(strings.ToLower(parts[2]), "same"),
			Colour: colors[len(cfg.Buttons)%3],
		})
		saveWelcome(sess.ChatID, cfg)
	}
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
		storeWelcomeMedia(chatID, nil)
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
		_ = editQuotedCaption(cb, wrapBQ(smallcaps("send text or photo video with caption")), welcomePanelMarkup(chatID))
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
	key := fmt.Sprintf("%d:%d", canonChatID(chatID), uid)
	if v, ok := welOnce.Load(key); ok {
		if t, ok := v.(time.Time); ok && time.Since(t) < 20*time.Second {
			return
		}
	}
	welOnce.Store(key, time.Now())
	go sendWelcome(chatID, uid, name, uname)
}

func handleWelcomeParticipant(p *telegram.ParticipantUpdate) error {
	if p == nil || p.User == nil || !(p.IsAdded() || p.IsJoined()) {
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
	if !(strings.Contains(kind, "adduser") || strings.Contains(kind, "joined")) {
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
