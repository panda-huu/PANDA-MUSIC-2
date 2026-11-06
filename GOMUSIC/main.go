package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if err := loadConfig(); err != nil {
		log.Fatal(err)
	}
	if LoggerID == 0 {
		log.Println("LOGGER_ID / LOG_GROUP_ID is empty — start/play logs disabled")
	} else {
		log.Println("start/play logs will go to", LoggerID)
	}

	if err := startStore(); err != nil {
		log.Fatalf("PostgreSQL initialization failed: %v", err)
	}
	defer func() { if storeDB != nil { _ = storeDB.Close() } }()
	log.Println("PostgreSQL store ready.")

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("GOMUSICV2 is running"))
		})
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("OK"))
		})
		addr := fmt.Sprintf(":%d", Port)
		log.Println("health server on port", Port)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Println("health server:", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("GOMUSICV2 stopped")
		if Bot != nil {
			Bot.Stop()
		}
		if Assistant != nil {
			Assistant.Stop()
		}
		if Calls != nil {
			Calls.Close()
		}
		os.Exit(0)
	}()

	for {
		if err := runBot(); err != nil {
			wait := telegramFloodWait(err)
			if wait > 0 {
				log.Printf("Telegram FLOOD_WAIT detected; pausing retries for %s\\n", wait)
			} else {
				wait = 10 * time.Second
				log.Printf("bot crashed: %v — retrying in %s\\n", err, wait)
			}
			if Calls != nil {
				Calls.Close()
				Calls = nil
			}
			Bot = nil
			Assistant = nil
			cachedAssistant = nil
			time.Sleep(wait)
			continue
		}
		// clean exit (signal) — break
		break
	}
}

// runBot initialises clients, registers handlers, and blocks on Bot.Idle().
// Any panic from gogram's internal goroutines is caught here and returned
// as an error so the outer loop in main() can restart cleanly.
func runBot() (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()

	if err := initClients(); err != nil {
		return fmt.Errorf("client start failed: %w", err)
	}
	log.Println("Bot client started")

	me, err := retryGetMe(Bot, 5)
	if err != nil {
		return fmt.Errorf("bot GetMe failed: %w", err)
	}
	// Always use Telegram's live bot profile name in the /start menu.
	botUsername = strings.TrimSpace(me.Username)
	BotName = strings.TrimSpace(me.FirstName)
	if BotName == "" {
		BotName = strings.TrimSpace(me.Username)
	}
	if BotName == "" {
		BotName = "Music Bot"
	}
	log.Printf("Bot: @%s | display name: %s\\n", me.Username, BotName)
	setBotCommands()

	if am, err := retryGetMe(Assistant, 5); err != nil {
		log.Println("Assistant GetMe failed (bot will still run):", err)
		assistantUsername = "sykrs"
	} else {
		assistantUsername = am.Username
		assistantID = am.ID
		log.Printf("Assistant: @%s id=%d\n", assistantUsername, assistantID)
	}

	registerHandlers()
	registerVoiceChatHandlers()
	registerCallbackFallback()
	go warmLogger()
	log.Println("draining old telegram updates for 8s")
	go func() {
		time.Sleep(8 * time.Second)
		enableUpdates()
		log.Println("GOMUSICV2 is ready for commands")
	}()

	Bot.Idle()
	return nil
}

func retryGetMe(c *telegram.Client, tries int) (*telegram.UserObj, error) {
	var last error
	for i := 1; i <= tries; i++ {
		me, err := c.GetMe()
		if err == nil && me != nil {
			return me, nil
		}
		last = err
		log.Printf("GetMe retry %d/%d: %v\n", i, tries, err)
		time.Sleep(time.Duration(i) * 2 * time.Second)
	}
	return nil, last
}

// telegramFloodWait extracts Telegram's server-provided FLOOD_WAIT duration.
// Waiting prevents repeated AuthImportBotAuthorization calls during rate limits.
func telegramFloodWait(err error) time.Duration {
	if err == nil {
		return 0
	}
	match := regexp.MustCompile(`Please wait ([0-9]+) seconds`).FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0
	}
	seconds, parseErr := strconv.Atoi(match[1])
	if parseErr != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds+10) * time.Second
}
