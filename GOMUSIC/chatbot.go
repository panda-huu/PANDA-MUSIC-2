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

// chatbotShouldReply checks if bot was mentioned, replied to, or name triggered.
func chatbotShouldReply(m *telegram.NewMessage) bool {
	text := strings.ToLower(strings.TrimSpace(m.Text()))

	// 1. reply to bot message
	if m.Message != nil && m.Message.ReplyTo != nil {
		replyID := m.Message.ReplyTo.ReplyToMsgID
		if replyID != 0 {
			// fetch the replied-to message sender
			msgs, err := Bot.GetMessages(m.ChatID(), &telegram.InputMessageID{ID: replyID})
			if err == nil {
				for _, msg := range msgs {
					if mm, ok := msg.(*telegram.MessageObj); ok {
						if from, ok2 := mm.FromID.(*telegram.PeerUser); ok2 {
							me, _ := Bot.GetMe()
							if me != nil && from.UserID == me.ID {
								return true
							}
						}
					}
				}
			}
		}
	}

	// 2. @mention in text
	if u := strings.ToLower(strings.TrimSpace(botUsername)); u != "" {
		if strings.Contains(text, "@"+u) {
			return true
		}
	}

	// 3. bot display name in text
	if n := strings.ToLower(strings.TrimSpace(BotName)); n != "" && len(n) > 2 {
		if strings.Contains(text, n) {
			return true
		}
	}

	// 4. hardcoded triggers
	for _, t := range chatbotTriggers {
		if strings.Contains(text, t) {
			return true
		}
	}

	return false
}

func sendTyping(chatID int64) {
	_, _ = Bot.MessagesSetTyping(chatID, 0, &telegram.SendMessageTypingAction{})
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

	// group: only reply if bot was mentioned / replied-to / name triggered
	isPrivate := m.IsPrivate()
	if !isPrivate && !chatbotShouldReply(m) {
		return nil
	}

	// capture everything needed before goroutine
	userID := userIDOf(m)
	name := senderFullName(m)
	authorized := isOwner(userID) || (!isPrivate && isAuthorized(m))

	// typing action — show immediately, before ARU call
	sendTyping(chatID)

	go func() {
		role := "user"
		if isOwner(userID) {
			role = "owner"
		} else if authorized {
			role = "group admin"
		}

		prompt := fmt.Sprintf("[bot: %s | user: %s | role: %s] %s", BotName, name, role, text)

		// keep sending typing every 4s while waiting for ARU
		stop := make(chan struct{})
		go func() {
			t := time.NewTicker(4 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-t.C:
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
