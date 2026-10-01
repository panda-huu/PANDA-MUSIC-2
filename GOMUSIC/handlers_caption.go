package main

import (
	"github.com/amarnathcjd/gogram/telegram"
)

func playRepliedNow(m *telegram.NewMessage, video bool) error {
	if m == nil || !claimPlay(m) {
		return nil
	}
	chatID := m.ChatID()
	pm, _ := sendHTML(Bot, chatID, wrapBQ(smallcaps("downloading...")), nil)
	song, why := songFromReply(m, video)
	_, _ = m.Delete()
	if song != nil {
		return playPreparedMsg(m, *song, pm)
	}
	if pm != nil {
		_, _ = pm.Delete()
	}
	if why != "" {
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps(why)), nil)
	}
	return nil
}
