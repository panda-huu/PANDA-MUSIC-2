package main

import (
	"os"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nikhil390u8o/GOMUSICV2/GOMUSIC/GAMES"
)

func handleCouple(m *telegram.NewMessage) error {
	if m == nil || blocked(m) {
		return nil
	}
	chatID := m.ChatID()
	if m.IsPrivate() {
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("use /couple in a group")), nil)
		return nil
	}
	_, _ = m.Delete()
	pm, _ := sendHTML(Bot, chatID, wrapBQ(smallcaps("finding today's pair...")), nil)
	pair := games.Pick(Bot, chatID)
	if pm != nil {
		_, _ = pm.Delete()
	}
	if pair.ErrText != "" {
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps(pair.ErrText)), nil)
		return nil
	}
	var markup telegram.ReplyMarkup
	if u := strings.TrimPrefix(strings.TrimSpace(OwnerUsername), "@"); u != "" {
		markup = gogramMarkup([][]InlineBtn{{styleURLBtn(smallcaps("owner"), "https://t.me/"+u, ColourBlue)}})
	}
	cap := "<blockquote>" + pair.Caption + "</blockquote>"
	_, err := Bot.SendMedia(chatID, pair.Image, &telegram.MediaOptions{
		Caption:     cap,
		ParseMode:   "HTML",
		ReplyMarkup: markup,
	})
	if err != nil {
		_, _ = sendHTML(Bot, chatID, cap, markup)
	}
	if pair.Image != "" {
		_ = os.Remove(pair.Image)
	}
	return nil
}
