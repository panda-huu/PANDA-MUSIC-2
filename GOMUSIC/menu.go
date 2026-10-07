package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

var (
	menuHTTP   = &http.Client{Timeout: 5 * time.Second}
	menuEditID sync.Map
)

func styleFromColour(colour string) *telegram.KeyboardButtonStyle {
	switch strings.ToLower(colour) {
	case ColourRed:
		return &telegram.KeyboardButtonStyle{BgDanger: true}
	case ColourGreen:
		return &telegram.KeyboardButtonStyle{BgSuccess: true}
	case ColourBlue:
		return &telegram.KeyboardButtonStyle{BgPrimary: true}
	default:
		return nil
	}
}

func gogramMarkup(rows [][]InlineBtn) telegram.ReplyMarkup {
	kb := telegram.NewKeyboard()
	for _, row := range rows {
		btns := make([]telegram.KeyboardInlineButton, 0, len(row))
		for _, b := range row {
			var btn telegram.KeyboardInlineButton
			if b.WebApp != "" {
				btn = telegram.Button.WebView(b.Text, b.WebApp)
			} else if b.URL != "" {
				btn = telegram.Button.URL(b.Text, b.URL)
			} else {
				btn = telegram.Button.Data(b.Text, b.Data)
			}
			if st := styleFromColour(b.Colour); st != nil {
				btn.Style = st
			}
			btns = append(btns, btn)
		}
		if len(btns) > 0 {
			kb.AddRow(btns...)
		}
	}
	return kb.Build()
}

func apiMarkup(rows [][]InlineBtn, withStyle bool) map[string]any {
	var out [][]map[string]string
	for _, row := range rows {
		var btns []map[string]string
		for _, b := range row {
			item := map[string]string{"text": b.Text}
			if b.URL != "" {
				item["url"] = b.URL
			} else {
				item["callback_data"] = b.Data
			}
			if withStyle && b.Colour != "" {
				item["style"] = colourToStyle(b.Colour)
			}
			btns = append(btns, item)
		}
		if len(btns) > 0 {
			out = append(out, btns)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return map[string]any{"inline_keyboard": out}
}

func botAPICall(method string, body map[string]any) error {
	_, err := botAPICallID(method, body)
	return err
}

func botAPICallID(method string, body map[string]any) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	resp, err := menuHTTP.Post("https://api.telegram.org/bot"+BotToken+"/"+method, "application/json", bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(out, &parsed)
	if !parsed.OK {
		desc := parsed.Description
		if desc == "" {
			desc = string(out)
		}
		return 0, fmt.Errorf("%s", desc)
	}
	return parsed.Result.MessageID, nil
}

func quotedCaption(inner string) string {
	inner = strings.TrimSpace(inner)
	if strings.Contains(inner, "<blockquote") {
		return inner
	}
	return "<blockquote>" + inner + "</blockquote>"
}

func menuEditIDs(chatID int64, msgID int) []int64 {
	seen := map[int64]bool{}
	var out []int64
	add := func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	if v, ok := menuEditID.Load(msgID); ok {
		if id, _ := v.(int64); id != 0 {
			add(id)
		}
	}
	add(canonChatID(chatID))
	add(chatID)
	return out
}

func fastEditCaption(chatID int64, msgID int, inner string, rows [][]InlineBtn) bool {
	if chatID == 0 || msgID == 0 {
		return false
	}
	caption := quotedCaption(inner)
	kb := apiMarkup(rows, true)
	for _, id := range menuEditIDs(chatID, msgID) {
		body := map[string]any{
			"chat_id":    id,
			"message_id": msgID,
			"caption":    caption,
			"parse_mode": "HTML",
		}
		if kb != nil {
			body["reply_markup"] = kb
		}
		err := botAPICall("editMessageCaption", body)
		if err == nil || (err != nil && strings.Contains(strings.ToLower(err.Error()), "not modified")) {
			menuEditID.Store(msgID, id)
			return true
		}
	}
	return false
}

func editStartCaption(chatID int64, msgID int, inner string, rows [][]InlineBtn) {
	_ = fastEditCaption(chatID, msgID, inner, rows)
}

func sendQuotedPhoto(chatID int64, inner string, rows [][]InlineBtn) (*telegram.NewMessage, error) {
	if chatID == 0 {
		return nil, fmt.Errorf("chat id missing")
	}
	caption := quotedCaption(inner)
	photo := pickStartPhoto()
	apiChatID := canonChatID(chatID)
	body := map[string]any{
		"chat_id":    apiChatID,
		"photo":      photo,
		"caption":    caption,
		"parse_mode": "HTML",
	}
	if kb := apiMarkup(rows, true); kb != nil {
		body["reply_markup"] = kb
	}
	if id, err := botAPICallID("sendPhoto", body); err == nil {
		menuEditID.Store(id, apiChatID)
		return nil, nil
	}
	msg, err := Bot.SendMedia(chatID, photo, &telegram.MediaOptions{
		Caption:     caption,
		ParseMode:   "HTML",
		ReplyMarkup: gogramMarkup(rows),
	})
	if err == nil && msg != nil {
		menuEditID.Store(int(msg.ID), canonChatID(chatID))
		return msg, nil
	}
	log.Println("sendPhoto failed:", err)
	return Bot.SendMessage(chatID, caption, &telegram.SendOptions{
		ParseMode:   "HTML",
		ReplyMarkup: gogramMarkup(rows),
	})
}

func showQuotedMenu(cb *telegram.CallbackQuery, inner string, rows [][]InlineBtn) {
	if cb == nil {
		return
	}
	if editQuotedCaption(cb, inner, rows) {
		return
	}
	chatID := cb.ChatID
	if chatID == 0 {
		if msg, _ := cb.GetMessage(); msg != nil {
			chatID = msg.ChatID()
		}
	}
	_, _ = sendQuotedPhoto(chatID, inner, rows)
}
