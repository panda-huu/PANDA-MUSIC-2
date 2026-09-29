package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/amarnathcjd/gogram/telegram"
)

const playlistLimit = 8

type plItem struct {
	Title string
	URL   string
	Dur   string
	Thumb string
}

type plSession struct {
	ChatID      int64
	MsgID       int
	Waiting     bool
	Deleting    bool
	PendingName string
}

var (
	userPL = map[int64][]plItem{}
	plSess = map[int64]*plSession{}
	plMu   sync.Mutex
)

func samePlaylistChat(a, b int64) bool {
	if a == 0 || b == 0 {
		return false
	}
	return a == b || canonChatID(a) == canonChatID(b)
}

func playlistPrompt() string {
	return "<blockquote><b>" + smallcaps("wanna to set your custom play list") + "</b></blockquote>\n" +
		"<blockquote expandable>" + smallcaps("send the song names your play list will be get set") + "</blockquote>"
}

func playlistAddedCaption() string {
	return "<blockquote><b>" + smallcaps("your song was added in your play list") + "</b></blockquote>\n" +
		"<blockquote expandable>" + smallcaps("wanna to add more send song name") + "</blockquote>"
}

func playlistSearchingCaption() string {
	return "<blockquote><b>" + smallcaps("searching song") + "......</b></blockquote>"
}

func playlistDeletePrompt() string {
	return "<blockquote><b>" + smallcaps("wanna to delete playlists songs send name") + "</b></blockquote>"
}

func playlistSelectFormat() string {
	return "<blockquote><b>" + smallcaps("select format") + "</b></blockquote>"
}

func playlistDeleteProgress() string {
	return "<blockquote><b>" + smallcaps("delete in progress") + "</b></blockquote>"
}

func playlistMainCaption(uid int64) string {
	plMu.Lock()
	n := len(userPL[uid])
	plMu.Unlock()
	if n == 0 {
		return playlistPrompt()
	}
	return playlistAddedCaption()
}

func playlistMarkup(uid int64) [][]InlineBtn {
	plMu.Lock()
	items := append([]plItem(nil), userPL[uid]...)
	plMu.Unlock()
	colors := []string{ColourRed, ColourBlue, ColourGreen}
	var rows [][]InlineBtn
	for i, it := range items {
		label := shortTitle(it.Title, 16)
		if label == "" {
			label = smallcaps("song")
		}
		rows = append(rows, []InlineBtn{
			styleBtn(smallcaps("audio")+" · "+label, fmt.Sprintf("pl_a:%d", i), colors[i%3]),
			styleBtn(smallcaps("video")+" · "+label, fmt.Sprintf("pl_v:%d", i), colors[(i+1)%3]),
		})
	}
	rows = append(rows, []InlineBtn{
		styleBtn(smallcaps("delete"), "pl_del", ColourRed),
		styleBtn(smallcaps("close"), "pl_done", ColourBlue),
	})
	return rows
}

func playlistDeleteFormatMarkup() [][]InlineBtn {
	return [][]InlineBtn{
		{styleBtn(smallcaps("audio"), "pl_delfmt:a", ColourGreen), styleBtn(smallcaps("video"), "pl_delfmt:v", ColourRed)},
		{styleBtn(smallcaps("back"), "pl_delback", ColourBlue)},
	}
}

func editPlaylistPanel(chatID int64, msgID int, inner string, rows [][]InlineBtn) {
	if chatID == 0 || msgID == 0 {
		return
	}
	caption := quotedCaption(inner)
	for _, id := range chatIDForms(chatID) {
		try := func(withStyle bool, method string) error {
			body := map[string]any{
				"chat_id":    id,
				"message_id": msgID,
				"parse_mode": "HTML",
			}
			if method == "editMessageCaption" {
				body["caption"] = caption
			} else {
				body["text"] = caption
			}
			if kb := apiMarkup(rows, withStyle); kb != nil {
				body["reply_markup"] = kb
			}
			return botAPICall(method, body)
		}
		if err := try(true, "editMessageCaption"); err == nil || (err != nil && strings.Contains(strings.ToLower(err.Error()), "not modified")) {
			return
		}
		if err := try(false, "editMessageCaption"); err == nil || (err != nil && strings.Contains(strings.ToLower(err.Error()), "not modified")) {
			return
		}
		if err := try(true, "editMessageText"); err == nil || (err != nil && strings.Contains(strings.ToLower(err.Error()), "not modified")) {
			return
		}
	}
}

func deletePlaylistPanel(chatID int64, msgID int) {
	if chatID == 0 || msgID == 0 {
		return
	}
	for _, id := range chatIDForms(chatID) {
		_ = botAPICall("deleteMessage", map[string]any{"chat_id": id, "message_id": msgID})
	}
}

func stopPlaybackEverywhere(chatID int64) {
	for _, id := range chatIDForms(chatID) {
		leaveVCNow(id)
	}
}

func handlePlaylist(m *telegram.NewMessage) error {
	if blocked(m) {
		return nil
	}
	if cmdRoot(m) != "playlist" {
		return nil
	}
	uid := userIDOf(m)
	chatID := m.ChatID()
	if id := m.ChannelID(); id != 0 {
		chatID = id
	}
	chatID = canonChatID(chatID)
	_, _ = m.Delete()
	photo := pickStartPhoto()
	content := playlistPrompt()
	if photo != "" {
		content = richImg(photo) + content
	}
	msg, err := sendHTML(Bot, chatID, content, gogramMarkup(playlistMarkup(uid)))
	if err != nil || msg == nil {
		msg, err = sendHTML(Bot, m.ChatID(), content, gogramMarkup(playlistMarkup(uid)))
	}
	if err != nil || msg == nil {
		_, _ = sendQuotedPhoto(chatID, playlistPrompt(), playlistMarkup(uid))
		return nil
	}
	mid := int(msg.ID)
	sid := msg.ChatID()
	if id := msgBotChatID(msg); id != 0 {
		sid = id
	}
	plMu.Lock()
	plSess[uid] = &plSession{ChatID: canonChatID(sid), MsgID: mid, Waiting: true}
	plMu.Unlock()
	return nil
}

func handlePlaylistListen(m *telegram.NewMessage) error {
	if m == nil || blocked(m) {
		return nil
	}
	text := strings.TrimSpace(m.Text())
	if text == "" || strings.HasPrefix(text, "/") {
		return nil
	}
	uid := userIDOf(m)
	inChat := m.ChatID()
	if id := m.ChannelID(); id != 0 {
		inChat = id
	}
	plMu.Lock()
	sess := plSess[uid]
	plMu.Unlock()
	if sess == nil || !samePlaylistChat(sess.ChatID, inChat) {
		return nil
	}
	if sess.Deleting && sess.PendingName == "" {
		_, _ = m.Delete()
		plMu.Lock()
		sess.PendingName = text
		plMu.Unlock()
		editPlaylistPanel(sess.ChatID, sess.MsgID, playlistSelectFormat(), playlistDeleteFormatMarkup())
		return nil
	}
	if !sess.Waiting || sess.Deleting {
		return nil
	}
	_, _ = m.Delete()
	editPlaylistPanel(sess.ChatID, sess.MsgID, playlistSearchingCaption(), playlistMarkup(uid))
	urlStr, title, durISO, thumb, _, err := searchSmart(text)
	if err != nil || urlStr == "" {
		editPlaylistPanel(sess.ChatID, sess.MsgID,
			"<blockquote><b>"+smallcaps("song not found")+"</b></blockquote>\n<blockquote expandable>"+smallcaps("send another song name")+"</blockquote>",
			playlistMarkup(uid))
		return nil
	}
	if title == "" {
		title = text
	}
	plMu.Lock()
	items := userPL[uid]
	if len(items) >= playlistLimit {
		plMu.Unlock()
		editPlaylistPanel(sess.ChatID, sess.MsgID,
			"<blockquote><b>"+smallcaps("playlist full")+"</b></blockquote>\n<blockquote expandable>"+smallcaps("max")+" 8 "+smallcaps("songs")+"</blockquote>",
			playlistMarkup(uid))
		return nil
	}
	userPL[uid] = append(items, plItem{Title: title, URL: urlStr, Dur: isoToHuman(durISO), Thumb: thumb})
	plMu.Unlock()
	editPlaylistPanel(sess.ChatID, sess.MsgID, playlistAddedCaption(), playlistMarkup(uid))
	return nil
}

func playlistChatID(cb *telegram.CallbackQuery) int64 {
	if cb == nil {
		return 0
	}
	chatID := cb.ChatID
	msg, _ := cb.GetMessage()
	if msg != nil {
		if id := msg.ChatID(); id != 0 {
			chatID = id
		}
		if id := msgBotChatID(msg); id != 0 {
			chatID = id
		}
	}
	return canonChatID(chatID)
}

func playlistIsPrivate(cb *telegram.CallbackQuery) bool {
	msg, _ := cb.GetMessage()
	if msg != nil && msg.IsPrivate() {
		return true
	}
	if cb.Sender != nil && (cb.ChatID == cb.Sender.ID || canonChatID(cb.ChatID) == cb.Sender.ID) {
		return true
	}
	return false
}

func removePlaylistSong(uid int64, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	plMu.Lock()
	defer plMu.Unlock()
	items := userPL[uid]
	for i, it := range items {
		t := strings.ToLower(it.Title)
		if strings.Contains(t, name) || strings.Contains(name, t) {
			userPL[uid] = append(items[:i], items[i+1:]...)
			return true
		}
	}
	return false
}

func handlePlaylistCallback(cb *telegram.CallbackQuery, data string) bool {
	if cb == nil || cb.Sender == nil {
		return false
	}
	alert := &telegram.CallbackOptions{Alert: true}
	uid := cb.Sender.ID
	switch {
	case data == "pl_done":
		plMu.Lock()
		s := plSess[uid]
		chatID, msgID := int64(0), 0
		if s != nil {
			chatID, msgID = s.ChatID, s.MsgID
			s.Waiting = false
			s.Deleting = false
			s.PendingName = ""
		}
		plMu.Unlock()
		_, _ = cb.Answer(smallcaps("closed"))
		deletePlaylistPanel(chatID, msgID)
		if msg, _ := cb.GetMessage(); msg != nil {
			_, _ = msg.Delete()
		}
		return true
	case data == "pl_del":
		plMu.Lock()
		s := plSess[uid]
		if s == nil {
			s = &plSession{ChatID: playlistChatID(cb)}
			plSess[uid] = s
		}
		s.Waiting = false
		s.Deleting = true
		s.PendingName = ""
		if s.ChatID == 0 {
			s.ChatID = playlistChatID(cb)
		}
		if s.MsgID == 0 {
			s.MsgID = int(cb.MessageID)
		}
		chatID, msgID := s.ChatID, s.MsgID
		plMu.Unlock()
		_, _ = cb.Answer("")
		editPlaylistPanel(chatID, msgID, playlistDeletePrompt(), [][]InlineBtn{
			{styleBtn(smallcaps("back"), "pl_delback", ColourBlue)},
		})
		return true
	case data == "pl_delback":
		plMu.Lock()
		s := plSess[uid]
		if s != nil {
			s.Deleting = false
			s.PendingName = ""
			s.Waiting = true
		}
		chatID, msgID := int64(0), 0
		if s != nil {
			chatID, msgID = s.ChatID, s.MsgID
		}
		plMu.Unlock()
		_, _ = cb.Answer("")
		editPlaylistPanel(chatID, msgID, playlistMainCaption(uid), playlistMarkup(uid))
		return true
	case data == "pl_delfmt:a" || data == "pl_delfmt:v":
		plMu.Lock()
		s := plSess[uid]
		name := ""
		chatID, msgID := int64(0), 0
		if s != nil {
			name = s.PendingName
			chatID, msgID = s.ChatID, s.MsgID
		}
		plMu.Unlock()
		_, _ = cb.Answer("")
		editPlaylistPanel(chatID, msgID, playlistDeleteProgress(), nil)
		ok := removePlaylistSong(uid, name)
		plMu.Lock()
		if s := plSess[uid]; s != nil {
			s.Deleting = false
			s.PendingName = ""
			s.Waiting = true
		}
		plMu.Unlock()
		caption := playlistMainCaption(uid)
		if !ok {
			caption = "<blockquote><b>" + smallcaps("song not in playlist") + "</b></blockquote>\n" + playlistPrompt()
		}
		editPlaylistPanel(chatID, msgID, caption, playlistMarkup(uid))
		return true
	case strings.HasPrefix(data, "pl_a:") || strings.HasPrefix(data, "pl_v:"):
		video := strings.HasPrefix(data, "pl_v:")
		idxStr := strings.TrimPrefix(data, "pl_a:")
		idxStr = strings.TrimPrefix(idxStr, "pl_v:")
		idx := 0
		fmt.Sscanf(idxStr, "%d", &idx)
		if playlistIsPrivate(cb) {
			_, _ = cb.Answer(smallcaps("play playlist in group"), alert)
			return true
		}
		chatID := playlistChatID(cb)
		plMu.Lock()
		items := userPL[uid]
		plMu.Unlock()
		if idx < 0 || idx >= len(items) {
			_, _ = cb.Answer(smallcaps("song missing"), alert)
			return true
		}
		it := items[idx]
		name := sanitizeDisplayName(cb.Sender.FirstName)
		song := Song{
			URL:         it.URL,
			Title:       it.Title,
			Duration:    it.Dur,
			Requester:   name,
			RequesterID: uid,
			Thumbnail:   it.Thumb,
			Video:       video,
		}
		_, _ = cb.Answer(smallcaps("playing"))
		go func() {
			_ = RoomPlay(chatID, song, false, nil)
		}()
		return true
	}
	return false
}
