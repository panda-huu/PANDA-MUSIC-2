package main

import (
	"log"

	"github.com/amarnathcjd/gogram/telegram"
)

func registerCallbackFallback() {
	// handleCallbackQuery is already registered in registerHandlers.
	// A second Bot.On(callback) made every menu edit run twice and
	// the second pass dropped the blockquote.
	log.Println("Loaded module: callbacks")

	// Chatbot listener (/chaton, /chatoff, and AI replies).
	Bot.On(telegram.OnNewMessage, live(handleChatbotListen))
	log.Println("Loaded module: chatbot")
}
