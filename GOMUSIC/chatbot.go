package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

// Chatbot: replies via ARU-AI API (POST {ARU_API_URL}/chat).
// Env: ARU_API_URL, ARU_API_KEY

var (
	chatbotMu      sync.RWMutex
	chatbotEnabled = map[int64]bool{}
	chatbotHTTP    = &http.Client{Timeout: 30 * time.Second}
)

var chatbotTriggers = []string{"panda", "music bot", "gomusic"}

type aruChatRequest struct {
	Text   string `json:"text"`
	ChatID string `json:"chat_id"`
}

type aruChatResponse struct {
	Reply string `json:"reply"`
	Error string `json:"error"`
}

func chatbotIsOn(chatID int64) bool {
	chatbotMu.RLock()
	defer chatbotMu.RUnlock()
	return chatbotEnabled[chatID]
}

func askARU(chatID int64, text string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("ARU_API_URL")), "/")
	key := strings.TrimSpace(os.Getenv("ARU_API_KEY"))
	if base == "" || key == "" {
		return "", fmt.Errorf("ARU_API_URL or ARU_API_KEY is not set")
	}

	body, err := json.Marshal(aruChatRequest{
		Text:   text,
		ChatID: strconv.FormatInt(chatID, 10),
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, base+"/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := chatbotHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out aruChatResponse
	_ = json.Unmarshal(raw, &out)

	if resp.StatusCode != http.StatusOK {
		if out.Error != "" {
			return "", fmt.Errorf("ARU %d: %s", resp.StatusCode, out.Error)
		}
		return "", fmt.Errorf("ARU returned status %d", resp.StatusCode)
	}
	return strings.TrimSpace(out.Reply), nil
}

// isReplyToBot returns true if m is a reply to a message sent by the bot.
func isReplyToBot(m *telegram.NewMessage) bool {
	if m == nil || m.Message == nil || m.Message.ReplyTo == nil {
		return false
	}
	replyID := replyMsgID(m)
	if replyID == 0 {
		return false
	}
	msgs, err := Bot.GetMessages(m.ChatID(), &telegram.SearchOption{IDs: []int32{replyID}})
	if err != nil || len(msgs) == 0 {
		return false
	}
	me, err := Bot.GetMe()
	if err != nil || me == nil {
		return false
	}
	return userIDOf(&msgs[0]) == me.ID
}

// chatbotShouldReply checks mention / reply-to-bot / trigger word.
func chatbotShouldReply(m *telegram.NewMessage) bool {
	text := strings.ToLower(strings.TrimSpace(m.Text()))

	if isReplyToBot(m) {
		return true
	}
	if u := strings.ToLower(strings.TrimSpace(botUsername)); u != "" {
		if strings.Contains(text, "@"+u) {
			return true
		}
	}
	if n := strings.ToLower(strings.TrimSpace(BotName)); len(n) > 2 {
		if strings.Contains(text, n) {
			return true
		}
	}
	for _, t := range chatbotTriggers {
		if strings.Contains(text, t) {
			return true
		}
	}
	return false
}

// sendTyping sends a typing chat action — signature: (InputPeer, int32, SendMessageAction).
func sendTyping(chatID int64) {
	peer, err := Bot.ResolvePeer(chatID)
	if err != nil || peer == nil {
		return
	}
	_, _ = Bot.MessagesSetTyping(peer, 0, &telegram.SendMessageTypingAction{})
}

func handleChatbotOn(m *telegram.NewMessage) error {
	_, _ = m.Delete()
	chatID := m.ChatID()

	if !m.IsPrivate() && !isAuthorized(m) {
		_, _ = sendHTML(Bot, chatID,
			richHeading("admin permission required", 3)+richNote("Only group admins can enable the chatbot."), nil)
		return nil
	}

	chatbotMu.Lock()
	already := chatbotEnabled[chatID]
	chatbotEnabled[chatID] = true
	chatbotMu.Unlock()

	if already {
		_, _ = sendHTML(Bot, chatID, richHeading("chatbot already on", 3), nil)
		return nil
	}
	_, _ = sendHTML(Bot, chatID,
		richHeading("chatbot enabled", 3)+richNote("Naam se bulao ya mention karo, main jawab dunga."), nil)
	return nil
}

func handleChatbotOff(m *telegram.NewMessage) error {
	_, _ = m.Delete()
	chatID := m.ChatID()

	if !m.IsPrivate() && !isAuthorized(m) {
		_, _ = sendHTML(Bot, chatID,
			richHeading("admin permission required", 3)+richNote("Only group admins can disable the chatbot."), nil)
		return nil
	}

	chatbotMu.Lock()
	was := chatbotEnabled[chatID]
	delete(chatbotEnabled, chatID)
	chatbotMu.Unlock()

	if !was {
		_, _ = sendHTML(Bot, chatID, richHeading("chatbot already off", 3), nil)
		return nil
	}
	_, _ = sendHTML(Bot, chatID, richHeading("chatbot disabled", 3), nil)
	return nil
}

// handleChatbotListen runs on every new message.
func handleChatbotListen(m *telegram.NewMessage) error {
	if m == nil || m.Message == nil {
		return nil
	}

	switch cmdRoot(m) {
	case "chaton":
		return handleChatbotOn(m)
	case "chatoff":
		return handleChatbotOff(m)
	}

	text := strings.TrimSpace(m.Text())
	if text == "" || strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!") || strings.HasPrefix(text, ".") {
		return nil
	}
	if blocked(m) {
		return nil
	}

	chatID := m.ChatID()
	if !chatbotIsOn(chatID) {
		return nil
	}

	isPrivate := m.IsPrivate()
	if !isPrivate && !chatbotShouldReply(m) {
		return nil
	}

	// capture before goroutine
	userID := userIDOf(m)
	name := senderFullName(m)
	authorized := isOwner(userID) || (!isPrivate && isAuthorized(m))

	sendTyping(chatID)

	go func() {
		role := "user"
		if isOwner(userID) {
			role = "owner"
		} else if authorized {
			role = "group admin"
		}

		prompt := fmt.Sprintf("[bot: %s | user: %s | role: %s] %s", BotName, name, role, text)

		stop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(4 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					sendTyping(chatID)
				case <-stop:
					return
				}
			}
		}()

		reply, err := askARU(chatID, prompt)
		close(stop)

		if err != nil {
			log.Println("[chatbot] ARU error:", err)
			return
		}
		if reply == "" {
			return
		}
		if _, err := sendHTML(Bot, chatID, richEsc(reply), nil); err != nil {
			log.Println("[chatbot] send failed:", err)
		}
	}()
	return nil
}
