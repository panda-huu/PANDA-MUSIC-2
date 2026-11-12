package main

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

var blockedWords = []string{"blocked"}
var lastCmd = map[int64]time.Time{}
var acceptUpdates atomic.Bool

func enableUpdates() {
	acceptUpdates.Store(true)
}

func live(h func(*telegram.NewMessage) error) func(*telegram.NewMessage) error {
	return func(m *telegram.NewMessage) error {
		if isStaleMessage(m) {
			return nil
		}
		return h(m)
	}
}

func registerHandlers() {
	// A single message dispatcher also handles Telegram's /command@botusername
	// form used in groups. The old message:/command filters could silently miss
	// those updates, which made menus and commands appear broken in clones.
	Bot.On(telegram.OnNewMessage, live(dispatchCommand))
	Bot.On(telegram.OnNewMessage, live(handleTrackServed))
	Bot.On(telegram.OnNewMessage, live(handleWelcomeListen))
	Bot.On(telegram.OnNewMessage, live(handlePlaylistListen))
	Bot.On(telegram.OnNewMessage, live(handleChatbotListen)) // chatbot handler
	Bot.On(telegram.OnParticipant, handleParticipant)
	Bot.On(telegram.OnParticipant, handleWelcomeParticipant)
	Bot.On(telegram.OnAction, handleServiceMessage)
	Bot.AddRawHandler(&telegram.UpdateChannelParticipant{}, handleRawChannelParticipant)
	Bot.On(telegram.OnCallbackQuery, handleCallbackQuery)
}

func dispatchCommand(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}
	root := cmdRoot(m)
	var handler func(*telegram.NewMessage) error
	switch root {
	case "start": handler = handleStart
	case "help": handler = handleHelp
	case "play": handler = handlePlay
	case "vplay", "vplya": handler = handleVPlay
	case "playlist": handler = handlePlaylist
	case "welcome": handler = handleWelcome
	case "setwelcome": handler = handleSetWelcome
	case "resetwelcome": handler = handleResetWelcome
	case "pause": handler = handlePause
	case "resume": handler = handleResume
	case "skip": handler = handleSkip
	case "stop", "end": handler = handleStop
	case "clear": handler = handleClear
	case "queue": handler = handleQueue
	case "reboot": handler = handleReboot
	case "ping": handler = handlePing
	case "repo": handler = handleRepo
	case "id": handler = handleID
	case "nothumb": handler = handleNoThumb
	case "speed": handler = handleSpeed
	case "speedreset": handler = handleSpeedReset
	case "bass": handler = handleBass
	case "bassoff": handler = handleBassOff
	case "effecton": handler = handleEffectOn
	case "effectoff": handler = handleEffectOff
	case "effects": handler = handleEffects
	case "seek": handler = handleSeek
	case "seekback": handler = handleSeekBack
	case "gblock": handler = handleGBlock
	case "gunblock": handler = handleGUnblock
	case "ublock": handler = handleUBlock
	case "uunblock": handler = handleUUnblock
	case "blocklist": handler = handleBlocklist
	case "broadcast", "gcast": handler = handleBroadcast
	case "ubroadcast": handler = handleUserBroadcast
	case "gbroadcast": handler = handleGroupBroadcast
	case "stats": handler = handleStats
	case "couple", "couples": handler = handleCouple
	default:
		return nil
	}

	groupOnly := map[string]bool{
		"play": true, "vplay": true, "vplya": true,
		"pause": true, "resume": true, "skip": true, "stop": true,
		"end": true, "clear": true, "queue": true, "reboot": true,
		"speed": true, "speedreset": true, "bass": true, "bassoff": true,
		"effecton": true, "effectoff": true, "effects": true,
		"seek": true, "seekback": true, "couple": true, "couples": true,
		"welcome": true, "setwelcome": true, "resetwelcome": true,
	}
	if groupOnly[root] && m.IsPrivate() {
		_, _ = sendHTML(Bot, m.ChatID(),
			richHeading("group command", 3)+richNote("This command works in a group. Add the bot to your group and try again."), nil)
		return nil
	}

	adminOnly := map[string]bool{
		"pause": true, "resume": true, "skip": true, "stop": true,
		"end": true, "clear": true, "speed": true, "speedreset": true,
		"bass": true, "bassoff": true, "effecton": true, "effectoff": true,
		"seek": true, "seekback": true, "nothumb": true,
	}
	if adminOnly[root] && !isOwner(userIDOf(m)) && !isAuthorized(m) {
		_, _ = sendHTML(Bot, m.ChatID(),
			richHeading("admin permission required", 3)+richNote("Only group admins can use this command. Make sure the bot can read group members."), nil)
		return nil
	}
	return handler(m)
}

func messageDate(m *telegram.NewMessage) int64 {
	if m == nil || m.Message == nil {
		return 0
	}
	return int64(m.Message.Date)
}

func isStaleMessage(m *telegram.NewMessage) bool {
	if !acceptUpdates.Load() {
		return true
	}
	if m == nil {
		return true
	}
	ts := messageDate(m)
	text := strings.ToLower(strings.TrimSpace(m.Text()))
	danger := strings.HasPrefix(text, "/start") || strings.HasPrefix(text, "/help") || strings.HasPrefix(text, "/broadcast") || strings.HasPrefix(text, "/gcast")
	if ts == 0 {
		return danger
	}
	return time.Unix(ts, 0).Before(botStartTime.Add(-2 * time.Second))
}

func cmdArgs(m *telegram.NewMessage) string {
	parts := strings.SplitN(strings.TrimSpace(m.Text()), " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func chatIDOf(m *telegram.NewMessage) int64 { return m.ChatID() }

func userIDOf(m *telegram.NewMessage) int64 {
	if m.Sender == nil {
		return 0
	}
	return m.Sender.ID
}

func userNameOf(m *telegram.NewMessage) string {
	if m.Sender == nil {
		return "Unknown"
	}
	return sanitizeDisplayName(m.Sender.FirstName)
}

func senderFullName(m *telegram.NewMessage) string {
	if m == nil || m.Sender == nil {
		return "User"
	}
	name := strings.TrimSpace(strings.TrimSpace(m.Sender.FirstName) + " " + strings.TrimSpace(m.Sender.LastName))
	return sanitizeDisplayName(name)
}

func senderUsername(m *telegram.NewMessage) string {
	if m == nil || m.Sender == nil || strings.TrimSpace(m.Sender.Username) == "" {
		return "-"
	}
	return "@" + strings.TrimSpace(m.Sender.Username)
}

func mentionOf(m *telegram.NewMessage) string {
	if m.Sender == nil {
		return "Unknown"
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, m.Sender.ID, richEsc(m.Sender.FirstName))
}

func isOwner(id int64) bool { return id == OwnerID || id == 777000 }

func isAuthorized(m *telegram.NewMessage) bool {
	uid := userIDOf(m)
	if isOwner(uid) {
		return true
	}
	if m.IsPrivate() {
		return false
	}
	member, err := Bot.GetChatMember(m.ChatID(), uid)
	if err != nil || member == nil {
		return false
	}
	st := strings.ToLower(fmt.Sprint(member.Status))
	return strings.Contains(st, "admin") || strings.Contains(st, "creator") || strings.Contains(st, "owner")
}

func blocked(m *telegram.NewMessage) bool {
	if isStaleMessage(m) {
		return true
	}
	if isGroupBlocked(m.ChatID()) {
		return true
	}
	return isUserBlockedDB(userIDOf(m))
}

func pickStartPhoto() string {
	if len(StartPhotos) == 0 {
		return ""
	}
	return StartPhotos[rand.Intn(len(StartPhotos))]
}

func startInner(uid int64, name string) string { return startPrivateHTML(uid, name, BotName) }
func aboutInner() string                       { return aboutHTML() }
func helpInner(uid int64, name string) string  { return helpMainHTML() }
func startCaption(uid int64, name string) string {
	return startInner(uid, name)
}
func aboutCaption() string { return aboutInner() }
func helpListCaption(uid int64, name string) string {
	return helpInner(uid, name)
}

func loggerTargets() []int64 {
	if LoggerID == 0 {
		return nil
	}
	// Keep Telegram's canonical supergroup ID (-100xxxxxxxxxx).
	// Stripping -100 creates invalid chat IDs and noisy GetChat failures.
	return []int64{LoggerID}
}

func sendLogger(text string, markup telegram.ReplyMarkup) {
	if LoggerID == 0 {
		log.Println("logger skip: LOGGER_ID is 0")
		return
	}
	clients := []*telegram.Client{Bot, Assistant}
	var last error
	for _, c := range clients {
		if c == nil {
			continue
		}
		for _, id := range loggerTargets() {
			_, _ = c.ResolvePeer(id)
			_, err := sendHTML(c, id, text, markup)
			if err == nil {
				log.Println("logger sent to", id)
				return
			}
			last = err
			log.Println("logger try", id, "failed:", err)
		}
	}
	if last != nil {
		log.Println("logger send failed:", last)
	}
}

func warmLogger() {
	if LoggerID == 0 {
		log.Println("LOGGER_ID empty")
		return
	}
	log.Println("warming logger", LoggerID)
	for _, id := range loggerTargets() {
		for _, c := range []*telegram.Client{Bot, Assistant} {
			if c == nil {
				continue
			}
			// ResolvePeer is sufficient to warm the channel peer/access hash.
			// GetChat is not required here and can report a false "no chat" for
			// valid channel IDs that were resolved successfully.
			if _, err := c.ResolvePeer(id); err != nil {
				log.Println("logger ResolvePeer", id, err)
			}
		}
	}
}

func chatTitleOf(chatID int64) string {
	chatID = canonChatID(chatID)
	if g := cachedGroupOf(chatID); g.Title != "" {
		return g.Title
	}
	meta := refreshGroupMeta(chatID)
	if meta.Title != "" {
		return meta.Title
	}
	return "Group"
}

func playSourceOf(song Song) string {
	return "Searched on Youtube"
}

func playModeOf(song Song, queued bool) string {
	if queued {
		return smallcaps("queue")
	}
	if song.Video {
		return smallcaps("vplay")
	}
	return smallcaps("play")
}

func logPlayAction(chatID int64, song Song, queued bool) {
	if LoggerID == 0 || chatID == 0 {
		log.Println("play log skip: logger id empty")
		return
	}
	go refreshGroupMeta(chatID)
	name := song.Requester
	if name == "" {
		name = "User"
	}
	rememberUser(song.RequesterID, name, "")
	title := song.Title
	if title == "" {
		title = "Unknown"
	}
	head := smallcaps("new play log")
	if queued {
		head = smallcaps("new queue log")
	}
	body := "<blockquote expandable><b>" + head + "</b>\n\n" +
		smallcaps("user") + " : " + mentionHTML(song.RequesterID, name) + " [<code>" + fmt.Sprintf("%d", song.RequesterID) + "</code>]\n" +
		smallcaps("group") + " : " + richEsc(chatTitleOf(chatID)) + "\n" +
		smallcaps("group id") + " : <code>" + fmt.Sprintf("%d", canonChatID(chatID)) + "</code>\n" +
		smallcaps("query/song") + " : " + richEsc(title) + "\n" +
		smallcaps("source") + " : " + richEsc(playSourceOf(song)) + "\n" +
		smallcaps("mode") + " : " + playModeOf(song, queued) + "</blockquote>"
	sendLogger(body, profileMarkup(song.RequesterID, ""))
}

func logNewUserStart(m *telegram.NewMessage) {
	if LoggerID == 0 || m == nil {
		log.Println("start log skip: logger id empty or no message")
		return
	}
	if !m.IsPrivate() {
		return
	}
	uid := userIDOf(m)
	if uid == 0 {
		return
	}
	name := senderFullName(m)
	uname := senderUsername(m)
	rememberUser(uid, name, uname)
	total := getServedUsersCount()
	body := "<blockquote expandable><b>NEW USER STARTED BOT</b>\n\n" +
		"<b>NAME</b> — " + mentionHTML(uid, name) + "\n" +
		"<b>U NAME</b> — <code>" + richEsc(uname) + "</code>\n" +
		"<b>U ID</b> — <code>" + fmt.Sprintf("%d", uid) + "</code>\n" +
		"<b>TOTAL USER</b> — <code>" + fmt.Sprintf("%d", total) + "</code></blockquote>"
	sendLogger(body, profileMarkup(uid, uname))
}

func sendStartMenu(chatID int64, inner string, rows [][]InlineBtn) {
	ids := []int64{chatID, canonChatID(chatID)}
	var last error
	for _, id := range ids {
		if id == 0 {
			continue
		}
		_, err := sendQuotedPhoto(id, inner, rows)
		if err == nil {
			return
		}
		last = err
		log.Println("start photo fail", id, err)
		_, err = sendHTML(Bot, id, inner, gogramMarkup(rows))
		if err == nil {
			return
		}
		last = err
		log.Println("start text fail", id, err)
	}
	if last != nil {
		log.Println("start menu failed:", last)
	}
}

func handleStart(m *telegram.NewMessage) error {
	if blocked(m) {
		return nil
	}
	uid := userIDOf(m)
	name := userNameOf(m)
	chatID := m.ChatID()
	_ = addServedUser(uid)
	if !m.IsPrivate() {
		addServedChat(canonChatID(chatID))
	}
	rememberUser(uid, senderFullName(m), senderUsername(m))
	arg := strings.ToLower(cmdArgs(m))
	if m.IsPrivate() {
		go logNewUserStart(m)
		if arg == "pm_help" {
			sendStartMenu(chatID, helpInner(uid, name), GetHelpMarkup())
		} else {
			sendStartMenu(chatID, startInner(uid, name), GetStartMarkup())
		}
		addBroadcastChat(chatID, "private")
		_, _ = m.Delete()
		return nil
	}
	go refreshGroupMeta(chatID)
	sendStartMenu(chatID, startGroupHTML(), GetGroupStartMarkup())
	addBroadcastChat(chatID, "group")
	_, _ = m.Delete()
	return nil
}

func handleHelp(m *telegram.NewMessage) error {
	if blocked(m) {
		return nil
	}
	if !m.IsPrivate() {
		sendStartMenu(m.ChatID(), helpPrivateOnlyHTML(), GetGroupStartMarkup())
		_, _ = m.Delete()
		return nil
	}
	sendStartMenu(m.ChatID(), helpInner(userIDOf(m), userNameOf(m)), GetHelpMarkup())
	_, _ = m.Delete()
	return nil
}

func handleTrackServed(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}
	if m.Sender != nil {
		_ = addServedUser(m.Sender.ID)
		rememberUser(m.Sender.ID, senderFullName(m), senderUsername(m))
	}
	if !m.IsPrivate() {
		chatID := canonChatID(m.ChatID())
		addServedChat(chatID)
		if m.Channel != nil {
			rememberGroup(chatID, m.Channel.Title, m.Channel.Username, "")
		} else if m.Chat != nil {
			rememberGroup(chatID, m.Chat.Title, "", "")
		}
	}
	return nil
}
