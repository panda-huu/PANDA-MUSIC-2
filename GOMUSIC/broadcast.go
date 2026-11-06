package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

type broadcastResult struct {
	Sent   int
	Failed int
}

type botAPIResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

var broadcastHTTP = &http.Client{Timeout: 30 * time.Second}

func broadcastTargets(kind string) ([]int64, error) {
	if storeDB == nil {
		return nil, fmt.Errorf("database is not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var query string
	switch kind {
	case "users":
		query = "SELECT user_id FROM " + dbPrefix + "served_users WHERE user_id > 0 ORDER BY user_id"
	case "groups":
		query = "SELECT chat_id FROM " + dbPrefix + "served_chats WHERE chat_id < 0 ORDER BY chat_id"
	default:
		return nil, fmt.Errorf("invalid broadcast target")
	}
	rows, err := storeDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", kind, err)
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != 0 {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func canBroadcast(userID int64) bool {
	if isOwner(userID) {
		return true
	}
	if storeDB == nil || userID <= 0 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var allowed bool
	err := storeDB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM "+dbPrefix+"sudoers WHERE id='sudo' AND $1 = ANY(sudoers))",
		userID,
	).Scan(&allowed)
	return err == nil && allowed
}

func callBroadcastBotAPI(method string, values url.Values) error {
	if strings.TrimSpace(BotToken) == "" {
		return fmt.Errorf("BOT_TOKEN is missing")
	}
	endpoint := "https://api.telegram.org/bot" + BotToken + "/" + method

	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := broadcastHTTP.Do(req)
		if err != nil {
			cancel()
			return err
		}
		var result botAPIResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		cancel()
		if decodeErr != nil {
			return decodeErr
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && result.OK {
			return nil
		}
		if resp.StatusCode == http.StatusTooManyRequests && result.Parameters.RetryAfter > 0 && attempt == 0 {
			time.Sleep(time.Duration(result.Parameters.RetryAfter) * time.Second)
			continue
		}
		if result.Description == "" {
			result.Description = resp.Status
		}
		return fmt.Errorf("%s", result.Description)
	}
	return fmt.Errorf("Telegram rate limit retry exhausted")
}

func broadcastOne(target int64, m *telegram.NewMessage, text string, replyID int32) error {
	values := url.Values{"chat_id": {strconv.FormatInt(target, 10)}}
	if replyID != 0 {
		source := m.ChatID()
		if channelID := m.ChannelID(); channelID != 0 {
			source = channelID
		}
		values.Set("from_chat_id", strconv.FormatInt(canonChatID(source), 10))
		values.Set("message_id", strconv.FormatInt(int64(replyID), 10))
		return callBroadcastBotAPI("forwardMessage", values)
	}
	values.Set("text", text)
	return callBroadcastBotAPI("sendMessage", values)
}

func runBroadcast(m *telegram.NewMessage, mode string) error {
	if m == nil || !canBroadcast(userIDOf(m)) {
		return nil
	}
	text := cmdArgs(m)
	replyID := replyMsgID(m)
	if replyID == 0 && text == "" {
		_, _ = sendHTML(Bot, m.ChatID(),
			richHeading("broadcast usage", 3)+richNote(
				"<code>/broadcast text</code> — users + groups\n"+
					"<code>/ubroadcast text</code> — users only\n"+
					"<code>/gbroadcast text</code> — groups only\n\n"+
					"Or reply to any text/media message with one of these commands.",
			), nil)
		return nil
	}

	var userTargets, groupTargets []int64
	var err error
	if mode == "all" || mode == "users" {
		userTargets, err = broadcastTargets("users")
		if err != nil {
			_, _ = sendHTML(Bot, m.ChatID(), richHeading("broadcast failed", 3)+richNote(richEsc(err.Error())), nil)
			return nil
		}
	}
	if mode == "all" || mode == "groups" {
		groupTargets, err = broadcastTargets("groups")
		if err != nil {
			_, _ = sendHTML(Bot, m.ChatID(), richHeading("broadcast failed", 3)+richNote(richEsc(err.Error())), nil)
			return nil
		}
	}

	_, _ = m.Delete()
	status, _ := sendHTML(Bot, m.ChatID(), richHeading("broadcast started", 3)+richNote("Preparing recipients..."), nil)
	result := broadcastResult{}
	total := len(userTargets) + len(groupTargets)
	done := 0

	sendTargets := func(ids []int64) {
		for _, id := range ids {
			if err := broadcastOne(id, m, text, replyID); err != nil {
				result.Failed++
			} else {
				result.Sent++
			}
			done++
			if done%25 == 0 && status != nil {
				_ = editHTML(status, broadcastProgress(mode, result, done, total), nil)
			}
			time.Sleep(40 * time.Millisecond)
		}
	}
	sendTargets(userTargets)
	sendTargets(groupTargets)

	if status != nil {
		_ = editHTML(status, broadcastSummary(mode, result, total), nil)
	} else {
		_, _ = sendHTML(Bot, m.ChatID(), broadcastSummary(mode, result, total), nil)
	}
	return nil
}

func broadcastProgress(mode string, r broadcastResult, done, total int) string {
	return richHeading("broadcast in progress", 3) + richKVTable([][2]string{
		{"mode", "<code>" + richEsc(mode) + "</code>"},
		{"sent", fmt.Sprintf("<code>%d</code>", r.Sent)},
		{"failed", fmt.Sprintf("<code>%d</code>", r.Failed)},
		{"processed", fmt.Sprintf("<code>%d / %d</code>", done, total)},
	})
}

func broadcastSummary(mode string, r broadcastResult, total int) string {
	return richHeading("broadcast completed", 3) + richKVTable([][2]string{
		{"mode", "<code>" + richEsc(mode) + "</code>"},
		{"sent", fmt.Sprintf("<code>%d</code>", r.Sent)},
		{"failed", fmt.Sprintf("<code>%d</code>", r.Failed)},
		{"total tried", fmt.Sprintf("<code>%d</code>", total)},
	})
}

func handleBroadcast(m *telegram.NewMessage) error { return runBroadcast(m, "all") }
func handleUserBroadcast(m *telegram.NewMessage) error { return runBroadcast(m, "users") }
func handleGroupBroadcast(m *telegram.NewMessage) error { return runBroadcast(m, "groups") }
