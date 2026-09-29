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
	ChatID  int64
	MsgID   int
	Waiting bool
}

var (
	userPL = map[int64][]plItem{}
	plSess = map[int64]*plSession{}
	plMu   sync.Mutex
)

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

func playlistMarkup(uid int64) [][]InlineBtn {
	plMu.Lock()
	items := append([]plItem(nil), userPL[uid]...)
	plMu.Unlock()
	colors := []string{ColourRed, ColourBlue, ColourGreen}
	rows := [][]InlineBtn{
		{styleBtn(smallcaps("audio"), "noop", ColourGreen)},
	}
	for i, it := range items {
		label := shortTitle(it.Title, 22)
		if label == "" {
			label = smallcaps("song")
		}
		rows = append(rows, []InlineBtn{styleBtn(label, fmt.Sprintf("pl_a:%d", i), colors[i%3])})
	}
	rows = append(rows, []InlineBtn{styleBtn(smallcaps("video"), "noop", ColourRed)})
	for i, it := range items {
		label := shortTitle(it.Title, 22)
		if label == "" {
			label = smallcaps("song")
		}
		rows = append(rows, []InlineBtn{styleBtn(label, fmt.Sprintf("pl_v:%d", i), colors[(i+1)%3])})
	}
	rows = append(rows, []InlineBtn{styleBtn(smallcaps("close"), "pl_done", ColourRed)})
	return rows
}

func editPlaylistPanel(chatID int64, msgID int, inner string, rows [][]InlineBtn) {
	if chatID == 0 || msgID == 0 {
		return
	}
	caption := quotedCaption(inner)
	try := func(withStyle bool) error {
		body := map[string]any{
			"chat_id":    chatID,
			"message_id": msgID,
			"caption":    caption,
			"parse_mode": "HTML",
		}
		if kb := apiMarkup(rows, withStyle); kb != nil {
			body["reply_markup"] = kb
		}
		err := botAPICall("editMessageCaption", body)
		if err != nil {
			delete(body, "caption")
			body["text"] = caption
			err = botAPICall("editMessageText", body)
		}
		return err
	}
	if err := try(true); err == nil || (err != nil && strings.Contains(strings.ToLower(err.Error()), "not modified")) {
		return
	}
	_ = try(false)
}

func handlePlaylist(m *telegram.NewMessage) error {
	if blocked(m) {
		return nil
	}
	uid := userIDOf(m)
	chatID := m.ChatID()
	_, _ = m.Delete()
	photo := pickStartPhoto()
	content := playlistPrompt()
	if photo != "" {
		content = richImg(photo) + content
	}
	msg, err := sendHTML(Bot, chatID, content, gogramMarkup(playlistMarkup(uid)))
	if err != nil || msg == nil {
		_, _ = sendQuotedPhoto(chatID, playlistPrompt(), playlistMarkup(uid))
		return nil
	}
	plMu.Lock()
	plSess[uid] = &plSession{ChatID: chatID, MsgID: int(msg.ID), Waiting: true}
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
	plMu.Lock()
	sess := plSess[uid]
	plMu.Unlock()
	if sess == nil || !sess.Waiting || sess.ChatID != m.ChatID() {
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

func handlePlaylistCallback(cb *telegram.CallbackQuery, data string) bool {
	if cb == nil {
		return false
	}
	alert := &telegram.CallbackOptions{Alert: true}
	switch {
	case data == "pl_done":
		uid := cb.Sender.ID
		plMu.Lock()
		if s := plSess[uid]; s != nil {
			s.Waiting = false
		}
		plMu.Unlock()
		_, _ = cb.Answer(smallcaps("closed"))
		return true
	case strings.HasPrefix(data, "pl_a:") || strings.HasPrefix(data, "pl_v:"):
		video := strings.HasPrefix(data, "pl_v:")
		idxStr := strings.TrimPrefix(strings.TrimPrefix(data, "pl_a:"), "pl_v:")
		idx := 0
		fmt.Sscanf(idxStr, "%d", &idx)
		chatID := cb.ChatID
		if msg, _ := cb.GetMessage(); chatID == 0 && msg != nil {
			chatID = msg.ChatID()
		}
		if chatID >= 0 {
			_, _ = cb.Answer(smallcaps("play playlist in group"), alert)
			return true
		}
		plMu.Lock()
		items := userPL[cb.Sender.ID]
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
			RequesterID: cb.Sender.ID,
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
