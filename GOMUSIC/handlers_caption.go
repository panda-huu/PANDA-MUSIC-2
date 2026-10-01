package main

import (
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
)

func playRepliedNow(m *telegram.NewMessage, video bool) error {
	if m == nil {
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

func handleCaptionPlay(m *telegram.NewMessage) error {
	if m == nil || m.IsPrivate() || blocked(m) {
		return nil
	}
	root := cmdRoot(m)
	if root == "vplya" || (strings.HasPrefix(root, "vplay") && root != "vplay") {
		return playRepliedNow(m, true)
	}
	return nil
}
