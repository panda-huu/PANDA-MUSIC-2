package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nikhil390u8o/GOMUSICV2/ntgcalls"
)

type callAPI struct {
	*ntgcalls.Client
}

func (c *callAPI) Close() {
	if c != nil && c.Client != nil {
		c.Free()
	}
}

func (c *callAPI) SeekBy(chatID int64, ms int64) error {
	// CallbackQuery may expose a canonical -100... chat ID while playback
	// state was created from the raw channel ID. Resolve the real state key.
	playerID := playbackChatID(chatID)
	unlock := lockChatPlay(playerID)
	defer unlock()

	path := currentPath(playerID)
	current := peekCurrent(playerID)
	if path == "" && current != nil {
		path = current.FilePath
	}
	if path == "" {
		log.Printf("seek state missing callback_chat=%d resolved_chat=%d queue=%d local_calls=%v",
			chatID, playerID, queueSize(playerID), Calls.Calls())
		return fmt.Errorf("nothing playing (player state not found for this chat)")
	}

	video := current != nil && current.Video
	next := nextSeekSec(playerID, int(ms/1000))
	media := buildMedia(path, video, next)

	// Replacing FFmpeg sources emits a transient StreamEnd event. Mark the
	// operation as an intentional switch so the stream-end callback cannot
	// mistake the seek for a finished track and disconnect the assistant.
	holdSwitch(playerID)
	err := ntgPlay(playerID, media, video)
	releaseSwitch(playerID)
	if err != nil {
		log.Printf("seek source switch failed callback_chat=%d player_chat=%d target=%ds: %v",
			chatID, playerID, next, err)
		return fmt.Errorf("stream switch failed: %w", err)
	}

	setSeekState(playerID, next)
	bumpStream(playerID)
	log.Printf("seek applied callback_chat=%d player_chat=%d target=%ds video=%t",
		chatID, playerID, next, video)
	return nil
}

var (
	Bot               *telegram.Client
	Assistant         *telegram.Client
	Calls             *callAPI
	botStartTime      = time.Now()
	assistantUsername string
	botUsername       string
	assistantID       int64
	activeCalls       = map[int64]telegram.InputGroupCall{}
)

func waitReady(c *telegram.Client, label string) error {
	if c == nil {
		return fmt.Errorf("%s client is nil", label)
	}
	var last error
	for i := 1; i <= 8; i++ {
		_ = c.Connect()
		me, err := c.GetMe()
		if err == nil && me != nil {
			log.Printf("%s ready @%s id=%d\n", label, me.Username, me.ID)
			return nil
		}
		last = err
		log.Printf("%s GetMe retry %d/8: %v\n", label, i, err)
		time.Sleep(time.Duration(i) * time.Second)
	}
	return last
}

func initClients() error {
	if BotToken == "" {
		return fmt.Errorf("BOT_TOKEN is empty - set it in .env")
	}
	if StringSession == "" {
		return fmt.Errorf("STRING_SESSION is empty - set it in .env")
	}

	_ = os.MkdirAll("sessions", 0o755)
	botCache := telegram.NewCache("sessions/bot.cache", &telegram.CacheConfig{
		MaxSize:  5000,
		Memory:   false,
		Disabled: false,
	})
	botClient, err := telegram.NewClient(telegram.ClientConfig{
		AppID:           int32(APIID),
		AppHash:         APIHash,
		Session:         "sessions/bot.session",
		MemorySession:   false,
		ParseMode:       "HTML",
		NoPreconnect:    true,
		RawUpdates:      true,
		DisableGapFetch: true,
		Cache:           botCache,
	})
	if err != nil {
		return err
	}
	if err := botClient.Connect(); err != nil {
		return fmt.Errorf("bot connect: %w", err)
	}
	if err := botClient.LoginBot(BotToken); err != nil {
		return fmt.Errorf("bot login: %w", err)
	}
	time.Sleep(1500 * time.Millisecond)
	_ = botClient.Connect()
	Bot = botClient

	encoded, err := decodePyrogramSessionString(StringSession)
	if err != nil {
		return err
	}
	asst, err := telegram.NewClient(telegram.ClientConfig{
		AppID:           int32(APIID),
		AppHash:         APIHash,
		StringSession:   encoded,
		MemorySession:   true,
		ParseMode:       "HTML",
		NoPreconnect:    true,
		RawUpdates:      true,
		DisableGapFetch: true,
	})
	if err != nil {
		return err
	}
	if err := asst.Connect(); err != nil {
		return fmt.Errorf("assistant connect: %w", err)
	}
	time.Sleep(800 * time.Millisecond)
	_ = asst.Connect()
	Assistant = asst

	Calls = &callAPI{Client: ntgcalls.NTgCalls()}
	Calls.OnStreamEnd(func(chat int64, t ntgcalls.StreamType, d ntgcalls.StreamDevice) {
		if t == ntgcalls.VideoStream || d == ntgcalls.CameraStream || d == ntgcalls.ScreenStream {
			log.Println("ignore video/camera stream-end", chat)
			return
		}
		if isSwitching(chat) {
			return
		}
		connectMu.Lock()
		started := streamAt[chat]
		connectMu.Unlock()
		if isLiveSession(chat) && (started.IsZero() || time.Since(started) < 8*time.Second) {
			return
		}
		go handleStreamEnd(chat)
	})
	Calls.OnConnectionChange(func(chat int64, info ntgcalls.NetworkInfo) {
		if info.State == ntgcalls.Closed || info.State == ntgcalls.Failed {
			if isSwitching(chat) || isLiveSession(chat) {
				return
			}
		}
		connectMu.Lock()
		ch := connectWait[chat]
		connectMu.Unlock()
		if ch != nil {
			switch info.State {
			case ntgcalls.Connected:
				select {
				case ch <- nil:
				default:
				}
			case ntgcalls.Failed, ntgcalls.Timeout, ntgcalls.Closed:
				select {
				case ch <- fmt.Errorf("ntgcalls state %v", info.State):
				default:
				}
			}
			return
		}
		if info.State == ntgcalls.Timeout || info.State == ntgcalls.Failed || info.State == ntgcalls.Closed {
			if isSwitching(chat) || isLiveSession(chat) {
				return
			}
			song := peekCurrent(chat)
			if song == nil {
				return
			}
			log.Println("mid-stream disconnect, auto-reconnecting", chat, info.State)
			go func() {
				time.Sleep(1500 * time.Millisecond)
				clearLiveSession(chat)
				if Calls != nil {
					_ = Calls.Stop(chat)
				}
				connectMu.Lock()
				delete(joinParams, chat)
				connectMu.Unlock()
				media := buildMedia(song.FilePath, song.Video, getSeekState(chat))
				if err := joinExistingCall(chat, media, song.Video); err != nil {
					log.Println("auto-reconnect failed:", chat, err)
					leaveVCNow(chat)
				} else {
					markLiveSession(chat)
					log.Println("auto-reconnect success", chat)
				}
			}()
		}
	})
	return nil
}

func setBotCommands() {
	// Keep Telegram's command picker in sync with the handlers registered by
	// dispatchCommand. The previous short list hid many working commands.
	names := []struct{ command, description string }{
		{"start", "open the start menu"}, {"help", "show command help"},
		{"play", "play a song in the group voice chat"}, {"vplay", "play video in voice chat"},
		{"playlist", "manage your personal playlist"}, {"pause", "pause playback"},
		{"resume", "resume playback"}, {"skip", "skip the current song"},
		{"stop", "stop playback"}, {"clear", "clear the queue"}, {"queue", "show the queue"},
		{"ping", "check bot response time"}, {"id", "show user and chat IDs"},
		{"stats", "show bot statistics"}, {"nothumb", "toggle player artwork"},
		{"speed", "set playback speed"}, {"speedreset", "reset playback speed"},
		{"bass", "set bass level"}, {"bassoff", "turn bass off"},
		{"effecton", "enable audio effects"}, {"effectoff", "disable audio effects"},
		{"effects", "show audio effect settings"}, {"seek", "seek forward"},
		{"seekback", "seek backward"}, {"welcome", "show welcome settings"},
		{"setwelcome", "set the group welcome message"}, {"resetwelcome", "reset welcome message"},
		{"couple", "run the group couple game"},
		{"chaton", "enable ARU-AI chatbot"}, {"chatoff", "disable ARU-AI chatbot"},
	}
	cmds := make([]*telegram.BotCommand, 0, len(names))
	for _, item := range names {
		cmds = append(cmds, &telegram.BotCommand{Command: item.command, Description: item.description})
	}
	_, err := Bot.BotsSetBotCommands(&telegram.BotCommandScopeDefault{}, "", cmds)
	if err != nil {
		log.Println("Could not set bot commands:", err)
	}
}
